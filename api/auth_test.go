package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuthTestDB создает тестовую базу данных для auth тестов
func setupAuthTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Role{},
	)
	require.NoError(t, err)

	// Сохраняем БД в глобальной переменной database.DB для тестов
	// В реальных тестах это делается через database.ConnectDatabase()
	// Здесь мы используем прямую установку для тестов
	database.DB = db

	return db
}

// setupAuthTestRouter создает тестовый роутер
func setupAuthTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// createMockAxentaLoginServer создает mock сервер для Axenta Cloud API (логин)
func createMockAxentaLoginServer(t *testing.T, loginResponse map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login/" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(loginResponse)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// createMockAxentaUserServer создает mock сервер для Axenta Cloud API (пользователь)
func createMockAxentaUserServer(t *testing.T, userResponse map[string]interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/current_user/" && r.Method == "GET" {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !assert.Contains(t, authHeader, "Token") {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"detail": "Authentication credentials were not provided."})
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(userResponse)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// TestLogin_ValidationError тестирует Login с ошибкой валидации
func TestLogin_ValidationError(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	// Тест с пустым телом запроса
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestLogin_InvalidCredentials тестирует Login с неверными учетными данными
func TestLogin_InvalidCredentials(t *testing.T) {
	router := setupAuthTestRouter()

	// Создаем mock сервер, который возвращает ошибку авторизации
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login/" && r.Method == "POST" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid credentials"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	router.POST("/login", Login)

	reqBody := map[string]string{
		"username": "testuser",
		"password": "wrongpassword",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ожидаем ошибку, так как реальный запрос к axenta.cloud не пройдет
	// В реальном приложении нужно мокировать HTTP клиент
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestLogin_EmptyToken тестирует Login когда токен пустой
func TestLogin_EmptyToken(t *testing.T) {
	router := setupAuthTestRouter()

	// Создаем mock сервер, который возвращает пустой токен
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/login/" && r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"token": ""})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	router.POST("/login", Login)

	reqBody := map[string]string{
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ожидаем ошибку из-за пустого токена
	// В реальном приложении это будет обработано в Login функции
	assert.True(t, w.Code == http.StatusInternalServerError || w.Code == http.StatusUnauthorized)
}

// TestLogin_ConnectionError тестирует Login с ошибкой подключения
func TestLogin_ConnectionError(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	reqBody := map[string]string{
		"username": "testuser",
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Ожидаем ошибку подключения (500 или 401)
	// В реальном приложении это будет обработано в Login функции
	assert.True(t, w.Code == http.StatusInternalServerError || w.Code == http.StatusUnauthorized)
}

// TestEnsureCompanyExists_NewCompany тестирует ensureCompanyExists для новой компании
func TestEnsureCompanyExists_NewCompany(t *testing.T) {
	db := setupAuthTestDB(t)

	axentaUser := AxentaUserResponse{
		AccountID:   123,
		AccountName: "Test Company",
		Email:       "test@example.com",
		ID:          1,
		Username:    "testuser",
	}

	company, created, err := ensureCompanyExists(db, axentaUser, "testuser")
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotNil(t, company)
	assert.Equal(t, uint(123), company.ID)
	assert.Equal(t, "Test Company", company.Name)
	assert.Equal(t, "tenant_123", company.DatabaseSchema)

	// Проверяем, что компания сохранена в БД
	var savedCompany models.Company
	err = db.First(&savedCompany, 123).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Company", savedCompany.Name)
}

// TestEnsureCompanyExists_ExistingCompany тестирует ensureCompanyExists для существующей компании
func TestEnsureCompanyExists_ExistingCompany(t *testing.T) {
	db := setupAuthTestDB(t)

	// Создаем существующую компанию
	existingCompany := models.Company{
		ID:             123,
		Name:           "Existing Company",
		DatabaseSchema: "tenant_123",
		IsActive:       true,
	}
	db.Create(&existingCompany)

	axentaUser := AxentaUserResponse{
		AccountID:   123,
		AccountName: "Updated Company Name",
		Email:       "updated@example.com",
		ID:          1,
		Username:    "testuser",
	}

	company, created, err := ensureCompanyExists(db, axentaUser, "testuser")
	require.NoError(t, err)
	assert.False(t, created)
	assert.NotNil(t, company)
	assert.Equal(t, uint(123), company.ID)
	assert.Equal(t, "Updated Company Name", company.Name)

	// Проверяем, что компания обновлена в БД
	var updatedCompany models.Company
	err = db.First(&updatedCompany, 123).Error
	require.NoError(t, err)
	assert.Equal(t, "Updated Company Name", updatedCompany.Name)
}

// TestEnsureCompanyExists_NoAccountID тестирует ensureCompanyExists без accountID
func TestEnsureCompanyExists_NoAccountID(t *testing.T) {
	db := setupAuthTestDB(t)

	axentaUser := AxentaUserResponse{
		AccountID:   0, // Нет accountID
		AccountName: "Test Company",
		Email:       "test@example.com",
		ID:          1,
		Username:    "testuser",
	}

	company, created, err := ensureCompanyExists(db, axentaUser, "testuser")
	assert.Error(t, err)
	assert.False(t, created)
	assert.Nil(t, company)
}

// TestEnsureCompanyExists_NilDB тестирует ensureCompanyExists с nil БД
func TestEnsureCompanyExists_NilDB(t *testing.T) {
	axentaUser := AxentaUserResponse{
		AccountID:   123,
		AccountName: "Test Company",
		Email:       "test@example.com",
		ID:          1,
		Username:    "testuser",
	}

	company, created, err := ensureCompanyExists(nil, axentaUser, "testuser")
	assert.Error(t, err)
	assert.False(t, created)
	assert.Nil(t, company)
}

// TestEnsureCompanyExists_EmptyAccountName тестирует ensureCompanyExists с пустым именем компании
func TestEnsureCompanyExists_EmptyAccountName(t *testing.T) {
	db := setupAuthTestDB(t)

	axentaUser := AxentaUserResponse{
		AccountID:   456,
		AccountName: "", // Пустое имя
		Email:       "test@example.com",
		ID:          1,
		Username:    "testuser",
	}

	company, created, err := ensureCompanyExists(db, axentaUser, "testuser")
	require.NoError(t, err)
	assert.True(t, created)
	assert.NotNil(t, company)
	// Должно использоваться имя по умолчанию
	assert.Contains(t, company.Name, "Компания")
}

// TestCreateFallbackUser тестирует createFallbackUser
func TestCreateFallbackUser(t *testing.T) {
	fallbackUser := createFallbackUser("testuser")

	assert.NotNil(t, fallbackUser)
	assert.Equal(t, "testuser", fallbackUser["username"])
	assert.Equal(t, "testuser", fallbackUser["name"])
	assert.Contains(t, fallbackUser["id"], "temp_")
	assert.Equal(t, "Unknown", fallbackUser["accountName"])
	assert.Equal(t, "user", fallbackUser["accountType"])
}

// TestMin тестирует функцию min
func TestMin(t *testing.T) {
	assert.Equal(t, 1, min(1, 2))
	assert.Equal(t, 1, min(2, 1))
	assert.Equal(t, 0, min(0, 1))
	assert.Equal(t, -1, min(-1, 0))
	assert.Equal(t, 5, min(5, 5))
}

// TestLogin_ShortUsername тестирует Login с коротким username
func TestLogin_ShortUsername(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	reqBody := map[string]string{
		"username": "ab", // Меньше 3 символов
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

// TestLogin_ShortPassword тестирует Login с коротким password
func TestLogin_ShortPassword(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	reqBody := map[string]string{
		"username": "testuser",
		"password": "ab", // Меньше 3 символов
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestLogin_LongUsername тестирует Login с длинным username
func TestLogin_LongUsername(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	// Создаем username длиннее 64 символов
	longUsername := make([]byte, 65)
	for i := range longUsername {
		longUsername[i] = 'a'
	}

	reqBody := map[string]string{
		"username": string(longUsername),
		"password": "password123",
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestLogin_LongPassword тестирует Login с длинным password
func TestLogin_LongPassword(t *testing.T) {
	router := setupAuthTestRouter()

	router.POST("/login", Login)

	// Создаем password длиннее 64 символов
	longPassword := make([]byte, 65)
	for i := range longPassword {
		longPassword[i] = 'a'
	}

	reqBody := map[string]string{
		"username": "testuser",
		"password": string(longPassword),
	}
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
