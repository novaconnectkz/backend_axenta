package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/middleware"
)

// LifecyclePoint — одна точка timeseries (день).
type LifecyclePoint struct {
	Date    string `json:"date"`    // YYYY-MM-DD
	Created int    `json:"created"` // создано за день
	Deleted int    `json:"deleted"` // удалено за день
}

// LifecycleSource — серия created/deleted per source.
type LifecycleSource struct {
	Key          string           `json:"key"`           // axenta | wialon | all
	Label        string           `json:"label"`
	Points       []LifecyclePoint `json:"points"`        // per-day
	TotalCreated int              `json:"total_created"` // SUM за период
	TotalDeleted int              `json:"total_deleted"`
}

// LifecycleResponse — payload /api/auth/dashboard/lifecycle.
type LifecycleResponse struct {
	Sources     []LifecycleSource `json:"sources"`
	Total       LifecycleSource   `json:"total"`
	Period      string            `json:"period"`
	From        time.Time         `json:"from"`
	To          time.Time         `json:"to"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// GetDashboardLifecycle возвращает per-day created/deleted объектов
// по источникам Axenta + Wialon за период.
//
// GET /api/auth/dashboard/lifecycle?period=7d|30d|90d
//
// Источники:
//   - Axenta: COUNT по axenta_object_snapshots.axenta_created_at /
//     axenta_deleted_at (заполнены с 2024-03-14)
//   - Wialon: SUM(units_created/units_deleted) из wialon_daily_snapshots
//     (per-day агрегаты, заполняются on-demand backfill'ом)
func GetDashboardLifecycle(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных компании",
		})
		return
	}
	publicDB := publicDBOrTenant(tenantDB)

	period := c.DefaultQuery("period", "30d")
	days := lifecycleDays(period)

	now := time.Now()
	loc := now.Location()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -days)

	axenta := buildAxentaLifecycle(tenantDB, from, days, loc)
	wialon := buildWialonLifecycle(publicDB, from, days, loc)
	total := combineLifecycle(from, days, loc, axenta, wialon)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": LifecycleResponse{
			Sources:     []LifecycleSource{axenta, wialon},
			Total:       total,
			Period:      period,
			From:        from,
			To:          to,
			GeneratedAt: now,
		},
	})
}

func lifecycleDays(period string) int {
	switch period {
	case "7d":
		return 7
	case "1m":
		return 30
	case "3m":
		return 90
	case "6m":
		return 180
	case "1y":
		return 365
	// legacy aliases
	case "30d":
		return 30
	case "90d":
		return 90
	default:
		return 30
	}
}

// initLifecyclePoints создаёт пустой массив точек на [days] дней от from.
func initLifecyclePoints(from time.Time, days int, loc *time.Location) []LifecyclePoint {
	pts := make([]LifecyclePoint, days)
	for i := 0; i < days; i++ {
		d := from.AddDate(0, 0, i).In(loc)
		pts[i] = LifecyclePoint{Date: d.Format("2006-01-02")}
	}
	return pts
}

// buildAxentaLifecycle — created/deleted из axenta_object_snapshots.
func buildAxentaLifecycle(db *gorm.DB, from time.Time, days int, loc *time.Location) LifecycleSource {
	src := LifecycleSource{
		Key:    "axenta",
		Label:  "Axenta",
		Points: initLifecyclePoints(from, days, loc),
	}

	type dayRow struct {
		Day   time.Time
		Count int
	}

	to := from.AddDate(0, 0, days)

	// Created — не фильтруем по GORM soft-delete (Шаг 3 cleanup
	// в sync soft-удаляет исчезнувшие, но lifecycle событие creation
	// произошло реально и должно учитываться).
	var created []dayRow
	db.Raw(`
		SELECT date_trunc('day', axenta_created_at AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM axenta_object_snapshots
		WHERE axenta_created_at IS NOT NULL
		  AND axenta_created_at >= ?
		  AND axenta_created_at < ?
		GROUP BY 1
	`, from, to).Scan(&created)

	// Deleted
	var deleted []dayRow
	db.Raw(`
		SELECT date_trunc('day', axenta_deleted_at AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM axenta_object_snapshots
		WHERE axenta_deleted_at IS NOT NULL
		  AND axenta_deleted_at >= ?
		  AND axenta_deleted_at < ?
		GROUP BY 1
	`, from, to).Scan(&deleted)

	for _, r := range created {
		if i := dateIndex(from, r.Day, days); i >= 0 {
			src.Points[i].Created = r.Count
			src.TotalCreated += r.Count
		}
	}
	for _, r := range deleted {
		if i := dateIndex(from, r.Day, days); i >= 0 {
			src.Points[i].Deleted = r.Count
			src.TotalDeleted += r.Count
		}
	}
	return src
}

// buildWialonLifecycle — SUM(units_created/units_deleted) per day из
// wialon_daily_snapshots (глобальная public-таблица).
func buildWialonLifecycle(db *gorm.DB, from time.Time, days int, loc *time.Location) LifecycleSource {
	src := LifecycleSource{
		Key:    "wialon",
		Label:  "Wialon",
		Points: initLifecyclePoints(from, days, loc),
	}

	type dayRow struct {
		Day     time.Time
		Created int
		Deleted int
	}

	to := from.AddDate(0, 0, days)

	// Wialon API не возвращает avl_unit_created/deleted для большинства
	// аккаунтов — эти поля в wialon_daily_snapshots обычно 0. Поэтому
	// derive created/deleted из дельты total_units день-к-дню через LAG.
	// Net positive = created, net negative = deleted. Не различает одновременные
	// create+delete в один день (показывает только net), но лучше чем нули.
	// Берём previous_day сразу за окно (snapshot_date >= from-1) чтобы первый
	// день периода имел корректную дельту.
	var rows []dayRow
	db.Raw(`
		WITH daily AS (
			SELECT snapshot_date::date AS day,
			       SUM(total_units) AS total
			FROM `+publicTable(db, "wialon_daily_snapshots")+`
			WHERE snapshot_date >= ? AND snapshot_date < ?
			GROUP BY snapshot_date
		),
		lagged AS (
			SELECT day, total,
			       total - LAG(total) OVER (ORDER BY day) AS delta
			FROM daily
		)
		SELECT day::timestamp AS day,
		       GREATEST(COALESCE(delta, 0), 0)::int AS created,
		       GREATEST(-COALESCE(delta, 0), 0)::int AS deleted
		FROM lagged
		WHERE day >= ? AND day < ?
	`, from.AddDate(0, 0, -1), to, from, to).Scan(&rows)

	for _, r := range rows {
		if i := dateIndex(from, r.Day, days); i >= 0 {
			src.Points[i].Created = r.Created
			src.Points[i].Deleted = r.Deleted
			src.TotalCreated += r.Created
			src.TotalDeleted += r.Deleted
		}
	}
	return src
}

// combineLifecycle складывает per-day значения по источникам.
func combineLifecycle(from time.Time, days int, loc *time.Location, srcs ...LifecycleSource) LifecycleSource {
	total := LifecycleSource{
		Key:    "all",
		Label:  "Все",
		Points: initLifecyclePoints(from, days, loc),
	}
	for _, s := range srcs {
		for i, p := range s.Points {
			if i >= days {
				break
			}
			total.Points[i].Created += p.Created
			total.Points[i].Deleted += p.Deleted
		}
		total.TotalCreated += s.TotalCreated
		total.TotalDeleted += s.TotalDeleted
	}
	return total
}

// dateIndex возвращает offset (дни) от from до t, или -1 если вне диапазона.
func dateIndex(from time.Time, t time.Time, days int) int {
	delta := t.Sub(from).Hours() / 24
	idx := int(delta + 0.5) // round для ловли DST/округлений
	if idx < 0 || idx >= days {
		return -1
	}
	return idx
}
