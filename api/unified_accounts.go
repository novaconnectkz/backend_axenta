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
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"` // client | partner
	AdminFullname      string  `json:"adminFullname,omitempty"`
	AdminID            int     `json:"adminId,omitempty"`
	AdminIsActive      bool    `json:"adminIsActive,omitempty"`
	ParentAccountName  string  `json:"parentAccountName,omitempty"`
	ObjectsActive      int     `json:"objectsActive"`
	ObjectsTotal       int     `json:"objectsTotal"`
	ObjectsDeleted     int     `json:"objectsDeleted,omitempty"`
	IsActive           bool    `json:"isActive"`
	Hierarchy          string  `json:"hierarchy,omitempty"`
	CreationDatetime   string  `json:"creationDatetime,omitempty"`
	Comment            *string `json:"comment,omitempty"`
	BlockingDatetime   *string `json:"blockingDatetime,omitempty"`
	DaysBeforeBlocking *int    `json:"daysBeforeBlocking,omitempty"`
	Source             string  `json:"source"` // "axenta" | "WH(...)" | "WL(...)" | "skif"
	SourceLabel        string  `json:"sourceLabel,omitempty"`
	IsMine             bool    `json:"isMine,omitempty"`       // прямой ребёнок нашей интеграционной учётки (scope=mine)
	ConnectionID       *uint   `json:"connectionId,omitempty"` // wialon + skif
	DealerRights       bool    `json:"dealerRights,omitempty"`
	SkifCompanyID      string  `json:"skifCompanyId,omitempty"`      // UUID дилерской компании в SKIF
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
	WialonPartners int `json:"wialon_partners"` // Ф2: дилеры (dealer_rights) из wialon_account_statuses
	SkifTotal      int `json:"skif_total"`
	SkifActive     int `json:"skif_active"`
	GeliosTotal    int `json:"gelios_total"`
	GeliosActive   int `json:"gelios_active"`
	GeliosClients  int `json:"gelios_clients"`
	GeliosPartners int `json:"gelios_partners"`
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
	// scope=mine — только прямые дети наших интеграционных учёток (см. /parents).
	// Предикат per-system: axenta/wialon depth(hierarchy)==2, skif — компании,
	// gelios — creator_login == имя подключения.
	mineOnly := strings.ToLower(c.Query("scope")) == "mine"
	// Ф0: режим выбора партнёра в дропдауне — обходим TTL-гейт Axenta для полноты списка.
	pickerMode := c.Query("for") == "picker"

	if page < 1 {
		page = 1
	}
	// Picker-режим (дропдаун выбора партнёра) требует ПОЛНЫЙ список — иначе при
	// большом числе партнёров (напр. 330 Wialon-дилеров) cap отрезал бы хвост.
	// Обычные страницы остаются с cap 200 (default 20).
	if pickerMode {
		if perPage < 1 {
			perPage = 20
		} else if perPage > 100000 {
			perPage = 100000
		}
	} else if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	allAccounts := make([]UnifiedAccount, 0)
	var stats UnifiedAccountsStats
	var wg sync.WaitGroup
	var mu sync.Mutex

	loadAxenta := source == "all" || source == "axenta"
	loadWialon := source == "all" || source == "wialon" || source == "wh" || source == "wl"
	loadSkif := source == "all" || source == "skif"
	loadGelios := source == "all" || source == "gelios"

	if loadAxenta {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tenantDB := middleware.GetTenantDB(c)
			t0 := time.Now()
			items, total, active, clients, partners := fetchAxentaAccountsForUnified(tenantDB, search, accountType, activeStr, parent, mineOnly, pickerMode)
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
			items, wTotal, wActive, whTotal, whActive, wlTotal, wlActive := fetchWialonAccountsForUnified(companyID.(uint), search, accountType, activeStr, source, parent, mineOnly)
			log.Printf("🔍 unified/accounts wialon: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))

			// Ф2: Wialon-партнёры (дилеры) из wialon_account_statuses (DB, не Redis) —
			// отдельными type=partner строками. Только при accountType=partner
			// (запрос дропдауна договора): Redis-блок дилеров не отдаёт как partner,
			// поэтому дублей нет. DB-источник → работает на staging (Redis холодный).
			var partnerItems []UnifiedAccount
			var wPartners int
			if accountType == "partner" {
				partnerItems, wPartners = fetchWialonPartnersForUnified(companyID.(uint), search, activeStr, parent)
			}

			mu.Lock()
			allAccounts = append(allAccounts, items...)
			allAccounts = append(allAccounts, partnerItems...)
			stats.WialonTotal = wTotal
			stats.WialonActive = wActive
			stats.WialonWHTotal = whTotal
			stats.WialonWHActive = whActive
			stats.WialonWLTotal = wlTotal
			stats.WialonWLActive = wlActive
			stats.WialonPartners = wPartners
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
			items, sTotal, sActive := fetchSkifAccountsForUnified(companyID.(uint), search, accountType, activeStr, parent, mineOnly)
			log.Printf("🔍 unified/accounts skif: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allAccounts = append(allAccounts, items...)
			stats.SkifTotal = sTotal
			stats.SkifActive = sActive
			mu.Unlock()
		}()
	}

	if loadGelios {
		wg.Add(1)
		go func() {
			defer wg.Done()
			companyID, exists := c.Get("company_id")
			if !exists {
				return
			}
			t0 := time.Now()
			items, gTotal, gActive, gClients, gPartners := fetchGeliosAccountsForUnified(companyID.(uint), search, accountType, activeStr, parent, mineOnly)
			log.Printf("🔍 unified/accounts gelios: %d items за %s", len(items), time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allAccounts = append(allAccounts, items...)
			stats.GeliosTotal = gTotal
			stats.GeliosActive = gActive
			stats.GeliosClients = gClients
			stats.GeliosPartners = gPartners
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
func fetchAxentaAccountsForUnified(db *gorm.DB, search, accountType, activeStr, parent string, mineOnly, ignoreTTL bool) ([]UnifiedAccount, int, int, int, int) {
	if db == nil {
		return nil, 0, 0, 0, 0
	}

	// TTL check.
	// ignoreTTL=true (режим ?for=picker) обходит гейт «снимок устарел → пусто»:
	// для дропдауна выбора партнёра нам важна ПОЛНОТА списка (даже по устаревшему
	// снимку), а не свежесть. Основная страница /accounts (ignoreTTL=false) сохраняет
	// строгий TTL, чтобы не показывать протухшие данные.
	var lastSync time.Time
	if err := db.Model(&models.AxentaAccountSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, 0, 0, 0, 0
	}
	if !ignoreTTL && time.Since(lastSync) > SnapshotTTL {
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
		// Прямой ребёнок нашей учётки: hierarchy "GLOMOS > X" (глубина 2).
		// Корень (сам GLOMOS) и внуки (depth>2) — не наши прямые.
		isMine := hierarchyDepth(r.Hierarchy) == 2
		if mineOnly && !isMine {
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
			ID:                 int(r.ExternalAccountID),
			Name:               r.AccountName,
			Type:               r.AccountType,
			AdminFullname:      r.AdminFullname,
			ParentAccountName:  r.ParentAccountName,
			ObjectsActive:      r.ObjectsActive,
			ObjectsTotal:       r.ObjectsTotal,
			IsActive:           r.IsActive,
			Hierarchy:          r.Hierarchy,
			IsMine:             isMine,
			Source:             "axenta",
			SourceLabel:        "Axenta Cloud",
			Comment:            strPtrIfNotEmpty(r.Comment),
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

// hierarchyDepth — число узлов в "A > B > Self". 0 для пустой строки.
// Прямой ребёнок корня = глубина 2 ("Корень > Ребёнок").
func hierarchyDepth(hierarchy string) int {
	hierarchy = strings.TrimSpace(hierarchy)
	if hierarchy == "" {
		return 0
	}
	return len(strings.Split(hierarchy, " > "))
}

// accountParentLabel — имя непосредственного родителя записи (для дропдауна /parents).
// ParentAccountName приоритетнее (axenta/skif/gelios), иначе предпоследний узел hierarchy (wialon).
// Возвращаем RAW (без TrimSpace) — значение дропдауна должно побайтово совпасть с тем,
// что сравнивает isDirectParent / parent=-фильтр, иначе выбор из списка ничего не найдёт.
func accountParentLabel(ua UnifiedAccount) string {
	if ua.ParentAccountName != "" {
		return ua.ParentAccountName
	}
	parts := strings.Split(ua.Hierarchy, " > ")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
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
func fetchWialonAccountsForUnified(companyID uint, search, accountType, activeStr, sourceFilter, parent string, mineOnly bool) ([]UnifiedAccount, int, int, int, int, int, int) {
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

	// scope=mine: иерархия wialon = "SourceLabel > Родитель > Сам" (immediate parent =
	// parts[len-2]). Прямой ребёнок интеграции = тот, чей непосредственный родитель ==
	// имя интеграционной у/з подключения. Авторитетный источник имени — wialon_connections.user_name
	// (glomoskz/glomosuz/Профмонитор), НЕ wialon_connections.name (для conn 7 name=app.gpsnetwork.ru,
	// а у/з = Профмонитор). Парсинг self-set ненадёжен: интеграционная у/з сама есть в списке,
	// а при флапе/пагинации wialon строки теряются.
	userNameByConn := make(map[uint]string)
	if database.DB != nil {
		var crows []struct {
			ID       uint
			UserName string
		}
		if err := database.DB.Table("wialon_connections").
			Select("id, user_name").Where("company_id = ?", companyID).
			Scan(&crows).Error; err != nil {
			log.Printf("⚠️ unified/accounts wialon user_name map: %v", err)
		}
		for _, r := range crows {
			userNameByConn[r.ID] = r.UserName
		}
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
		// Прямой ребёнок интеграции: immediate parent (parts[len-2]) == user_name подключения.
		isMine := false
		if un := userNameByConn[a.ConnectionID]; un != "" {
			if parts := strings.Split(a.Hierarchy, " > "); len(parts) >= 2 {
				isMine = parts[len(parts)-2] == un
			}
		}
		if mineOnly && !isMine {
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
			IsMine:           isMine,
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

// fetchWialonPartnersForUnified — Wialon-дилеры (partner billing Ф2) из
// wialon_account_statuses (DB, не Redis → работает на staging). Дилер =
// dealer_rights=true. Per-account модель (A): дерево в Wialon-данных не строится
// (нет bpact, crt схлопывается, общий мастер-ресурс) → биллинг по прямым units_count.
//
// Source="wialon" + ID=wialon_user_id → ключ дропдауна wialon|connID|userID,
// partner_external_id=userID для снимков W4.
func fetchWialonPartnersForUnified(companyID uint, search, activeStr, parent string) ([]UnifiedAccount, int) {
	if parent != "" { // у Wialon-партнёров нет иерархии-родителя в этой модели
		return nil, 0
	}
	var rows []models.WialonAccountStatus
	// Только ПРЯМЫЕ дилеры под интеграционной у/з (is_direct_dealer): дилеры-дилеров
	// глубже по иерархии в дропдаун не выводим (parentAccountId != bact интеграции).
	if err := database.DB.
		Joins("JOIN wialon_connections wc ON wc.id = wialon_account_statuses.connection_id").
		Where("wc.company_id = ? AND wialon_account_statuses.dealer_rights = ? AND wialon_account_statuses.is_direct_dealer = ?", companyID, true, true).
		Find(&rows).Error; err != nil {
		log.Printf("⚠️ unified/accounts wialon partners: %v", err)
		return nil, 0
	}

	pattern := strings.ToLower(search)
	items := make([]UnifiedAccount, 0, len(rows))
	for _, r := range rows {
		if pattern != "" && !strings.Contains(strings.ToLower(r.Name), pattern) {
			continue
		}
		switch activeStr {
		case "true", "1":
			if !r.IsActive {
				continue
			}
		case "false", "0":
			if r.IsActive {
				continue
			}
		}
		connID := r.ConnectionID
		items = append(items, UnifiedAccount{
			ID:            int(r.WialonUserID),
			Name:          r.Name,
			Type:          "partner",
			ObjectsTotal:  r.UnitsCount,
			ObjectsActive: r.UnitsCount, // per-account: активные = прямые units
			IsActive:      r.IsActive,
			Source:        "wialon",
			SourceLabel:   "Wialon",
			ConnectionID:  &connID,
			DealerRights:  true,
			IsMine:        true, // is_direct_dealer=true → дилер прямо под интеграционной у/з
		})
	}
	return items, len(items)
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

// UnifiedParentsResponse — список родительских аккаунтов для дропдауна фильтра,
// разбитый на «наши» (прямые корни-подключения) и «остальные» (поддилеры).
type UnifiedParentsResponse struct {
	Mine   []string `json:"mine"`   // корни наших подключений (только с ≥1 прямым ребёнком)
	Others []string `json:"others"` // прочие родители (поддилеры и т.п.)
}

// GetUnifiedAccountParents — собирает уникальные имена родителей из всех 4 источников
// и классифицирует: mine = корень нашей интеграционной учётки (есть прямые дети),
// others = всё прочее. Питает дропдаун «Родительский аккаунт» на /accounts.
// @Router /api/unified/accounts/parents [get]
func GetUnifiedAccountParents(c *gin.Context) {
	all := make([]UnifiedAccount, 0, 4096)

	if tenantDB := middleware.GetTenantDB(c); tenantDB != nil {
		items, _, _, _, _ := fetchAxentaAccountsForUnified(tenantDB, "", "", "", "", false, true)
		all = append(all, items...)
	}
	mineSet := make(map[string]bool)
	othersSet := make(map[string]bool)

	if companyID, ok := c.Get("company_id"); ok {
		cid := companyID.(uint)
		w, _, _, _, _, _, _ := fetchWialonAccountsForUnified(cid, "", "", "", "all", "", false)
		all = append(all, w...)
		s, _, _ := fetchSkifAccountsForUnified(cid, "", "", "", "", false)
		all = append(all, s...)
		// GELIOS — отдельным DISTINCT-запросом, минуя account-fetch с Limit(5000):
		// при большом дереве лимит мог бы молча обрезать часть родителей дропдауна.
		collectGeliosParents(cid, mineSet, othersSet)
	}

	for _, ua := range all {
		label := accountParentLabel(ua)
		if label == "" {
			continue
		}
		if ua.IsMine {
			mineSet[label] = true
		} else {
			othersSet[label] = true
		}
	}

	mine := make([]string, 0, len(mineSet))
	for k := range mineSet {
		mine = append(mine, k)
	}
	others := make([]string, 0, len(othersSet))
	for k := range othersSet {
		if !mineSet[k] { // имя могло встретиться в обеих ролях — приоритет «наши»
			others = append(others, k)
		}
	}
	ciLess := func(s []string) func(i, j int) bool {
		return func(i, j int) bool { return strings.ToLower(s[i]) < strings.ToLower(s[j]) }
	}
	sort.Slice(mine, ciLess(mine))
	sort.Slice(others, ciLess(others))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   UnifiedParentsResponse{Mine: mine, Others: others},
	})
}

// collectGeliosParents — lightweight DISTINCT creator_login для дропдауна /parents,
// без Limit(5000)/Count/Order из account-fetch. Классифицирует: creator == имя
// подключения → mine (наш token-owner), иначе others. Семантика аккаунта (is_admin
// OR units>0) сохраняется, чтобы список родителей совпадал с тем, что реально в таблице.
func collectGeliosParents(companyID uint, mineSet, othersSet map[string]bool) {
	if database.DB == nil {
		return
	}
	var connections []models.GeliosConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).
		Find(&connections).Error; err != nil || len(connections) == 0 {
		return
	}
	connIDs := make([]uint, 0, len(connections))
	connNameSet := make(map[string]bool, len(connections))
	for _, cn := range connections {
		connIDs = append(connIDs, cn.ID)
		connNameSet[strings.ToLower(strings.TrimSpace(cn.Name))] = true
	}
	var logins []string
	if err := database.DB.Model(&models.GeliosUser{}).
		Where("connection_id IN ? AND gelios_deleted_at IS NULL AND creator_login <> ''", connIDs).
		Where("is_admin = ? OR units_count > 0", true).
		Distinct().Pluck("creator_login", &logins).Error; err != nil {
		log.Printf("⚠️ /parents gelios distinct: %v", err)
		return
	}
	for _, login := range logins {
		if connNameSet[strings.ToLower(strings.TrimSpace(login))] {
			mineSet[login] = true
		} else {
			othersSet[login] = true
		}
	}
}

// RegisterUnifiedAccountsRoutes регистрирует /api/unified/accounts.
func RegisterUnifiedAccountsRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/unified/accounts", GetUnifiedAccounts)
	apiGroup.GET("/unified/accounts/", GetUnifiedAccounts)
	apiGroup.GET("/unified/accounts/parents", GetUnifiedAccountParents)
	apiGroup.GET("/unified/accounts/parents/", GetUnifiedAccountParents)
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

// fetchSkifAccountsForUnified — учётки SKIF = дилерские компании.
//
// Primary source: skif_company_statuses (kepа billing-статуса от SkifSyncScheduler).
// Fallback: GROUP BY skif_units если таблица statuses пуста (cold start, до первого sync).
//
// Возвращает: items, total, active. Для KPI карточки SKIF на /accounts.
func fetchSkifAccountsForUnified(companyID uint, search, accountType, activeStr, parent string, mineOnly bool) ([]UnifiedAccount, int, int) {
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

	// Created-at кеш из skif_object_created (object_type=company)
	createdAtMap := make(map[string]time.Time)
	var createdRows []models.SkifObjectCreated
	if err := database.DB.
		Joins("JOIN skif_connections sc ON sc.id = skif_object_created.connection_id").
		Where("sc.company_id = ? AND skif_object_created.object_type = ?", companyID, "company").
		Find(&createdRows).Error; err == nil {
		for _, r := range createdRows {
			if !r.CreatedSkifAt.IsZero() {
				createdAtMap[r.ObjectID] = r.CreatedSkifAt
			}
		}
	}

	// Statuses map: skif_company_id → CompanyStatus (ACTIVE/BLOCKED/...) + owning dealer.
	type statusInfo struct {
		Status            string
		TerminalBlockType string
		DealerID          string
	}
	statusMap := make(map[string]statusInfo)
	var statuses []models.SkifCompanyStatus
	if err := database.DB.
		Joins("JOIN skif_connections sc ON sc.id = skif_company_statuses.connection_id").
		Where("sc.company_id = ?", companyID).
		Find(&statuses).Error; err == nil {
		for _, st := range statuses {
			statusMap[st.SkifCompanyID] = statusInfo{Status: st.CompanyStatus, TerminalBlockType: st.TerminalBlockType, DealerID: st.SkifDealerID}
		}
	}

	// Субдилеры (skif_dealers) — наши прямые дилеры под интегратором (parent = наш интегратор).
	// Загружаем заранее: используем для (1) классификации компаний (компания под субдилером =
	// внук, НЕ mine; её parent = имя субдилера) и (2) вывода самих субдилеров как mine.
	var dealers []models.SkifDealer
	if err := database.DB.
		Joins("JOIN skif_connections sc ON sc.id = skif_dealers.connection_id").
		Where("sc.company_id = ? AND skif_dealers.hidden = ?", companyID, false).
		Find(&dealers).Error; err != nil {
		log.Printf("⚠️ unified/accounts skif dealers: %v", err)
	}
	subdealerNameByID := make(map[string]string, len(dealers))
	for _, d := range dealers {
		subdealerNameByID[d.SkifDealerID] = d.Name
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
	// COALESCE: skif_created_at часто NULL (SKIF API не всегда отдаёт states[0].date_from)
	// → fallback на created_at (когда юнит впервые попал в наш snapshot).
	q := `
		SELECT
			u.skif_company_id                                                  AS skif_company_id,
			MAX(u.skif_company)                                                AS name,
			MAX(u.connection_id)                                               AS connection_id,
			MAX(c.name)                                                        AS connection_name,
			COUNT(*)                                                           AS objects_total,
			COUNT(*) FILTER (WHERE u.is_active AND u.skif_deleted_at IS NULL)  AS objects_active,
			COALESCE(MIN(u.skif_created_at), MIN(u.created_at))                AS created_at
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
		// Активность определяется реальным billing.company_status из SKIF
		// (sync через auth_admin_query → skif_company_statuses).
		// Если status в кэше отсутствует (первый запуск до sync) — считаем активной.
		isActive := true
		if st, ok := statusMap[r.SkifCompanyID]; ok && st.Status != "" {
			isActive = st.Status == "ACTIVE"
		}
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
		// Родитель компании = её владелец-дилер. Если dealer ∈ субдилеры → компания
		// внук (parent = имя субдилера, НЕ mine). Иначе компания прямо под нашим
		// интегратором (parent = имя подключения, mine).
		dealerID := statusMap[r.SkifCompanyID].DealerID
		subName, underSubdealer := subdealerNameByID[dealerID]
		parentLabel := r.ConnectionName
		if underSubdealer {
			parentLabel = subName
		}
		isMineCompany := !underSubdealer
		if parent != "" && parentLabel != parent {
			continue
		}
		if mineOnly && !isMineCompany {
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
			ParentAccountName: parentLabel,
			ObjectsTotal:      r.ObjectsTotal,
			ObjectsActive:     r.ObjectsActive,
			IsActive:          isActive,
			Source:            "skif",
			SourceLabel:       "SKIF",
			ConnectionID:      &connID,
			SkifCompanyID:     r.SkifCompanyID,
			IsMine:            isMineCompany,
		}
		// Приоритет: skif_object_created (точная дата из /updates/query)
		// > skif_units.skif_created_at > skif_units.created_at (наш sync).
		if exact, ok := createdAtMap[r.SkifCompanyID]; ok && !exact.IsZero() {
			ua.CreationDatetime = exact.Format(time.RFC3339)
		} else if r.CreatedAt != nil && !r.CreatedAt.IsZero() {
			ua.CreationDatetime = r.CreatedAt.Format(time.RFC3339)
		}
		if scheduledFor, ok := pendingMap[r.SkifCompanyID]; ok && !scheduledFor.IsZero() {
			s := scheduledFor.Format(time.RFC3339)
			ua.DeleteScheduledFor = &s
		}
		items = append(items, ua)
	}

	// Subdealers (type=partner) — наши прямые дилеры под интегратором → mine.
	// Показываем если accountType не "client" (для type=client — только компании).
	if accountType != "client" {
		{
			for _, d := range dealers {
				dealerActive := !d.Blocked
				switch activeStr {
				case "true", "1":
					if !dealerActive {
						continue
					}
				case "false", "0":
					if dealerActive {
						continue
					}
				}
				if pattern != "" && !strings.Contains(strings.ToLower(d.Name), pattern) {
					continue
				}
				if parent != "" && d.ParentName != parent {
					continue
				}
				total++
				if dealerActive {
					active++
				}
				connID := d.ConnectionID
				ua := UnifiedAccount{
					ID:                int(connID)*1000000 + hashSkifID(d.SkifDealerID),
					Name:              d.Name,
					Type:              "partner",
					ParentAccountName: d.ParentName,
					ObjectsTotal:      d.UnitsCount,
					ObjectsActive:     d.UnitsCount, // SKIF subdealer не имеет breakdown по активным юнитам
					IsActive:          dealerActive,
					Source:            "skif",
					SourceLabel:       "SKIF",
					ConnectionID:      &connID,
					SkifCompanyID:     d.SkifDealerID, // synthetic — для UI идентификации
					DealerRights:      true,
					IsMine:            true, // субдилер = прямой ребёнок нашего интегратора
					CreationDatetime:  d.CreatedAt.Format(time.RFC3339),
				}
				items = append(items, ua)
			}
		}
	}

	return items, total, active
}

// fetchGeliosAccountsForUnified — учётки GELIOS = узлы дерева users,
// которые являются биллинг-сущностью: is_admin (дилер/админ-узел) ИЛИ
// units_count>0 (владеет объектами = клиентский аккаунт). Биллинг живёт
// на юзере. type: is_admin→partner, иначе client. active = !is_block.
// parent = creator_login. ObjectsTotal/Active = units_count (GELIOS
// per-active разбивку в списке не отдаёт → total≈active при !is_block).
func fetchGeliosAccountsForUnified(companyID uint, search, accountType, activeStr, parent string, mineOnly bool) ([]UnifiedAccount, int, int, int, int) {
	if database.DB == nil {
		return nil, 0, 0, 0, 0
	}
	var connections []models.GeliosConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).
		Find(&connections).Error; err != nil {
		log.Printf("⚠️ fetchGeliosAccountsForUnified: connections: %v", err)
		return nil, 0, 0, 0, 0
	}
	if len(connections) == 0 {
		return nil, 0, 0, 0, 0
	}
	connByID := make(map[uint]string, len(connections))
	connIDs := make([]uint, 0, len(connections))
	connNameSet := make(map[string]bool, len(connections)) // lowercased имена подключений для scope=mine
	for _, cn := range connections {
		connByID[cn.ID] = cn.Name
		connIDs = append(connIDs, cn.ID)
		connNameSet[strings.ToLower(strings.TrimSpace(cn.Name))] = true
	}

	// Аккаунт = биллинг-узел: дилер/админ ИЛИ владелец объектов.
	q := database.DB.Model(&models.GeliosUser{}).
		Where("connection_id IN ? AND gelios_deleted_at IS NULL", connIDs).
		Where("is_admin = ? OR units_count > 0", true)

	if search != "" {
		terms := splitSearchTerms(search)
		pattern := func(t string) string { return "%" + strings.ToLower(t) + "%" }
		if len(terms) > 1 {
			ph := make([]string, 0, len(terms)*3)
			args := make([]any, 0, len(terms)*3)
			for _, t := range terms {
				p := pattern(t)
				ph = append(ph, "LOWER(login) LIKE ?", "LOWER(email) LIKE ?", "LOWER(legal_name) LIKE ?")
				args = append(args, p, p, p)
			}
			q = q.Where(strings.Join(ph, " OR "), args...)
		} else {
			p := pattern(search)
			q = q.Where("LOWER(login) LIKE ? OR LOWER(email) LIKE ? OR LOWER(legal_name) LIKE ?", p, p, p)
		}
	}
	switch accountType {
	case "partner", "Партнёр", "Партнер":
		q = q.Where("is_admin = ?", true)
	case "client", "Клиент":
		q = q.Where("is_admin = ?", false)
	}
	if activeStr == "true" || activeStr == "1" {
		q = q.Where("is_block = ?", false)
	} else if activeStr == "false" || activeStr == "0" {
		q = q.Where("is_block = ?", true)
	}
	// COLLATE "und-x-icu": PG с lc_ctype=C не понижает кириллицу через LOWER()
	// (ГАРАЖ24 → ГАРАЖ24), поэтому имена подключений-кириллицей не матчатся.
	// ICU case-folding (как в search) даёт корректный lower без миграции схемы.
	if parent != "" {
		q = q.Where(`LOWER(creator_login COLLATE "und-x-icu") = ?`, strings.ToLower(parent))
	}
	// scope=mine: прямые дети = те, чей creator = token-owner (= имя подключения).
	if mineOnly {
		if len(connNameSet) == 0 {
			return nil, 0, 0, 0, 0
		}
		names := make([]string, 0, len(connNameSet))
		for n := range connNameSet {
			names = append(names, n)
		}
		q = q.Where(`LOWER(creator_login COLLATE "und-x-icu") IN ?`, names)
	}

	var totalCount int64
	if err := q.Count(&totalCount).Error; err != nil {
		log.Printf("⚠️ fetchGeliosAccountsForUnified count: %v", err)
		return nil, 0, 0, 0, 0
	}
	var activeCount int64
	q.Session(&gorm.Session{}).Where("is_block = ?", false).Count(&activeCount)

	var rows []models.GeliosUser
	if err := q.Order("login ASC").Limit(5000).Find(&rows).Error; err != nil {
		log.Printf("⚠️ fetchGeliosAccountsForUnified find: %v", err)
		return nil, 0, 0, 0, 0
	}

	items := make([]UnifiedAccount, 0, len(rows))
	clients, partners := 0, 0
	for _, r := range rows {
		typ := "client"
		if r.IsAdmin {
			typ = "partner"
			partners++
		} else {
			clients++
		}
		name := r.Login
		if r.LegalName != "" {
			name = r.LegalName
		}
		connName := connByID[r.ConnectionID]
		sourceLabel := "GELIOS"
		if connName != "" {
			sourceLabel = "GELIOS(" + connName + ")"
		}
		active := !r.IsBlock
		objActive := 0
		if active {
			objActive = r.UnitsCount
		}
		creationDT := ""
		if r.GeliosCreatedAt != nil {
			creationDT = r.GeliosCreatedAt.Format(time.RFC3339)
		}
		connID := r.ConnectionID
		items = append(items, UnifiedAccount{
			ID:                int(r.ID),
			Name:              name,
			Type:              typ,
			ObjectsTotal:      r.UnitsCount,
			ObjectsActive:     objActive,
			IsActive:          active,
			CreationDatetime:  creationDT,
			ParentAccountName: r.CreatorLogin,
			Hierarchy:         r.CreatorLogin,
			IsMine:            connNameSet[strings.ToLower(strings.TrimSpace(r.CreatorLogin))],
			Source:            "gelios",
			SourceLabel:       sourceLabel,
			ConnectionID:      &connID,
			DealerRights:      r.IsAdmin,
		})
	}
	return items, int(totalCount), int(activeCount), clients, partners
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
