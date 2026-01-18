package services

import (
	"backend_axenta/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"gorm.io/gorm"
)

// AxentaIntegrationService сервис для работы с интеграцией Axenta Cloud
type AxentaIntegrationService struct {
	db *gorm.DB
}

// AxentaIntegrationConfig конфигурация интеграции с Axenta Cloud
type AxentaIntegrationConfig struct {
	CompanyID       uint   `json:"company_id"`
	APIURL          string `json:"api_url"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	SyncInterval    int    `json:"sync_interval"` // в минутах
	AutoSyncEnabled bool   `json:"auto_sync_enabled"`
	RetryAttempts   int    `json:"retry_attempts"`
	Timeout         int    `json:"timeout"` // в секундах
}

// AxentaCredentials учетные данные для API
type AxentaCredentials struct {
	APIURL   string
	Username string
	Password string
	Token    string
	Timeout  int // в секундах
}

// IntegrationError ошибка интеграции
type IntegrationError struct {
	ID         string     `json:"id"`
	CompanyID  uint       `json:"company_id"`
	ErrorType  string     `json:"error_type"`
	Message    string     `json:"message"`
	Details    string     `json:"details"`
	Resolved   bool       `json:"resolved"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

// IntegrationStatus статус интеграции
type IntegrationStatus struct {
	IsActive     bool       `json:"is_active"`
	LastSyncAt   *time.Time `json:"last_sync_at"`
	LastErrorAt  *time.Time `json:"last_error_at"`
	ErrorMessage string     `json:"error_message"`
	SyncCount    int        `json:"sync_count"`
	ErrorCount   int        `json:"error_count"`
	SuccessCount int        `json:"success_count"`
	SuccessRate  float64    `json:"success_rate"`
	NextSyncAt   *time.Time `json:"next_sync_at"`
}

// NewAxentaIntegrationService создает новый сервис
func NewAxentaIntegrationService(db *gorm.DB) *AxentaIntegrationService {
	return &AxentaIntegrationService{
		db: db,
	}
}

// TestConnection тестирует подключение к Axenta Cloud
func (s *AxentaIntegrationService) TestConnection(ctx context.Context, companyID uint) error {
	// Получаем учетные данные
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных данных: %w", err)
	}

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: time.Duration(credentials.Timeout) * time.Second,
	}

	// Подготавливаем данные для авторизации
	loginData := map[string]string{
		"username": credentials.Username,
		"password": credentials.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return fmt.Errorf("ошибка сериализации данных авторизации: %w", err)
	}

	// Делаем запрос к Axenta Cloud
	resp, err := client.Post(credentials.APIURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка подключения к Axenta Cloud: %w", err)
	}
	defer resp.Body.Close()

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("авторизация в Axenta Cloud не удалась (статус: %d)", resp.StatusCode)
	}

	log.Printf("✅ Подключение к Axenta Cloud успешно для компании %d", companyID)
	return nil
}

// GetCredentials получает учетные данные для Axenta Cloud из БД
func (s *AxentaIntegrationService) GetCredentials(ctx context.Context, companyID uint) (*AxentaCredentials, error) {
	// Получаем интеграцию из БД (используем схему public)
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с Axenta Cloud не настроена для компании %d: %w", companyID, err)
	}

	// Парсим конфигурацию
	var config AxentaIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга настроек Axenta Cloud: %w", err)
	}

	return &AxentaCredentials{
		APIURL:   config.APIURL,
		Username: config.Username,
		Password: config.Password,
		Token:    "", // Токен получается при авторизации
		Timeout:  config.Timeout,
	}, nil
}

// SyncObjects синхронизирует объекты с Axenta Cloud
func (s *AxentaIntegrationService) SyncObjects(ctx context.Context, companyID uint) error {
	// Получаем учетные данные
	credentials, err := s.GetCredentials(ctx, companyID)
	if err != nil {
		return fmt.Errorf("ошибка получения учетных данных: %w", err)
	}

	// Получаем токен авторизации
	token, err := s.getAuthToken(ctx, credentials)
	if err != nil {
		return fmt.Errorf("ошибка получения токена авторизации: %w", err)
	}

	// Получаем объекты из локальной БД
	var objects []models.Object
	if err := s.db.Where("company_id = ?", companyID).Find(&objects).Error; err != nil {
		return fmt.Errorf("ошибка получения объектов: %w", err)
	}

	// Синхронизируем каждый объект
	successCount := 0
	errorCount := 0

	for _, object := range objects {
		if err := s.syncObject(ctx, credentials, token, &object); err != nil {
			log.Printf("❌ Ошибка синхронизации объекта %d: %v", object.ID, err)
			errorCount++
		} else {
			successCount++
		}
	}

	// Обновляем статистику интеграции
	s.updateIntegrationStats(companyID, successCount > 0, fmt.Sprintf("Синхронизировано: %d, Ошибок: %d", successCount, errorCount))

	log.Printf("✅ Синхронизация завершена для компании %d: успешно %d, ошибок %d", companyID, successCount, errorCount)
	return nil
}

// getAuthToken получает токен авторизации от Axenta Cloud
func (s *AxentaIntegrationService) getAuthToken(_ context.Context, credentials *AxentaCredentials) (string, error) {
	client := &http.Client{
		Timeout: time.Duration(credentials.Timeout) * time.Second,
	}

	loginData := map[string]string{
		"username": credentials.Username,
		"password": credentials.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации данных авторизации: %w", err)
	}

	resp, err := client.Post(credentials.APIURL+"/api/auth/login/", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка подключения к Axenta Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("авторизация в Axenta Cloud не удалась (статус: %d)", resp.StatusCode)
	}

	var authResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа авторизации: %w", err)
	}

	// Извлекаем токен
	token, ok := authResponse["token"].(string)
	if !ok {
		if accessToken, ok := authResponse["access"].(string); ok {
			token = accessToken
		} else {
			return "", fmt.Errorf("токен не найден в ответе авторизации")
		}
	}

	return token, nil
}

// syncObject синхронизирует один объект с Axenta Cloud
func (s *AxentaIntegrationService) syncObject(ctx context.Context, credentials *AxentaCredentials, token string, object *models.Object) error {
	client := &http.Client{
		Timeout: time.Duration(credentials.Timeout) * time.Second,
	}

	// Подготавливаем данные объекта
	objectData := map[string]interface{}{
		"name":          object.Name,
		"type":          object.Type,
		"description":   object.Description,
		"imei":          object.IMEI,
		"phone_number":  object.PhoneNumber,
		"serial_number": object.SerialNumber,
		"latitude":      object.Latitude,
		"longitude":     object.Longitude,
		"address":       object.Address,
	}

	// Парсим настройки из JSON строки
	if object.Settings != "" {
		var settings map[string]interface{}
		if err := json.Unmarshal([]byte(object.Settings), &settings); err == nil {
			objectData["settings"] = settings
		}
	}

	jsonData, err := json.Marshal(objectData)
	if err != nil {
		return fmt.Errorf("ошибка сериализации данных объекта: %w", err)
	}

	// Определяем метод запроса (POST для создания, PUT для обновления)
	method := "POST"
	url := credentials.APIURL + "/api/objects/"

	// Если у объекта есть внешний ID, используем PUT для обновления
	if object.ExternalID != "" {
		method = "PUT"
		url = credentials.APIURL + "/api/objects/" + object.ExternalID + "/"
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("User-Agent", "AxentaCRM/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("ошибка синхронизации объекта (статус: %d)", resp.StatusCode)
	}

	// Парсим ответ и обновляем внешний ID
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err == nil {
		if id, ok := responseData["id"].(string); ok {
			object.ExternalID = id
			s.db.Save(object)
		}
	}

	return nil
}

// ScheduleAutoSync планирует автоматическую синхронизацию
func (s *AxentaIntegrationService) ScheduleAutoSync(ctx context.Context, companyID uint) error {
	// Получаем конфигурацию интеграции
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		return fmt.Errorf("интеграция с Axenta Cloud не найдена: %w", err)
	}

	var config AxentaIntegrationConfig
	if err := json.Unmarshal([]byte(integration.Settings), &config); err != nil {
		return fmt.Errorf("ошибка парсинга конфигурации: %w", err)
	}

	if !config.AutoSyncEnabled {
		return fmt.Errorf("автоматическая синхронизация отключена")
	}

	// Здесь можно добавить логику планирования задач (например, через cron)
	// Пока просто логируем
	log.Printf("📅 Автоматическая синхронизация запланирована для компании %d (интервал: %d минут)", companyID, config.SyncInterval)

	return nil
}

// GetIntegrationErrors получает список ошибок интеграции
func (s *AxentaIntegrationService) GetIntegrationErrors(ctx context.Context, companyID uint) ([]IntegrationError, error) {
	// Здесь можно реализовать получение ошибок из БД или логов
	// Пока возвращаем пустой список
	return []IntegrationError{}, nil
}

// ResolveError отмечает ошибку как решенную
func (s *AxentaIntegrationService) ResolveError(ctx context.Context, companyID uint, errorID string) error {
	// Здесь можно реализовать логику решения ошибок
	log.Printf("🔧 Ошибка %s отмечена как решенная для компании %d", errorID, companyID)
	return nil
}

// GetIntegrationStatus получает статус интеграции
func (s *AxentaIntegrationService) GetIntegrationStatus(ctx context.Context, companyID uint) (*IntegrationStatus, error) {
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("интеграция с Axenta Cloud не найдена: %w", err)
	}

	status := &IntegrationStatus{
		IsActive:     integration.IsActive,
		LastSyncAt:   integration.LastSyncAt,
		LastErrorAt:  integration.LastErrorAt,
		ErrorMessage: integration.ErrorMessage,
		SyncCount:    integration.SyncCount,
		ErrorCount:   integration.ErrorCount,
		SuccessCount: integration.SuccessCount,
		SuccessRate:  integration.GetSuccessRate(),
	}

	// Рассчитываем время следующей синхронизации
	if integration.LastSyncAt != nil {
		var config AxentaIntegrationConfig
		if err := json.Unmarshal([]byte(integration.Settings), &config); err == nil {
			nextSync := integration.LastSyncAt.Add(time.Duration(config.SyncInterval) * time.Minute)
			status.NextSyncAt = &nextSync
		}
	}

	return status, nil
}

// updateIntegrationStats обновляет статистику интеграции
func (s *AxentaIntegrationService) updateIntegrationStats(companyID uint, success bool, errorMessage string) {
	var integration models.Integration
	if err := s.db.Where("company_id = ? AND integration_type = ?", companyID, "axenta_cloud").First(&integration).Error; err != nil {
		log.Printf("❌ Ошибка получения интеграции для обновления статистики: %v", err)
		return
	}

	integration.UpdateStats(success, errorMessage)
	if err := s.db.Save(&integration).Error; err != nil {
		log.Printf("❌ Ошибка обновления статистики интеграции: %v", err)
	}
}
