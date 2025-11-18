package main

import (
	"log"
	"os"

	"backend_axenta/config"
	"backend_axenta/database"
)

func main() {
	log.Println("🚀 Применение миграции для колонок contracts.start_date и contracts.end_date")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Ошибка подключения к базе данных: %v", err)
	}

	db := database.DB
	if db == nil {
		log.Fatal("❌ База данных не инициализирована")
	}

	log.Println("✅ Подключение к базе данных установлено")

	// Получаем список всех tenant-схем
	var schemas []string
	query := "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'tenant_%' ORDER BY schema_name"
	if err := db.Raw(query).Scan(&schemas).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка схем: %v", err)
	}

	if len(schemas) == 0 {
		log.Println("⚠️  Не найдено tenant-схем")
		os.Exit(0)
	}

	log.Printf("✅ Найдено схем: %d\n\n", len(schemas))

	successCount := 0
	errorCount := 0

	// Применяем миграцию для каждой схемы
	for _, schema := range schemas {
		log.Printf("🔄 Обработка схемы: %s", schema)

		// Переключаемся на схему
		if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
			log.Printf("  ❌ Ошибка переключения на схему: %v\n", err)
			errorCount++
			continue
		}

		// Проверяем существование таблицы contracts
		var exists bool
		checkQuery := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = ? AND table_name = 'contracts'
		)`
		if err := db.Raw(checkQuery, schema).Scan(&exists).Error; err != nil {
			log.Printf("  ❌ Ошибка проверки таблицы: %v\n", err)
			errorCount++
			continue
		}

		if !exists {
			log.Println("  ⏭️  Таблица contracts не существует, пропускаем")
			continue
		}

		// Получаем текущее состояние колонок
		type ColumnInfo struct {
			ColumnName string
			IsNullable string
		}
		var columns []ColumnInfo
		columnQuery := `SELECT column_name, is_nullable 
			FROM information_schema.columns 
			WHERE table_schema = ? AND table_name = 'contracts' 
			AND column_name IN ('start_date', 'end_date') 
			ORDER BY column_name`
		if err := db.Raw(columnQuery, schema).Scan(&columns).Error; err != nil {
			log.Printf("  ❌ Ошибка получения информации о колонках: %v\n", err)
			errorCount++
			continue
		}

		log.Println("  📋 Текущее состояние колонок:")
		needsMigration := false
		for _, col := range columns {
			nullable := "YES"
			if col.IsNullable == "NO" {
				nullable = "NO"
				needsMigration = true
			}
			log.Printf("    %s: nullable=%s", col.ColumnName, nullable)
		}

		if !needsMigration {
			log.Println("  ✅ Колонки уже nullable, миграция не требуется")
			successCount++
			continue
		}

		// Применяем миграцию
		log.Println("  🔄 Применение миграции...")
		
		// Изменяем start_date на nullable
		if err := db.Exec("ALTER TABLE contracts ALTER COLUMN start_date DROP NOT NULL").Error; err != nil {
			log.Printf("  ❌ Ошибка изменения start_date: %v\n", err)
			errorCount++
			continue
		}

		// Изменяем end_date на nullable
		if err := db.Exec("ALTER TABLE contracts ALTER COLUMN end_date DROP NOT NULL").Error; err != nil {
			log.Printf("  ❌ Ошибка изменения end_date: %v\n", err)
			errorCount++
			continue
		}

		log.Println("  ✅ Миграция применена успешно")
		successCount++
		log.Println()
	}

	// Возвращаемся к схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️  Не удалось вернуться к схеме public: %v", err)
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("✅ Миграция завершена")
	log.Printf("📊 Обработано схем: %d\n", len(schemas))
	log.Printf("✅ Успешно: %d\n", successCount)
	if errorCount > 0 {
		log.Printf("❌ Ошибок: %d\n", errorCount)
	}
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if errorCount > 0 {
		os.Exit(1)
	}
}

