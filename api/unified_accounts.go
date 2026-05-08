package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
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

// UnifiedAccount — единая структура учётной записи из любого источника.
// Унификация с UnifiedUser/UnifiedObject (camelCase), поля совпадают с
// frontend-таблицей AccountsTable.vue.
type UnifiedAccount struct {
	ID                int     `json:"id"`
	Name              string  `json:"name"`
	Type              string  `json:"type"` // client | partner
	AdminFullname     string  `json:"adminFullname,omitempty"`
	AdminID           int     `json:"adminId,omitempty"`
	AdminIsActive     bool    `json:"adminIsActive,omitempty"`
	ParentAccountName string  `json:"parentAccountName,omitempty"`
	ObjectsActive     int     `json:"objectsActive"`
	ObjectsTotal      int     `json:"objectsTotal"`
	ObjectsDeleted    int     `json:"objectsDeleted,omitempty"`
	IsActive          bool    `json:"isActive"`
	Hierarchy         string  `json:"hierarchy,omitempty"`
	CreationDatetime  string  `json:"creationDatetime,omitempty"`
	Comment           *string `json:"comment,omitempty"`
	BlockingDatetime  *string `json:"blockingDatetime,omitempty"`
	DaysBeforeBlocking *int   `json:"daysBeforeBlocking,omitempty"`
	Source            string  `json:"source"`                 // "axenta" | "WH(...)" | "WL(...)" | "skif"
	SourceLabel       string  `json:"sourceLabel,omitempty"`
	ConnectionID      *uint   `json:"connectionId,omitempty"` // wialon + skif
	DealerRights      bool    `json:"dealerRights,omitempty"`
	SkifCompanyID     string  `json:"skifCompanyId,omitempty"`     // UUID дилерской компании в SKIF
	DeleteScheduledFor *string `json:"deleteScheduledFor,omitempty"` // ISO timestamp когда SKIF удалит компанию (RFC3339)
}

// UnifiedAccountsStats — KPI разбивка по источникам (как для users/objects).
type UnifiedAccountsStats struct {
	AxentaTotal    int `json:"axenta_total"`
	AxentaActive   int `json:"axenta_active"`
	AxentaClients  int `json:"axenta_clients"`
	AxentaPartners int `json:"axenta_partners"`
	WialonTotal    int `json:"wialon_total"`
	WialonActive   int `json:"wialon_active"`
	WialonWHTotal  int `json:"wialon_wh_total"`
	WialonWHActive int `json:"wialon_wh_active"`
	WialonWLTotal  int `json:"wialon_wl_total"`
	WialonWLActive int `json:"wialon_wl_active"`
	SkifTotal      int `json:"skif_total"`
	SkifActive     int `json:"skif_active"`
}

// UnifiedAccountsResponse — формат ответа.
type UnifiedAccountsResponse struct {
	Items      []UnifiedAccount     `json:"items"`
	Total      int                  `json:"total"`
	Page       int                  `json:"page"`
	PerPage    int                  `json:"per_page"`
	TotalPages int                  `json:"total_pages"`
	Stats      UnifiedAccountsStats `json:"stats"`
}

// GetUnifiedAccounts — единая выдача учёток из Axenta + Wialon с серверной пагинацией.
// Решает костыль double-pagination в frontend: useAccountsList грузил все 1000+ axenta
// + все wialon, потом client-side merge + slice. Теперь backend сам merge'ит и отдаёт страницу.
//
// @Param page query int false "Номер страницы" default(1)
// @Param per_page query int false "Записей на страницу" default(20)
// @Param search query string false "Поиск по имени/админу"
// @Param source query string false "Источник: all|axenta|wh|wl" default(all)
// @Param type query string false "Тип: client|partner"
// @Param is_active query bool false "Только активные"
// @Param ordering query string false "Сортировка: name|-name|creationDatetime|-creationDatetime"
// @Router /api/unified/accounts [get]
func GetUnifiedAccounts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	search := strings.TrimSpace(c.Query("search"))
	source := strings.ToLower(c.DefaultQuery("source", "all"))
	accountType := c.Query("type")
	activeStr := c.Query("is_active")
	ordering := c.DefaultQuery("ordering", "-creationDatetime")
	parent := strings.TrimSpace(c.Query("parent"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	allAccounts := make([]UnifiedAccount, 0)
	var stats UnifiedAccountsStats
	var wg sync.WaitGroup
	var mu sync.Mutex

	loadAxenta := source == "all" || source == "axenta"
	loadWialon := source == "all" || source == "wialon" || source == "wh" || source == "wl"
	loadSkif := source == "all" || source == "skif"

	if loadAxenta {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tenantDB := middleware.GetTenantDB(c)
			t0 := time.Now()
			items, total, active, clients, partners := fetchAxentaAccountsForUnified(tenantDB, search, accountType, activeStr, parent)
			log.Printf("🔍 unified/accounts axenta: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allAccounts = append(allAccounts, items...)
			stats.AxentaTotal = total
			stats.AxentaActive = active
			stats.AxentaClients = clients
			stats.AxentaPartners = partners
			mu.Unlock()
		}()
	}

	if loadWialon {
		wg.Add(1)
		go func() {
			defer wg.Done()
			companyID, exists := c.Get("company_id")
			if !exists {
				return
			}
			t0 := time.Now()
			items, wTotal, wActive, whTotal, whActive, wlTotal, wlActive := fetchWialonAccountsForUnified(companyID.(uint), search, accountType, activeStr, source, parent)
			log.Printf("🔍 unified/accounts wialon: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allAccounts = append(allAccounts, items...)
			stats.WialonTotal = wTotal
			stats.WialonActive = wActive
			stats.WialonWHTotal = whTotal
			stats.WialonWHActive = whActive
			stats.WialonWLTotal = wlTotal
			stats.WialonWLActive = wlActive
			mu.Unlock()
		}()
	}

	if loadSkif {
		wg.Add(1)
		go func() {
			defer wg.Done()
			companyID, exists := c.Get("company_id")
			if !exists {
				return
			}
			t0 := time.Now()
			items, sTotal, sActive := fetchSkifAccountsForUnified(companyID.(uint), search, accountType, activeStr, parent)
			log.Printf("🔍 unified/accounts skif: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allAccounts = append(allAccounts, items...)
			stats.SkifTotal = sTotal
			stats.SkifActive = sActive
			mu.Unlock()
		}()
	}

	wg.Wait()

	sortUnifiedAccounts(allAccounts, ordering)

	total := len(allAccounts)
	startIndex := (page - 1) * perPage
	endIndex := startIndex + perPage
	if startIndex > total {
		startIndex = total
	}
	if endIndex > total {
		endIndex = total
	}
	pageItems := allAccounts[startIndex:endIndex]
	totalPages := (total + perPage - 1) / perPage

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": UnifiedAccountsResponse{
			Items:      pageItems,
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
			Stats:      stats,
		},
	})
}

// fetchAxentaAccountsForUnified читает axenta_account_snapshots с фильтрами.
// Возвращает: items, total, active, clients, partners.
func fetchAxentaAccountsForUnified(db *gorm.DB, search, accountType, activeStr, parent string) ([]UnifiedAccount, int, int, int, int) {
	if db == nil {
		return nil, 0, 0, 0, 0
	}

	// TTL check
	var lastSync time.Time
	if err := db.Model(&models.AxentaAccountSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, 0, 0, 0, 0
	}
	if time.Since(lastSync) > SnapshotTTL {
		log.Printf("⏰ unified/accounts: snapshot устарел (last=%v), результат пустой", lastSync)
		return nil, 0, 0, 0, 0
	}

	q := db.Model(&models.AxentaAccountSnapshot{})

	if search != "" {
		// Case-insensitive поиск с поддержкой кириллицы:
		// LOWER(col COLLATE "und-x-icu") LIKE LOWER(?). Без COLLATE LOWER() в БД
		// с lc_ctype=C не downcase'ит кириллицу (PostgreSQL libc локализация).
		// Inline COLLATE "und-x-icu" даёт ICU case-folding без миграций схемы
		// (nondeterministic collation на колонке ломает LIKE — см. PG docs).
		terms := splitSearchTerms(search)
		if len(terms) > 1 {
			args := make([]any, 0, len(terms)*3)
			parts := make([]string, 0, len(terms)*3)
			for _, t := range terms {
				p := "%" + strings.ToLower(t) + "%"
				args = append(args, p, p, p)
				parts = append(parts,
					`LOWER(account_name COLLATE "und-x-icu") LIKE ?`,
					`LOWER(admin_fullname COLLATE "und-x-icu") LIKE ?`,
					`LOWER(parent_account_name COLLATE "und-x-icu") LIKE ?`,
				)
			}
			q = q.Where(strings.Join(parts, " OR "), args...)
		} else {
			pattern := "%" + strings.ToLower(search) + "%"
			q = q.Where(
				`LOWER(account_name COLLATE "und-x-icu") LIKE ? OR `+
					`LOWER(admin_fullname COLLATE "und-x-icu") LIKE ? OR `+
					`LOWER(parent_account_name COLLATE "und-x-icu") LIKE ?`,
				pattern, pattern, pattern)
		}
	}
	if accountType != "" {
		q = q.Where("account_type = ?", accountType)
	}
	switch activeStr {
	case "true", "1":
		q = q.Where("is_active = ?", true)
	case "false", "0":
		q = q.Where("is_active = ?", false)
	}
	var rows []models.AxentaAccountSnapshot
	if err := q.Find(&rows).Error; err != nil {
		log.Printf("⚠️ unified/accounts axenta find: %v", err)
		return nil, 0, 0, 0, 0
	}

	items := make([]UnifiedAccount, 0, len(rows))
	total, active, clients, partners := 0, 0, 0, 0
	for _, r := range rows {
		if parent != "" && !isDirectParent(r.ParentAccountName, r.Hierarchy, parent) {
			continue
		}
		total++
		if r.IsActive {
			active++
		}
		switch r.AccountType {
		case "client":
			clients++
		case "partner":
			partners++
		}
		ua := UnifiedAccount{
			ID:                int(r.ExternalAccountID),
			Name:              r.AccountName,
			Type:              r.AccountType,
			AdminFullname:     r.AdminFullname,
			ParentAccountName: r.ParentAccountName,
			ObjectsActive:     r.ObjectsActive,
			ObjectsTotal:      r.ObjectsTotal,
			IsActive:          r.IsActive,
			Hierarchy:         r.Hierarchy,
			Source:            "axenta",
			SourceLabel:       "Axenta Cloud",
			Comment:           strPtrIfNotEmpty(r.Comment),
			DaysBeforeBlocking: r.DaysBeforeBlocking,
		}
		if r.AdminExternalID != nil {
			ua.AdminID = int(*r.AdminExternalID)
		}
		if !r.CreatedAt.IsZero() {
			ua.CreationDatetime = r.CreatedAt.Format(time.RFC3339)
		}
		if r.BlockingDatetime != nil {
			s := r.BlockingDatetime.Format(time.RFC3339)
			ua.BlockingDatetime = &s
		}
		items = append(items, ua)
	}
	return items, total, active, clients, partners
}

// isDirectParent проверяет что `target` — прямой родитель (parent_account_name либо
// предпоследний элемент hierarchy "A > B > Self").
func isDirectParent(parentName, hierarchy, target string) bool {
	if parentName == target {
		return true
	}
	if hierarchy == "" {
		return false
	}
	parts := strings.Split(hierarchy, " > ")
	if len(parts) < 2 {
		return false
	}
	return parts[len(parts)-2] == target
}

func strPtrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// fetchWialonAccountsForUnified читает Redis-cache wialon:all-accounts:<companyID>
// с фильтрами. Возвращает: items, total, active, whTotal, whActive, wlTotal, wlActive.
//
// objectsTotal/Active обогащаются из wialon_object_stats (per-resource БД-кэш,
// заполняется WialonStatsScheduler) одним SELECT по всем connection_ids компании.
// Это убирает sentinel objectsTotal=-1 в Redis-кэше — frontend получает готовые
// цифры сразу, без второго round-trip к /wialon/connections/{id}/objects-stats
// (который делал live-fetch на 5-15 сек при холодном Redis).
func fetchWialonAccountsForUnified(companyID uint, search, accountType, activeStr, sourceFilter, parent string) ([]UnifiedAccount, int, int, int, int, int, int) {
	if database.RedisClient == nil {
		return nil, 0, 0, 0, 0, 0, 0
	}
	cached, err := database.RedisClient.Get(context.Background(), allAccountsCacheKey(companyID)).Bytes()
	if err != nil || len(cached) == 0 {
		return nil, 0, 0, 0, 0, 0, 0
	}

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []WialonAccountInfo `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(cached, &resp); err != nil {
		log.Printf("⚠️ unified/accounts wialon parse: %v", err)
		return nil, 0, 0, 0, 0, 0, 0
	}

	statsByUserID := loadWialonStatsForCompany(companyID)

	pattern := strings.ToLower(search)
	items := make([]UnifiedAccount, 0, len(resp.Data.Items))
	total, active, whTotal, whActive, wlTotal, wlActive := 0, 0, 0, 0, 0, 0
	for _, a := range resp.Data.Items {
		if sourceFilter == "wh" && !strings.HasPrefix(a.SourceLabel, "WH(") {
			continue
		}
		if sourceFilter == "wl" && !strings.HasPrefix(a.SourceLabel, "WL(") {
			continue
		}
		if accountType != "" && a.Type != accountType {
			continue
		}
		switch activeStr {
		case "true", "1":
			if !a.IsActive {
				continue
			}
		case "false", "0":
			if a.IsActive {
				continue
			}
		}
		if pattern != "" {
			if !strings.Contains(strings.ToLower(a.Name), pattern) {
				continue
			}
		}
		if parent != "" && !isDirectParent("", a.Hierarchy, parent) {
			continue
		}

		total++
		if a.IsActive {
			active++
		}
		if strings.HasPrefix(a.SourceLabel, "WH(") {
			whTotal++
			if a.IsActive {
				whActive++
			}
		} else if strings.HasPrefix(a.SourceLabel, "WL(") {
			wlTotal++
			if a.IsActive {
				wlActive++
			}
		}

		connID := a.ConnectionID
		objectsActive := a.ObjectsActive
		objectsTotal := a.ObjectsTotal
		// Обогащаем из БД-кэша wialon_object_stats. Redis хранит -1 (sentinel),
		// БД-кэш заполняется WialonStatsScheduler. Если БД-stats нет — оставляем
		// значение из Redis (frontend сам решает что показать).
		if st, ok := statsByUserID[int64(a.ID)]; ok {
			objectsTotal = st.ObjectsTotal
			objectsActive = st.ObjectsActive
		}
		ua := UnifiedAccount{
			ID:               a.ID,
			Name:             a.Name,
			Type:             a.Type,
			ObjectsActive:    objectsActive,
			ObjectsTotal:     objectsTotal,
			IsActive:         a.IsActive,
			Hierarchy:        a.Hierarchy,
			Source:           a.SourceLabel,
			SourceLabel:      a.SourceLabel,
			ConnectionID:     &connID,
			DealerRights:     a.DealerRights,
			CreationDatetime: a.CreatedAt,
		}
		items = append(items, ua)
	}

	return items, total, active, whTotal, whActive, wlTotal, wlActive
}

// loadWialonStatsForCompany читает wialon_object_stats для всех wialon-подключений
// компании одним SELECT. Возвращает map[user_id] → stat (тот же ключ что использует
// frontend и /wialon/connections/:id/objects-stats endpoint).
//
// Без этого WH/WL items в /unified/accounts приходят с objectsTotal=-1 (Redis-sentinel)
// и frontend вынужден делать второй round-trip per-connection — холодный live-fetch
// занимал 5-15 сек. БД-кэш заполняется WialonStatsScheduler фоном, чтение мгновенное.
func loadWialonStatsForCompany(companyID uint) map[int64]models.WialonObjectStat {
	result := make(map[int64]models.WialonObjectStat)
	if database.DB == nil {
		return result
	}
	// JOIN по wialon_connections.company_id — wialon_object_stats глобальная.
	var rows []models.WialonObjectStat
	err := database.DB.
		Joins("JOIN wialon_connections ON wialon_connections.id = wialon_object_stats.connection_id").
		Where("wialon_connections.company_id = ?", companyID).
		Find(&rows).Error
	if err != nil {
		log.Printf("⚠️ unified/accounts: load wialon_object_stats: %v", err)
		return result
	}
	for _, r := range rows {
		if r.UserID > 0 {
			result[r.UserID] = r
		}
	}
	return result
}

// RegisterUnifiedAccountsRoutes регистрирует /api/unified/accounts.
func RegisterUnifiedAccountsRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/unified/accounts", GetUnifiedAccounts)
	apiGroup.GET("/unified/accounts/", GetUnifiedAccounts)
	log.Println("✅ Unified Accounts API routes registered")
}

// sortUnifiedAccounts применяет ordering ("name", "-name", "creationDatetime", "-creationDatetime", "objectsTotal", "-objectsTotal").
func sortUnifiedAccounts(items []UnifiedAccount, ordering string) {
	desc := strings.HasPrefix(ordering, "-")
	field := strings.TrimPrefix(ordering, "-")

	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		switch field {
		case "name":
			less = strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		case "objectsTotal", "objects_total":
			less = items[i].ObjectsTotal < items[j].ObjectsTotal
		case "objectsActive", "objects_active":
			less = items[i].ObjectsActive < items[j].ObjectsActive
		case "creationDatetime", "creation_datetime":
			less = items[i].CreationDatetime < items[j].CreationDatetime
		default:
			less = items[i].ID < items[j].ID
		}
		if desc {
			return !less
		}
		return less
	})
}

// fetchSkifAccountsForUnified — учётки SKIF = дилерские компании (DISTINCT skif_company_id).
// SkifCompany не имеет своей таблицы, агрегируем из skif_units одним SQL.
//
// Возвращает: items, total, active. Для KPI карточки SKIF на /accounts.
func fetchSkifAccountsForUnified(companyID uint, search, accountType, activeStr, parent string) ([]UnifiedAccount, int, int) {
	if database.DB == nil {
		return nil, 0, 0
	}

	// Pending-deletes для UI countdown (LEFT JOIN не делаем — отдельный map).
	pendingMap := make(map[string]time.Time)
	var pendings []models.SkifPendingDelete
	if err := database.DB.
		Joins("JOIN skif_connections sc ON sc.id = skif_pending_deletes.connection_id").
		Where("sc.company_id = ?", companyID).
		Find(&pendings).Error; err == nil {
		for _, p := range pendings {
			pendingMap[p.SkifCompanyID] = p.ScheduledFor
		}
	}
	// SkifCompany считается активной если у неё есть хотя бы один активный юнит.
	// Источник created_at — MIN(skif_created_at) по юнитам компании.
	type row struct {
		SkifCompanyID  string
		Name           string
		ConnectionID   uint
		ConnectionName string
		ObjectsTotal   int
		ObjectsActive  int
		CreatedAt      *time.Time
	}
	var rows []row
	q := `
		SELECT
			u.skif_company_id                                             AS skif_company_id,
			MAX(u.skif_company)                                           AS name,
			MAX(u.connection_id)                                          AS connection_id,
			MAX(c.name)                                                   AS connection_name,
			COUNT(*)                                                      AS objects_total,
			COUNT(*) FILTER (WHERE u.is_active AND u.skif_deleted_at IS NULL) AS objects_active,
			MIN(u.skif_created_at)                                        AS created_at
		FROM skif_units u
		LEFT JOIN skif_connections c ON c.id = u.connection_id
		WHERE u.company_id = ?
		  AND u.skif_company_id <> ''
		  AND u.skif_deleted_at IS NULL
		GROUP BY u.skif_company_id
	`
	if err := database.DB.Raw(q, companyID).Scan(&rows).Error; err != nil {
		log.Printf("⚠️ unified/accounts skif aggregate: %v", err)
		return nil, 0, 0
	}

	pattern := strings.ToLower(search)
	items := make([]UnifiedAccount, 0, len(rows))
	total, active := 0, 0
	for _, r := range rows {
		isActive := r.ObjectsActive > 0
		// SKIF не делит на client/partner — все дилерские компании = client.
		if accountType != "" && accountType != "client" {
			continue
		}
		switch activeStr {
		case "true", "1":
			if !isActive {
				continue
			}
		case "false", "0":
			if isActive {
				continue
			}
		}
		if pattern != "" && !strings.Contains(strings.ToLower(r.Name), pattern) {
			continue
		}
		// parent для SKIF — имя SkifConnection (дилеры висят под connection как под "партнёром").
		if parent != "" && r.ConnectionName != parent {
			continue
		}

		total++
		if isActive {
			active++
		}

		connID := r.ConnectionID
		ua := UnifiedAccount{
			ID:                int(connID)*1000000 + hashSkifID(r.SkifCompanyID), // synthetic — UnifiedAccount.ID должен быть int
			Name:              r.Name,
			Type:              "client",
			ParentAccountName: r.ConnectionName,
			ObjectsTotal:      r.ObjectsTotal,
			ObjectsActive:     r.ObjectsActive,
			IsActive:          isActive,
			Source:            "skif",
			SourceLabel:       "SKIF",
			ConnectionID:      &connID,
			SkifCompanyID:     r.SkifCompanyID,
		}
		if r.CreatedAt != nil && !r.CreatedAt.IsZero() {
			ua.CreationDatetime = r.CreatedAt.Format(time.RFC3339)
		}
		if scheduledFor, ok := pendingMap[r.SkifCompanyID]; ok && !scheduledFor.IsZero() {
			s := scheduledFor.Format(time.RFC3339)
			ua.DeleteScheduledFor = &s
		}
		items = append(items, ua)
	}
	return items, total, active
}

// hashSkifID — детерминированный int ID из UUID для UnifiedAccount.ID (frontend ключ).
// Реальный SkifCompanyID лежит в SkifCompanyID поле.
func hashSkifID(uuid string) int {
	var h int
	for i := 0; i < len(uuid) && i < 16; i++ {
		h = h*31 + int(uuid[i])
	}
	if h < 0 {
		h = -h
	}
	return h % 1000000
}
