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
	skif := buildSkifSourceStats(companyID)
	gelios := buildGeliosSourceStats(companyID)

	total := SourceStats{Key: "all", Label: "Все"}
	for _, s := range []SourceStats{axenta, wh, wl, skif, gelios} {
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
			Sources: []SourceStats{axenta, wh, wl, skif, gelios},
			Total:   total,
		},
	})
}

// buildSkifSourceStats — агрегаты по SKIF.PRO юнитам и подключениям (current company).
// Объекты: COUNT skif_units по company_id. Аккаунты: 1 на каждый skif_connection.
func buildSkifSourceStats(companyID uint) SourceStats {
	s := SourceStats{Key: "skif", Label: "SKIF"}
	if database.DB == nil {
		return s
	}

	type objCounts struct {
		Total    int64
		Active   int64
		Inactive int64
	}
	var oc objCounts
	// Фильтр skif_deleted_at IS NULL — иначе считаем юниты которые исчезли в
	// SKIF (soft-delete метка), завышая total на 5-10 между sync-cleanup'ами.
	// admin.skif.pro показывает только живые.
	if err := database.DB.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active = true) AS active,
			COUNT(*) FILTER (WHERE is_active = false) AS inactive
		FROM skif_units
		WHERE company_id = ? AND skif_deleted_at IS NULL
	`, companyID).Scan(&oc).Error; err != nil {
		log.Printf("⚠️ sources-stats skif objects: %v", err)
	}
	s.Objects.Total = int(oc.Total)
	s.Objects.Active = int(oc.Active)
	s.Objects.Inactive = int(oc.Inactive)

	type connCounts struct {
		Total   int64
		Active  int64
		Blocked int64
	}
	var cc connCounts
	if err := database.DB.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active = true) AS active,
			COUNT(*) FILTER (WHERE is_active = false) AS blocked
		FROM skif_connections
		WHERE company_id = ? AND deleted_at IS NULL
	`, companyID).Scan(&cc).Error; err != nil {
		log.Printf("⚠️ sources-stats skif connections: %v", err)
	}
	s.Accounts.Total = int(cc.Total)
	s.Accounts.Active = int(cc.Active)
	s.Accounts.Blocked = int(cc.Blocked)
	s.Accounts.Clients = int(cc.Total) // SKIF подключение = клиентский аккаунт
	return s
}

// buildGeliosSourceStats — агрегаты по GELIOS юнитам и подключениям.
// Объекты: COUNT gelios_units по company_id (gelios_deleted_at учитывается
// отдельно — soft-delete). Аккаунты: 1 на каждый gelios_connection.
func buildGeliosSourceStats(companyID uint) SourceStats {
	s := SourceStats{Key: "gelios", Label: "GELIOS"}
	if database.DB == nil {
		return s
	}

	type objCounts struct {
		Total    int64
		Active   int64
		Inactive int64
		Deleted  int64
	}
	var oc objCounts
	if err := database.DB.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE gelios_deleted_at IS NULL) AS total,
			COUNT(*) FILTER (WHERE gelios_deleted_at IS NULL AND is_active = true) AS active,
			COUNT(*) FILTER (WHERE gelios_deleted_at IS NULL AND is_active = false) AS inactive,
			COUNT(*) FILTER (WHERE gelios_deleted_at IS NOT NULL) AS deleted
		FROM gelios_units
		WHERE company_id = ?
	`, companyID).Scan(&oc).Error; err != nil {
		log.Printf("⚠️ sources-stats gelios objects: %v", err)
	}
	s.Objects.Total = int(oc.Total)
	s.Objects.Active = int(oc.Active)
	s.Objects.Inactive = int(oc.Inactive)
	s.Objects.Deleted = int(oc.Deleted)

	type connCounts struct {
		Total   int64
		Active  int64
		Blocked int64
	}
	var cc connCounts
	if err := database.DB.Raw(`
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active = true) AS active,
			COUNT(*) FILTER (WHERE is_active = false) AS blocked
		FROM gelios_connections
		WHERE company_id = ? AND deleted_at IS NULL
	`, companyID).Scan(&cc).Error; err != nil {
		log.Printf("⚠️ sources-stats gelios connections: %v", err)
	}
	s.Accounts.Total = int(cc.Total)
	s.Accounts.Active = int(cc.Active)
	s.Accounts.Blocked = int(cc.Blocked)
	s.Accounts.Clients = int(cc.Total) // GELIOS подключение = клиентский аккаунт
	return s
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

	// Объекты по типу подключения — точные числа из wialon_connection_summary
	// (заполняется WialonStatsScheduler из top-level resource user.bact). Это те же
	// числа что Wialon CMS Manager показывает на корневом аккаунте.
	type objRow struct {
		ConnType string
		Total    int64
		Active   int64
		Inactive int64
	}
	var rows []objRow
	if err := database.DB.Raw(`
		SELECT
			wc.connection_type                       AS conn_type,
			COALESCE(SUM(wcs.objects_total), 0)      AS total,
			COALESCE(SUM(wcs.objects_active), 0)     AS active,
			COALESCE(SUM(wcs.objects_inactive), 0)   AS inactive
		FROM wialon_connections wc
		LEFT JOIN wialon_connection_summary wcs ON wcs.connection_id = wc.id
		WHERE wc.company_id = ? AND wc.deleted_at IS NULL AND wc.is_active = true
		GROUP BY wc.connection_type
	`, companyID).Scan(&rows).Error; err != nil {
		log.Printf("⚠️ sources-stats wialon objects: %v", err)
	}

	for _, r := range rows {
		switch r.ConnType {
		case "hosting":
			wh.Objects.Total = int(r.Total)
			wh.Objects.Active = int(r.Active)
			wh.Objects.Inactive = int(r.Inactive)
		case "local":
			wl.Objects.Total = int(r.Total)
			wl.Objects.Active = int(r.Active)
			wl.Objects.Inactive = int(r.Inactive)
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
