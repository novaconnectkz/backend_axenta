package middleware

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB настраивает тестовую базу данных
func setupTestDB(t *testing.T) *gorm.DB {
	// Используем тестовую БД
	os.Setenv("DB_NAME", "axenta_test_db")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "postgres")
	os.Setenv("DB_PASSWORD", "")
	os.Setenv("DB_SSLMODE", "disable")

	// Создаем тестовую БД если не существует
	err := database.CreateDatabaseIfNotExists()
	require.NoError(t, err)

	// Подключаемся к тестовой БД
	err = database.ConnectDatabase()
	require.NoError(t, err)

	db := database.GetDB()
	require.NotNil(t, db)

	// Очищаем БД перед тестами
	cleanupTestDB(t, db)

	return db
}

// cleanupTestDB очищает тестовую базу данных
func cleanupTestDB(t *testing.T, db *gorm.DB) {
	// Удаляем все схемы компаний
	var companies []models.Company
	db.Find(&companies)

	for _, company := range companies {
		schemaName := company.GetSchemaName()
		db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	}

	// Очищаем глобальные таблицы
	db.Exec("DELETE FROM companies")
	db.Exec("DELETE FROM billing_plans WHERE company_id IS NOT NULL")
}

// createTestCompany создает тестовую компанию
func createTestCompany(t *testing.T, db *gorm.DB, name, schema string) *models.Company {
	company := &models.Company{
		Name:                 name,
		DatabaseSchema:       schema,
		AxetnaLogin:          "test_login",
		AxetnaPassword:       "encrypted_password",
		Bitrix24WebhookURL:   "",
		Bitrix24ClientID:     "",
		Bitrix24ClientSecret: "",
		ContactEmail:         fmt.Sprintf("%s@test.com", name),
		ContactPhone:         "",
		ContactPerson:        "",
		Address:              "",
		City:                 "",
		Country:              "Russia",
		IsActive:             true,
		MaxUsers:             10,
		MaxObjects:           100,
		StorageQuota:         1024,
		Language:             "ru",
		Timezone:             "Europe/Moscow",
		Currency:             "RUB",
	}

	err := db.Create(company).Error
	require.NoError(t, err)

	return company
}

// TestTenantMiddleware_SetTenant тестирует основную функциональность middleware
func TestTenantMiddleware_SetTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Создаем тестовые компании
	company1 := createTestCompany(t, db, "TestCompany1", "tenant_test1")
	company2 := createTestCompany(t, db, "TestCompany2", "tenant_test2")

	middleware := NewTenantMiddleware(db)

	t.Run("Переключение по заголовку X-Tenant-ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)
		c.Request.Header.Set("X-Tenant-ID", fmt.Sprintf("%d", company1.ID))

		// Создаем схему для компании
		err := middleware.createTenantSchema(company1.GetSchemaName())
		require.NoError(t, err)

		middleware.SetTenant()(c)

		// Проверяем, что компания установлена в контекст
		contextCompany := GetCurrentCompany(c)
		require.NotNil(t, contextCompany)
		assert.Equal(t, company1.ID, contextCompany.ID)
		assert.Equal(t, company1.Name, contextCompany.Name)

		// Проверяем, что БД переключена на правильную схему
		tenantDB := GetTenantDB(c)
		require.NotNil(t, tenantDB)
	})

	t.Run("Переключение по домену", func(t *testing.T) {
		// Обновляем компанию с доменом
		company1.Domain = "test1.example.com"
		db.Save(company1)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)
		c.Request.Header.Set("Host", "test1.example.com")

		middleware.SetTenant()(c)

		contextCompany := GetCurrentCompany(c)
		require.NotNil(t, contextCompany)
		assert.Equal(t, company1.ID, contextCompany.ID)
	})

	t.Run("Компания по умолчанию", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)

		middleware.SetTenant()(c)

		// Должна быть выбрана одна из существующих компаний
		contextCompany := GetCurrentCompany(c)
		require.NotNil(t, contextCompany)
		assert.True(t, contextCompany.ID == company1.ID || contextCompany.ID == company2.ID)
	})

	t.Run("Публичные маршруты пропускаются", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/ping", nil)

		middleware.SetTenant()(c)

		// Для публичных маршрутов компания не должна быть установлена
		contextCompany := GetCurrentCompany(c)
		assert.Nil(t, contextCompany)
	})
}

// TestDataIsolation тестирует изоляцию данных между компаниями
func TestDataIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Создаем две тестовые компании
	company1 := createTestCompany(t, db, "IsolationTest1", "tenant_isolation1")
	company2 := createTestCompany(t, db, "IsolationTest2", "tenant_isolation2")

	middleware := NewTenantMiddleware(db)

	// Создаем схемы для обеих компаний
	err := middleware.createTenantSchema(company1.GetSchemaName())
	require.NoError(t, err)

	err = middleware.createTenantSchema(company2.GetSchemaName())
	require.NoError(t, err)

	t.Run("Изоляция пользователей", func(t *testing.T) {
		// Создаем пользователя в схеме первой компании
		tenantDB1 := middleware.switchToTenantSchema(company1.GetSchemaName())
		require.NotNil(t, tenantDB1)

		user1 := models.User{
			Username:  "user1@company1.com",
			Email:     "user1@company1.com",
			Password:  "password1",
			FirstName: "User",
			LastName:  "One",
			IsActive:  true,
		}
		err := tenantDB1.Create(&user1).Error
		require.NoError(t, err)

		// Создаем пользователя в схеме второй компании
		tenantDB2 := middleware.switchToTenantSchema(company2.GetSchemaName())
		require.NotNil(t, tenantDB2)

		user2 := models.User{
			Username:  "user2@company2.com",
			Email:     "user2@company2.com",
			Password:  "password2",
			FirstName: "User",
			LastName:  "Two",
			IsActive:  true,
		}
		err = tenantDB2.Create(&user2).Error
		require.NoError(t, err)

		// Проверяем изоляцию: в схеме company1 должен быть только user1
		var users1 []models.User
		err = tenantDB1.Find(&users1).Error
		require.NoError(t, err)
		assert.Len(t, users1, 1)
		assert.Equal(t, "user1@company1.com", users1[0].Username)

		// В схеме company2 должен быть только user2
		var users2 []models.User
		err = tenantDB2.Find(&users2).Error
		require.NoError(t, err)
		assert.Len(t, users2, 1)
		assert.Equal(t, "user2@company2.com", users2[0].Username)
	})

	t.Run("Изоляция объектов мониторинга", func(t *testing.T) {
		tenantDB1 := middleware.switchToTenantSchema(company1.GetSchemaName())
		tenantDB2 := middleware.switchToTenantSchema(company2.GetSchemaName())

		// Создаем объекты в разных схемах
		object1 := models.Object{
			Name:        "Object Company 1",
			Description: "Test object for company 1",
			IsActive:    true,
		}
		err := tenantDB1.Create(&object1).Error
		require.NoError(t, err)

		object2 := models.Object{
			Name:        "Object Company 2",
			Description: "Test object for company 2",
			IsActive:    true,
		}
		err = tenantDB2.Create(&object2).Error
		require.NoError(t, err)

		// Проверяем изоляцию объектов
		var objects1 []models.Object
		err = tenantDB1.Find(&objects1).Error
		require.NoError(t, err)
		assert.Len(t, objects1, 1)
		assert.Equal(t, "Object Company 1", objects1[0].Name)

		var objects2 []models.Object
		err = tenantDB2.Find(&objects2).Error
		require.NoError(t, err)
		assert.Len(t, objects2, 1)
		assert.Equal(t, "Object Company 2", objects2[0].Name)
	})
}

// TestTenantSchemaManagement тестирует управление схемами БД
func TestTenantSchemaManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	middleware := NewTenantMiddleware(db)

	t.Run("Создание схемы компании", func(t *testing.T) {
		schemaName := "tenant_schema_test"

		err := middleware.createTenantSchema(schemaName)
		assert.NoError(t, err)

		// Проверяем, что схема создана
		var exists bool
		query := "SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = ?)"
		err = db.Raw(query, schemaName).Scan(&exists).Error
		require.NoError(t, err)
		assert.True(t, exists, "Схема должна быть создана")

		// Проверяем, что в схеме созданы таблицы
		tenantDB := middleware.switchToTenantSchema(schemaName)
		require.NotNil(t, tenantDB)

		// Проверяем наличие основных таблиц
		assert.True(t, tenantDB.Migrator().HasTable(&models.User{}))
		assert.True(t, tenantDB.Migrator().HasTable(&models.Object{}))
		assert.True(t, tenantDB.Migrator().HasTable(&models.Role{}))
		assert.True(t, tenantDB.Migrator().HasTable(&models.Contract{}))
	})

	t.Run("Переключение между схемами", func(t *testing.T) {
		schema1 := "tenant_switch_test1"
		schema2 := "tenant_switch_test2"

		// Создаем две схемы
		err := middleware.createTenantSchema(schema1)
		require.NoError(t, err)

		err = middleware.createTenantSchema(schema2)
		require.NoError(t, err)

		// Переключаемся на первую схему и создаем данные
		tenantDB1 := middleware.switchToTenantSchema(schema1)
		require.NotNil(t, tenantDB1)

		user1 := models.User{
			Username: "schema1_user",
			Email:    "schema1@test.com",
			Password: "password",
		}
		err = tenantDB1.Create(&user1).Error
		require.NoError(t, err)

		// Переключаемся на вторую схему и создаем данные
		tenantDB2 := middleware.switchToTenantSchema(schema2)
		require.NotNil(t, tenantDB2)

		user2 := models.User{
			Username: "schema2_user",
			Email:    "schema2@test.com",
			Password: "password",
		}
		err = tenantDB2.Create(&user2).Error
		require.NoError(t, err)

		// Проверяем, что данные изолированы
		var count1, count2 int64

		tenantDB1.Model(&models.User{}).Count(&count1)
		tenantDB2.Model(&models.User{}).Count(&count2)

		assert.Equal(t, int64(1), count1)
		assert.Equal(t, int64(1), count2)

		// Проверяем содержимое
		var testUser1, testUser2 models.User
		tenantDB1.First(&testUser1)
		tenantDB2.First(&testUser2)

		assert.Equal(t, "schema1_user", testUser1.Username)
		assert.Equal(t, "schema2_user", testUser2.Username)
	})
}

// TestCompanyExtraction тестирует извлечение информации о компании
func TestCompanyExtraction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	company := createTestCompany(t, db, "ExtractionTest", "tenant_extraction")
	middleware := NewTenantMiddleware(db)

	t.Run("Извлечение по X-Tenant-ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)
		c.Request.Header.Set("X-Tenant-ID", fmt.Sprintf("%d", company.ID))

		extractedCompany, err := middleware.extractCompany(c)
		require.NoError(t, err)
		require.NotNil(t, extractedCompany)
		assert.Equal(t, company.ID, extractedCompany.ID)
	})

	t.Run("Извлечение по домену", func(t *testing.T) {
		company.Domain = "extraction.test.com"
		db.Save(company)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)
		c.Request.Header.Set("Host", "extraction.test.com")

		extractedCompany, err := middleware.extractCompany(c)
		require.NoError(t, err)
		require.NotNil(t, extractedCompany)
		assert.Equal(t, company.ID, extractedCompany.ID)
	})

	t.Run("Обработка неактивной компании", func(t *testing.T) {
		// Деактивируем компанию
		company.IsActive = false
		db.Save(company)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/api/test", nil)
		c.Request.Header.Set("X-Tenant-ID", fmt.Sprintf("%d", company.ID))

		middleware.SetTenant()(c)

		// Запрос должен быть отклонен
		assert.Equal(t, http.StatusForbidden, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["status"])
		assert.Contains(t, response["error"].(string), "деактивирована")
	})
}

// BenchmarkTenantSwitching бенчмарк переключения между схемами
func BenchmarkTenantSwitching(b *testing.B) {
	gin.SetMode(gin.TestMode)

	// Настройка тестовой БД
	os.Setenv("DB_NAME", "axenta_bench_db")
	database.CreateDatabaseIfNotExists()
	database.ConnectDatabase()

	db := database.GetDB()
	middleware := NewTenantMiddleware(db)

	// Создаем тестовые компании
	companies := make([]*models.Company, 10)
	for i := 0; i < 10; i++ {
		company := &models.Company{
			Name:           fmt.Sprintf("BenchCompany%d", i),
			DatabaseSchema: fmt.Sprintf("tenant_bench_%d", i),
			AxetnaLogin:    "bench_login",
			AxetnaPassword: "encrypted_password",
			ContactEmail:   fmt.Sprintf("bench%d@test.com", i),
			IsActive:       true,
		}
		db.Create(company)
		middleware.createTenantSchema(company.GetSchemaName())
		companies[i] = company
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			company := companies[i%len(companies)]

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/api/test", nil)
			c.Request.Header.Set("X-Tenant-ID", fmt.Sprintf("%d", company.ID))

			middleware.SetTenant()(c)

			i++
		}
	})

	// Очистка
	for _, company := range companies {
		db.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", company.GetSchemaName()))
	}
	db.Exec("DELETE FROM companies")
}
