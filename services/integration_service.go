package services

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// AxetnaClientInterface интерфейс для работы с Axetna.cloud API
type AxetnaClientInterface interface {
	Authenticate(ctx context.Context, login, password string) (*TenantCredentials, error)
	CreateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error)
	UpdateObject(ctx context.Context, credentials *TenantCredentials, object *models.Object) (*AxetnaObjectResponse, error)
	DeleteObject(ctx context.Context, credentials *TenantCredentials, externalID string) error
	IsHealthy(ctx context.Context) error
}

// IntegrationService сервис для управления интеграциями с внешними системами
type IntegrationService struct {
	AxetnaClient     AxetnaClientInterface
	credentialsCache map[uint]*TenantCredentials // Кэш учетных данных по tenant_id
	cacheMutex       sync.RWMutex
	encryptionKey    []byte
	logger           *log.Logger
}

// IntegrationError ошибка интеграции с дополнительной информацией
type IntegrationError struct {
	TenantID    uint
	Operation   string
	ObjectID    uint
	ExternalID  string
	Message     string
	Retryable   bool
	Timestamp   time.Time
	OriginalErr error
}

func (e *IntegrationError) Error() string {
	return fmt.Sprintf("Ошибка интеграции [%s] для tenant %d, объект %d: %s",
		e.Operation, e.TenantID, e.ObjectID, e.Message)
}

// NewIntegrationService создает новый сервис интеграций
func NewIntegrationService(axetnaBaseURL string, logger *log.Logger) (*IntegrationService, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "[INTEGRATION] ", log.LstdFlags|log.Lshortfile)
	}

	axetnaClient := NewAxetnaClient(axetnaBaseURL, logger)

	// Получаем ключ шифрования из конфигурации
	cfg := config.GetConfig()
	encryptionKey := cfg.Axenta.EncryptionKey
	if encryptionKey == "" {
		// В продакшене это должно быть обязательно
		logger.Println("ПРЕДУПРЕЖДЕНИЕ: Не задан ENCRYPTION_KEY, используется значение по умолчанию")
		encryptionKey = "default-key-32-chars-long!!!!!"
	}

	if len(encryptionKey) != 32 {
		return nil, fmt.Errorf("ключ шифрования должен быть длиной 32 символа")
	}

	return &IntegrationService{
		AxetnaClient:     axetnaClient,
		credentialsCache: make(map[uint]*TenantCredentials),
		encryptionKey:    []byte(encryptionKey),
		logger:           logger,
	}, nil
}

// GetTenantCredentials получает учетные данные компании для Axetna.cloud
func (s *IntegrationService) GetTenantCredentials(ctx context.Context, tenantID uint) (*TenantCredentials, error) {
	// Проверяем кэш
	s.cacheMutex.RLock()
	if creds, exists := s.credentialsCache[tenantID]; exists {
		// Проверяем, не истек ли токен
		if time.Now().Before(creds.ExpiresAt.Add(-5 * time.Minute)) { // Обновляем за 5 минут до истечения
			s.cacheMutex.RUnlock()
			return creds, nil
		}
	}
	s.cacheMutex.RUnlock()

	// Получаем данные компании из БД
	db := database.GetDB()
	var company models.Company
	if err := db.First(&company, tenantID).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения данных компании %d: %w", tenantID, err)
	}

	if !company.IsValidForTenant() {
		return nil, fmt.Errorf("компания %d не настроена для интеграции с Axetna.cloud", tenantID)
	}

	// Расшифровываем пароль
	password, err := s.decryptPassword(company.AxetnaPassword)
	if err != nil {
		return nil, fmt.Errorf("ошибка расшифровки пароля для компании %d: %w", tenantID, err)
	}

	// Авторизуемся в Axetna.cloud
	credentials, err := s.AxetnaClient.Authenticate(ctx, company.AxetnaLogin, password)
	if err != nil {
		return nil, fmt.Errorf("ошибка авторизации в Axetna.cloud для компании %d: %w", tenantID, err)
	}

	// Сохраняем в кэш
	s.cacheMutex.Lock()
	s.credentialsCache[tenantID] = credentials
	s.cacheMutex.Unlock()

	s.logger.Printf("Получены учетные данные для компании %d", tenantID)
	return credentials, nil
}

// SyncObjectCreate синхронизирует создание объекта с Axetna.cloud
func (s *IntegrationService) SyncObjectCreate(ctx context.Context, tenantID uint, object *models.Object) error {
	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return s.createIntegrationError(tenantID, "create", object.ID, "",
			"Не удалось получить учетные данные", true, err)
	}

	// Создаем объект в Axetna.cloud
	response, err := s.AxetnaClient.CreateObject(ctx, credentials, object)
	if err != nil {
		return s.createIntegrationError(tenantID, "create", object.ID, "",
			"Ошибка создания объекта в Axetna.cloud", true, err)
	}

	// Обновляем ExternalID в локальной БД
	tenantDB := database.GetTenantDBByID(tenantID)
	if tenantDB != nil {
		object.ExternalID = response.ID
		if err := tenantDB.Save(object).Error; err != nil {
			s.logger.Printf("ПРЕДУПРЕЖДЕНИЕ: Не удалось сохранить ExternalID для объекта %d: %v", object.ID, err)
		}
	}

	s.logger.Printf("Объект %d успешно синхронизирован с Axetna.cloud (External ID: %s)", object.ID, response.ID)
	return nil
}

// SyncObjectUpdate синхронизирует обновление объекта с Axetna.cloud
func (s *IntegrationService) SyncObjectUpdate(ctx context.Context, tenantID uint, object *models.Object) error {
	if object.ExternalID == "" {
		return s.createIntegrationError(tenantID, "update", object.ID, "",
			"Отсутствует ExternalID для синхронизации", false, nil)
	}

	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return s.createIntegrationError(tenantID, "update", object.ID, object.ExternalID,
			"Не удалось получить учетные данные", true, err)
	}

	// Обновляем объект в Axetna.cloud
	_, err = s.AxetnaClient.UpdateObject(ctx, credentials, object)
	if err != nil {
		return s.createIntegrationError(tenantID, "update", object.ID, object.ExternalID,
			"Ошибка обновления объекта в Axetna.cloud", true, err)
	}

	s.logger.Printf("Объект %d успешно обновлен в Axetna.cloud (External ID: %s)", object.ID, object.ExternalID)
	return nil
}

// SyncObjectDelete синхронизирует удаление объекта с Axetna.cloud
func (s *IntegrationService) SyncObjectDelete(ctx context.Context, tenantID uint, object *models.Object) error {
	if object.ExternalID == "" {
		// Если нет ExternalID, объект не был синхронизирован - это не ошибка
		s.logger.Printf("Объект %d не имеет ExternalID, пропускаем синхронизацию удаления", object.ID)
		return nil
	}

	credentials, err := s.GetTenantCredentials(ctx, tenantID)
	if err != nil {
		return s.createIntegrationError(tenantID, "delete", object.ID, object.ExternalID,
			"Не удалось получить учетные данные", true, err)
	}

	// Удаляем объект в Axetna.cloud
	err = s.AxetnaClient.DeleteObject(ctx, credentials, object.ExternalID)
	if err != nil {
		return s.createIntegrationError(tenantID, "delete", object.ID, object.ExternalID,
			"Ошибка удаления объекта в Axetna.cloud", true, err)
	}

	s.logger.Printf("Объект %d успешно удален из Axetna.cloud (External ID: %s)", object.ID, object.ExternalID)
	return nil
}

// SyncObjectAsync выполняет синхронизацию объекта асинхронно
func (s *IntegrationService) SyncObjectAsync(tenantID uint, operation string, object *models.Object) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var err error
		switch operation {
		case "create":
			err = s.SyncObjectCreate(ctx, tenantID, object)
		case "update":
			err = s.SyncObjectUpdate(ctx, tenantID, object)
		case "delete":
			err = s.SyncObjectDelete(ctx, tenantID, object)
		default:
			s.logger.Printf("Неизвестная операция синхронизации: %s", operation)
			return
		}

		if err != nil {
			// В реальном приложении здесь можно добавить логику повторных попыток
			// или сохранение ошибок в очередь для последующей обработки
			s.logger.Printf("Ошибка асинхронной синхронизации: %v", err)

			// Сохраняем ошибку в БД для последующего анализа
			s.logIntegrationError(err)
		}
	}()
}

// CheckHealth проверяет доступность внешних систем
func (s *IntegrationService) CheckHealth(ctx context.Context) map[string]error {
	results := make(map[string]error)

	// Проверяем Axetna.cloud API
	results["axetna_cloud"] = s.AxetnaClient.IsHealthy(ctx)

	return results
}

// EncryptPassword шифрует пароль для хранения в БД
func (s *IntegrationService) EncryptPassword(password string) (string, error) {
	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("ошибка создания шифра: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("ошибка создания GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("ошибка генерации nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptPassword расшифровывает пароль из БД
func (s *IntegrationService) decryptPassword(encryptedPassword string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedPassword)
	if err != nil {
		return "", fmt.Errorf("ошибка декодирования base64: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("ошибка создания шифра: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("ошибка создания GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("зашифрованные данные слишком короткие")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка расшифровки: %w", err)
	}

	return string(plaintext), nil
}

// createIntegrationError создает структурированную ошибку интеграции
func (s *IntegrationService) createIntegrationError(tenantID uint, operation string, objectID uint, externalID string, message string, retryable bool, originalErr error) *IntegrationError {
	return &IntegrationError{
		TenantID:    tenantID,
		Operation:   operation,
		ObjectID:    objectID,
		ExternalID:  externalID,
		Message:     message,
		Retryable:   retryable,
		Timestamp:   time.Now(),
		OriginalErr: originalErr,
	}
}

// logIntegrationError сохраняет ошибку интеграции в БД для анализа
func (s *IntegrationService) logIntegrationError(err error) {
	integrationErr, ok := err.(*IntegrationError)
	if !ok {
		s.logger.Printf("Попытка сохранить не-IntegrationError: %v", err)
		return
	}

	// Создаем запись об ошибке в БД
	dbError := &models.IntegrationError{
		TenantID:     integrationErr.TenantID,
		Operation:    integrationErr.Operation,
		ObjectID:     integrationErr.ObjectID,
		ExternalID:   integrationErr.ExternalID,
		Service:      models.IntegrationServiceAxetnaCloud,
		ErrorMessage: integrationErr.Message,
		Retryable:    integrationErr.Retryable,
		Status:       models.IntegrationErrorStatusPending,
		MaxRetries:   3,
	}

	// Если есть оригинальная ошибка, сохраняем её детали
	if integrationErr.OriginalErr != nil {
		dbError.StackTrace = integrationErr.OriginalErr.Error()
	}

	// Устанавливаем время следующей попытки
	if dbError.Retryable {
		nextRetry := time.Now().Add(dbError.GetRetryDelay())
		dbError.NextRetryAt = &nextRetry
	}

	// Сохраняем в основную БД (не tenant-специфичную)
	db := database.GetDB()
	if err := db.Create(dbError).Error; err != nil {
		s.logger.Printf("Ошибка сохранения integration_error в БД: %v", err)
	} else {
		s.logger.Printf("Ошибка интеграции сохранена в БД (ID: %d): %s", dbError.ID, integrationErr.Message)
	}
}

// ClearCredentialsCache очищает кэш учетных данных
func (s *IntegrationService) ClearCredentialsCache(tenantID uint) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	if tenantID == 0 {
		// Очищаем весь кэш
		s.credentialsCache = make(map[uint]*TenantCredentials)
		s.logger.Println("Очищен весь кэш учетных данных")
	} else {
		// Очищаем кэш для конкретной компании
		delete(s.credentialsCache, tenantID)
		s.logger.Printf("Очищен кэш учетных данных для компании %d", tenantID)
	}
}

// SetupCompanyCredentials настраивает учетные данные компании для Axetna.cloud
func (s *IntegrationService) SetupCompanyCredentials(ctx context.Context, tenantID uint, login, password string) error {
	// Проверяем авторизацию
	_, err := s.AxetnaClient.Authenticate(ctx, login, password)
	if err != nil {
		return fmt.Errorf("ошибка проверки учетных данных: %w", err)
	}

	// Шифруем пароль
	encryptedPassword, err := s.EncryptPassword(password)
	if err != nil {
		return fmt.Errorf("ошибка шифрования пароля: %w", err)
	}

	// Обновляем данные компании в БД
	db := database.GetDB()
	result := db.Model(&models.Company{}).Where("id = ?", tenantID).Updates(map[string]interface{}{
		"axetna_login":    login,
		"axetna_password": encryptedPassword,
	})

	if result.Error != nil {
		return fmt.Errorf("ошибка сохранения учетных данных: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("компания %d не найдена", tenantID)
	}

	// Очищаем кэш для этой компании
	s.ClearCredentialsCache(tenantID)

	s.logger.Printf("Учетные данные для компании %d успешно настроены", tenantID)
	return nil
}

// GetCachedCredentialsCount возвращает количество кэшированных учетных данных
func (s *IntegrationService) GetCachedCredentialsCount() int {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return len(s.credentialsCache)
}
