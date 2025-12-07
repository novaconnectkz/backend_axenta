package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"time"
)

func main() {
	log.Println("🔍 Поиск объекта с самой ранней датой создания в снимках...")

	// Загружаем конфигурацию
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Table("public.companies").Find(&companies).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка компаний: %v", err)
	}

	fmt.Printf("\n📋 Поиск объекта с самой ранней датой создания в %d компаниях...\n\n", len(companies))

	var earliestObject *models.AxentaObjectSnapshot
	var earliestDate *time.Time
	var foundInCompany *models.Company

	// Ищем объект в каждой компании
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("⚠️ Не удалось получить tenant DB для компании %s (ID=%d)\n", company.Name, company.ID)
			continue
		}

		// Сначала проверяем, есть ли вообще объекты в этой схеме
		var count int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&count)
		fmt.Printf("📊 Компания %s (схема: %s): найдено объектов: %d\n", company.Name, company.DatabaseSchema, count)

		if count == 0 {
			continue
		}

		// Проверяем, сколько объектов с axenta_created_at
		var countWithDate int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("axenta_created_at IS NOT NULL").
			Count(&countWithDate)
		fmt.Printf("   - Объектов с axenta_created_at: %d\n", countWithDate)

		// Ищем объект с самой ранней датой создания (axenta_created_at)
		var object models.AxentaObjectSnapshot
		query := tenantDB.
			Where("axenta_created_at IS NOT NULL").
			Order("axenta_created_at ASC").
			Limit(1)

		if err := query.First(&object).Error; err == nil {
			if object.AxentaCreatedAt != nil {
				// Если это первый найденный объект или дата раньше текущей самой ранней
				if earliestDate == nil || object.AxentaCreatedAt.Before(*earliestDate) {
					earliestDate = object.AxentaCreatedAt
					earliestObject = &object
					foundInCompany = &company
				}
			}
		} else {
			// Если не нашли объекты с axenta_created_at, пробуем найти по created_at
			var fallbackObject models.AxentaObjectSnapshot
			fallbackQuery := tenantDB.
				Order("created_at ASC").
				Limit(1)

			if err := fallbackQuery.First(&fallbackObject).Error; err == nil {
				// Используем created_at как fallback
				fallbackDate := fallbackObject.CreatedAt
				if earliestDate == nil || fallbackDate.Before(*earliestDate) {
					earliestDate = &fallbackDate
					earliestObject = &fallbackObject
					foundInCompany = &company
					fmt.Printf("   ✅ Найден объект по created_at: ID=%d, дата=%s\n", fallbackObject.ExternalObjectID, fallbackDate.Format("2006-01-02"))
				}
			}
		}
		fmt.Println()
	}

	if earliestObject == nil || earliestDate == nil {
		fmt.Println("❌ Объекты не найдены в снимках")
		fmt.Println("\n💡 Попробуйте проверить:")
		fmt.Println("   - Синхронизацию данных из Axenta Cloud")
		fmt.Println("   - Наличие объектов в таблице axenta_object_snapshots")
		return
	}

	// Определяем, какое поле использовалось для даты
	dateSource := "created_at (дата создания записи в БД)"
	if earliestObject.AxentaCreatedAt != nil {
		dateSource = "axenta_created_at (дата создания в Axenta Cloud)"
	}

	// Выводим результат
	fmt.Println("✅ Найден объект с самой ранней датой создания:")
	fmt.Println()
	fmt.Printf("   📦 ID объекта: %d\n", earliestObject.ExternalObjectID)
	fmt.Printf("   📅 Дата создания: %s\n", earliestDate.Format("2006-01-02 15:04:05"))
	fmt.Printf("   📅 Дата создания (только дата): %s\n", earliestDate.Format("2006-01-02"))
	fmt.Printf("   📌 Источник даты: %s\n", dateSource)
	fmt.Println()
	fmt.Printf("   📋 Дополнительная информация:\n")
	fmt.Printf("      Название: %s\n", earliestObject.ObjectName)
	if foundInCompany != nil {
		fmt.Printf("      Компания: %s (ID=%d, схема: %s)\n", foundInCompany.Name, foundInCompany.ID, foundInCompany.DatabaseSchema)
	}
	if earliestObject.UniqueID != "" {
		fmt.Printf("      Unique ID: %s\n", earliestObject.UniqueID)
	}
	if earliestObject.DeviceTypeName != "" {
		fmt.Printf("      Тип устройства: %s\n", earliestObject.DeviceTypeName)
	}
	fmt.Printf("      Активен: %v\n", earliestObject.IsActive)
	if earliestObject.CreatorName != nil {
		fmt.Printf("      Создатель: %s\n", *earliestObject.CreatorName)
	}
	if earliestObject.AxentaDeletedAt != nil {
		fmt.Printf("      Удален в Axenta: %s\n", earliestObject.AxentaDeletedAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Printf("      Последняя синхронизация: %s\n", earliestObject.LastSyncedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	fmt.Println("✅ Поиск завершен")
}
