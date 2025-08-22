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
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	ID             uuid.UUID `json:"id"`
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

	offset := (page - 1) * limit

	// Базовый запрос
	query := api.DB.Model(&models.Company{})

	// Применяем фильтры
	if search != "" {
		query = query.Where("name ILIKE ? OR contact_email ILIKE ? OR city ILIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	if isActive != "" {
		if isActive == "true" {
			query = query.Where("is_active = ?", true)
		} else if isActive == "false" {
			query = query.Where("is_active = ?", false)
		}
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
func (api *CompaniesAPI) clearCompanyCache(companyID uuid.UUID) {
	cacheKey := fmt.Sprintf("company:id:%s", companyID.String())
	database.CacheDel(cacheKey)
}
