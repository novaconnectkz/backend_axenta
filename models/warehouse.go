package models

import (
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Equipment представляет оборудование на складе
type Equipment struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные характеристики оборудования
	Type         string `json:"type" gorm:"not null;type:varchar(50)"`              // GPS-tracker, sensor, camera, etc.
	Model        string `json:"model" gorm:"not null;type:varchar(100)"`            // Модель устройства
	Brand        string `json:"brand" gorm:"type:varchar(100)"`                     // Производитель
	SerialNumber string `json:"serial_number" gorm:"uniqueIndex;type:varchar(100)"` // Серийный номер

	// Идентификаторы
	IMEI        string `json:"imei" gorm:"uniqueIndex;type:varchar(20)"`     // IMEI для GSM устройств
	PhoneNumber string `json:"phone_number" gorm:"type:varchar(20)"`         // Номер телефона SIM-карты
	MACAddress  string `json:"mac_address" gorm:"type:varchar(20)"`          // MAC адрес для WiFi устройств
	QRCode      string `json:"qr_code" gorm:"uniqueIndex;type:varchar(100)"` // QR код для быстрого поиска

	// Статус оборудования
	Status    string `json:"status" gorm:"default:'in_stock';type:varchar(20)"` // in_stock, reserved, installed, maintenance, broken, disposed
	Condition string `json:"condition" gorm:"default:'new';type:varchar(20)"`   // new, used, refurbished, damaged

	// Связь с объектом (если установлено)
	ObjectID *uint   `json:"object_id"`
	Object   *Object `json:"object,omitempty" gorm:"foreignKey:ObjectID"`

	// Категория оборудования
	CategoryID *uint              `json:"category_id"`
	Category   *EquipmentCategory `json:"category,omitempty" gorm:"foreignKey:CategoryID"`

	// Местоположение на складе
	WarehouseLocation string `json:"warehouse_location" gorm:"type:varchar(100)"` // Полка, ячейка и т.д.

	// Финансовая информация
	PurchasePrice decimal.Decimal `json:"purchase_price" gorm:"type:decimal(10,2)"` // Закупочная цена
	PurchaseDate  *time.Time      `json:"purchase_date"`                            // Дата закупки
	WarrantyUntil *time.Time      `json:"warranty_until"`                           // Гарантия до

	// Технические характеристики (JSON)
	Specifications string `json:"specifications" gorm:"type:jsonb"` // Технические характеристики

	// История и заметки
	Notes             string     `json:"notes" gorm:"type:text"`
	LastMaintenanceAt *time.Time `json:"last_maintenance_at"` // Последнее обслуживание

	// Связи
	Installations []Installation `json:"installations,omitempty" gorm:"many2many:installation_equipment;"`
}

// TableName задает имя таблицы для модели Equipment
func (Equipment) TableName() string {
	return "equipment"
}

// IsAvailable проверяет, доступно ли оборудование для использования
func (e *Equipment) IsAvailable() bool {
	return e.Status == "in_stock" && e.Condition != "damaged" && e.Condition != "broken"
}

// NeedsAttention проверяет, требует ли оборудование внимания
func (e *Equipment) NeedsAttention() bool {
	// Проверяем состояние
	if e.Condition == "damaged" || e.Status == "maintenance" {
		return true
	}

	// Проверяем гарантию
	if e.WarrantyUntil != nil && time.Now().After(*e.WarrantyUntil) {
		return true
	}

	return false
}

// Location представляет локацию (город) для планирования монтажей
type Location struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Географическая информация
	City      string   `json:"city" gorm:"not null;type:varchar(100)"`
	Region    string   `json:"region" gorm:"type:varchar(100)"`
	Country   string   `json:"country" gorm:"default:'Russia';type:varchar(100)"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`

	// Временная зона
	Timezone string `json:"timezone" gorm:"default:'Europe/Moscow';type:varchar(50)"`

	// Статус
	IsActive bool `json:"is_active" gorm:"default:true"`

	// Дополнительная информация
	Notes string `json:"notes" gorm:"type:text"`

	// Связи
	Objects []Object `json:"objects,omitempty" gorm:"foreignKey:LocationID"`
}

// TableName задает имя таблицы для модели Location
func (Location) TableName() string {
	return "locations"
}

// GetFullName возвращает полное название локации
func (l *Location) GetFullName() string {
	if l.Region != "" {
		return l.City + ", " + l.Region
	}
	return l.City
}

// Installer представляет монтажника
type Installer struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	FirstName  string `json:"first_name" gorm:"not null;type:varchar(50)"`
	LastName   string `json:"last_name" gorm:"not null;type:varchar(50)"`
	MiddleName string `json:"middle_name" gorm:"type:varchar(50)"`
	Type       string `json:"type" gorm:"not null;type:varchar(20)"` // staff (штатный), contractor (наемный), partner (партнер)

	// Контактная информация
	Phone      string `json:"phone" gorm:"not null;type:varchar(20)"`
	Email      string `json:"email" gorm:"uniqueIndex;type:varchar(100)"`
	TelegramID string `json:"telegram_id" gorm:"type:varchar(50)"`

	// Специализация и навыки
	Specialization []string `json:"specialization" gorm:"type:text[]"`                    // GPS-trackers, sensors, cameras, etc.
	SkillLevel     string   `json:"skill_level" gorm:"default:'junior';type:varchar(20)"` // junior, middle, senior
	Experience     int      `json:"experience"`                                           // Опыт в месяцах

	// Рабочие локации
	LocationIDs []uint     `json:"location_ids" gorm:"type:integer[]"` // ID городов где работает
	Locations   []Location `json:"locations,omitempty" gorm:"many2many:installer_locations;"`

	// Рабочие параметры
	MaxDailyInstallations int             `json:"max_daily_installations" gorm:"default:3"`   // Максимум монтажей в день
	WorkingHoursStart     string          `json:"working_hours_start" gorm:"default:'09:00'"` // Начало рабочего дня
	WorkingHoursEnd       string          `json:"working_hours_end" gorm:"default:'18:00'"`   // Конец рабочего дня
	WorkingDays           []int           `json:"working_days" gorm:"type:integer[]"`         // Рабочие дни (1-7, где 1=понедельник)
	HourlyRate            decimal.Decimal `json:"hourly_rate" gorm:"type:decimal(8,2)"`       // Ставка за час

	// Статус и доступность
	IsActive     bool       `json:"is_active" gorm:"default:true"`
	Status       string     `json:"status" gorm:"default:'available';type:varchar(20)"` // available, busy, vacation, sick
	LastWorkedAt *time.Time `json:"last_worked_at"`

	// Рейтинг и статистика
	Rating        float32 `json:"rating" gorm:"default:5.0"` // Рейтинг от 1 до 5
	CompletedJobs int     `json:"completed_jobs" gorm:"default:0"`

	// Дополнительная информация
	Notes string `json:"notes" gorm:"type:text"`

	// Связи
	Installations []Installation `json:"installations,omitempty" gorm:"foreignKey:InstallerID"`
}

// TableName задает имя таблицы для модели Installer
func (Installer) TableName() string {
	return "installers"
}

// GetFullName возвращает полное имя монтажника
func (i *Installer) GetFullName() string {
	fullName := i.FirstName
	if i.MiddleName != "" {
		fullName += " " + i.MiddleName
	}
	fullName += " " + i.LastName
	return fullName
}

// GetDisplayName возвращает короткое отображаемое имя
func (i *Installer) GetDisplayName() string {
	return i.FirstName + " " + i.LastName
}

// IsAvailableOnDate проверяет доступность монтажника на определенную дату
func (i *Installer) IsAvailableOnDate(date time.Time) bool {
	if !i.IsActive || i.Status != "available" {
		return false
	}

	// Проверяем рабочие дни
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7 // Воскресенье = 7
	}

	for _, workDay := range i.WorkingDays {
		if workDay == weekday {
			return true
		}
	}
	return false
}

// CanWorkInLocation проверяет, может ли монтажник работать в указанной локации
func (i *Installer) CanWorkInLocation(locationID uint) bool {
	for _, id := range i.LocationIDs {
		if id == locationID {
			return true
		}
	}
	return false
}

// HasSpecialization проверяет, есть ли у монтажника указанная специализация
func (i *Installer) HasSpecialization(spec string) bool {
	for _, s := range i.Specialization {
		if s == spec {
			return true
		}
	}
	return false
}

// Installation представляет монтаж или диагностику
type Installation struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	Type        string `json:"type" gorm:"not null;type:varchar(50)"`             // монтаж, диагностика, демонтаж, обслуживание
	Status      string `json:"status" gorm:"default:'planned';type:varchar(50)"`  // planned, in_progress, completed, cancelled, postponed
	Priority    string `json:"priority" gorm:"default:'normal';type:varchar(20)"` // low, normal, high, urgent
	Description string `json:"description" gorm:"type:text"`

	// Даты и время
	ScheduledAt       time.Time  `json:"scheduled_at" gorm:"not null"` // Запланированная дата и время
	EstimatedDuration int        `json:"estimated_duration"`           // Оценочная продолжительность в минутах
	StartedAt         *time.Time `json:"started_at"`                   // Фактическое начало
	CompletedAt       *time.Time `json:"completed_at"`                 // Фактическое завершение

	// Связи с другими сущностями
	ObjectID uint    `json:"object_id" gorm:"not null;index"`
	Object   *Object `json:"object,omitempty" gorm:"foreignKey:ObjectID"`

	InstallerID uint       `json:"installer_id" gorm:"not null;index"`
	Installer   *Installer `json:"installer,omitempty" gorm:"foreignKey:InstallerID"`

	LocationID *uint     `json:"location_id" gorm:"index"`
	Location   *Location `json:"location,omitempty" gorm:"foreignKey:LocationID"`

	// Оборудование для монтажа
	Equipment []Equipment `json:"equipment,omitempty" gorm:"many2many:installation_equipment;"`

	// Дополнительная информация
	ClientContact string `json:"client_contact" gorm:"type:varchar(100)"` // Контакт клиента
	Address       string `json:"address" gorm:"type:text"`                // Адрес монтажа
	Notes         string `json:"notes" gorm:"type:text"`                  // Заметки
	Result        string `json:"result" gorm:"type:text"`                 // Результат выполнения

	// Метаданные
	CreatedByUserID uint  `json:"created_by_user_id" gorm:"index"`
	CreatedByUser   *User `json:"created_by_user,omitempty" gorm:"foreignKey:CreatedByUserID"`

	// Настройки напоминаний
	ReminderSent     bool       `json:"reminder_sent" gorm:"default:false"`
	ReminderSentAt   *time.Time `json:"reminder_sent_at"`
	NotificationSent bool       `json:"notification_sent" gorm:"default:false"`

	// Дополнительные поля для отчетности
	ActualDuration int     `json:"actual_duration"` // Фактическая продолжительность в минутах
	TravelTime     int     `json:"travel_time"`     // Время в пути в минутах
	MaterialsCost  float64 `json:"materials_cost"`  // Стоимость материалов
	LaborCost      float64 `json:"labor_cost"`      // Стоимость работы

	// Качество работы (из старой модели)
	QualityRating  *float32 `json:"quality_rating"` // Оценка качества от 1 до 5
	ClientFeedback string   `json:"client_feedback" gorm:"type:text"`
	Issues         string   `json:"issues" gorm:"type:text"`   // Проблемы, возникшие при работе
	Photos         []string `json:"photos" gorm:"type:text[]"` // Пути к фотографиям

	// Стоимость (из старой модели)
	Cost       decimal.Decimal `json:"cost" gorm:"type:decimal(10,2)"`  // Стоимость работы
	IsBillable bool            `json:"is_billable" gorm:"default:true"` // Оплачиваемая ли работа

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели Installation
func (Installation) TableName() string {
	return "installations"
}

// IsOverdue проверяет, просрочен ли монтаж
func (i *Installation) IsOverdue() bool {
	if i.Status == "completed" || i.Status == "cancelled" {
		return false
	}
	return time.Now().After(i.ScheduledAt)
}

// CanBeStarted проверяет, можно ли начать монтаж
func (i *Installation) CanBeStarted() bool {
	return i.Status == "planned" && time.Now().After(i.ScheduledAt.Add(-30*time.Minute))
}

// GetStatusDisplayName возвращает читаемое название статуса
func (i *Installation) GetStatusDisplayName() string {
	statusMap := map[string]string{
		"planned":     "Запланирован",
		"in_progress": "Выполняется",
		"completed":   "Завершен",
		"cancelled":   "Отменен",
		"postponed":   "Перенесен",
	}
	if displayName, exists := statusMap[i.Status]; exists {
		return displayName
	}
	return i.Status
}

// GetTypeDisplayName возвращает читаемое название типа работы
func (i *Installation) GetTypeDisplayName() string {
	typeMap := map[string]string{
		"монтаж":       "Монтаж",
		"диагностика":  "Диагностика",
		"демонтаж":     "Демонтаж",
		"обслуживание": "Обслуживание",
	}
	if displayName, exists := typeMap[i.Type]; exists {
		return displayName
	}
	return i.Type
}

// GetPriorityDisplayName возвращает читаемое название приоритета
func (i *Installation) GetPriorityDisplayName() string {
	priorityMap := map[string]string{
		"low":    "Низкий",
		"normal": "Обычный",
		"high":   "Высокий",
		"urgent": "Срочный",
	}
	if displayName, exists := priorityMap[i.Priority]; exists {
		return displayName
	}
	return i.Priority
}

// GetDuration возвращает продолжительность работы
func (i *Installation) GetDuration() time.Duration {
	if i.StartedAt != nil && i.CompletedAt != nil {
		return i.CompletedAt.Sub(*i.StartedAt)
	}
	return 0
}

// IsCompleted проверяет, завершена ли работа
func (i *Installation) IsCompleted() bool {
	return i.Status == "completed" && i.CompletedAt != nil
}

// WarehouseOperation представляет операцию на складе
type WarehouseOperation struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	Type        string `json:"type" gorm:"not null;type:varchar(50)"` // receive, issue, transfer, inventory, maintenance, disposal
	Description string `json:"description" gorm:"type:text"`
	Status      string `json:"status" gorm:"default:'completed';type:varchar(20)"` // pending, completed, cancelled

	// Связанное оборудование
	EquipmentID uint       `json:"equipment_id" gorm:"not null;index"`
	Equipment   *Equipment `json:"equipment,omitempty" gorm:"foreignKey:EquipmentID"`

	// Количество (для групповых операций)
	Quantity int `json:"quantity" gorm:"default:1"`

	// Местоположение
	FromLocation string `json:"from_location" gorm:"type:varchar(100)"` // Откуда
	ToLocation   string `json:"to_location" gorm:"type:varchar(100)"`   // Куда

	// Ответственное лицо
	UserID uint  `json:"user_id" gorm:"index"`
	User   *User `json:"user,omitempty" gorm:"foreignKey:UserID"`

	// Документы и заметки
	DocumentNumber string `json:"document_number" gorm:"type:varchar(50)"` // Номер документа
	Notes          string `json:"notes" gorm:"type:text"`

	// Связанная установка (если операция связана с монтажом)
	InstallationID *uint         `json:"installation_id"`
	Installation   *Installation `json:"installation,omitempty" gorm:"foreignKey:InstallationID"`

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели WarehouseOperation
func (WarehouseOperation) TableName() string {
	return "warehouse_operations"
}

// GetTypeDisplayName возвращает читаемое название типа операции
func (wo *WarehouseOperation) GetTypeDisplayName() string {
	typeMap := map[string]string{
		"receive":     "Поступление",
		"issue":       "Выдача",
		"transfer":    "Перемещение",
		"inventory":   "Инвентаризация",
		"maintenance": "Обслуживание",
		"disposal":    "Списание",
	}
	if displayName, exists := typeMap[wo.Type]; exists {
		return displayName
	}
	return wo.Type
}

// EquipmentCategory представляет категорию оборудования для группировки
type EquipmentCategory struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основная информация
	Name        string `json:"name" gorm:"not null;uniqueIndex;type:varchar(100)"`
	Description string `json:"description" gorm:"type:text"`
	Code        string `json:"code" gorm:"uniqueIndex;type:varchar(20)"` // Код категории

	// Настройки
	MinStockLevel int  `json:"min_stock_level" gorm:"default:5"` // Минимальный остаток для уведомлений
	IsActive      bool `json:"is_active" gorm:"default:true"`

	// Связи
	Equipment []Equipment `json:"equipment,omitempty" gorm:"foreignKey:CategoryID"`
}

// TableName задает имя таблицы для модели EquipmentCategory
func (EquipmentCategory) TableName() string {
	return "equipment_categories"
}

// StockAlert представляет уведомление о низких остатках
type StockAlert struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Информация об оповещении
	Type        string `json:"type" gorm:"not null;type:varchar(50)"` // low_stock, expired_warranty, maintenance_due
	Title       string `json:"title" gorm:"not null;type:varchar(200)"`
	Description string `json:"description" gorm:"type:text"`
	Severity    string `json:"severity" gorm:"default:'medium';type:varchar(20)"` // low, medium, high, critical

	// Связанное оборудование или категория
	EquipmentID         *uint              `json:"equipment_id"`
	Equipment           *Equipment         `json:"equipment,omitempty" gorm:"foreignKey:EquipmentID"`
	EquipmentCategoryID *uint              `json:"equipment_category_id"`
	EquipmentCategory   *EquipmentCategory `json:"equipment_category,omitempty" gorm:"foreignKey:EquipmentCategoryID"`

	// Статус и обработка
	Status     string     `json:"status" gorm:"default:'active';type:varchar(20)"` // active, acknowledged, resolved
	ReadAt     *time.Time `json:"read_at"`
	ResolvedAt *time.Time `json:"resolved_at"`

	// Ответственное лицо
	AssignedUserID *uint `json:"assigned_user_id"`
	AssignedUser   *User `json:"assigned_user,omitempty" gorm:"foreignKey:AssignedUserID"`

	// Дополнительные данные
	Metadata string `json:"metadata" gorm:"type:jsonb"` // Дополнительные данные в JSON

	// Для мультитенантности
	CompanyID uint `json:"company_id" gorm:"index"`
}

// TableName задает имя таблицы для модели StockAlert
func (StockAlert) TableName() string {
	return "stock_alerts"
}

// IsActive проверяет, активно ли уведомление
func (sa *StockAlert) IsActive() bool {
	return sa.Status == "active"
}

// GetSeverityDisplayName возвращает читаемое название уровня важности
func (sa *StockAlert) GetSeverityDisplayName() string {
	severityMap := map[string]string{
		"low":      "Низкий",
		"medium":   "Средний",
		"high":     "Высокий",
		"critical": "Критический",
	}
	if displayName, exists := severityMap[sa.Severity]; exists {
		return displayName
	}
	return sa.Severity
}
