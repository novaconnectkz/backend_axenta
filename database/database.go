package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"backend_axenta/models"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// CreateDatabaseIfNotExists создает базу данных, если она не существует
func CreateDatabaseIfNotExists() error {
	// Получаем настройки подключения
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "")
	dbname := getEnv("DB_NAME", "axenta_db")
	sslmode := getEnv("DB_SSLMODE", "disable")

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
	// Получаем переменные окружения для подключения к БД
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "")
	dbname := getEnv("DB_NAME", "axenta_db")
	sslmode := getEnv("DB_SSLMODE", "disable")

	// Формируем DSN (Data Source Name)
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	// Подключаемся к базе данных
	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})

	if err != nil {
		return fmt.Errorf("не удалось подключиться к базе данных: %w", err)
	}

	log.Println("✅ Успешно подключено к PostgreSQL")

	// Автомиграция моделей
	if err := autoMigrate(); err != nil {
		return fmt.Errorf("ошибка автомиграции: %w", err)
	}

	return nil
}

// getEnv получает переменную окружения или возвращает значение по умолчанию
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetDB возвращает экземпляр базы данных
func GetDB() *gorm.DB {
	return DB
}

// autoMigrate выполняет автомиграцию всех моделей
func autoMigrate() error {
	err := DB.AutoMigrate(
		&models.User{},
		&models.Object{},
		// Добавляйте новые модели здесь
	)

	if err != nil {
		return err
	}

	log.Println("✅ Автомиграция моделей выполнена успешно")
	return nil
}
