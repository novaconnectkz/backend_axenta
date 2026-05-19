package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// objectsStatsCacheTTL — TTL Redis-кэша для /objects/stats. 60s — точность KPI vs нагрузка на БД.
const objectsStatsCacheTTL = 60 * time.Second

// objectsStatsCacheKey — ключ Redis-кэша stats. Глобальный snapshot, общий на всех партнёров.
const objectsStatsCacheKey = "objects:stats:axenta:v1"

// objectsSnapshotTTL — окно свежести AxentaObjectSnapshot. Сверх него — fallback на live Axenta Cloud.
// Использует общий SnapshotTTL (60 мин), см. snapshot_ttl.go.
const objectsSnapshotTTL = SnapshotTTL

// applyObjectSnapshotFilters строит отфильтрованный (без сортировки/пагинации)
// запрос к axenta_object_snapshots из query-параметров. Единый источник
// фильтрации для списка (/cms/objects, /unified/objects) и XLSX-экспорта —
// чтобы набор строк экспорта был 1:1 с тем, что видит пользователь в списке.
func applyObjectSnapshotFilters(c *gin.Context, db *gorm.DB) *gorm.DB {
	q := db.Model(&models.AxentaObjectSnapshot{}).Where("axenta_deleted_at IS NULL")

	search := c.Query("search")
	status := c.Query("status")
	isActive := c.Query("is_active")
	accountID := c.Query("accountId")
	accountName := c.Query("accountName")
	creatorName := c.Query("creatorName")
	deviceTypeName := c.Query("deviceTypeName")
	uniqueID := c.Query("uniqueId")
	contractID := c.Query("contract_id")

	if search != "" {
		// prod PG имеет lc_ctype=C → LOWER() не опускает кириллицу, а Go strings.ToLower
		// опускает: LOWER(account_name) LIKE LOWER(go) даёт 0 совпадений на кириллице.
		// Проектный паттерн — ILIKE ? COLLATE "und-x-icu" без strings.ToLower.
		pattern := "%" + search + "%"
		q = q.Where(
			`object_name ILIKE ? COLLATE "und-x-icu" OR unique_id ILIKE ? COLLATE "und-x-icu" OR account_name ILIKE ? COLLATE "und-x-icu" OR phone_numbers::text ILIKE ?`,
			pattern, pattern, pattern, pattern,
		)
	}
	switch isActive {
	case "true", "1":
		q = q.Where("is_active = ?", true)
	case "false", "0":
		q = q.Where("is_active = ?", false)
	}
	switch status {
	case "active":
		q = q.Where("is_active = ?", true)
	case "inactive":
		q = q.Where("is_active = ?", false)
	case "scheduled_delete":
		q = q.Where("scheduled_delete_at IS NOT NULL")
	}
	if accountID != "" {
		if v, err := strconv.ParseInt(accountID, 10, 64); err == nil {
			q = q.Where("account_external_id = ?", v)
		}
	}
	if accountName != "" {
		q = q.Where(`account_name ILIKE ? COLLATE "und-x-icu"`, "%"+accountName+"%")
	}
	if creatorName != "" {
		q = q.Where(`creator_name ILIKE ? COLLATE "und-x-icu"`, "%"+creatorName+"%")
	}
	if deviceTypeName != "" {
		q = q.Where(`device_type_name ILIKE ? COLLATE "und-x-icu"`, "%"+deviceTypeName+"%")
	}
	if uniqueID != "" {
		q = q.Where(`unique_id ILIKE ? COLLATE "und-x-icu"`, "%"+uniqueID+"%")
	}
	if contractID != "" {
		// contract_id в текущей модели не отделён от account_external_id
		if v, err := strconv.ParseInt(contractID, 10, 64); err == nil {
			q = q.Where("account_external_id = ?", v)
		}
	}
	return q
}

// serveObjectsFromSnapshot отдаёт страницу объектов из axenta_object_snapshots.
// Всегда 200 (snapshot-only, Ф3-B): свежий → from_snapshot:true; устаревший →
// данные + degraded:true; нет БД/snapshot/SQL-ошибка → пустой список + degraded:true.
// Без live-proxy в axenta.cloud по request-токену — после Ф1 логин = локальный
// JWT, невалиден для Axenta (см. concepts/local-auth-ph1.md грабля #2).
func serveObjectsFromSnapshot(c *gin.Context, page, perPage int) {
	// Хардening: функция всегда безопасна даже при прямом вызове с мусором
	// (защита от паники total_pages = ... / perPage при perPage<=0).
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 1000 {
		perPage = 50
	}

	emptyDegraded := func() {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items":       []gin.H{},
				"total":       0,
				"page":        page,
				"per_page":    perPage,
				"total_pages": 0,
				"degraded":    true,
			},
		})
	}

	db := middleware.GetTenantDB(c)
	if db == nil {
		db = database.DB
	}
	if db == nil {
		emptyDegraded()
		return
	}

	var lastSync time.Time
	if err := db.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		emptyDegraded()
		return
	}
	stale := time.Since(lastSync) > objectsSnapshotTTL
	if stale {
		log.Printf("⏰ AxentaObjectSnapshot устарел (last=%v) — degraded", lastSync)
	}

	// Фильтры — единый источник applyObjectSnapshotFilters (общий со /objects/export).
	q := applyObjectSnapshotFilters(c, db)
	ordering := c.DefaultQuery("ordering", "name")

	var total int64
	if err := q.Count(&total).Error; err != nil {
		log.Printf("⚠️ AxentaObjectSnapshot count: %v", err)
		emptyDegraded()
		return
	}

	// Сортировка
	orderClause := "object_name ASC"
	switch strings.TrimPrefix(ordering, "-") {
	case "name", "":
		orderClause = "object_name"
	case "createdAt", "created_at", "axenta_created_at":
		orderClause = "axenta_created_at"
	case "lastMessageDatetime", "last_communication_at":
		orderClause = "last_communication_at"
	case "uniqueId", "unique_id":
		orderClause = "unique_id"
	}
	if strings.HasPrefix(ordering, "-") {
		orderClause += " DESC NULLS LAST"
	} else {
		orderClause += " ASC NULLS LAST"
	}

	offset := (page - 1) * perPage
	var rows []models.AxentaObjectSnapshot
	if err := q.Order(orderClause).Limit(perPage).Offset(offset).Find(&rows).Error; err != nil {
		log.Printf("⚠️ AxentaObjectSnapshot find: %v", err)
		emptyDegraded()
		return
	}

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var phones []string
		if r.PhoneNumbers != nil && *r.PhoneNumbers != "" {
			_ = json.Unmarshal([]byte(*r.PhoneNumbers), &phones)
		}
		creator := ""
		if r.CreatorName != nil {
			creator = *r.CreatorName
		}
		creatorActive := false
		if r.CreatorIsActive != nil {
			creatorActive = *r.CreatorIsActive
		}
		accountActive := false
		if r.AccountIsActive != nil {
			accountActive = *r.AccountIsActive
		}
		var createdAt string
		if r.AxentaCreatedAt != nil {
			createdAt = r.AxentaCreatedAt.UTC().Format(time.RFC3339)
		}
		var lastMsg string
		if r.LastCommunicationAt != nil {
			lastMsg = r.LastCommunicationAt.UTC().Format(time.RFC3339)
		}
		statusStr := "inactive"
		if r.IsActive {
			statusStr = "active"
		}
		phoneNumber := ""
		if len(phones) > 0 {
			phoneNumber = phones[0]
		}

		items = append(items, gin.H{
			"id":                  r.ExternalObjectID,
			"name":                r.ObjectName,
			"type":                "vehicle",
			"uniqueId":            r.UniqueID,
			"imei":                r.UniqueID,
			"serial_number":       r.UniqueID,
			"is_active":           r.IsActive,
			"status":              statusStr,
			"accountName":         r.AccountName,
			"deviceTypeName":      r.DeviceTypeName,
			"creatorName":         creator,
			"creatorIsActive":     creatorActive,
			"accountIsActive":     accountActive,
			"phoneNumbers":        phones,
			"phone_number":        phoneNumber,
			"createdAt":           createdAt,
			"created_at":          createdAt,
			"updated_at":          createdAt,
			"lastMessageDatetime": lastMsg,
			"company_id":          r.AccountExternalID,
			"contract_id":         r.AccountExternalID,
			"location_id":         r.AccountExternalID,
			"address":             r.AccountName,
			"description":         r.DeviceTypeName + " - " + r.AccountName,
			"settings":            "{}",
			"tags":                []string{r.DeviceTypeName},
			"notes":               "Создатель: " + creator,
			"external_id":         r.UniqueID,
		})
	}

	totalPages := (int(total) + perPage - 1) / perPage
	data := gin.H{
		"items":                items,
		"total":                total,
		"page":                 page,
		"per_page":             perPage,
		"total_pages":          totalPages,
		"snapshot_age_seconds": int(time.Since(lastSync).Seconds()),
	}
	if stale {
		data["degraded"] = true
	} else {
		data["from_snapshot"] = true
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

// serveDeletedObjectsFromSnapshot — корзина объектов (/cms/trash) из
// axenta_object_snapshots WHERE axenta_deleted_at IS NOT NULL. Ф3-B7:
// snapshot-only, без live-proxy в axenta.cloud по request-токену (после Ф1
// невалиден). Всегда 200; нет БД/snapshot/SQL-ошибка → пустой список +
// degraded:true. Envelope 1:1 со старым proxy ({items,total,page,limit,total_pages}).
func serveDeletedObjectsFromSnapshot(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 1000 {
		perPage = 50
	}
	search := c.Query("search")

	emptyDegraded := func() {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"items":       []gin.H{},
				"total":       0,
				"page":        page,
				"limit":       perPage,
				"total_pages": 0,
				"degraded":    true,
			},
		})
	}

	db := middleware.GetTenantDB(c)
	if db == nil {
		db = database.DB
	}
	if db == nil {
		emptyDegraded()
		return
	}

	var lastSync time.Time
	if err := db.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		emptyDegraded()
		return
	}
	stale := time.Since(lastSync) > objectsSnapshotTTL
	if stale {
		log.Printf("⏰ AxentaObjectSnapshot устарел для trash (last=%v) — degraded", lastSync)
	}

	q := db.Model(&models.AxentaObjectSnapshot{}).Where("axenta_deleted_at IS NOT NULL")
	if search != "" {
		pattern := "%" + search + "%"
		q = q.Where(
			`object_name ILIKE ? COLLATE "und-x-icu" OR unique_id ILIKE ? COLLATE "und-x-icu" OR account_name ILIKE ? COLLATE "und-x-icu"`,
			pattern, pattern, pattern,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		log.Printf("⚠️ trash snapshot count: %v", err)
		emptyDegraded()
		return
	}

	offset := (page - 1) * perPage
	var rows []models.AxentaObjectSnapshot
	if err := q.Order("axenta_deleted_at DESC NULLS LAST").
		Limit(perPage).Offset(offset).Find(&rows).Error; err != nil {
		log.Printf("⚠️ trash snapshot find: %v", err)
		emptyDegraded()
		return
	}

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var phones []string
		if r.PhoneNumbers != nil && *r.PhoneNumbers != "" {
			_ = json.Unmarshal([]byte(*r.PhoneNumbers), &phones)
		}
		creator := ""
		if r.CreatorName != nil {
			creator = *r.CreatorName
		}
		var createdAt string
		if r.AxentaCreatedAt != nil {
			createdAt = r.AxentaCreatedAt.UTC().Format(time.RFC3339)
		}
		var deletedAt string
		if r.AxentaDeletedAt != nil {
			deletedAt = r.AxentaDeletedAt.UTC().Format(time.RFC3339)
		}
		var lastMsg string
		if r.LastCommunicationAt != nil {
			lastMsg = r.LastCommunicationAt.UTC().Format(time.RFC3339)
		}
		phoneNumber := ""
		if len(phones) > 0 {
			phoneNumber = phones[0]
		}
		items = append(items, gin.H{
			"id":                  r.ExternalObjectID,
			"name":                r.ObjectName,
			"type":                "vehicle",
			"description":         "",
			"latitude":            nil,
			"longitude":           nil,
			"address":             r.AccountName,
			"imei":                r.UniqueID,
			"phone_number":        phoneNumber,
			"serial_number":       r.UniqueID,
			"status":              "deleted",
			"is_active":           false,
			"scheduled_delete_at": nil,
			"last_activity_at":    nil,
			"company_id":          r.AccountExternalID,
			"contract_id":         r.AccountExternalID,
			"template_id":         nil,
			"location_id":         r.AccountExternalID,
			"created_at":          createdAt,
			"updated_at":          createdAt,
			"deleted_at":          deletedAt,
			"accountName":         r.AccountName,
			"creatorName":         creator,
			"deviceTypeName":      r.DeviceTypeName,
			"phoneNumbers":        phones,
			"createdAt":           createdAt,
			"lastMessageDatetime": lastMsg,
			"uniqueId":            r.UniqueID,
			"settings":            "{}",
			"tags":                []string{r.DeviceTypeName},
			"notes":               "Создатель: " + creator,
			"external_id":         r.UniqueID,
		})
	}

	totalPages := (int(total) + perPage - 1) / perPage
	data := gin.H{
		"items":       items,
		"total":       total,
		"page":        page,
		"limit":       perPage,
		"total_pages": totalPages,
	}
	if stale {
		data["degraded"] = true
	} else {
		data["from_snapshot"] = true
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

// objectsStatsResult — структура для bind COUNT FILTER в одном SQL.
type objectsStatsResult struct {
	Total              int64 `json:"total"`
	Active             int64 `json:"active"`
	Inactive           int64 `json:"inactive"`
	ScheduledForDelete int64 `json:"scheduled_for_delete"`
	Deleted            int64 `json:"deleted"` // объекты в корзине (axenta_deleted_at IS NOT NULL)
}

// serveObjectsStatsFromSnapshot отдаёт KPI-агрегаты объектов одним SQL по
// axenta_object_snapshots. Всегда отвечает 200 (snapshot-only, Ф3-B):
//   - snapshot свежий     → реальные цифры, from_snapshot:true (+ Redis cache TTL 60s)
//   - snapshot устарел    → реальные цифры + degraded:true + snapshot_age_seconds
//   - нет БД/snapshot/SQL  → нули + degraded:true
//
// Никакого live-proxy в axenta.cloud по токену запроса: после Ф1 логин = локальный
// JWT, request-токен невалиден для Axenta (см. concepts/local-auth-ph1.md грабля #2).
func serveObjectsStatsFromSnapshot(c *gin.Context) {
	emptyDegraded := func() {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"total":                0,
				"active":               0,
				"inactive":             0,
				"scheduled_for_delete": 0,
				"deleted":              0,
				"by_type":              gin.H{"vehicle": 0},
				"by_status":            gin.H{"active": 0, "inactive": 0},
				"degraded":             true,
			},
		})
	}

	db := middleware.GetTenantDB(c)
	if db == nil {
		db = database.DB
	}
	if db == nil {
		emptyDegraded()
		return
	}

	// Свежесть snapshot
	var lastSync time.Time
	if err := db.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		emptyDegraded()
		return
	}
	stale := time.Since(lastSync) > objectsSnapshotTTL

	// Redis cache hit — только для свежего snapshot
	if !stale {
		if rdb := database.GetRedis(); rdb != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
			defer cancel()
			if cached, err := rdb.Get(ctx, objectsStatsCacheKey).Bytes(); err == nil && len(cached) > 0 {
				var stats objectsStatsResult
				if err := json.Unmarshal(cached, &stats); err == nil {
					c.JSON(http.StatusOK, gin.H{
						"status": "success",
						"data": gin.H{
							"total":                stats.Total,
							"active":               stats.Active,
							"inactive":             stats.Inactive,
							"scheduled_for_delete": stats.ScheduledForDelete,
							"deleted":              stats.Deleted,
							"by_type":              gin.H{"vehicle": stats.Total},
							"by_status": gin.H{
								"active":   stats.Active,
								"inactive": stats.Inactive,
							},
							"from_cache": true,
						},
					})
					return
				}
			}
		}
	}

	// Один SQL агрегат (паттерн из dashboard_sources_stats.buildAxentaSourceStats)
	var stats objectsStatsResult
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
			) AS scheduled_for_delete,
			COUNT(*) FILTER (
				WHERE axenta_deleted_at IS NOT NULL
			) AS deleted
		FROM axenta_object_snapshots
	`).Scan(&stats).Error; err != nil {
		log.Printf("⚠️ AxentaObjectSnapshot stats SQL: %v", err)
		emptyDegraded()
		return
	}

	data := gin.H{
		"total":                stats.Total,
		"active":               stats.Active,
		"inactive":             stats.Inactive,
		"scheduled_for_delete": stats.ScheduledForDelete,
		"deleted":              stats.Deleted,
		"by_type":              gin.H{"vehicle": stats.Total},
		"by_status": gin.H{
			"active":   stats.Active,
			"inactive": stats.Inactive,
		},
		"snapshot_age_seconds": int(time.Since(lastSync).Seconds()),
	}
	if stale {
		// Устаревший snapshot: отдаём реальные цифры, помечаем degraded,
		// в Redis НЕ кладём (чтобы свежий синк не ждал TTL).
		data["degraded"] = true
		log.Printf("⏰ AxentaObjectSnapshot устарел для stats (last=%v) — degraded", lastSync)
	} else {
		data["from_snapshot"] = true
		if rdb := database.GetRedis(); rdb != nil {
			if payload, err := json.Marshal(stats); err == nil {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
				defer cancel()
				_ = rdb.Set(ctx, objectsStatsCacheKey, payload, objectsStatsCacheTTL).Err()
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}
