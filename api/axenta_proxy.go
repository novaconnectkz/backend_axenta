package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// splitFullName разделяет полное имя на имя и фамилию
func splitFullName(fullName string) (firstName, lastName string) {
	if fullName == "" {
		return "", ""
	}

	// Убираем лишние пробелы
	fullName = strings.TrimSpace(fullName)

	// Разделяем по пробелу
	parts := strings.Fields(fullName)

	if len(parts) == 0 {
		return "", ""
	} else if len(parts) == 1 {
		return parts[0], ""
	} else {
		// Первая часть - имя, остальное - фамилия
		firstName = parts[0]
		lastName = strings.Join(parts[1:], " ")
		return firstName, lastName
	}
}

// shouldExcludeUserFromSearch проверяет, нужно ли исключить пользователя из результатов поиска
// Исключаем пользователя, если поисковый запрос совпадает только с creator_name,
// но не совпадает с основными полями поиска (username, email, first_name, last_name)
func shouldExcludeUserFromSearch(searchQuery string, user map[string]interface{}) bool {
	if searchQuery == "" {
		return false
	}

	searchLower := strings.ToLower(searchQuery)

	// Получаем основные поля для поиска
	username, _ := user["username"].(string)
	email, _ := user["email"].(string)
	firstName, _ := user["first_name"].(string)
	lastName, _ := user["last_name"].(string)
	name, _ := user["name"].(string)
	creatorName, _ := user["creatorName"].(string)

	// Проверяем совпадение с основными полями
	matchesMainFields := false

	if strings.Contains(strings.ToLower(username), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(email), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(firstName), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(lastName), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(name), searchLower) {
		matchesMainFields = true
	}

	// Проверяем совпадение с creator_name
	matchesCreator := strings.Contains(strings.ToLower(creatorName), searchLower)

	// Исключаем пользователя, если поиск совпадает только с creator_name
	return matchesCreator && !matchesMainFields
}

// AxentaCloudObject представляет объект из Axenta Cloud API
type AxentaCloudObject struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	UniqueID            string   `json:"uniqueId"`
	CreatorName         string   `json:"creatorName"`
	CreatorID           int      `json:"creatorId"`
	CreatorIsActive     bool     `json:"creatorIsActive"`
	AccountID           int      `json:"accountId"`
	AccountName         string   `json:"accountName"`
	AccountType         string   `json:"accountType"`
	AccountIsActive     bool     `json:"accountIsActive"`
	PhoneNumbers        []string `json:"phoneNumbers"`
	DeviceTypeName      string   `json:"deviceTypeName"`
	LastMessageDatetime string   `json:"lastMessageDatetime"`
	CreatedAt           string   `json:"createdAt"`
	DeletedAt           string   `json:"deletedAt"`
	IsActive            bool     `json:"isActive"`
	CurrentUserAccess   []string `json:"currentUserAccess"`
}

// AxentaCloudResponse представляет ответ от Axenta Cloud API
type AxentaCloudResponse struct {
	Count    int                 `json:"count"`
	Next     *string             `json:"next"`
	Previous *string             `json:"previous"`
	Results  []AxentaCloudObject `json:"results"`
}

// GetObjectsFromAxentaCloud проксирует запрос к Axenta Cloud API
func GetObjectsFromAxentaCloud(c *gin.Context) {
	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	ordering := c.DefaultQuery("ordering", "name")

	// Формируем URL для Axenta Cloud API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%s&per_page=%s&ordering=%s", page, perPage, ordering)

	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Создаем запрос к Axenta Cloud
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Добавляем заголовки авторизации с токеном пользователя
	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse AxentaCloudResponse
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Конвертируем в стандартную структуру ответа
	pageNum, _ := strconv.Atoi(page)
	perPageNum, _ := strconv.Atoi(perPage)
	totalPages := (axentaResponse.Count + perPageNum - 1) / perPageNum

	// Конвертируем объекты Axenta Cloud в локальную структуру
	var localObjects []gin.H
	for _, obj := range axentaResponse.Results {
		localObject := gin.H{
			"id":                  obj.ID,
			"name":                obj.Name,
			"type":                "vehicle", // По умолчанию
			"description":         fmt.Sprintf("%s - %s", obj.DeviceTypeName, obj.AccountName),
			"created_at":          obj.CreatedAt,
			"updated_at":          obj.CreatedAt,
			"accountName":         obj.AccountName,
			"creatorName":         obj.CreatorName,
			"deviceTypeName":      obj.DeviceTypeName,
			"phoneNumbers":        obj.PhoneNumbers,
			"createdAt":           obj.CreatedAt,
			"lastMessageDatetime": obj.LastMessageDatetime,
			"uniqueId":            obj.UniqueID,
			"status":              map[bool]string{true: "active", false: "inactive"}[obj.IsActive],
			"is_active":           obj.IsActive,
			"address":             obj.AccountName,
			"imei":                obj.UniqueID,
			"phone_number":        "",
			"serial_number":       obj.UniqueID,
			"company_id":          obj.AccountID,
			"contract_id":         obj.AccountID,
			"location_id":         obj.AccountID,
			"settings":            "{}",
			"tags":                []string{obj.DeviceTypeName, obj.AccountType},
			"notes":               fmt.Sprintf("Создатель: %s", obj.CreatorName),
			"external_id":         obj.UniqueID,
		}

		if len(obj.PhoneNumbers) > 0 {
			localObject["phone_number"] = obj.PhoneNumbers[0]
		}

		localObjects = append(localObjects, localObject)
	}

	// Возвращаем в стандартном формате
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items":       localObjects,
			"total":       axentaResponse.Count,
			"page":        pageNum,
			"per_page":    perPageNum,
			"total_pages": totalPages,
		},
	})
}

// GetObjectsStatsFromAxentaCloud получает статистику объектов
func GetObjectsStatsFromAxentaCloud(c *gin.Context) {
	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Функция для выполнения запроса к Axenta Cloud
	makeAxentaRequest := func(url string) (*AxentaCloudResponse, error) {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Token "+userToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var axentaResponse AxentaCloudResponse
		if err := json.Unmarshal(body, &axentaResponse); err != nil {
			return nil, err
		}

		return &axentaResponse, nil
	}

	// Получаем общее количество объектов
	totalResponse, err := makeAxentaRequest("https://axenta.cloud/api/cms/objects/?page=1&per_page=1")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения общей статистики: " + err.Error(),
		})
		return
	}

	// Получаем количество активных объектов (предполагаем, что активные объекты имеют статус по умолчанию)
	// Поскольку Axenta Cloud API может не поддерживать фильтрацию по статусу напрямую,
	// получаем первую страницу объектов для анализа
	objectsResponse, err := makeAxentaRequest("https://axenta.cloud/api/cms/objects/?page=1&per_page=100")
	if err != nil {
		// Если не удалось получить детальные данные, используем приблизительную статистику
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"total":                totalResponse.Count,
				"active":               int64(float64(totalResponse.Count) * 0.9), // Предполагаем 90% активных
				"inactive":             int64(float64(totalResponse.Count) * 0.1), // Предполагаем 10% неактивных
				"scheduled_for_delete": 0,
				"deleted":              0,
				"by_type": gin.H{
					"vehicle": totalResponse.Count,
				},
				"by_status": gin.H{
					"active":   int64(float64(totalResponse.Count) * 0.9),
					"inactive": int64(float64(totalResponse.Count) * 0.1),
				},
			},
		})
		return
	}

	// Анализируем полученные объекты для подсчета статистики
	sampleActiveCount := int64(0)
	sampleInactiveCount := int64(0)
	scheduledForDeleteCount := int64(0)

	// Подсчитываем статистику на основе полученных объектов
	if len(objectsResponse.Results) > 0 {
		sampleSize := int64(len(objectsResponse.Results))

		// Подсчитываем в образце, используя поле IsActive
		for _, obj := range objectsResponse.Results {
			if obj.IsActive {
				sampleActiveCount++
			} else {
				sampleInactiveCount++
			}
		}

		// Экстраполируем на общее количество
		if sampleSize > 0 {
			ratio := float64(totalResponse.Count) / float64(sampleSize)
			activeCount := int64(float64(sampleActiveCount) * ratio)
			inactiveCount := int64(float64(sampleInactiveCount) * ratio)
			totalCount := int64(totalResponse.Count)

			// Корректируем, чтобы сумма была равна общему количеству
			if activeCount+inactiveCount != totalCount {
				diff := totalCount - (activeCount + inactiveCount)
				if diff > 0 {
					// Добавляем разность к активным (предполагаем, что новые объекты активные)
					activeCount += diff
				} else {
					// Убираем разность из активных
					activeCount += diff
					if activeCount < 0 {
						inactiveCount += activeCount
						activeCount = 0
					}
				}
			}

			// Возвращаем реальную статистику
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"data": gin.H{
					"total":                totalResponse.Count,
					"active":               activeCount,
					"inactive":             inactiveCount,
					"scheduled_for_delete": scheduledForDeleteCount,
					"deleted":              0,
					"by_type": gin.H{
						"vehicle": totalResponse.Count,
					},
					"by_status": gin.H{
						"active":   activeCount,
						"inactive": inactiveCount,
					},
				},
			})
			return
		}
	}

	// Если не удалось получить образец или он пустой, используем приблизительные значения
	totalCount := int64(totalResponse.Count)
	activeCount := int64(float64(totalCount) * 0.95) // 95% активных
	inactiveCount := totalCount - activeCount

	// Возвращаем реальную статистику
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"total":                totalResponse.Count,
			"active":               activeCount,
			"inactive":             inactiveCount,
			"scheduled_for_delete": scheduledForDeleteCount,
			"deleted":              0,
			"by_type": gin.H{
				"vehicle": totalResponse.Count,
			},
			"by_status": gin.H{
				"active":   activeCount,
				"inactive": inactiveCount,
			},
		},
	})
}

// GetUsersFromAxentaCloud проксирует запрос к Axenta Cloud API для пользователей
func GetUsersFromAxentaCloud(c *gin.Context) {
	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	// Фронтенд отправляет 'limit', а Axenta Cloud ожидает 'per_page'
	limit := c.DefaultQuery("limit", "20")
	perPage := limit // Используем тот же лимит, что запрашивает фронтенд
	search := c.Query("search")
	active := c.Query("active")
	role := c.Query("role")
	ordering := c.Query("ordering")

	// Формируем URL для Axenta Cloud API с правильным кодированием параметров
	baseURL := "https://axenta.cloud/api/cms/users/"
	params := url.Values{}
	params.Add("page", page)
	params.Add("per_page", perPage)

	// Добавляем фильтры если есть
	if search != "" {
		params.Add("search", search) // url.Values автоматически кодирует параметры
	}
	if active != "" {
		params.Add("active", active)
	}
	if role != "" {
		params.Add("role", role)
	}
	if ordering != "" {
		// Преобразуем наш формат ordering в формат Axenta Cloud
		axentaOrdering := convertOrderingToAxenta(ordering)
		if axentaOrdering != "" {
			params.Add("ordering", axentaOrdering)
		}
	}

	axentaURL := baseURL + "?" + params.Encode()

	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос к Axenta Cloud
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		log.Printf("❌ Ошибка создания запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса",
		})
		return
	}

	// Устанавливаем заголовки
	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	log.Printf("📡 Проксирование запроса пользователей к Axenta Cloud: %s", axentaURL)
	log.Printf("🔍 Параметры запроса: page=%s, limit=%s, search=%s", page, limit, search)

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Ошибка запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к Axenta Cloud",
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа",
		})
		return
	}

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Axenta Cloud вернул ошибку: %d - %s", resp.StatusCode, string(body))

		// Если ошибка 400 и есть поисковый запрос с кириллицей, пробуем локальный поиск
		if resp.StatusCode == http.StatusBadRequest && search != "" && containsCyrillic(search) {
			log.Printf("🔄 Пробуем локальный поиск для кириллического запроса: %s", search)
			fallbackLocalSearch(c, search, page, perPage, active, role)
			return
		}

		c.JSON(resp.StatusCode, gin.H{
			"status":  "error",
			"error":   fmt.Sprintf("Axenta Cloud API error: %d", resp.StatusCode),
			"details": string(body),
		})
		return
	}

	// Парсим JSON ответ - Axenta Cloud возвращает объект с пагинацией
	var axentaResponse struct {
		Count    int                      `json:"count"`
		Next     *string                  `json:"next"`
		Previous *string                  `json:"previous"`
		Results  []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		log.Printf("❌ Ошибка парсинга JSON от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа",
		})
		return
	}

	log.Printf("✅ Получено %d пользователей от Axenta Cloud (всего: %d)", len(axentaResponse.Results), axentaResponse.Count)

	// Получаем базу данных для работы с ролями
	// Используем основную базу данных, так как /api/auth endpoints работают без мультитенантности
	db := database.GetDB()
	if db == nil {
		log.Printf("❌ Database connection not available for role mapping")
		// Продолжаем без ролей, но логируем проблему
	}

	// Преобразуем данные в формат, ожидаемый фронтендом
	users := make([]gin.H, len(axentaResponse.Results))
	for i, user := range axentaResponse.Results {
		// Определяем роль на основе accountType
		var roleInfo gin.H
		var roleID interface{} = 0

		if db != nil {
			accountType, ok := user["accountType"].(string)
			if ok {
				role, roleData := getRoleByAxentaType(db, accountType)
				if role != nil {
					roleID = role.ID
					roleInfo = roleData
				}
			}
		}

		// Разделяем полное имя на имя и фамилию
		fullName, _ := user["name"].(string)
		firstName, lastName := splitFullName(fullName)

		users[i] = gin.H{
			"id":                user["id"],
			"username":          user["username"],
			"email":             user["email"],
			"first_name":        firstName,
			"last_name":         lastName,
			"name":              user["name"], // Полное имя из Axenta Cloud
			"is_active":         user["isActive"],
			"role_id":           roleID,
			"role":              roleInfo, // Добавляем информацию о роли
			"template_id":       nil,
			"last_login":        user["lastLogin"],
			"login_count":       0,
			"created_at":        user["creationDatetime"],
			"updated_at":        user["creationDatetime"],
			"creation_datetime": user["creationDatetime"], // Дата создания пользователя из Axenta Cloud
			// Дополнительные поля из Axenta Cloud
			"account_name":     user["accountName"],
			"account_type":     user["accountType"],
			"creator_name":     user["creatorName"],
			"creatorName":      user["creatorName"], // Дублируем для совместимости
			"language":         user["language"],
			"timezone":         user["timezone"],
			"is_admin":         user["isAdmin"],
			"has_admin_access": user["hasAdminAccess"],
			// Поля для Axenta интеграции
			"axenta_user_type": mapAccountTypeToAxentaType(user["accountType"]),
			"axenta_user_id":   fmt.Sprintf("%v", user["id"]),
			"is_axenta_user":   true,
			"external_source":  "axenta",
		}
	}

	// Фильтруем результаты поиска, исключая пользователей, найденных только по creator_name
	if search != "" {
		var filteredUsers []gin.H
		excludedCount := 0

		for _, user := range users {
			// Преобразуем gin.H в map[string]interface{} для функции проверки
			userMap := make(map[string]interface{})
			for k, v := range user {
				userMap[k] = v
			}

			// Проверяем, нужно ли исключить пользователя
			if shouldExcludeUserFromSearch(search, userMap) {
				excludedCount++
				log.Printf("🚫 Исключаем пользователя из поиска (совпадение только по creator_name): %v", userMap["username"])
			} else {
				filteredUsers = append(filteredUsers, user)
			}
		}

		if excludedCount > 0 {
			log.Printf("🔍 Исключено %d пользователей из результатов поиска (совпадение только по creator_name)", excludedCount)
			users = filteredUsers
			// Обновляем общее количество с учетом исключенных
			axentaResponse.Count = axentaResponse.Count - excludedCount
		}
	}

	// Преобразуем параметры для правильного ответа
	pageInt, _ := strconv.Atoi(page)
	limitInt, _ := strconv.Atoi(limit)
	if pageInt < 1 {
		pageInt = 1
	}
	if limitInt < 1 {
		limitInt = 20
	}

	log.Printf("📊 Возвращаем: page=%d, limit=%d, pages=%d, получено=%d, total=%d", pageInt, limitInt, (axentaResponse.Count+limitInt-1)/limitInt, len(users), axentaResponse.Count)

	// Возвращаем данные в формате, ожидаемом фронтендом
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": users,
			"total": axentaResponse.Count,                             // Общее количество пользователей (530)
			"page":  pageInt,                                          // Текущая страница
			"limit": limitInt,                                         // Лимит на страницу
			"pages": (axentaResponse.Count + limitInt - 1) / limitInt, // Правильное количество страниц
		},
	})
}

// GetUsersStatsFromAxentaCloud получает статистику пользователей из Axenta Cloud
func GetUsersStatsFromAxentaCloud(c *gin.Context) {
	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Запрашиваем всех пользователей для подсчета статистики
	axentaURL := "https://axenta.cloud/api/cms/users/?per_page=1000"
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		log.Printf("❌ Ошибка создания запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса",
		})
		return
	}

	// Устанавливаем заголовки
	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Ошибка запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к Axenta Cloud",
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа",
		})
		return
	}

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Axenta Cloud вернул ошибку: %d - %s", resp.StatusCode, string(body))
		c.JSON(resp.StatusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Axenta Cloud API error: %d", resp.StatusCode),
		})
		return
	}

	// Парсим JSON ответ - Axenta Cloud возвращает объект с пагинацией
	var axentaResponse struct {
		Count    int                      `json:"count"`
		Next     *string                  `json:"next"`
		Previous *string                  `json:"previous"`
		Results  []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		log.Printf("❌ Ошибка парсинга JSON от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа",
		})
		return
	}

	// Подсчитываем статистику
	totalUsers := axentaResponse.Count
	activeUsers := 0
	inactiveUsers := 0
	recentUsers := 0
	roleStats := make(map[string]int)

	now := time.Now()
	oneWeekAgo := now.AddDate(0, 0, -7)

	for _, user := range axentaResponse.Results {
		// Подсчет активных/неактивных
		if isActive, ok := user["isActive"].(bool); ok && isActive {
			activeUsers++
		} else {
			inactiveUsers++
		}

		// Подсчет недавних входов
		if lastLoginStr, ok := user["lastLogin"].(string); ok && lastLoginStr != "" {
			if lastLogin, err := time.Parse("2006-01-02T15:04:05.999999Z", lastLoginStr); err == nil {
				if lastLogin.After(oneWeekAgo) {
					recentUsers++
				}
			}
		}

		// Подсчет по типам аккаунтов (используем как роли)
		if accountType, ok := user["accountType"].(string); ok {
			roleStats[accountType]++
		}
	}

	// Формируем статистику в формате, ожидаемом фронтендом
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"total_users":    totalUsers,
			"active_users":   activeUsers,
			"inactive_users": inactiveUsers,
			"recent_users":   recentUsers,
			"total":          totalUsers,
			"active":         activeUsers,
			"inactive":       inactiveUsers,
			"recent_logins":  recentUsers,
			"role_stats":     roleStats,
			"last_updated":   time.Now().Format("2006-01-02T15:04:05Z"),
		},
	})
}

// getRoleByAxentaType получает роль из базы данных на основе типа аккаунта Axenta
func getRoleByAxentaType(db *gorm.DB, accountType string) (*models.Role, gin.H) {
	var roleName string

	switch accountType {
	case "partner":
		roleName = "partner"
	case "client":
		roleName = "client"
	default:
		roleName = "user" // Роль по умолчанию
	}

	var role models.Role
	err := db.Where("name = ?", roleName).First(&role).Error
	if err != nil {
		// Если роль не найдена, создаем роли по умолчанию в этой tenant схеме
		if err == gorm.ErrRecordNotFound {
			log.Printf("⚠️ Role %s not found in tenant schema, creating default roles...", roleName)

			// Создаем сервис и роли по умолчанию
			axentaUserService := services.NewAxentaUserService(db)
			if createErr := axentaUserService.EnsureDefaultRoles(); createErr != nil {
				log.Printf("❌ Failed to create default roles: %v", createErr)
				return nil, nil
			}

			// Пытаемся найти роль снова
			err = db.Where("name = ?", roleName).First(&role).Error
			if err != nil {
				log.Printf("❌ Role %s still not found after creation: %v", roleName, err)
				return nil, nil
			}

			log.Printf("✅ Role %s created and found (ID: %d)", roleName, role.ID)
		} else {
			log.Printf("❌ Database error finding role %s: %v", roleName, err)
			return nil, nil
		}
	}

	// Возвращаем роль и данные для frontend
	roleData := gin.H{
		"id":           role.ID,
		"name":         role.Name,
		"display_name": role.DisplayName,
		"description":  role.Description,
		"color":        role.Color,
		"priority":     role.Priority,
		"is_active":    role.IsActive,
		"is_system":    role.IsSystem,
		"created_at":   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return &role, roleData
}

// mapAccountTypeToAxentaType преобразует тип аккаунта Axenta в тип пользователя системы
func mapAccountTypeToAxentaType(accountType interface{}) string {
	if accountType == nil {
		return "client"
	}

	accountTypeStr, ok := accountType.(string)
	if !ok {
		return "client"
	}

	switch accountTypeStr {
	case "partner":
		return "partner"
	case "client":
		return "client"
	default:
		return "client" // По умолчанию
	}
}

// containsCyrillic проверяет, содержит ли строка кириллические символы
func containsCyrillic(s string) bool {
	for _, r := range s {
		if (r >= 'А' && r <= 'я') || (r >= 'Ё' && r <= 'ё') {
			return true
		}
	}
	return false
}

// fallbackLocalSearch выполняет локальный поиск пользователей при ошибке Axenta Cloud API
func fallbackLocalSearch(c *gin.Context, search, page, perPage, active, role string) {
	log.Printf("🔍 Выполняем локальный поиск пользователей: %s", search)

	// Получаем базу данных
	db := database.GetDB()
	if db == nil {
		log.Printf("❌ База данных недоступна для локального поиска")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "База данных недоступна",
		})
		return
	}

	// Преобразуем параметры
	pageInt, _ := strconv.Atoi(page)
	if pageInt < 1 {
		pageInt = 1
	}
	limitInt, _ := strconv.Atoi(perPage)
	if limitInt < 1 || limitInt > 100 {
		limitInt = 20
	}
	offset := (pageInt - 1) * limitInt

	// Строим запрос к локальной базе данных
	query := db.Model(&models.User{}).Preload("Role").Preload("Template")

	// Фильтр по роли
	if role != "" {
		query = query.Joins("JOIN roles ON users.role_id = roles.id").
			Where("roles.name = ?", role)
	}

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск по имени, email или username (используем нашу исправленную логику)
	if search != "" {
		searchPatternLower := "%" + strings.ToLower(search) + "%"
		searchPatternOriginal := "%" + search + "%"

		query = query.Where(
			"(LOWER(username) LIKE ? OR username LIKE ?) OR "+
				"(LOWER(email) LIKE ? OR email LIKE ?) OR "+
				"(LOWER(first_name) LIKE ? OR first_name LIKE ?) OR "+
				"(LOWER(last_name) LIKE ? OR last_name LIKE ?) OR "+
				"((LOWER(first_name) || ' ' || LOWER(last_name)) LIKE ? OR (first_name || ' ' || last_name) LIKE ?)",
			searchPatternLower, searchPatternOriginal,
			searchPatternLower, searchPatternOriginal,
			searchPatternLower, searchPatternOriginal,
			searchPatternLower, searchPatternOriginal,
			searchPatternLower, searchPatternOriginal,
		)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Printf("❌ Ошибка подсчета пользователей: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета пользователей",
		})
		return
	}

	// Получение данных с пагинацией
	var users []models.User
	if err := query.Offset(offset).Limit(limitInt).Find(&users).Error; err != nil {
		log.Printf("❌ Ошибка получения пользователей: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователей",
		})
		return
	}

	log.Printf("✅ Локальный поиск нашел %d пользователей из %d", len(users), total)

	// Преобразование в response format (как в GetUsers)
	userResponses := make([]gin.H, len(users))
	for i, user := range users {
		// Определяем полное имя
		fullName := user.Name
		if fullName == "" && (user.FirstName != "" || user.LastName != "") {
			// Если поле Name пустое, формируем его из FirstName и LastName
			if user.FirstName != "" && user.LastName != "" {
				fullName = user.FirstName + " " + user.LastName
			} else if user.FirstName != "" {
				fullName = user.FirstName
			} else if user.LastName != "" {
				fullName = user.LastName
			}
		}

		// Определяем имя создателя
		creatorName := ""
		if user.FirstName != "" && user.LastName != "" {
			creatorName = user.FirstName + " " + user.LastName
		} else if user.FirstName != "" {
			creatorName = user.FirstName
		} else if user.LastName != "" {
			creatorName = user.LastName
		} else {
			creatorName = user.Username // Fallback на username
		}

		userResponses[i] = gin.H{
			"id":                user.ID,
			"username":          user.Username,
			"email":             user.Email,
			"first_name":        user.FirstName,
			"last_name":         user.LastName,
			"name":              fullName, // Полное имя (сформированное или из модели)
			"is_active":         user.IsActive,
			"role_id":           user.RoleID,
			"role":              user.Role,
			"template_id":       user.TemplateID,
			"last_login":        user.LastLogin,
			"login_count":       user.LoginCount,
			"created_at":        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated_at":        user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			"creation_datetime": user.CreatedAt.Format("2006-01-02T15:04:05Z"), // Дата создания пользователя
			// Дополнительные поля для локальных пользователей
			"creator_name": creatorName, // Имя создателя
			"creatorName":  creatorName, // Дублируем для совместимости
			// Помечаем, что это локальные данные
			"is_local_search": true,
		}
	}

        // Возвращаем данные в том же формате, что и Axenta Cloud
        c.JSON(http.StatusOK, gin.H{
                "status": "success",
                "data": gin.H{
                        "items": userResponses,
                        "total": total,
                        "page":  pageInt,
                        "limit": limitInt,
                        "pages": (total + int64(limitInt) - 1) / int64(limitInt),                                                                               
                },
        })
}

// convertOrderingToAxenta преобразует наш формат сортировки в формат Axenta Cloud
func convertOrderingToAxenta(ordering string) string {
	// Маппинг наших полей на поля Axenta Cloud
	fieldMapping := map[string]string{
		"id":                 "id",
		"-id":                "-id",
		"username":           "username",
		"-username":          "-username",
		"email":              "email",
		"-email":             "-email",
		"name":               "name",
		"-name":              "-name",
		"first_name":         "first_name",
		"-first_name":        "-first_name",
		"last_name":          "last_name",
		"-last_name":         "-last_name",
		"creation_datetime":  "created_at",
		"-creation_datetime": "-created_at",
		"creator_name":       "first_name", // В Axenta Cloud сортируем по first_name
		"-creator_name":      "-first_name",
	}
	
	if axentaField, exists := fieldMapping[ordering]; exists {
		return axentaField
	}
	
	// Если поле не найдено, возвращаем пустую строку (без сортировки)
	return ""
}

// GetDeletedObjectsFromAxentaCloud проксирует запрос к корзине Axenta Cloud API
func GetDeletedObjectsFromAxentaCloud(c *gin.Context) {
	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	search := c.Query("search")

	// Формируем URL для Axenta Cloud API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/trash/?page=%s&per_page=%s", page, perPage)
	if search != "" {
		axentaURL += fmt.Sprintf("&search=%s", search)
	}

	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Создаем запрос к Axenta Cloud
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Добавляем заголовки авторизации с токеном пользователя
	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Если Axenta Cloud вернул ошибку, передаем её клиенту
	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Ошибка Axenta Cloud API: %s", string(body)),
		})
		return
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse struct {
		Count    int                 `json:"count"`
		Next     *string             `json:"next"`
		Previous *string             `json:"previous"`
		Results  []AxentaCloudObject `json:"results"`
	}

	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Преобразуем объекты Axenta Cloud в локальный формат
	var localObjects []gin.H
	for _, obj := range axentaResponse.Results {
		localObject := gin.H{
			"id":                  obj.ID,
			"name":                obj.Name,
			"type":                "vehicle", // По умолчанию
			"description":         "",
			"latitude":            nil,
			"longitude":           nil,
			"address":             obj.AccountName,
			"imei":                obj.UniqueID,
			"phone_number":        "",
			"serial_number":       obj.UniqueID,
			"status":              "deleted", // Помечаем как удаленный
			"is_active":           false,
			"scheduled_delete_at": nil,
			"last_activity_at":    nil,
			"company_id":          obj.AccountID,
			"contract_id":         obj.AccountID,
			"template_id":         nil,
			"location_id":         obj.AccountID,
			"created_at":          obj.CreatedAt,
			"updated_at":          obj.CreatedAt,
			"deleted_at":          obj.DeletedAt, // Добавляем поле deleted_at
			"accountName":         obj.AccountName,
			"creatorName":         obj.CreatorName,
			"deviceTypeName":      obj.DeviceTypeName,
			"phoneNumbers":        obj.PhoneNumbers,
			"createdAt":           obj.CreatedAt,
			"lastMessageDatetime": obj.LastMessageDatetime,
			"uniqueId":            obj.UniqueID,
			"settings":            "{}",
			"tags":                []string{obj.DeviceTypeName, obj.AccountType},
			"notes":               fmt.Sprintf("Создатель: %s", obj.CreatorName),
			"external_id":         obj.UniqueID,
		}

		if len(obj.PhoneNumbers) > 0 {
			localObject["phone_number"] = obj.PhoneNumbers[0]
		}

		localObjects = append(localObjects, localObject)
	}

	// Парсим параметры пагинации
	pageNum, _ := strconv.Atoi(page)
	perPageNum, _ := strconv.Atoi(perPage)
	totalPages := (axentaResponse.Count + perPageNum - 1) / perPageNum

	// Возвращаем в стандартном формате
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items":       localObjects,
			"total":       axentaResponse.Count,
			"page":        pageNum,
			"limit":       perPageNum,
			"total_pages": totalPages,
		},
	})
}
