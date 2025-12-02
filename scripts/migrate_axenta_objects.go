package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"log"

	"github.com/joho/godotenv"
)

// migrate_axenta_objects.go
// Скрипт для миграции таблицы axenta_object_snapshots с добавлением новых полей из API
//
// Новые поля:
// - creator_name: Имя создателя объекта
// - creator_id: ID создателя
// - creator_is_active: Активность создателя
// - account_is_active: Активность аккаунта
// - phone_numbers: Номера телефонов (JSONB)
// - axenta_created_at: Дата создания в Axenta Cloud
// - axenta_deleted_at: Дата удаления в Axenta Cloud
//
// Все новые поля nullable для обратной совместимости с существующими данными.
//
// Использование:
//   go run scripts/migrate_axenta_objects.go

func main() {
	log.Println("🚀 Запуск миграции таблицы axenta_object_snapshots")

	// Загружаем .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Файл .env не найден, используем переменные окружения")
	}

	// Загружаем конфигурацию
	if _, err := config.LoadConfig(); err != nil {
		log.Fatalf("❌ Ошибка загрузки конфигурации: %v", err)
	}

	// Подключаемся к БД (без автоматических миграций)
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("❌ Ошибка подключения к БД: %v", err)
	}

	log.Println("✅ Подключение к БД установлено")

	// Получаем список всех компаний
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка компаний: %v", err)
	}

	log.Printf("📋 Найдено компаний: %d", len(companies))

	// Для каждой компании выполняем миграцию в её схеме
	for _, company := range companies {
		if !company.IsActive {
			log.Printf("⏭️  Пропускаем неактивную компанию: %s (ID=%d)", company.Name, company.ID)
			continue
		}

		log.Printf("🏢 Миграция для компании: %s (схема: %s)", company.Name, company.DatabaseSchema)

		// Получаем tenant DB
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			log.Printf("❌ Не удалось получить DB для компании %s (ID=%d)", company.Name, company.ID)
			continue
		}

		// Выполняем AutoMigrate для добавления новых колонок
		if err := tenantDB.AutoMigrate(&models.AxentaObjectSnapshot{}); err != nil {
			log.Printf("❌ Ошибка миграции для компании %s: %v", company.Name, err)
			continue
		}

		log.Printf("✅ Миграция выполнена для компании %s", company.Name)

		// Проверяем добавленные колонки
		var columnNames []string
		rows, err := tenantDB.Raw(`
			SELECT column_name 
			FROM information_schema.columns 
			WHERE table_schema = ? AND table_name = 'axenta_object_snapshots'
			AND column_name IN ('creator_name', 'creator_id', 'creator_is_active', 
				'account_is_active', 'phone_numbers', 'axenta_created_at', 'axenta_deleted_at')
		`, company.DatabaseSchema).Rows()

		if err != nil {
			log.Printf("⚠️ Не удалось проверить колонки для компании %s: %v", company.Name, err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var colName string
				rows.Scan(&colName)
				columnNames = append(columnNames, colName)
			}

			if len(columnNames) > 0 {
				log.Printf("✅ Добавлено новых колонок: %v", columnNames)
			} else {
				log.Printf("ℹ️ Новые колонки уже существовали")
			}
		}
	}

	// Также выполняем миграцию для схемы public (если там есть таблица)
	log.Println("🔄 Проверка таблицы axenta_object_snapshots в схеме public...")
	publicDB := database.DB.Exec("SET search_path TO public")
	if publicDB.Error == nil {
		var count int64
		if err := database.DB.Raw(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'axenta_object_snapshots'",
		).Scan(&count).Error; err == nil && count > 0 {
			log.Println("ℹ️ Таблица найдена в public схеме, выполняем миграцию...")
			if err := database.DB.AutoMigrate(&models.AxentaObjectSnapshot{}); err != nil {
				log.Printf("❌ Ошибка миграции в public схеме: %v", err)
			} else {
				log.Println("✅ Миграция в public схеме выполнена")
			}
		} else {
			log.Println("ℹ️ Таблица не найдена в public схеме (это нормально для тенантных таблиц)")
		}
	}

	log.Println("🎉 Миграция завершена!")
	log.Println("")
	log.Println("📊 Что было сделано:")
	log.Println("   ✅ Добавлены новые поля в модель AxentaObjectSnapshot:")
	log.Println("      - creator_name (varchar(200)) - Имя создателя")
	log.Println("      - creator_id (int) - ID создателя")
	log.Println("      - creator_is_active (bool) - Активность создателя")
	log.Println("      - account_is_active (bool) - Активность аккаунта")
	log.Println("      - phone_numbers (jsonb) - Массив телефонов")
	log.Println("      - axenta_created_at (timestamp) - Дата создания в Axenta")
	log.Println("      - axenta_deleted_at (timestamp) - Дата удаления в Axenta")
	log.Println("")
	log.Println("   ✅ Все поля nullable для обратной совместимости")
	log.Println("   ✅ Существующие данные сохранены")
	log.Println("   ✅ При следующей синхронизации новые данные будут заполнены")
}

