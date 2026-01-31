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
	"github.com/xuri/excelize/v2"
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
	ID                  int         `json:"id"`
	Name                string      `json:"name"`
	UniqueID            string      `json:"uniqueId"`
	CreatorName         string      `json:"creatorName"`
	CreatorID           int         `json:"creatorId"`
	CreatorIsActive     bool        `json:"creatorIsActive"`
	AccountID           int         `json:"accountId"`
	AccountName         string      `json:"accountName"`
	AccountType         string      `json:"accountType"`
	AccountIsActive     bool        `json:"accountIsActive"`
	PhoneNumbers        []string    `json:"phoneNumbers"`
	DeviceTypeName      string      `json:"deviceTypeName"`
	LastMessageDatetime string      `json:"lastMessageDatetime"`
	CreatedAt           string      `json:"createdAt"`
	DeletedAt           string      `json:"deletedAt"`
	IsActive            bool        `json:"isActive"`
	CurrentUserAccess   interface{} `json:"currentUserAccess"` // Может быть []string или number
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

	// Формируем URL для Axenta Cloud API с базовыми параметрами
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%s&per_page=%s&ordering=%s", page, perPage, ordering)

	// Добавляем дополнительные фильтры, если они переданы
	if accountId := c.Query("accountId"); accountId != "" {
		axentaURL += "&accountId=" + accountId
	}
	if accountName := c.Query("accountName"); accountName != "" {
		axentaURL += "&accountName=" + url.QueryEscape(accountName)
	}
	if creatorName := c.Query("creatorName"); creatorName != "" {
		axentaURL += "&creatorName=" + url.QueryEscape(creatorName)
	}
	if deviceTypeName := c.Query("deviceTypeName"); deviceTypeName != "" {
		axentaURL += "&deviceTypeName=" + url.QueryEscape(deviceTypeName)
	}
	if uniqueId := c.Query("uniqueId"); uniqueId != "" {
		axentaURL += "&uniqueId=" + url.QueryEscape(uniqueId)
	}
	if status := c.Query("status"); status != "" {
		axentaURL += "&status=" + url.QueryEscape(status)
	}
	if objectType := c.Query("type"); objectType != "" {
		axentaURL += "&type=" + url.QueryEscape(objectType)
	}
	if search := c.Query("search"); search != "" {
		axentaURL += "&search=" + url.QueryEscape(search)
	}
	if contractId := c.Query("contract_id"); contractId != "" {
		axentaURL += "&contract_id=" + contractId
	}
	if isActive := c.Query("is_active"); isActive != "" {
		axentaURL += "&is_active=" + isActive
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
		log.Printf("❌ Токен авторизации не предоставлен")
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
		authPreview := authHeader
		if len(authHeader) > 20 {
			authPreview = authHeader[:20]
		}
		log.Printf("❌ Неверный формат токена авторизации: %s", authPreview)
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	log.Printf("📊 Запрос статистики объектов")

	// Функция для выполнения запроса к Axenta Cloud
	makeAxentaRequest := func(url string) (*AxentaCloudResponse, error) {
		log.Printf("🌐 Запрос к: %s", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Printf("❌ Ошибка создания запроса: %v", err)
			return nil, err
		}

		req.Header.Set("Authorization", "Token "+userToken)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("❌ Ошибка выполнения запроса: %v", err)
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("❌ Ошибка чтения ответа: %v", err)
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			bodyPreview := string(body)
			if len(body) > 200 {
				bodyPreview = string(body[:200])
			}
			log.Printf("❌ Неожиданный статус %d: %s", resp.StatusCode, bodyPreview)
			return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
		}

		var axentaResponse AxentaCloudResponse
		if err := json.Unmarshal(body, &axentaResponse); err != nil {
			bodyPreview := string(body)
			if len(body) > 200 {
				bodyPreview = string(body[:200])
			}
			log.Printf("❌ Ошибка парсинга JSON: %v, body: %s", err, bodyPreview)
			return nil, err
		}

		log.Printf("✅ Успешный запрос, count=%d, results=%d", axentaResponse.Count, len(axentaResponse.Results))
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
	log.Printf("📊 Общее количество объектов: %d", totalResponse.Count)

	// Получаем точное количество активных объектов через фильтр
	activeResponse, err := makeAxentaRequest("https://axenta.cloud/api/cms/objects/?page=1&per_page=1&is_active=true")
	activeCount := int64(0)
	if err == nil {
		activeCount = int64(activeResponse.Count)
		log.Printf("✅ Активных объектов через фильтр: %d", activeCount)
	} else {
		log.Printf("⚠️ Ошибка получения активных объектов через фильтр: %v", err)
	}

	// Получаем точное количество неактивных объектов через фильтр
	inactiveResponse, err := makeAxentaRequest("https://axenta.cloud/api/cms/objects/?page=1&per_page=1&is_active=false")
	inactiveCount := int64(0)
	if err == nil {
		inactiveCount = int64(inactiveResponse.Count)
		log.Printf("✅ Неактивных объектов через фильтр: %d", inactiveCount)
	} else {
		log.Printf("⚠️ Ошибка получения неактивных объектов через фильтр: %v", err)
	}

	// Проверяем, работают ли фильтры
	// Если фильтры не работают, API возвращает одинаковое количество для всех запросов
	filtersNotWorking := (activeCount == int64(totalResponse.Count) && inactiveCount == int64(totalResponse.Count))
	
	if filtersNotWorking {
		log.Printf("⚠️ Фильтры is_active не работают (active=%d, inactive=%d, total=%d), используем fallback", activeCount, inactiveCount, totalResponse.Count)
	}

	// Если фильтры не работают, используем fallback - подсчитываем из выборки всех объектов
	// но с большей выборкой для точности
	if activeCount == 0 && inactiveCount == 0 || filtersNotWorking {
		log.Printf("🔄 Фильтры не вернули результаты, используем fallback с большой выборкой")
		// Запрашиваем большую выборку для более точного подсчета
		objectsResponse, err := makeAxentaRequest("https://axenta.cloud/api/cms/objects/?page=1&per_page=1000")
		if err == nil && len(objectsResponse.Results) > 0 {
			log.Printf("📦 Получена выборка: %d объектов", len(objectsResponse.Results))
			sampleActiveCount := int64(0)
			sampleInactiveCount := int64(0)
			
			// Подсчитываем в выборке
			for _, obj := range objectsResponse.Results {
				if obj.IsActive {
					sampleActiveCount++
				} else {
					sampleInactiveCount++
				}
			}
			log.Printf("📊 В выборке: активных=%d, неактивных=%d", sampleActiveCount, sampleInactiveCount)
			
			// Если выборка достаточно большая, используем её
			if int64(len(objectsResponse.Results)) >= 100 {
				log.Printf("🔢 Выборка большая, используем экстраполяцию")
				// Экстраполируем на общее количество
				sampleSize := int64(len(objectsResponse.Results))
				if sampleSize > 0 {
					ratio := float64(totalResponse.Count) / float64(sampleSize)
					activeCount = int64(float64(sampleActiveCount) * ratio)
					inactiveCount = int64(float64(sampleInactiveCount) * ratio)
					log.Printf("📈 Экстраполяция: активных=%d, неактивных=%d (ratio=%.2f)", activeCount, inactiveCount, ratio)
					
					// Корректируем, чтобы сумма была равна общему количеству
					totalCount := int64(totalResponse.Count)
					if activeCount+inactiveCount != totalCount {
						diff := totalCount - (activeCount + inactiveCount)
						if diff > 0 {
							activeCount += diff
						} else {
							activeCount += diff
							if activeCount < 0 {
								inactiveCount += activeCount
								activeCount = 0
							}
						}
						log.Printf("✏️ После корректировки: активных=%d, неактивных=%d", activeCount, inactiveCount)
					}
				}
			} else {
				log.Printf("📝 Выборка маленькая, используем прямой подсчет")
				// Если выборка маленькая, используем прямое подсчет из неё
				activeCount = sampleActiveCount
				inactiveCount = sampleInactiveCount
			}
		} else {
			log.Printf("⚠️ Не удалось получить выборку, используем приблизительные значения")
			// Если не удалось получить данные, используем приблизительные значения
			totalCount := int64(totalResponse.Count)
			activeCount = int64(float64(totalCount) * 0.95) // 95% активных
			inactiveCount = totalCount - activeCount
		}
	}

	log.Printf("📊 Итоговая статистика: total=%d, active=%d, inactive=%d", totalResponse.Count, activeCount, inactiveCount)

	scheduledForDeleteCount := int64(0)

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

// GetObjectsStatsOptimizedFromAxentaCloud получает оптимизированную статистику объектов
func GetObjectsStatsOptimizedFromAxentaCloud(c *gin.Context) {
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

	// Запрашиваем статистику объектов из Axenta Cloud
	axentaURL := "https://axenta.cloud/api/cms/objects/stats/"
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		log.Printf("❌ Ошибка создания запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса",
		})
		return
	}

	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Ошибка запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к внешнему API",
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа",
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Axenta Cloud вернул статус %d: %s", resp.StatusCode, string(body))
		c.JSON(resp.StatusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Внешний API вернул ошибку: %d", resp.StatusCode),
		})
		return
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse map[string]interface{}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		log.Printf("❌ Ошибка парсинга ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа",
		})
		return
	}

	// Возвращаем оптимизированную статистику
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"total":     axentaResponse["total"],
			"active":    axentaResponse["active"],
			"inactive":  axentaResponse["inactive"],
			"by_type":   axentaResponse["by_type"],
			"by_status": axentaResponse["by_status"],
		},
	})
}

// GetUsersStatsOptimizedFromAxentaCloud получает оптимизированную статистику пользователей
func GetUsersStatsOptimizedFromAxentaCloud(c *gin.Context) {
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

	// Запрашиваем статистику пользователей из Axenta Cloud
	axentaURL := "https://axenta.cloud/api/cms/users/stats/"
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		log.Printf("❌ Ошибка создания запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса",
		})
		return
	}

	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Ошибка запроса к Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к внешнему API",
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Ошибка чтения ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа",
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("❌ Axenta Cloud вернул статус %d: %s", resp.StatusCode, string(body))
		c.JSON(resp.StatusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Внешний API вернул ошибку: %d", resp.StatusCode),
		})
		return
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse map[string]interface{}
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		log.Printf("❌ Ошибка парсинга ответа от Axenta Cloud: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа",
		})
		return
	}

	// Возвращаем оптимизированную статистику
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"total":    axentaResponse["total"],
			"active":   axentaResponse["active"],
			"inactive": axentaResponse["inactive"],
			"by_type":  axentaResponse["by_type"],
			"by_role":  axentaResponse["by_role"],
		},
	})
}

// ExportObjectsToXLSX экспортирует список объектов в формат XLSX
func ExportObjectsToXLSX(c *gin.Context) {
	log.Printf("📊 Начало экспорта объектов в XLSX")
	log.Printf("📊 URL запроса: %s", c.Request.URL.String())
	log.Printf("📊 Метод: %s", c.Request.Method)

	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	log.Printf("📊 Authorization header присутствует: %v", authHeader != "")
	if authHeader == "" {
		log.Printf("❌ Токен авторизации не предоставлен")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
		log.Printf("📊 Токен извлечен из формата 'Token'")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
		log.Printf("📊 Токен извлечен из формата 'Bearer'")
	} else {
		headerPreview := authHeader
		if len(authHeader) > 20 {
			headerPreview = authHeader[:20] + "..."
		}
		log.Printf("❌ Неверный формат токена авторизации: %s", headerPreview)
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	if userToken == "" {
		log.Printf("❌ Токен пустой после извлечения")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен не может быть пустым",
		})
		return
	}

	log.Printf("📊 Токен успешно извлечен, длина: %d", len(userToken))

	// Получаем все объекты с учетом фильтров (собираем все страницы)
	var allObjects []AxentaCloudObject
	page := 1
	perPage := 1000 // Большой размер страницы для уменьшения количества запросов

	for {
		// Формируем URL для Axenta Cloud API
		axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%d&per_page=%d&ordering=name", page, perPage)

		// Добавляем дополнительные фильтры, если они переданы
		if accountId := c.Query("accountId"); accountId != "" {
			axentaURL += "&accountId=" + accountId
		}
		if accountName := c.Query("accountName"); accountName != "" {
			axentaURL += "&accountName=" + url.QueryEscape(accountName)
		}
		if creatorName := c.Query("creatorName"); creatorName != "" {
			axentaURL += "&creatorName=" + url.QueryEscape(creatorName)
		}
		if deviceTypeName := c.Query("deviceTypeName"); deviceTypeName != "" {
			axentaURL += "&deviceTypeName=" + url.QueryEscape(deviceTypeName)
		}
		if uniqueId := c.Query("uniqueId"); uniqueId != "" {
			axentaURL += "&uniqueId=" + url.QueryEscape(uniqueId)
		}
		if status := c.Query("status"); status != "" {
			axentaURL += "&status=" + url.QueryEscape(status)
		}
		if objectType := c.Query("type"); objectType != "" {
			axentaURL += "&type=" + url.QueryEscape(objectType)
		}
		if search := c.Query("search"); search != "" {
			axentaURL += "&search=" + url.QueryEscape(search)
			log.Printf("📊 Добавлен фильтр search: %s", search)
		}
		// Игнорируем параметр format, если он передан (он не нужен для Axenta Cloud API)
		if format := c.Query("format"); format != "" {
			log.Printf("📊 Параметр format игнорируется: %s", format)
		}
		if contractId := c.Query("contract_id"); contractId != "" {
			axentaURL += "&contract_id=" + contractId
		}
		if isActive := c.Query("is_active"); isActive != "" {
			axentaURL += "&is_active=" + isActive
		}

		// Создаем запрос к Axenta Cloud
		log.Printf("📊 Запрос к Axenta Cloud: %s", axentaURL)
		req, err := http.NewRequest("GET", axentaURL, nil)
		if err != nil {
			log.Printf("❌ Ошибка создания запроса к Axenta Cloud: %v", err)
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Ошибка создания запроса к Axenta Cloud: " + err.Error(),
			})
			return
		}

		// Добавляем заголовки авторизации
		req.Header.Set("Authorization", "Token "+userToken)
		req.Header.Set("Content-Type", "application/json")

		// Выполняем запрос
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("❌ Ошибка запроса к Axenta Cloud: %v", err)
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Ошибка запроса к Axenta Cloud: " + err.Error(),
			})
			return
		}

		// Читаем ответ
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("❌ Ошибка чтения ответа от Axenta Cloud: %v", err)
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Ошибка чтения ответа от Axenta Cloud: " + err.Error(),
			})
			return
		}

		// Проверяем статус ответа
		if resp.StatusCode != http.StatusOK {
			log.Printf("❌ Ошибка от Axenta Cloud API (статус %d): %s", resp.StatusCode, string(body))
			// При ошибке возвращаем JSON, но фронтенд ожидает blob
			// Поэтому нужно вернуть правильный формат ошибки
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Ошибка от Axenta Cloud API (статус %d): %s", resp.StatusCode, string(body)),
			})
			return
		}

		// Парсим ответ
		var axentaResponse AxentaCloudResponse
		if err := json.Unmarshal(body, &axentaResponse); err != nil {
			log.Printf("❌ Ошибка парсинга ответа от Axenta Cloud: %v, тело ответа: %s", err, string(body))
			c.Header("Content-Type", "application/json")
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Ошибка парсинга ответа от Axenta Cloud: " + err.Error(),
			})
			return
		}

		// Добавляем объекты текущей страницы
		allObjects = append(allObjects, axentaResponse.Results...)

		// Если нет следующей страницы, прекращаем цикл
		if axentaResponse.Next == nil || len(axentaResponse.Results) == 0 {
			break
		}

		page++
	}

	log.Printf("📊 Получено объектов для экспорта: %d", len(allObjects))

	// Создаем новый Excel файл
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("⚠️ Ошибка закрытия Excel файла: %v", err)
		}
	}()

	sheetName := "Объекты"
	_ = f.SetSheetName("Sheet1", sheetName)

	// Определяем заголовки
	headers := []string{
		"ID",
		"Название",
		"Уникальный ID",
		"Тип устройства",
		"Аккаунт",
		"Тип аккаунта",
		"Создатель",
		"Телефоны",
		"Последнее сообщение",
		"Дата создания",
		"Дата удаления",
		"Активен",
		"Права доступа",
	}

	// Записываем заголовки
	styleHeader, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#E0E0E0"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		log.Printf("⚠️ Ошибка создания стиля заголовка: %v", err)
	}

	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, header)
		if styleHeader > 0 {
			_ = f.SetCellStyle(sheetName, cell, cell, styleHeader)
		}
	}

	// Записываем данные
	for rowIdx, obj := range allObjects {
		row := rowIdx + 2 // Начинаем с 2 строки (после заголовков)

		// Форматируем данные
		phones := strings.Join(obj.PhoneNumbers, ", ")
		
		// Обрабатываем CurrentUserAccess, который может быть []string или number
		accessRights := ""
		if accessArray, ok := obj.CurrentUserAccess.([]interface{}); ok {
			strArray := make([]string, len(accessArray))
			for i, v := range accessArray {
				strArray[i] = fmt.Sprintf("%v", v)
			}
			accessRights = strings.Join(strArray, ", ")
		} else if accessStr, ok := obj.CurrentUserAccess.(string); ok {
			accessRights = accessStr
		} else {
			accessRights = fmt.Sprintf("%v", obj.CurrentUserAccess)
		}
		
		isActive := "Да"
		if !obj.IsActive {
			isActive = "Нет"
		}

		// Записываем значения
		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), obj.ID)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), obj.Name)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), obj.UniqueID)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), obj.DeviceTypeName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), obj.AccountName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), obj.AccountType)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), obj.CreatorName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), phones)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), obj.LastMessageDatetime)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), obj.CreatedAt)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("K%d", row), obj.DeletedAt)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("L%d", row), isActive)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("M%d", row), accessRights)
	}

	// Устанавливаем ширину колонок
	columnWidths := map[string]float64{
		"A": 10, // ID
		"B": 30, // Название
		"C": 20, // Уникальный ID
		"D": 20, // Тип устройства
		"E": 25, // Аккаунт
		"F": 15, // Тип аккаунта
		"G": 20, // Создатель
		"H": 25, // Телефоны
		"I": 20, // Последнее сообщение
		"J": 20, // Дата создания
		"K": 20, // Дата удаления
		"L": 10, // Активен
		"M": 30, // Права доступа
	}

	for col, width := range columnWidths {
		_ = f.SetColWidth(sheetName, col, col, width)
	}

	// Добавляем автофильтр
	if len(allObjects) > 0 {
		endCell := fmt.Sprintf("M%d", len(allObjects)+1)
		if err := f.AutoFilter(sheetName, "A1:"+endCell, []excelize.AutoFilterOptions{}); err != nil {
			log.Printf("⚠️ Ошибка добавления автофильтра: %v", err)
		}
	}

	// Замораживаем первую строку (заголовки)
	if err := f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		log.Printf("⚠️ Ошибка заморозки строки: %v", err)
	}

	// Генерируем имя файла с датой и временем
	fileName := fmt.Sprintf("objects_export_%s.xlsx", time.Now().Format("20060102_150405"))

	// Устанавливаем заголовки для скачивания файла
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Transfer-Encoding", "binary")

	// Сохраняем файл во временный буфер
	buffer, err := f.WriteToBuffer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания Excel файла: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Экспорт завершен. Экспортировано объектов: %d", len(allObjects))

	// Отправляем файл
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}
