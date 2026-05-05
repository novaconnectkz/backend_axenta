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
	Source            string  `json:"source"`                 // "axenta" | "WH(...)" | "WL(...)"
	SourceLabel       string  `json:"sourceLabel,omitempty"`
	ConnectionID      *uint   `json:"connectionId,omitempty"` // только для wialon
	DealerRights      bool    `json:"dealerRights,omitempty"`
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
		// ILIKE вместо LOWER(x) LIKE LOWER(?) — postgres LOWER() без ICU collate
		// не downcase'ит кириллицу, из-за чего "Служе" не находил "Служебные авто".
		// ILIKE использует collate-aware case-folding и работает с UTF-8 искаропки.
		terms := splitSearchTerms(search)
		if len(terms) > 1 {
			args := make([]any, 0, len(terms)*3)
			parts := make([]string, 0, len(terms)*3)
			for _, t := range terms {
				p := "%" + t + "%"
				args = append(args, p, p, p)
				parts = append(parts, "account_name ILIKE ?", "admin_fullname ILIKE ?", "parent_account_name ILIKE ?")
			}
			q = q.Where(strings.Join(parts, " OR "), args...)
		} else {
			pattern := "%" + search + "%"
			q = q.Where("account_name ILIKE ? OR admin_fullname ILIKE ? OR parent_account_name ILIKE ?", pattern, pattern, pattern)
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
		ua := UnifiedAccount{
			ID:               a.ID,
			Name:             a.Name,
			Type:             a.Type,
			ObjectsActive:    a.ObjectsActive,
			ObjectsTotal:     a.ObjectsTotal,
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
