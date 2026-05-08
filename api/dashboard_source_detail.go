package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/middleware"
)

// ConnectionHistoryPoint — точка истории по одному подключению на бакет.
type ConnectionHistoryPoint struct {
	Date    string `json:"date"`            // "2026-04-13"
	Label   string `json:"label,omitempty"` // подпись бакета (для оси)
	Total   int64  `json:"total"`           // общее число объектов на конец бакета
	Created int64  `json:"created"`         // создано в окне бакета
	Deleted int64  `json:"deleted"`         // удалено в окне бакета
}

// ConnectionDetail — агрегат по одному подключению за period.
type ConnectionDetail struct {
	ID      uint                     `json:"id"`
	Name    string                   `json:"name"`
	Type    string                   `json:"type"` // "hosting" | "local" | "skif" | "axenta"
	Total   int64                    `json:"total"`
	Created int64                    `json:"created"`
	Deleted int64                    `json:"deleted"`
	History []ConnectionHistoryPoint `json:"history"`
}

// SourceDetailResponse — ответ /dashboard/source-detail.
type SourceDetailResponse struct {
	Key         string             `json:"key"`
	Period      string             `json:"period"`
	Connections []ConnectionDetail `json:"connections"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// GetDashboardSourceDetail возвращает breakdown по подключениям выбранного источника.
//
// GET /api/auth/dashboard/source-detail?key=wh|wl|skif|axenta&period=7d|1m|3m|6m|1y
func GetDashboardSourceDetail(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка подключения к БД"})
		return
	}

	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "key required"})
		return
	}

	period := c.Query("period")
	switch period {
	case "7d", "1m", "3m", "6m", "1y":
	default:
		period = "7d"
	}

	companyID := middleware.GetCompanyID(c)
	publicDB := publicDBOrTenant(tenantDB)

	now := time.Now()
	buckets := buildBuckets(now, period)

	resp := SourceDetailResponse{
		Key:         key,
		Period:      period,
		Connections: []ConnectionDetail{},
		GeneratedAt: now,
	}

	switch key {
	case "wh", "wl":
		connType := "hosting"
		if key == "wl" {
			connType = "local"
		}
		resp.Connections = buildWialonDetails(publicDB, companyID, connType, buckets)
	case "skif":
		resp.Connections = buildSkifDetails(publicDB, companyID, buckets)
	case "axenta":
		resp.Connections = buildAxentaDetails(tenantDB, buckets)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "unknown key"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": resp})
}

// buildWialonDetails — per-connection из wialon_connections.
// Total = live из wialon_connection_summary.objects_total (то же число что показывает
// карточка дашборда и Wialon CMS Manager — top-level user.bact).
// Created/Deleted за period = SUM из wialon_daily_snapshots в окне.
// History.Total per bucket = SUM(total_units) WHERE snapshot_date = bucket_eom_date —
// это число get_statistics(recursive=1), оно может отличаться от live total.
func buildWialonDetails(publicDB *gorm.DB, companyID uint, connType string, buckets []chartBucket) []ConnectionDetail {
	if companyID == 0 {
		return []ConnectionDetail{}
	}

	type connRow struct {
		ID         uint
		Name       string
		LiveTotal  int64
	}
	var conns []connRow
	publicDB.Raw(`
		SELECT wc.id, wc.name, COALESCE(wcs.objects_total, 0) AS live_total
		FROM `+publicTable(publicDB, "wialon_connections")+` wc
		LEFT JOIN `+publicTable(publicDB, "wialon_connection_summary")+` wcs ON wcs.connection_id = wc.id
		WHERE wc.company_id = ? AND wc.connection_type = ? AND wc.is_active = true
		ORDER BY wc.name
	`, companyID, connType).Scan(&conns)

	out := make([]ConnectionDetail, 0, len(conns))
	for _, c := range conns {
		detail := ConnectionDetail{
			ID:      c.ID,
			Name:    c.Name,
			Type:    connType,
			Total:   c.LiveTotal,
			History: make([]ConnectionHistoryPoint, 0, len(buckets)),
		}

		for _, b := range buckets {
			startStr := b.start.Format("2006-01-02")
			endStr := b.end.Format("2006-01-02")
			eomDate := b.end.AddDate(0, 0, -1).Format("2006-01-02")

			var total struct{ Cnt int64 }
			publicDB.Raw(`
				SELECT COALESCE(SUM(total_units), 0) AS cnt
				FROM `+publicTable(publicDB, "wialon_daily_snapshots")+`
				WHERE connection_id = ? AND snapshot_date = ?
			`, c.ID, eomDate).Scan(&total)

			var ev struct {
				Created int64
				Deleted int64
			}
			publicDB.Raw(`
				SELECT COALESCE(SUM(units_created), 0) AS created,
				       COALESCE(SUM(units_deleted), 0) AS deleted
				FROM `+publicTable(publicDB, "wialon_daily_snapshots")+`
				WHERE connection_id = ? AND snapshot_date >= ? AND snapshot_date < ?
			`, c.ID, startStr, endStr).Scan(&ev)

			detail.History = append(detail.History, ConnectionHistoryPoint{
				Date:    b.key,
				Label:   b.label,
				Total:   total.Cnt,
				Created: ev.Created,
				Deleted: ev.Deleted,
			})
			detail.Created += ev.Created
			detail.Deleted += ev.Deleted
		}

		out = append(out, detail)
	}
	return out
}

// buildSkifDetails — per skif_connection.
// Total на конец бакета = COUNT(skif_units) на eom (skif_created_at <= eom AND (skif_deleted_at IS NULL OR > eom)).
// Created/Deleted = SUM из skif_daily_snapshots в окне [start, end).
func buildSkifDetails(publicDB *gorm.DB, companyID uint, buckets []chartBucket) []ConnectionDetail {
	if companyID == 0 {
		return []ConnectionDetail{}
	}

	type connRow struct {
		ID   uint
		Name string
	}
	var conns []connRow
	publicDB.Raw(`
		SELECT id, name FROM `+publicTable(publicDB, "skif_connections")+`
		WHERE company_id = ?
		ORDER BY name
	`, companyID).Scan(&conns)

	out := make([]ConnectionDetail, 0, len(conns))
	for _, c := range conns {
		detail := ConnectionDetail{
			ID:      c.ID,
			Name:    c.Name,
			Type:    "skif",
			History: make([]ConnectionHistoryPoint, 0, len(buckets)),
		}

		for _, b := range buckets {
			startStr := b.start.Format("2006-01-02")
			endStr := b.end.Format("2006-01-02")
			eom := b.end.Add(-time.Nanosecond)

			var total int64
			publicDB.Raw(`
				SELECT COUNT(*) FROM `+publicTable(publicDB, "skif_units")+`
				WHERE connection_id = ?
				  AND COALESCE(skif_created_at, created_at) <= ?
				  AND (skif_deleted_at IS NULL OR skif_deleted_at > ?)
			`, c.ID, eom, eom).Scan(&total)

			var ev struct {
				Created int64
				Deleted int64
			}
			publicDB.Raw(`
				SELECT COALESCE(SUM(units_created), 0) AS created,
				       COALESCE(SUM(units_deleted), 0) AS deleted
				FROM `+publicTable(publicDB, "skif_daily_snapshots")+`
				WHERE connection_id = ? AND snapshot_date >= ? AND snapshot_date < ?
			`, c.ID, startStr, endStr).Scan(&ev)

			detail.History = append(detail.History, ConnectionHistoryPoint{
				Date:    b.key,
				Label:   b.label,
				Total:   total,
				Created: ev.Created,
				Deleted: ev.Deleted,
			})
			detail.Created += ev.Created
			detail.Deleted += ev.Deleted
		}

		if n := len(detail.History); n > 0 {
			detail.Total = detail.History[n-1].Total
		}

		out = append(out, detail)
	}
	return out
}

// buildAxentaDetails — Axenta представлена одной "виртуальной" connection.
// Total / Created / Deleted считаются из axenta_object_snapshots tenantDB.
func buildAxentaDetails(tenantDB *gorm.DB, buckets []chartBucket) []ConnectionDetail {
	detail := ConnectionDetail{
		ID:      0,
		Name:    "Axenta Cloud",
		Type:    "axenta",
		History: make([]ConnectionHistoryPoint, 0, len(buckets)),
	}

	for _, b := range buckets {
		eom := b.end.Add(-time.Nanosecond)

		var total int64
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("COALESCE(axenta_created_at, created_at) <= ?", eom).
			Where("(axenta_deleted_at IS NULL OR axenta_deleted_at > ?)", eom).
			Count(&total)

		var created int64
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("COALESCE(axenta_created_at, created_at) >= ? AND COALESCE(axenta_created_at, created_at) < ?", b.start, b.end).
			Count(&created)

		var deleted int64
		tenantDB.Unscoped().Table("axenta_object_snapshots").
			Where("axenta_deleted_at >= ? AND axenta_deleted_at < ?", b.start, b.end).
			Count(&deleted)

		detail.History = append(detail.History, ConnectionHistoryPoint{
			Date:    b.key,
			Label:   b.label,
			Total:   total,
			Created: created,
			Deleted: deleted,
		})
		detail.Created += created
		detail.Deleted += deleted
	}

	if n := len(detail.History); n > 0 {
		detail.Total = detail.History[n-1].Total
	}

	return []ConnectionDetail{detail}
}
