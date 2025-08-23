package database

import (
	"database/sql"
	"fmt"
	"log"

	"backend_axenta/config"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// CreateDatabaseIfNotExists создает базу данных, если она не существует
func CreateDatabaseIfNotExists() error {
	cfg := config.GetConfig()

	// Получаем настройки подключения
	host := cfg.Database.Host
	port := cfg.Database.Port
	user := cfg.Database.User
	password := cfg.Database.Password
	dbname := cfg.Database.Name
	sslmode := cfg.Database.SSLMode

	// Подключаемся к PostgreSQL без указания конкретной БД (к postgres по умолчанию)
	adminDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=%s",
		host, port, user, password, sslmode)

	db, err := sql.Open("postgres", adminDSN)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к PostgreSQL: %w", err)
	}
	defer db.Close()

	// Проверяем подключение
	if err := db.Ping(); err != nil {
		return fmt.Errorf("не удалось проверить подключение к PostgreSQL: %w", err)
	}

	// Проверяем, существует ли база данных
	var exists bool
	query := "SELECT EXISTS(SELECT datname FROM pg_catalog.pg_database WHERE datname = $1);"
	err = db.QueryRow(query, dbname).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка при проверке существования базы данных: %w", err)
	}

	if exists {
		log.Printf("✅ База данных '%s' уже существует", dbname)
		return nil
	}

	// Создаем базу данных
	createQuery := fmt.Sprintf("CREATE DATABASE %s;", dbname)
	_, err = db.Exec(createQuery)
	if err != nil {
		return fmt.Errorf("не удалось создать базу данных '%s': %w", dbname, err)
	}

	log.Printf("✅ База данных '%s' успешно создана", dbname)
	return nil
}

// ConnectDatabase инициализирует подключение к PostgreSQL
func ConnectDatabase() error {
	cfg := config.GetConfig()

	// Формируем DSN (Data Source Name)
	dsn := cfg.GetDatabaseDSN()

	// Подключаемся к базе данных
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		return fmt.Errorf("не удалось подключиться к базе данных: %w", err)
	}

	log.Println("✅ Успешно подключено к PostgreSQL")

	// Автомиграция моделей - отключена из-за проблем с моделью Company
	// if err := autoMigrate(); err != nil {
	// 	return fmt.Errorf("ошибка автомиграции: %w", err)
	// }
	log.Println("⚠️ Автомиграция отключена - таблицы должны быть созданы вручную")

	return nil
}

// getEnv получает переменную окружения или возвращает значение по умолчанию
// Deprecated: используйте config.GetConfig() вместо этого
func getEnv(key, defaultValue string) string {
	cfg := config.GetConfig()
	switch key {
	case "DB_HOST":
		return cfg.Database.Host
	case "DB_PORT":
		return cfg.Database.Port
	case "DB_USER":
		return cfg.Database.User
	case "DB_PASSWORD":
		return cfg.Database.Password
	case "DB_NAME":
		return cfg.Database.Name
	case "DB_SSLMODE":
		return cfg.Database.SSLMode
	default:
		// Fallback для обратной совместимости
		return defaultValue
	}
}

// GetDB возвращает экземпляр базы данных
func GetDB() *gorm.DB {
	return DB
}

// GetTenantDB возвращает базу данных для текущего tenant из контекста
func GetTenantDB(c *gin.Context) *gorm.DB {
	// Получаем tenant DB из контекста, установленного middleware
	if tenantDB, exists := c.Get("tenant_db"); exists {
		if db, ok := tenantDB.(*gorm.DB); ok {
			return db
		}
	}
	// Возвращаем основную DB как fallback
	return DB
}

// GetTenantDBByID возвращает базу данных для указанного tenant ID
func GetTenantDBByID(tenantID uint) *gorm.DB {
	// Получаем данные компании
	var company struct {
		DatabaseSchema string `gorm:"column:database_schema"`
	}

	if err := DB.Table("companies").Select("database_schema").Where("id = ?", tenantID).First(&company).Error; err != nil {
		log.Printf("Ошибка получения схемы для tenant %d: %v", tenantID, err)
		return DB
	}

	// Переключаемся на схему компании
	tenantDB := DB.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema))
	if tenantDB.Error != nil {
		log.Printf("Ошибка переключения на схему %s: %v", company.DatabaseSchema, tenantDB.Error)
		return DB
	}

	return tenantDB
}

// autoMigrate выполняет автомиграцию только глобальных моделей (не мультитенантных)
func autoMigrate() error {
	log.Println("🔄 Выполняем автомиграцию глобальных таблиц")

	// Миграции для глобальных таблиц (в схеме public)
	globalModels := []interface{}{
		&models.Company{},
		&models.IntegrationError{},
	}

	for _, model := range globalModels {
		if err := DB.AutoMigrate(model); err != nil {
			return fmt.Errorf("ошибка миграции модели %T: %v", model, err)
		}
	}

	log.Println("✅ Автомиграция глобальных таблиц завершена")
	return nil
}

// SetupTestDatabase создает тестовую базу данных в памяти
func SetupTestDatabase() error {
	// Используем временную БД в памяти для тестов
	var err error
	DB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to test database: %v", err)
	}

	// Выполняем базовые миграции
	err = DB.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Role{},
		&models.Permission{},
		&models.Integration{},
		&models.IntegrationError{},
	)
	if err != nil {
		return fmt.Errorf("failed to migrate test database: %v", err)
	}

	log.Println("✅ Тестовая база данных настроена успешно")
	return nil
}

// CleanupTestDatabase очищает тестовую базу данных
func CleanupTestDatabase() {
	if DB != nil {
		sqlDB, _ := DB.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	}
}
