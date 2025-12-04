package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

func main() {
	log.Println("🔍 Поиск объекта с ID 181550...")

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

	objectID := int64(181550)

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Table("public.companies").Find(&companies).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка компаний: %v", err)
	}

	fmt.Printf("\n📋 Поиск объекта ID=%d в %d компаниях...\n\n", objectID, len(companies))

	found := false

	// Ищем объект в каждой компании
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Ищем в таблице objects
		var obj models.Object
		if err := tenantDB.Where("id = ?", objectID).First(&obj).Error; err == nil {
			found = true
			fmt.Printf("✅ Найден в компании: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
			fmt.Printf("   📦 Объект:\n")
			fmt.Printf("      ID: %d\n", obj.ID)
			fmt.Printf("      Название: %s\n", obj.Name)
			fmt.Printf("      IMEI: %s\n", obj.IMEI)
			fmt.Printf("      Телефон: %s\n", obj.PhoneNumber)
			fmt.Printf("      Тип: %s\n", obj.Type)
			fmt.Printf("      Статус: %s\n", obj.Status)
			fmt.Printf("      Активен: %v\n", obj.IsActive)
			fmt.Printf("      Адрес: %s\n", obj.Address)
			fmt.Printf("      Создан: %s\n", obj.CreatedAt.Format("2006-01-02 15:04:05"))
			if obj.DeletedAt.Valid {
				fmt.Printf("      Удален: %s\n", obj.DeletedAt.Time.Format("2006-01-02 15:04:05"))
			}
			fmt.Println()
		}

		// Ищем в таблице axenta_object_snapshots
		var snapshot models.AxentaObjectSnapshot
		if err := tenantDB.Where("external_object_id = ?", objectID).Order("last_synced_at DESC").First(&snapshot).Error; err == nil {
			found = true
			fmt.Printf("✅ Найден в снимках Axenta для компании: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)
			fmt.Printf("   📸 Снимок объекта:\n")
			fmt.Printf("      Axenta ID: %d\n", snapshot.ExternalObjectID)
			fmt.Printf("      Название: %s\n", snapshot.ObjectName)
			if snapshot.UniqueID != "" {
				fmt.Printf("      Unique ID: %s\n", snapshot.UniqueID)
			}
			if snapshot.PhoneNumbers != nil {
				fmt.Printf("      Телефоны: %s\n", *snapshot.PhoneNumbers)
			}
			if snapshot.DeviceTypeName != "" {
				fmt.Printf("      Тип устройства: %s\n", snapshot.DeviceTypeName)
			}
			fmt.Printf("      Активен: %v\n", snapshot.IsActive)
			if snapshot.CreatorName != nil {
				fmt.Printf("      Создатель: %s\n", *snapshot.CreatorName)
			}
			if snapshot.AxentaCreatedAt != nil {
				fmt.Printf("      Создан в Axenta: %s\n", snapshot.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
			}
			if snapshot.AxentaDeletedAt != nil {
				fmt.Printf("      Удален в Axenta: %s\n", snapshot.AxentaDeletedAt.Format("2006-01-02 15:04:05"))
			}
			fmt.Printf("      Последняя синхронизация: %s\n", snapshot.LastSyncedAt.Format("2006-01-02 15:04:05"))
			fmt.Println()
		}
	}

	if !found {
		fmt.Printf("❌ Объект с ID %d не найден ни в одной компании\n", objectID)
		fmt.Println("\n💡 Попробуйте проверить:")
		fmt.Println("   - Правильность ID объекта")
		fmt.Println("   - Синхронизацию данных из Axenta Cloud")
	}

	fmt.Println("✅ Поиск завершен")
}

