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
// json-теги в camelCase для совместимости с frontend-таблицей (которая исторически
// читает поля Axenta Cloud в camelCase: accountName, creatorName, deviceTypeName и т.п.).
type UnifiedObject struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	UniqueID            string   `json:"uniqueId"`
	IMEI                string   `json:"imei"`
	IsActive            bool     `json:"is_active"`
	AccountName         string   `json:"accountName,omitempty"`
	AccountID           int64    `json:"accountId,omitempty"`
	CreatorName         string   `json:"creatorName,omitempty"`
	DeviceTypeName      string   `json:"deviceTypeName,omitempty"`
	PhoneNumbers        []string `json:"phoneNumbers,omitempty"`
	CreatedAt           string   `json:"createdAt,omitempty"`
	LastMessageDatetime string   `json:"lastMessageDatetime,omitempty"`
	Source              string   `json:"source"`                 // "axenta" | "wh" | "wl"
	SourceLabel         string   `json:"sourceLabel,omitempty"`  // "Axenta Cloud" | "WH(...)" | "WL(...)"
	ConnectionID        *uint    `json:"connectionId,omitempty"` // только для wh/wl
	ScheduledDelete     bool     `json:"scheduledDelete,omitempty"`
}

// UnifiedObjectsStats — KPI-разбивка по источникам.
type UnifiedObjectsStats struct {
	AxentaTotal        int `json:"axenta_total"`
	AxentaActive       int `json:"axenta_active"`
	AxentaInactive     int `json:"axenta_inactive"`
	AxentaDeleted      int `json:"axenta_deleted"`
	AxentaScheduledDel int `json:"axenta_scheduled_delete"`
	// Ф3-B6: true если Axenta-источник деградировал (snapshot пуст/устарел;
	// live-proxy в axenta.cloud по request-токену убран — после Ф1 невалиден).
	AxentaDegraded bool `json:"axenta_degraded"`
	WialonTotal    int  `json:"wialon_total"`
	WialonActive   int  `json:"wialon_active"`
	WialonWHTotal  int  `json:"wialon_wh_total"`
	WialonWHActive int  `json:"wialon_wh_active"`
	WialonWLTotal  int  `json:"wialon_wl_total"`
	WialonWLActive int  `json:"wialon_wl_active"`
	GeliosTotal    int  `json:"gelios_total"`
	GeliosActive   int  `json:"gelios_active"`
	GeliosInactive int  `json:"gelios_inactive"`
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
	loadGelios := source == "all" || source == "gelios"

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
			if !ok {
				stats.AxentaDegraded = true
			}
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

	if gCompanyID, gOK := companyIDRaw.(uint); loadGelios && gOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t0 := time.Now()
			gObjects, gTotal, gActive, gInactive := fetchGeliosObjectsFast(gCompanyID, search, activeStr)
			log.Printf("🔍 unified/objects gelios: %d за %s",
				len(gObjects), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			all = append(all, gObjects...)
			stats.GeliosTotal = gTotal
			stats.GeliosActive = gActive
			stats.GeliosInactive = gInactive
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
		// БД создана с lc_collate=C → LOWER() и ILIKE без COLLATE не понимают кириллицу.
		// Используем ILIKE + COLLATE "und-x-icu" (тот же приём что в /unified/accounts).
		pattern := "%" + search + "%"
		q = q.Where(
			`object_name ILIKE ? COLLATE "und-x-icu" OR unique_id ILIKE ? COLLATE "und-x-icu" OR account_name ILIKE ? COLLATE "und-x-icu" OR device_type_name ILIKE ? COLLATE "und-x-icu" OR creator_name ILIKE ? COLLATE "und-x-icu" OR phone_numbers::text ILIKE ? OR CAST(external_object_id AS TEXT) LIKE ?`,
			pattern, pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}
	switch active {
	case "true", "1":
		q = q.Where("is_active = ?", true)
	case "false", "0":
		q = q.Where("is_active = ?", false)
	}

	var rows []models.AxentaObjectSnapshot
	if err := q.Order("object_name ASC").Find(&rows).Error; err != nil {
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
			createdAt = r.AxentaCreatedAt.UTC().Format(time.RFC3339)
		}
		if r.LastCommunicationAt != nil {
			lastMsg = r.LastCommunicationAt.UTC().Format(time.RFC3339)
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
	connOwner := make(map[uint]string, len(connections)) // имя пользователя подключения (fallback для creator)
	for _, conn := range connections {
		if conn.ConnectionType == models.WialonConnectionTypeHosting {
			connType[conn.ID] = "wh"
			connLabel[conn.ID] = "WH(" + conn.UserName + ")"
		} else {
			connType[conn.ID] = "wl"
			connLabel[conn.ID] = "WL(" + conn.UserName + ")"
		}
		connOwner[conn.ID] = conn.UserName
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
		liveUnits, partial, err := connService.GetAllUnitsFromActiveConnectionsDetailed(companyID)
		if err != nil {
			log.Printf("⚠️ unified/objects wialon: %v", err)
			return nil, 0, 0, 0, 0, false
		}
		units = liveUnits

		// Сохраняем в кэш только если результат полный (нет упавших подключений).
		// При partial=true (например WH timeout) кэшировать не нужно — следующий
		// запрос попробует live ещё раз и может получить полный список.
		if !partial && database.RedisClient != nil {
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

	// KPI breakdown WH/WL — точные числа из wialon_connection_summary (тот же SQL что /sources-stats).
	// Это даёт согласованность с дашбордом: live-fetch /wialon/all-units может прыгать при timeout WH,
	// а connection_summary стабильно (заполняется WialonStatsScheduler из top-level resource).
	whTotal, whActive, wlTotal, wlActive := 0, 0, 0, 0
	type kpiRow struct {
		ConnType string
		Total    int64
		Active   int64
	}
	var kpiRows []kpiRow
	if err := database.DB.Raw(`
		SELECT
			wc.connection_type                     AS conn_type,
			COALESCE(SUM(wcs.objects_total), 0)    AS total,
			COALESCE(SUM(wcs.objects_active), 0)   AS active
		FROM wialon_connections wc
		LEFT JOIN wialon_connection_summary wcs ON wcs.connection_id = wc.id
		WHERE wc.company_id = ? AND wc.deleted_at IS NULL AND wc.is_active = true
		GROUP BY wc.connection_type
	`, companyID).Scan(&kpiRows).Error; err != nil {
		log.Printf("⚠️ unified/objects wialon kpi: %v", err)
	}
	for _, r := range kpiRows {
		switch r.ConnType {
		case "hosting":
			whTotal = int(r.Total)
			whActive = int(r.Active)
		case "local":
			wlTotal = int(r.Total)
			wlActive = int(r.Active)
		}
	}

	// Маппинг user_id → name через Redis cache /wialon/all-accounts.
	// Нужен для подмены accountName (биллинг-ресурс bact) и creatorName (crt user)
	// реальными именами аккаунтов вместо UserName подключения.
	userIDToName := buildWialonUserIDToNameMap(companyID)

	// Items — отдельным проходом (для отображения нужны телефоны/hw/last_message).
	pattern := strings.ToLower(search)
	out := make([]UnifiedObject, 0, len(units))

	for _, u := range units {
		t := connType[u.ConnectionID]
		if source == "wh" && t != "wh" {
			continue
		}
		if source == "wl" && t != "wl" {
			continue
		}

		if pattern != "" {
			idStr := strconv.FormatInt(u.ID, 10)
			// Имена аккаунта-владельца (биллинг ресурс) и creator-юзера —
			// нужны для cross-section поиска "объекты аккаунта X".
			billingAccName := strings.ToLower(userIDToName[u.BillingAccountId])
			crtName := strings.ToLower(userIDToName[u.CreatorId])
			if !strings.Contains(strings.ToLower(u.Name), pattern) &&
				!strings.Contains(strings.ToLower(u.UniqueID), pattern) &&
				!strings.Contains(strings.ToLower(u.HardwareTypeName), pattern) &&
				!strings.Contains(strings.ToLower(u.ConnectionName), pattern) &&
				!strings.Contains(strings.ToLower(connOwner[u.ConnectionID]), pattern) &&
				!strings.Contains(billingAccName, pattern) &&
				!strings.Contains(crtName, pattern) &&
				!strings.Contains(u.PhoneNumber, pattern) &&
				!strings.Contains(u.PhoneNumber2, pattern) &&
				!strings.Contains(idStr, pattern) {
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

		// accountName ← имя биллинг-ресурса (bact) если в map; иначе ConnectionName
		// creatorName ← имя creator user (crt); иначе UserName подключения
		accountName := userIDToName[u.BillingAccountId]
		if accountName == "" {
			accountName = u.ConnectionName
		}
		creatorName := userIDToName[u.CreatorId]
		if creatorName == "" {
			creatorName = connOwner[u.ConnectionID]
		}

		connID := u.ConnectionID
		out = append(out, UnifiedObject{
			ID:                  u.ID,
			Name:                u.Name,
			UniqueID:            u.UniqueID,
			IMEI:                u.UniqueID,
			IsActive:            true,
			AccountName:         accountName,
			CreatorName:         creatorName,
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

// buildWialonUserIDToNameMap строит map id → name для Wialon-юнитов.
// Источники приоритетно:
//  1. wialon_users.short_name (prp.label/short_name — то что CMS Manager показывает)
//  2. wialon_users.name (полное имя avl_user.nm)
//  3. Redis cache /wialon/all-accounts (top-level + sub_users)
//  4. resource_id → user.name через wialon_object_stats (для bact ресурсов)
func buildWialonUserIDToNameMap(companyID uint) map[int64]string {
	out := make(map[int64]string)

	// 1+2) wialon_users — самый точный источник, ровно как CMS показывает.
	type userRow struct {
		UserID    int64
		Name      string
		ShortName string
	}
	var userRows []userRow
	if err := database.DB.Raw(`
		SELECT wu.user_id, wu.name, wu.short_name
		FROM wialon_users wu
		JOIN wialon_connections wc ON wc.id = wu.connection_id
		WHERE wc.company_id = ? AND wc.deleted_at IS NULL
	`, companyID).Scan(&userRows).Error; err == nil {
		for _, r := range userRows {
			if r.UserID <= 0 {
				continue
			}
			if r.ShortName != "" {
				out[r.UserID] = r.ShortName
			} else if r.Name != "" {
				out[r.UserID] = r.Name
			}
		}
	}

	// 3) Redis fallback на случай если scheduler ещё не успел собрать users
	if database.RedisClient == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	cached, err := database.RedisClient.Get(ctx, allAccountsCacheKey(companyID)).Bytes()
	if err != nil || len(cached) == 0 {
		return out
	}
	// Cache wrap: { "data": { "items": [...] } } или { "items": [...] }
	type accountItem struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		SubUsers []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"sub_users,omitempty"`
	}
	var wrapped struct {
		Data struct {
			Items []accountItem `json:"items"`
		} `json:"data"`
		Items []accountItem `json:"items"`
	}
	if err := json.Unmarshal(cached, &wrapped); err != nil {
		return out
	}
	items := wrapped.Data.Items
	if len(items) == 0 {
		items = wrapped.Items
	}
	for _, a := range items {
		if a.ID > 0 && a.Name != "" && out[a.ID] == "" {
			out[a.ID] = a.Name
		}
		for _, s := range a.SubUsers {
			if s.ID > 0 && s.Name != "" && out[s.ID] == "" {
				out[s.ID] = s.Name
			}
		}
	}

	// Добавляем resource_id → user.name через wialon_object_stats.
	// avl_unit.bact = resource_id; resource.user_id = avl_user.id.
	type resRow struct {
		ResourceID int64
		UserID     int64
	}
	var resRows []resRow
	if err := database.DB.Raw(`
		SELECT DISTINCT wos.resource_id, wos.user_id
		FROM wialon_object_stats wos
		JOIN wialon_connections wc ON wc.id = wos.connection_id
		WHERE wc.company_id = ? AND wc.deleted_at IS NULL
	`, companyID).Scan(&resRows).Error; err == nil {
		for _, r := range resRows {
			if r.ResourceID > 0 && r.UserID > 0 {
				if name, ok := out[r.UserID]; ok && out[r.ResourceID] == "" {
					out[r.ResourceID] = name
				}
			}
		}
	}
	return out
}

// fetchGeliosObjectsFast — объекты GELIOS (gelios_units) per company.
// Зеркало паттерна fetchGeliosUsersFast/Accounts. active = is_active
// (!removed && !isBlock из sync). Возвращает: objects, total, active, inactive.
func fetchGeliosObjectsFast(companyID uint, search, active string) ([]UnifiedObject, int, int, int) {
	var connections []models.GeliosConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).
		Find(&connections).Error; err != nil {
		log.Printf("⚠️ fetchGeliosObjectsFast: connections: %v", err)
		return nil, 0, 0, 0
	}
	if len(connections) == 0 {
		return nil, 0, 0, 0
	}
	connByID := make(map[uint]string, len(connections))
	connIDs := make([]uint, 0, len(connections))
	for _, cn := range connections {
		connByID[cn.ID] = cn.Name
		connIDs = append(connIDs, cn.ID)
	}

	q := database.DB.Model(&models.GeliosUnit{}).
		Where("connection_id IN ? AND gelios_deleted_at IS NULL", connIDs)

	if search != "" {
		terms := splitSearchTerms(search)
		// Поиск по name/imei/phone/phone2 — строка поиска обещает
		// «по названию, IMEI, номеру телефона» (Codex #5b).
		if len(terms) > 1 {
			ph := make([]string, 0, len(terms)*4)
			args := make([]any, 0, len(terms)*4)
			for _, t := range terms {
				p := "%" + strings.ToLower(t) + "%"
				ph = append(ph, "LOWER(name) LIKE ?", "LOWER(imei) LIKE ?", "LOWER(phone) LIKE ?", "LOWER(phone2) LIKE ?")
				args = append(args, p, p, p, p)
			}
			q = q.Where(strings.Join(ph, " OR "), args...)
		} else {
			p := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(name) LIKE ? OR LOWER(imei) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(phone2) LIKE ?", p, p, p, p)
		}
	}
	if active == "true" || active == "1" {
		q = q.Where("is_active = ?", true)
	} else if active == "false" || active == "0" {
		q = q.Where("is_active = ?", false)
	}

	var totalCount int64
	if err := q.Count(&totalCount).Error; err != nil {
		log.Printf("⚠️ fetchGeliosObjectsFast count: %v", err)
		return nil, 0, 0, 0
	}
	var activeCount int64
	q.Session(&gorm.Session{}).Where("is_active = ?", true).Count(&activeCount)

	var rows []models.GeliosUnit
	if err := q.Order("name ASC").Limit(5000).Find(&rows).Error; err != nil {
		log.Printf("⚠️ fetchGeliosObjectsFast find: %v", err)
		return nil, 0, 0, 0
	}

	out := make([]UnifiedObject, 0, len(rows))
	for _, r := range rows {
		connName := connByID[r.ConnectionID]
		sourceLabel := "GELIOS"
		if connName != "" {
			sourceLabel = "GELIOS(" + connName + ")"
		}
		createdAt := ""
		if r.GeliosCreatedAt != nil {
			createdAt = r.GeliosCreatedAt.Format(time.RFC3339)
		}
		lastMsg := ""
		if r.LastMsgAt != nil {
			lastMsg = r.LastMsgAt.Format(time.RFC3339)
		}
		phones := make([]string, 0, 2)
		if r.Phone != "" {
			phones = append(phones, r.Phone)
		}
		if r.Phone2 != "" {
			phones = append(phones, r.Phone2)
		}
		// Идентификатор (колонка ID/IMEI) = IMEI (реальный device id в GELIOS),
		// fallback на внутренний gelios unit id если imei пуст.
		uniqueID := r.IMEI
		if uniqueID == "" {
			uniqueID = r.GeliosUnitID
		}
		connID := r.ConnectionID
		out = append(out, UnifiedObject{
			ID:       int64(r.ID),
			Name:     r.Name,
			UniqueID: uniqueID,
			IMEI:     r.IMEI,
			IsActive: r.IsActive,
			// Учетка = владелец объекта в GELIOS (creator-узел дерева).
			AccountName:         r.GeliosCreatorLogin,
			CreatorName:         r.GeliosCreatorLogin,
			DeviceTypeName:      r.HwTypeName,
			PhoneNumbers:        phones,
			CreatedAt:           createdAt,
			LastMessageDatetime: lastMsg,
			Source:              "gelios",
			SourceLabel:         sourceLabel,
			ConnectionID:        &connID,
		})
	}
	return out, int(totalCount), int(activeCount), int(totalCount) - int(activeCount)
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
