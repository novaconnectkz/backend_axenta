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
	log.Println("🔍 Анализ: почему 1770 объектов не попадают в подсчет на дату 07/12/2025...")

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

	// Получаем первую активную компанию
	var firstCompany models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Order("id ASC").First(&firstCompany).Error; err != nil {
		log.Fatalf("❌ Не найдено активных компаний: %v", err)
	}

	fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n\n", firstCompany.Name, firstCompany.ID, firstCompany.DatabaseSchema)

	// Получаем tenant DB
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Fatalf("❌ Не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Дата для проверки
	checkDate, _ := time.Parse("2006-01-02", "2025-12-07")
	dayEnd := time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(), 23, 59, 59, 0, time.UTC)

	fmt.Printf("📅 Дата проверки: %s\n", checkDate.Format("2006-01-02"))
	fmt.Printf("📅 Конец дня: %s\n\n", dayEnd.Format("2006-01-02 15:04:05"))

	// Общее количество объектов в БД
	var totalInDB int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&totalInDB)
	fmt.Printf("📊 Всего объектов в БД: %d\n", totalInDB)

	// Объекты, которые попадают в подсчет на дату
	var countOnDate int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("(axenta_created_at IS NULL OR axenta_created_at <= ?) AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?)",
			dayEnd, dayEnd).
		Count(&countOnDate)
	fmt.Printf("📊 Объектов на дату %s: %d\n\n", checkDate.Format("2006-01-02"), countOnDate)

	missingCount := totalInDB - countOnDate
	fmt.Printf("❌ Объектов не попадает в подсчет: %d\n\n", missingCount)

	// Анализируем причины
	fmt.Println("🔍 Анализ причин:")

	// 1. Объекты без axenta_created_at
	var noCreatedAt int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NULL").
		Count(&noCreatedAt)
	fmt.Printf("   1. Объектов без axenta_created_at: %d\n", noCreatedAt)

	// 2. Объекты, созданные после даты
	var createdAfter int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL AND axenta_created_at > ?", dayEnd).
		Count(&createdAfter)
	fmt.Printf("   2. Объектов, созданных после %s: %d\n", checkDate.Format("2006-01-02"), createdAfter)

	// 3. Объекты, удаленные до или в эту дату
	var deletedBeforeOrOnDate int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_deleted_at IS NOT NULL AND axenta_deleted_at <= ?", dayEnd).
		Count(&deletedBeforeOrOnDate)
	fmt.Printf("   3. Объектов, удаленных до или в %s: %d\n", checkDate.Format("2006-01-02"), deletedBeforeOrOnDate)

	// 4. Объекты, которые должны попадать, но не попадают (проверка логики)
	fmt.Println("\n🔍 Детальная проверка объектов, которые не попадают в подсчет:")

	// Объекты без axenta_created_at, но с axenta_deleted_at после даты
	var noCreatedButNotDeleted int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NULL AND (axenta_deleted_at IS NULL OR axenta_deleted_at > ?)", dayEnd).
		Count(&noCreatedButNotDeleted)
	fmt.Printf("   - Без axenta_created_at, но не удалены на дату: %d (должны попадать в подсчет)\n", noCreatedButNotDeleted)

	// Объекты, созданные после даты
	if createdAfter > 0 {
		fmt.Printf("\n   📋 Примеры объектов, созданных после %s:\n", checkDate.Format("2006-01-02"))
		var examples []models.AxentaObjectSnapshot
		tenantDB.Where("axenta_created_at IS NOT NULL AND axenta_created_at > ?", dayEnd).
			Order("axenta_created_at ASC").
			Limit(5).
			Find(&examples)
		for i, obj := range examples {
			fmt.Printf("   %d. ID=%d, Name=%s, CreatedAt=%s\n",
				i+1, obj.ExternalObjectID, obj.ObjectName, obj.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
		}
	}

	// Объекты, удаленные до даты
	if deletedBeforeOrOnDate > 0 {
		fmt.Printf("\n   📋 Примеры объектов, удаленных до или в %s:\n", checkDate.Format("2006-01-02"))
		var examples []models.AxentaObjectSnapshot
		tenantDB.Where("axenta_deleted_at IS NOT NULL AND axenta_deleted_at <= ?", dayEnd).
			Order("axenta_deleted_at DESC").
			Limit(5).
			Find(&examples)
		for i, obj := range examples {
			fmt.Printf("   %d. ID=%d, Name=%s, CreatedAt=%s, DeletedAt=%s\n",
				i+1, obj.ExternalObjectID, obj.ObjectName,
				obj.AxentaCreatedAt.Format("2006-01-02 15:04:05"),
				obj.AxentaDeletedAt.Format("2006-01-02 15:04:05"))
		}
	}

	// Проверяем логику подсчета
	fmt.Println("\n💡 Объяснение логики подсчета:")
	fmt.Printf("   Объект попадает в подсчет на дату %s, если:\n", checkDate.Format("2006-01-02"))
	fmt.Printf("   1. axenta_created_at IS NULL ИЛИ axenta_created_at <= %s\n", dayEnd.Format("2006-01-02 15:04:05"))
	fmt.Printf("   2. И axenta_deleted_at IS NULL ИЛИ axenta_deleted_at > %s\n", dayEnd.Format("2006-01-02 15:04:05"))
	fmt.Printf("\n   Это означает, что объект должен существовать на эту дату:\n")
	fmt.Printf("   - Создан до или в этот день\n")
	fmt.Printf("   - Не удален или удален после этого дня\n")

	// Итоговая статистика
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 Итоговая статистика:")
	fmt.Printf("   - Всего объектов в БД: %d\n", totalInDB)
	fmt.Printf("   - Объектов на дату %s: %d\n", checkDate.Format("2006-01-02"), countOnDate)
	fmt.Printf("   - Не попадает в подсчет: %d\n", missingCount)
	fmt.Printf("      • Без axenta_created_at: %d\n", noCreatedAt)
	fmt.Printf("      • Созданы после даты: %d\n", createdAfter)
	fmt.Printf("      • Удалены до или в дату: %d\n", deletedBeforeOrOnDate)
	fmt.Println(strings.Repeat("=", 60))
}
