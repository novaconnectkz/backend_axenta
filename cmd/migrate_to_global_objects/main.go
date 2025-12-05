package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"strings"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	fmt.Printf("🔄 Миграция объектов на глобальное хранение\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	totalDuplicatesDeleted := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		fmt.Printf("🏢 Компания: %s\n", company.Name)

		// Находим дубликаты объектов (одинаковые external_object_id)
		var duplicates []struct {
			ExternalObjectID int64
			Count            int64
		}

		// Используем raw SQL для поиска дубликатов
		err := tenantDB.Raw(`
			SELECT external_object_id, COUNT(*) as count
			FROM axenta_object_snapshots
			WHERE deleted_at IS NULL
			GROUP BY external_object_id
			HAVING COUNT(*) > 1
			ORDER BY count DESC
		`).Scan(&duplicates).Error

		if err != nil {
			log.Printf("   ⚠️ Ошибка поиска дубликатов: %v\n", err)
			continue
		}

		if len(duplicates) == 0 {
			fmt.Printf("   ✅ Дубликатов не найдено\n\n")
			continue
		}

		fmt.Printf("   Найдено дубликатов: %d\n", len(duplicates))

		deletedCount := 0

		// Для каждого дубликата оставляем одну запись (самую свежую по last_synced_at)
		for _, dup := range duplicates {
			// Находим все записи с этим external_object_id
			var objects []models.AxentaObjectSnapshot
			if err := tenantDB.
				Where("external_object_id = ?", dup.ExternalObjectID).
				Order("last_synced_at DESC").
				Find(&objects).Error; err != nil {
				continue
			}

			if len(objects) <= 1 {
				continue
			}

			// Оставляем первую запись (самую свежую), удаляем остальные
			for i := 1; i < len(objects); i++ {
				if err := tenantDB.Unscoped().Delete(&objects[i]).Error; err != nil {
					log.Printf("   ⚠️ Ошибка удаления дубликата объекта %d: %v\n", dup.ExternalObjectID, err)
					continue
				}
				deletedCount++
			}
		}

		fmt.Printf("   ✅ Удалено дубликатов: %d\n\n", deletedCount)
		totalDuplicatesDeleted += deletedCount
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГО:\n")
	fmt.Printf("   - Удалено дубликатов: %d\n", totalDuplicatesDeleted)
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
