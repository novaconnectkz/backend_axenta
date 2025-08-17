package api

import (
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

// setupTestAPI создает тестовое API с in-memory базой данных
func setupTestAPI(t *testing.T) (*gin.Engine, *gorm.DB) {
	// Создаем in-memory SQLite базу
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Permission{},
		&models.Role{},
		&models.User{},
		&models.UserTemplate{},
	)
	require.NoError(t, err)

	// Создаем таблицу связей many2many вручную для SQLite
	err = db.Exec("CREATE TABLE IF NOT EXISTS role_permissions (role_id INTEGER, permission_id INTEGER, PRIMARY KEY (role_id, permission_id))").Error
	require.NoError(t, err)

	// Создаем тестовые данные
	setupTestData(t, db)

	// Настраиваем Gin в тестовом режиме
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant DB в контекст
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	// Регистрируем роуты
	router.GET("/users", GetUsers)
	router.GET("/users/:id", GetUser)
	router.POST("/users", CreateUser)
	router.PUT("/users/:id", UpdateUser)
	router.DELETE("/users/:id", DeleteUser)

	return router, db
}

// setupTestData создает тестовые данные
func setupTestData(t *testing.T, db *gorm.DB) {
	// Создаем разрешения
	permissions := []models.Permission{
		{
			Name:        "users.read",
			DisplayName: "Чтение пользователей",
			Resource:    "users",
			Action:      "read",
			Category:    "management",
			IsActive:    true,
		},
		{
			Name:        "users.write",
			DisplayName: "Запись пользователей",
			Resource:    "users",
			Action:      "write",
			Category:    "management",
			IsActive:    true,
		},
	}

	for _, perm := range permissions {
		require.NoError(t, db.Create(&perm).Error)
	}

	// Создаем роли
	adminRole := models.Role{
		Name:        "admin",
		DisplayName: "Администратор",
		Description: "Полные права доступа",
		Priority:    100,
		IsActive:    true,
		IsSystem:    false,
	}
	require.NoError(t, db.Create(&adminRole).Error)

	// Связываем роль с разрешениями
	require.NoError(t, db.Model(&adminRole).Association("Permissions").Append(permissions))

	userRole := models.Role{
		Name:        "user",
		DisplayName: "Пользователь",
		Description: "Базовые права",
		Priority:    10,
		IsActive:    true,
		IsSystem:    false,
	}
	require.NoError(t, db.Create(&userRole).Error)

	// Создаем шаблон пользователя
	template := models.UserTemplate{
		Name:        "Стандартный пользователь",
		Description: "Шаблон для обычных пользователей",
		RoleID:      userRole.ID,
		Settings:    `{"theme": "light", "language": "ru"}`,
		IsActive:    true,
	}
	require.NoError(t, db.Create(&template).Error)

	// Создаем тестового пользователя
	user := models.User{
		Username:  "testuser",
		Email:     "test@example.com",
		Password:  "$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi", // password
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		RoleID:    adminRole.ID,
	}
	require.NoError(t, db.Create(&user).Error)
}

// TestGetUsers тестирует получение списка пользователей
func TestGetUsers(t *testing.T) {
	router, _ := setupTestAPI(t)

	t.Run("Успешное получение списка пользователей", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		data := response["data"].(map[string]interface{})
		assert.Contains(t, data, "items")
		assert.Contains(t, data, "total")

		items := data["items"].([]interface{})
		assert.Greater(t, len(items), 0)
	})

	t.Run("Фильтрация по активности", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users?active=true", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})

	t.Run("Поиск пользователей", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users?search=test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})
}

// TestGetUser тестирует получение конкретного пользователя
func TestGetUser(t *testing.T) {
	router, _ := setupTestAPI(t)

	t.Run("Успешное получение пользователя", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users/1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		user := response["data"].(map[string]interface{})
		assert.Equal(t, "testuser", user["username"])
		assert.Equal(t, "test@example.com", user["email"])
	})

	t.Run("Пользователь не найден", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users/999", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "User not found", response["error"])
	})

	t.Run("Неверный ID пользователя", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/users/invalid", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Invalid user ID", response["error"])
	})
}

// TestCreateUser тестирует создание пользователя
func TestCreateUser(t *testing.T) {
	router, _ := setupTestAPI(t)

	t.Run("Успешное создание пользователя", func(t *testing.T) {
		newUser := CreateUserRequest{
			Username:  "newuser",
			Email:     "newuser@example.com",
			Password:  "password123",
			FirstName: "New",
			LastName:  "User",
			RoleID:    1, // admin role
		}

		jsonData, _ := json.Marshal(newUser)
		req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		user := response["data"].(map[string]interface{})
		assert.Equal(t, "newuser", user["username"])
		assert.Equal(t, "newuser@example.com", user["email"])
	})

	t.Run("Создание пользователя с дублирующимся username", func(t *testing.T) {
		duplicateUser := CreateUserRequest{
			Username: "testuser", // уже существует
			Email:    "duplicate@example.com",
			Password: "password123",
			RoleID:   1,
		}

		jsonData, _ := json.Marshal(duplicateUser)
		req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Contains(t, response["error"].(string), "already exists")
	})

	t.Run("Создание пользователя с невалидными данными", func(t *testing.T) {
		invalidUser := CreateUserRequest{
			Username: "ab", // слишком короткий
			Email:    "invalid-email",
			Password: "123", // слишком короткий
			RoleID:   1,
		}

		jsonData, _ := json.Marshal(invalidUser)
		req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Contains(t, response["error"].(string), "Invalid request data")
	})

	t.Run("Создание пользователя с несуществующей ролью", func(t *testing.T) {
		userWithInvalidRole := CreateUserRequest{
			Username: "userinvalidrole",
			Email:    "userinvalidrole@example.com",
			Password: "password123",
			RoleID:   999, // несуществующая роль
		}

		jsonData, _ := json.Marshal(userWithInvalidRole)
		req, _ := http.NewRequest("POST", "/users", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Role not found", response["error"])
	})
}

// TestUpdateUser тестирует обновление пользователя
func TestUpdateUser(t *testing.T) {
	router, _ := setupTestAPI(t)

	t.Run("Успешное обновление пользователя", func(t *testing.T) {
		updateData := UpdateUserRequest{
			FirstName: "Updated",
			LastName:  "Name",
		}

		jsonData, _ := json.Marshal(updateData)
		req, _ := http.NewRequest("PUT", "/users/1", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		user := response["data"].(map[string]interface{})
		assert.Equal(t, "Updated", user["first_name"])
		assert.Equal(t, "Name", user["last_name"])
	})

	t.Run("Обновление несуществующего пользователя", func(t *testing.T) {
		updateData := UpdateUserRequest{
			FirstName: "Updated",
		}

		jsonData, _ := json.Marshal(updateData)
		req, _ := http.NewRequest("PUT", "/users/999", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "User not found", response["error"])
	})
}

// TestDeleteUser тестирует удаление пользователя
func TestDeleteUser(t *testing.T) {
	router, db := setupTestAPI(t)

	t.Run("Успешное удаление пользователя", func(t *testing.T) {
		// Создаем дополнительного пользователя для удаления
		user := models.User{
			Username: "userToDelete",
			Email:    "delete@example.com",
			Password: "password",
			RoleID:   2, // user role
		}
		require.NoError(t, db.Create(&user).Error)

		req, _ := http.NewRequest("DELETE", "/users/2", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "User deleted successfully", response["message"])

		// Проверяем, что пользователь действительно удален (soft delete)
		var deletedUser models.User
		err = db.First(&deletedUser, user.ID).Error
		assert.Error(t, err) // Должна быть ошибка, так как пользователь удален

		// Проверяем с Unscoped - пользователь должен быть найден
		err = db.Unscoped().First(&deletedUser, user.ID).Error
		assert.NoError(t, err)
		assert.NotNil(t, deletedUser.DeletedAt)
	})

	t.Run("Удаление несуществующего пользователя", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "/users/999", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "User not found", response["error"])
	})
}
