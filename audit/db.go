package audit

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"
)

// AuditLog модель для хранения аудит-логов в базе данных
type AuditLog struct {
	ID         uint                   `gorm:"primaryKey" json:"id"`
	Timestamp  time.Time              `gorm:"index;not null" json:"timestamp"`
	UserID     string                 `gorm:"index;size:50" json:"user_id"`
	Username   string                 `gorm:"size:255" json:"username"`
	Role       string                 `gorm:"size:100" json:"role"`
	TenantID   string                 `gorm:"index;size:50" json:"tenant_id"`
	IP         string                 `gorm:"size:45" json:"ip"`
	UserAgent  string                 `gorm:"size:512" json:"user_agent"`
	Action     string                 `gorm:"index;size:255;not null" json:"action"`
	Resource   string                 `gorm:"size:255" json:"resource"`
	Method     string                 `gorm:"size:10" json:"method"`
	Path       string                 `gorm:"size:512" json:"path"`
	StatusCode int                    `json:"status_code"`
	Details    string                 `gorm:"type:jsonb" json:"-"` // Храним как JSONB в PostgreSQL
	DetailsMap map[string]interface{} `gorm:"-" json:"details,omitempty"`
	Success    bool                   `gorm:"index" json:"success"`
	Level      string                 `gorm:"size:20;index" json:"level"`
	Error      string                 `gorm:"type:text" json:"error,omitempty"`
	Duration   int64                  `json:"duration_ms"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	DeletedAt  gorm.DeletedAt         `gorm:"index" json:"-"`
}

// TableName указывает имя таблицы для модели
func (AuditLog) TableName() string {
	return "audit_logs"
}

// BeforeSave хук перед сохранением - сериализуем Details в JSON
func (a *AuditLog) BeforeSave(tx *gorm.DB) error {
	if a.DetailsMap != nil {
		jsonData, err := json.Marshal(a.DetailsMap)
		if err != nil {
			return err
		}
		a.Details = string(jsonData)
	}
	return nil
}

// AfterFind хук после загрузки - десериализуем Details из JSON
func (a *AuditLog) AfterFind(tx *gorm.DB) error {
	if a.Details != "" {
		var detailsMap map[string]interface{}
		if err := json.Unmarshal([]byte(a.Details), &detailsMap); err == nil {
			a.DetailsMap = detailsMap
		}
	}
	return nil
}

// DBLogger расширенный логгер с поддержкой базы данных
type DBLogger struct {
	*Logger
	db *gorm.DB
}

// NewDBLogger создает новый логгер с поддержкой базы данных
func NewDBLogger(cfg *Config, db *gorm.DB) (*DBLogger, error) {
	logger := &Logger{
		logToStdout: cfg.LogToStdout,
		logToFile:   cfg.LogToFile,
		config:      cfg,
	}

	if cfg.LogToFile && cfg.LogFilePath != "" {
		// Инициализация файла для логов
		if err := Init(cfg); err != nil {
			return nil, err
		}
		logger = GetLogger()
	}

	return &DBLogger{
		Logger: logger,
		db:     db,
	}, nil
}

// Log записывает событие в аудит-лог (файл + stdout + БД)
func (l *DBLogger) Log(entry *AuditEntry) {
	// Логируем в файл и stdout через базовый логгер
	if l.Logger != nil {
		l.Logger.Log(entry)
	}

	// Логируем в базу данных асинхронно для производительности
	if l.db != nil {
		go func() {
			auditLog := &AuditLog{
				Timestamp:  entry.Timestamp,
				UserID:     entry.UserID,
				Username:   entry.Username,
				Role:       entry.Role,
				TenantID:   entry.TenantID,
				IP:         entry.IP,
				UserAgent:  entry.UserAgent,
				Action:     entry.Action,
				Resource:   entry.Resource,
				Method:     entry.Method,
				Path:       entry.Path,
				StatusCode: entry.StatusCode,
				DetailsMap: entry.Details,
				Success:    entry.Success,
				Level:      string(entry.Level),
				Error:      entry.Error,
				Duration:   entry.Duration,
			}

			if err := l.db.Create(auditLog).Error; err != nil {
				// Не прерываем работу приложения из-за ошибки логирования
				// Просто логируем ошибку
				if l.Logger != nil && l.Logger.logToStdout {
					log.Printf("Failed to save audit log to database: %v", err)
				}
			}
		}()
	}
}

// SetGlobalDBLogger устанавливает глобальный логгер с поддержкой БД
func SetGlobalDBLogger(logger *DBLogger) {
	globalLogger = logger.Logger
}
