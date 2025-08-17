package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// Bitrix24ClientInterface интерфейс для работы с Битрикс24 API
type Bitrix24ClientInterface interface {
	CallMethod(ctx context.Context, credentials *Bitrix24Credentials, method string, params map[string]interface{}) (*Bitrix24Response, error)
	CreateContact(ctx context.Context, credentials *Bitrix24Credentials, contact *Bitrix24Contact) (string, error)
	UpdateContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string, contact *Bitrix24Contact) error
	GetContact(ctx context.Context, credentials *Bitrix24Credentials, contactID string) (*Bitrix24Contact, error)
	GetContacts(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Contact, int, error)
	CreateDeal(ctx context.Context, credentials *Bitrix24Credentials, deal *Bitrix24Deal) (string, error)
	UpdateDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string, deal *Bitrix24Deal) error
	GetDeal(ctx context.Context, credentials *Bitrix24Credentials, dealID string) (*Bitrix24Deal, error)
	GetDeals(ctx context.Context, credentials *Bitrix24Credentials, limit int, start int) ([]Bitrix24Deal, int, error)
	IsHealthy(ctx context.Context, credentials *Bitrix24Credentials) error
}

// Bitrix24IntegrationService сервис для интеграции с Битрикс24
type Bitrix24IntegrationService struct {
	Bitrix24Client   Bitrix24ClientInterface
	credentialsCache map[uint]*Bitrix24Credentials
	cacheMutex       sync.RWMutex
	logger           *log.Logger
	syncMutex        sync.RWMutex    // Мьютекс для предотвращения циклических синхронизаций
	syncInProgress   map[string]bool // Отслеживание активных синхронизаций
}

// Bitrix24SyncMapping маппинг между локальными объектами и Битрикс24
type Bitrix24SyncMapping struct {
	ID            uint      `json:"id" gorm:"primarykey"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	TenantID      uint      `json:"tenant_id" gorm:"not null;index"`
	LocalType     string    `json:"local_type" gorm:"not null;type:varchar(50)"` // object, user, contract
	LocalID       uint      `json:"local_id" gorm:"not null;index"`
	Bitrix24Type  string    `json:"bitrix24_type" gorm:"not null;type:varchar(50)"` // contact, deal, company
	Bitrix24ID    string    `json:"bitrix24_id" gorm:"not null;type:varchar(100);index"`
	LastSyncAt    time.Time `json:"last_sync_at"`
	SyncDirection string    `json:"sync_direction" gorm:"type:varchar(20)"` // to_bitrix, from_bitrix, bidirectional
}

// TableName задает имя таблицы для модели Bitrix24SyncMapping
func (Bitrix24SyncMapping) TableName() string {
	return "bitrix24_sync_mappings"
}

// NewBitrix24IntegrationService создает новый сервис интеграции с Битрикс24
func NewBitrix24IntegrationService(logger *log.Logger) *Bitrix24IntegrationService {
	if logger == nil {
		logger = log.New(os.Stdout, "[BITRIX24] ", log.LstdFlags|log.Lshortfile)
	}

	bitrix24Client := NewBitrix24Client(logger)

	return &Bitrix24IntegrationService{
		Bitrix24Client:   bitrix24Client,
		credentialsCache: make(map[uint]*Bitrix24Credentials),
		logger:           logger,
		syncInProgress:   make(map[string]bool),
	}
}

// GetTenantCredentials получает учетные данные компании для Битрикс24
func (s *Bitrix24IntegrationService) GetTenantCredentials(ctx context.Context, tenantID uint) (*Bitrix24Credentials, error) {
	// Проверяем кэш
	s.cacheMutex.RLock()
	if creds, exists := s.credentialsCache[tenantID]; exists {
		s.cacheMutex.RUnlock()
		return creds, nil
	}
	s.cacheMutex.RUnlock()

	// Получаем данные компании из БД
	db := database.GetDB()
	var company models.Company
	if err := db.First(&company, tenantID).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения данных компании %d: %w", tenantID, err)
	}

	if company.Bitrix24WebhookURL == "" {
		return nil, fmt.Errorf("компания %d не настроена для интеграции с Битрикс24", tenantID)
	}

	credentials := &Bitrix24Credentials{
		WebhookURL: company.Bitrix24WebhookURL,
	}

	// Сохраняем в кэш
	s.cacheMutex.Lock()
	s.credentialsCache[tenantID] = credentials
	s.cacheMutex.Unlock()

	s.logger.Printf("Получены учетные данные для компании %d", tenantID)
	return credentials, nil
}

// SyncObjectToBitrix24 синхронизирует объект CRM в Битрикс24 как сделку
func (s *Bitrix24IntegrationService) SyncObjectToBitrix24(ctx context.Context, tenantID uint, object *models.Object) error {
	// Проверяем, не выполняется ли уже синхронизация для этого объекта
	syncKey := fmt.Sprintf("object_%d_%d", tenantID, object.ID)
	s.syncMutex.Lock()
	if s.syncInProgress[syncKey] {
		s.syncMutex.Unlock()
		s.logger.Printf("Синхронизация объекта %d уже выполняется, пропускаем", object.ID)
		return nil
	}
	s.syncInProgress[syncKey] = true
	s.syncMutex.Unlock()

	defer func() {
		s.syncMutex.Lock()
		delete(s.syncInProgress, syncKey)
		s.syncMutex.Unlock()
	}()

	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return s.createIntegrationError(tenantID, "sync_object", object.ID, "",
			"Не удалось получить учетные данные Битрикс24", true, err)
	}

	// Проверяем, есть ли уже маппинг для этого объекта
	db := database.GetDB()
	var mapping Bitrix24SyncMapping
	err = db.Where("tenant_id = ? AND local_type = ? AND local_id = ?", tenantID, "object", object.ID).First(&mapping).Error

	if err == nil {
		// Обновляем существующую сделку
		return s.updateExistingDeal(ctx, credentials, &mapping, object, tenantID)
	}

	// Создаем новую сделку
	return s.createNewDeal(ctx, credentials, object, tenantID)
}

// createNewDeal создает новую сделку в Битрикс24
func (s *Bitrix24IntegrationService) createNewDeal(ctx context.Context, credentials *Bitrix24Credentials, object *models.Object, tenantID uint) error {
	// Подготавливаем данные сделки
	deal := &Bitrix24Deal{
		Title:    fmt.Sprintf("Объект: %s", object.Name),
		Comments: fmt.Sprintf("IMEI: %s\nТип: %s\nОписание: %s", object.IMEI, object.Type, object.Description),
	}

	// Если объект привязан к договору, добавляем информацию
	if object.ContractID != 0 {
		tenantDB := database.GetTenantDBByID(tenantID)
		if tenantDB != nil {
			var contract models.Contract
			if err := tenantDB.First(&contract, object.ContractID).Error; err == nil {
				deal.Title = fmt.Sprintf("Договор %s - Объект: %s", contract.Number, object.Name)
				deal.Comments += fmt.Sprintf("\nДоговор: %s\nКлиент: %s",
					contract.Number, contract.ClientName)

				// Устанавливаем даты сделки на основе договора
				if !contract.StartDate.IsZero() {
					deal.BeginDate = contract.StartDate
				}
				if !contract.EndDate.IsZero() {
					deal.CloseDate = contract.EndDate
				}
			}
		}
	}

	// Создаем сделку в Битрикс24
	dealID, err := s.Bitrix24Client.CreateDeal(ctx, credentials, deal)
	if err != nil {
		return s.createIntegrationError(tenantID, "create_deal", object.ID, "",
			"Ошибка создания сделки в Битрикс24", true, err)
	}

	// Сохраняем маппинг
	mapping := &Bitrix24SyncMapping{
		TenantID:      tenantID,
		LocalType:     "object",
		LocalID:       object.ID,
		Bitrix24Type:  "deal",
		Bitrix24ID:    dealID,
		LastSyncAt:    time.Now(),
		SyncDirection: "to_bitrix",
	}

	db := database.GetDB()
	if err := db.Create(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось сохранить маппинг для объекта %d: %v", object.ID, err)
	}

	s.logger.Printf("Объект %d успешно синхронизирован с Битрикс24 как сделка %s", object.ID, dealID)
	return nil
}

// updateExistingDeal обновляет существующую сделку в Битрикс24
func (s *Bitrix24IntegrationService) updateExistingDeal(ctx context.Context, credentials *Bitrix24Credentials, mapping *Bitrix24SyncMapping, object *models.Object, tenantID uint) error {
	// Подготавливаем данные для обновления
	deal := &Bitrix24Deal{
		Title:    fmt.Sprintf("Объект: %s", object.Name),
		Comments: fmt.Sprintf("IMEI: %s\nТип: %s\nОписание: %s", object.IMEI, object.Type, object.Description),
	}

	// Если объект привязан к договору, добавляем информацию
	if object.ContractID != 0 {
		tenantDB := database.GetTenantDBByID(tenantID)
		if tenantDB != nil {
			var contract models.Contract
			if err := tenantDB.First(&contract, object.ContractID).Error; err == nil {
				deal.Title = fmt.Sprintf("Договор %s - Объект: %s", contract.Number, object.Name)
				deal.Comments += fmt.Sprintf("\nДоговор: %s\nКлиент: %s",
					contract.Number, contract.ClientName)

				// Устанавливаем даты сделки на основе договора
				if !contract.StartDate.IsZero() {
					deal.BeginDate = contract.StartDate
				}
				if !contract.EndDate.IsZero() {
					deal.CloseDate = contract.EndDate
				}
			}
		}
	}

	// Обновляем сделку в Битрикс24
	err := s.Bitrix24Client.UpdateDeal(ctx, credentials, mapping.Bitrix24ID, deal)
	if err != nil {
		return s.createIntegrationError(tenantID, "update_deal", object.ID, mapping.Bitrix24ID,
			"Ошибка обновления сделки в Битрикс24", true, err)
	}

	// Обновляем время последней синхронизации
	mapping.LastSyncAt = time.Now()
	db := database.GetDB()
	if err := db.Save(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось обновить маппинг для объекта %d: %v", object.ID, err)
	}

	s.logger.Printf("Объект %d успешно обновлен в Битрикс24 (сделка %s)", object.ID, mapping.Bitrix24ID)
	return nil
}

// SyncUserToBitrix24 синхронизирует пользователя в Битрикс24 как контакт
func (s *Bitrix24IntegrationService) SyncUserToBitrix24(ctx context.Context, tenantID uint, user *models.User) error {
	// Проверяем, не выполняется ли уже синхронизация для этого пользователя
	syncKey := fmt.Sprintf("user_%d_%d", tenantID, user.ID)
	s.syncMutex.Lock()
	if s.syncInProgress[syncKey] {
		s.syncMutex.Unlock()
		s.logger.Printf("Синхронизация пользователя %d уже выполняется, пропускаем", user.ID)
		return nil
	}
	s.syncInProgress[syncKey] = true
	s.syncMutex.Unlock()

	defer func() {
		s.syncMutex.Lock()
		delete(s.syncInProgress, syncKey)
		s.syncMutex.Unlock()
	}()

	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return s.createIntegrationError(tenantID, "sync_user", user.ID, "",
			"Не удалось получить учетные данные Битрикс24", true, err)
	}

	// Проверяем, есть ли уже маппинг для этого пользователя
	db := database.GetDB()
	var mapping Bitrix24SyncMapping
	err = db.Where("tenant_id = ? AND local_type = ? AND local_id = ?", tenantID, "user", user.ID).First(&mapping).Error

	if err == nil {
		// Обновляем существующий контакт
		return s.updateExistingContact(ctx, credentials, &mapping, user, tenantID)
	}

	// Создаем новый контакт
	return s.createNewContact(ctx, credentials, user, tenantID)
}

// createNewContact создает новый контакт в Битрикс24
func (s *Bitrix24IntegrationService) createNewContact(ctx context.Context, credentials *Bitrix24Credentials, user *models.User, tenantID uint) error {
	// Получаем имя и фамилию пользователя
	firstName := user.FirstName
	lastName := user.LastName

	// Если имя пустое, используем username
	if firstName == "" {
		firstName = user.Username
	}

	// Подготавливаем данные контакта
	contact := &Bitrix24Contact{
		Name:     firstName,
		LastName: lastName,
		Email:    user.Email,
		Phone:    user.TelegramID, // Используем TelegramID как контактный номер
		Comments: fmt.Sprintf("Пользователь CRM\nUsername: %s\nСоздан: %s",
			user.Username, user.CreatedAt.Format("2006-01-02 15:04:05")),
	}

	// Создаем контакт в Битрикс24
	contactID, err := s.Bitrix24Client.CreateContact(ctx, credentials, contact)
	if err != nil {
		return s.createIntegrationError(tenantID, "create_contact", user.ID, "",
			"Ошибка создания контакта в Битрикс24", true, err)
	}

	// Сохраняем маппинг
	mapping := &Bitrix24SyncMapping{
		TenantID:      tenantID,
		LocalType:     "user",
		LocalID:       user.ID,
		Bitrix24Type:  "contact",
		Bitrix24ID:    contactID,
		LastSyncAt:    time.Now(),
		SyncDirection: "to_bitrix",
	}

	db := database.GetDB()
	if err := db.Create(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось сохранить маппинг для пользователя %d: %v", user.ID, err)
	}

	s.logger.Printf("Пользователь %d успешно синхронизирован с Битрикс24 как контакт %s", user.ID, contactID)
	return nil
}

// updateExistingContact обновляет существующий контакт в Битрикс24
func (s *Bitrix24IntegrationService) updateExistingContact(ctx context.Context, credentials *Bitrix24Credentials, mapping *Bitrix24SyncMapping, user *models.User, tenantID uint) error {
	// Получаем имя и фамилию пользователя
	firstName := user.FirstName
	lastName := user.LastName

	// Если имя пустое, используем username
	if firstName == "" {
		firstName = user.Username
	}

	// Подготавливаем данные для обновления
	contact := &Bitrix24Contact{
		Name:     firstName,
		LastName: lastName,
		Email:    user.Email,
		Phone:    user.TelegramID, // Используем TelegramID как контактный номер
		Comments: fmt.Sprintf("Пользователь CRM\nUsername: %s\nОбновлен: %s",
			user.Username, time.Now().Format("2006-01-02 15:04:05")),
	}

	// Обновляем контакт в Битрикс24
	err := s.Bitrix24Client.UpdateContact(ctx, credentials, mapping.Bitrix24ID, contact)
	if err != nil {
		return s.createIntegrationError(tenantID, "update_contact", user.ID, mapping.Bitrix24ID,
			"Ошибка обновления контакта в Битрикс24", true, err)
	}

	// Обновляем время последней синхронизации
	mapping.LastSyncAt = time.Now()
	db := database.GetDB()
	if err := db.Save(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось обновить маппинг для пользователя %d: %v", user.ID, err)
	}

	s.logger.Printf("Пользователь %d успешно обновлен в Битрикс24 (контакт %s)", user.ID, mapping.Bitrix24ID)
	return nil
}

// SyncFromBitrix24 синхронизирует данные из Битрикс24 в локальную систему
func (s *Bitrix24IntegrationService) SyncFromBitrix24(ctx context.Context, tenantID uint) error {
	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных данных: %w", err)
	}

	// Синхронизируем контакты
	if err := s.syncContactsFromBitrix24(ctx, credentials, tenantID); err != nil {
		s.logger.Printf("Ошибка синхронизации контактов из Битрикс24: %v", err)
	}

	// Синхронизируем сделки
	if err := s.syncDealsFromBitrix24(ctx, credentials, tenantID); err != nil {
		s.logger.Printf("Ошибка синхронизации сделок из Битрикс24: %v", err)
	}

	return nil
}

// syncContactsFromBitrix24 синхронизирует контакты из Битрикс24
func (s *Bitrix24IntegrationService) syncContactsFromBitrix24(ctx context.Context, credentials *Bitrix24Credentials, tenantID uint) error {
	const batchSize = 50
	start := 0

	for {
		contacts, total, err := s.Bitrix24Client.GetContacts(ctx, credentials, batchSize, start)
		if err != nil {
			return fmt.Errorf("ошибка получения контактов: %w", err)
		}

		if len(contacts) == 0 {
			break
		}

		// Обрабатываем каждый контакт
		for _, contact := range contacts {
			if err := s.processContactFromBitrix24(ctx, &contact, tenantID); err != nil {
				s.logger.Printf("Ошибка обработки контакта %s: %v", contact.ID, err)
				continue
			}
		}

		start += batchSize
		if start >= total {
			break
		}

		// Небольшая пауза между запросами
		time.Sleep(100 * time.Millisecond)
	}

	s.logger.Printf("Синхронизация контактов завершена для компании %d", tenantID)
	return nil
}

// processContactFromBitrix24 обрабатывает контакт из Битрикс24
func (s *Bitrix24IntegrationService) processContactFromBitrix24(ctx context.Context, contact *Bitrix24Contact, tenantID uint) error {
	// Проверяем, есть ли уже маппинг для этого контакта
	db := database.GetDB()
	var mapping Bitrix24SyncMapping
	err := db.Where("tenant_id = ? AND bitrix24_type = ? AND bitrix24_id = ?",
		tenantID, "contact", contact.ID).First(&mapping).Error

	if err == nil {
		// Обновляем существующего пользователя
		return s.updateUserFromBitrix24Contact(ctx, &mapping, contact, tenantID)
	}

	// Создаем нового пользователя (если это необходимо)
	// В данной реализации мы только логируем новые контакты
	s.logger.Printf("Найден новый контакт в Битрикс24: %s (%s %s)",
		contact.ID, contact.Name, contact.LastName)

	return nil
}

// updateUserFromBitrix24Contact обновляет пользователя на основе данных из Битрикс24
func (s *Bitrix24IntegrationService) updateUserFromBitrix24Contact(ctx context.Context, mapping *Bitrix24SyncMapping, contact *Bitrix24Contact, tenantID uint) error {
	tenantDB := database.GetTenantDBByID(tenantID)
	if tenantDB == nil {
		return fmt.Errorf("не удалось получить БД для компании %d", tenantID)
	}

	var user models.User
	if err := tenantDB.First(&user, mapping.LocalID).Error; err != nil {
		return fmt.Errorf("пользователь %d не найден: %w", mapping.LocalID, err)
	}

	// Обновляем данные пользователя
	if contact.Name != "" {
		user.FirstName = contact.Name
	}
	if contact.LastName != "" {
		user.LastName = contact.LastName
	}
	if contact.Email != "" {
		user.Email = contact.Email
	}
	if contact.Phone != "" {
		user.TelegramID = contact.Phone // Сохраняем телефон в TelegramID
	}

	if err := tenantDB.Save(&user).Error; err != nil {
		return fmt.Errorf("ошибка обновления пользователя: %w", err)
	}

	// Обновляем время последней синхронизации
	mapping.LastSyncAt = time.Now()
	db := database.GetDB()
	if err := db.Save(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось обновить маппинг: %v", err)
	}

	s.logger.Printf("Пользователь %d обновлен из Битрикс24 (контакт %s)", user.ID, contact.ID)
	return nil
}

// syncDealsFromBitrix24 синхронизирует сделки из Битрикс24
func (s *Bitrix24IntegrationService) syncDealsFromBitrix24(ctx context.Context, credentials *Bitrix24Credentials, tenantID uint) error {
	const batchSize = 50
	start := 0

	for {
		deals, total, err := s.Bitrix24Client.GetDeals(ctx, credentials, batchSize, start)
		if err != nil {
			return fmt.Errorf("ошибка получения сделок: %w", err)
		}

		if len(deals) == 0 {
			break
		}

		// Обрабатываем каждую сделку
		for _, deal := range deals {
			if err := s.processDealFromBitrix24(ctx, &deal, tenantID); err != nil {
				s.logger.Printf("Ошибка обработки сделки %s: %v", deal.ID, err)
				continue
			}
		}

		start += batchSize
		if start >= total {
			break
		}

		// Небольшая пауза между запросами
		time.Sleep(100 * time.Millisecond)
	}

	s.logger.Printf("Синхронизация сделок завершена для компании %d", tenantID)
	return nil
}

// processDealFromBitrix24 обрабатывает сделку из Битрикс24
func (s *Bitrix24IntegrationService) processDealFromBitrix24(ctx context.Context, deal *Bitrix24Deal, tenantID uint) error {
	// Проверяем, есть ли уже маппинг для этой сделки
	db := database.GetDB()
	var mapping Bitrix24SyncMapping
	err := db.Where("tenant_id = ? AND bitrix24_type = ? AND bitrix24_id = ?",
		tenantID, "deal", deal.ID).First(&mapping).Error

	if err == nil {
		// Обновляем существующий объект
		return s.updateObjectFromBitrix24Deal(ctx, &mapping, deal, tenantID)
	}

	// Логируем новую сделку (создание объектов из сделок может быть реализовано позже)
	s.logger.Printf("Найдена новая сделка в Битрикс24: %s (%s)", deal.ID, deal.Title)

	return nil
}

// updateObjectFromBitrix24Deal обновляет объект на основе данных из Битрикс24
func (s *Bitrix24IntegrationService) updateObjectFromBitrix24Deal(ctx context.Context, mapping *Bitrix24SyncMapping, deal *Bitrix24Deal, tenantID uint) error {
	tenantDB := database.GetTenantDBByID(tenantID)
	if tenantDB == nil {
		return fmt.Errorf("не удалось получить БД для компании %d", tenantID)
	}

	var object models.Object
	if err := tenantDB.First(&object, mapping.LocalID).Error; err != nil {
		return fmt.Errorf("объект %d не найден: %w", mapping.LocalID, err)
	}

	// Обновляем описание объекта на основе комментариев сделки
	if deal.Comments != "" && !strings.Contains(object.Description, "Обновлено из Битрикс24") {
		object.Description += fmt.Sprintf("\n\nОбновлено из Битрикс24: %s", deal.Comments)
	}

	if err := tenantDB.Save(&object).Error; err != nil {
		return fmt.Errorf("ошибка обновления объекта: %w", err)
	}

	// Обновляем время последней синхронизации
	mapping.LastSyncAt = time.Now()
	db := database.GetDB()
	if err := db.Save(mapping).Error; err != nil {
		s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось обновить маппинг: %v", err)
	}

	s.logger.Printf("Объект %d обновлен из Битрикс24 (сделка %s)", object.ID, deal.ID)
	return nil
}

// CheckHealth проверяет доступность Битрикс24 API
func (s *Bitrix24IntegrationService) CheckHealth(ctx context.Context, tenantID uint) error {
	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных данных: %w", err)
	}

	return s.Bitrix24Client.IsHealthy(ctx, credentials)
}

// SetupCompanyCredentials настраивает учетные данные компании для Битрикс24
func (s *Bitrix24IntegrationService) SetupCompanyCredentials(ctx context.Context, tenantID uint, webhookURL string) error {
	// Проверяем работоспособность вебхука
	credentials := &Bitrix24Credentials{
		WebhookURL: webhookURL,
	}

	if err := s.Bitrix24Client.IsHealthy(ctx, credentials); err != nil {
		return fmt.Errorf("ошибка проверки вебхука Битрикс24: %w", err)
	}

	// Обновляем данные компании в БД
	db := database.GetDB()
	result := db.Model(&models.Company{}).Where("id = ?", tenantID).Update("bitrix24_webhook_url", webhookURL)

	if result.Error != nil {
		return fmt.Errorf("ошибка сохранения вебхука: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("компания %d не найдена", tenantID)
	}

	// Очищаем кэш для этой компании
	s.ClearCredentialsCache(tenantID)

	s.logger.Printf("Вебхук Битрикс24 для компании %d успешно настроен", tenantID)
	return nil
}

// ClearCredentialsCache очищает кэш учетных данных
func (s *Bitrix24IntegrationService) ClearCredentialsCache(tenantID uint) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	if tenantID == 0 {
		// Очищаем весь кэш
		s.credentialsCache = make(map[uint]*Bitrix24Credentials)
		s.logger.Println("Очищен весь кэш учетных данных Битрикс24")
	} else {
		// Очищаем кэш для конкретной компании
		delete(s.credentialsCache, tenantID)
		s.logger.Printf("Очищен кэш учетных данных Битрикс24 для компании %d", tenantID)
	}
}

// GetSyncMappings возвращает маппинги синхронизации для компании
func (s *Bitrix24IntegrationService) GetSyncMappings(tenantID uint, limit int, offset int) ([]Bitrix24SyncMapping, int64, error) {
	db := database.GetDB()
	var mappings []Bitrix24SyncMapping
	var total int64

	// Подсчитываем общее количество
	if err := db.Model(&Bitrix24SyncMapping{}).Where("tenant_id = ?", tenantID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчета маппингов: %w", err)
	}

	// Получаем маппинги с пагинацией
	query := db.Where("tenant_id = ?", tenantID).Order("updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit).Offset(offset)
	}

	if err := query.Find(&mappings).Error; err != nil {
		return nil, 0, fmt.Errorf("ошибка получения маппингов: %w", err)
	}

	return mappings, total, nil
}

// createIntegrationError создает ошибку интеграции для Битрикс24
func (s *Bitrix24IntegrationService) createIntegrationError(tenantID uint, operation string, objectID uint, externalID string, message string, retryable bool, originalErr error) error {
	// Создаем запись об ошибке в БД
	dbError := &models.IntegrationError{
		TenantID:     tenantID,
		Operation:    operation,
		ObjectID:     objectID,
		ExternalID:   externalID,
		Service:      models.IntegrationServiceBitrix24,
		ErrorMessage: message,
		Retryable:    retryable,
		Status:       models.IntegrationErrorStatusPending,
		MaxRetries:   3,
	}

	// Если есть оригинальная ошибка, сохраняем её детали
	if originalErr != nil {
		dbError.StackTrace = originalErr.Error()
	}

	// Устанавливаем время следующей попытки
	if dbError.Retryable {
		nextRetry := time.Now().Add(dbError.GetRetryDelay())
		dbError.NextRetryAt = &nextRetry
	}

	// Сохраняем в основную БД
	db := database.GetDB()
	if err := db.Create(dbError).Error; err != nil {
		s.logger.Printf("Ошибка сохранения integration_error в БД: %v", err)
	}

	// Возвращаем оригинальную ошибку
	if originalErr != nil {
		return fmt.Errorf("%s: %w", message, originalErr)
	}
	return fmt.Errorf(message)
}
