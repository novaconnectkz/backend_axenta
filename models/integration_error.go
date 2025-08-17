package models

import (
	"time"

	"gorm.io/gorm"
)

// IntegrationError модель для сохранения ошибок интеграции в БД
type IntegrationError struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Информация об ошибке
	TenantID   uint   `json:"tenant_id" gorm:"not null;index"`
	Operation  string `json:"operation" gorm:"not null;type:varchar(50)"` // create, update, delete
	ObjectID   uint   `json:"object_id" gorm:"index"`                     // ID локального объекта
	ExternalID string `json:"external_id" gorm:"type:varchar(100);index"` // ID во внешней системе
	Service    string `json:"service" gorm:"not null;type:varchar(50)"`   // axetna_cloud, bitrix24, 1c

	// Детали ошибки
	ErrorMessage string `json:"error_message" gorm:"type:text"`
	ErrorCode    string `json:"error_code" gorm:"type:varchar(100)"`
	Retryable    bool   `json:"retryable" gorm:"default:true"`

	// Информация о повторных попытках
	RetryCount  int        `json:"retry_count" gorm:"default:0"`
	MaxRetries  int        `json:"max_retries" gorm:"default:3"`
	NextRetryAt *time.Time `json:"next_retry_at"`
	LastRetryAt *time.Time `json:"last_retry_at"`

	// Статус обработки
	Status     string     `json:"status" gorm:"default:'pending';type:varchar(50)"` // pending, processing, resolved, failed
	ResolvedAt *time.Time `json:"resolved_at"`
	ResolvedBy string     `json:"resolved_by" gorm:"type:varchar(100)"` // user_id или system

	// Дополнительная информация
	RequestData  string `json:"request_data" gorm:"type:text"`  // JSON данных запроса
	ResponseData string `json:"response_data" gorm:"type:text"` // JSON ответа
	StackTrace   string `json:"stack_trace" gorm:"type:text"`   // Стек вызовов
	UserAgent    string `json:"user_agent" gorm:"type:varchar(255)"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:TenantID"`
}

// TableName задает имя таблицы для модели IntegrationError
func (IntegrationError) TableName() string {
	return "integration_errors"
}

// IntegrationErrorStatus константы для статусов ошибок
const (
	IntegrationErrorStatusPending    = "pending"
	IntegrationErrorStatusProcessing = "processing"
	IntegrationErrorStatusResolved   = "resolved"
	IntegrationErrorStatusFailed     = "failed"
)

// IntegrationErrorService константы для сервисов
const (
	IntegrationServiceAxetnaCloud = "axetna_cloud"
	IntegrationServiceBitrix24    = "bitrix24"
	IntegrationServiceOneC        = "1c"
	IntegrationServiceTelegram    = "telegram"
)

// IntegrationErrorOperation константы для операций
const (
	IntegrationOperationCreate = "create"
	IntegrationOperationUpdate = "update"
	IntegrationOperationDelete = "delete"
	IntegrationOperationSync   = "sync"
	IntegrationOperationAuth   = "auth"
)

// CanRetry проверяет, можно ли повторить операцию
func (ie *IntegrationError) CanRetry() bool {
	if !ie.Retryable {
		return false
	}

	if ie.RetryCount >= ie.MaxRetries {
		return false
	}

	if ie.Status == IntegrationErrorStatusResolved || ie.Status == IntegrationErrorStatusFailed {
		return false
	}

	if ie.NextRetryAt != nil && time.Now().Before(*ie.NextRetryAt) {
		return false
	}

	return true
}

// MarkAsProcessing отмечает ошибку как обрабатываемую
func (ie *IntegrationError) MarkAsProcessing() {
	ie.Status = IntegrationErrorStatusProcessing
	ie.UpdatedAt = time.Now()
}

// MarkAsResolved отмечает ошибку как решенную
func (ie *IntegrationError) MarkAsResolved(resolvedBy string) {
	ie.Status = IntegrationErrorStatusResolved
	now := time.Now()
	ie.ResolvedAt = &now
	ie.ResolvedBy = resolvedBy
	ie.UpdatedAt = now
}

// MarkAsFailed отмечает ошибку как неразрешимую
func (ie *IntegrationError) MarkAsFailed() {
	ie.Status = IntegrationErrorStatusFailed
	ie.UpdatedAt = time.Now()
}

// IncrementRetryCount увеличивает счетчик повторных попыток
func (ie *IntegrationError) IncrementRetryCount(nextRetryDelay time.Duration) {
	ie.RetryCount++
	now := time.Now()
	ie.LastRetryAt = &now

	if nextRetryDelay > 0 {
		nextRetry := now.Add(nextRetryDelay)
		ie.NextRetryAt = &nextRetry
	}

	ie.UpdatedAt = now
}

// IsExpired проверяет, истекла ли ошибка (старше определенного времени)
func (ie *IntegrationError) IsExpired(maxAge time.Duration) bool {
	return time.Since(ie.CreatedAt) > maxAge
}

// GetRetryDelay вычисляет задержку для следующей попытки с экспоненциальным backoff
func (ie *IntegrationError) GetRetryDelay() time.Duration {
	// Базовая задержка: 1 минута
	baseDelay := 1 * time.Minute

	// Экспоненциальный backoff: 1m, 2m, 4m, 8m, 16m, ...
	delay := baseDelay * time.Duration(1<<uint(ie.RetryCount))

	// Максимальная задержка: 1 час
	maxDelay := 1 * time.Hour
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// IntegrationErrorStats статистика по ошибкам интеграции
type IntegrationErrorStats struct {
	TotalErrors       int64              `json:"total_errors"`
	PendingErrors     int64              `json:"pending_errors"`
	ResolvedErrors    int64              `json:"resolved_errors"`
	FailedErrors      int64              `json:"failed_errors"`
	ErrorsByService   map[string]int64   `json:"errors_by_service"`
	ErrorsByOperation map[string]int64   `json:"errors_by_operation"`
	RecentErrors      []IntegrationError `json:"recent_errors"`
}

// GetIntegrationErrorStats возвращает статистику ошибок интеграции для компании
func GetIntegrationErrorStats(db *gorm.DB, tenantID uint, limit int) (*IntegrationErrorStats, error) {
	stats := &IntegrationErrorStats{
		ErrorsByService:   make(map[string]int64),
		ErrorsByOperation: make(map[string]int64),
	}

	// Базовый запрос
	baseQuery := db.Model(&IntegrationError{}).Where("tenant_id = ?", tenantID)

	// Общее количество ошибок
	if err := baseQuery.Count(&stats.TotalErrors).Error; err != nil {
		return nil, err
	}

	// Ошибки по статусам
	if err := baseQuery.Where("status = ?", IntegrationErrorStatusPending).Count(&stats.PendingErrors).Error; err != nil {
		return nil, err
	}

	if err := baseQuery.Where("status = ?", IntegrationErrorStatusResolved).Count(&stats.ResolvedErrors).Error; err != nil {
		return nil, err
	}

	if err := baseQuery.Where("status = ?", IntegrationErrorStatusFailed).Count(&stats.FailedErrors).Error; err != nil {
		return nil, err
	}

	// Ошибки по сервисам
	var serviceStats []struct {
		Service string
		Count   int64
	}
	if err := baseQuery.Select("service, COUNT(*) as count").Group("service").Scan(&serviceStats).Error; err != nil {
		return nil, err
	}

	for _, stat := range serviceStats {
		stats.ErrorsByService[stat.Service] = stat.Count
	}

	// Ошибки по операциям
	var operationStats []struct {
		Operation string
		Count     int64
	}
	if err := baseQuery.Select("operation, COUNT(*) as count").Group("operation").Scan(&operationStats).Error; err != nil {
		return nil, err
	}

	for _, stat := range operationStats {
		stats.ErrorsByOperation[stat.Operation] = stat.Count
	}

	// Последние ошибки
	if limit > 0 {
		if err := baseQuery.Order("created_at DESC").Limit(limit).Find(&stats.RecentErrors).Error; err != nil {
			return nil, err
		}
	}

	return stats, nil
}
