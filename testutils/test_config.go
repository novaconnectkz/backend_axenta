package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"backend_axenta/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SetupTestDBWithTempFile создает тестовую базу данных SQLite с временным файлом
func SetupTestDBWithTempFile(t *testing.T) *gorm.DB {
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

	// Выполняем миграции с тестовыми моделями
	err = db.AutoMigrate(GetTestModels()...)
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


