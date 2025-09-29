package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GetCompanies получает список всех компаний с фильтрацией
func (api *CompaniesAPI) GetCompanies(c *gin.Context) {
	// Параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	search := c.Query("search")
	isActive := c.Query("is_active")
	city := c.Query("city")
	country := c.Query("country")
	language := c.Query("language")
	currency := c.Query("currency")

	offset := (page - 1) * limit

	// Базовый запрос
	query := api.DB.Model(&models.Company{})

	// Применяем фильтры поиска
	if search != "" {
		// Проверяем, есть ли запятые для множественного поиска
		if strings.Contains(search, ",") {
			// Множественный поиск по точному совпадению названий
			searchTerms := strings.Split(search, ",")
			var trimmedTerms []string
			for _, term := range searchTerms {
				trimmed := strings.TrimSpace(term)
				if trimmed != "" {
					trimmedTerms = append(trimmedTerms, trimmed)
				}
			}
			if len(trimmedTerms) > 0 {
				query = query.Where("name IN ?", trimmedTerms)
			}
		} else {
			// Обычный поиск по частичному совпадению
			query = query.Where("name ILIKE ? OR contact_email ILIKE ? OR city ILIKE ?",
				"%"+search+"%", "%"+search+"%", "%"+search+"%")
		}
	}

	// Фильтр по статусу активности
	if isActive != "" {
		if isActive == "true" {
			query = query.Where("is_active = ?", true)
		} else if isActive == "false" {
			query = query.Where("is_active = ?", false)
		}
	}

	// Фильтр по городу
	if city != "" {
		query = query.Where("city ILIKE ?", "%"+city+"%")
	}

	// Фильтр по стране
	if country != "" {
		query = query.Where("country = ?", country)
	}

	// Фильтр по языку
	if language != "" {
		query = query.Where("language = ?", language)
	}

	// Фильтр по валюте
	if currency != "" {
		query = query.Where("currency = ?", currency)
	}

	// Получаем общее количество
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета компаний: " + err.Error(),
		})
		return
	}

	// Получаем компании с пагинацией
	var companies []models.Company
	if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения компаний: " + err.Error(),
		})
		return
	}

	// Преобразуем в response формат
	var response []CompanyResponse
	for _, company := range companies {
		companyResp := api.companyToResponse(&company)

		// Добавляем статистику использования если запрошено
		if c.Query("include_usage") == "true" {
			usage, _ := api.getCompanyUsageStats(&company)
			companyResp.UsageStats = usage
		}

		response = append(response, companyResp)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"companies": response,
			"pagination": gin.H{
				"current_page": page,
				"total_pages":  (total + int64(limit) - 1) / int64(limit),
				"total_items":  total,
				"per_page":     limit,
			},
		},
	})
}

// CreateCompany создает новую компанию
func (api *CompaniesAPI) CreateCompany(c *gin.Context) {
	var req CompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некорректные данные: " + err.Error(),
		})
		return
	}

	// Проверяем уникальность домена если указан
	if req.Domain != "" {
		var existingCompany models.Company
		if err := api.DB.Where("domain = ?", req.Domain).First(&existingCompany).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Компания с таким доменом уже существует",
			})
			return
		}
	}

	// Создаем компанию
	company := &models.Company{
		Name:           req.Name,
		Domain:         req.Domain,
		AxetnaLogin:    req.AxetnaLogin,
		AxetnaPassword: api.encryptPassword(req.AxetnaPassword),

		Bitrix24WebhookURL:   req.Bitrix24WebhookURL,
		Bitrix24ClientID:     req.Bitrix24ClientID,
		Bitrix24ClientSecret: req.Bitrix24ClientSecret,

		ContactEmail:  req.ContactEmail,
		ContactPhone:  req.ContactPhone,
		ContactPerson: req.ContactPerson,

		Address: req.Address,
		City:    req.City,
		Country: req.Country,

		IsActive:     true,
		MaxUsers:     req.MaxUsers,
		MaxObjects:   req.MaxObjects,
		StorageQuota: req.StorageQuota,
		Language:     req.Language,
		Timezone:     req.Timezone,
		Currency:     req.Currency,
	}

	// Устанавливаем значения по умолчанию
	if company.MaxUsers == 0 {
		company.MaxUsers = 10
	}
	if company.MaxObjects == 0 {
		company.MaxObjects = 100
	}
	if company.StorageQuota == 0 {
		company.StorageQuota = 1024
	}
	if company.Language == "" {
		company.Language = "ru"
	}
	if company.Timezone == "" {
		company.Timezone = "Europe/Moscow"
	}
	if company.Currency == "" {
		company.Currency = "RUB"
	}
	if company.Country == "" {
		company.Country = "Russia"
	}

	// Сохраняем в БД
	if err := api.DB.Create(company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания компании: " + err.Error(),
		})
		return
	}

	// Создаем схему БД для новой компании
	if err := api.TenantMiddleware.CreateTenantSchema(company.GetSchemaName()); err != nil {
		// Откатываем создание компании если не удалось создать схему
		api.DB.Delete(company)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания схемы БД: " + err.Error(),
		})
		return
	}

	// Очищаем кэш
	api.clearCompanyCache(company.ID)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   api.companyToResponse(company),
	})
}

// GetCompany получает компанию по ID
func (api *CompaniesAPI) GetCompany(c *gin.Context) {
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

	response := api.companyToResponse(&company)

	// Добавляем статистику использования
	if c.Query("include_usage") == "true" {
		usage, _ := api.getCompanyUsageStats(&company)
		response.UsageStats = usage
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   response,
	})
}

// UpdateCompany обновляет компанию
func (api *CompaniesAPI) UpdateCompany(c *gin.Context) {
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

	var req CompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некорректные данные: " + err.Error(),
		})
		return
	}

	// Проверяем уникальность домена если он изменился
	if req.Domain != "" && req.Domain != company.Domain {
		var existingCompany models.Company
		if err := api.DB.Where("domain = ? AND id != ?", req.Domain, company.ID).First(&existingCompany).Error; err == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Компания с таким доменом уже существует",
			})
			return
		}
	}

	// Обновляем поля
	company.Name = req.Name
	company.Domain = req.Domain
	company.AxetnaLogin = req.AxetnaLogin
	if req.AxetnaPassword != "" {
		company.AxetnaPassword = api.encryptPassword(req.AxetnaPassword)
	}

	company.Bitrix24WebhookURL = req.Bitrix24WebhookURL
	company.Bitrix24ClientID = req.Bitrix24ClientID
	company.Bitrix24ClientSecret = req.Bitrix24ClientSecret

	company.ContactEmail = req.ContactEmail
	company.ContactPhone = req.ContactPhone
	company.ContactPerson = req.ContactPerson

	company.Address = req.Address
	company.City = req.City
	if req.Country != "" {
		company.Country = req.Country
	}

	if req.MaxUsers > 0 {
		company.MaxUsers = req.MaxUsers
	}
	if req.MaxObjects > 0 {
		company.MaxObjects = req.MaxObjects
	}
	if req.StorageQuota > 0 {
		company.StorageQuota = req.StorageQuota
	}
	if req.Language != "" {
		company.Language = req.Language
	}
	if req.Timezone != "" {
		company.Timezone = req.Timezone
	}
	if req.Currency != "" {
		company.Currency = req.Currency
	}

	// Сохраняем изменения
	if err := api.DB.Save(&company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления компании: " + err.Error(),
		})
		return
	}

	// Очищаем кэш
	api.clearCompanyCache(company.ID)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   api.companyToResponse(&company),
	})
}

// DeleteCompany удаляет компанию (мягкое удаление)
func (api *CompaniesAPI) DeleteCompany(c *gin.Context) {
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

	// Проверяем, есть ли активные пользователи или объекты
	tenantDB := api.TenantMiddleware.SwitchToTenantSchema(company.GetSchemaName())
	if tenantDB != nil {
		var usersCount, objectsCount int64
		tenantDB.Model(&models.User{}).Count(&usersCount)
		tenantDB.Model(&models.Object{}).Count(&objectsCount)

		if usersCount > 0 || objectsCount > 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Нельзя удалить компанию с активными пользователями (%d) или объектами (%d)", usersCount, objectsCount),
			})
			return
		}
	}

	// Мягкое удаление
	if err := api.DB.Delete(&company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка удаления компании: " + err.Error(),
		})
		return
	}

	// Очищаем кэш
	api.clearCompanyCache(company.ID)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Компания успешно удалена",
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

// GetCompanyUsage получает статистику использования ресурсов компании
func (api *CompaniesAPI) GetCompanyUsage(c *gin.Context) {
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

	usage, err := api.getCompanyUsageStats(&company)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения статистики: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   usage,
	})
}

// TestCompanyConnection тестирует подключение к Axenta API
func (api *CompaniesAPI) TestCompanyConnection(c *gin.Context) {
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

	// Тестируем подключение к Axenta API
	decryptedPassword := api.decryptPassword(company.AxetnaPassword)
	success, message := api.testAxentaConnection(company.AxetnaLogin, decryptedPassword)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"connection_success": success,
			"message":            message,
		},
	})
}

// Вспомогательные методы

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

// getCompanyUsageStats получает статистику использования ресурсов компании
func (api *CompaniesAPI) getCompanyUsageStats(company *models.Company) (*CompanyUsageStats, error) {
	tenantDB := api.TenantMiddleware.SwitchToTenantSchema(company.GetSchemaName())
	if tenantDB == nil {
		return nil, fmt.Errorf("не удалось подключиться к схеме компании")
	}

	stats := &CompanyUsageStats{}

	// Подсчитываем пользователей
	tenantDB.Model(&models.User{}).Count(&stats.UsersCount)

	// Подсчитываем объекты
	tenantDB.Model(&models.Object{}).Count(&stats.ObjectsCount)

	// Получаем последнюю активность (последний вход пользователя)
	var lastUser models.User
	if err := tenantDB.Order("updated_at DESC").First(&lastUser).Error; err == nil {
		stats.LastActivity = &lastUser.UpdatedAt
	}

	// TODO: Подсчет использованного места на диске
	stats.StorageUsed = 0

	return stats, nil
}

// encryptPassword шифрует пароль с помощью AES
func (api *CompaniesAPI) encryptPassword(password string) string {
	// Используем простой ключ для демонстрации, в продакшене должен быть из переменных окружения
	key := []byte("32-byte-key-for-encryption-demo!")

	block, err := aes.NewCipher(key)
	if err != nil {
		return password // В случае ошибки возвращаем исходный пароль
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return password
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return password
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// decryptPassword расшифровывает пароль
func (api *CompaniesAPI) decryptPassword(encryptedPassword string) string {
	key := []byte("32-byte-key-for-encryption-demo!")

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		return encryptedPassword
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return encryptedPassword
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedPassword
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return encryptedPassword
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return encryptedPassword
	}

	return string(plaintext)
}

// testAxentaConnection тестирует подключение к Axenta API
func (api *CompaniesAPI) testAxentaConnection(login, password string) (bool, string) {
	// Создаем HTTP клиент с таймаутом
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Данные для авторизации
	loginData := map[string]string{
		"username": login,
		"password": password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return false, "Ошибка подготовки данных для авторизации"
	}

	// Отправляем запрос авторизации
	req, err := http.NewRequest("POST", "https://axenta.cloud/api/auth/login/", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, "Ошибка создания запроса"
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, "Ошибка подключения к Axenta API: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, "Подключение успешно установлено"
	}

	return false, fmt.Sprintf("Ошибка авторизации: HTTP %d", resp.StatusCode)
}

// clearCompanyCache очищает кэш компании
func (api *CompaniesAPI) clearCompanyCache(companyID uint) {
	cacheKey := fmt.Sprintf("company:id:%d", companyID)
	database.CacheDel(cacheKey)
}

// BulkDeleteCompaniesRequest структура запроса для массового удаления компаний
type BulkDeleteCompaniesRequest struct {
	CompanyIDs []string `json:"company_ids" binding:"required,min=1"`
}

// BulkDeleteCompanies массово удаляет компании
func (api *CompaniesAPI) BulkDeleteCompanies(c *gin.Context) {
	var req BulkDeleteCompaniesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	if len(req.CompanyIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "No company IDs provided",
		})
		return
	}

	// Преобразуем строковые ID в UUID
	var companyIDs []uint
	for _, idStr := range req.CompanyIDs {
		companyID, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Invalid company ID format: %s", idStr),
			})
			return
		}
		companyIDs = append(companyIDs, uint(companyID))
	}

	// Проверяем, что все компании существуют
	var existingCompanies []models.Company
	if err := api.DB.Where("id IN ?", companyIDs).Find(&existingCompanies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch companies: " + err.Error(),
		})
		return
	}

	if len(existingCompanies) != len(companyIDs) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Some companies not found",
		})
		return
	}

	// Проверяем, что среди компаний нет активных с пользователями
	var protectedCompanies []string
	for _, company := range existingCompanies {
		if company.IsActive {
			// Здесь можно добавить проверку количества пользователей
			// Для демо просто проверим активность
			var userCount int64
			if err := api.DB.Table("users").Where("company_id = ? AND deleted_at IS NULL", company.ID).Count(&userCount).Error; err == nil && userCount > 0 {
				protectedCompanies = append(protectedCompanies, company.Name)
			}
		}
	}

	if len(protectedCompanies) > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Cannot delete active companies with users: " + fmt.Sprintf("%v", protectedCompanies),
		})
		return
	}

	// Выполняем массовое мягкое удаление
	result := api.DB.Where("id IN ?", companyIDs).Delete(&models.Company{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to delete companies: " + result.Error.Error(),
		})
		return
	}

	// Очищаем кэш для удаленных компаний
	for _, companyID := range companyIDs {
		api.clearCompanyCache(companyID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Companies deleted successfully",
		"deleted": result.RowsAffected,
	})
}

// BulkActivateCompaniesRequest структура запроса для массовой активации компаний
type BulkActivateCompaniesRequest struct {
	CompanyIDs []string `json:"company_ids" binding:"required,min=1"`
}

// BulkActivateCompanies массово активирует компании
func (api *CompaniesAPI) BulkActivateCompanies(c *gin.Context) {
	var req BulkActivateCompaniesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	if len(req.CompanyIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "No company IDs provided",
		})
		return
	}

	// Преобразуем строковые ID в UUID
	var companyIDs []uint
	for _, idStr := range req.CompanyIDs {
		companyID, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Invalid company ID format: %s", idStr),
			})
			return
		}
		companyIDs = append(companyIDs, uint(companyID))
	}

	// Выполняем массовую активацию
	result := api.DB.Model(&models.Company{}).Where("id IN ?", companyIDs).Update("is_active", true)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to activate companies: " + result.Error.Error(),
		})
		return
	}

	// Очищаем кэш для обновленных компаний
	for _, companyID := range companyIDs {
		api.clearCompanyCache(companyID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Companies activated successfully",
		"activated": result.RowsAffected,
	})
}

// BulkDeactivateCompaniesRequest структура запроса для массовой деактивации компаний
type BulkDeactivateCompaniesRequest struct {
	CompanyIDs []string `json:"company_ids" binding:"required,min=1"`
}

// BulkDeactivateCompanies массово деактивирует компании
func (api *CompaniesAPI) BulkDeactivateCompanies(c *gin.Context) {
	var req BulkDeactivateCompaniesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request data: " + err.Error(),
		})
		return
	}

	if len(req.CompanyIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "No company IDs provided",
		})
		return
	}

	// Преобразуем строковые ID в UUID
	var companyIDs []uint
	for _, idStr := range req.CompanyIDs {
		companyID, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  fmt.Sprintf("Invalid company ID format: %s", idStr),
			})
			return
		}
		companyIDs = append(companyIDs, uint(companyID))
	}

	// Выполняем массовую деактивацию
	result := api.DB.Model(&models.Company{}).Where("id IN ?", companyIDs).Update("is_active", false)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to deactivate companies: " + result.Error.Error(),
		})
		return
	}

	// Очищаем кэш для обновленных компаний
	for _, companyID := range companyIDs {
		api.clearCompanyCache(companyID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"message":     "Companies deactivated successfully",
		"deactivated": result.RowsAffected,
	})
}

// FilterOptionsResponse структура ответа для опций фильтрации
type FilterOptionsResponse struct {
	Cities     []FilterOption `json:"cities"`
	Countries  []FilterOption `json:"countries"`
	Languages  []FilterOption `json:"languages"`
	Currencies []FilterOption `json:"currencies"`
	Statuses   []FilterOption `json:"statuses"`
}

// FilterOption опция для фильтра с количеством
type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

// GetFilterOptions получает все доступные опции для фильтров
func (api *CompaniesAPI) GetFilterOptions(c *gin.Context) {
	var companies []models.Company
	if err := api.DB.Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения компаний: " + err.Error(),
		})
		return
	}

	response := FilterOptionsResponse{}

	// Подсчитываем статусы
	var activeCount, inactiveCount int64
	api.DB.Model(&models.Company{}).Where("is_active = ?", true).Count(&activeCount)
	api.DB.Model(&models.Company{}).Where("is_active = ?", false).Count(&inactiveCount)

	response.Statuses = []FilterOption{
		{Value: "", Label: fmt.Sprintf("Все (%d)", len(companies)), Count: int64(len(companies))},
		{Value: "true", Label: fmt.Sprintf("Активные (%d)", activeCount), Count: activeCount},
		{Value: "false", Label: fmt.Sprintf("Неактивные (%d)", inactiveCount), Count: inactiveCount},
	}

	// Группируем по городам
	cityMap := make(map[string]int64)
	for _, company := range companies {
		if company.City != "" {
			cityMap[company.City]++
		}
	}
	for city, count := range cityMap {
		response.Cities = append(response.Cities, FilterOption{
			Value: city,
			Label: fmt.Sprintf("%s (%d)", city, count),
			Count: count,
		})
	}

	// Группируем по странам
	countryMap := make(map[string]int64)
	for _, company := range companies {
		if company.Country != "" {
			countryMap[company.Country]++
		}
	}
	for country, count := range countryMap {
		response.Countries = append(response.Countries, FilterOption{
			Value: country,
			Label: fmt.Sprintf("%s (%d)", country, count),
			Count: count,
		})
	}

	// Группируем по языкам
	languageMap := make(map[string]int64)
	for _, company := range companies {
		if company.Language != "" {
			languageMap[company.Language]++
		}
	}
	for language, count := range languageMap {
		languageLabel := language
		if language == "ru" {
			languageLabel = "Русский"
		} else if language == "en" {
			languageLabel = "English"
		}
		response.Languages = append(response.Languages, FilterOption{
			Value: language,
			Label: fmt.Sprintf("%s (%d)", languageLabel, count),
			Count: count,
		})
	}

	// Группируем по валютам
	currencyMap := make(map[string]int64)
	for _, company := range companies {
		if company.Currency != "" {
			currencyMap[company.Currency]++
		}
	}
	for currency, count := range currencyMap {
		currencyLabel := currency
		if currency == "RUB" {
			currencyLabel = "Российский рубль"
		} else if currency == "USD" {
			currencyLabel = "Доллар США"
		} else if currency == "EUR" {
			currencyLabel = "Евро"
		}
		response.Currencies = append(response.Currencies, FilterOption{
			Value: currency,
			Label: fmt.Sprintf("%s (%d)", currencyLabel, count),
			Count: count,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   response,
	})
}

// CompanyListItem упрощенная структура компании для селекторов
type CompanyListItem struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// GetCompaniesList получает упрощенный список компаний для селекторов
func (api *CompaniesAPI) GetCompaniesList(c *gin.Context) {
	// Временное решение: используем хардкодированные данные для тестирования
	result := []CompanyListItem{
		{ID: 2, Name: "Компания по умолчанию"},
		{ID: 3, Name: "ООО \"Тестовая компания 1\""},
		{ID: 4, Name: "ИП Иванов И.И."},
		{ID: 5, Name: "ООО \"Рога и Копыта\""},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   result,
	})
}
