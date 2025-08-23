package testutils

import (
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDB создает и настраивает тестовую базу данных в памяти
// Эта функция должна использоваться во всех тестах для обеспечения консистентности
func SetupTestDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// Используем упрощенные модели для тестов, совместимые с SQLite
	err = db.AutoMigrate(GetTestModels()...)
	if err != nil {
		return nil, err
	}

	// Создаем специальную таблицу для ошибок интеграции с 1С
	err = db.Exec(`CREATE TABLE IF NOT EXISTS "1c_integration_errors" (
		"id" integer PRIMARY KEY AUTOINCREMENT,
		"created_at" datetime,
		"updated_at" datetime,
		"company_id" text NOT NULL,
		"operation" text NOT NULL,
		"entity_type" text,
		"entity_id" text,
		"error_code" text,
		"error_message" text,
		"request_data" text,
		"response_data" text,
		"resolved" boolean DEFAULT false,
		"resolved_at" datetime
	)`).Error
	if err != nil {
		return nil, err
	}

	// Создаем таблицы связей many-to-many
	err = db.Exec(`CREATE TABLE IF NOT EXISTS "role_permissions" (
		"role_id" integer,
		"permission_id" integer,
		PRIMARY KEY ("role_id", "permission_id")
	)`).Error
	if err != nil {
		return nil, err
	}

	err = db.Exec(`CREATE TABLE IF NOT EXISTS "installer_locations" (
		"installer_id" integer,
		"location_id" integer,
		PRIMARY KEY ("installer_id", "location_id")
	)`).Error
	if err != nil {
		return nil, err
	}

	err = db.Exec(`CREATE TABLE IF NOT EXISTS "installation_equipment" (
		"installation_id" integer,
		"equipment_id" integer,
		PRIMARY KEY ("installation_id", "equipment_id")
	)`).Error
	if err != nil {
		return nil, err
	}

	return db, nil
}

// CleanupTestDB очищает тестовую базу данных
func CleanupTestDB(db *gorm.DB) {
	if db != nil {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
}

// CreateTestCompany создает тестовую компанию для использования в тестах
func CreateTestCompany(db *gorm.DB) *TestCompany {
	company := &TestCompany{
		Name:           "Test Company",
		DatabaseSchema: "tenant_test",
		Domain:         "test.example.com",
		AxetnaLogin:    "test_login",
		AxetnaPassword: "test_password",
		IsActive:       true,
	}

	if err := db.Create(company).Error; err != nil {
		log.Printf("Failed to create test company: %v", err)
		return nil
	}

	return company
}

// CreateTestUser создает тестового пользователя
func CreateTestUser(db *gorm.DB, companyID string) *TestUser {
	user := &TestUser{
		Username:  "testuser",
		Email:     "test@example.com",
		Password:  "hashed_password",
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		CompanyID: companyID,
	}

	if err := db.Create(user).Error; err != nil {
		log.Printf("Failed to create test user: %v", err)
		return nil
	}

	return user
}

// CreateTestRole создает тестовую роль
func CreateTestRole(db *gorm.DB) *TestRole {
	role := &TestRole{
		Name:        "test_role",
		Description: "Role for testing",
		IsSystem:    false,
	}

	if err := db.Create(role).Error; err != nil {
		log.Printf("Failed to create test role: %v", err)
		return nil
	}

	return role
}

// CreateTestPermission создает тестовое разрешение
func CreateTestPermission(db *gorm.DB) *TestPermission {
	permission := &TestPermission{
		Name:        "test.permission",
		Description: "Permission for testing",
		Resource:    "test",
		Action:      "read",
	}

	if err := db.Create(permission).Error; err != nil {
		log.Printf("Failed to create test permission: %v", err)
		return nil
	}

	return permission
}
