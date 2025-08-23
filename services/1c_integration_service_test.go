package services

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"

	"backend_axenta/models"
	"backend_axenta/testutils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOneCIntegrationTest() (*gorm.DB, *OneCIntegrationService, *OneCClientMock) {
	// Создаем временную БД в памяти используя общую функцию
	db, err := testutils.SetupTestDB()
	if err != nil {
		panic("failed to connect database")
	}

	// Создаем мок клиент
	logger := log.New(os.Stdout, "TEST: ", log.LstdFlags)
	mockClient := NewOneCClientMock(logger)
	mockClient.SetupMockData()

	// Создаем кэш сервис (можно использовать простой мок)
	cacheService := NewCacheService(nil, logger)

	// Создаем сервис интеграции
	integrationService := NewOneCIntegrationService(db, nil, cacheService, logger)
	integrationService.SetOneCClient(mockClient)

	return db, integrationService, mockClient
}

func createTestCompany(db *gorm.DB) *models.Company {
	company := &models.Company{
		Name:      "Тестовая компания",
		Domain:    "test.example.com",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	db.Create(company)
	return company
}

func createTestIntegration(db *gorm.DB, companyID uuid.UUID) *models.Integration {
	config := OneCIntegrationConfig{
		CompanyID:         companyID,
		BaseURL:           "http://test.1c.local",
		Username:          "testuser",
		Password:          "testpass",
		Database:          "testdb",
		APIVersion:        "v1",
		OrganizationCode:  "ORG001",
		BankAccountCode:   "BANK001",
		PaymentTypeCode:   "PAYMENT001",
		ContractTypeCode:  "CONTRACT001",
		CurrencyCode:      "RUB",
		AutoExportEnabled: true,
		AutoImportEnabled: true,
		SyncInterval:      60,
	}

	configJSON, _ := json.Marshal(config)
	integration := &models.Integration{
		CompanyID:       companyID,
		IntegrationType: "1c",
		Name:            "Интеграция с 1С",
		Description:     "Тестовая интеграция",
		Settings:        string(configJSON),
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	db.Create(integration)
	return integration
}

func createTestInvoice(db *gorm.DB, companyID uuid.UUID, status string) *models.Invoice {
	invoice := &models.Invoice{
		Number:             "INV-TEST-001",
		Title:              "Тестовый счет",
		Description:        "Описание тестового счета",
		InvoiceDate:        time.Now(),
		DueDate:            time.Now().AddDate(0, 0, 14),
		CompanyID:          companyID,
		BillingPeriodStart: time.Now().AddDate(0, -1, 0),
		BillingPeriodEnd:   time.Now(),
		SubtotalAmount:     decimal.NewFromFloat(10000),
		TaxRate:            decimal.NewFromFloat(20),
		TaxAmount:          decimal.NewFromFloat(2000),
		TotalAmount:        decimal.NewFromFloat(12000),
		Currency:           "RUB",
		Status:             status,
		PaidAmount:         decimal.Zero,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if status == "paid" {
		paidAt := time.Now()
		invoice.PaidAt = &paidAt
		invoice.PaidAmount = invoice.TotalAmount
	}

	db.Create(invoice)
	return invoice
}

func TestOneCIntegrationService_GetCredentials(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	ctx := context.Background()

	// Тест получения учетных данных
	credentials, err := service.GetCredentials(ctx, company.ID)
	require.NoError(t, err)
	assert.Equal(t, "http://test.1c.local", credentials.BaseURL)
	assert.Equal(t, "testuser", credentials.Username)
	assert.Equal(t, "testpass", credentials.Password)
	assert.Equal(t, "testdb", credentials.Database)
	assert.Equal(t, "v1", credentials.APIVersion)
}

func TestOneCIntegrationService_GetCredentials_NotFound(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)

	ctx := context.Background()

	// Тест получения несуществующих учетных данных
	_, err := service.GetCredentials(ctx, company.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "интеграция с 1С не настроена")
}

func TestOneCIntegrationService_ExportPaymentRegistry(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем тестовые оплаченные счета
	invoice1 := createTestInvoice(db, company.ID, "paid")
	invoice2 := createTestInvoice(db, company.ID, "paid")
	invoice2.Number = "INV-TEST-002"
	db.Save(invoice2)

	ctx := context.Background()
	invoices := []models.Invoice{*invoice1, *invoice2}
	registryNumber := "TEST-REG-001"

	// Экспортируем реестр
	err := service.ExportPaymentRegistry(ctx, company.ID, invoices, registryNumber)
	require.NoError(t, err)

	// Проверяем, что мок клиент был вызван
	exportedRegistries := mockClient.GetExportedRegistries()
	require.Len(t, exportedRegistries, 1)

	registry := exportedRegistries[0]
	assert.Equal(t, registryNumber, registry.RegistryNumber)
	assert.Equal(t, 2, registry.PaymentsCount)
	assert.Equal(t, 24000.0, registry.TotalAmount) // 12000 * 2
}

func TestOneCIntegrationService_ExportPaymentRegistry_Failure(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	invoice := createTestInvoice(db, company.ID, "paid")

	// Настраиваем мок на ошибку
	mockClient.SetFailure(true, "Ошибка подключения к 1С")

	ctx := context.Background()
	invoices := []models.Invoice{*invoice}
	registryNumber := "TEST-REG-FAIL"

	// Экспортируем реестр
	err := service.ExportPaymentRegistry(ctx, company.ID, invoices, registryNumber)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ошибка экспорта реестра платежей")

	// Проверяем, что ошибка была залогирована
	var errors []OneCIntegrationError
	db.Where("company_id = ? AND operation = ?", company.ID, "export_payment").Find(&errors)
	require.Len(t, errors, 1)
	assert.Equal(t, "EXPORT_ERROR", errors[0].ErrorCode)
}

func TestOneCIntegrationService_ImportCounterparties(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	ctx := context.Background()

	// Импортируем контрагентов
	err := service.ImportCounterparties(ctx, company.ID)
	require.NoError(t, err)

	// Проверяем, что контрагенты были созданы как пользователи
	var users []models.User
	db.Where("company_id = ? AND user_type = ?", company.ID, "client").Find(&users)

	// В моке есть 2 активных контрагента
	assert.Len(t, users, 2)

	// Проверяем данные первого пользователя
	user1 := users[0]
	assert.Equal(t, "ООО Тестовая компания 1", user1.Name)
	assert.Equal(t, "info@testcompany1.ru", user1.Email)
	assert.Equal(t, "+7 (495) 123-45-67", user1.Phone)
	assert.Equal(t, "counterparty-1", user1.ExternalID)
	assert.Equal(t, "1c", user1.ExternalSource)
	assert.True(t, user1.IsActive)
}

func TestOneCIntegrationService_ImportCounterparties_UpdateExisting(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем существующего пользователя
	existingUser := &models.User{
		CompanyID:      company.ID,
		Name:           "Старое имя",
		Email:          "info@testcompany1.ru",
		Phone:          "+7 (495) 123-45-67",
		UserType:       "client",
		ExternalID:     "counterparty-1",
		ExternalSource: "1c",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	db.Create(existingUser)

	ctx := context.Background()

	// Импортируем контрагентов
	err := service.ImportCounterparties(ctx, company.ID)
	require.NoError(t, err)

	// Проверяем, что пользователь был обновлен
	var updatedUser models.User
	db.Where("id = ?", existingUser.ID).First(&updatedUser)
	assert.Equal(t, "ООО Тестовая компания 1", updatedUser.Name) // Должно обновиться
	assert.Equal(t, "info@testcompany1.ru", updatedUser.Email)

	// Проверяем общее количество пользователей (должно быть 2: 1 обновленный + 1 новый)
	var totalUsers []models.User
	db.Where("company_id = ? AND user_type = ?", company.ID, "client").Find(&totalUsers)
	assert.Len(t, totalUsers, 2)
}

func TestOneCIntegrationService_SyncPaymentStatuses(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем неоплаченный счет
	invoice := createTestInvoice(db, company.ID, "sent")

	// Добавляем платеж в мок (проведенный)
	payment := &OneCPayment{
		ID:         "payment-test",
		Number:     "PAY-TEST",
		Date:       time.Now(),
		Posted:     true,
		Amount:     12000.0,
		ExternalID: "invoice_" + string(rune(invoice.ID)),
	}
	mockClient.AddPayment("invoice_"+string(rune(invoice.ID)), payment)

	ctx := context.Background()

	// Синхронизируем статусы платежей
	err := service.SyncPaymentStatuses(ctx, company.ID)
	require.NoError(t, err)

	// Проверяем, что статус счета обновился
	var updatedInvoice models.Invoice
	db.Where("id = ?", invoice.ID).First(&updatedInvoice)
	assert.Equal(t, "paid", updatedInvoice.Status)
	assert.NotNil(t, updatedInvoice.PaidAt)
	assert.Equal(t, decimal.NewFromFloat(12000.0), updatedInvoice.PaidAmount)
}

func TestOneCIntegrationService_TestConnection(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	ctx := context.Background()

	// Тест успешного подключения
	err := service.TestConnection(ctx, company.ID)
	require.NoError(t, err)

	// Тест неуспешного подключения
	mockClient.SetConnectionHealth(false)
	err = service.TestConnection(ctx, company.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "тест подключения к 1С не пройден")
}

func TestOneCIntegrationService_GetIntegrationErrors(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем тестовые ошибки
	error1 := OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "export_payment",
		EntityType:   "registry",
		EntityID:     "REG-001",
		ErrorCode:    "CONNECTION_ERROR",
		ErrorMessage: "Ошибка подключения",
		Resolved:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error1)

	error2 := OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "import_counterparty",
		EntityType:   "counterparty",
		EntityID:     "CP-001",
		ErrorCode:    "VALIDATION_ERROR",
		ErrorMessage: "Ошибка валидации",
		Resolved:     true,
		ResolvedAt:   &[]time.Time{time.Now()}[0],
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error2)

	ctx := context.Background()

	// Получаем нерешенные ошибки
	unresolvedErrors, err := service.GetIntegrationErrors(ctx, company.ID, false)
	require.NoError(t, err)
	assert.Len(t, unresolvedErrors, 1)
	assert.Equal(t, "CONNECTION_ERROR", unresolvedErrors[0].ErrorCode)

	// Получаем решенные ошибки
	resolvedErrors, err := service.GetIntegrationErrors(ctx, company.ID, true)
	require.NoError(t, err)
	assert.Len(t, resolvedErrors, 1)
	assert.Equal(t, "VALIDATION_ERROR", resolvedErrors[0].ErrorCode)
}

func TestOneCIntegrationService_ResolveError(t *testing.T) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)

	// Создаем тестовую ошибку
	error1 := OneCIntegrationError{
		CompanyID:    company.ID,
		Operation:    "export_payment",
		ErrorCode:    "CONNECTION_ERROR",
		ErrorMessage: "Ошибка подключения",
		Resolved:     false,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&error1)

	ctx := context.Background()

	// Разрешаем ошибку
	err := service.ResolveError(ctx, error1.ID)
	require.NoError(t, err)

	// Проверяем, что ошибка помечена как решенная
	var resolvedError OneCIntegrationError
	db.Where("id = ?", error1.ID).First(&resolvedError)
	assert.True(t, resolvedError.Resolved)
	assert.NotNil(t, resolvedError.ResolvedAt)
}

func TestOneCIntegrationService_ScheduleAutoExport(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем оплаченный счет за последний день
	invoice := createTestInvoice(db, company.ID, "paid")
	paidAt := time.Now().Add(-time.Hour * 12) // 12 часов назад
	invoice.PaidAt = &paidAt
	db.Save(invoice)

	ctx := context.Background()

	// Запускаем автоэкспорт
	err := service.ScheduleAutoExport(ctx, company.ID)
	require.NoError(t, err)

	// Проверяем, что реестр был экспортирован
	exportedRegistries := mockClient.GetExportedRegistries()
	require.Len(t, exportedRegistries, 1)

	registry := exportedRegistries[0]
	assert.Equal(t, 1, registry.PaymentsCount)
	assert.Contains(t, registry.RegistryNumber, "REG-")
}

func TestOneCIntegrationService_ScheduleAutoExport_NoNewPayments(t *testing.T) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем старый оплаченный счет (более 2 дней назад)
	invoice := createTestInvoice(db, company.ID, "paid")
	paidAt := time.Now().AddDate(0, 0, -3)
	invoice.PaidAt = &paidAt
	db.Save(invoice)

	ctx := context.Background()

	// Запускаем автоэкспорт
	err := service.ScheduleAutoExport(ctx, company.ID)
	require.NoError(t, err)

	// Проверяем, что реестр НЕ был экспортирован
	exportedRegistries := mockClient.GetExportedRegistries()
	assert.Len(t, exportedRegistries, 0)
}

// Benchmark тесты
func BenchmarkOneCIntegrationService_ImportCounterparties(b *testing.B) {
	db, service, mockClient := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	ctx := context.Background()

	// Добавляем больше контрагентов в мок для более реалистичного теста
	for i := 0; i < 100; i++ {
		mockClient.Counterparties = append(mockClient.Counterparties, OneCCounterparty{
			ID:          "counterparty-bench-" + string(rune(i)),
			Code:        "CP" + string(rune(i)),
			Description: "Контрагент " + string(rune(i)),
			Email:       "test" + string(rune(i)) + "@example.com",
			IsActive:    true,
		})
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Очищаем пользователей перед каждой итерацией
		db.Where("company_id = ?", company.ID).Delete(&models.User{})

		err := service.ImportCounterparties(ctx, company.ID)
		if err != nil {
			b.Fatalf("ImportCounterparties failed: %v", err)
		}
	}
}

func BenchmarkOneCIntegrationService_ExportPaymentRegistry(b *testing.B) {
	db, service, _ := setupOneCIntegrationTest()
	company := createTestCompany(db)
	createTestIntegration(db, company.ID)

	// Создаем множество оплаченных счетов
	var invoices []models.Invoice
	for i := 0; i < 50; i++ {
		invoice := createTestInvoice(db, company.ID, "paid")
		invoice.Number = "INV-BENCH-" + string(rune(i))
		db.Save(invoice)
		invoices = append(invoices, *invoice)
	}

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		registryNumber := "BENCH-REG-" + string(rune(i))
		err := service.ExportPaymentRegistry(ctx, company.ID, invoices, registryNumber)
		if err != nil {
			b.Fatalf("ExportPaymentRegistry failed: %v", err)
		}
	}
}
