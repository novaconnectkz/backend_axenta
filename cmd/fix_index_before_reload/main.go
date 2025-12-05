package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"strings"
	"time"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Дата снимка: 1 декабря 2025 года
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	fmt.Printf("🔧 Исправление индекса и очистка объектов за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	// ШАГ 1: Удаляем все объекты за эту дату
	fmt.Printf("🗑️  ШАГ 1: Удаление всех объектов за %s...\n\n", snapshotDate.Format("2006-01-02"))

	totalObjectsDeleted := 0
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		result := tenantDB.Unscoped().
			Where("DATE(last_synced_at AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Delete(&models.AxentaObjectSnapshot{})

		if result.Error == nil && result.RowsAffected > 0 {
			fmt.Printf("   🗑️ Компания %s: удалено объектов: %d\n", company.Name, result.RowsAffected)
			totalObjectsDeleted += int(result.RowsAffected)
		}
	}

	fmt.Printf("\n✅ Удалено объектов: %d\n\n", totalObjectsDeleted)

	// ШАГ 2: Удаляем старый индекс и создаем новый (если нужно)
	fmt.Printf("🔧 ШАГ 2: Исправление уникального индекса...\n\n")

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		fmt.Printf("🏢 Компания: %s\n", company.Name)

		// Удаляем старый составной индекс если существует
		var oldIndexExists bool
		tenantDB.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes 
				WHERE schemaname = ? 
				AND tablename = 'axenta_object_snapshots' 
				AND indexname = 'idx_axenta_object_admin_external'
			)
		`, company.DatabaseSchema).Scan(&oldIndexExists)

		if oldIndexExists {
			fmt.Printf("   🗑️ Удаление старого индекса idx_axenta_object_admin_external...\n")
			if err := tenantDB.Exec("DROP INDEX IF EXISTS " + company.DatabaseSchema + ".idx_axenta_object_admin_external").Error; err != nil {
				fmt.Printf("   ⚠️ Ошибка удаления старого индекса: %v\n", err)
			} else {
				fmt.Printf("   ✅ Старый индекс удален\n")
			}
		}

		// Проверяем существует ли новый индекс
		var newIndexExists bool
		tenantDB.Raw(`
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes 
				WHERE schemaname = ? 
				AND tablename = 'axenta_object_snapshots' 
				AND indexname = 'idx_axenta_object_external'
			)
		`, company.DatabaseSchema).Scan(&newIndexExists)

		if !newIndexExists {
			fmt.Printf("   🔧 Создание нового уникального индекса idx_axenta_object_external...\n")
			// Создаем индекс только если нет дубликатов
			if err := tenantDB.Exec(fmt.Sprintf(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_axenta_object_external 
				ON %s.axenta_object_snapshots(external_object_id) 
				WHERE deleted_at IS NULL
			`, company.DatabaseSchema)).Error; err != nil {
				fmt.Printf("   ⚠️ Ошибка создания индекса: %v\n", err)
				fmt.Printf("   💡 Возможно, есть дубликаты. Попробуем удалить их...\n")

				// Удаляем дубликаты, оставляя самую свежую запись
				result := tenantDB.Exec(fmt.Sprintf(`
					DELETE FROM %s.axenta_object_snapshots a
					USING %s.axenta_object_snapshots b
					WHERE a.external_object_id = b.external_object_id
					AND a.id < b.id
					AND a.deleted_at IS NULL
					AND b.deleted_at IS NULL
				`, company.DatabaseSchema, company.DatabaseSchema))

				if result.Error != nil {
					fmt.Printf("   ⚠️ Ошибка удаления дубликатов: %v\n", result.Error)
				} else {
					fmt.Printf("   ✅ Удалено дубликатов: %d\n", result.RowsAffected)

					// Пробуем создать индекс снова
					if err := tenantDB.Exec(fmt.Sprintf(`
						CREATE UNIQUE INDEX IF NOT EXISTS idx_axenta_object_external 
						ON %s.axenta_object_snapshots(external_object_id) 
						WHERE deleted_at IS NULL
					`, company.DatabaseSchema)).Error; err != nil {
						fmt.Printf("   ❌ Все еще не удается создать индекс: %v\n", err)
					} else {
						fmt.Printf("   ✅ Новый индекс создан\n")
					}
				}
			} else {
				fmt.Printf("   ✅ Новый индекс создан\n")
			}
		} else {
			fmt.Printf("   ✅ Новый индекс уже существует\n")
		}

		fmt.Printf("\n")
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("✅ Индекс исправлен. Теперь можно запускать перезагрузку объектов.\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}

