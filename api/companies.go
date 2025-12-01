package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CompaniesAPI управляет API для учетных записей компаний
type CompaniesAPI struct {
	DB               *gorm.DB
	TenantMiddleware *middleware.TenantMiddleware
}

// NewCompaniesAPI создает новый экземпляр CompaniesAPI
func NewCompaniesAPI(db *gorm.DB, tenantMiddleware *middleware.TenantMiddleware) *CompaniesAPI {
	return &CompaniesAPI{
		DB:               db,
		TenantMiddleware: tenantMiddleware,
	}
}

// CompanyRequest структура для создания/обновления компании
type CompanyRequest struct {
	Name           string `json:"name" binding:"required,min=1,max=100"`
	Domain         string `json:"domain,omitempty"`
	AxetnaLogin    string `json:"axetna_login" binding:"required,min=1,max=100"`
	AxetnaPassword string `json:"axetna_password" binding:"required,min=1"`

	// Интеграция с Битрикс24
	Bitrix24WebhookURL   string `json:"bitrix24_webhook_url,omitempty"`
	Bitrix24ClientID     string `json:"bitrix24_client_id,omitempty"`
	Bitrix24ClientSecret string `json:"bitrix24_client_secret,omitempty"`

	// Контактная информация
	ContactEmail  string `json:"contact_email,omitempty"`
	ContactPhone  string `json:"contact_phone,omitempty"`
	ContactPerson string `json:"contact_person,omitempty"`

	// Адрес
	Address string `json:"address,omitempty"`
	City    string `json:"city,omitempty"`
	Country string `json:"country,omitempty"`

	// Настройки
	MaxUsers     int    `json:"max_users,omitempty"`
	MaxObjects   int    `json:"max_objects,omitempty"`
	StorageQuota int    `json:"storage_quota,omitempty"`
	Language     string `json:"language,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
	Currency     string `json:"currency,omitempty"`
}

// CompanyResponse структура ответа для компании
type CompanyResponse struct {
	ID             uint      `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Name           string    `json:"name"`
	DatabaseSchema string    `json:"database_schema"`
	Domain         string    `json:"domain"`

	// Контактная информация
	ContactEmail  string `json:"contact_email"`
	ContactPhone  string `json:"contact_phone"`
	ContactPerson string `json:"contact_person"`

	// Адрес
	Address string `json:"address"`
	City    string `json:"city"`
	Country string `json:"country"`

	// Настройки и статус
	IsActive     bool   `json:"is_active"`
	MaxUsers     int    `json:"max_users"`
	MaxObjects   int    `json:"max_objects"`
	StorageQuota int    `json:"storage_quota"`
	Language     string `json:"language"`
	Timezone     string `json:"timezone"`
	Currency     string `json:"currency"`

	// Статистика использования
	UsageStats *CompanyUsageStats `json:"usage_stats,omitempty"`
}

// CompanyUsageStats статистика использования ресурсов компании
type CompanyUsageStats struct {
	UsersCount   int64      `json:"users_count"`
	ObjectsCount int64      `json:"objects_count"`
	StorageUsed  int64      `json:"storage_used_mb"`
	LastActivity *time.Time `json:"last_activity"`
}

// RegisterCompaniesRoutes регистрирует маршруты для управления компаниями
func (api *CompaniesAPI) RegisterCompaniesRoutes(r *gin.RouterGroup) {
	companies := r.Group("/accounts")
	{
		companies.GET("", api.GetCompanies)
		companies.POST("", api.CreateCompany)
		companies.GET("/filter-options", api.GetFilterOptions)
		companies.GET("/list", api.GetCompaniesList) // Упрощенный список для селекторов
		companies.POST("/bulk-delete", api.BulkDeleteCompanies)
		companies.POST("/bulk-activate", api.BulkActivateCompanies)
		companies.POST("/bulk-deactivate", api.BulkDeactivateCompanies)
		companies.GET("/:id", api.GetCompany)
		companies.PUT("/:id", api.UpdateCompany)
		companies.DELETE("/:id", api.DeleteCompany)
		companies.PUT("/:id/activate", api.ActivateCompany)
		companies.PUT("/:id/deactivate", api.DeactivateCompany)
		companies.GET("/:id/usage", api.GetCompanyUsage)
		companies.POST("/:id/test-connection", api.TestCompanyConnection)
	}
}

// AxentaAccount представляет учетную запись из Axenta Cloud API
type AxentaAccount struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	AdminFullname      string `json:"admin_fullname"`
	AdminID            int    `json:"admin_id"`
	AdminIsActive      bool   `json:"admin_is_active"`
	ParentAccountName  string `json:"parent_account_name"`
	ObjectsActive      int    `json:"objects_active"`
	ObjectsTotal       int    `json:"objects_total"`
	ObjectsDeleted     int    `json:"objects_deleted"`
	Comment            string `json:"comment"`
	IsActive           bool   `json:"is_active"`
	BlockingDatetime   string `json:"blocking_datetime"`
	Hierarchy          string `json:"hierarchy"`
	DaysBeforeBlocking int    `json:"days_before_blocking"`
	CreationDatetime   string `json:"creation_datetime"`
}

// AxentaAccountsResponse представляет ответ от Axenta Cloud API
type AxentaAccountsResponse struct {
	Count    int             `json:"count"`
	Next     string          `json:"next"`
	Previous string          `json:"previous"`
	Results  []AxentaAccount `json:"results"`
}

// GetCompanies получает список всех компаний напрямую с Axenta Cloud API
func (api *CompaniesAPI) GetCompanies(c *gin.Context) {
	fmt.Println("🔧 DEBUG: GetCompanies called - NEW VERSION")

	// Получаем токен из контекста (должен быть установлен middleware)
	token, exists := c.Get("token")
	if !exists {
		// Пробуем получить токен из заголовка напрямую
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			authHeader = c.GetHeader("authorization")
		}

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Authorization token required",
			})
			return
		}

		// Извлекаем токен
		if strings.HasPrefix(authHeader, "Token ") {
			token = strings.TrimPrefix(authHeader, "Token ")
		} else if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			token = authHeader
		}
	}

	// Параметры запроса
	page := c.DefaultQuery("page", "1")
	// Поддерживаем оба варианта: per_page (старый) и limit (новый)
	perPage := c.Query("per_page")
	if perPage == "" {
		perPage = c.Query("limit")
	}
	if perPage == "" {
		perPage = "50" // Значение по умолчанию
	}
	ordering := c.DefaultQuery("ordering", "name")
	search := c.Query("search")
	isActive := c.Query("is_active")
	accountType := c.Query("type")

	fmt.Printf("🔧 DEBUG: Received parameters - page: %s, per_page/limit: %s, ordering: %s, search: %s, is_active: '%s', type: %s\n",
		page, perPage, ordering, search, isActive, accountType)

	// Формируем URL для запроса к Axenta Cloud API
	axentaURL := "https://axenta.cloud/api/cms/accounts/"
	params := fmt.Sprintf("?page=%s&per_page=%s&ordering=%s", page, perPage, ordering)

	if search != "" {
		params += "&search=" + search
	}
	if isActive != "" {
		params += "&is_active=" + isActive
	}
	if accountType != "" {
		params += "&type=" + accountType
	}

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос к Axenta Cloud API
	req, err := http.NewRequest("GET", axentaURL+params, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request to Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Добавляем авторизацию
	req.Header.Set("Authorization", "Token "+token.(string))
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to connect to Axenta Cloud: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read response from Axenta Cloud: " + err.Error(),
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Axenta Cloud API returned status %d: %s", resp.StatusCode, string(body)),
		})
		return
	}

	// Логируем сырой ответ от Axenta Cloud для отладки (первые 500 символов)
	if len(body) > 500 {
		fmt.Printf("🔧 DEBUG: Raw response from Axenta Cloud (first 500 chars): %s...\n", string(body[:500]))
	} else {
		fmt.Printf("🔧 DEBUG: Raw response from Axenta Cloud: %s\n", string(body))
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse AxentaAccountsResponse
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse response from Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Логируем распарсенные данные
	fmt.Printf("🔧 DEBUG: Parsed %d accounts from Axenta Cloud\n", len(axentaResponse.Results))
	if len(axentaResponse.Results) > 0 {
		fmt.Printf("🔧 DEBUG: First account hierarchy: '%s'\n", axentaResponse.Results[0].Hierarchy)
	}

	// Возвращаем данные напрямую в точном формате Axenta Cloud API
	fmt.Printf("🔧 DEBUG: Returning %d accounts directly from Axenta Cloud\n", len(axentaResponse.Results))

	// Передаем данные с дополнением реальной статистики объектов
	companies := make([]gin.H, len(axentaResponse.Results))
	for i, account := range axentaResponse.Results {
		fmt.Printf("🔧 DEBUG: Processing account %d: name='%s', hierarchy='%s'\n", account.ID, account.Name, account.Hierarchy)

		// Получаем реальное количество объектов из локальной БД для этой компании
		objectsTotal := api.getObjectsCountForCompany(account.ID)
		objectsActive := api.getActiveObjectsCountForCompany(account.ID)

		// Возвращаем все поля с реальной статистикой объектов
		companies[i] = gin.H{
			"id":                 account.ID,
			"name":               account.Name,
			"type":               account.Type,
			"adminFullname":      account.AdminFullname,
			"adminId":            account.AdminID,
			"adminIsActive":      account.AdminIsActive,
			"parentAccountName":  account.ParentAccountName,
			"objectsActive":      objectsActive,                // Реальное количество активных объектов
			"objectsTotal":       objectsTotal,                 // Реальное общее количество объектов
			"objectsDeleted":     objectsTotal - objectsActive, // Вычисляем удаленные
			"comment":            account.Comment,
			"isActive":           account.IsActive,
			"blockingDatetime":   account.BlockingDatetime,
			"hierarchy":          account.Hierarchy,
			"daysBeforeBlocking": account.DaysBeforeBlocking,
			"creationDatetime":   account.CreationDatetime,
		}

		fmt.Printf("🔧 DEBUG: Account %d - Objects: total=%d, active=%d, deleted=%d\n",
			account.ID, objectsTotal, objectsActive, objectsTotal-objectsActive)
		fmt.Printf("🔧 DEBUG: Final JSON for account %d: type='%s', hierarchy='%s'\n",
			account.ID, companies[i]["type"], companies[i]["hierarchy"])
	}

	// Формируем пагинацию
	pagination := gin.H{
		"current_page": page,
		"per_page":     perPage,
		"total_items":  axentaResponse.Count,
		"total_pages":  (axentaResponse.Count + parseInt(perPage) - 1) / parseInt(perPage),
	}

	// Возвращаем ответ в том же формате, что ожидает фронтенд
	fmt.Printf("🎯 DEBUG: ФИНАЛЬНЫЙ ОТВЕТ - Возвращаем %d компаний клиенту (type='%s')\n", len(companies), accountType)
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"companies":  companies,
			"pagination": pagination,
		},
	})
}

// parseAxentaDate парсит дату из Axenta Cloud API
// Временно закомментировано - функция не используется
/*
func parseAxentaDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Now()
	}

	// Пробуем разные форматы дат
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05+03:00",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	// Если не удалось распарсить, возвращаем текущее время
	return time.Now()
}
*/

// parseInt безопасно парсит строку в int
func parseInt(s string) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return 0
}

// CreateCompany создает новую компанию (заглушка - пока не реализовано)
func (api *CompaniesAPI) CreateCompany(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Creating companies is not implemented yet",
	})
}

// GetCompany получает компанию по ID (заглушка - пока не реализовано)
func (api *CompaniesAPI) GetCompany(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Getting single company is not implemented yet",
	})
}

// UpdateCompany обновляет компанию (заглушка - пока не реализовано)
func (api *CompaniesAPI) UpdateCompany(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Updating companies is not implemented yet",
	})
}

// DeleteCompany удаляет компанию (заглушка - пока не реализовано)
func (api *CompaniesAPI) DeleteCompany(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Deleting companies is not implemented yet",
	})
}

// GetFilterOptions получает опции для фильтров (заглушка)
func (api *CompaniesAPI) GetFilterOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"cities":     []string{},
			"countries":  []string{"Russia"},
			"languages":  []string{"ru", "en"},
			"currencies": []string{"RUB", "USD", "EUR"},
		},
	})
}

// GetCompaniesList получает упрощенный список компаний (заглушка)
func (api *CompaniesAPI) GetCompaniesList(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   []gin.H{},
	})
}

// BulkDeleteCompanies массовое удаление компаний (заглушка)
func (api *CompaniesAPI) BulkDeleteCompanies(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Bulk operations are not implemented yet",
	})
}

// BulkActivateCompanies массовая активация компаний (заглушка)
func (api *CompaniesAPI) BulkActivateCompanies(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Bulk operations are not implemented yet",
	})
}

// BulkDeactivateCompanies массовая деактивация компаний (заглушка)
func (api *CompaniesAPI) BulkDeactivateCompanies(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Bulk operations are not implemented yet",
	})
}

// ActivateCompany активирует компанию
func (api *CompaniesAPI) ActivateCompany(c *gin.Context) {
	api.toggleCompanyStatus(c, true)
}

// DeactivateCompany деактивирует компанию
func (api *CompaniesAPI) DeactivateCompany(c *gin.Context) {
	api.toggleCompanyStatus(c, false)
}

// toggleCompanyStatus изменяет статус активности компании
func (api *CompaniesAPI) toggleCompanyStatus(c *gin.Context, isActive bool) {
	id := c.Param("id")

	var company models.Company
	if err := api.DB.Where("id = ?", id).First(&company).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Компания не найдена",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения компании: " + err.Error(),
		})
		return
	}

	company.IsActive = isActive

	if err := api.DB.Save(&company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка изменения статуса компании: " + err.Error(),
		})
		return
	}

	// Очищаем кэш
	api.clearCompanyCache(company.ID)

	action := "деактивирована"
	if isActive {
		action = "активирована"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Компания успешно %s", action),
		"data":    api.companyToResponse(&company),
	})
}

// GetCompanyUsage получает статистику использования компании (заглушка)
func (api *CompaniesAPI) GetCompanyUsage(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Company usage stats are not implemented yet",
	})
}

// TestCompanyConnection тестирует подключение к Axenta API (заглушка)
func (api *CompaniesAPI) TestCompanyConnection(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"status": "error",
		"error":  "Connection testing is not implemented yet",
	})
}

// getObjectsCountForCompany получает общее количество объектов для компании
func (api *CompaniesAPI) getObjectsCountForCompany(companyID int) int64 {
	// Получаем подключение к БД компании по ID
	tenantDB := api.TenantMiddleware.GetTenantDBByCompanyID(uint(companyID))
	if tenantDB == nil {
		fmt.Printf("⚠️ WARNING: No tenant DB found for company ID %d\n", companyID)
		return 0
	}

	var count int64
	if err := tenantDB.Table("objects").Count(&count).Error; err != nil {
		fmt.Printf("❌ ERROR: Failed to count objects for company %d: %v\n", companyID, err)
		return 0
	}

	fmt.Printf("📊 INFO: Company %d has %d total objects\n", companyID, count)
	return count
}

// getActiveObjectsCountForCompany получает количество активных объектов для компании
func (api *CompaniesAPI) getActiveObjectsCountForCompany(companyID int) int64 {
	// Получаем подключение к БД компании по ID
	tenantDB := api.TenantMiddleware.GetTenantDBByCompanyID(uint(companyID))
	if tenantDB == nil {
		fmt.Printf("⚠️ WARNING: No tenant DB found for company ID %d\n", companyID)
		return 0
	}

	var count int64
	if err := tenantDB.Table("objects").Where("status = ?", "active").Count(&count).Error; err != nil {
		fmt.Printf("❌ ERROR: Failed to count active objects for company %d: %v\n", companyID, err)
		return 0
	}

	fmt.Printf("📊 INFO: Company %d has %d active objects\n", companyID, count)
	return count
}

// encryptPassword шифрует пароль с использованием AES
func (api *CompaniesAPI) encryptPassword(password string) string {
	// Получаем ключ шифрования из переменной окружения
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		// Используем дефолтный ключ для тестов (32 байта для AES-256)
		key = "axenta-default-encryption-key-32b"
	}

	// Убеждаемся, что ключ имеет правильную длину (32 байта для AES-256)
	if len(key) < 32 {
		key = key + strings.Repeat("0", 32-len(key))
	} else if len(key) > 32 {
		key = key[:32]
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to create cipher: %v\n", err)
		return password // Возвращаем исходный пароль в случае ошибки
	}

	// Создаем GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to create GCM: %v\n", err)
		return password
	}

	// Создаем случайный nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		fmt.Printf("❌ ERROR: Failed to generate nonce: %v\n", err)
		return password
	}

	// Шифруем пароль
	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)

	// Кодируем в base64
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// decryptPassword дешифрует пароль
func (api *CompaniesAPI) decryptPassword(encryptedPassword string) string {
	// Получаем ключ шифрования из переменной окружения
	key := os.Getenv("ENCRYPTION_KEY")
	if key == "" {
		// Используем дефолтный ключ для тестов (32 байта для AES-256)
		key = "axenta-default-encryption-key-32b"
	}

	// Убеждаемся, что ключ имеет правильную длину (32 байта для AES-256)
	if len(key) < 32 {
		key = key + strings.Repeat("0", 32-len(key))
	} else if len(key) > 32 {
		key = key[:32]
	}

	// Декодируем из base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to decode base64: %v\n", err)
		return encryptedPassword // Возвращаем исходную строку в случае ошибки
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to create cipher: %v\n", err)
		return encryptedPassword
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to create GCM: %v\n", err)
		return encryptedPassword
	}

	if len(ciphertext) < gcm.NonceSize() {
		fmt.Printf("❌ ERROR: Ciphertext too short\n")
		return encryptedPassword
	}

	// Извлекаем nonce и зашифрованные данные
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// Дешифруем
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to decrypt: %v\n", err)
		return encryptedPassword
	}

	return string(plaintext)
}

// companyToResponse преобразует модель компании в response формат
func (api *CompaniesAPI) companyToResponse(company *models.Company) CompanyResponse {
	return CompanyResponse{
		ID:             company.ID,
		CreatedAt:      company.CreatedAt,
		UpdatedAt:      company.UpdatedAt,
		Name:           company.Name,
		DatabaseSchema: company.DatabaseSchema,
		Domain:         company.Domain,
		ContactEmail:   company.ContactEmail,
		ContactPhone:   company.ContactPhone,
		ContactPerson:  company.ContactPerson,
		Address:        company.Address,
		City:           company.City,
		Country:        company.Country,
		IsActive:       company.IsActive,
		MaxUsers:       company.MaxUsers,
		MaxObjects:     company.MaxObjects,
		StorageQuota:   company.StorageQuota,
		Language:       company.Language,
		Timezone:       company.Timezone,
		Currency:       company.Currency,
	}
}

// clearCompanyCache очищает кэш компании
func (api *CompaniesAPI) clearCompanyCache(companyID uint) {
	// Простая реализация без Redis
	// В будущем можно добавить Redis кэширование
}
