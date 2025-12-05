package models

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Contract представляет договор в системе
type Contract struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля договора
	Number      string `json:"number" gorm:"uniqueIndex;not null;type:varchar(50)"`
	Title       string `json:"title" gorm:"type:varchar(200)"` // Необязательное поле для однотипных услуг
	Description string `json:"description" gorm:"type:text"`

	// Связь с администратором и компанией (мультитенантность)
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	// Тип договора
	ContractType string `json:"contract_type" gorm:"type:varchar(20);default:'client'"` // client или partner

	// Для партнерских договоров - ID учетной записи партнера
	// Все объекты из этой учетной записи будут тарифицироваться
	PartnerCompanyID *uint    `json:"partner_company_id" gorm:"index"`
	PartnerCompany   *Company `json:"partner_company,omitempty" gorm:"foreignKey:PartnerCompanyID;constraint:-"`

	// Клиент
	ClientType      string `json:"client_type" gorm:"type:varchar(50)"` // organization, individual_entrepreneur, physical_person
	ClientName      string `json:"client_name" gorm:"not null;type:varchar(200)"`
	ClientShortName string `json:"client_short_name" gorm:"type:varchar(200)"` // Сокращенное название с ОПФ (для организаций)
	ClientINN       string `json:"client_inn" gorm:"type:varchar(20)"`
	ClientKPP       string `json:"client_kpp" gorm:"type:varchar(20)"`
	ClientEmail     string `json:"client_email" gorm:"type:varchar(100)"`
	ClientPhone     string `json:"client_phone" gorm:"type:varchar(20)"`
	ClientAddress   string `json:"client_address" gorm:"type:text"`

	// Дополнительные поля для организаций
	ClientLegalAddress  string `json:"client_legal_address" gorm:"type:text"`
	ClientPostalAddress string `json:"client_postal_address" gorm:"type:text"`
	ClientOGRN          string `json:"client_ogrn" gorm:"type:varchar(20)"`
	ClientOKPO          string `json:"client_okpo" gorm:"type:varchar(20)"`
	ClientDirector      string `json:"client_director" gorm:"type:varchar(200)"`
	ClientBasedOn       string `json:"client_based_on" gorm:"type:varchar(200)"` // Действует на основании
	ClientWebsite       string `json:"client_website" gorm:"type:varchar(200)"`

	// Банковские реквизиты
	ClientBankName                 string `json:"client_bank_name" gorm:"type:varchar(200)"`
	ClientBankBIK                  string `json:"client_bank_bik" gorm:"type:varchar(20)"`
	ClientBankCorrespondentAccount string `json:"client_bank_correspondent_account" gorm:"type:varchar(20)"`
	ClientBankAccount              string `json:"client_bank_account" gorm:"type:varchar(20)"`
	ClientBankRecipient            string `json:"client_bank_recipient" gorm:"type:varchar(200)"`

	// Поля для физических лиц
	ClientPassportSeries         string `json:"client_passport_series" gorm:"type:varchar(10)"`
	ClientPassportNumber         string `json:"client_passport_number" gorm:"type:varchar(20)"`
	ClientPassportIssuedBy       string `json:"client_passport_issued_by" gorm:"type:text"`
	ClientPassportIssueDate      string `json:"client_passport_issue_date" gorm:"type:varchar(20)"`
	ClientPassportDepartmentCode string `json:"client_passport_department_code" gorm:"type:varchar(10)"`
	ClientRegistrationAddress    string `json:"client_registration_address" gorm:"type:text"`
	ClientActualAddress          string `json:"client_actual_address" gorm:"type:text"`
	ClientSNILS                  string `json:"client_snils" gorm:"type:varchar(20)"`
	ClientOGRNIP                 string `json:"client_ogrnip" gorm:"column:client_ogrn_ip;type:varchar(20)"`

	// Даты договора
	StartDate *time.Time `json:"start_date" gorm:"default:NULL"` // Опционально, будет установлено через подписку
	EndDate   *time.Time `json:"end_date" gorm:"default:NULL"`   // Опционально, будет установлено через подписку
	SignedAt  *time.Time `json:"signed_at"`

	// Тарификация (опционально, будет привязан через подписку)
	TariffPlanID *uint       `json:"tariff_plan_id" gorm:"default:NULL"`
	TariffPlan   BillingPlan `json:"tariff_plan" gorm:"foreignKey:TariffPlanID;constraint:-"`

	// Стоимость
	TotalAmount decimal.Decimal `json:"total_amount" gorm:"type:decimal(15,2)"`
	Currency    string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Скидки (для партнерских договоров) - используется только один тип
	DiscountType          string          `json:"discount_type" gorm:"type:varchar(20);default:'none'"`       // none, manual_percent, manual_fixed, auto
	ManualDiscountPercent decimal.Decimal `json:"manual_discount_percent" gorm:"type:decimal(5,2);default:0"` // 0-100 (%)
	ManualDiscountFixed   decimal.Decimal `json:"manual_discount_fixed" gorm:"type:decimal(12,2);default:0"`  // Фиксированная скидка (₽)
	UseAutoDiscount       bool            `json:"use_auto_discount" gorm:"default:false"`                     // Использовать автоматические скидки

	// Страны и реквизиты (поля не используются в текущей версии БД)
	SellerCountryCode string           `json:"seller_country_code" gorm:"-"` // Страна продавца (не используется)
	BuyerCountryCode  string           `json:"buyer_country_code" gorm:"-"`  // Страна покупателя (не используется)
	NDSRateOverride   *decimal.Decimal `json:"nds_rate_override" gorm:"-"`   // Переопределение ставки НДС (не используется)

	// Статус договора
	Status   string `json:"status" gorm:"default:'draft';type:varchar(20)"` // draft, active, expired, cancelled, suspended
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Настройки автоматической пролонгации (настраиваются через подписку)
	IsAutoRenew          bool `json:"is_auto_renew" gorm:"-"`          // Автоматическая пролонгация договора (настраивается через подписку)
	ContractPeriodMonths *int `json:"contract_period_months" gorm:"-"` // Период договора в месяцах (настраивается через подписку)

	// Настройки уведомлений
	NotifyBefore int `json:"notify_before" gorm:"default:30"` // За сколько дней уведомлять об истечении

	// Дополнительная информация
	Notes      string `json:"notes" gorm:"type:text"`
	ExternalID string `json:"external_id" gorm:"type:varchar(100)"` // ID во внешних системах (1С, Битрикс24)

	// Связи
	Appendices      []ContractAppendix `json:"appendices,omitempty" gorm:"foreignKey:ContractID;constraint:-"`
	Objects         []Object           `json:"objects,omitempty" gorm:"foreignKey:ContractID;constraint:-"`          // Прямая связь (для обратной совместимости)
	ContractObjects []ContractObject   `json:"contract_objects,omitempty" gorm:"foreignKey:ContractID;constraint:-"` // Связи через junction table
}

// TableName задает имя таблицы для модели Contract
func (Contract) TableName() string {
	return "contracts"
}

// IsExpired проверяет, истек ли договор
func (c *Contract) IsExpired() bool {
	if c.EndDate == nil {
		return false // Если дата окончания не установлена, договор не истек
	}
	return time.Now().After(*c.EndDate)
}

// IsExpiringSoon проверяет, истекает ли договор скоро
func (c *Contract) IsExpiringSoon() bool {
	if c.EndDate == nil {
		return false // Если дата окончания не установлена, договор не истекает скоро
	}
	notifyDate := c.EndDate.AddDate(0, 0, -c.NotifyBefore)
	return time.Now().After(notifyDate) && !c.IsExpired()
}

// GetDaysUntilExpiry возвращает количество дней до истечения договора
func (c *Contract) GetDaysUntilExpiry() int {
	if c.EndDate == nil {
		return 0 // Если дата окончания не установлена, возвращаем 0
	}
	if c.IsExpired() {
		return 0
	}
	duration := c.EndDate.Sub(time.Now())
	return int(duration.Hours() / 24)
}

// CalculateAutoDiscount рассчитывает автоматическую скидку на основе количества активных объектов
// Правила: >1000 объектов = 10%, >2000 = 20%, >4000 = 30%
func CalculateAutoDiscount(activeObjectsCount int) decimal.Decimal {
	if activeObjectsCount >= 4000 {
		return decimal.NewFromInt(30) // 30%
	} else if activeObjectsCount >= 2000 {
		return decimal.NewFromInt(20) // 20%
	} else if activeObjectsCount >= 1000 {
		return decimal.NewFromInt(10) // 10%
	}
	return decimal.Zero // Нет скидки
}

// GetDiscountPercent возвращает применяемый процент скидки
func (c *Contract) GetDiscountPercent(activeObjectsCount int) decimal.Decimal {
	if c.DiscountType == "manual_percent" || c.DiscountType == "manual" {
		return c.ManualDiscountPercent
	} else if c.DiscountType == "auto" || c.UseAutoDiscount {
		return CalculateAutoDiscount(activeObjectsCount)
	}
	return decimal.Zero
}

// GetDiscountFixed возвращает фиксированную сумму скидки
func (c *Contract) GetDiscountFixed() decimal.Decimal {
	if c.DiscountType == "manual_fixed" {
		return c.ManualDiscountFixed
	}
	return decimal.Zero
}

// GetDiscountType возвращает тип скидки для отображения
func (c *Contract) GetDiscountType() string {
	if c.DiscountType == "manual_fixed" {
		return "fixed"
	} else if c.DiscountType == "manual_percent" || c.DiscountType == "manual" {
		return "percent"
	} else if c.DiscountType == "auto" || c.UseAutoDiscount {
		return "auto"
	}
	return "none"
}

// ContractAppendix представляет приложение к договору
type ContractAppendix struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Идентификатор учетной записи администратора для изоляции данных
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`

	// Связь с договором
	ContractID uint     `json:"contract_id" gorm:"not null;index"`
	Contract   Contract `json:"contract,omitempty" gorm:"foreignKey:ContractID;constraint:-"`

	// Основные поля приложения
	Number      string `json:"number" gorm:"not null;type:varchar(50)"`
	Title       string `json:"title" gorm:"not null;type:varchar(200)"`
	Description string `json:"description" gorm:"type:text"`

	// Даты приложения
	StartDate time.Time  `json:"start_date" gorm:"not null"`
	EndDate   time.Time  `json:"end_date" gorm:"not null"`
	SignedAt  *time.Time `json:"signed_at"`

	// Стоимость приложения
	Amount   decimal.Decimal `json:"amount" gorm:"type:decimal(15,2)"`
	Currency string          `json:"currency" gorm:"default:'RUB';type:varchar(3)"`

	// Статус приложения
	Status   string `json:"status" gorm:"default:'draft';type:varchar(20)"` // draft, active, expired, cancelled
	IsActive bool   `json:"is_active" gorm:"default:true"`

	// Дополнительная информация
	Notes      string `json:"notes" gorm:"type:text"`
	ExternalID string `json:"external_id" gorm:"type:varchar(100)"`
}

// TableName задает имя таблицы для модели ContractAppendix
func (ContractAppendix) TableName() string {
	return "contract_appendices"
}

// IsExpired проверяет, истекло ли приложение
func (ca *ContractAppendix) IsExpired() bool {
	return time.Now().After(ca.EndDate)
}

// TariffPlan представляет тарифный план (расширяет BillingPlan)
type TariffPlan struct {
	BillingPlan

	// Дополнительные поля для тарификации
	SetupFee         decimal.Decimal `json:"setup_fee" gorm:"type:decimal(10,2);default:0.00"`       // Плата за подключение
	MinimumPeriod    int             `json:"minimum_period" gorm:"default:1"`                        // Минимальный период в месяцах
	DiscountPercent  decimal.Decimal `json:"discount_percent" gorm:"type:decimal(5,2);default:0.00"` // Скидка в процентах
	IsPromotional    bool            `json:"is_promotional" gorm:"default:false"`                    // Акционный тариф
	PromotionalUntil *time.Time      `json:"promotional_until"`                                      // До какой даты действует акция

	// Тарификация по объектам
	PricePerObject   decimal.Decimal `json:"price_per_object" gorm:"type:decimal(10,2)"` // Цена за объект
	FreeObjectsCount int             `json:"free_objects_count" gorm:"default:0"`        // Количество бесплатных объектов

	// Специальные тарифы
	InactivePriceRatio decimal.Decimal `json:"inactive_price_ratio" gorm:"type:decimal(3,2);default:0.50"` // Коэффициент для неактивных объектов
}

// TableName задает имя таблицы для модели TariffPlan
func (TariffPlan) TableName() string {
	return "tariff_plans"
}

// CalculateObjectPrice рассчитывает стоимость для определенного количества объектов
func (tp *TariffPlan) CalculateObjectPrice(objectCount int, inactiveCount int) decimal.Decimal {
	if objectCount <= 0 {
		return decimal.Zero
	}

	// Учитываем бесплатные объекты
	billableObjects := objectCount - tp.FreeObjectsCount
	if billableObjects <= 0 {
		return decimal.Zero
	}

	// Основная стоимость
	activeObjects := billableObjects - inactiveCount
	totalPrice := tp.PricePerObject.Mul(decimal.NewFromInt(int64(activeObjects)))

	// Добавляем стоимость неактивных объектов со скидкой
	if inactiveCount > 0 {
		inactivePrice := tp.PricePerObject.Mul(tp.InactivePriceRatio).Mul(decimal.NewFromInt(int64(inactiveCount)))
		totalPrice = totalPrice.Add(inactivePrice)
	}

	// Применяем скидку
	if tp.DiscountPercent.GreaterThan(decimal.Zero) {
		discount := totalPrice.Mul(tp.DiscountPercent).Div(decimal.NewFromInt(100))
		totalPrice = totalPrice.Sub(discount)
	}

	return totalPrice
}

// ContractNumerator представляет нумератор для договоров
type ContractNumerator struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Связь с администратором и компанией
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index"`
	CompanyID      uint `json:"company_id" gorm:"not null;index"`

	// Основные поля нумератора
	Name        string `json:"name" gorm:"not null;type:varchar(100)"`     // Название нумератора
	Prefix      string `json:"prefix" gorm:"not null;type:varchar(10)"`    // Префикс (например "AX")
	Template    string `json:"template" gorm:"not null;type:varchar(200)"` // Шаблон номера (например "{PREFIX}-{DAY}{MONTH}{YEAR}/{CLIENT_ID}")
	Description string `json:"description" gorm:"type:text"`               // Описание нумератора

	// Счетчик для последовательных номеров
	CounterValue int `json:"counter_value" gorm:"default:0"` // Текущее значение счетчика

	// Настройки
	IsDefault   bool   `json:"is_default" gorm:"default:false"`      // Нумератор по умолчанию
	IsActive    bool   `json:"is_active" gorm:"default:true"`        // Активен ли нумератор
	AutoReset   bool   `json:"auto_reset" gorm:"default:false"`      // Автоматически сбрасывать счетчик (например, в новом году)
	ResetPeriod string `json:"reset_period" gorm:"type:varchar(20)"` // Период сброса: yearly, monthly, never

	// Дополнительные поля
	Notes string `json:"notes" gorm:"type:text"`
}

// TableName задает имя таблицы для модели ContractNumerator
func (ContractNumerator) TableName() string {
	return "contract_numerators"
}

// GenerateNumber генерирует номер договора по шаблону
func (cn *ContractNumerator) GenerateNumber(clientID uint, companyID uint, contractID uint) (string, error) {
	now := time.Now()
	day := now.Day()
	month := now.Month()
	year := now.Year()

	// Парсим шаблон и заменяем плейсхолдеры
	result := cn.Template

	// Заменяем доступные плейсхолдеры
	// SEQ начинается с 001 (3 цифры с ведущими нулями)
	// Используем CounterValue + 1, чтобы первый номер был 001, а не 000
	seqValue := cn.CounterValue + 1
	if seqValue < 1 {
		seqValue = 1 // Гарантируем, что первый номер будет 001
	}

	// Генерируем случайное число от 0 до 99 (2 цифры)
	randomValue := rand.Intn(100)

	replacements := map[string]string{
		"{PREFIX}":     cn.Prefix,
		"{SEQ}":        fmt.Sprintf("%03d", seqValue), // 3 цифры: 001, 002, 003...
		"{DAY}":        fmt.Sprintf("%02d", day),
		"{MONTH}":      fmt.Sprintf("%02d", int(month)),
		"{YEAR}":       fmt.Sprintf("%d", year),
		"{YEAR_SHORT}": fmt.Sprintf("%02d", year%100),    // 2 цифры: 24, 25...
		"{RANDOM}":     fmt.Sprintf("%02d", randomValue), // 2 цифры: 00, 01, 02...99
		// {ID}, {COMPANY_ID}, {CLIENT_ID} удалены из нумератора
	}

	log.Printf("GenerateNumber: шаблон='%s', CounterValue=%d, seqValue=%d, randomValue=%d",
		cn.Template, cn.CounterValue, seqValue, randomValue)

	for placeholder, value := range replacements {
		oldResult := result
		result = replaceAll(result, placeholder, value)
		if oldResult != result {
			log.Printf("GenerateNumber: заменен '%s' на '%s', результат: '%s'", placeholder, value, result)
		}
	}

	log.Printf("GenerateNumber: итоговый номер: '%s'", result)
	return result, nil
}

// IncrementCounter увеличивает счетчик на 1
func (cn *ContractNumerator) IncrementCounter() {
	cn.CounterValue++
}

// ShouldResetCounter проверяет, нужно ли сбрасывать счетчик
func (cn *ContractNumerator) ShouldResetCounter() bool {
	if !cn.AutoReset {
		return false
	}

	switch cn.ResetPeriod {
	case "yearly":
		// Проверяем, изменился ли год (нужно хранить последний использованный год)
		return true // Упрощенная версия
	case "monthly":
		return true // Упрощенная версия
	case "never":
		return false
	default:
		return false
	}
}

// Helper function для замены всех вхождений
func replaceAll(s, old, new string) string {
	result := s
	for {
		newResult := replaceAllOnce(result, old, new)
		if newResult == result {
			break
		}
		result = newResult
	}
	return result
}

func replaceAllOnce(s, old, new string) string {
	for i := 0; i <= len(s)-len(old); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}
