package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// AuditLog модель для аудит логов
type AuditLog struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	TenantID   uint      `json:"tenant_id" gorm:"not null;index"`
	UserID     *uint     `json:"user_id" gorm:"index"`
	Action     string    `json:"action" gorm:"not null;index"`
	Resource   string    `json:"resource" gorm:"not null;index"`
	ResourceID *uint     `json:"resource_id" gorm:"index"`
	IPAddress  string    `json:"ip_address" gorm:"size:45"`
	UserAgent  string    `json:"user_agent" gorm:"size:500"`
	Details    string    `json:"details" gorm:"type:text"`
	OldValues  string    `json:"old_values" gorm:"type:text"`
	NewValues  string    `json:"new_values" gorm:"type:text"`
	Success    bool      `json:"success" gorm:"default:true;index"`
	ErrorMsg   string    `json:"error_message" gorm:"size:1000"`
	CreatedAt  time.Time `json:"created_at" gorm:"index"`
}

// AuditAction типы действий для аудита
type AuditAction string

const (
	// Пользовательские действия
	ActionUserLogin    AuditAction = "user.login"
	ActionUserLogout   AuditAction = "user.logout"
	ActionUserCreate   AuditAction = "user.create"
	ActionUserUpdate   AuditAction = "user.update"
	ActionUserDelete   AuditAction = "user.delete"
	ActionUserActivate AuditAction = "user.activate"

	// Объекты
	ActionObjectCreate  AuditAction = "object.create"
	ActionObjectUpdate  AuditAction = "object.update"
	ActionObjectDelete  AuditAction = "object.delete"
	ActionObjectRestore AuditAction = "object.restore"

	// Договоры
	ActionContractCreate AuditAction = "contract.create"
	ActionContractUpdate AuditAction = "contract.update"
	ActionContractDelete AuditAction = "contract.delete"

	// Биллинг
	ActionInvoiceCreate  AuditAction = "invoice.create"
	ActionInvoicePayment AuditAction = "invoice.payment"
	ActionInvoiceCancel  AuditAction = "invoice.cancel"

	// Монтажи
	ActionInstallationCreate   AuditAction = "installation.create"
	ActionInstallationUpdate   AuditAction = "installation.update"
	ActionInstallationComplete AuditAction = "installation.complete"
	ActionInstallationCancel   AuditAction = "installation.cancel"

	// Склад
	ActionEquipmentCreate  AuditAction = "equipment.create"
	ActionEquipmentUpdate  AuditAction = "equipment.update"
	ActionEquipmentMove    AuditAction = "equipment.move"
	ActionEquipmentInstall AuditAction = "equipment.install"

	// Системные действия
	ActionSystemBackup      AuditAction = "system.backup"
	ActionSystemRestore     AuditAction = "system.restore"
	ActionSystemConfig      AuditAction = "system.config"
	ActionSystemMaintenance AuditAction = "system.maintenance"

	// Безопасность
	ActionSecurityBreach           AuditAction = "security.breach"
	ActionSecurityRoleChange       AuditAction = "security.role_change"
	ActionSecurityPermissionChange AuditAction = "security.permission_change"
)

// AuditService сервис для аудит логов
type AuditService struct {
	db     *gorm.DB
	logger *log.Logger
}

// NewAuditService создает новый сервис аудита
func NewAuditService(db *gorm.DB, logger *log.Logger) *AuditService {
	return &AuditService{
		db:     db,
		logger: logger,
	}
}

// AuditContext контекст для аудита
type AuditContext struct {
	TenantID   uint
	UserID     *uint
	IPAddress  string
	UserAgent  string
	Action     AuditAction
	Resource   string
	ResourceID *uint
	OldValues  interface{}
	NewValues  interface{}
	Details    map[string]interface{}
	Success    bool
	ErrorMsg   string
}

// Log записывает аудит лог
func (as *AuditService) Log(ctx AuditContext) error {
	auditLog := &AuditLog{
		TenantID:   ctx.TenantID,
		UserID:     ctx.UserID,
		Action:     string(ctx.Action),
		Resource:   ctx.Resource,
		ResourceID: ctx.ResourceID,
		IPAddress:  ctx.IPAddress,
		UserAgent:  ctx.UserAgent,
		Success:    ctx.Success,
		ErrorMsg:   ctx.ErrorMsg,
		CreatedAt:  time.Now(),
	}

	// Сериализуем детали
	if ctx.Details != nil {
		if detailsJSON, err := json.Marshal(ctx.Details); err == nil {
			auditLog.Details = string(detailsJSON)
		}
	}

	// Сериализуем старые значения
	if ctx.OldValues != nil {
		if oldJSON, err := json.Marshal(ctx.OldValues); err == nil {
			auditLog.OldValues = string(oldJSON)
		}
	}

	// Сериализуем новые значения
	if ctx.NewValues != nil {
		if newJSON, err := json.Marshal(ctx.NewValues); err == nil {
			auditLog.NewValues = string(newJSON)
		}
	}

	// Используем основное подключение к БД
	db := as.db

	if err := db.Create(auditLog).Error; err != nil {
		if as.logger != nil {
			as.logger.Printf("Failed to create audit log: %v", err)
		}
		return err
	}

	return nil
}

// LogSuccess записывает успешное действие
func (as *AuditService) LogSuccess(ctx AuditContext) error {
	ctx.Success = true
	return as.Log(ctx)
}

// LogFailure записывает неуспешное действие
func (as *AuditService) LogFailure(ctx AuditContext, err error) error {
	ctx.Success = false
	ctx.ErrorMsg = err.Error()
	return as.Log(ctx)
}

// GetAuditLogs получает аудит логи с фильтрацией
func (as *AuditService) GetAuditLogs(tenantID uint, filters AuditFilters) ([]AuditLog, error) {
	db := as.db

	query := db.Where("tenant_id = ?", tenantID)

	// Применяем фильтры
	if filters.UserID != nil {
		query = query.Where("user_id = ?", *filters.UserID)
	}

	if filters.Action != "" {
		query = query.Where("action = ?", filters.Action)
	}

	if filters.Resource != "" {
		query = query.Where("resource = ?", filters.Resource)
	}

	if filters.ResourceID != nil {
		query = query.Where("resource_id = ?", *filters.ResourceID)
	}

	if filters.Success != nil {
		query = query.Where("success = ?", *filters.Success)
	}

	if !filters.StartDate.IsZero() {
		query = query.Where("created_at >= ?", filters.StartDate)
	}

	if !filters.EndDate.IsZero() {
		query = query.Where("created_at <= ?", filters.EndDate)
	}

	if filters.IPAddress != "" {
		query = query.Where("ip_address = ?", filters.IPAddress)
	}

	// Сортировка и пагинация
	query = query.Order("created_at DESC")

	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var logs []AuditLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}

// AuditFilters фильтры для поиска аудит логов
type AuditFilters struct {
	UserID     *uint
	Action     string
	Resource   string
	ResourceID *uint
	Success    *bool
	StartDate  time.Time
	EndDate    time.Time
	IPAddress  string
	Limit      int
	Offset     int
}

// GetAuditStats получает статистику аудит логов
func (as *AuditService) GetAuditStats(tenantID uint, period string) (*AuditStats, error) {
	db := as.db

	var startDate time.Time
	now := time.Now()

	switch period {
	case "day":
		startDate = now.AddDate(0, 0, -1)
	case "week":
		startDate = now.AddDate(0, 0, -7)
	case "month":
		startDate = now.AddDate(0, -1, 0)
	default:
		startDate = now.AddDate(0, 0, -7) // По умолчанию неделя
	}

	stats := &AuditStats{
		Period:    period,
		StartDate: startDate,
		EndDate:   now,
	}

	// Общее количество логов
	if err := db.Model(&AuditLog{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, startDate).
		Count(&stats.TotalLogs).Error; err != nil {
		return nil, err
	}

	// Количество успешных операций
	if err := db.Model(&AuditLog{}).
		Where("tenant_id = ? AND created_at >= ? AND success = ?", tenantID, startDate, true).
		Count(&stats.SuccessfulLogs).Error; err != nil {
		return nil, err
	}

	// Количество неуспешных операций
	if err := db.Model(&AuditLog{}).
		Where("tenant_id = ? AND created_at >= ? AND success = ?", tenantID, startDate, false).
		Count(&stats.FailedLogs).Error; err != nil {
		return nil, err
	}

	// Топ действий
	type ActionCount struct {
		Action string `json:"action"`
		Count  int64  `json:"count"`
	}

	var topActions []ActionCount
	if err := db.Model(&AuditLog{}).
		Select("action, COUNT(*) as count").
		Where("tenant_id = ? AND created_at >= ?", tenantID, startDate).
		Group("action").
		Order("count DESC").
		Limit(10).
		Find(&topActions).Error; err != nil {
		return nil, err
	}

	stats.TopActions = make(map[string]int64)
	for _, action := range topActions {
		stats.TopActions[action.Action] = action.Count
	}

	// Топ пользователей
	type UserCount struct {
		UserID uint  `json:"user_id"`
		Count  int64 `json:"count"`
	}

	var topUsers []UserCount
	if err := db.Model(&AuditLog{}).
		Select("user_id, COUNT(*) as count").
		Where("tenant_id = ? AND created_at >= ? AND user_id IS NOT NULL", tenantID, startDate).
		Group("user_id").
		Order("count DESC").
		Limit(10).
		Find(&topUsers).Error; err != nil {
		return nil, err
	}

	stats.TopUsers = make(map[uint]int64)
	for _, user := range topUsers {
		stats.TopUsers[user.UserID] = user.Count
	}

	// Активность по часам
	type HourlyActivity struct {
		Hour  int   `json:"hour"`
		Count int64 `json:"count"`
	}

	var hourlyActivity []HourlyActivity
	if err := db.Model(&AuditLog{}).
		Select("EXTRACT(hour FROM created_at) as hour, COUNT(*) as count").
		Where("tenant_id = ? AND created_at >= ?", tenantID, startDate).
		Group("hour").
		Order("hour").
		Find(&hourlyActivity).Error; err != nil {
		return nil, err
	}

	stats.HourlyActivity = make(map[int]int64)
	for _, activity := range hourlyActivity {
		stats.HourlyActivity[activity.Hour] = activity.Count
	}

	return stats, nil
}

// AuditStats статистика аудит логов
type AuditStats struct {
	Period         string           `json:"period"`
	StartDate      time.Time        `json:"start_date"`
	EndDate        time.Time        `json:"end_date"`
	TotalLogs      int64            `json:"total_logs"`
	SuccessfulLogs int64            `json:"successful_logs"`
	FailedLogs     int64            `json:"failed_logs"`
	TopActions     map[string]int64 `json:"top_actions"`
	TopUsers       map[uint]int64   `json:"top_users"`
	HourlyActivity map[int]int64    `json:"hourly_activity"`
}

// CleanupOldLogs удаляет старые аудит логи
func (as *AuditService) CleanupOldLogs(tenantID uint, retentionDays int) error {
	db := as.db

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	result := db.Where("tenant_id = ? AND created_at < ?", tenantID, cutoffDate).
		Delete(&AuditLog{})

	if result.Error != nil {
		return result.Error
	}

	if as.logger != nil {
		as.logger.Printf("Cleaned up %d audit logs older than %d days for tenant %d",
			result.RowsAffected, retentionDays, tenantID)
	}

	return nil
}

// ExportAuditLogs экспортирует аудит логи в JSON
func (as *AuditService) ExportAuditLogs(tenantID uint, filters AuditFilters) ([]byte, error) {
	logs, err := as.GetAuditLogs(tenantID, filters)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(logs, "", "  ")
}

// GetSecurityAlerts получает алерты безопасности
func (as *AuditService) GetSecurityAlerts(tenantID uint, hours int) ([]SecurityAlert, error) {
	db := as.db

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)
	var alerts []SecurityAlert

	// Множественные неудачные попытки входа
	type FailedLoginCount struct {
		IPAddress string `json:"ip_address"`
		Count     int64  `json:"count"`
	}

	var failedLogins []FailedLoginCount
	if err := db.Model(&AuditLog{}).
		Select("ip_address, COUNT(*) as count").
		Where("tenant_id = ? AND action = ? AND success = ? AND created_at >= ?",
			tenantID, ActionUserLogin, false, startTime).
		Group("ip_address").
		Having("COUNT(*) >= ?", 5). // 5 или более неудачных попыток
		Find(&failedLogins).Error; err != nil {
		return nil, err
	}

	for _, login := range failedLogins {
		alerts = append(alerts, SecurityAlert{
			Type:        "failed_logins",
			Severity:    "high",
			Description: fmt.Sprintf("Multiple failed login attempts from IP %s (%d attempts)", login.IPAddress, login.Count),
			IPAddress:   login.IPAddress,
			Count:       login.Count,
			DetectedAt:  time.Now(),
		})
	}

	// Подозрительная активность пользователей
	type UserActivity struct {
		UserID uint  `json:"user_id"`
		Count  int64 `json:"count"`
	}

	var suspiciousActivity []UserActivity
	if err := db.Model(&AuditLog{}).
		Select("user_id, COUNT(*) as count").
		Where("tenant_id = ? AND created_at >= ? AND user_id IS NOT NULL", tenantID, startTime).
		Group("user_id").
		Having("COUNT(*) >= ?", 100). // 100 или более действий за период
		Find(&suspiciousActivity).Error; err != nil {
		return nil, err
	}

	for _, activity := range suspiciousActivity {
		alerts = append(alerts, SecurityAlert{
			Type:        "suspicious_activity",
			Severity:    "medium",
			Description: fmt.Sprintf("Unusual activity from user ID %d (%d actions)", activity.UserID, activity.Count),
			UserID:      &activity.UserID,
			Count:       activity.Count,
			DetectedAt:  time.Now(),
		})
	}

	return alerts, nil
}

// SecurityAlert алерт безопасности
type SecurityAlert struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"` // low, medium, high, critical
	Description string    `json:"description"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserID      *uint     `json:"user_id,omitempty"`
	Count       int64     `json:"count"`
	DetectedAt  time.Time `json:"detected_at"`
}
