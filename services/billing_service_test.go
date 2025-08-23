package services

import (
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBillingTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("Failed to connect to test database")
	}

	// Создаем таблицы вручную для совместимости с SQLite
	err = db.Exec(`
		CREATE TABLE companies (
			id TEXT PRIMARY KEY,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT NOT NULL,
			database_schema TEXT NOT NULL,
			domain TEXT,
			is_active BOOLEAN DEFAULT TRUE
		)
	`).Error
	if err != nil {
		panic("Failed to create companies table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE billing_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			price DECIMAL(10,2) NOT NULL,
			currency TEXT DEFAULT 'RUB',
			billing_period TEXT DEFAULT 'monthly',
			max_devices INTEGER DEFAULT 0,
			max_users INTEGER DEFAULT 0,
			max_storage INTEGER DEFAULT 0,
			has_analytics BOOLEAN DEFAULT FALSE,
			has_api BOOLEAN DEFAULT FALSE,
			has_support BOOLEAN DEFAULT FALSE,
			has_custom_domain BOOLEAN DEFAULT FALSE,
			is_active BOOLEAN DEFAULT TRUE,
			is_popular BOOLEAN DEFAULT FALSE,
			company_id TEXT
		)
	`).Error
	if err != nil {
		panic("Failed to create billing_plans table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE tariff_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			price DECIMAL(10,2) NOT NULL,
			currency TEXT DEFAULT 'RUB',
			billing_period TEXT DEFAULT 'monthly',
			max_devices INTEGER DEFAULT 0,
			max_users INTEGER DEFAULT 0,
			max_storage INTEGER DEFAULT 0,
			has_analytics BOOLEAN DEFAULT FALSE,
			has_api BOOLEAN DEFAULT FALSE,
			has_support BOOLEAN DEFAULT FALSE,
			has_custom_domain BOOLEAN DEFAULT FALSE,
			is_active BOOLEAN DEFAULT TRUE,
			is_popular BOOLEAN DEFAULT FALSE,
			company_id TEXT,
			setup_fee DECIMAL(10,2) DEFAULT 0,
			minimum_period INTEGER DEFAULT 1,
			discount_percent DECIMAL(5,2) DEFAULT 0,
			is_promotional BOOLEAN DEFAULT FALSE,
			promotional_until DATETIME,
			price_per_object DECIMAL(10,2),
			free_objects_count INTEGER DEFAULT 0,
			inactive_price_ratio DECIMAL(3,2) DEFAULT 0.5
		)
	`).Error
	if err != nil {
		panic("Failed to create tariff_plans table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE contracts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			number TEXT UNIQUE NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			company_id TEXT NOT NULL,
			client_name TEXT NOT NULL,
			start_date DATETIME NOT NULL,
			end_date DATETIME NOT NULL,
			tariff_plan_id INTEGER NOT NULL,
			total_amount DECIMAL(15,2),
			currency TEXT DEFAULT 'RUB',
			status TEXT DEFAULT 'draft',
			is_active BOOLEAN DEFAULT TRUE
		)
	`).Error
	if err != nil {
		panic("Failed to create contracts table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE objects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			is_active BOOLEAN DEFAULT TRUE,
			contract_id INTEGER NOT NULL
		)
	`).Error
	if err != nil {
		panic("Failed to create objects table: " + err.Error())
	}

	err = db.Exec(`
		CREATE TABLE billing_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			company_id TEXT UNIQUE NOT NULL,
			default_tax_rate DECIMAL(5,2) DEFAULT 20,
			tax_included BOOLEAN DEFAULT FALSE,
			enable_inactive_discounts BOOLEAN DEFAULT TRUE,
			inactive_discount_ratio DECIMAL(3,2) DEFAULT 0.5,
			invoice_payment_term_days INTEGER DEFAULT 14,
			invoice_number_prefix TEXT DEFAULT 'INV',
			currency TEXT DEFAULT 'RUB'
		)
	`).Error
	if err != nil {
		panic("Failed to create billing_settings table: " + err.Error())
	}

	return db
}

func TestBillingService_CalculateBillingForContract(t *testing.T) {
	db := setupBillingTestDB()
	billingService := &BillingService{db: db}

	// Создаем тестовые данные
	company := &models.Company{
		Name:   "Test Company",
		Domain: "test.com",
	}
	db.Create(company)

	tariffPlan := &models.TariffPlan{
		BillingPlan: models.BillingPlan{
			Name:     "Basic Plan",
			Price:    decimal.NewFromFloat(1000.0),
			Currency: "RUB",
		},
		PricePerObject:     decimal.NewFromFloat(100),
		FreeObjectsCount:   2,
		InactivePriceRatio: decimal.NewFromFloat(0.5),
	}
	db.Create(tariffPlan)

	contract := &models.Contract{
		Number:       "TEST-001",
		Title:        "Test Contract",
		ClientName:   "Test Client",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		CompanyID:    company.ID,
		Status:       "active",
	}
	db.Create(contract)

	// Создаем объекты
	objects := []models.Object{
		{
			Name:       "Active Object 1",
			Status:     "active",
			IsActive:   true,
			ContractID: contract.ID,
		},
		{
			Name:       "Active Object 2",
			Status:     "active",
			IsActive:   true,
			ContractID: contract.ID,
		},
		{
			Name:       "Active Object 3",
			Status:     "active",
			IsActive:   true,
			ContractID: contract.ID,
		},
		{
			Name:       "Inactive Object 1",
			Status:     "inactive",
			IsActive:   false,
			ContractID: contract.ID,
		},
	}

	for _, obj := range objects {
		db.Create(&obj)
	}

	// Создаем настройки биллинга
	settings := &models.BillingSettings{
		CompanyID:               company.ID,
		DefaultTaxRate:          decimal.NewFromFloat(20),
		TaxIncluded:             false,
		EnableInactiveDiscounts: true,
		InactiveDiscountRatio:   decimal.NewFromFloat(0.5),
	}
	db.Create(settings)

	// Тестируем расчет
	periodStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	result, err := billingService.CalculateBillingForContract(contract.ID, periodStart, periodEnd)

	// Проверяем результат
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, contract.ID, result.ContractID)
	assert.Equal(t, company.ID, result.CompanyID)
	assert.Equal(t, 3, result.ActiveObjects)
	assert.Equal(t, 1, result.InactiveObjects)

	// Проверяем расчеты:
	// Базовая стоимость: 1000 RUB
	assert.Equal(t, decimal.NewFromFloat(1000), result.BaseAmount)

	// Стоимость объектов:
	// 3 активных - 2 бесплатных = 1 платный * 100 = 100
	// 1 неактивный * 100 * 0.5 = 50
	// Итого: 150
	expectedObjectsAmount := decimal.NewFromFloat(150)
	assert.Equal(t, expectedObjectsAmount, result.ObjectsAmount)

	// Промежуточная сумма: 1000 + 150 = 1150
	expectedSubtotal := decimal.NewFromFloat(1150)
	assert.Equal(t, expectedSubtotal, result.SubtotalAmount)

	// НДС 20%: 1150 * 0.2 = 230
	expectedTaxAmount := decimal.NewFromFloat(230)
	assert.Equal(t, expectedTaxAmount, result.TaxAmount)

	// Итого: 1150 + 230 = 1380
	expectedTotalAmount := decimal.NewFromFloat(1380)
	assert.Equal(t, expectedTotalAmount, result.TotalAmount)

	// Проверяем позиции
	assert.Len(t, result.Items, 3) // Подписка + активные объекты + неактивные объекты
}

func TestBillingService_GenerateInvoiceForContract(t *testing.T) {
	db := setupBillingTestDB()
	billingService := &BillingService{db: db}

	// Создаем тестовые данные (аналогично предыдущему тесту)
	company := &models.Company{
		Name:   "Test Company",
		Domain: "test.com",
	}
	db.Create(company)

	tariffPlan := &models.TariffPlan{
		BillingPlan: models.BillingPlan{
			Name:     "Basic Plan",
			Price:    decimal.NewFromFloat(1000.0),
			Currency: "RUB",
		},
		PricePerObject:     decimal.NewFromFloat(100),
		FreeObjectsCount:   1,
		InactivePriceRatio: decimal.NewFromFloat(0.5),
	}
	db.Create(tariffPlan)

	contract := &models.Contract{
		Number:       "TEST-001",
		Title:        "Test Contract",
		ClientName:   "Test Client",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now().AddDate(1, 0, 0),
		TariffPlanID: tariffPlan.ID,
		CompanyID:    company.ID,
		Status:       "active",
	}
	db.Create(contract)

	// Создаем объекты
	objects := []models.Object{
		{
			Name:       "Active Object 1",
			Status:     "active",
			IsActive:   true,
			ContractID: contract.ID,
		},
		{
			Name:       "Active Object 2",
			Status:     "active",
			IsActive:   true,
			ContractID: contract.ID,
		},
	}

	for _, obj := range objects {
		db.Create(&obj)
	}

	// Создаем настройки биллинга
	settings := &models.BillingSettings{
		CompanyID:              company.ID,
		InvoicePaymentTermDays: 14,
		DefaultTaxRate:         decimal.NewFromFloat(20),
		TaxIncluded:            false,
		InvoiceNumberPrefix:    "TEST",
		Currency:               "RUB",
	}
	db.Create(settings)

	// Генерируем счет
	periodStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	invoice, err := billingService.GenerateInvoiceForContract(contract.ID, periodStart, periodEnd)

	// Проверяем результат
	assert.NoError(t, err)
	assert.NotNil(t, invoice)
	assert.NotEmpty(t, invoice.Number)
	assert.Equal(t, "draft", invoice.Status)
	assert.Equal(t, company.ID, invoice.CompanyID)
	assert.Equal(t, contract.ID, *invoice.ContractID)
	assert.Equal(t, tariffPlan.ID, invoice.TariffPlanID)

	// Проверяем даты
	assert.Equal(t, periodStart, invoice.BillingPeriodStart)
	assert.Equal(t, periodEnd, invoice.BillingPeriodEnd)
	assert.True(t, invoice.DueDate.After(invoice.InvoiceDate))

	// Проверяем суммы
	// Базовая стоимость: 1000
	// Объекты: 2 активных - 1 бесплатный = 1 * 100 = 100
	// Промежуточная сумма: 1100
	// НДС 20%: 220
	// Итого: 1320
	expectedTotal := decimal.NewFromFloat(1320)
	assert.Equal(t, expectedTotal, invoice.TotalAmount)

	// Проверяем позиции счета
	assert.Len(t, invoice.Items, 2) // Подписка + активные объекты

	// Проверяем создание записи в истории
	var historyCount int64
	db.Model(&models.BillingHistory{}).Where("invoice_id = ?", invoice.ID).Count(&historyCount)
	assert.Equal(t, int64(1), historyCount)
}

func TestBillingService_ProcessPayment(t *testing.T) {
	db := setupBillingTestDB()
	billingService := &BillingService{db: db}

	// Создаем тестовый счет
	company := &models.Company{
		Name:   "Test Company",
		Domain: "test.com",
	}
	db.Create(company)

	invoice := &models.Invoice{
		Number:      "TEST-001",
		Title:       "Test Invoice",
		InvoiceDate: time.Now(),
		DueDate:     time.Now().AddDate(0, 0, 14),
		CompanyID:   company.ID,
		TotalAmount: decimal.NewFromFloat(1000),
		PaidAmount:  decimal.Zero,
		Currency:    "RUB",
		Status:      "draft",
	}
	db.Create(invoice)

	// Тест частичной оплаты
	err := billingService.ProcessPayment(invoice.ID, decimal.NewFromFloat(500), "bank_transfer", "Частичная оплата")
	assert.NoError(t, err)

	// Проверяем обновление счета
	db.First(invoice, invoice.ID)
	assert.Equal(t, decimal.NewFromFloat(500), invoice.PaidAmount)
	assert.Equal(t, "partially_paid", invoice.Status)
	assert.Nil(t, invoice.PaidAt)

	// Тест полной оплаты
	err = billingService.ProcessPayment(invoice.ID, decimal.NewFromFloat(500), "bank_transfer", "Доплата")
	assert.NoError(t, err)

	// Проверяем обновление счета
	db.First(invoice, invoice.ID)
	assert.Equal(t, decimal.NewFromFloat(1000), invoice.PaidAmount)
	assert.Equal(t, "paid", invoice.Status)
	assert.NotNil(t, invoice.PaidAt)

	// Проверяем создание записей в истории
	var historyCount int64
	db.Model(&models.BillingHistory{}).Where("invoice_id = ?", invoice.ID).Count(&historyCount)
	assert.Equal(t, int64(2), historyCount) // Два платежа
}

func TestBillingService_CancelInvoice(t *testing.T) {
	db := setupBillingTestDB()
	billingService := &BillingService{db: db}

	// Создаем тестовый счет
	company := &models.Company{
		Name:   "Test Company",
		Domain: "test.com",
	}
	db.Create(company)

	invoice := &models.Invoice{
		Number:      "TEST-001",
		Title:       "Test Invoice",
		InvoiceDate: time.Now(),
		DueDate:     time.Now().AddDate(0, 0, 14),
		CompanyID:   company.ID,
		TotalAmount: decimal.NewFromFloat(1000),
		PaidAmount:  decimal.Zero,
		Currency:    "RUB",
		Status:      "draft",
	}
	db.Create(invoice)

	// Отменяем счет
	reason := "Ошибка в расчетах"
	err := billingService.CancelInvoice(invoice.ID, reason)
	assert.NoError(t, err)

	// Проверяем обновление счета
	db.First(invoice, invoice.ID)
	assert.Equal(t, "cancelled", invoice.Status)
	assert.Equal(t, reason, invoice.Notes)

	// Проверяем создание записи в истории
	var historyCount int64
	db.Model(&models.BillingHistory{}).Where("invoice_id = ? AND operation = ?", invoice.ID, "invoice_cancelled").Count(&historyCount)
	assert.Equal(t, int64(1), historyCount)

	// Тест попытки отменить уже отмененный счет
	err = billingService.CancelInvoice(invoice.ID, "Повторная отмена")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "уже отменен")
}

func TestBillingService_GetOverdueInvoices(t *testing.T) {
	db := setupBillingTestDB()
	billingService := &BillingService{db: db}

	// Создаем тестовую компанию
	company := &models.Company{
		Name:   "Test Company",
		Domain: "test.com",
	}
	db.Create(company)

	// Создаем просроченный счет
	overdueInvoice := &models.Invoice{
		Number:      "OVERDUE-001",
		Title:       "Overdue Invoice",
		InvoiceDate: time.Now().AddDate(0, 0, -30),
		DueDate:     time.Now().AddDate(0, 0, -5), // Просрочен на 5 дней
		CompanyID:   company.ID,
		TotalAmount: decimal.NewFromFloat(1000),
		PaidAmount:  decimal.Zero,
		Currency:    "RUB",
		Status:      "sent",
	}
	db.Create(overdueInvoice)

	// Создаем не просроченный счет
	currentInvoice := &models.Invoice{
		Number:      "CURRENT-001",
		Title:       "Current Invoice",
		InvoiceDate: time.Now(),
		DueDate:     time.Now().AddDate(0, 0, 14),
		CompanyID:   company.ID,
		TotalAmount: decimal.NewFromFloat(1000),
		PaidAmount:  decimal.Zero,
		Currency:    "RUB",
		Status:      "sent",
	}
	db.Create(currentInvoice)

	// Создаем оплаченный просроченный счет (не должен попасть в результат)
	paidInvoice := &models.Invoice{
		Number:      "PAID-001",
		Title:       "Paid Invoice",
		InvoiceDate: time.Now().AddDate(0, 0, -30),
		DueDate:     time.Now().AddDate(0, 0, -5),
		CompanyID:   company.ID,
		TotalAmount: decimal.NewFromFloat(1000),
		PaidAmount:  decimal.NewFromFloat(1000),
		Currency:    "RUB",
		Status:      "paid",
	}
	db.Create(paidInvoice)

	// Получаем просроченные счета
	overdueInvoices, err := billingService.GetOverdueInvoices(&company.ID)
	assert.NoError(t, err)
	assert.Len(t, overdueInvoices, 1)
	assert.Equal(t, overdueInvoice.ID, overdueInvoices[0].ID)

	// Получаем все просроченные счета (без фильтра по компании)
	allOverdueInvoices, err := billingService.GetOverdueInvoices(nil)
	assert.NoError(t, err)
	assert.Len(t, allOverdueInvoices, 1)
}

func TestBillingService_CalculateObjectsAmount(t *testing.T) {
	billingService := &BillingService{}

	tariff := &models.TariffPlan{
		PricePerObject:     decimal.NewFromFloat(100),
		FreeObjectsCount:   2,
		InactivePriceRatio: decimal.NewFromFloat(0.5),
	}

	tests := []struct {
		name           string
		activeCount    int
		inactiveCount  int
		enableDiscount bool
		expectedAmount string
	}{
		{
			name:           "Все объекты бесплатные",
			activeCount:    1,
			inactiveCount:  1,
			enableDiscount: true,
			expectedAmount: "0",
		},
		{
			name:           "Только активные объекты",
			activeCount:    4,
			inactiveCount:  0,
			enableDiscount: true,
			expectedAmount: "200", // (4-2) * 100
		},
		{
			name:           "Активные и неактивные со скидкой",
			activeCount:    3,
			inactiveCount:  2,
			enableDiscount: true,
			expectedAmount: "150", // (3-2)*100 + 2*100*0.5
		},
		{
			name:           "Активные и неактивные без скидки",
			activeCount:    3,
			inactiveCount:  2,
			enableDiscount: false,
			expectedAmount: "300", // (5-2)*100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := billingService.calculateObjectsAmount(tariff, tt.activeCount, tt.inactiveCount, tt.enableDiscount)
			expected, _ := decimal.NewFromString(tt.expectedAmount)
			assert.Equal(t, expected, result)
		})
	}
}
