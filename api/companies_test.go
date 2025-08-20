package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type CompaniesTestSuite struct {
	suite.Suite
	db               *gorm.DB
	api              *CompaniesAPI
	router           *gin.Engine
	tenantMiddleware *middleware.TenantMiddleware
}

func (suite *CompaniesTestSuite) SetupSuite() {
	// Настраиваем тестовую БД
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=axenta_test port=5432 sslmode=disable"
	}

	var err error
	suite.db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	suite.Require().NoError(err)

	// Инициализируем глобальную БД для кэша
	database.DB = suite.db

	// Выполняем миграции
	err = suite.db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Object{},
	)
	suite.Require().NoError(err)

	// Создаем middleware
	suite.tenantMiddleware = middleware.NewTenantMiddleware(suite.db)

	// Создаем API
	suite.api = NewCompaniesAPI(suite.db, suite.tenantMiddleware)

	// Настраиваем роутер
	gin.SetMode(gin.TestMode)
	suite.router = gin.New()

	// Регистрируем маршруты
	api := suite.router.Group("/api")
	suite.api.RegisterCompaniesRoutes(api)
}

func (suite *CompaniesTestSuite) SetupTest() {
	// Очищаем данные перед каждым тестом
	suite.db.Exec("DELETE FROM companies")
}

func (suite *CompaniesTestSuite) TearDownSuite() {
	// Очищаем тестовые данные
	suite.db.Exec("DROP SCHEMA IF EXISTS tenant_test CASCADE")
	suite.db.Exec("DELETE FROM companies")
}

// TestCreateCompany тестирует создание компании
func (suite *CompaniesTestSuite) TestCreateCompany() {
	reqData := CompanyRequest{
		Name:           "Тестовая компания",
		Domain:         "test.example.com",
		AxetnaLogin:    "test_user",
		AxetnaPassword: "test_password",
		ContactEmail:   "test@example.com",
		ContactPhone:   "+7 (999) 123-45-67",
		ContactPerson:  "Иван Иванов",
		Address:        "ул. Тестовая, 1",
		City:           "Москва",
		Country:        "Russia",
		MaxUsers:       50,
		MaxObjects:     500,
		StorageQuota:   2048,
		Language:       "ru",
		Timezone:       "Europe/Moscow",
		Currency:       "RUB",
	}

	jsonData, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])

	data := response["data"].(map[string]interface{})
	assert.Equal(suite.T(), reqData.Name, data["name"])
	assert.Equal(suite.T(), reqData.Domain, data["domain"])
	assert.Equal(suite.T(), reqData.ContactEmail, data["contact_email"])
	assert.Equal(suite.T(), true, data["is_active"])
}

// TestCreateCompanyValidation тестирует валидацию при создании компании
func (suite *CompaniesTestSuite) TestCreateCompanyValidation() {
	// Тест без обязательных полей
	reqData := CompanyRequest{
		Name: "", // Пустое имя
	}

	jsonData, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "error", response["status"])
	assert.Contains(suite.T(), response["error"], "Некорректные данные")
}

// TestCreateCompanyDuplicateDomain тестирует создание компании с дублирующимся доменом
func (suite *CompaniesTestSuite) TestCreateCompanyDuplicateDomain() {
	// Создаем первую компанию
	company1 := &models.Company{
		Name:           "Компания 1",
		Domain:         "duplicate.example.com",
		AxetnaLogin:    "user1",
		AxetnaPassword: "password1",
		IsActive:       true,
	}
	suite.db.Create(company1)

	// Пытаемся создать вторую компанию с тем же доменом
	reqData := CompanyRequest{
		Name:           "Компания 2",
		Domain:         "duplicate.example.com",
		AxetnaLogin:    "user2",
		AxetnaPassword: "password2",
	}

	jsonData, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("POST", "/api/accounts", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "error", response["status"])
	assert.Contains(suite.T(), response["error"], "домен уже существует")
}

// TestGetCompanies тестирует получение списка компаний
func (suite *CompaniesTestSuite) TestGetCompanies() {
	// Создаем тестовые компании
	companies := []models.Company{
		{
			Name:           "Компания A",
			Domain:         "a.example.com",
			AxetnaLogin:    "user_a",
			AxetnaPassword: "password_a",
			IsActive:       true,
			City:           "Москва",
		},
		{
			Name:           "Компания B",
			Domain:         "b.example.com",
			AxetnaLogin:    "user_b",
			AxetnaPassword: "password_b",
			IsActive:       false,
			City:           "Санкт-Петербург",
		},
	}

	for _, company := range companies {
		suite.db.Create(&company)
	}

	// Тестируем получение всех компаний
	req, _ := http.NewRequest("GET", "/api/accounts", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])

	data := response["data"].(map[string]interface{})
	companiesList := data["companies"].([]interface{})
	assert.Len(suite.T(), companiesList, 2)
}

// TestGetCompaniesWithFilters тестирует получение компаний с фильтрацией
func (suite *CompaniesTestSuite) TestGetCompaniesWithFilters() {
	// Создаем тестовые компании
	companies := []models.Company{
		{
			Name:           "Активная компания",
			Domain:         "active.example.com",
			AxetnaLogin:    "active_user",
			AxetnaPassword: "password",
			IsActive:       true,
		},
		{
			Name:           "Неактивная компания",
			Domain:         "inactive.example.com",
			AxetnaLogin:    "inactive_user",
			AxetnaPassword: "password",
			IsActive:       false,
		},
	}

	for _, company := range companies {
		suite.db.Create(&company)
	}

	// Тестируем фильтр по активности
	req, _ := http.NewRequest("GET", "/api/accounts?is_active=true", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	data := response["data"].(map[string]interface{})
	companiesList := data["companies"].([]interface{})
	assert.Len(suite.T(), companiesList, 1)

	company := companiesList[0].(map[string]interface{})
	assert.Equal(suite.T(), "Активная компания", company["name"])
}

// TestGetCompaniesSearch тестирует поиск компаний
func (suite *CompaniesTestSuite) TestGetCompaniesSearch() {
	// Создаем тестовые компании
	companies := []models.Company{
		{
			Name:           "ООО Рога и Копыта",
			AxetnaLogin:    "roga",
			AxetnaPassword: "password",
			ContactEmail:   "info@roga.ru",
			IsActive:       true,
		},
		{
			Name:           "АО Технологии",
			AxetnaLogin:    "tech",
			AxetnaPassword: "password",
			ContactEmail:   "contact@tech.ru",
			IsActive:       true,
		},
	}

	for _, company := range companies {
		suite.db.Create(&company)
	}

	// Тестируем поиск по названию
	req, _ := http.NewRequest("GET", "/api/accounts?search=Рога", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	data := response["data"].(map[string]interface{})
	companiesList := data["companies"].([]interface{})
	assert.Len(suite.T(), companiesList, 1)

	company := companiesList[0].(map[string]interface{})
	assert.Equal(suite.T(), "ООО Рога и Копыта", company["name"])
}

// TestGetCompany тестирует получение компании по ID
func (suite *CompaniesTestSuite) TestGetCompany() {
	// Создаем тестовую компанию
	company := &models.Company{
		Name:           "Тестовая компания",
		Domain:         "test.example.com",
		AxetnaLogin:    "test_user",
		AxetnaPassword: "password",
		ContactEmail:   "test@example.com",
		IsActive:       true,
	}
	suite.db.Create(company)

	// Получаем компанию по ID
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/accounts/%d", company.ID), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])

	data := response["data"].(map[string]interface{})
	assert.Equal(suite.T(), company.Name, data["name"])
	assert.Equal(suite.T(), company.Domain, data["domain"])
	assert.Equal(suite.T(), company.ContactEmail, data["contact_email"])
}

// TestGetCompanyNotFound тестирует получение несуществующей компании
func (suite *CompaniesTestSuite) TestGetCompanyNotFound() {
	req, _ := http.NewRequest("GET", "/api/accounts/999", nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "error", response["status"])
	assert.Contains(suite.T(), response["error"], "не найдена")
}

// TestUpdateCompany тестирует обновление компании
func (suite *CompaniesTestSuite) TestUpdateCompany() {
	// Создаем тестовую компанию
	company := &models.Company{
		Name:           "Старое название",
		Domain:         "old.example.com",
		AxetnaLogin:    "old_user",
		AxetnaPassword: "old_password",
		IsActive:       true,
	}
	suite.db.Create(company)

	// Обновляем компанию
	reqData := CompanyRequest{
		Name:           "Новое название",
		Domain:         "new.example.com",
		AxetnaLogin:    "new_user",
		AxetnaPassword: "new_password",
		ContactEmail:   "new@example.com",
	}

	jsonData, _ := json.Marshal(reqData)
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/accounts/%d", company.ID), bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])

	data := response["data"].(map[string]interface{})
	assert.Equal(suite.T(), reqData.Name, data["name"])
	assert.Equal(suite.T(), reqData.Domain, data["domain"])
	assert.Equal(suite.T(), reqData.ContactEmail, data["contact_email"])
}

// TestActivateDeactivateCompany тестирует активацию/деактивацию компании
func (suite *CompaniesTestSuite) TestActivateDeactivateCompany() {
	// Создаем тестовую компанию
	company := &models.Company{
		Name:           "Тестовая компания",
		AxetnaLogin:    "test_user",
		AxetnaPassword: "password",
		IsActive:       true,
	}
	suite.db.Create(company)

	// Деактивируем компанию
	req, _ := http.NewRequest("PUT", fmt.Sprintf("/api/accounts/%d/deactivate", company.ID), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])
	assert.Contains(suite.T(), response["message"], "деактивирована")

	data := response["data"].(map[string]interface{})
	assert.Equal(suite.T(), false, data["is_active"])

	// Активируем компанию
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/api/accounts/%d/activate", company.ID), nil)
	w = httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])
	assert.Contains(suite.T(), response["message"], "активирована")

	data = response["data"].(map[string]interface{})
	assert.Equal(suite.T(), true, data["is_active"])
}

// TestDeleteCompany тестирует удаление компании
func (suite *CompaniesTestSuite) TestDeleteCompany() {
	// Создаем тестовую компанию
	company := &models.Company{
		Name:           "Компания для удаления",
		AxetnaLogin:    "delete_user",
		AxetnaPassword: "password",
		IsActive:       true,
	}
	suite.db.Create(company)

	// Удаляем компанию
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("/api/accounts/%d", company.ID), nil)
	w := httptest.NewRecorder()
	suite.router.ServeHTTP(w, req)

	assert.Equal(suite.T(), http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(suite.T(), err)

	assert.Equal(suite.T(), "success", response["status"])
	assert.Contains(suite.T(), response["message"], "удалена")

	// Проверяем, что компания действительно удалена (мягкое удаление)
	var deletedCompany models.Company
	err = suite.db.Where("id = ?", company.ID).First(&deletedCompany).Error
	assert.Error(suite.T(), err) // Должна быть ошибка, так как запись помечена как удаленная

	// Проверяем с учетом мягкого удаления
	err = suite.db.Unscoped().Where("id = ?", company.ID).First(&deletedCompany).Error
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), deletedCompany.DeletedAt)
}

// TestPasswordEncryption тестирует шифрование/дешифрование паролей
func (suite *CompaniesTestSuite) TestPasswordEncryption() {
	originalPassword := "test_password_123"

	encrypted := suite.api.encryptPassword(originalPassword)
	assert.NotEqual(suite.T(), originalPassword, encrypted)

	decrypted := suite.api.decryptPassword(encrypted)
	assert.Equal(suite.T(), originalPassword, decrypted)
}

func TestCompaniesTestSuite(t *testing.T) {
	suite.Run(t, new(CompaniesTestSuite))
}
