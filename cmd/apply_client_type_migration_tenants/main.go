package main

import (
	"fmt"
	"log"
	"strings"

	"backend_axenta/config"
	"backend_axenta/database"

	"github.com/joho/godotenv"
)

func main() {
	// Загружаем .env файл если он существует
	if err := godotenv.Load(); err != nil {
		log.Printf("⚠️  Предупреждение: .env файл не найден или не может быть загружен: %v", err)
	}

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	// Подключаемся к основной БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Ошибка подключения к БД: %v", err)
	}

	fmt.Println("✅ Подключение к БД установлено")
	fmt.Println()

	// Получаем список всех tenant-схем
	var schemas []string
	rows, err := database.DB.Raw(`
		SELECT schema_name 
		FROM information_schema.schemata 
		WHERE schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast', 'pg_temp_1', 'pg_toast_temp_1', 'public')
		AND schema_name LIKE 'tenant_%'
		ORDER BY schema_name
	`).Rows()
	if err != nil {
		log.Fatalf("❌ Ошибка получения списка схем: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			log.Printf("⚠️  Ошибка чтения схемы: %v", err)
			continue
		}
		schemas = append(schemas, schema)
	}

	fmt.Printf("📋 Найдено tenant-схем: %d\n", len(schemas))
	fmt.Println()

	// Миграция SQL
	migrationSQL := `ALTER TABLE contracts ALTER COLUMN client_type TYPE VARCHAR(50);`

	// Применяем миграцию к каждой схеме
	successCount := 0
	errorCount := 0

	for _, schema := range schemas {
		fmt.Printf("🔧 Применение миграции к схеме: %s\n", schema)

		// Проверяем, существует ли таблица contracts в этой схеме
		var tableExists bool
		err := database.DB.Raw(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = ? AND table_name = 'contracts'
			)
		`, schema).Scan(&tableExists).Error

		if err != nil {
			log.Printf("⚠️  Ошибка проверки таблицы в схеме %s: %v", schema, err)
			errorCount++
			continue
		}

		if !tableExists {
			fmt.Printf("   ⏭️  Таблица contracts не найдена, пропускаем\n")
			continue
		}

		// Применяем миграцию с указанием схемы
		fullSQL := fmt.Sprintf(`SET search_path TO %s; %s`, schema, migrationSQL)

		err = database.DB.Exec(fullSQL).Error
		if err != nil {
			log.Printf("   ❌ Ошибка применения миграции: %v\n", err)
			errorCount++
		} else {
			fmt.Printf("   ✅ Миграция успешно применена\n")
			successCount++
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("📊 Итоги:\n")
	fmt.Printf("   ✅ Успешно: %d схем\n", successCount)
	fmt.Printf("   ❌ Ошибок: %d схем\n", errorCount)
	fmt.Println(strings.Repeat("=", 60))
}
