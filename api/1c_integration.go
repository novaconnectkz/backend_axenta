package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var oneCIntegrationService *services.OneCIntegrationService

// InitOneCService инициализирует сервис интеграции с 1С
func InitOneCService() {
	logger := log.New(os.Stdout, "[1C_API] ", log.LstdFlags|log.Lshortfile)

	// Создаем клиент 1С
	oneCClient := services.NewOneCClient(logger)

	// Создаем кэш сервис
	cacheService := services.NewCacheService(database.RedisClient, logger)

	// Создаем сервис интеграции
	oneCIntegrationService = services.NewOneCIntegrationService(database.DB, oneCClient, cacheService, logger)
}

// OneCIntegrationAPI API для работы с интеграцией 1С
type OneCIntegrationAPI struct {
	db                     *gorm.DB
	oneCIntegrationService *services.OneCIntegrationService
}

// NewOneCIntegrationAPI создает новый API для интеграции с 1С
func NewOneCIntegrationAPI() *OneCIntegrationAPI {
	return &OneCIntegrationAPI{
		db:                     database.DB,
		oneCIntegrationService: oneCIntegrationService,
	}
}

// RegisterRoutes регистрирует маршруты для API интеграции с 1С
func (api *OneCIntegrationAPI) RegisterRoutes(r *gin.RouterGroup) {
	oneC := r.Group("/1c")
	{
		// Настройка интеграции
		oneC.POST("/setup", api.SetupIntegration)
		oneC.PUT("/setup", api.UpdateIntegration)
		oneC.GET("/config", api.GetIntegrationConfig)
		oneC.DELETE("/setup", api.DeleteIntegration)

		// Тестирование подключения
		oneC.POST("/test-connection", api.TestConnection)

		// Экспорт данных в 1С
		oneC.POST("/export/payment-registry", api.ExportPaymentRegistry)
		oneC.POST("/export/payment-registry/auto", api.ScheduleAutoExport)

		// Импорт данных из 1С
		oneC.POST("/import/counterparties", api.ImportCounterparties)

		// Синхронизация
		oneC.POST("/sync/payment-statuses", api.SyncPaymentStatuses)

		// Мониторинг и ошибки
		oneC.GET("/errors", api.GetIntegrationErrors)
		oneC.PUT("/errors/:id/resolve", api.ResolveError)
		oneC.GET("/status", api.GetIntegrationStatus)
	}
}

// SetupIntegrationRequest запрос на настройку интеграции
type SetupIntegrationRequest struct {
	BaseURL           string `json:"base_url" binding:"required"`
	Username          string `json:"username" binding:"required"`
	Password          string `json:"password" binding:"required"`
	Database          string `json:"database" binding:"required"`
	APIVersion        string `json:"api_version"`
	OrganizationCode  string `json:"organization_code" binding:"required"`
	BankAccountCode   string `json:"bank_account_code" binding:"required"`
	PaymentTypeCode   string `json:"payment_type_code" binding:"required"`
	ContractTypeCode  string `json:"contract_type_code" binding:"required"`
	CurrencyCode      string `json:"currency_code"`
	AutoExportEnabled bool   `json:"auto_export_enabled"`
	AutoImportEnabled bool   `json:"auto_import_enabled"`
	SyncInterval      int    `json:"sync_interval"`
}

// SetupIntegration настраивает интеграцию с 1С
func (api *OneCIntegrationAPI) SetupIntegration(c *gin.Context) {
	companyID := GetCompanyID(c)

	var req SetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Устанавливаем значения по умолчанию
	if req.APIVersion == "" {
		req.APIVersion = "v1"
	}
	if req.CurrencyCode == "" {
		req.CurrencyCode = "RUB"
	}
	if req.SyncInterval == 0 {
		req.SyncInterval = 60 // 1 час
	}

	// Создаем конфигурацию
	config := services.OneCIntegrationConfig{
		CompanyID:         companyID,
		BaseURL:           req.BaseURL,
		Username:          req.Username,
		Password:          req.Password,
		Database:          req.Database,
		APIVersion:        req.APIVersion,
		OrganizationCode:  req.OrganizationCode,
		BankAccountCode:   req.BankAccountCode,
		PaymentTypeCode:   req.PaymentTypeCode,
		ContractTypeCode:  req.ContractTypeCode,
		CurrencyCode:      req.CurrencyCode,
		AutoExportEnabled: req.AutoExportEnabled,
		AutoImportEnabled: req.AutoImportEnabled,
		SyncInterval:      req.SyncInterval,
	}

	// Сериализуем конфигурацию
	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Проверяем, есть ли уже интеграция
	var existingIntegration models.Integration
	err = api.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&existingIntegration).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую интеграцию
		integration := models.Integration{
			CompanyID:       companyID,
			IntegrationType: "1c",
			Name:            "Интеграция с 1С",
			Description:     "Интеграция с системой 1С для обмена данными о платежах и контрагентах",
			Settings:        string(configJSON),
			IsActive:        true,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		}

		if err := api.db.Create(&integration).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка создания интеграции"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":        "Интеграция с 1С успешно настроена",
			"integration_id": integration.ID,
		})
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка проверки существующей интеграции"})
		return
	} else {
		c.JSON(http.StatusConflict, gin.H{"error": "Интеграция с 1С уже настроена. Используйте PUT для обновления"})
	}
}

// UpdateIntegration обновляет настройки интеграции с 1С
func (api *OneCIntegrationAPI) UpdateIntegration(c *gin.Context) {
	companyID := GetCompanyID(c)

	var req SetupIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Находим существующую интеграцию
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с 1С не найдена"})
		return
	}

	// Обновляем конфигурацию
	config := services.OneCIntegrationConfig{
		CompanyID:         companyID,
		BaseURL:           req.BaseURL,
		Username:          req.Username,
		Password:          req.Password,
		Database:          req.Database,
		APIVersion:        req.APIVersion,
		OrganizationCode:  req.OrganizationCode,
		BankAccountCode:   req.BankAccountCode,
		PaymentTypeCode:   req.PaymentTypeCode,
		ContractTypeCode:  req.ContractTypeCode,
		CurrencyCode:      req.CurrencyCode,
		AutoExportEnabled: req.AutoExportEnabled,
		AutoImportEnabled: req.AutoImportEnabled,
		SyncInterval:      req.SyncInterval,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка сериализации конфигурации"})
		return
	}

	// Обновляем интеграцию
	integration.Settings = string(configJSON)
	integration.UpdatedAt = time.Now()

	if err := api.db.Save(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка обновления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Настройки интеграции с 1С обновлены"})
}

// GetIntegrationConfig получает конфигурацию интеграции
func (api *OneCIntegrationAPI) GetIntegrationConfig(c *gin.Context) {
	companyID := GetCompanyID(c)

	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Интеграция с 1С не настроена"})
		return
	}

	var config services.OneCIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка парсинга конфигурации"})
		return
	}

	// Скрываем пароль в ответе
	config.Password = "***"

	c.JSON(http.StatusOK, gin.H{
		"integration": integration,
		"config":      config,
	})
}

// DeleteIntegration удаляет интеграцию с 1С
func (api *OneCIntegrationAPI) DeleteIntegration(c *gin.Context) {
	companyID := GetCompanyID(c)

	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").Delete(&models.Integration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка удаления интеграции"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Интеграция с 1С удалена"})
}

// TestConnection тестирует подключение к 1С
func (api *OneCIntegrationAPI) TestConnection(c *gin.Context) {
	companyID := GetCompanyID(c)

	if err := api.oneCIntegrationService.TestConnection(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":     "Тест подключения не пройден",
			"details":   err.Error(),
			"connected": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Подключение к 1С успешно",
		"connected": true,
	})
}

// ExportPaymentRegistryRequest запрос на экспорт реестра платежей
type ExportPaymentRegistryRequest struct {
	InvoiceIDs     []uint  `json:"invoice_ids"`
	RegistryNumber string  `json:"registry_number"`
	StartDate      *string `json:"start_date"`
	EndDate        *string `json:"end_date"`
}

// ExportPaymentRegistry экспортирует реестр платежей в 1С
func (api *OneCIntegrationAPI) ExportPaymentRegistry(c *gin.Context) {
	companyID := GetCompanyID(c)

	var req ExportPaymentRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат данных: " + err.Error()})
		return
	}

	// Получаем счета для экспорта
	var invoices []models.Invoice
	query := api.db.Where("company_id = ? AND status = 'paid'", companyID)

	if len(req.InvoiceIDs) > 0 {
		query = query.Where("id IN ?", req.InvoiceIDs)
	} else if req.StartDate != nil && req.EndDate != nil {
		startDate, err := time.Parse("2006-01-02", *req.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты начала"})
			return
		}
		endDate, err := time.Parse("2006-01-02", *req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный формат даты окончания"})
			return
		}
		query = query.Where("paid_at BETWEEN ? AND ?", startDate, endDate)
	}

	if err := query.Find(&invoices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка получения счетов"})
		return
	}

	if len(invoices) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Нет оплаченных счетов для экспорта"})
		return
	}

	// Генерируем номер реестра если не указан
	registryNumber := req.RegistryNumber
	if registryNumber == "" {
		registryNumber = fmt.Sprintf("REG-%d-%s", companyID, time.Now().Format("20060102-150405"))
	}

	// Экспортируем реестр
	if err := api.oneCIntegrationService.ExportPaymentRegistry(c.Request.Context(), companyID, invoices, registryNumber); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка экспорта реестра платежей",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Реестр платежей успешно экспортирован в 1С",
		"registry_number": registryNumber,
		"invoices_count":  len(invoices),
	})
}

// ScheduleAutoExport планирует автоматический экспорт
func (api *OneCIntegrationAPI) ScheduleAutoExport(c *gin.Context) {
	companyID := GetCompanyID(c)

	if err := api.oneCIntegrationService.ScheduleAutoExport(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка планирования автоэкспорта",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Автоэкспорт успешно запланирован"})
}

// ImportCounterparties импортирует контрагентов из 1С
func (api *OneCIntegrationAPI) ImportCounterparties(c *gin.Context) {
	companyID := GetCompanyID(c)

	if err := api.oneCIntegrationService.ImportCounterparties(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка импорта контрагентов",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Контрагенты успешно импортированы из 1С"})
}

// SyncPaymentStatuses синхронизирует статусы платежей
func (api *OneCIntegrationAPI) SyncPaymentStatuses(c *gin.Context) {
	companyID := GetCompanyID(c)

	if err := api.oneCIntegrationService.SyncPaymentStatuses(c.Request.Context(), companyID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка синхронизации статусов платежей",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Статусы платежей успешно синхронизированы"})
}

// GetIntegrationErrors получает список ошибок интеграции
func (api *OneCIntegrationAPI) GetIntegrationErrors(c *gin.Context) {
	companyID := GetCompanyID(c)

	resolved := c.Query("resolved") == "true"

	errors, err := api.oneCIntegrationService.GetIntegrationErrors(c.Request.Context(), companyID, resolved)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка получения списка ошибок",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"errors": errors,
		"count":  len(errors),
	})
}

// ResolveError помечает ошибку как решенную
func (api *OneCIntegrationAPI) ResolveError(c *gin.Context) {
	errorIDStr := c.Param("id")
	errorID, err := strconv.ParseUint(errorIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Неверный ID ошибки"})
		return
	}

	if err := api.oneCIntegrationService.ResolveError(c.Request.Context(), uint(errorID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Ошибка разрешения ошибки",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Ошибка помечена как решенная"})
}

// GetIntegrationStatus получает статус интеграции
func (api *OneCIntegrationAPI) GetIntegrationStatus(c *gin.Context) {
	companyID := GetCompanyID(c)

	// Проверяем наличие интеграции
	var integration models.Integration
	if err := api.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"configured":    false,
			"active":        false,
			"last_sync":     nil,
			"errors_count":  0,
			"connection_ok": false,
		})
		return
	}

	// Тестируем подключение
	connectionOK := false
	if err := api.oneCIntegrationService.TestConnection(c.Request.Context(), companyID); err == nil {
		connectionOK = true
	}

	// Получаем количество нерешенных ошибок
	errors, _ := api.oneCIntegrationService.GetIntegrationErrors(c.Request.Context(), companyID, false)
	errorsCount := len(errors)

	c.JSON(http.StatusOK, gin.H{
		"configured":    true,
		"active":        integration.IsActive,
		"last_sync":     integration.LastSyncAt,
		"errors_count":  errorsCount,
		"connection_ok": connectionOK,
		"created_at":    integration.CreatedAt,
		"updated_at":    integration.UpdatedAt,
	})
}
