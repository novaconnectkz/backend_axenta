package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"backend_axenta/config"
	"backend_axenta/database"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDB создает тестовую базу данных SQLite
func SetupTestDB(t *testing.T) *gorm.DB {
	// Создаем временный файл для тестовой БД
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	
	// Настраиваем подключение к SQLite
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Отключаем логи в тестах
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Выполняем миграции
	err = database.AutoMigrate(db)
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

// SetupTestConfig настраивает тестовую конфигурацию
func SetupTestConfig() {
	// Устанавливаем переменные окружения для тестов
	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_PATH", ":memory:")
	os.Setenv("REDIS_ENABLED", "false")
	os.Setenv("JWT_SECRET", "test-secret-key-for-testing-only")
	os.Setenv("ENVIRONMENT", "test")
	
	// Загружаем конфигурацию
	config.LoadConfig()
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
