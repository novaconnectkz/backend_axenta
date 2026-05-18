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
	companyID := middleware.GetCompanyID(c)

	axenta := buildAxentaLifecycle(tenantDB, from, days, loc)
	wh := buildWialonLifecycleByType(publicDB, "hosting", "wh", "Wialon Hosting", from, days, loc)
	wl := buildWialonLifecycleByType(publicDB, "local", "wl", "Wialon Local", from, days, loc)
	skif := buildSkifLifecycle(publicDB, companyID, from, days, loc)
	gelios := buildGeliosLifecycle(publicDB, companyID, from, days, loc)
	total := combineLifecycle(from, days, loc, axenta, wh, wl, skif, gelios)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": LifecycleResponse{
			Sources:     []LifecycleSource{axenta, wh, wl, skif, gelios},
			Total:       total,
			Period:      period,
			From:        from,
			To:          to,
			GeneratedAt: now,
		},
	})
}

// buildSkifLifecycle — created proxy через skif_units.created_at per day.
// SKIF API не возвращает per-day creation history, поэтому используем
// "когда впервые увидели юнит" (как у Wialon fallback).
// Deleted = 0 пока не введём soft-delete для skif_units (TODO).
func buildSkifLifecycle(db *gorm.DB, companyID uint, from time.Time, days int, loc *time.Location) LifecycleSource {
	src := LifecycleSource{
		Key:    "skif",
		Label:  "SKIF",
		Points: initLifecyclePoints(from, days, loc),
	}
	if companyID == 0 {
		return src
	}

	type dayRow struct {
		Day     time.Time
		Created int
		Deleted int
	}
	to := from.AddDate(0, 0, days)

	// Prefer skif_daily_snapshots если есть данные за период (точная история
	// из POST /company/updates/query, заполняется backfill'ом).
	var snapshots []dayRow
	db.Raw(`
		SELECT snapshot_date::timestamp AS day,
		       COALESCE(SUM(units_created), 0) AS created,
		       COALESCE(SUM(units_deleted), 0) AS deleted
		FROM `+publicTable(db, "skif_daily_snapshots")+` sds
		JOIN `+publicTable(db, "skif_connections")+` sc ON sc.id = sds.connection_id
		WHERE sc.company_id = ?
		  AND sds.snapshot_date >= ? AND sds.snapshot_date < ?
		GROUP BY snapshot_date
	`, companyID, from, to).Scan(&snapshots)

	if len(snapshots) > 0 {
		for _, r := range snapshots {
			if i := dateIndex(from, r.Day, days); i >= 0 {
				src.Points[i].Created = r.Created
				src.Points[i].Deleted = r.Deleted
				src.TotalCreated += r.Created
				src.TotalDeleted += r.Deleted
			}
		}
		return src
	}

	// Fallback: skif_units.created_at + skif_deleted_at (proxy без backfill).
	type cntRow struct {
		Day   time.Time
		Count int
	}
	var created []cntRow
	db.Raw(`
		SELECT date_trunc('day', created_at AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM `+publicTable(db, "skif_units")+`
		WHERE company_id = ?
		  AND created_at >= ?
		  AND created_at < ?
		GROUP BY 1
	`, companyID, from, to).Scan(&created)

	var deleted []cntRow
	db.Raw(`
		SELECT date_trunc('day', skif_deleted_at AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM `+publicTable(db, "skif_units")+`
		WHERE company_id = ?
		  AND skif_deleted_at IS NOT NULL
		  AND skif_deleted_at >= ?
		  AND skif_deleted_at < ?
		GROUP BY 1
	`, companyID, from, to).Scan(&deleted)

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

// buildGeliosLifecycle — created/deleted proxy через gelios_units.
// GELIOS не имеет operation-log API (нет skif_daily_snapshots-аналога),
// поэтому строим напрямую из реестра:
//   - created = COALESCE(gelios_created_at, created_at) per day (реальная
//     дата создания в GELIOS, fallback на наш created_at)
//   - deleted = gelios_deleted_at per day (момент подтверждённого
//     исчезновения из выгрузки, НЕ реального удаления в GELIOS)
// Прошлое до первого sync невосстановимо (forward-сбор).
func buildGeliosLifecycle(db *gorm.DB, companyID uint, from time.Time, days int, loc *time.Location) LifecycleSource {
	src := LifecycleSource{
		Key:    "gelios",
		Label:  "GELIOS",
		Points: initLifecyclePoints(from, days, loc),
	}
	if companyID == 0 {
		return src
	}

	type cntRow struct {
		Day   time.Time
		Count int
	}
	to := from.AddDate(0, 0, days)

	var created []cntRow
	db.Raw(`
		SELECT date_trunc('day', COALESCE(gelios_created_at, created_at) AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM `+publicTable(db, "gelios_units")+`
		WHERE company_id = ?
		  AND COALESCE(gelios_created_at, created_at) >= ?
		  AND COALESCE(gelios_created_at, created_at) < ?
		GROUP BY 1
	`, companyID, from, to).Scan(&created)

	var deleted []cntRow
	db.Raw(`
		SELECT date_trunc('day', gelios_deleted_at AT TIME ZONE 'UTC') AS day,
		       COUNT(*) AS count
		FROM `+publicTable(db, "gelios_units")+`
		WHERE company_id = ?
		  AND gelios_deleted_at IS NOT NULL
		  AND gelios_deleted_at >= ?
		  AND gelios_deleted_at < ?
		GROUP BY 1
	`, companyID, from, to).Scan(&deleted)

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

// buildWialonLifecycleByType — derive created/deleted per-type.
//
// Стратегия:
//  1. Если есть строки в wialon_daily_snapshots за период → LAG total_units
//     (точная динамика +/- net change).
//  2. Иначе fallback на wialon_units.created_at COUNT per day (proxy
//     «когда впервые увидели юнит», deleted=0).
//
// WL обычно попадает в fallback: backfill пропускается для shared-connections
// (recursive=1 от top-resource считает всю иерархию, см. CLAUDE.md).
func buildWialonLifecycleByType(db *gorm.DB, connType, key, label string, from time.Time, days int, loc *time.Location) LifecycleSource {
	src := LifecycleSource{
		Key:    key,
		Label:  label,
		Points: initLifecyclePoints(from, days, loc),
	}

	type dayRow struct {
		Day     time.Time
		Created int
		Deleted int
	}
	to := from.AddDate(0, 0, days)

	var rows []dayRow
	// SUM по latest row per (connection, day): иначе разные account_wialon_id одного
	// connection двоят total_units и LAG-дельта получается мусором.
	db.Raw(`
		WITH latest AS (
			SELECT DISTINCT ON (connection_id, snapshot_date)
			       connection_id, snapshot_date, total_units
			FROM `+publicTable(db, "wialon_daily_snapshots")+`
			WHERE snapshot_date >= ? AND snapshot_date < ?
			ORDER BY connection_id, snapshot_date, updated_at DESC, id DESC
		),
		daily AS (
			SELECT latest.snapshot_date::date AS day,
			       SUM(latest.total_units) AS total
			FROM latest
			JOIN `+publicTable(db, "wialon_connections")+` wc ON wc.id = latest.connection_id
			WHERE wc.connection_type = ?
			GROUP BY latest.snapshot_date
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
	`, from.AddDate(0, 0, -1), to, connType, from, to).Scan(&rows)

	hasSnapshotData := false
	for _, r := range rows {
		if r.Created > 0 || r.Deleted > 0 {
			hasSnapshotData = true
			break
		}
	}

	// Fallback: COUNT wialon_units по created_at per day (когда впервые увидели юнит).
	// Используется когда нет данных в wialon_daily_snapshots за период (типичный
	// случай для WL where backfill пропущен).
	if !hasSnapshotData {
		type proxyRow struct {
			Day   time.Time
			Count int
		}
		var proxy []proxyRow
		db.Raw(`
			SELECT date_trunc('day', wu.created_at AT TIME ZONE 'UTC') AS day,
			       COUNT(*) AS count
			FROM `+publicTable(db, "wialon_units")+` wu
			JOIN `+publicTable(db, "wialon_connections")+` wc ON wc.id = wu.connection_id
			WHERE wc.connection_type = ?
			  AND wu.created_at >= ?
			  AND wu.created_at < ?
			GROUP BY 1
		`, connType, from, to).Scan(&proxy)
		for _, r := range proxy {
			if i := dateIndex(from, r.Day, days); i >= 0 {
				src.Points[i].Created = r.Count
				src.TotalCreated += r.Count
			}
		}
		return src
	}

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
