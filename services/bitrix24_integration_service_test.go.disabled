package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDBForBitrix24() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("failed to connect database for testing")
	}

	// Мигрируем тестовые модели
	db.AutoMigrate(&models.Company{}, &Bitrix24SyncMapping{}, &models.IntegrationError{})
	db.AutoMigrate(&models.Object{}, &models.User{}, &models.Contract{})

	return db
}

func createBitrixTestCompany(db *gorm.DB) *models.Company {
	company := &models.Company{
		Name:               "Test Company",
		DatabaseSchema:     "test_schema",
		AxetnaLogin:        "test@example.com",
		AxetnaPassword:     "encrypted_password",
		Bitrix24WebhookURL: "https://test.bitrix24.com/rest/1/webhook_key/",
		IsActive:           true,
	}

	if err := db.Create(company).Error; err != nil {
		panic("failed to create test company: " + err.Error())
	}

	return company
}

func TestBitrix24IntegrationService_GetTenantCredentials(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	ctx := context.Background()

	// Тест успешного получения учетных данных
	credentials, err := service.GetTenantCredentials(ctx, company.ID)
	if err != nil {
		t.Fatalf("Ошибка получения учетных данных: %v", err)
	}

	if credentials.WebhookURL != company.Bitrix24WebhookURL {
		t.Errorf("Неверный WebhookURL. Ожидался: %s, получен: %s",
			company.Bitrix24WebhookURL, credentials.WebhookURL)
	}

	// Тест кэширования
	credentials2, err := service.GetTenantCredentials(ctx, company.ID)
	if err != nil {
		t.Fatalf("Ошибка получения кэшированных учетных данных: %v", err)
	}

	if credentials2.WebhookURL != credentials.WebhookURL {
		t.Error("Кэшированные учетные данные не совпадают с оригинальными")
	}

	// Тест несуществующей компании
	_, err = service.GetTenantCredentials(ctx, 999)
	if err == nil {
		t.Error("Ожидалась ошибка для несуществующей компании")
	}
}

func TestBitrix24IntegrationService_SyncObjectToBitrix24(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	// Создаем тестовый объект
	object := &models.Object{
		ID:          1,
		Name:        "Test Object",
		Type:        "GPS Tracker",
		Description: "Test GPS tracker object",
		IMEI:        "123456789012345",
	}

	// Тест успешной синхронизации нового объекта
	err := service.SyncObjectToBitrix24(ctx, company.ID, object)
	if err != nil {
		t.Fatalf("Ошибка синхронизации объекта: %v", err)
	}

	// Проверяем, что сделка была создана
	if mockClient.GetDealsCount() != 1 {
		t.Errorf("Ожидалось 1 сделка, получено: %d", mockClient.GetDealsCount())
	}

	// Проверяем, что маппинг был создан
	var mapping Bitrix24SyncMapping
	err = db.Where("tenant_id = ? AND local_type = ? AND local_id = ?",
		company.ID, "object", object.ID).First(&mapping).Error
	if err != nil {
		t.Fatalf("Маппинг не был создан: %v", err)
	}

	if mapping.Bitrix24Type != "deal" {
		t.Errorf("Неверный тип Битрикс24. Ожидался: deal, получен: %s", mapping.Bitrix24Type)
	}

	// Тест обновления существующего объекта
	object.Name = "Updated Test Object"
	err = service.SyncObjectToBitrix24(ctx, company.ID, object)
	if err != nil {
		t.Fatalf("Ошибка обновления объекта: %v", err)
	}

	// Проверяем, что количество сделок не увеличилось
	if mockClient.GetDealsCount() != 1 {
		t.Errorf("Ожидалось 1 сделка после обновления, получено: %d", mockClient.GetDealsCount())
	}
}

func TestBitrix24IntegrationService_SyncUserToBitrix24(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	// Создаем тестового пользователя
	user := &models.User{
		ID:         1,
		Username:   "ivan.petrov",
		FirstName:  "Иван",
		LastName:   "Петров",
		Email:      "ivan@example.com",
		TelegramID: "+7 (123) 456-78-90",
	}

	// Тест успешной синхронизации нового пользователя
	err := service.SyncUserToBitrix24(ctx, company.ID, user)
	if err != nil {
		t.Fatalf("Ошибка синхронизации пользователя: %v", err)
	}

	// Проверяем, что контакт был создан
	if mockClient.GetContactsCount() != 1 {
		t.Errorf("Ожидался 1 контакт, получено: %d", mockClient.GetContactsCount())
	}

	// Проверяем, что маппинг был создан
	var mapping Bitrix24SyncMapping
	err = db.Where("tenant_id = ? AND local_type = ? AND local_id = ?",
		company.ID, "user", user.ID).First(&mapping).Error
	if err != nil {
		t.Fatalf("Маппинг не был создан: %v", err)
	}

	if mapping.Bitrix24Type != "contact" {
		t.Errorf("Неверный тип Битрикс24. Ожидался: contact, получен: %s", mapping.Bitrix24Type)
	}
}

func TestBitrix24IntegrationService_ErrorHandling(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент с ошибками
	mockClient := NewMockBitrix24Client()
	mockClient.SetShouldFail(true, "Test error")
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	// Создаем тестовый объект
	object := &models.Object{
		ID:          1,
		Name:        "Test Object",
		Type:        "GPS Tracker",
		Description: "Test GPS tracker object",
		IMEI:        "123456789012345",
	}

	// Тест обработки ошибки синхронизации
	err := service.SyncObjectToBitrix24(ctx, company.ID, object)
	if err == nil {
		t.Error("Ожидалась ошибка синхронизации")
	}

	// Проверяем, что ошибка была сохранена в БД
	var integrationError models.IntegrationError
	err = db.Where("tenant_id = ? AND service = ? AND object_id = ?",
		company.ID, models.IntegrationServiceBitrix24, object.ID).First(&integrationError).Error
	if err != nil {
		t.Fatalf("Ошибка интеграции не была сохранена: %v", err)
	}

	if integrationError.Operation != "create_deal" {
		t.Errorf("Неверная операция в ошибке. Ожидалась: create_deal, получена: %s",
			integrationError.Operation)
	}

	if !integrationError.Retryable {
		t.Error("Ошибка должна быть помечена как повторяемая")
	}
}

func TestBitrix24IntegrationService_SyncFromBitrix24(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	// Добавляем тестовые данные в мок
	testContact := &Bitrix24Contact{
		Name:     "Тест",
		LastName: "Контакт",
		Email:    "test@example.com",
		Phone:    "+7 (123) 456-78-90",
		Comments: "Test contact",
	}
	contactID := mockClient.AddTestContact(testContact)

	testDeal := &Bitrix24Deal{
		Title:       "Тест сделка",
		Comments:    "Test deal",
		Opportunity: 100000,
		CurrencyID:  "RUB",
	}
	dealID := mockClient.AddTestDeal(testDeal)

	// Создаем маппинги для тестирования обновлений
	contactMapping := &Bitrix24SyncMapping{
		TenantID:      company.ID,
		LocalType:     "user",
		LocalID:       1,
		Bitrix24Type:  "contact",
		Bitrix24ID:    contactID,
		LastSyncAt:    time.Now().Add(-1 * time.Hour),
		SyncDirection: "bidirectional",
	}
	db.Create(contactMapping)

	dealMapping := &Bitrix24SyncMapping{
		TenantID:      company.ID,
		LocalType:     "object",
		LocalID:       1,
		Bitrix24Type:  "deal",
		Bitrix24ID:    dealID,
		LastSyncAt:    time.Now().Add(-1 * time.Hour),
		SyncDirection: "bidirectional",
	}
	db.Create(dealMapping)

	ctx := context.Background()

	// Тест синхронизации из Битрикс24
	err := service.SyncFromBitrix24(ctx, company.ID)
	if err != nil {
		t.Fatalf("Ошибка синхронизации из Битрикс24: %v", err)
	}

	// Проверяем, что время последней синхронизации обновилось
	var updatedContactMapping Bitrix24SyncMapping
	err = db.First(&updatedContactMapping, contactMapping.ID).Error
	if err != nil {
		t.Fatalf("Ошибка получения обновленного маппинга контакта: %v", err)
	}

	if !updatedContactMapping.LastSyncAt.After(contactMapping.LastSyncAt) {
		t.Error("Время последней синхронизации контакта не обновилось")
	}
}

func TestBitrix24IntegrationService_CheckHealth(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	// Тест успешной проверки здоровья
	err := service.CheckHealth(ctx, company.ID)
	if err != nil {
		t.Fatalf("Ошибка проверки здоровья: %v", err)
	}

	// Тест проверки здоровья с ошибкой
	mockClient.SetHealthStatus(false)
	err = service.CheckHealth(ctx, company.ID)
	if err == nil {
		t.Error("Ожидалась ошибка проверки здоровья")
	}
}

func TestBitrix24IntegrationService_SetupCompanyCredentials(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	// Тест настройки учетных данных
	newWebhookURL := "https://newtest.bitrix24.com/rest/1/new_webhook_key/"
	err := service.SetupCompanyCredentials(ctx, company.ID, newWebhookURL)
	if err != nil {
		t.Fatalf("Ошибка настройки учетных данных: %v", err)
	}

	// Проверяем, что данные обновились в БД
	var updatedCompany models.Company
	err = db.First(&updatedCompany, company.ID).Error
	if err != nil {
		t.Fatalf("Ошибка получения обновленной компании: %v", err)
	}

	if updatedCompany.Bitrix24WebhookURL != newWebhookURL {
		t.Errorf("WebhookURL не обновился. Ожидался: %s, получен: %s",
			newWebhookURL, updatedCompany.Bitrix24WebhookURL)
	}

	// Проверяем, что кэш очистился
	if service.GetCachedCredentialsCount() != 0 {
		t.Error("Кэш не был очищен после обновления учетных данных")
	}
}

func TestBitrix24IntegrationService_GetSyncMappings(t *testing.T) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	service := NewBitrix24IntegrationService(logger)

	// Создаем тестовые маппинги
	for i := 1; i <= 5; i++ {
		mapping := &Bitrix24SyncMapping{
			TenantID:      company.ID,
			LocalType:     "object",
			LocalID:       uint(i),
			Bitrix24Type:  "deal",
			Bitrix24ID:    fmt.Sprintf("deal_%d", i),
			LastSyncAt:    time.Now().Add(-time.Duration(i) * time.Hour),
			SyncDirection: "to_bitrix",
		}
		db.Create(mapping)
	}

	// Тест получения маппингов с пагинацией
	mappings, total, err := service.GetSyncMappings(company.ID, 3, 0)
	if err != nil {
		t.Fatalf("Ошибка получения маппингов: %v", err)
	}

	if total != 5 {
		t.Errorf("Неверное общее количество. Ожидалось: 5, получено: %d", total)
	}

	if len(mappings) != 3 {
		t.Errorf("Неверное количество маппингов на странице. Ожидалось: 3, получено: %d", len(mappings))
	}

	// Проверяем сортировку по updated_at DESC
	if len(mappings) >= 2 {
		if mappings[0].UpdatedAt.Before(mappings[1].UpdatedAt) {
			t.Error("Маппинги не отсортированы по убыванию времени обновления")
		}
	}
}

// GetCachedCredentialsCount возвращает количество кэшированных учетных данных (для тестирования)
func (s *Bitrix24IntegrationService) GetCachedCredentialsCount() int {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return len(s.credentialsCache)
}

// Benchmark тесты
func BenchmarkBitrix24IntegrationService_SyncObjectToBitrix24(b *testing.B) {
	db := setupTestDBForBitrix24()
	company := createBitrixTestCompany(db)

	// Подменяем глобальную БД для тестирования
	originalDB := database.DB
	database.DB = db
	defer func() { database.DB = originalDB }()

	logger := log.New(io.Discard, "", 0) // Отключаем логирование для бенчмарка
	service := NewBitrix24IntegrationService(logger)

	// Используем мок клиент
	mockClient := NewMockBitrix24Client()
	service.Bitrix24Client = mockClient

	ctx := context.Background()

	object := &models.Object{
		ID:          1,
		Name:        "Benchmark Object",
		Type:        "GPS Tracker",
		Description: "Benchmark GPS tracker object",
		IMEI:        "123456789012345",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		object.ID = uint(i + 1)
		mockClient.ClearData() // Очищаем данные между итерациями

		err := service.SyncObjectToBitrix24(ctx, company.ID, object)
		if err != nil {
			b.Fatalf("Ошибка синхронизации: %v", err)
		}
	}
}
