package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// OneCIntegrationService сервис для интеграции с 1С
type OneCIntegrationService struct {
	db           *gorm.DB
	oneCClient   *OneCClient
	logger       *log.Logger
	cacheService *CacheService
}

// OneCIntegrationConfig конфигурация интеграции с 1С
type OneCIntegrationConfig struct {
	CompanyID         uint   `json:"company_id"`
	BaseURL           string `json:"base_url"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	Database          string `json:"database"`
	APIVersion        string `json:"api_version"`
	OrganizationCode  string `json:"organization_code"`  // Код организации в 1С
	BankAccountCode   string `json:"bank_account_code"`  // Код банковского счета
	PaymentTypeCode   string `json:"payment_type_code"`  // Код типа платежа
	ContractTypeCode  string `json:"contract_type_code"` // Код типа договора
	CurrencyCode      string `json:"currency_code"`      // Код валюты (RUB)
	AutoExportEnabled bool   `json:"auto_export_enabled"`
	AutoImportEnabled bool   `json:"auto_import_enabled"`
	SyncInterval      int    `json:"sync_interval"` // Интервал синхронизации в минутах
}

// OneCIntegrationError ошибка интеграции с 1С
type OneCIntegrationError struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	CompanyID    uint       `json:"company_id" gorm:"not null;index"`
	Operation    string     `json:"operation" gorm:"not null;type:varchar(50)"` // export_payment, import_counterparty, sync_status
	EntityType   string     `json:"entity_type" gorm:"type:varchar(50)"`        // invoice, payment, counterparty
	EntityID     string     `json:"entity_id" gorm:"type:varchar(50)"`
	ErrorCode    string     `json:"error_code" gorm:"type:varchar(50)"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`
	RequestData  string     `json:"request_data" gorm:"type:jsonb"`
	ResponseData string     `json:"response_data" gorm:"type:jsonb"`
	Resolved     bool       `json:"resolved" gorm:"default:false"`
	ResolvedAt   *time.Time `json:"resolved_at"`
}

// TableName задает имя таблицы для модели OneCIntegrationError
func (OneCIntegrationError) TableName() string {
	return "1c_integration_errors"
}

// NewOneCIntegrationService создает новый сервис интеграции с 1С
func NewOneCIntegrationService(db *gorm.DB, oneCClient *OneCClient, cacheService *CacheService, logger *log.Logger) *OneCIntegrationService {
	return &OneCIntegrationService{
		db:           db,
		oneCClient:   oneCClient,
		cacheService: cacheService,
		logger:       logger,
	}
}

// SetOneCClient устанавливает клиент 1С (используется в тестах)
func (s *OneCIntegrationService) SetOneCClient(client interface{}) {
	if oneCClient, ok := client.(*OneCClient); ok {
		s.oneCClient = oneCClient
	}
}

// GetCredentials получает учетные данные для 1С из кэша или БД
func (s *OneCIntegrationService) GetCredentials(ctx context.Context, companyID uint) (*OneCCredentials, error) {
	cacheKey := fmt.Sprintf("1c_credentials_%d", companyID)

	// Пытаемся получить из кэша
	if s.cacheService != nil {
		if cachedData, err := s.cacheService.Get(ctx, cacheKey); err == nil {
			var config OneCIntegrationConfig
			if err := json.Unmarshal([]byte(cachedData), &config); err == nil {
				return &OneCCredentials{
					BaseURL:    config.BaseURL,
					Username:   config.Username,
					Password:   config.Password,
					Database:   config.Database,
					APIVersion: config.APIVersion,
				}, nil
			}
		}
	}

	// Получаем из БД
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с 1С не настроена для компании %d: %w", companyID, err)
	}

	// Расшифровываем учетные данные
	var config OneCIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return nil, fmt.Errorf("ошибка расшифровки настроек 1С: %w", err)
	}

	credentials := &OneCCredentials{
		BaseURL:    config.BaseURL,
		Username:   config.Username,
		Password:   config.Password,
		Database:   config.Database,
		APIVersion: config.APIVersion,
	}

	// Кэшируем на 1 час
	if s.cacheService != nil {
		configData, _ := json.Marshal(config)
		s.cacheService.Set(ctx, cacheKey, string(configData), time.Hour)
	}

	return credentials, nil
}

// ExportPaymentRegistry экспортирует реестр платежей в 1С
func (s *OneCIntegrationService) ExportPaymentRegistry(ctx context.Context, companyID uint, invoices []models.Invoice, registryNumber string) error {
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return err
	}

	// Получаем конфигурацию
	config, err := s.getConfig(ctx, companyID)
	if err != nil {
		return err
	}

	// Формируем реестр платежей
	registry := &OneCPaymentRegistry{
		RegistryNumber: registryNumber,
		RegistryDate:   time.Now(),
		Organization:   config.OrganizationCode,
		BankAccount:    config.BankAccountCode,
		Status:         "pending",
	}

	var totalAmount float64
	var payments []OneCPayment

	for _, invoice := range invoices {
		if invoice.Status == "paid" {
			payment := OneCPayment{
				Number:        invoice.Number,
				Date:          *invoice.PaidAt,
				Posted:        true,
				Amount:        float64(invoice.PaidAmount.InexactFloat64()),
				Purpose:       fmt.Sprintf("Оплата по счету %s", invoice.Number),
				PaymentMethod: config.PaymentTypeCode,
				OperationType: "income",
				Currency:      config.CurrencyCode,
				ExternalID:    fmt.Sprintf("invoice_%d", invoice.ID),
				Comment:       invoice.Description,
			}

			// Если есть привязанный договор
			if invoice.ContractID != nil {
				payment.Contract = fmt.Sprintf("contract_%d", *invoice.ContractID)
			}

			payments = append(payments, payment)
			totalAmount += payment.Amount
		}
	}

	registry.Payments = payments
	registry.PaymentsCount = len(payments)
	registry.TotalAmount = totalAmount
	registry.Period = OneCPeriod{
		StartDate: time.Now().AddDate(0, -1, 0), // Прошлый месяц
		EndDate:   time.Now(),
	}

	// Экспортируем в 1С
	if err := s.oneCClient.ExportPaymentRegistry(ctx, credentials, registry); err != nil {
		// Логируем ошибку
		s.logError(ctx, companyID, "export_payment", "registry", registryNumber, "EXPORT_ERROR", err.Error(), registry, nil)
		return fmt.Errorf("ошибка экспорта реестра платежей: %w", err)
	}

	s.logger.Printf("Реестр платежей успешно экспортирован в 1С: %s (компания: %d, платежей: %d, сумма: %.2f)",
		registryNumber, companyID, len(payments), totalAmount)

	return nil
}

// ImportCounterparties импортирует контрагентов из 1С
func (s *OneCIntegrationService) ImportCounterparties(ctx context.Context, companyID uint) error {
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return err
	}

	// Получаем контрагентов из 1С
	limit := 100
	offset := 0
	totalImported := 0

	for {
		counterparties, total, err := s.oneCClient.GetCounterparties(ctx, credentials, limit, offset)
		if err != nil {
			s.logError(ctx, companyID, "import_counterparty", "counterparties", "", "IMPORT_ERROR", err.Error(), nil, nil)
			return fmt.Errorf("ошибка получения контрагентов из 1С: %w", err)
		}

		// Импортируем каждого контрагента
		for _, cp := range counterparties {
			if err := s.importSingleCounterparty(ctx, companyID, &cp); err != nil {
				s.logger.Printf("Ошибка импорта контрагента %s: %v", cp.Code, err)
				s.logError(ctx, companyID, "import_counterparty", "counterparty", cp.ID, "IMPORT_SINGLE_ERROR", err.Error(), cp, nil)
				continue
			}
			totalImported++
		}

		// Проверяем, есть ли еще данные
		if offset+limit >= total {
			break
		}
		offset += limit
	}

	s.logger.Printf("Импорт контрагентов завершен: %d из 1С в компанию %d", totalImported, companyID)
	return nil
}

// importSingleCounterparty импортирует одного контрагента
func (s *OneCIntegrationService) importSingleCounterparty(ctx context.Context, companyID uint, cp *OneCCounterparty) error {
	// Проверяем, есть ли уже такой контрагент
	var existingUser models.User
	err := s.db.Where("company_id = ? AND (email = ? OR phone = ?) AND external_id = ?",
		companyID, cp.Email, cp.Phone, cp.ID).First(&existingUser).Error

	if err == nil {
		// Обновляем существующего
		existingUser.Name = cp.Description
		existingUser.Email = cp.Email
		existingUser.Phone = cp.Phone
		existingUser.UpdatedAt = time.Now()

		if err := s.db.Save(&existingUser).Error; err != nil {
			return fmt.Errorf("ошибка обновления контрагента: %w", err)
		}

		s.logger.Printf("Контрагент обновлен: %s", cp.Description)
		return nil
	}

	// Создаем нового пользователя
	user := models.User{
		CompanyID:      companyID,
		Name:           cp.Description,
		Email:          cp.Email,
		Phone:          cp.Phone,
		UserType:       "client",
		IsActive:       cp.IsActive,
		ExternalID:     cp.ID,
		ExternalSource: "1c",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Создаем временный пароль
	user.Password = "temp_password_" + strconv.FormatInt(time.Now().Unix(), 10)

	if err := s.db.Create(&user).Error; err != nil {
		return fmt.Errorf("ошибка создания контрагента: %w", err)
	}

	s.logger.Printf("Новый контрагент создан: %s", cp.Description)
	return nil
}

// SyncPaymentStatuses синхронизирует статусы платежей с 1С
func (s *OneCIntegrationService) SyncPaymentStatuses(ctx context.Context, companyID uint) error {
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return err
	}

	// Получаем неоплаченные счета
	var invoices []models.Invoice
	if err := s.db.Where("company_id = ? AND status IN (?)", companyID, []string{"sent", "overdue"}).Find(&invoices).Error; err != nil {
		return fmt.Errorf("ошибка получения счетов: %w", err)
	}

	syncedCount := 0
	for _, invoice := range invoices {
		externalID := fmt.Sprintf("invoice_%d", invoice.ID)

		// Получаем статус платежа из 1С
		payment, err := s.oneCClient.GetPaymentStatus(ctx, credentials, externalID)
		if err != nil {
			s.logger.Printf("Ошибка получения статуса платежа для счета %s: %v", invoice.Number, err)
			continue
		}

		// Если платеж проведен в 1С, обновляем статус счета
		if payment.Posted && payment.Amount > 0 {
			invoice.Status = "paid"
			paidAt := payment.Date
			invoice.PaidAt = &paidAt
			invoice.PaidAmount = decimal.NewFromFloat(payment.Amount)
			invoice.UpdatedAt = time.Now()

			if err := s.db.Save(&invoice).Error; err != nil {
				s.logger.Printf("Ошибка обновления статуса счета %s: %v", invoice.Number, err)
				continue
			}

			s.logger.Printf("Статус счета %s обновлен на 'paid' на основе данных из 1С", invoice.Number)
			syncedCount++
		}
	}

	s.logger.Printf("Синхронизация статусов платежей завершена: %d счетов обновлено", syncedCount)
	return nil
}

// getConfig получает конфигурацию интеграции
func (s *OneCIntegrationService) getConfig(ctx context.Context, companyID uint) (*OneCIntegrationConfig, error) {
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "1c").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с 1С не настроена: %w", err)
	}

	var config OneCIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга настроек 1С: %w", err)
	}

	return &config, nil
}

// logError логирует ошибку интеграции
func (s *OneCIntegrationService) logError(ctx context.Context, companyID uint, operation, entityType, entityID, errorCode, errorMessage string, requestData, responseData interface{}) {
	var requestJSON, responseJSON string

	if requestData != nil {
		if data, err := json.Marshal(requestData); err == nil {
			requestJSON = string(data)
		}
	}

	if responseData != nil {
		if data, err := json.Marshal(responseData); err == nil {
			responseJSON = string(data)
		}
	}

	integrationError := OneCIntegrationError{
		CompanyID:    companyID,
		Operation:    operation,
		EntityType:   entityType,
		EntityID:     entityID,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
		RequestData:  requestJSON,
		ResponseData: responseJSON,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(&integrationError).Error; err != nil {
		s.logger.Printf("Ошибка сохранения лога интеграции: %v", err)
	}
}

// TestConnection тестирует подключение к 1С
func (s *OneCIntegrationService) TestConnection(ctx context.Context, companyID uint) error {
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return err
	}

	if err := s.oneCClient.IsHealthy(ctx, credentials); err != nil {
		return fmt.Errorf("тест подключения к 1С не пройден: %w", err)
	}

	s.logger.Printf("Тест подключения к 1С успешно пройден для компании %d", companyID)
	return nil
}

// GetIntegrationErrors возвращает список ошибок интеграции
func (s *OneCIntegrationService) GetIntegrationErrors(ctx context.Context, companyID uint, resolved bool) ([]OneCIntegrationError, error) {
	var errors []OneCIntegrationError
	query := s.db.Where("company_id = ?", companyID)

	if resolved {
		query = query.Where("resolved = ?", true)
	} else {
		query = query.Where("resolved = ?", false)
	}

	if err := query.Order("created_at DESC").Find(&errors).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения списка ошибок интеграции: %w", err)
	}

	return errors, nil
}

// ResolveError помечает ошибку как решенную
func (s *OneCIntegrationService) ResolveError(ctx context.Context, errorID uint) error {
	now := time.Now()
	if err := s.db.Model(&OneCIntegrationError{}).Where("id = ?", errorID).Updates(map[string]interface{}{
		"resolved":    true,
		"resolved_at": &now,
		"updated_at":  now,
	}).Error; err != nil {
		return fmt.Errorf("ошибка разрешения ошибки интеграции: %w", err)
	}

	return nil
}

// ScheduleAutoExport планирует автоматический экспорт
func (s *OneCIntegrationService) ScheduleAutoExport(ctx context.Context, companyID uint) error {
	config, err := s.getConfig(ctx, companyID)
	if err != nil {
		return err
	}

	if !config.AutoExportEnabled {
		return nil // Автоэкспорт отключен
	}

	// Получаем оплаченные счета за последний период
	var invoices []models.Invoice
	since := time.Now().AddDate(0, 0, -1) // За последний день

	if err := s.db.Where("company_id = ? AND status = 'paid' AND paid_at > ?", companyID, since).Find(&invoices).Error; err != nil {
		return fmt.Errorf("ошибка получения оплаченных счетов: %w", err)
	}

	if len(invoices) == 0 {
		return nil // Нет новых оплаченных счетов
	}

	// Генерируем номер реестра
	registryNumber := fmt.Sprintf("REG-%d-%s", companyID, time.Now().Format("20060102-150405"))

	// Экспортируем реестр
	if err := s.ExportPaymentRegistry(ctx, companyID, invoices, registryNumber); err != nil {
		return fmt.Errorf("ошибка автоэкспорта: %w", err)
	}

	s.logger.Printf("Автоэкспорт выполнен для компании %d: %d счетов", companyID, len(invoices))
	return nil
}
