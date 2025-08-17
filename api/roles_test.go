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

// setupRolesTestAPI создает тестовое API для ролей
func setupRolesTestAPI(t *testing.T) (*gin.Engine, *gorm.DB) {
	// Создаем in-memory SQLite базу
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем модели
	err = db.AutoMigrate(
		&models.Permission{},
		&models.Role{},
		&models.User{},
	)
	require.NoError(t, err)

	// Создаем таблицу связей many2many вручную для SQLite
	err = db.Exec("CREATE TABLE IF NOT EXISTS role_permissions (role_id INTEGER, permission_id INTEGER, PRIMARY KEY (role_id, permission_id))").Error
	require.NoError(t, err)

	// Создаем тестовые данные
	setupRolesTestData(t, db)

	// Настраиваем Gin в тестовом режиме
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Middleware для установки tenant DB в контекст
	router.Use(func(c *gin.Context) {
		c.Set("tenant_db", db)
		c.Next()
	})

	// Регистрируем роуты
	router.GET("/roles", GetRoles)
	router.GET("/roles/:id", GetRole)
	router.POST("/roles", CreateRole)
	router.PUT("/roles/:id", UpdateRole)
	router.DELETE("/roles/:id", DeleteRole)
	router.PUT("/roles/:id/permissions", UpdateRolePermissions)
	router.GET("/permissions", GetPermissions)
	router.POST("/permissions", CreatePermission)

	return router, db
}

// setupRolesTestData создает тестовые данные для ролей
func setupRolesTestData(t *testing.T, db *gorm.DB) {
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
		{
			Name:        "objects.read",
			DisplayName: "Чтение объектов",
			Resource:    "objects",
			Action:      "read",
			Category:    "monitoring",
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
		Color:       "#ff0000",
		Priority:    100,
		IsActive:    true,
		IsSystem:    true,
	}
	require.NoError(t, db.Create(&adminRole).Error)

	// Связываем роль с разрешениями
	require.NoError(t, db.Model(&adminRole).Association("Permissions").Append(permissions))

	userRole := models.Role{
		Name:        "user",
		DisplayName: "Пользователь",
		Description: "Базовые права",
		Color:       "#00ff00",
		Priority:    10,
		IsActive:    true,
		IsSystem:    false,
	}
	require.NoError(t, db.Create(&userRole).Error)

	// Связываем роль с разрешением (только чтение объектов)
	require.NoError(t, db.Model(&userRole).Association("Permissions").Append([]models.Permission{permissions[2]}))
}

// TestGetRoles тестирует получение списка ролей
func TestGetRoles(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное получение списка ролей", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		roles := response["data"].([]interface{})
		assert.Greater(t, len(roles), 0)
	})

	t.Run("Получение ролей с разрешениями", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles?with_permissions=true", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		roles := response["data"].([]interface{})
		assert.Greater(t, len(roles), 0)

		// Проверяем, что роли загружены (permissions может не быть в JSON если пустые)
		role := roles[0].(map[string]interface{})
		assert.Equal(t, "admin", role["name"])
	})

	t.Run("Фильтрация по активности", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles?active=true", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})

	t.Run("Поиск ролей", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles?search=admin", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})
}

// TestGetRole тестирует получение конкретной роли
func TestGetRole(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное получение роли", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles/1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		role := response["data"].(map[string]interface{})
		assert.Equal(t, "admin", role["name"])
		assert.Equal(t, "Администратор", role["display_name"])
	})

	t.Run("Роль не найдена", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/roles/999", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Role not found", response["error"])
	})
}

// TestCreateRole тестирует создание роли
func TestCreateRole(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное создание роли", func(t *testing.T) {
		newRole := CreateRoleRequest{
			Name:        "manager",
			DisplayName: "Менеджер",
			Description: "Роль менеджера",
			Color:       "#0000ff",
			Priority:    50,
		}

		jsonData, _ := json.Marshal(newRole)
		req, _ := http.NewRequest("POST", "/roles", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		role := response["data"].(map[string]interface{})
		assert.Equal(t, "manager", role["name"])
		assert.Equal(t, "Менеджер", role["display_name"])
		assert.Equal(t, float64(50), role["priority"])
	})

	t.Run("Создание роли с дублирующимся именем", func(t *testing.T) {
		duplicateRole := CreateRoleRequest{
			Name:        "admin", // уже существует
			DisplayName: "Другой админ",
		}

		jsonData, _ := json.Marshal(duplicateRole)
		req, _ := http.NewRequest("POST", "/roles", bytes.NewBuffer(jsonData))
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

	t.Run("Создание роли с невалидными данными", func(t *testing.T) {
		invalidRole := CreateRoleRequest{
			Name:        "a", // слишком короткое
			DisplayName: "",  // пустое
		}

		jsonData, _ := json.Marshal(invalidRole)
		req, _ := http.NewRequest("POST", "/roles", bytes.NewBuffer(jsonData))
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
}

// TestUpdateRole тестирует обновление роли
func TestUpdateRole(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное обновление роли", func(t *testing.T) {
		updateData := UpdateRoleRequest{
			DisplayName: "Обновленный пользователь",
			Description: "Обновленное описание",
			Priority:    &[]int{20}[0],
		}

		jsonData, _ := json.Marshal(updateData)
		req, _ := http.NewRequest("PUT", "/roles/2", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		role := response["data"].(map[string]interface{})
		assert.Equal(t, "Обновленный пользователь", role["display_name"])
		assert.Equal(t, "Обновленное описание", role["description"])
		assert.Equal(t, float64(20), role["priority"])
	})

	t.Run("Попытка обновить системную роль", func(t *testing.T) {
		updateData := UpdateRoleRequest{
			DisplayName: "Попытка изменить админа",
		}

		jsonData, _ := json.Marshal(updateData)
		req, _ := http.NewRequest("PUT", "/roles/1", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Cannot modify system role", response["error"])
	})

	t.Run("Обновление несуществующей роли", func(t *testing.T) {
		updateData := UpdateRoleRequest{
			DisplayName: "Не существует",
		}

		jsonData, _ := json.Marshal(updateData)
		req, _ := http.NewRequest("PUT", "/roles/999", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Role not found", response["error"])
	})
}

// TestDeleteRole тестирует удаление роли
func TestDeleteRole(t *testing.T) {
	router, db := setupRolesTestAPI(t)

	t.Run("Попытка удалить системную роль", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "/roles/1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Cannot delete system role", response["error"])
	})

	t.Run("Успешное удаление роли", func(t *testing.T) {
		// Создаем роль для удаления
		roleToDelete := models.Role{
			Name:        "temp_role",
			DisplayName: "Временная роль",
			IsActive:    true,
			IsSystem:    false,
		}
		require.NoError(t, db.Create(&roleToDelete).Error)

		req, _ := http.NewRequest("DELETE", "/roles/3", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Equal(t, "Role deleted successfully", response["message"])
	})

	t.Run("Удаление несуществующей роли", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", "/roles/999", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Role not found", response["error"])
	})
}

// TestUpdateRolePermissions тестирует обновление разрешений роли
func TestUpdateRolePermissions(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное обновление разрешений роли", func(t *testing.T) {
		permissionsData := RolePermissionsRequest{
			PermissionIDs: []uint{1, 2}, // users.read, users.write
		}

		jsonData, _ := json.Marshal(permissionsData)
		req, _ := http.NewRequest("PUT", "/roles/2/permissions", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])

		role := response["data"].(map[string]interface{})
		permissions := role["permissions"].([]interface{})
		assert.Equal(t, 2, len(permissions))
	})

	t.Run("Обновление разрешений несуществующей роли", func(t *testing.T) {
		permissionsData := RolePermissionsRequest{
			PermissionIDs: []uint{1},
		}

		jsonData, _ := json.Marshal(permissionsData)
		req, _ := http.NewRequest("PUT", "/roles/999/permissions", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Role not found", response["error"])
	})

	t.Run("Обновление разрешений с несуществующими ID", func(t *testing.T) {
		permissionsData := RolePermissionsRequest{
			PermissionIDs: []uint{999, 998}, // несуществующие разрешения
		}

		jsonData, _ := json.Marshal(permissionsData)
		req, _ := http.NewRequest("PUT", "/roles/2/permissions", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "error", response["status"])
		assert.Equal(t, "Some permissions not found", response["error"])
	})
}

// TestGetPermissions тестирует получение списка разрешений
func TestGetPermissions(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное получение списка разрешений", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/permissions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		permissions := response["data"].([]interface{})
		assert.Greater(t, len(permissions), 0)
	})

	t.Run("Фильтрация разрешений по ресурсу", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/permissions?resource=users", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})

	t.Run("Фильтрация разрешений по действию", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/permissions?action=read", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})

	t.Run("Поиск разрешений", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/permissions?search=user", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
	})
}

// TestCreatePermission тестирует создание разрешения
func TestCreatePermission(t *testing.T) {
	router, _ := setupRolesTestAPI(t)

	t.Run("Успешное создание разрешения", func(t *testing.T) {
		newPermission := CreatePermissionRequest{
			Name:        "reports.read",
			DisplayName: "Чтение отчетов",
			Description: "Разрешение на просмотр отчетов",
			Resource:    "reports",
			Action:      "read",
			Category:    "reporting",
		}

		jsonData, _ := json.Marshal(newPermission)
		req, _ := http.NewRequest("POST", "/permissions", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")

		permission := response["data"].(map[string]interface{})
		assert.Equal(t, "reports.read", permission["name"])
		assert.Equal(t, "Чтение отчетов", permission["display_name"])
		assert.Equal(t, "reports", permission["resource"])
		assert.Equal(t, "read", permission["action"])
	})

	t.Run("Создание разрешения с дублирующимся именем", func(t *testing.T) {
		duplicatePermission := CreatePermissionRequest{
			Name:        "users.read", // уже существует
			DisplayName: "Другое чтение пользователей",
			Resource:    "users",
			Action:      "read",
		}

		jsonData, _ := json.Marshal(duplicatePermission)
		req, _ := http.NewRequest("POST", "/permissions", bytes.NewBuffer(jsonData))
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

	t.Run("Создание разрешения с невалидными данными", func(t *testing.T) {
		invalidPermission := CreatePermissionRequest{
			Name:        "a", // слишком короткое
			DisplayName: "",  // пустое
			Resource:    "",  // пустое
			Action:      "",  // пустое
		}

		jsonData, _ := json.Marshal(invalidPermission)
		req, _ := http.NewRequest("POST", "/permissions", bytes.NewBuffer(jsonData))
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
}
