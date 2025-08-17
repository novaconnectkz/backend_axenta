package models

import (
	"time"

	"gorm.io/gorm"
)

// ReportType представляет тип отчета
type ReportType string

const (
	ReportTypeObjects       ReportType = "objects"
	ReportTypeUsers         ReportType = "users"
	ReportTypeBilling       ReportType = "billing"
	ReportTypeInstallations ReportType = "installations"
	ReportTypeWarehouse     ReportType = "warehouse"
	ReportTypeContracts     ReportType = "contracts"
	ReportTypeGeneral       ReportType = "general"
)

// ReportFormat представляет формат экспорта отчета
type ReportFormat string

const (
	ReportFormatPDF   ReportFormat = "pdf"
	ReportFormatExcel ReportFormat = "excel"
	ReportFormatCSV   ReportFormat = "csv"
	ReportFormatJSON  ReportFormat = "json"
)

// ReportStatus представляет статус генерации отчета
type ReportStatus string

const (
	ReportStatusPending    ReportStatus = "pending"
	ReportStatusProcessing ReportStatus = "processing"
	ReportStatusCompleted  ReportStatus = "completed"
	ReportStatusFailed     ReportStatus = "failed"
)

// ReportScheduleType представляет тип расписания для автоматических отчетов
type ReportScheduleType string

const (
	ScheduleTypeDaily   ReportScheduleType = "daily"
	ScheduleTypeWeekly  ReportScheduleType = "weekly"
	ScheduleTypeMonthly ReportScheduleType = "monthly"
	ScheduleTypeYearly  ReportScheduleType = "yearly"
)

// Report представляет модель отчета
type Report struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля отчета
	Name        string     `json:"name" gorm:"not null;type:varchar(200)"`
	Description string     `json:"description" gorm:"type:text"`
	Type        ReportType `json:"type" gorm:"not null;type:varchar(50)"`

	// Параметры отчета
	Parameters string `json:"parameters" gorm:"type:jsonb"` // JSON с параметрами отчета

	// Период отчета
	DateFrom *time.Time `json:"date_from"`
	DateTo   *time.Time `json:"date_to"`

	// Статус и результат
	Status      ReportStatus `json:"status" gorm:"default:pending;type:varchar(20)"`
	ErrorMsg    string       `json:"error_msg" gorm:"type:text"`
	FilePath    string       `json:"file_path" gorm:"type:varchar(500)"` // Путь к сгенерированному файлу
	FileSize    int64        `json:"file_size"`                          // Размер файла в байтах
	RecordCount int          `json:"record_count"`                       // Количество записей в отчете

	// Формат экспорта
	Format ReportFormat `json:"format" gorm:"not null;type:varchar(20)"`

	// Пользователь, создавший отчет
	CreatedByID uint  `json:"created_by_id" gorm:"not null;index"`
	CreatedBy   *User `json:"created_by,omitempty" gorm:"foreignKey:CreatedByID"`

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`

	// Время выполнения
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Duration    int        `json:"duration"` // Время выполнения в секундах
}

// TableName задает имя таблицы для модели Report
func (Report) TableName() string {
	return "reports"
}

// ReportTemplate представляет шаблон отчета
type ReportTemplate struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля шаблона
	Name        string     `json:"name" gorm:"not null;type:varchar(200)"`
	Description string     `json:"description" gorm:"type:text"`
	Type        ReportType `json:"type" gorm:"not null;type:varchar(50)"`

	// Конфигурация шаблона
	Config     string `json:"config" gorm:"type:jsonb"`     // JSON конфигурация шаблона
	SQLQuery   string `json:"sql_query" gorm:"type:text"`   // SQL запрос для получения данных
	Parameters string `json:"parameters" gorm:"type:jsonb"` // Параметры по умолчанию

	// Настройки форматирования
	Headers    string `json:"headers" gorm:"type:jsonb"`    // Заголовки колонок
	Formatting string `json:"formatting" gorm:"type:jsonb"` // Настройки форматирования

	// Статус и доступность
	IsActive bool `json:"is_active" gorm:"default:true"`
	IsPublic bool `json:"is_public" gorm:"default:false"` // Доступен для всех пользователей компании

	// Автор шаблона
	CreatedByID uint  `json:"created_by_id" gorm:"not null;index"`
	CreatedBy   *User `json:"created_by,omitempty" gorm:"foreignKey:CreatedByID"`

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели ReportTemplate
func (ReportTemplate) TableName() string {
	return "report_templates"
}

// ReportSchedule представляет расписание автоматических отчетов
type ReportSchedule struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля расписания
	Name        string             `json:"name" gorm:"not null;type:varchar(200)"`
	Description string             `json:"description" gorm:"type:text"`
	Type        ReportScheduleType `json:"type" gorm:"not null;type:varchar(20)"`

	// Связь с шаблоном отчета
	TemplateID uint            `json:"template_id" gorm:"not null"`
	Template   *ReportTemplate `json:"template,omitempty" gorm:"foreignKey:TemplateID"`

	// Параметры расписания
	CronExpression string `json:"cron_expression" gorm:"type:varchar(100)"` // Cron выражение для сложных расписаний
	TimeOfDay      string `json:"time_of_day" gorm:"type:varchar(10)"`      // Время запуска (HH:MM)
	DayOfWeek      int    `json:"day_of_week"`                              // День недели (0-6, 0=Sunday)
	DayOfMonth     int    `json:"day_of_month"`                             // День месяца (1-31)

	// Параметры отчета
	Parameters string       `json:"parameters" gorm:"type:jsonb"` // Параметры для генерации отчета
	Format     ReportFormat `json:"format" gorm:"not null;type:varchar(20)"`

	// Получатели отчета
	Recipients string `json:"recipients" gorm:"type:jsonb"` // JSON массив email адресов

	// Статус расписания
	IsActive     bool       `json:"is_active" gorm:"default:true"`
	LastRunAt    *time.Time `json:"last_run_at"`
	NextRunAt    *time.Time `json:"next_run_at"`
	LastReportID *uint      `json:"last_report_id"`
	LastReport   *Report    `json:"last_report,omitempty" gorm:"foreignKey:LastReportID"`

	// Статистика
	RunCount  int `json:"run_count" gorm:"default:0"`
	FailCount int `json:"fail_count" gorm:"default:0"`

	// Создатель расписания
	CreatedByID uint  `json:"created_by_id" gorm:"not null;index"`
	CreatedBy   *User `json:"created_by,omitempty" gorm:"foreignKey:CreatedByID"`

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели ReportSchedule
func (ReportSchedule) TableName() string {
	return "report_schedules"
}

// ReportExecution представляет выполнение отчета по расписанию
type ReportExecution struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с расписанием и отчетом
	ScheduleID uint            `json:"schedule_id" gorm:"not null;index"`
	Schedule   *ReportSchedule `json:"schedule,omitempty" gorm:"foreignKey:ScheduleID"`
	ReportID   *uint           `json:"report_id"`
	Report     *Report         `json:"report,omitempty" gorm:"foreignKey:ReportID"`

	// Статус выполнения
	Status   ReportStatus `json:"status" gorm:"default:pending;type:varchar(20)"`
	ErrorMsg string       `json:"error_msg" gorm:"type:text"`

	// Время выполнения
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	Duration    int        `json:"duration"` // Время выполнения в секундах

	// Результат отправки
	EmailsSent     int    `json:"emails_sent"`
	EmailsFailures int    `json:"emails_failures"`
	DeliveryLog    string `json:"delivery_log" gorm:"type:text"`

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели ReportExecution
func (ReportExecution) TableName() string {
	return "report_executions"
}
