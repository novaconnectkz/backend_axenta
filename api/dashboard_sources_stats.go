package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/database"
	"backend_axenta/middleware"
)

// SourceObjectStats — агрегаты по объектам одной системы.
type SourceObjectStats struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Inactive int `json:"inactive"`
	Deleted  int `json:"deleted"` // Корзина: только Axenta её отслеживает, для WH/WL = 0
}

// SourceAccountStats — агрегаты по учётным записям одной системы.
type SourceAccountStats struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Blocked  int `json:"blocked"`
	Clients  int `json:"clients"`
	Partners int `json:"partners"`
}

// SourceStats — статистика одной внешней системы (Axenta, WH или WL).
type SourceStats struct {
	Key      string             `json:"key"`   // "axenta" | "wh" | "wl" | "all"
	Label    string             `json:"label"` // "Axenta" | "Wialon Hosting" | "Wialon Local" | "Все"
	Objects  SourceObjectStats  `json:"objects"`
	Accounts SourceAccountStats `json:"accounts"`
}

// SourcesStatsResponse — payload /api/auth/dashboard/sources-stats.
type SourcesStatsResponse struct {
	Sources []SourceStats `json:"sources"`
	Total   SourceStats   `json:"total"`
}

// GetDashboardSourcesStats возвращает агрегированную статистику по 3 источникам:
// Axenta, Wialon Hosting (WH) и Wialon Local (WL). Используется dashboard-карточками
// «Активные объекты», «Учётные записи» (combined) и кликабельным донатом «Статус объектов».
//
// GET /api/auth/dashboard/sources-stats
//
// Источники данных:
//   - Axenta: axenta_object_snapshots + axenta_account_snapshots (наполняются Axenta Sync, ~1ч TTL)
//   - WH/WL объекты: wialon_object_stats JOIN wialon_connections (наполняется WialonStatsScheduler)
//   - WH/WL аккаунты: Redis-cache /wialon/all-accounts (5 мин TTL, наполняется scheduler-refresh)
//
// Если кэш Wialon пуст — вернёт нули по WH/WL (без блокировки на 5+ минут на live-fetch).
// Свежие данные появятся при следующем срабатывании scheduler.
func GetDashboardSourcesStats(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		tenantDB = database.DB
	}
	companyID := middleware.GetCompanyID(c)

	axenta := buildAxentaSourceStats(tenantDB)
	wh, wl := buildWialonSourceStats(companyID)

	total := SourceStats{Key: "all", Label: "Все"}
	for _, s := range []SourceStats{axenta, wh, wl} {
		total.Objects.Total += s.Objects.Total
		total.Objects.Active += s.Objects.Active
		total.Objects.Inactive += s.Objects.Inactive
		total.Objects.Deleted += s.Objects.Deleted
		total.Accounts.Total += s.Accounts.Total
		total.Accounts.Active += s.Accounts.Active
		total.Accounts.Blocked += s.Accounts.Blocked
		total.Accounts.Clients += s.Accounts.Clients
		total.Accounts.Partners += s.Accounts.Partners
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": SourcesStatsResponse{
			Sources: []SourceStats{axenta, wh, wl},
			Total:   total,
		},
	})
}

// buildAxentaSourceStats — агрегирует объекты и аккаунты из локальных snapshot-таблиц Axenta.
func buildAxentaSourceStats(db *gorm.DB) SourceStats {
	s := SourceStats{Key: "axenta", Label: "Axenta"}

	// Объекты: total/active/inactive не считают объекты в корзине;
	// deleted = объекты в корзине (scheduled_delete_at OR axenta_deleted_at).
	type objCounts struct {
		Total    int64
		Active   int64
		Inactive int64
		Deleted  int64
	}
	var oc objCounts
	if err := db.Raw(`
		SELECT
			COUNT(*) FILTER (
				WHERE deleted_at IS NULL
				  AND scheduled_delete_at IS NULL
				  AND axenta_deleted_at IS NULL
			) AS total,
			COUNT(*) FILTER (
				WHERE deleted_at IS NULL
				  AND scheduled_delete_at IS NULL
				  AND axenta_deleted_at IS NULL
				  AND is_active = true
			) AS active,
			COUNT(*) FILTER (
				WHERE deleted_at IS NULL
				  AND scheduled_delete_at IS NULL
				  AND axenta_deleted_at IS NULL
				  AND is_active = false
			) AS inactive,
			COUNT(*) FILTER (
				WHERE scheduled_delete_at IS NOT NULL
				   OR axenta_deleted_at IS NOT NULL
			) AS deleted
		FROM axenta_object_snapshots
	`).Scan(&oc).Error; err != nil {
		log.Printf("⚠️ sources-stats axenta objects: %v", err)
	}

	s.Objects.Total = int(oc.Total)
	s.Objects.Active = int(oc.Active)
	s.Objects.Inactive = int(oc.Inactive)
	s.Objects.Deleted = int(oc.Deleted)

	// Учётные записи Axenta
	type accCounts struct {
		Total    int64
		Active   int64
		Clients  int64
		Partners int64
	}
	var ac accCounts
	if err := db.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active = true) AS active,
			COUNT(*) FILTER (WHERE account_type = 'client') AS clients,
			COUNT(*) FILTER (WHERE account_type = 'partner') AS partners
		FROM axenta_account_snapshots
		WHERE deleted_at IS NULL
	`).Scan(&ac).Error; err != nil {
		log.Printf("⚠️ sources-stats axenta accounts: %v", err)
	}

	s.Accounts.Total = int(ac.Total)
	s.Accounts.Active = int(ac.Active)
	s.Accounts.Blocked = int(ac.Total - ac.Active)
	s.Accounts.Clients = int(ac.Clients)
	s.Accounts.Partners = int(ac.Partners)

	return s
}

// buildWialonSourceStats — собирает агрегаты по WH и WL.
// Объекты: SUM из wialon_object_stats JOIN wialon_connections (мгновенный SELECT из БД).
// Аккаунты: парсим Redis-кэш /wialon/all-accounts. При cache miss — нули.
func buildWialonSourceStats(companyID uint) (SourceStats, SourceStats) {
	wh := SourceStats{Key: "wh", Label: "Wialon Hosting"}
	wl := SourceStats{Key: "wl", Label: "Wialon Local"}

	if companyID == 0 {
		return wh, wl
	}

	// Объекты по типу подключения. Точный COUNT(DISTINCT unit_id) из wialon_units.
	// Активность per unit не отслеживаем (Wialon API не отдаёт активацию пер-unit), поэтому
	// applying пропорцию active/total из агрегатов wialon_object_stats к distinct_total.
	type objRow struct {
		ConnType      string
		DistinctTotal int64
		AggTotal      int64
		AggActive     int64
		AggInactive   int64
	}
	var rows []objRow
	if err := database.DB.Raw(`
		WITH distinct_units AS (
			SELECT wc.connection_type AS conn_type, COUNT(*) AS distinct_total
			FROM wialon_units wu
			JOIN wialon_connections wc ON wc.id = wu.connection_id
			WHERE wc.company_id = ? AND wc.deleted_at IS NULL AND wc.is_active = true
			GROUP BY wc.connection_type
		),
		agg AS (
			SELECT wc.connection_type AS conn_type,
			       COALESCE(SUM(wos.objects_total), 0)        AS agg_total,
			       COALESCE(SUM(wos.objects_active), 0)       AS agg_active,
			       COALESCE(SUM(wos.objects_deactivated), 0)  AS agg_inactive
			FROM wialon_object_stats wos
			JOIN wialon_connections wc ON wc.id = wos.connection_id
			WHERE wc.company_id = ? AND wc.deleted_at IS NULL AND wc.is_active = true
			GROUP BY wc.connection_type
		)
		SELECT
			COALESCE(d.conn_type, a.conn_type) AS conn_type,
			COALESCE(d.distinct_total, 0)      AS distinct_total,
			COALESCE(a.agg_total, 0)           AS agg_total,
			COALESCE(a.agg_active, 0)          AS agg_active,
			COALESCE(a.agg_inactive, 0)        AS agg_inactive
		FROM distinct_units d
		FULL OUTER JOIN agg a ON a.conn_type = d.conn_type
	`, companyID, companyID).Scan(&rows).Error; err != nil {
		log.Printf("⚠️ sources-stats wialon objects: %v", err)
	}

	// Применяем proportion активности из агрегатов к distinct-total.
	scaleByProp := func(distinctTotal, aggTotal, aggActive, aggInactive int64) (int, int, int) {
		if distinctTotal == 0 {
			return 0, 0, 0
		}
		if aggTotal == 0 {
			// нет агрегатов — считаем всё активным
			return int(distinctTotal), int(distinctTotal), 0
		}
		active := int(distinctTotal * aggActive / aggTotal)
		inactive := int(distinctTotal * aggInactive / aggTotal)
		// Округление: добавляем разницу к активным
		if rem := int(distinctTotal) - active - inactive; rem > 0 {
			active += rem
		}
		return int(distinctTotal), active, inactive
	}

	for _, r := range rows {
		total, active, inactive := scaleByProp(r.DistinctTotal, r.AggTotal, r.AggActive, r.AggInactive)
		switch r.ConnType {
		case "hosting":
			wh.Objects.Total = total
			wh.Objects.Active = active
			wh.Objects.Inactive = inactive
		case "local":
			wl.Objects.Total = total
			wl.Objects.Active = active
			wl.Objects.Inactive = inactive
		}
	}

	// Аккаунты — из Redis-кэша
	if database.RedisClient == nil {
		return wh, wl
	}
	cached, err := database.RedisClient.Get(context.Background(), allAccountsCacheKey(companyID)).Bytes()
	if err != nil || len(cached) == 0 {
		return wh, wl
	}

	var resp struct {
		Data struct {
			Items []WialonAccountInfo `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(cached, &resp); err != nil {
		log.Printf("⚠️ sources-stats wialon accounts cache parse: %v", err)
		return wh, wl
	}

	for _, a := range resp.Data.Items {
		var dst *SourceStats
		switch {
		case strings.HasPrefix(a.SourceLabel, "WH("):
			dst = &wh
		case strings.HasPrefix(a.SourceLabel, "WL("):
			dst = &wl
		default:
			continue
		}
		dst.Accounts.Total++
		if a.IsActive {
			dst.Accounts.Active++
		} else {
			dst.Accounts.Blocked++
		}
		if a.Type == "partner" {
			dst.Accounts.Partners++
		} else {
			dst.Accounts.Clients++
		}
	}

	return wh, wl
}
