package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB создает тестовую базу данных в памяти
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем все модели
	err = db.AutoMigrate(
		&Permission{},
		&Role{},
		&User{},
		&UserTemplate{},
		&Company{},
		&BillingPlan{},
		&Object{},
		&Contract{},
		&ContractAppendix{},
		&Location{},
		&Installer{},
		&Equipment{},
		&Installation{},
		&ObjectTemplate{},
		&MonitoringTemplate{},
		&NotificationTemplate{},
		&TariffPlan{},
		&Subscription{},
	)
	require.NoError(t, err)

	return db
}

// TestUserModel тестирует модель User
func TestUserModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание пользователя", func(t *testing.T) {
		user := User{
			Username:  "testuser",
			Email:     "test@example.com",
			Password:  "hashedpassword",
			FirstName: "Test",
			LastName:  "User",
			IsActive:  true,
		}

		err := db.Create(&user).Error
		require.NoError(t, err)
		assert.NotZero(t, user.ID)
		assert.NotZero(t, user.CreatedAt)
	})

	t.Run("Уникальность username и email", func(t *testing.T) {
		user1 := User{
			Username: "unique_user",
			Email:    "unique@example.com",
			Password: "password1",
		}
		err := db.Create(&user1).Error
		require.NoError(t, err)

		// Попытка создать пользователя с тем же username
		user2 := User{
			Username: "unique_user",
			Email:    "different@example.com",
			Password: "password2",
		}
		err = db.Create(&user2).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования username")

		// Попытка создать пользователя с тем же email
		user3 := User{
			Username: "different_user",
			Email:    "unique@example.com",
			Password: "password3",
		}
		err = db.Create(&user3).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования email")
	})

	t.Run("Soft Delete", func(t *testing.T) {
		user := User{
			Username: "soft_delete_user",
			Email:    "softdelete@example.com",
			Password: "password",
		}
		err := db.Create(&user).Error
		require.NoError(t, err)

		userID := user.ID

		// Удаляем пользователя (soft delete)
		err = db.Delete(&user).Error
		require.NoError(t, err)

		// Проверяем, что пользователь не найден обычным запросом
		var foundUser User
		err = db.First(&foundUser, userID).Error
		assert.Error(t, err)

		// Проверяем, что пользователь найден с Unscoped
		err = db.Unscoped().First(&foundUser, userID).Error
		require.NoError(t, err)
		assert.NotNil(t, foundUser.DeletedAt)
	})

	t.Run("Связь с ролью", func(t *testing.T) {
		// Создаем роль
		role := Role{
			Name:        "test_role",
			DisplayName: "Test Role",
			Description: "Test role for testing",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем пользователя с ролью
		user := User{
			Username: "role_user",
			Email:    "role@example.com",
			Password: "password",
			RoleID:   role.ID,
		}
		err = db.Create(&user).Error
		require.NoError(t, err)

		// Загружаем пользователя с ролью
		var userWithRole User
		err = db.Preload("Role").First(&userWithRole, user.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, userWithRole.Role)
		assert.Equal(t, "test_role", userWithRole.Role.Name)
	})

	t.Run("Связь с шаблоном пользователя", func(t *testing.T) {
		// Создаем роль для шаблона
		role := Role{
			Name:        "template_role",
			DisplayName: "Template Role",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем шаблон пользователя
		template := UserTemplate{
			Name:        "Test Template",
			Description: "Test user template",
			RoleID:      role.ID,
			Settings:    `{"theme": "dark", "language": "ru"}`,
			IsActive:    true,
		}
		err = db.Create(&template).Error
		require.NoError(t, err)

		// Создаем пользователя с шаблоном
		user := User{
			Username:   "template_user",
			Email:      "template@example.com",
			Password:   "password",
			TemplateID: &template.ID,
		}
		err = db.Create(&user).Error
		require.NoError(t, err)

		// Загружаем пользователя с шаблоном
		var userWithTemplate User
		err = db.Preload("Template").First(&userWithTemplate, user.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, userWithTemplate.Template)
		assert.Equal(t, "Test Template", userWithTemplate.Template.Name)
	})

	t.Run("Обновление времени последнего входа", func(t *testing.T) {
		user := User{
			Username:   "login_user",
			Email:      "login@example.com",
			Password:   "password",
			LoginCount: 0,
		}
		err := db.Create(&user).Error
		require.NoError(t, err)

		// Обновляем время последнего входа
		now := time.Now()
		user.LastLogin = &now
		user.LoginCount = 1

		err = db.Save(&user).Error
		require.NoError(t, err)

		// Проверяем обновление
		var updatedUser User
		err = db.First(&updatedUser, user.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, updatedUser.LastLogin)
		assert.Equal(t, 1, updatedUser.LoginCount)
	})
}

// TestPermissionModel тестирует модель Permission
func TestPermissionModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание разрешения", func(t *testing.T) {
		permission := Permission{
			Name:        "objects.create",
			DisplayName: "Создать объект",
			Description: "Разрешение на создание объектов мониторинга",
			Resource:    "objects",
			Action:      "create",
			Category:    "management",
			IsActive:    true,
		}

		err := db.Create(&permission).Error
		require.NoError(t, err)
		assert.NotZero(t, permission.ID)
		assert.Equal(t, "objects.create", permission.Name)
	})

	t.Run("Уникальность имени разрешения", func(t *testing.T) {
		permission1 := Permission{
			Name:        "unique.permission",
			DisplayName: "Unique Permission",
			Resource:    "test",
			Action:      "read",
			IsActive:    true,
		}
		err := db.Create(&permission1).Error
		require.NoError(t, err)

		// Попытка создать разрешение с тем же именем
		permission2 := Permission{
			Name:        "unique.permission",
			DisplayName: "Another Permission",
			Resource:    "test",
			Action:      "write",
			IsActive:    true,
		}
		err = db.Create(&permission2).Error
		assert.Error(t, err, "Должна быть ошибка из-за дублирования имени разрешения")
	})

	t.Run("Валидация полей", func(t *testing.T) {
		permission := Permission{
			Name:        "test.permission",
			DisplayName: "Test Permission",
			Resource:    "test_resource",
			Action:      "test_action",
			Category:    "test_category",
		}

		err := db.Create(&permission).Error
		require.NoError(t, err)
		assert.Equal(t, "test_resource", permission.Resource)
		assert.Equal(t, "test_action", permission.Action)
		assert.Equal(t, "test_category", permission.Category)
	})
}

// TestRoleModel тестирует модель Role
func TestRoleModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание роли", func(t *testing.T) {
		role := Role{
			Name:        "admin",
			DisplayName: "Администратор",
			Description: "Полные права доступа к системе",
			Color:       "#ff0000",
			Priority:    100,
			IsActive:    true,
			IsSystem:    false,
		}

		err := db.Create(&role).Error
		require.NoError(t, err)
		assert.NotZero(t, role.ID)
		assert.Equal(t, "admin", role.Name)
		assert.Equal(t, 100, role.Priority)
	})

	t.Run("Связь роли с разрешениями", func(t *testing.T) {
		// Создаем разрешения
		permission1 := Permission{
			Name:        "users.read",
			DisplayName: "Чтение пользователей",
			Resource:    "users",
			Action:      "read",
			IsActive:    true,
		}
		permission2 := Permission{
			Name:        "users.write",
			DisplayName: "Запись пользователей",
			Resource:    "users",
			Action:      "write",
			IsActive:    true,
		}

		err := db.Create(&permission1).Error
		require.NoError(t, err)
		err = db.Create(&permission2).Error
		require.NoError(t, err)

		// Создаем роль
		role := Role{
			Name:        "manager",
			DisplayName: "Менеджер",
			IsActive:    true,
		}
		err = db.Create(&role).Error
		require.NoError(t, err)

		// Связываем роль с разрешениями
		err = db.Model(&role).Association("Permissions").Append(&permission1, &permission2)
		require.NoError(t, err)

		// Загружаем роль с разрешениями
		var roleWithPermissions Role
		err = db.Preload("Permissions").First(&roleWithPermissions, role.ID).Error
		require.NoError(t, err)
		assert.Len(t, roleWithPermissions.Permissions, 2)
	})

	t.Run("Метод HasPermission", func(t *testing.T) {
		// Создаем разрешение
		permission := Permission{
			Name:        "objects.delete",
			DisplayName: "Удаление объектов",
			Resource:    "objects",
			Action:      "delete",
			IsActive:    true,
		}
		err := db.Create(&permission).Error
		require.NoError(t, err)

		// Создаем роль с разрешением
		role := Role{
			Name:        "operator",
			DisplayName: "Оператор",
			IsActive:    true,
			Permissions: []Permission{permission},
		}
		err = db.Create(&role).Error
		require.NoError(t, err)

		// Загружаем роль с разрешениями
		var loadedRole Role
		err = db.Preload("Permissions").First(&loadedRole, role.ID).Error
		require.NoError(t, err)

		// Тестируем метод HasPermission
		assert.True(t, loadedRole.HasPermission("objects.delete"))
		assert.False(t, loadedRole.HasPermission("objects.create"))
	})

	t.Run("Метод HasPermissionFor", func(t *testing.T) {
		// Создаем разрешения
		permissions := []Permission{
			{
				Name:        "reports.read",
				DisplayName: "Чтение отчетов",
				Resource:    "reports",
				Action:      "read",
				IsActive:    true,
			},
			{
				Name:        "reports.*",
				DisplayName: "Все права на отчеты",
				Resource:    "reports",
				Action:      "*",
				IsActive:    true,
			},
			{
				Name:        "*.*",
				DisplayName: "Все права",
				Resource:    "*",
				Action:      "*",
				IsActive:    true,
			},
		}

		for _, perm := range permissions {
			err := db.Create(&perm).Error
			require.NoError(t, err)
		}

		// Создаем роль с wildcard разрешениями
		role := Role{
			Name:        "super_admin",
			DisplayName: "Супер Администратор",
			IsActive:    true,
			Permissions: permissions,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Загружаем роль с разрешениями
		var loadedRole Role
		err = db.Preload("Permissions").First(&loadedRole, role.ID).Error
		require.NoError(t, err)

		// Тестируем метод HasPermissionFor
		assert.True(t, loadedRole.HasPermissionFor("reports", "read"))
		assert.True(t, loadedRole.HasPermissionFor("reports", "write"))
		assert.True(t, loadedRole.HasPermissionFor("users", "delete"))
	})

	t.Run("Метод GetPermissionNames", func(t *testing.T) {
		// Создаем разрешения
		permissions := []Permission{
			{
				Name:        "billing.read",
				DisplayName: "Чтение биллинга",
				Resource:    "billing",
				Action:      "read",
				IsActive:    true,
			},
			{
				Name:        "billing.write",
				DisplayName: "Запись биллинга",
				Resource:    "billing",
				Action:      "write",
				IsActive:    true,
			},
		}

		for _, perm := range permissions {
			err := db.Create(&perm).Error
			require.NoError(t, err)
		}

		// Создаем роль
		role := Role{
			Name:        "accountant",
			DisplayName: "Бухгалтер",
			IsActive:    true,
			Permissions: permissions,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Загружаем роль с разрешениями
		var loadedRole Role
		err = db.Preload("Permissions").First(&loadedRole, role.ID).Error
		require.NoError(t, err)

		// Тестируем метод GetPermissionNames
		permissionNames := loadedRole.GetPermissionNames()
		assert.Len(t, permissionNames, 2)
		assert.Contains(t, permissionNames, "billing.read")
		assert.Contains(t, permissionNames, "billing.write")
	})
}

// TestUserTemplateModel тестирует модель UserTemplate
func TestUserTemplateModel(t *testing.T) {
	db := setupTestDB(t)

	t.Run("Создание шаблона пользователя", func(t *testing.T) {
		// Создаем роль для шаблона
		role := Role{
			Name:        "default_user",
			DisplayName: "Обычный пользователь",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем шаблон
		template := UserTemplate{
			Name:        "Стандартный пользователь",
			Description: "Шаблон для обычных пользователей системы",
			RoleID:      role.ID,
			Settings:    `{"theme": "light", "language": "ru", "notifications": true}`,
			IsActive:    true,
		}

		err = db.Create(&template).Error
		require.NoError(t, err)
		assert.NotZero(t, template.ID)
		assert.Equal(t, "Стандартный пользователь", template.Name)
	})

	t.Run("Связь шаблона с ролью", func(t *testing.T) {
		// Создаем роль
		role := Role{
			Name:        "template_test_role",
			DisplayName: "Роль для тестирования шаблонов",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем шаблон с ролью
		template := UserTemplate{
			Name:        "Тестовый шаблон",
			Description: "Шаблон для тестирования",
			RoleID:      role.ID,
			IsActive:    true,
		}
		err = db.Create(&template).Error
		require.NoError(t, err)

		// Загружаем шаблон с ролью
		var templateWithRole UserTemplate
		err = db.Preload("Role").First(&templateWithRole, template.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, templateWithRole.Role)
		assert.Equal(t, "template_test_role", templateWithRole.Role.Name)
	})

	t.Run("Связь шаблона с пользователями", func(t *testing.T) {
		// Создаем роль
		role := Role{
			Name:        "user_template_role",
			DisplayName: "Роль пользователя шаблона",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем шаблон
		template := UserTemplate{
			Name:        "Шаблон с пользователями",
			Description: "Шаблон для тестирования связи с пользователями",
			RoleID:      role.ID,
			IsActive:    true,
		}
		err = db.Create(&template).Error
		require.NoError(t, err)

		// Создаем пользователей с этим шаблоном
		user1 := User{
			Username:   "template_user1",
			Email:      "template_user1@example.com",
			Password:   "password",
			TemplateID: &template.ID,
		}
		user2 := User{
			Username:   "template_user2",
			Email:      "template_user2@example.com",
			Password:   "password",
			TemplateID: &template.ID,
		}

		err = db.Create(&user1).Error
		require.NoError(t, err)
		err = db.Create(&user2).Error
		require.NoError(t, err)

		// Загружаем шаблон с пользователями
		var templateWithUsers UserTemplate
		err = db.Preload("Users").First(&templateWithUsers, template.ID).Error
		require.NoError(t, err)
		assert.Len(t, templateWithUsers.Users, 2)
	})

	t.Run("JSON настройки шаблона", func(t *testing.T) {
		// Создаем роль
		role := Role{
			Name:        "settings_role",
			DisplayName: "Роль для настроек",
			IsActive:    true,
		}
		err := db.Create(&role).Error
		require.NoError(t, err)

		// Создаем шаблон с JSON настройками
		settings := `{
			"theme": "dark",
			"language": "en",
			"notifications": {
				"email": true,
				"sms": false,
				"telegram": true
			},
			"dashboard": {
				"widgets": ["objects", "alerts", "reports"],
				"refresh_interval": 30
			}
		}`

		template := UserTemplate{
			Name:        "Расширенный шаблон",
			Description: "Шаблон с подробными настройками",
			RoleID:      role.ID,
			Settings:    settings,
			IsActive:    true,
		}

		err = db.Create(&template).Error
		require.NoError(t, err)

		// Загружаем шаблон и проверяем настройки
		var loadedTemplate UserTemplate
		err = db.First(&loadedTemplate, template.ID).Error
		require.NoError(t, err)
		assert.Contains(t, loadedTemplate.Settings, "dark")
		assert.Contains(t, loadedTemplate.Settings, "widgets")
	})
}
