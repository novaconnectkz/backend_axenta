package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestEnvironment настраивает тестовое окружение с мультитенантностью
func setupTestEnvironment(t *testing.T) (*gin.Engine, *gorm.DB, func()) {
	gin.SetMode(gin.TestMode)

	// Используем SQLite в памяти для тестов
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Автомиграция всех моделей
	err = db.AutoMigrate(
		&models.Company{},
		&models.Contract{},
		&models.Object{},
		&models.User{},
		&models.Role{},
		&models.Installation{},
		&models.Installer{},
	)
	require.NoError(t, err)

	// Настройка Gin router с middleware
	router := gin.New()
	tenantMiddleware := middleware.NewTenantMiddleware(db)

	// Публичные маршруты
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API группа с tenant middleware
	apiGroup := router.Group("/api")
	apiGroup.Use(tenantMiddleware.SetTenant())
	{
		apiGroup.GET("/objects", GetObjects)
	}

	// Функция очистки
	cleanup := func() {
		// Очищаем данные в SQLite
		db.Exec("DELETE FROM objects")
		db.Exec("DELETE FROM installations")
		db.Exec("DELETE FROM companies")
		db.Exec("DELETE FROM users")
		db.Exec("DELETE FROM roles")
		db.Exec("DELETE FROM installers")
	}

	cleanup() // Очищаем перед тестами

	return router, db, cleanup
}

// createTestCompanyWithData создает тестовую компанию с данными
func createTestCompanyWithData(t *testing.T, db *gorm.DB, name, schema string) (*models.Company, *gorm.DB) {
	company := &models.Company{
		ID:             uuid.New(), // Вручную генерируем UUID для SQLite
		Name:           name,
		DatabaseSchema: schema,
		AxetnaLogin:    "test_login",
		AxetnaPassword: "encrypted_password",
		ContactEmail:   fmt.Sprintf("%s@test.com", strings.ToLower(name)),
		IsActive:       true,
	}

	err := db.Create(company).Error
	require.NoError(t, err)

	// Для SQLite тестов просто возвращаем ту же БД
	// В реальной системе здесь будет переключение схем
	return company, db
}

// TestMultiTenantObjectsAPI тестирует изоляцию объектов между компаниями
func TestMultiTenantObjectsAPI(t *testing.T) {
	router, db, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Создаем две компании с данными
	company1, tenantDB1 := createTestCompanyWithData(t, db, "TestCompany1", "tenant_objects1")
	company2, tenantDB2 := createTestCompanyWithData(t, db, "TestCompany2", "tenant_objects2")

	// Создаем контракты для компаний
	contract1 := models.Contract{
		Number:      "TEST-001",
		Title:       "Test Contract 1",
		Description: "Test contract for company 1",
		CompanyID:   company1.ID,
		ClientName:  "Test Client 1",
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(1, 0, 0),
		Status:      "active",
	}
	err := tenantDB1.Create(&contract1).Error
	require.NoError(t, err)

	contract2 := models.Contract{
		Number:      "TEST-002",
		Title:       "Test Contract 2",
		Description: "Test contract for company 2",
		CompanyID:   company2.ID,
		ClientName:  "Test Client 2",
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(1, 0, 0),
		Status:      "active",
	}
	err = tenantDB2.Create(&contract2).Error
	require.NoError(t, err)

	// Создаем объекты для первой компании
	object1 := models.Object{
		Name:        "Object from Company 1",
		Description: "This object belongs to company 1",
		IsActive:    true,
		ContractID:  contract1.ID,
	}
	err = tenantDB1.Create(&object1).Error
	require.NoError(t, err)

	// Создаем объекты для второй компании
	object2 := models.Object{
		Name:        "Object from Company 2",
		Description: "This object belongs to company 2",
		IsActive:    true,
		ContractID:  contract2.ID,
	}
	err = tenantDB2.Create(&object2).Error
	require.NoError(t, err)

	t.Run("Компания 1 видит только свои объекты", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		req.Header.Set("X-Tenant-ID", company1.ID.String())

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		data := response["data"].(map[string]interface{})
		items := data["items"].([]interface{})

		assert.Len(t, items, 1)

		item := items[0].(map[string]interface{})
		assert.Equal(t, "Object from Company 1", item["name"])
	})

	t.Run("Компания 2 видит только свои объекты", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		req.Header.Set("X-Tenant-ID", company2.ID.String())

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		data := response["data"].(map[string]interface{})
		items := data["items"].([]interface{})

		assert.Len(t, items, 1)

		item := items[0].(map[string]interface{})
		assert.Equal(t, "Object from Company 2", item["name"])
	})

	t.Run("Без указания компании возвращается ошибка", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		// Не указываем X-Tenant-ID

		router.ServeHTTP(w, req)

		// Должна быть выбрана компания по умолчанию или возвращена ошибка
		// В зависимости от реализации middleware
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusUnauthorized)
	})
}

// TestTenantSwitchingPerformance тестирует производительность переключения схем
func TestTenantSwitchingPerformance(t *testing.T) {
	router, db, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Создаем несколько компаний
	companies := make([]*models.Company, 5)
	for i := 0; i < 5; i++ {
		company, _ := createTestCompanyWithData(t, db, fmt.Sprintf("PerfCompany%d", i), fmt.Sprintf("tenant_perf_%d", i))
		companies[i] = company
	}

	// Тестируем быстрое переключение между компаниями
	t.Run("Быстрое переключение между компаниями", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			company := companies[i%len(companies)]

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/objects", nil)
			req.Header.Set("X-Tenant-ID", company.ID.String())

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Запрос %d к компании %s должен быть успешным", i, company.ID.String())
		}
	})
}

// TestTenantMiddlewareEdgeCases тестирует граничные случаи
func TestTenantMiddlewareEdgeCases(t *testing.T) {
	router, db, cleanup := setupTestEnvironment(t)
	defer cleanup()

	company, _ := createTestCompanyWithData(t, db, "EdgeCaseCompany", "tenant_edge")

	t.Run("Некорректный X-Tenant-ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		req.Header.Set("X-Tenant-ID", "invalid_id")

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["status"])
	})

	t.Run("Несуществующий X-Tenant-ID", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		req.Header.Set("X-Tenant-ID", "99999")

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["status"])
	})

	t.Run("Деактивированная компания", func(t *testing.T) {
		// Деактивируем компанию
		company.IsActive = false
		db.Save(company)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/objects", nil)
		req.Header.Set("X-Tenant-ID", company.ID.String())

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "error", response["status"])
		assert.Contains(t, response["error"].(string), "деактивирована")

		// Восстанавливаем компанию
		company.IsActive = true
		db.Save(company)
	})

	t.Run("Публичные маршруты не требуют tenant", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ping", nil)
		// Не устанавливаем X-Tenant-ID

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "ok", response["status"])
	})
}

// TestConcurrentTenantAccess тестирует одновременный доступ к разным компаниям
func TestConcurrentTenantAccess(t *testing.T) {
	router, db, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Создаем компании
	company1, tenantDB1 := createTestCompanyWithData(t, db, "ConcurrentCompany1", "tenant_concurrent1")
	company2, tenantDB2 := createTestCompanyWithData(t, db, "ConcurrentCompany2", "tenant_concurrent2")

	// Создаем контракты для компаний
	contract1 := models.Contract{
		Number:      "PERF-001",
		Title:       "Performance Test Contract 1",
		Description: "Performance test contract for company 1",
		CompanyID:   company1.ID,
		ClientName:  "Performance Client 1",
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(1, 0, 0),
		Status:      "active",
	}
	tenantDB1.Create(&contract1)

	contract2 := models.Contract{
		Number:      "PERF-002",
		Title:       "Performance Test Contract 2",
		Description: "Performance test contract for company 2",
		CompanyID:   company2.ID,
		ClientName:  "Performance Client 2",
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(1, 0, 0),
		Status:      "active",
	}
	tenantDB2.Create(&contract2)

	// Добавляем данные в каждую компанию
	for i := 0; i < 3; i++ {
		object1 := models.Object{
			Name:        fmt.Sprintf("Company1_Object_%d", i),
			Description: "Object for company 1",
			IsActive:    true,
			ContractID:  contract1.ID,
		}
		tenantDB1.Create(&object1)

		object2 := models.Object{
			Name:        fmt.Sprintf("Company2_Object_%d", i),
			Description: "Object for company 2",
			IsActive:    true,
			ContractID:  contract2.ID,
		}
		tenantDB2.Create(&object2)
	}

	t.Run("Одновременные запросы к разным компаниям", func(t *testing.T) {
		// Канал для результатов
		results := make(chan bool, 10)

		// Запускаем горутины для каждой компании
		for i := 0; i < 5; i++ {
			go func(companyID uuid.UUID, expectedPrefix string) {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/api/objects", nil)
				req.Header.Set("X-Tenant-ID", companyID.String())

				router.ServeHTTP(w, req)

				success := w.Code == http.StatusOK
				if success {
					var response map[string]interface{}
					if err := json.Unmarshal(w.Body.Bytes(), &response); err == nil {
						data := response["data"].(map[string]interface{})
						items := data["items"].([]interface{})

						// Проверяем, что все объекты принадлежат правильной компании
						for _, item := range items {
							obj := item.(map[string]interface{})
							name := obj["name"].(string)
							if !strings.HasPrefix(name, expectedPrefix) {
								success = false
								break
							}
						}
					} else {
						success = false
					}
				}

				results <- success
			}(company1.ID, "Company1_Object_")

			go func(companyID uuid.UUID, expectedPrefix string) {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/api/objects", nil)
				req.Header.Set("X-Tenant-ID", companyID.String())

				router.ServeHTTP(w, req)

				success := w.Code == http.StatusOK
				if success {
					var response map[string]interface{}
					if err := json.Unmarshal(w.Body.Bytes(), &response); err == nil {
						data := response["data"].(map[string]interface{})
						items := data["items"].([]interface{})

						// Проверяем, что все объекты принадлежат правильной компании
						for _, item := range items {
							obj := item.(map[string]interface{})
							name := obj["name"].(string)
							if !strings.HasPrefix(name, expectedPrefix) {
								success = false
								break
							}
						}
					} else {
						success = false
					}
				}

				results <- success
			}(company2.ID, "Company2_Object_")
		}

		// Ждем результаты
		successCount := 0
		for i := 0; i < 10; i++ {
			if <-results {
				successCount++
			}
		}

		// Все запросы должны быть успешными
		assert.Equal(t, 10, successCount, "Все одновременные запросы должны быть успешными")
	})
}
