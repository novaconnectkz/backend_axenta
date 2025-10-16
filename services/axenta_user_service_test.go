package services

import (
	"backend_axenta/models"
	"backend_axenta/testutils"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDBWithModels создает тестовую базу данных с основными моделями
func setupTestDBWithModels() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// Мигрируем основные модели для тестов
	err = db.AutoMigrate(
		&models.Role{},
		&models.Permission{},
		&models.User{},
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func TestAxentaUserService_EnsureDefaultRoles(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	// Тест создания ролей по умолчанию
	err = service.EnsureDefaultRoles()
	require.NoError(t, err)

	// Проверяем, что роли созданы
	var roles []models.Role
	err = db.Find(&roles).Error
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 3)

	// Проверяем конкретные роли
	var partnerRole models.Role
	err = db.Where("name = ?", "partner").First(&partnerRole).Error
	require.NoError(t, err)
	assert.Equal(t, "Партнер", partnerRole.DisplayName)
	assert.True(t, partnerRole.IsSystem)

	var clientRole models.Role
	err = db.Where("name = ?", "client").First(&clientRole).Error
	require.NoError(t, err)
	assert.Equal(t, "Клиент", clientRole.DisplayName)
	assert.True(t, clientRole.IsSystem)

	var userRole models.Role
	err = db.Where("name = ?", "user").First(&userRole).Error
	require.NoError(t, err)
	assert.Equal(t, "Пользователь", userRole.DisplayName)
	assert.True(t, userRole.IsSystem)

	// Повторный вызов не должен создавать дубликаты
	err = service.EnsureDefaultRoles()
	require.NoError(t, err)

	var rolesAfter []models.Role
	err = db.Find(&rolesAfter).Error
	require.NoError(t, err)
	assert.Equal(t, len(roles), len(rolesAfter))
}

func TestAxentaUserService_CreateLocalUser(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	// Создаем роль для теста
	role := models.Role{
		Name:        "test_role",
		DisplayName: "Test Role",
		IsActive:    true,
	}
	err = db.Create(&role).Error
	require.NoError(t, err)

	// Тест создания локального пользователя
	user, err := service.CreateLocalUser("testuser", "test@example.com", "hashedpassword", role.ID)
	require.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, "local", user.AxentaUserType)
	assert.False(t, user.IsAxentaUser)
	assert.Equal(t, role.ID, user.RoleID)

	// Проверяем, что пользователь сохранен в базе
	var savedUser models.User
	err = db.First(&savedUser, user.ID).Error
	require.NoError(t, err)
	assert.Equal(t, user.Username, savedUser.Username)
}

func TestAxentaUserService_GetUsersByType(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	// Создаем роль для тестов
	role := models.Role{
		Name:        "test_role",
		DisplayName: "Test Role",
		IsActive:    true,
	}
	err = db.Create(&role).Error
	require.NoError(t, err)

	// Создаем тестовых пользователей разных типов
	users := []models.User{
		{
			Username:       "partner1",
			Email:          "partner1@example.com",
			Password:       "password",
			RoleID:         &role.ID,
			AxentaUserType: "partner",
			IsAxentaUser:   true,
		},
		{
			Username:       "partner2",
			Email:          "partner2@example.com",
			Password:       "password",
			RoleID:         &role.ID,
			AxentaUserType: "partner",
			IsAxentaUser:   true,
		},
		{
			Username:       "client1",
			Email:          "client1@example.com",
			Password:       "password",
			RoleID:         &role.ID,
			AxentaUserType: "client",
			IsAxentaUser:   true,
		},
		{
			Username:       "local1",
			Email:          "local1@example.com",
			Password:       "password",
			RoleID:         &role.ID,
			AxentaUserType: "local",
			IsAxentaUser:   false,
		},
		{
			Username:       "local2",
			Email:          "local2@example.com",
			Password:       "password",
			RoleID:         &role.ID,
			AxentaUserType: "local",
			IsAxentaUser:   false,
		},
	}

	for _, user := range users {
		err := db.Create(&user).Error
		require.NoError(t, err)
	}

	// Тест получения партнеров
	partners, err := service.GetUsersByType("partner")
	require.NoError(t, err)
	assert.Len(t, partners, 2)
	for _, user := range partners {
		assert.Equal(t, "partner", user.AxentaUserType)
		assert.True(t, user.IsAxentaUser)
	}

	// Тест получения клиентов
	clients, err := service.GetUsersByType("client")
	require.NoError(t, err)
	assert.Len(t, clients, 1)
	assert.Equal(t, "client", clients[0].AxentaUserType)

	// Тест получения локальных пользователей
	localUsers, err := service.GetUsersByType("local")
	require.NoError(t, err)
	assert.Len(t, localUsers, 2)
	for _, user := range localUsers {
		assert.False(t, user.IsAxentaUser)
	}

	// Тест получения всех пользователей
	allUsers, err := service.GetUsersByType("all")
	require.NoError(t, err)
	assert.Len(t, allUsers, 5)

	// Тест неверного типа
	_, err = service.GetUsersByType("invalid")
	assert.Error(t, err)
}

func TestAxentaUserService_MapAccountTypeToUserType(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	tests := []struct {
		input    string
		expected string
	}{
		{"partner", "partner"},
		{"client", "client"},
		{"unknown", "client"}, // По умолчанию
		{"", "client"},        // По умолчанию
	}

	for _, test := range tests {
		result := service.mapAccountTypeToUserType(test.input)
		assert.Equal(t, test.expected, result, "For input: %s", test.input)
	}
}

func TestAxentaUserService_GetRoleIDForAxentaUserType(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	// Создаем роли для тестов
	roles := []models.Role{
		{Name: "partner", DisplayName: "Partner", IsActive: true},
		{Name: "client", DisplayName: "Client", IsActive: true},
		{Name: "user", DisplayName: "User", IsActive: true},
	}

	for _, role := range roles {
		err := db.Create(&role).Error
		require.NoError(t, err)
	}

	// Получаем созданные роли из базы для проверки
	var partnerRole, clientRole, userRole models.Role
	err = db.Where("name = ?", "partner").First(&partnerRole).Error
	require.NoError(t, err)
	err = db.Where("name = ?", "client").First(&clientRole).Error
	require.NoError(t, err)
	err = db.Where("name = ?", "user").First(&userRole).Error
	require.NoError(t, err)

	// Тест получения ID роли партнера
	partnerRoleID, err := service.getRoleIDForAxentaUserType("partner")
	require.NoError(t, err)
	assert.Equal(t, partnerRole.ID, partnerRoleID)

	// Тест получения ID роли клиента
	clientRoleID, err := service.getRoleIDForAxentaUserType("client")
	require.NoError(t, err)
	assert.Equal(t, clientRole.ID, clientRoleID)

	// Тест получения ID роли по умолчанию
	defaultRoleID, err := service.getRoleIDForAxentaUserType("unknown")
	require.NoError(t, err)
	assert.Equal(t, userRole.ID, defaultRoleID)

	// Тест несуществующей роли
	err = db.Where("name = ?", "partner").Delete(&models.Role{}).Error
	require.NoError(t, err)

	_, err = service.getRoleIDForAxentaUserType("partner")
	assert.Error(t, err)
}

func TestUserModel_AxentaRoleMethods(t *testing.T) {
	user := &models.User{}

	// Тест установки роли партнера
	user.SetAxentaRole("partner", "123")
	assert.Equal(t, "partner", user.AxentaUserType)
	assert.Equal(t, "123", user.AxentaUserID)
	assert.True(t, user.IsAxentaUser)
	assert.Equal(t, "axenta", user.ExternalSource)
	assert.Equal(t, "123", user.ExternalID)
	assert.True(t, user.IsPartner())
	assert.False(t, user.IsClient())
	assert.False(t, user.IsLocalUser())

	// Тест установки роли клиента
	user.SetAxentaRole("client", "456")
	assert.Equal(t, "client", user.AxentaUserType)
	assert.Equal(t, "456", user.AxentaUserID)
	assert.False(t, user.IsPartner())
	assert.True(t, user.IsClient())
	assert.False(t, user.IsLocalUser())

	// Тест очистки роли Axenta
	user.ClearAxentaRole()
	assert.Equal(t, "local", user.AxentaUserType)
	assert.Equal(t, "", user.AxentaUserID)
	assert.False(t, user.IsAxentaUser)
	assert.Equal(t, "", user.ExternalSource)
	assert.Equal(t, "", user.ExternalID)
	assert.False(t, user.IsPartner())
	assert.False(t, user.IsClient())
	assert.True(t, user.IsLocalUser())
}

func TestAxentaUserService_GetDefaultRoleID(t *testing.T) {
	db, err := setupTestDBWithModels()
	require.NoError(t, err)
	defer testutils.CleanupTestDB(db)

	service := NewAxentaUserService(db)

	// Создаем роль пользователя
	userRole := models.Role{
		Name:        "user",
		DisplayName: "User",
		IsActive:    true,
	}
	err = db.Create(&userRole).Error
	require.NoError(t, err)

	// Тест получения роли по умолчанию
	defaultRoleID := service.getDefaultRoleID()
	assert.Equal(t, userRole.ID, defaultRoleID)

	// Тест когда роль "user" не существует
	err = db.Delete(&userRole).Error
	require.NoError(t, err)

	// Создаем другую роль
	otherRole := models.Role{
		Name:        "other",
		DisplayName: "Other",
		IsActive:    true,
	}
	err = db.Create(&otherRole).Error
	require.NoError(t, err)

	defaultRoleID = service.getDefaultRoleID()
	assert.Equal(t, otherRole.ID, defaultRoleID)

	// Тест когда нет ролей вообще
	err = db.Delete(&otherRole).Error
	require.NoError(t, err)

	defaultRoleID = service.getDefaultRoleID()
	assert.Equal(t, uint(1), defaultRoleID) // Fallback ID
}
