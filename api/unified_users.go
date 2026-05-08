package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// UnifiedUser представляет единую структуру пользователя из любого источника.
//
// Унификация с UnifiedObject (camelCase): помимо исторических snake_case полей
// дублируются camelCase алиасы для creator/source/connection/account/etc.
// Frontend читает в первую очередь camelCase, snake_case остаётся для backward
// compatibility — будет удалён через 1-2 итерации.
type UnifiedUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`

	// Дата создания. snake_case + camelCase.
	CreationDatetime      string `json:"creation_datetime,omitempty"`
	CreationDatetimeAlias string `json:"creationDatetime,omitempty"`

	// Creator. snake_case + camelCase.
	CreatorName      string `json:"creator_name,omitempty"`
	CreatorNameAlias string `json:"creatorName,omitempty"`

	// Источник.
	Source           string `json:"source"`
	SourceLabel      string `json:"source_label,omitempty"`
	SourceLabelAlias string `json:"sourceLabel,omitempty"`

	Hierarchy string `json:"hierarchy,omitempty"`

	// ConnectionID. snake_case + camelCase.
	ConnectionID      *uint `json:"connection_id,omitempty"`
	ConnectionIDAlias *uint `json:"connectionId,omitempty"`

	// Account-привязка (для UI cross-section: User → Objects этого аккаунта).
	// Заполняется по AdminAccountID (Axenta) или BillingAccount (Wialon).
	AccountID        int64  `json:"accountId,omitempty"`
	AccountName      string `json:"accountName,omitempty"`

	AccountType      string `json:"account_type,omitempty"`
	AccountTypeAlias string `json:"accountType,omitempty"`

	DealerRights      bool `json:"dealer_rights,omitempty"`
	DealerRightsAlias bool `json:"dealerRights,omitempty"`

	ObjectsTotal       int `json:"objects_total,omitempty"`
	ObjectsTotalAlias  int `json:"objectsTotal,omitempty"`
	ObjectsActive      int `json:"objects_active,omitempty"`
	ObjectsActiveAlias int `json:"objectsActive,omitempty"`

	Phone string `json:"phone,omitempty"`

	TelegramID      string `json:"telegram_id,omitempty"`
	TelegramIDAlias string `json:"telegramId,omitempty"`

	LastLogin      string `json:"last_login,omitempty"`
	LastLoginAlias string `json:"lastLogin,omitempty"`

	// ExternalID — реальный ID в источнике для UUID-систем (SKIF). Для Axenta/Wialon
	// совпадает с ID, для SKIF — UUID (skif_user_id), пока ID хранит local PK skif_users.id
	// для уникальности int64 в merged-set.
	ExternalID      string `json:"external_id,omitempty"`
	ExternalIDAlias string `json:"externalId,omitempty"`

	// SkifCompanyID — UUID компании в SKIF (нужен FE для PUT/DELETE в правильную company).
	SkifCompanyID      string `json:"skif_company_id,omitempty"`
	SkifCompanyIDAlias string `json:"skifCompanyId,omitempty"`
}

// fillUnifiedUserAliases копирует snake_case поля в camelCase-алиасы.
// Вызывается ПОСЛЕ заполнения основных полей. Гарантирует что обе формы
// json-выхода всегда синхронизированы — UI получает одинаковое значение
// независимо от того какой вариант он читает.
func fillUnifiedUserAliases(u *UnifiedUser) {
	u.CreationDatetimeAlias = u.CreationDatetime
	u.CreatorNameAlias = u.CreatorName
	u.SourceLabelAlias = u.SourceLabel
	u.ConnectionIDAlias = u.ConnectionID
	u.AccountTypeAlias = u.AccountType
	u.DealerRightsAlias = u.DealerRights
	u.ObjectsTotalAlias = u.ObjectsTotal
	u.ObjectsActiveAlias = u.ObjectsActive
	u.TelegramIDAlias = u.TelegramID
	u.LastLoginAlias = u.LastLogin
	u.ExternalIDAlias = u.ExternalID
	u.SkifCompanyIDAlias = u.SkifCompanyID
}

// UnifiedUsersResponse структура ответа для унифицированного API
type UnifiedUsersResponse struct {
	Items      []UnifiedUser     `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
	Stats      UnifiedUsersStats `json:"stats"`
}

// UnifiedUsersStats статистика пользователей
type UnifiedUsersStats struct {
	AxentaTotal    int `json:"axenta_total"`
	AxentaActive   int `json:"axenta_active"`
	WialonTotal    int `json:"wialon_total"`
	WialonActive   int `json:"wialon_active"`
	WialonWHTotal  int `json:"wialon_wh_total"`
	WialonWHActive int `json:"wialon_wh_active"`
	WialonWLTotal  int `json:"wialon_wl_total"`
	WialonWLActive int `json:"wialon_wl_active"`
	SkifTotal      int `json:"skif_total"`
	SkifActive     int `json:"skif_active"`
}

// unifiedUsersSnapshotTTL — окно свежести snapshot Axenta для read-path.
// Использует общий SnapshotTTL (60 мин), см. snapshot_ttl.go.
const unifiedUsersSnapshotTTL = SnapshotTTL

// GetUnifiedUsers возвращает пользователей из всех источников (Axenta + Wialon)
// Read-path: snapshot для Axenta (TTL 60м) + Redis для Wialon (через wialon:all-accounts).
// Fallback: live-fetch если snapshot/cache пустые/устаревшие.
//
// @Summary Получить унифицированный список пользователей
// @Tags Unified Users
// @Produce json
// @Param page query int false "Номер страницы" default(1)
// @Param limit query int false "Количество записей на странице" default(20)
// @Param search query string false "Поисковый запрос"
// @Param source query string false "Источник: axenta, wialon, all" default(all)
// @Param active query bool false "Фильтр по активности"
// @Param role query string false "Фильтр по роли"
// @Success 200 {object} UnifiedUsersResponse
// @Router /api/unified/users [get]
func GetUnifiedUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := strings.TrimSpace(c.Query("search"))
	source := strings.ToLower(c.DefaultQuery("source", "all"))
	activeStr := c.Query("active")
	role := c.Query("role")
	ordering := c.Query("ordering")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	authHeader := c.GetHeader("Authorization")
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	companyID, _ := c.Get("company_id")

	allUsers := make([]UnifiedUser, 0)
	var stats UnifiedUsersStats
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
			axentaUsers, axentaTotal, axentaActive, fromSnapshot := fetchAxentaUsersFast(tenantDB, userToken, search, activeStr, role, ordering)
			log.Printf("🔍 unified/users axenta: %d users (snapshot=%v) за %s", len(axentaUsers), fromSnapshot, time.Since(t0).Round(time.Millisecond))
			mu.Lock()
			allUsers = append(allUsers, axentaUsers...)
			stats.AxentaTotal = axentaTotal
			stats.AxentaActive = axentaActive
			mu.Unlock()
		}()
	}

	if loadWialon {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if companyID != nil {
				t0 := time.Now()
				wialonUsers, wTotal, wActive, whTotal, whActive, wlTotal, wlActive, fromCache := fetchWialonUsersFast(companyID.(uint), search, activeStr, source, role)
				log.Printf("🔍 unified/users wialon: %d users (cache=%v) за %s", len(wialonUsers), fromCache, time.Since(t0).Round(time.Millisecond))
				mu.Lock()
				allUsers = append(allUsers, wialonUsers...)
				stats.WialonTotal = wTotal
				stats.WialonActive = wActive
				stats.WialonWHTotal = whTotal
				stats.WialonWHActive = whActive
				stats.WialonWLTotal = wlTotal
				stats.WialonWLActive = wlActive
				mu.Unlock()
			}
		}()
	}

	if loadSkif {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if companyID != nil {
				t0 := time.Now()
				skifUsers, sTotal, sActive := fetchSkifUsersFast(companyID.(uint), search, activeStr, role)
				log.Printf("🔍 unified/users skif: %d users за %s", len(skifUsers), time.Since(t0).Round(time.Millisecond))
				mu.Lock()
				allUsers = append(allUsers, skifUsers...)
				stats.SkifTotal = sTotal
				stats.SkifActive = sActive
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Серверная сортировка по ordering (если задан)
	sortUnifiedUsers(allUsers, ordering)

	total := len(allUsers)

	startIndex := (page - 1) * limit
	endIndex := startIndex + limit
	if startIndex > total {
		startIndex = total
	}
	if endIndex > total {
		endIndex = total
	}

	paginatedUsers := allUsers[startIndex:endIndex]
	totalPages := (total + limit - 1) / limit

	// Заполняем camelCase алиасы (унификация с UnifiedObject DTO).
	// Делаем для пагинированной страницы — лишних копий не нужно.
	for i := range paginatedUsers {
		fillUnifiedUserAliases(&paginatedUsers[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": UnifiedUsersResponse{
			Items:      paginatedUsers,
			Total:      total,
			Page:       page,
			PerPage:    limit,
			TotalPages: totalPages,
			Stats:      stats,
		},
	})
}

// fetchAxentaUsersFast — read-path с snapshot, fallback на live при пустом/устаревшем snapshot.
// Возвращает (users, total, active, fromSnapshot).
func fetchAxentaUsersFast(db *gorm.DB, userToken, search, active, role, ordering string) ([]UnifiedUser, int, int, bool) {
	if users, total, activeN, ok := tryServeUnifiedUsersFromSnapshot(db, search, active, role); ok {
		return users, total, activeN, true
	}
	users, total, activeN := fetchAxentaUsers(userToken, search, active, role, ordering)
	return users, total, activeN, false
}

// tryServeUnifiedUsersFromSnapshot читает пользователей Axenta из snapshot (axenta_user_snapshots).
// TTL 60 мин — fallback на live при устаревании. Фильтры применяются на стороне БД.
func tryServeUnifiedUsersFromSnapshot(db *gorm.DB, search, active, role string) ([]UnifiedUser, int, int, bool) {
	if db == nil {
		return nil, 0, 0, false
	}

	var lastSync time.Time
	if err := db.
		Model(&models.AxentaUserSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, 0, 0, false
	}

	if time.Since(lastSync) > unifiedUsersSnapshotTTL {
		log.Printf("⏰ AxentaUserSnapshot устарел (last=%v), fallback на live", lastSync)
		return nil, 0, 0, false
	}

	q := db.Model(&models.AxentaUserSnapshot{})

	// Множественный поиск через запятую (как для accounts): "user1, user2" → IN
	if search != "" {
		terms := splitSearchTerms(search)
		if len(terms) > 1 {
			// Множественный поиск — точное совпадение (с любым из перечисленных)
			lowered := make([]string, 0, len(terms)*3)
			placeholders := make([]string, 0, len(terms)*3)
			for _, t := range terms {
				p := "%" + strings.ToLower(t) + "%"
				lowered = append(lowered, p, p, p)
				placeholders = append(placeholders, "LOWER(username) LIKE ?", "LOWER(name) LIKE ?", "LOWER(email) LIKE ?")
			}
			args := make([]any, 0, len(lowered))
			for _, l := range lowered {
				args = append(args, l)
			}
			q = q.Where(strings.Join(placeholders, " OR "), args...)
		} else {
			pattern := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(username) LIKE ? OR LOWER(name) LIKE ? OR LOWER(email) LIKE ?", pattern, pattern, pattern)
		}
	}

	if active != "" {
		switch active {
		case "true", "1":
			q = q.Where("is_active = ?", true)
		case "false", "0":
			q = q.Where("is_active = ?", false)
		}
	}

	if role != "" {
		// role в API Axenta = display name ("Партнёр"/"Клиент") — мапим обратно на account_type
		switch role {
		case "Партнёр", "Партнер", "partner":
			q = q.Where("account_type = ?", "partner")
		case "Клиент", "client":
			q = q.Where("account_type = ?", "client")
		case "Администратор", "staff":
			q = q.Where("account_type = ?", "staff")
		default:
			q = q.Where("account_type = ?", role)
		}
	}

	// Считаем total/active отдельным запросом до применения LIMIT
	var totalCount int64
	if err := q.Count(&totalCount).Error; err != nil {
		log.Printf("⚠️ AxentaUserSnapshot count: %v", err)
		return nil, 0, 0, false
	}

	var activeCount int64
	q.Session(&gorm.Session{}).Where("is_active = ?", true).Count(&activeCount)

	// Загружаем все matched-rows (без пагинации — пагинация делается в caller для merged set)
	var rows []models.AxentaUserSnapshot
	if err := q.Order("creation_datetime DESC").Find(&rows).Error; err != nil {
		log.Printf("⚠️ AxentaUserSnapshot find: %v", err)
		return nil, 0, 0, false
	}

	users := make([]UnifiedUser, 0, len(rows))
	for _, r := range rows {
		users = append(users, UnifiedUser{
			ID:               r.ExternalUserID,
			Username:         r.Username,
			Name:             r.Name,
			Email:            r.Email,
			Role:             mapAxentaTypeToRole(r.AccountType),
			IsActive:         r.IsActive,
			CreationDatetime: r.CreationDatetime,
			CreatorName:      r.CreatorName,
			Source:           "axenta",
			SourceLabel:      "Axenta Cloud",
			AccountType:      r.AccountType,
			AccountID:        int64(r.AdminAccountID),
		})
	}

	return users, int(totalCount), int(activeCount), true
}

// tryServeUsersStatsFromSnapshot — read-path для GET /api/auth/users/stats.
// Один COUNT FILTER (...) запрос к snapshot вместо live-fetch 2.6с с per_page=1000.
// Возвращает (data, true) если snapshot непуст и свежий.
func tryServeUsersStatsFromSnapshot(db *gorm.DB) (gin.H, bool) {
	if db == nil {
		return nil, false
	}

	var lastSync time.Time
	if err := db.
		Model(&models.AxentaUserSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return nil, false
	}

	if time.Since(lastSync) > unifiedUsersSnapshotTTL {
		return nil, false
	}

	type counts struct {
		Total      int64
		Active     int64
		Inactive   int64
		Recent     int64
		PartnerCnt int64
		ClientCnt  int64
		StaffCnt   int64
	}
	weekAgo := time.Now().Add(-7 * 24 * time.Hour)
	var c counts
	if err := db.
		Model(&models.AxentaUserSnapshot{}).
		Select(`
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE is_active) AS active,
			COUNT(*) FILTER (WHERE NOT is_active) AS inactive,
			COUNT(*) FILTER (WHERE last_login IS NOT NULL AND last_login > ?) AS recent,
			COUNT(*) FILTER (WHERE account_type = 'partner') AS partner_cnt,
			COUNT(*) FILTER (WHERE account_type = 'client') AS client_cnt,
			COUNT(*) FILTER (WHERE account_type = 'staff') AS staff_cnt
		`, weekAgo).
		Scan(&c).Error; err != nil {
		log.Printf("⚠️ tryServeUsersStatsFromSnapshot count: %v", err)
		return nil, false
	}

	roleStats := gin.H{
		"partner": c.PartnerCnt,
		"client":  c.ClientCnt,
	}
	if c.StaffCnt > 0 {
		roleStats["staff"] = c.StaffCnt
	}

	return gin.H{
		"total_users":    c.Total,
		"active_users":   c.Active,
		"inactive_users": c.Inactive,
		"recent_users":   c.Recent,
		"total":          c.Total,
		"active":         c.Active,
		"inactive":       c.Inactive,
		"recent_logins":  c.Recent,
		"role_stats":     roleStats,
		"last_updated":   lastSync.Format("2006-01-02T15:04:05Z"),
		"from_snapshot":  true,
	}, true
}

// splitSearchTerms — разделяет search-строку через запятую, тримит, отбрасывает пустые
func splitSearchTerms(search string) []string {
	if !strings.Contains(search, ",") {
		return []string{search}
	}
	parts := strings.Split(search, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// fetchWialonUsersFast — read-path через Redis cache wialon:all-accounts:<cid>.
// Cache наполняется WialonAllAccountsScheduler @5m. Если cache пуст — fallback на live (старая реализация).
// Возвращает: users, total, active, whTotal, whActive, wlTotal, wlActive, fromCache.
func fetchWialonUsersFast(companyID uint, search, activeStr, sourceFilter, roleFilter string) ([]UnifiedUser, int, int, int, int, int, int, bool) {
	if database.RedisClient == nil {
		users, total, active := fetchWialonUsersFiltered(companyID, search, activeStr, sourceFilter)
		return users, total, active, 0, 0, 0, 0, false
	}

	cached, err := database.RedisClient.Get(context.Background(), allAccountsCacheKey(companyID)).Bytes()
	if err != nil || len(cached) == 0 {
		users, total, active := fetchWialonUsersFiltered(companyID, search, activeStr, sourceFilter)
		return users, total, active, 0, 0, 0, 0, false
	}

	// Маппинг роли: UI присылает display_name ("Партнёр"/"Клиент") или английский "partner"/"client"
	wantPartner := false
	wantClient := false
	switch roleFilter {
	case "Партнёр", "Партнер", "partner":
		wantPartner = true
	case "Клиент", "client":
		wantClient = true
	}

	// Парсим Redis-payload (тот же формат что отдаёт /wialon/all-accounts)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Items []WialonAccountInfo `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(cached, &resp); err != nil {
		log.Printf("⚠️ unified/users wialon cache parse: %v", err)
		users, total, active := fetchWialonUsersFiltered(companyID, search, activeStr, sourceFilter)
		return users, total, active, 0, 0, 0, 0, false
	}

	users := make([]UnifiedUser, 0, len(resp.Data.Items))
	totalUsers, activeUsers := 0, 0
	whTotal, whActive, wlTotal, wlActive := 0, 0, 0, 0
	bumpBreakdown := func(label string, isActive bool) {
		if strings.HasPrefix(label, "WH(") {
			whTotal++
			if isActive {
				whActive++
			}
		} else if strings.HasPrefix(label, "WL(") {
			wlTotal++
			if isActive {
				wlActive++
			}
		}
	}

	// Для определения connection_id по source_label нужны connections — подтянем 1 раз
	var connections []models.WialonConnection
	_ = database.DB.Where("company_id = ? AND is_active = ?", companyID, true).Find(&connections).Error
	connByName := make(map[string]uint)
	for _, c := range connections {
		connByName["WH("+c.UserName+")"] = c.ID
		connByName["WL("+c.UserName+")"] = c.ID
	}

	for _, acc := range resp.Data.Items {
		// Фильтрация по типу подключения
		if sourceFilter == "wh" && !strings.HasPrefix(acc.SourceLabel, "WH(") {
			continue
		}
		if sourceFilter == "wl" && !strings.HasPrefix(acc.SourceLabel, "WL(") {
			continue
		}

		// Фильтр по поиску. Учитываем sub-users: если parent.Name не matches,
		// но матчится хотя бы один sub — оставляем acc, parent отдельно скипнем ниже.
		parentMatched := true
		if search != "" {
			terms := splitSearchTerms(search)
			parentMatched = false
			nameLower := strings.ToLower(acc.Name)
			for _, t := range terms {
				if strings.Contains(nameLower, strings.ToLower(t)) {
					parentMatched = true
					break
				}
			}
			if !parentMatched {
				subMatched := false
				for _, su := range acc.SubUsers {
					suLower := strings.ToLower(su.Name)
					for _, t := range terms {
						if strings.Contains(suLower, strings.ToLower(t)) {
							subMatched = true
							break
						}
					}
					if subMatched {
						break
					}
				}
				if !subMatched {
					continue
				}
			}
		}

		// Фильтр активности
		if activeStr != "" {
			isActiveFilter := activeStr == "true" || activeStr == "1"
			if acc.IsActive != isActiveFilter {
				continue
			}
		}

		// Фильтр по роли (для wialon: dealer_rights = partner, иначе client)
		if wantPartner && !acc.DealerRights {
			continue
		}
		if wantClient && acc.DealerRights {
			continue
		}

		// hierarchy — берём из cache как есть, parent → CreatorName
		var creatorName string
		if acc.Hierarchy != "" {
			parts := strings.Split(acc.Hierarchy, " > ")
			if len(parts) >= 2 {
				creatorName = parts[len(parts)-2]
			}
		}

		accountType := acc.Type
		if accountType == "" {
			accountType = "client"
		}

		connID := connByName[acc.SourceLabel]
		var connIDPtr *uint
		if connID != 0 {
			connIDPtr = &connID
		}

		if parentMatched {
			users = append(users, UnifiedUser{
				ID:               int64(acc.ID),
				Username:         acc.Name,
				Name:             acc.Name,
				Email:            acc.Email,
				Role:             accountType,
				IsActive:         acc.IsActive,
				CreationDatetime: acc.CreatedAt,
				CreatorName:      creatorName,
				Source:           "wialon",
				SourceLabel:      acc.SourceLabel,
				Hierarchy:        acc.Hierarchy,
				ConnectionID:     connIDPtr,
				AccountType:      accountType,
				DealerRights:     acc.DealerRights,
				ObjectsTotal:     acc.ObjectsTotal,
				ObjectsActive:    acc.ObjectsActive,
				Phone:            acc.Phone,
				TelegramID:       acc.Telegram,
				LastLogin:        acc.LastLogin,
			})

			totalUsers++
			if acc.IsActive {
				activeUsers++
			}
			bumpBreakdown(acc.SourceLabel, acc.IsActive)
		}

		// Sub-users аккаунта (вложенные юзеры без своего биллинг-ресурса).
		// Применяем те же фильтры (search/active/role): role у sub всегда "client", DealerRights=false.
		if wantPartner {
			continue
		}
		for _, su := range acc.SubUsers {
			if search != "" {
				terms := splitSearchTerms(search)
				matched := false
				nameLower := strings.ToLower(su.Name)
				for _, t := range terms {
					if strings.Contains(nameLower, strings.ToLower(t)) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if activeStr != "" {
				isActiveFilter := activeStr == "true" || activeStr == "1"
				if su.IsActive != isActiveFilter {
					continue
				}
			}

			users = append(users, UnifiedUser{
				ID:               su.ID,
				Username:         su.Name,
				Name:             su.Name,
				Email:            su.Email,
				Role:             "client",
				IsActive:         su.IsActive,
				CreationDatetime: su.CreatedAt,
				CreatorName:      acc.Name,
				Source:           "wialon",
				SourceLabel:      acc.SourceLabel,
				Hierarchy:        acc.Hierarchy + " > " + su.Name,
				ConnectionID:     connIDPtr,
				AccountType:      "client",
				DealerRights:     false,
				Phone:            su.Phone,
				TelegramID:       su.Telegram,
				LastLogin:        su.LastLogin,
			})

			totalUsers++
			if su.IsActive {
				activeUsers++
			}
			bumpBreakdown(acc.SourceLabel, su.IsActive)
		}
	}

	return users, totalUsers, activeUsers, whTotal, whActive, wlTotal, wlActive, true
}

// sortUnifiedUsers сортирует merged-set по полю ordering ("-creation_datetime", "username", etc.)
func sortUnifiedUsers(users []UnifiedUser, ordering string) {
	if ordering == "" {
		ordering = "-creation_datetime"
	}
	desc := strings.HasPrefix(ordering, "-")
	field := strings.TrimPrefix(ordering, "-")

	sort.SliceStable(users, func(i, j int) bool {
		var less bool
		switch field {
		case "id":
			less = users[i].ID < users[j].ID
		case "username":
			less = strings.ToLower(users[i].Username) < strings.ToLower(users[j].Username)
		case "email":
			less = strings.ToLower(users[i].Email) < strings.ToLower(users[j].Email)
		case "name":
			less = strings.ToLower(users[i].Name) < strings.ToLower(users[j].Name)
		case "creator_name":
			less = strings.ToLower(users[i].CreatorName) < strings.ToLower(users[j].CreatorName)
		case "creation_datetime":
			less = users[i].CreationDatetime < users[j].CreationDatetime
		case "source":
			// Сорт по source_label (axenta → "Axenta Cloud", wialon → "WH(...)" / "WL(...)")
			a := strings.ToLower(users[i].SourceLabel)
			b := strings.ToLower(users[j].SourceLabel)
			if a == b {
				less = strings.ToLower(users[i].Username) < strings.ToLower(users[j].Username)
			} else {
				less = a < b
			}
		default:
			less = users[i].CreationDatetime < users[j].CreationDatetime
		}
		if desc {
			return !less
		}
		return less
	})
}

// fetchAxentaUsers — fallback live-fetch (старая реализация). Используется если snapshot пуст.
func fetchAxentaUsers(userToken, search, active, role, ordering string) ([]UnifiedUser, int, int) {
	var users []UnifiedUser
	var totalUsers, activeUsers int

	if userToken == "" {
		log.Printf("⚠️ fetchAxentaUsers: токен не предоставлен")
		return users, 0, 0
	}

	baseURL := "https://axenta.cloud/api/cms/users/"
	params := url.Values{}
	params.Add("page", "1")
	params.Add("per_page", "1000")

	if search != "" {
		params.Add("search", search)
	}
	if active != "" {
		params.Add("active", active)
	}
	if role != "" {
		params.Add("role", role)
	}
	if ordering != "" {
		params.Add("ordering", convertOrderingToAxenta(ordering))
	}

	axentaURL := baseURL + "?" + params.Encode()

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		log.Printf("❌ fetchAxentaUsers: ошибка создания запроса: %v", err)
		return users, 0, 0
	}

	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ fetchAxentaUsers: ошибка запроса: %v", err)
		return users, 0, 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ fetchAxentaUsers: ошибка чтения ответа: %v", err)
		return users, 0, 0
	}

	var axentaResp struct {
		Count   int `json:"count"`
		Results []struct {
			ID               int64  `json:"id"`
			Username         string `json:"username"`
			Name             string `json:"name"`
			FirstName        string `json:"first_name"`
			LastName         string `json:"last_name"`
			Email            string `json:"email"`
			AccountType      string `json:"accountType"`
			IsActive         bool   `json:"isActive"`
			CreationDatetime string `json:"creationDatetime"`
			CreatorName      string `json:"creatorName"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &axentaResp); err != nil {
		log.Printf("❌ fetchAxentaUsers: ошибка парсинга: %v", err)
		return users, 0, 0
	}

	totalUsers = axentaResp.Count

	for _, u := range axentaResp.Results {
		name := strings.TrimSpace(u.Name)
		if name == "" {
			name = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
		if name == "" {
			name = u.Username
		}

		role := mapAxentaTypeToRole(u.AccountType)

		users = append(users, UnifiedUser{
			ID:               u.ID,
			Username:         u.Username,
			Name:             name,
			Email:            u.Email,
			Role:             role,
			IsActive:         u.IsActive,
			CreationDatetime: u.CreationDatetime,
			CreatorName:      u.CreatorName,
			Source:           "axenta",
			SourceLabel:      "Axenta Cloud",
			AccountType:      u.AccountType,
		})

		if u.IsActive {
			activeUsers++
		}
	}

	log.Printf("✅ fetchAxentaUsers (live fallback): %d пользователей (активных: %d)", len(users), activeUsers)
	return users, totalUsers, activeUsers
}

// fetchWialonUsersFiltered — fallback live-fetch (без cache). Используется если Redis пуст.
func fetchWialonUsersFiltered(companyID uint, search, activeStr, sourceFilter string) ([]UnifiedUser, int, int) {
	users := make([]UnifiedUser, 0)
	var totalUsers, activeUsers int

	var connections []models.WialonConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).Find(&connections).Error; err != nil {
		log.Printf("❌ fetchWialonUsersFiltered: ошибка получения подключений: %v", err)
		return users, 0, 0
	}

	wialonService := services.NewWialonService()

	for _, conn := range connections {
		if sourceFilter == "wh" && conn.ConnectionType != models.WialonConnectionTypeHosting {
			continue
		}
		if sourceFilter == "wl" && conn.ConnectionType != models.WialonConnectionTypeLocal {
			continue
		}

		sourceLabel := "WL(" + conn.UserName + ")"
		if conn.ConnectionType == models.WialonConnectionTypeHosting {
			sourceLabel = "WH(" + conn.UserName + ")"
		}

		accounts, err := wialonService.GetAccountsQuickFromHost(conn.Host, conn.Token)
		if err != nil {
			log.Printf("⚠️ Ошибка получения аккаунтов из %s: %v", conn.Name, err)
			continue
		}

		for _, acc := range accounts {
			if search != "" {
				if !strings.Contains(strings.ToLower(acc.Name), strings.ToLower(search)) {
					continue
				}
			}
			if activeStr != "" {
				isActiveFilter := activeStr == "true" || activeStr == "1"
				if acc.IsActive != isActiveFilter {
					continue
				}
			}

			var hierarchy string
			if acc.ParentName != "" {
				hierarchy = sourceLabel + " > " + acc.ParentName + " > " + acc.Name
			} else {
				hierarchy = sourceLabel + " > " + acc.Name
			}

			accountType := "client"
			if acc.DealerRights {
				accountType = "partner"
			}

			connID := conn.ID
			users = append(users, UnifiedUser{
				ID:               int64(acc.ID),
				Username:         acc.Name,
				Name:             acc.Name,
				Email:            "",
				Role:             accountType,
				IsActive:         acc.IsActive,
				CreationDatetime: acc.CreatedAt,
				CreatorName:      acc.ParentName,
				Source:           "wialon",
				SourceLabel:      sourceLabel,
				Hierarchy:        hierarchy,
				ConnectionID:     &connID,
				AccountType:      accountType,
				DealerRights:     acc.DealerRights,
				ObjectsTotal:     acc.ObjectsTotal,
				ObjectsActive:    acc.ObjectsActive,
			})

			totalUsers++
			if acc.IsActive {
				activeUsers++
			}
		}
	}

	log.Printf("✅ fetchWialonUsersFiltered live (filter=%s): %d users (active=%d)", sourceFilter, len(users), activeUsers)
	return users, totalUsers, activeUsers
}

// fetchSkifUsersFast — read-path SKIF users из БД (skif_users — сама snapshot-таблица).
// SKIF не имеет отдельного snapshot уровня: сама БД заполняется SkifSyncScheduler.
// Возвращает (users, total, active).
//
// Фильтры применяются на стороне БД: search по name/email/phone, active по is_active,
// role mapping: "Партнёр"/"partner" → SUPERVISOR, "Клиент"/"client" → остальные роли.
func fetchSkifUsersFast(companyID uint, search, activeStr, roleFilter string) ([]UnifiedUser, int, int) {
	var connections []models.SkifConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).
		Find(&connections).Error; err != nil {
		log.Printf("⚠️ fetchSkifUsersFast: загрузка connections: %v", err)
		return nil, 0, 0
	}
	if len(connections) == 0 {
		return nil, 0, 0
	}

	connByID := make(map[uint]string, len(connections))
	connIDs := make([]uint, 0, len(connections))
	for _, c := range connections {
		connByID[c.ID] = c.Name
		connIDs = append(connIDs, c.ID)
	}

	q := database.DB.Model(&models.SkifUser{}).
		Where("connection_id IN ? AND skif_deleted_at IS NULL", connIDs)

	if search != "" {
		terms := splitSearchTerms(search)
		if len(terms) > 1 {
			placeholders := make([]string, 0, len(terms)*4)
			args := make([]any, 0, len(terms)*4)
			for _, t := range terms {
				p := "%" + strings.ToLower(t) + "%"
				placeholders = append(placeholders,
					"LOWER(name) LIKE ?", "LOWER(email) LIKE ?", "LOWER(phone) LIKE ?", "LOWER(skif_company) LIKE ?")
				args = append(args, p, p, p, p)
			}
			q = q.Where(strings.Join(placeholders, " OR "), args...)
		} else {
			pattern := "%" + strings.ToLower(search) + "%"
			q = q.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(skif_company) LIKE ?",
				pattern, pattern, pattern, pattern)
		}
	}

	if activeStr != "" {
		switch activeStr {
		case "true", "1":
			q = q.Where("is_active = ?", true)
		case "false", "0":
			q = q.Where("is_active = ?", false)
		}
	}

	// Role mapping: SUPERVISOR = partner-уровень (интегратор), остальные = client.
	if roleFilter != "" {
		switch roleFilter {
		case "Партнёр", "Партнер", "partner":
			q = q.Where("role_key = ?", "SUPERVISOR")
		case "Клиент", "client":
			q = q.Where("role_key != ? AND role_key != ?", "SUPERVISOR", "")
		}
	}

	var totalCount int64
	if err := q.Count(&totalCount).Error; err != nil {
		log.Printf("⚠️ fetchSkifUsersFast count: %v", err)
		return nil, 0, 0
	}

	var activeCount int64
	q.Session(&gorm.Session{}).Where("is_active = ?", true).Count(&activeCount)

	var rows []models.SkifUser
	if err := q.Order("name ASC").Limit(5000).Find(&rows).Error; err != nil {
		log.Printf("⚠️ fetchSkifUsersFast find: %v", err)
		return nil, 0, 0
	}

	users := make([]UnifiedUser, 0, len(rows))
	for _, r := range rows {
		role := mapSkifRoleToRole(r.RoleKey)
		sourceLabel := "SKIF"
		if r.SkifCompany != "" {
			sourceLabel = "SKIF(" + r.SkifCompany + ")"
		}
		creationDT := ""
		if r.SkifCreatedAt != nil {
			creationDT = r.SkifCreatedAt.Format(time.RFC3339)
		}
		lastLogin := ""
		if r.LastLoginAt != nil {
			lastLogin = r.LastLoginAt.Format(time.RFC3339)
		}
		connID := r.ConnectionID
		users = append(users, UnifiedUser{
			ID:               int64(r.ID),
			Username:         r.Email,
			Name:             r.Name,
			Email:            r.Email,
			Role:             role,
			IsActive:         r.IsActive,
			CreationDatetime: creationDT,
			CreatorName:      r.SkifCompany,
			Source:           "skif",
			SourceLabel:      sourceLabel,
			Hierarchy:        sourceLabel,
			ConnectionID:     &connID,
			AccountType:      role,
			DealerRights:     r.RoleKey == "SUPERVISOR",
			Phone:            r.Phone,
			TelegramID:       r.TelegramChatID,
			LastLogin:        lastLogin,
			ExternalID:       r.SkifUserID,
			SkifCompanyID:    r.SkifCompanyID,
		})
	}

	return users, int(totalCount), int(activeCount)
}

// mapSkifRoleToRole — роль SKIF (EDITOR/ADMIN/READER/SUPERVISOR/NoAccess) → внутренняя роль.
// Семантика: SUPERVISOR = интегратор-уровень (partner); ADMIN/EDITOR/READER внутри company → client.
func mapSkifRoleToRole(roleKey string) string {
	switch roleKey {
	case "SUPERVISOR":
		return "partner"
	case "ADMIN", "EDITOR", "READER", "NoAccess":
		return "client"
	default:
		return "client"
	}
}

// mapAxentaTypeToRole преобразует тип аккаунта Axenta в роль (для отображения)
func mapAxentaTypeToRole(accountType string) string {
	switch accountType {
	case "staff":
		return "Администратор"
	case "partner":
		return "Партнёр"
	case "client":
		return "Клиент"
	default:
		return accountType
	}
}

// RegisterUnifiedUsersRoutes регистрирует маршруты для унифицированного API пользователей
func RegisterUnifiedUsersRoutes(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/unified/users", GetUnifiedUsers)
	apiGroup.GET("/unified/users/", GetUnifiedUsers)
	log.Println("✅ Unified Users API routes registered")
}

// Подавить unused warning на fmt при отключённом debug
var _ = fmt.Sprintf
