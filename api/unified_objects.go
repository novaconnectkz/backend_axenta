package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UnifiedObject — единая структура объекта мониторинга из любого источника.
type UnifiedObject struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	UniqueID            string   `json:"unique_id"`
	IMEI                string   `json:"imei"`
	IsActive            bool     `json:"is_active"`
	AccountName         string   `json:"account_name,omitempty"`
	AccountID           int64    `json:"account_id,omitempty"`
	CreatorName         string   `json:"creator_name,omitempty"`
	DeviceTypeName      string   `json:"device_type_name,omitempty"`
	PhoneNumbers        []string `json:"phone_numbers,omitempty"`
	CreatedAt           string   `json:"created_at,omitempty"`
	LastMessageDatetime string   `json:"last_message_datetime,omitempty"`
	Source              string   `json:"source"`                  // "axenta" | "wh" | "wl"
	SourceLabel         string   `json:"source_label,omitempty"`  // "Axenta Cloud" | "WH(...)" | "WL(...)"
	ConnectionID        *uint    `json:"connection_id,omitempty"` // только для wh/wl
	ScheduledDelete     bool     `json:"scheduled_delete,omitempty"`
}

// UnifiedObjectsStats — KPI-разбивка по источникам.
type UnifiedObjectsStats struct {
	AxentaTotal        int `json:"axenta_total"`
	AxentaActive       int `json:"axenta_active"`
	AxentaInactive     int `json:"axenta_inactive"`
	AxentaDeleted      int `json:"axenta_deleted"`
	AxentaScheduledDel int `json:"axenta_scheduled_delete"`
	WialonTotal        int `json:"wialon_total"`
	WialonActive       int `json:"wialon_active"`
	WialonWHTotal      int `json:"wialon_wh_total"`
	WialonWHActive     int `json:"wialon_wh_active"`
	WialonWLTotal      int `json:"wialon_wl_total"`
	WialonWLActive     int `json:"wialon_wl_active"`
}

// UnifiedObjectsResponse — формат ответа.
type UnifiedObjectsResponse struct {
	Items      []UnifiedObject     `json:"items"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PerPage    int                 `json:"per_page"`
	TotalPages int                 `json:"total_pages"`
	Stats      UnifiedObjectsStats `json:"stats"`
}

// GetUnifiedObjects — единая выдача объектов из Axenta + Wialon (WH+WL).
// Read-path: snapshot для Axenta + Redis cache для Wialon.
//
// @Summary Унифицированный список объектов
// @Tags Unified Objects
// @Produce json
// @Param page query int false "Страница" default(1)
// @Param per_page query int false "На странице" default(50)
// @Param search query string false "Поиск по имени/IMEI/телефону"
// @Param source query string false "axenta|wialon|wh|wl|all" default(all)
// @Param active query bool false "Фильтр активности"
// @Success 200 {object} UnifiedObjectsResponse
// @Router /api/unified/objects [get]
func GetUnifiedObjects(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	search := strings.TrimSpace(c.Query("search"))
	source := strings.ToLower(c.DefaultQuery("source", "all"))
	activeStr := c.Query("active")
	ordering := c.Query("ordering")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 1000 {
		perPage = 50
	}

	companyIDRaw, _ := c.Get("company_id")

	all := make([]UnifiedObject, 0, 1024)
	var stats UnifiedObjectsStats
	var wg sync.WaitGroup
	var mu sync.Mutex

	loadAxenta := source == "all" || source == "axenta"
	loadWialon := source == "all" || source == "wialon" || source == "wh" || source == "wl"

	if loadAxenta {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tenantDB := middleware.GetTenantDB(c)
			t0 := time.Now()
			axentaObjects, st, ok := fetchAxentaObjectsFast(tenantDB, search, activeStr)
			log.Printf("🔍 unified/objects axenta: %d (snapshot=%v) за %s",
				len(axentaObjects), ok, time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			all = append(all, axentaObjects...)
			stats.AxentaTotal = st.Total
			stats.AxentaActive = st.Active
			stats.AxentaInactive = st.Inactive
			stats.AxentaDeleted = st.Deleted
			stats.AxentaScheduledDel = st.ScheduledDel
			mu.Unlock()
		}()
	}

	if loadWialon && companyIDRaw != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t0 := time.Now()
			wlObjects, whTotal, whActive, wlTotal, wlActive, fromCache := fetchWialonObjectsFast(companyIDRaw.(uint), search, activeStr, source)
			log.Printf("🔍 unified/objects wialon: %d (cache=%v) за %s",
				len(wlObjects), fromCache, time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			all = append(all, wlObjects...)
			stats.WialonWHTotal = whTotal
			stats.WialonWHActive = whActive
			stats.WialonWLTotal = wlTotal
			stats.WialonWLActive = wlActive
			stats.WialonTotal = whTotal + wlTotal
			stats.WialonActive = whActive + wlActive
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Сортировка по name (default)
	sortUnifiedObjects(all, ordering)

	total := len(all)
	startIdx := (page - 1) * perPage
	endIdx := startIdx + perPage
	if startIdx > total {
		startIdx = total
	}
	if endIdx > total {
		endIdx = total
	}
	totalPages := (total + perPage - 1) / perPage

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": UnifiedObjectsResponse{
			Items:      all[startIdx:endIdx],
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
			Stats:      stats,
		},
	})
}

// axentaObjectsAggregate — агрегаты по snapshot для KPI.
type axentaObjectsAggregate struct {
	Total        int
	Active       int
	Inactive     int
	Deleted      int
	ScheduledDel int
}

// fetchAxentaObjectsFast читает объекты Axenta из snapshot (с фильтрами).
func fetchAxentaObjectsFast(db *gorm.DB, search, active string) ([]UnifiedObject, axentaObjectsAggregate, bool) {
	var agg axentaObjectsAggregate
	if db == nil {
		return nil, agg, false
	}

	var lastSync time.Time
	if err := db.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, agg, false
	}
	if time.Since(lastSync) > objectsSnapshotTTL {
		return nil, agg, false
	}

	// Агрегаты (один запрос на 5 счётчиков)
	type aggRow struct {
		Total        int64
		Active       int64
		Inactive     int64
		Deleted      int64
		ScheduledDel int64
	}
	var ar aggRow
	if err := db.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL) AS total,
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL AND is_active = true) AS active,
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND scheduled_delete_at IS NULL AND axenta_deleted_at IS NULL AND is_active = false) AS inactive,
			COUNT(*) FILTER (WHERE axenta_deleted_at IS NOT NULL) AS deleted,
			COUNT(*) FILTER (WHERE scheduled_delete_at IS NOT NULL) AS scheduled_del
		FROM axenta_object_snapshots
	`).Scan(&ar).Error; err != nil {
		log.Printf("⚠️ unified/objects axenta agg: %v", err)
	}
	agg.Total = int(ar.Total)
	agg.Active = int(ar.Active)
	agg.Inactive = int(ar.Inactive)
	agg.Deleted = int(ar.Deleted)
	agg.ScheduledDel = int(ar.ScheduledDel)

	q := db.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_deleted_at IS NULL")

	if search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		q = q.Where(
			"LOWER(object_name) LIKE ? OR LOWER(unique_id) LIKE ? OR LOWER(account_name) LIKE ? OR phone_numbers ILIKE ?",
			pattern, pattern, pattern, pattern,
		)
	}
	switch active {
	case "true", "1":
		q = q.Where("is_active = ?", true)
	case "false", "0":
		q = q.Where("is_active = ?", false)
	}

	var rows []models.AxentaObjectSnapshot
	if err := q.Order("object_name ASC").Limit(5000).Find(&rows).Error; err != nil {
		log.Printf("⚠️ unified/objects axenta find: %v", err)
		return nil, agg, false
	}

	out := make([]UnifiedObject, 0, len(rows))
	for _, r := range rows {
		var phones []string
		if r.PhoneNumbers != nil && *r.PhoneNumbers != "" {
			_ = json.Unmarshal([]byte(*r.PhoneNumbers), &phones)
		}
		creator := ""
		if r.CreatorName != nil {
			creator = *r.CreatorName
		}
		var createdAt, lastMsg string
		if r.AxentaCreatedAt != nil {
			createdAt = r.AxentaCreatedAt.Format(time.RFC3339)
		}
		if r.LastCommunicationAt != nil {
			lastMsg = r.LastCommunicationAt.Format(time.RFC3339)
		}
		out = append(out, UnifiedObject{
			ID:                  r.ExternalObjectID,
			Name:                r.ObjectName,
			UniqueID:            r.UniqueID,
			IMEI:                r.UniqueID,
			IsActive:            r.IsActive,
			AccountName:         r.AccountName,
			AccountID:           r.AccountExternalID,
			CreatorName:         creator,
			DeviceTypeName:      r.DeviceTypeName,
			PhoneNumbers:        phones,
			CreatedAt:           createdAt,
			LastMessageDatetime: lastMsg,
			Source:              "axenta",
			SourceLabel:         "Axenta Cloud",
			ScheduledDelete:     r.ScheduledDeleteAt != nil,
		})
	}
	return out, agg, true
}

// fetchWialonObjectsFast — read-path Wialon: Redis cache + GetAllUnitsFromActiveConnections fallback.
// Возвращает (objects, whTotal, whActive, wlTotal, wlActive, fromCache).
func fetchWialonObjectsFast(companyID uint, search, active, source string) ([]UnifiedObject, int, int, int, int, bool) {
	connService := services.NewWialonConnectionService(database.DB)

	// Получаем активные подключения для разделения WH/WL
	connections, _ := connService.GetActiveByCompany(companyID)
	connType := make(map[uint]string, len(connections))
	connLabel := make(map[uint]string, len(connections))
	for _, conn := range connections {
		if conn.ConnectionType == models.WialonConnectionTypeHosting {
			connType[conn.ID] = "wh"
			connLabel[conn.ID] = "WH(" + conn.UserName + ")"
		} else {
			connType[conn.ID] = "wl"
			connLabel[conn.ID] = "WL(" + conn.UserName + ")"
		}
	}

	// Собираем units из cache или live (через тот же путь, что /wialon/all-units)
	type cachedPayload struct {
		Items []services.WialonUnitWithConnection `json:"items"`
	}
	var units []services.WialonUnitWithConnection
	fromCache := false

	if database.RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if cached, err := database.RedisClient.Get(ctx, allUnitsCacheKey(companyID)).Bytes(); err == nil && len(cached) > 0 {
			var payload cachedPayload
			if err := json.Unmarshal(cached, &payload); err == nil {
				units = payload.Items
				fromCache = true
			}
		}
	}
	if !fromCache {
		liveUnits, err := connService.GetAllUnitsFromActiveConnections(companyID)
		if err != nil {
			log.Printf("⚠️ unified/objects wialon: %v", err)
			return nil, 0, 0, 0, 0, false
		}
		units = liveUnits

		// Сохраняем в кэш по тому же ключу что использует /wialon/all-units
		if database.RedisClient != nil {
			payload := gin.H{
				"items":             liveUnits,
				"total":             len(liveUnits),
				"connections_count": len(connections),
			}
			if encoded, err := json.Marshal(payload); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				_ = database.RedisClient.Set(ctx, allUnitsCacheKey(companyID), encoded, allUnitsCacheTTL).Err()
			}
		}
	}

	// Фильтруем + считаем breakdown WH/WL
	pattern := strings.ToLower(search)
	out := make([]UnifiedObject, 0, len(units))
	whTotal, whActive, wlTotal, wlActive := 0, 0, 0, 0

	for _, u := range units {
		t := connType[u.ConnectionID]
		if source == "wh" && t != "wh" {
			continue
		}
		if source == "wl" && t != "wl" {
			continue
		}

		// services.WialonUnit не имеет IsActive — все объекты Wialon считаем активными
		// (статус определяется на уровне billing-resource, не объекта)
		switch t {
		case "wh":
			whTotal++
			whActive++
		case "wl":
			wlTotal++
			wlActive++
		}

		if pattern != "" {
			if !strings.Contains(strings.ToLower(u.Name), pattern) &&
				!strings.Contains(strings.ToLower(u.UniqueID), pattern) &&
				!strings.Contains(u.PhoneNumber, pattern) &&
				!strings.Contains(u.PhoneNumber2, pattern) {
				continue
			}
		}
		// active=false → пропустить все (Wialon юниты всегда активны)
		if active == "false" || active == "0" {
			continue
		}

		phones := make([]string, 0, 2)
		if u.PhoneNumber != "" {
			phones = append(phones, u.PhoneNumber)
		}
		if u.PhoneNumber2 != "" {
			phones = append(phones, u.PhoneNumber2)
		}
		var createdAt, lastMsg string
		if u.CreatedAt != 0 {
			createdAt = time.Unix(u.CreatedAt, 0).UTC().Format(time.RFC3339)
		}
		if u.LastMessage != 0 {
			lastMsg = time.Unix(u.LastMessage, 0).UTC().Format(time.RFC3339)
		}

		connID := u.ConnectionID
		out = append(out, UnifiedObject{
			ID:                  u.ID,
			Name:                u.Name,
			UniqueID:            u.UniqueID,
			IMEI:                u.UniqueID,
			IsActive:            true,
			AccountName:         u.ConnectionName,
			DeviceTypeName:      u.HardwareTypeName,
			PhoneNumbers:        phones,
			CreatedAt:           createdAt,
			LastMessageDatetime: lastMsg,
			Source:              t,
			SourceLabel:         connLabel[u.ConnectionID],
			ConnectionID:        &connID,
		})
	}

	return out, whTotal, whActive, wlTotal, wlActive, fromCache
}

// sortUnifiedObjects сортирует in-place по ordering ("name", "-name", "created_at", "-created_at").
func sortUnifiedObjects(items []UnifiedObject, ordering string) {
	desc := strings.HasPrefix(ordering, "-")
	field := strings.TrimPrefix(ordering, "-")
	if field == "" {
		field = "name"
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch field {
		case "created_at", "createdAt":
			less = items[i].CreatedAt < items[j].CreatedAt
		case "last_message_datetime", "lastMessageDatetime":
			less = items[i].LastMessageDatetime < items[j].LastMessageDatetime
		case "unique_id", "uniqueId", "imei":
			less = items[i].UniqueID < items[j].UniqueID
		default:
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		if desc {
			return !less
		}
		return less
	})
}

// RegisterUnifiedObjectsRoutes регистрирует /api/unified/objects.
func RegisterUnifiedObjectsRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/unified/objects", GetUnifiedObjects)
	apiGroup.GET("/unified/objects/", GetUnifiedObjects)
	log.Println("✅ Unified Objects API routes registered")
}
