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
	log.Println("🔍 Проверка количества объектов в БД...")

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

	// Общее количество записей в БД
	var totalRecords int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&totalRecords)
	fmt.Printf("📊 Всего записей в таблице: %d\n", totalRecords)

	// Уникальных external_object_id
	var uniqueExternalIDs int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("external_object_id").
		Count(&uniqueExternalIDs)
	fmt.Printf("📊 Уникальных external_object_id: %d\n", uniqueExternalIDs)

	// Ожидаемое количество из API
	expectedFromAPI := 10934
	fmt.Printf("📊 Ожидалось из API: %d объектов\n\n", expectedFromAPI)

	// Анализ
	fmt.Println("🔍 Анализ:")
	if uniqueExternalIDs < int64(expectedFromAPI) {
		missing := expectedFromAPI - int(uniqueExternalIDs)
		fmt.Printf("   ❌ Не хватает уникальных объектов: %d\n", missing)
		fmt.Printf("   💡 Возможные причины:\n")
		fmt.Printf("      1. В API есть дубликаты объектов с одинаковым external_object_id\n")
		fmt.Printf("      2. Некоторые объекты не были загружены из-за ошибок\n")
		fmt.Printf("      3. Объекты были удалены при очистке\n")
	} else if uniqueExternalIDs == int64(expectedFromAPI) {
		fmt.Printf("   ✅ Все уникальные объекты загружены!\n")
		if totalRecords < uniqueExternalIDs {
			fmt.Printf("   ⚠️ Но записей в БД (%d) меньше чем уникальных объектов (%d)\n",
				totalRecords, uniqueExternalIDs)
			fmt.Printf("   Это странно и указывает на проблему\n")
		} else if totalRecords > uniqueExternalIDs {
			fmt.Printf("   ℹ️ Записей больше чем уникальных объектов - возможны дубликаты\n")
		}
	} else {
		fmt.Printf("   ⚠️ Уникальных объектов больше чем ожидалось!\n")
	}

	// Проверяем, есть ли дубликаты по external_object_id
	fmt.Println("\n🔍 Проверка дубликатов:")
	var duplicateCount int64
	tenantDB.Raw(`
		SELECT COUNT(*) 
		FROM (
			SELECT external_object_id, COUNT(*) as cnt
			FROM axenta_object_snapshots
			GROUP BY external_object_id
			HAVING COUNT(*) > 1
		) as duplicates
	`).Scan(&duplicateCount)

	if duplicateCount > 0 {
		fmt.Printf("   ⚠️ Найдено external_object_id с дубликатами: %d\n", duplicateCount)
	} else {
		fmt.Printf("   ✅ Дубликатов по external_object_id нет\n")
	}

	// Проверяем распределение по admin_account_id
	fmt.Println("\n🔍 Распределение по admin_account_id:")
	type AdminCount struct {
		AdminAccountID *uint
		Count          int64
	}
	var adminCounts []AdminCount
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Select("admin_account_id, COUNT(*) as count").
		Group("admin_account_id").
		Order("count DESC").
		Scan(&adminCounts)

	for i, ac := range adminCounts {
		if i >= 5 {
			break
		}
		if ac.AdminAccountID == nil {
			fmt.Printf("   %d. admin_account_id = NULL: %d объектов\n", i+1, ac.Count)
		} else {
			fmt.Printf("   %d. admin_account_id = %d: %d объектов\n", i+1, *ac.AdminAccountID, ac.Count)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Проверка завершена")
	fmt.Println(strings.Repeat("=", 60))
}
