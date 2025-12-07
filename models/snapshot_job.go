package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// SnapshotJobStatus - статус выполнения задачи создания снимков
type SnapshotJobStatus string

const (
	SnapshotJobStatusRunning   SnapshotJobStatus = "running"   // Выполняется
	SnapshotJobStatusCompleted SnapshotJobStatus = "completed" // Успешно завершено
	SnapshotJobStatusFailed    SnapshotJobStatus = "failed"    // Ошибка
	SnapshotJobStatusPartial   SnapshotJobStatus = "partial"   // Частично выполнено (есть ошибки)
)

// SnapshotJobType - тип задачи создания снимков
type SnapshotJobType string

const (
	SnapshotJobTypeDailyAuto    SnapshotJobType = "daily_auto"    // Автоматическое ежедневное создание
	SnapshotJobTypeManual       SnapshotJobType = "manual"        // Ручное создание через UI
	SnapshotJobTypeScheduled    SnapshotJobType = "scheduled"     // По расписанию
	SnapshotJobTypeBillingStart SnapshotJobType = "billing_start" // Инициализация стартовой даты биллинга
)

// SnapshotJobDetails - детальная информация о выполнении задачи
type SnapshotJobDetails struct {
	Companies []CompanyJobDetail  `json:"companies,omitempty"`
	Contracts []ContractJobDetail `json:"contracts,omitempty"`
	Errors    []JobError          `json:"errors,omitempty"`
}

// CompanyJobDetail - информация о обработке компании
type CompanyJobDetail struct {
	CompanyID         uint   `json:"company_id"`
	CompanyName       string `json:"company_name"`
	ContractsCount    int    `json:"contracts_count"`
	SuccessCount      int    `json:"success_count"`
	ErrorCount        int    `json:"error_count"`
	ProcessingTimeS   int    `json:"processing_time_s,omitempty"`
	RealTotalObjects  int    `json:"real_total_objects,omitempty"`  // Реальное количество из API
	RealActiveObjects int    `json:"real_active_objects,omitempty"` // Реальное количество активных из API
}

// ContractJobDetail - информация о обработке договора
type ContractJobDetail struct {
	ContractID      uint   `json:"contract_id"`
	ContractNumber  string `json:"contract_number"`
	CompanyID       uint   `json:"company_id"`
	DaysProcessed   int    `json:"days_processed"`
	SuccessCount    int    `json:"success_count"`
	ErrorCount      int    `json:"error_count"`
	ErrorMessage    string `json:"error_message,omitempty"`
	ProcessingTimeS int    `json:"processing_time_s,omitempty"`
}

// JobError - информация об ошибке
type JobError struct {
	Timestamp   time.Time `json:"timestamp"`
	CompanyID   uint      `json:"company_id,omitempty"`
	ContractID  uint      `json:"contract_id,omitempty"`
	Date        string    `json:"date,omitempty"`
	Message     string    `json:"message"`
	ErrorType   string    `json:"error_type,omitempty"` // api_error, db_error, validation_error
	Recoverable bool      `json:"recoverable,omitempty"`
}

// ServerInfo - информация о сервере
type ServerInfo struct {
	Hostname  string `json:"hostname,omitempty"`
	Version   string `json:"version,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

// SnapshotJob - модель для хранения истории создания снимков
type SnapshotJob struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Информация о запуске
	JobType         SnapshotJobType `gorm:"type:varchar(50);not null;default:'daily_auto'" json:"job_type"`
	StartedAt       time.Time       `gorm:"not null" json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
	DurationSeconds *int            `json:"duration_seconds,omitempty"`

	// Статус
	Status SnapshotJobStatus `gorm:"type:varchar(20);not null;default:'running'" json:"status"`

	// Диапазон дат
	DateFrom time.Time `gorm:"type:date;not null" json:"date_from"`
	DateTo   time.Time `gorm:"type:date;not null" json:"date_to"`

	// Статистика
	TotalCompanies     int `gorm:"default:0" json:"total_companies"`
	TotalContracts     int `gorm:"default:0" json:"total_contracts"`
	TotalDaysProcessed int `gorm:"default:0" json:"total_days_processed"`
	SuccessCount       int `gorm:"default:0" json:"success_count"`
	ErrorCount         int `gorm:"default:0" json:"error_count"`
	SkippedCount       int `gorm:"default:0" json:"skipped_count"`

	// Статистика объектов
	TotalObjects  int `gorm:"default:0" json:"total_objects"`
	ActiveObjects int `gorm:"default:0" json:"active_objects"`

	// Время запуска по расписанию (для отображения в UI)
	ScheduledTime *time.Time `json:"scheduled_time,omitempty"`

	// Детали
	ErrorMessage string             `gorm:"type:text" json:"error_message,omitempty"`
	Details      SnapshotJobDetails `gorm:"type:jsonb" json:"details,omitempty"`

	// Метаданные
	TriggeredBy string     `gorm:"type:varchar(100)" json:"triggered_by,omitempty"` // system, user_id:123, cron, api
	ServerInfo  ServerInfo `gorm:"type:jsonb" json:"server_info,omitempty"`
}

// TableName - имя таблицы
func (SnapshotJob) TableName() string {
	return "snapshot_jobs"
}

// Scan implements sql.Scanner for SnapshotJobDetails
func (d *SnapshotJobDetails) Scan(value interface{}) error {
	if value == nil {
		*d = SnapshotJobDetails{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, d)
}

// Value implements driver.Valuer for SnapshotJobDetails
func (d SnapshotJobDetails) Value() (driver.Value, error) {
	if len(d.Companies) == 0 && len(d.Contracts) == 0 && len(d.Errors) == 0 {
		return nil, nil
	}
	return json.Marshal(d)
}

// Scan implements sql.Scanner for ServerInfo
func (s *ServerInfo) Scan(value interface{}) error {
	if value == nil {
		*s = ServerInfo{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}

// Value implements driver.Valuer for ServerInfo
func (s ServerInfo) Value() (driver.Value, error) {
	if s.Hostname == "" && s.Version == "" {
		return nil, nil
	}
	return json.Marshal(s)
}

// BeforeCreate hook
func (j *SnapshotJob) BeforeCreate(tx *gorm.DB) error {
	if j.StartedAt.IsZero() {
		j.StartedAt = time.Now()
	}
	if j.Status == "" {
		j.Status = SnapshotJobStatusRunning
	}
	return nil
}

// FinishJob - завершает задачу и вычисляет длительность
func (j *SnapshotJob) FinishJob(status SnapshotJobStatus, errorMessage string) {
	now := time.Now()
	j.FinishedAt = &now
	j.Status = status

	if errorMessage != "" {
		j.ErrorMessage = errorMessage
	}

	// Вычисляем длительность
	duration := int(now.Sub(j.StartedAt).Seconds())
	j.DurationSeconds = &duration
}

// AddCompanyDetail - добавляет информацию о обработке компании
func (j *SnapshotJob) AddCompanyDetail(detail CompanyJobDetail) {
	j.Details.Companies = append(j.Details.Companies, detail)
}

// AddContractDetail - добавляет информацию о обработке договора
func (j *SnapshotJob) AddContractDetail(detail ContractJobDetail) {
	j.Details.Contracts = append(j.Details.Contracts, detail)
}

// AddError - добавляет информацию об ошибке
func (j *SnapshotJob) AddError(err JobError) {
	if err.Timestamp.IsZero() {
		err.Timestamp = time.Now()
	}
	j.Details.Errors = append(j.Details.Errors, err)
	j.ErrorCount++
}
