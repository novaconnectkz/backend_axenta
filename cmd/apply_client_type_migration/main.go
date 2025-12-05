package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	// Загружаем .env файл если он существует
	if err := godotenv.Load(); err != nil {
		log.Printf("⚠️  Предупреждение: .env файл не найден или не может быть загружен: %v", err)
	}

	// Получаем параметры подключения к БД
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPassword := os.Getenv("DB_PASSWORD")
	if dbPassword == "" {
		log.Fatal("❌ Ошибка: DB_PASSWORD не установлен в переменных окружения")
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "axenta_db"
	}

	// Подключаемся к БД
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("❌ Ошибка подключения к БД: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("❌ Ошибка проверки подключения к БД: %v", err)
	}

	fmt.Println("✅ Подключение к БД установлено")
	fmt.Println()

	// Читаем миграцию
	migrationSQL := `
		-- Увеличиваем размер поля client_type для поддержки значений типа "individual_entrepreneur"
		ALTER TABLE contracts
		ALTER COLUMN client_type TYPE VARCHAR(50);
	`

	fmt.Println("🔧 Применение миграции для увеличения размера client_type...")
	fmt.Println("   Увеличиваем client_type с VARCHAR(20) до VARCHAR(50)")

	// Применяем миграцию
	_, err = db.Exec(migrationSQL)
	if err != nil {
		log.Fatalf("❌ Ошибка применения миграции: %v", err)
	}

	fmt.Println("✅ Миграция успешно применена!")
	fmt.Println()
	fmt.Println("📋 Теперь поле client_type может содержать значения:")
	fmt.Println("   - organization (12 символов)")
	fmt.Println("   - individual_entrepreneur (23 символа)")
	fmt.Println("   - physical_person (15 символов)")
}
