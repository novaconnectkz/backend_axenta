package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// UnifiedUser представляет единую структуру пользователя из любого источника
type UnifiedUser struct {
	ID               int64  `json:"id"`
	Username         string `json:"username"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	IsActive         bool   `json:"is_active"`
	CreationDatetime string `json:"creation_datetime,omitempty"`
	CreatorName      string `json:"creator_name,omitempty"`
	Source           string `json:"source"` // "axenta" или "wialon"
	SourceLabel      string `json:"source_label,omitempty"`
	Hierarchy        string `json:"hierarchy,omitempty"`
	ConnectionID     *uint  `json:"connection_id,omitempty"`
	AccountType      string `json:"account_type,omitempty"`
	DealerRights     bool   `json:"dealer_rights,omitempty"`
	ObjectsTotal     int    `json:"objects_total,omitempty"`
	ObjectsActive    int    `json:"objects_active,omitempty"`
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
	AxentaTotal  int `json:"axenta_total"`
	AxentaActive int `json:"axenta_active"`
	WialonTotal  int `json:"wialon_total"`
	WialonActive int `json:"wialon_active"`
}

// GetUnifiedUsers возвращает пользователей из всех источников (Axenta + Wialon)
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
	// Параметры запроса
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

	// Получаем токен из заголовка
	authHeader := c.GetHeader("Authorization")
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Получаем company_id для Wialon
	companyID, _ := c.Get("company_id")

	// Инициализируем пустой слайс, чтобы избежать null в JSON ответе
	allUsers := make([]UnifiedUser, 0)
	var stats UnifiedUsersStats
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Параллельная загрузка из обоих источников
	// source может быть: all, axenta, wialon, wh (Wialon Hosting), wl (Wialon Local)
	loadAxenta := source == "all" || source == "axenta"
	loadWialon := source == "all" || source == "wialon" || source == "wh" || source == "wl"

	if loadAxenta {
		wg.Add(1)
		go func() {
			defer wg.Done()
			axentaUsers, axentaTotal, axentaActive := fetchAxentaUsers(userToken, search, activeStr, role, ordering)
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
				// Передаём тип подключения для фильтрации (wh или wl)
				wialonUsers, wialonTotal, wialonActive := fetchWialonUsersFiltered(companyID.(uint), search, activeStr, source)
				mu.Lock()
				allUsers = append(allUsers, wialonUsers...)
				stats.WialonTotal = wialonTotal
				stats.WialonActive = wialonActive
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Общий счётчик
	total := len(allUsers)

	// Пагинация на объединённых данных
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

// fetchAxentaUsers загружает пользователей из Axenta Cloud
func fetchAxentaUsers(userToken, search, active, role, ordering string) ([]UnifiedUser, int, int) {
	var users []UnifiedUser
	var totalUsers, activeUsers int

	if userToken == "" {
		log.Printf("⚠️ fetchAxentaUsers: токен не предоставлен")
		return users, 0, 0
	}

	// Загружаем все страницы для получения полного списка (для фильтрации и статистики)
	// Но ограничиваем до 1000 пользователей для производительности
	baseURL := "https://axenta.cloud/api/cms/users/"
	params := url.Values{}
	params.Add("page", "1")
	params.Add("per_page", "1000") // Загружаем максимум

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

	// Парсим ответ
	// ВАЖНО: API Axenta Cloud использует camelCase для полей
	var axentaResp struct {
		Count   int `json:"count"`
		Results []struct {
			ID               int64  `json:"id"`
			Username         string `json:"username"`
			Name             string `json:"name"`       // Полное имя пользователя
			FirstName        string `json:"first_name"` // Fallback
			LastName         string `json:"last_name"`  // Fallback
			Email            string `json:"email"`
			AccountType      string `json:"accountType"`      // camelCase в API
			IsActive         bool   `json:"isActive"`         // camelCase в API
			CreationDatetime string `json:"creationDatetime"` // camelCase в API
			CreatorName      string `json:"creatorName"`      // camelCase в API
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &axentaResp); err != nil {
		log.Printf("❌ fetchAxentaUsers: ошибка парсинга: %v", err)
		return users, 0, 0
	}

	totalUsers = axentaResp.Count

	for _, u := range axentaResp.Results {
		// Приоритет: Name > FirstName + LastName > Username
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

	log.Printf("✅ fetchAxentaUsers: загружено %d пользователей (активных: %d)", len(users), activeUsers)
	return users, totalUsers, activeUsers
}

// fetchWialonUsersFiltered загружает пользователей из Wialon с поддержкой фильтрации по типу подключения
// sourceFilter: "all" | "wialon" - все подключения, "wh" - только Hosting, "wl" - только Local
func fetchWialonUsersFiltered(companyID uint, search, activeStr, sourceFilter string) ([]UnifiedUser, int, int) {
	// Инициализируем пустой слайс, чтобы избежать null в JSON ответе
	users := make([]UnifiedUser, 0)
	var totalUsers, activeUsers int

	// Получаем все активные подключения Wialon для компании
	var connections []models.WialonConnection
	if err := database.DB.Where("company_id = ? AND is_active = ?", companyID, true).Find(&connections).Error; err != nil {
		log.Printf("❌ fetchWialonUsersFiltered: ошибка получения подключений: %v", err)
		return users, 0, 0
	}

	wialonService := services.NewWialonService()

	for _, conn := range connections {
		// Фильтрация по типу подключения
		if sourceFilter == "wh" && conn.ConnectionType != models.WialonConnectionTypeHosting {
			continue
		}
		if sourceFilter == "wl" && conn.ConnectionType != models.WialonConnectionTypeLocal {
			continue
		}

		// Формируем метку источника
		sourceLabel := ""
		if conn.ConnectionType == models.WialonConnectionTypeHosting {
			sourceLabel = "WH(" + conn.UserName + ")"
		} else {
			sourceLabel = "WL(" + conn.UserName + ")"
		}

		// Получаем аккаунты из подключения
		accounts, err := wialonService.GetAccountsQuickFromHost(conn.Host, conn.Token)
		if err != nil {
			log.Printf("⚠️ Ошибка получения аккаунтов из %s: %v", conn.Name, err)
			continue
		}

		for _, acc := range accounts {
			// Фильтрация по поиску
			if search != "" {
				searchLower := strings.ToLower(search)
				nameLower := strings.ToLower(acc.Name)
				if !strings.Contains(nameLower, searchLower) {
					continue
				}
			}

			// Фильтрация по активности
			if activeStr != "" {
				isActiveFilter := activeStr == "true" || activeStr == "1"
				if acc.IsActive != isActiveFilter {
					continue
				}
			}

			// Формируем иерархию
			var hierarchy string
			if acc.ParentName != "" {
				hierarchy = sourceLabel + " > " + acc.ParentName + " > " + acc.Name
			} else {
				hierarchy = sourceLabel + " > " + acc.Name
			}

			// Определяем тип
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
				CreationDatetime: acc.CreatedAt,  // Дата создания из Wialon (ct)
				CreatorName:      acc.ParentName, // Создатель = родительская учётная запись (crt)
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

	log.Printf("✅ fetchWialonUsersFiltered (filter=%s): загружено %d пользователей (активных: %d)", sourceFilter, len(users), activeUsers)
	return users, totalUsers, activeUsers
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
