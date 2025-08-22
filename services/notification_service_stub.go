package services

import (
	"backend_axenta/models"
	"time"

	"gorm.io/gorm"
)

// NotificationService заглушка для временного отключения
type NotificationService struct {
	DB *gorm.DB
}

// NewNotificationService создает заглушку сервиса уведомлений
func NewNotificationService(db *gorm.DB, cache *CacheService) *NotificationService {
	return &NotificationService{DB: db}
}

// SendNotification заглушка для отправки уведомлений
func (s *NotificationService) SendNotification(notificationType, channel, recipient string, templateData map[string]interface{}, companyID, relatedID uint, relatedType string) error {
	// Заглушка - не отправляем уведомления
	return nil
}

// SendInstallationReminder заглушка
func (s *NotificationService) SendInstallationReminder(installation *models.Installation) error {
	return nil
}

// SendBillingAlert заглушка
func (s *NotificationService) SendBillingAlert(companyID uint, alertType string, message string) error {
	return nil
}

// SendWarehouseAlert заглушка
func (s *NotificationService) SendWarehouseAlert(companyID uint, alertType string, message string) error {
	return nil
}

// GetNotificationSettings заглушка
func (s *NotificationService) GetNotificationSettings(companyID uint) (*models.NotificationSettings, error) {
	return nil, nil
}

// SendInstallationCreated заглушка
func (s *NotificationService) SendInstallationCreated(installation *models.Installation) error {
	return nil
}

// SendInstallationUpdated заглушка
func (s *NotificationService) SendInstallationUpdated(installation *models.Installation) error {
	return nil
}

// SendInstallationCompleted заглушка
func (s *NotificationService) SendInstallationCompleted(installation *models.Installation) error {
	return nil
}

// SendInstallationCancelled заглушка
func (s *NotificationService) SendInstallationCancelled(installation *models.Installation) error {
	return nil
}

// SendInstallationRescheduled заглушка
func (s *NotificationService) SendInstallationRescheduled(installation *models.Installation, oldScheduledAt time.Time) error {
	return nil
}

// SendStockAlert заглушка
func (s *NotificationService) SendStockAlert(alert models.StockAlert) error {
	return nil
}

// SendWarrantyAlert заглушка
func (s *NotificationService) SendWarrantyAlert(alert models.StockAlert) error {
	return nil
}

// SendMaintenanceAlert заглушка
func (s *NotificationService) SendMaintenanceAlert(alert models.StockAlert) error {
	return nil
}

// SendEquipmentMovementNotification заглушка
func (s *NotificationService) SendEquipmentMovementNotification(operation models.WarehouseOperation) error {
	return nil
}

// ProcessRetryNotifications заглушка
func (s *NotificationService) ProcessRetryNotifications() error {
	return nil
}

// GetNotificationLogs заглушка
func (s *NotificationService) GetNotificationLogs(limit int, offset int, filters map[string]interface{}, companyID uint) ([]models.NotificationLog, int64, error) {
	return []models.NotificationLog{}, 0, nil
}

// GetNotificationStatistics заглушка
func (s *NotificationService) GetNotificationStatistics(companyID uint) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

// CreateDefaultTemplates заглушка
func (s *NotificationService) CreateDefaultTemplates(companyID uint) error {
	return nil
}
