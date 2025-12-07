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
	log.Println("🔍 Анализ: почему загружено 9164 вместо 10934 объектов...")

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

	// Статистика из API
	apiTotalObjects := 10934
	fmt.Printf("📊 Ожидаемое количество объектов из API: %d\n", apiTotalObjects)

	// Статистика в БД
	var dbTotalObjects int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&dbTotalObjects)
	fmt.Printf("📊 Фактическое количество объектов в БД: %d\n", dbTotalObjects)

	missingCount := apiTotalObjects - int(dbTotalObjects)
	fmt.Printf("❌ Не хватает объектов: %d\n\n", missingCount)

	// Проверяем уникальность объектов по external_object_id
	fmt.Println("🔍 Проверка уникальности объектов...")

	type UniqueCheck struct {
		ExternalObjectID int64
		Count            int64
	}

	var duplicates []UniqueCheck
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Select("external_object_id, COUNT(*) as count").
		Group("external_object_id").
		Having("COUNT(*) > 1").
		Scan(&duplicates)

	fmt.Printf("   - Дубликатов по external_object_id: %d\n", len(duplicates))
	if len(duplicates) > 0 {
		fmt.Printf("   ⚠️ Найдены дубликаты! Это может указывать на проблему с уникальным индексом\n")
		for i, dup := range duplicates {
			if i < 5 {
				fmt.Printf("      %d. Object ID %d встречается %d раз\n", i+1, dup.ExternalObjectID, dup.Count)
			}
		}
		if len(duplicates) > 5 {
			fmt.Printf("      ... и еще %d дубликатов\n", len(duplicates)-5)
		}
	}

	// Проверяем уникальность по admin_account_id + external_object_id
	fmt.Println("\n🔍 Проверка уникальности по admin_account_id + external_object_id...")

	var uniquePairs int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("admin_account_id", "external_object_id").
		Count(&uniquePairs)
	fmt.Printf("   - Уникальных пар (admin_account_id, external_object_id): %d\n", uniquePairs)

	// Проверяем, сколько объектов с NULL admin_account_id
	var nullAdminCount int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("admin_account_id IS NULL").
		Count(&nullAdminCount)
	fmt.Printf("   - Объектов с NULL admin_account_id: %d\n", nullAdminCount)

	// Проверяем распределение по admin_account_id
	fmt.Println("\n🔍 Распределение объектов по admin_account_id:")

	type AdminStats struct {
		AdminAccountID *uint
		Count          int64
	}

	var adminStats []AdminStats
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Select("admin_account_id, COUNT(*) as count").
		Group("admin_account_id").
		Order("count DESC").
		Scan(&adminStats)

	for i, stat := range adminStats {
		if i >= 10 {
			break
		}
		if stat.AdminAccountID == nil {
			fmt.Printf("   %d. admin_account_id = NULL: %d объектов\n", i+1, stat.Count)
		} else {
			fmt.Printf("   %d. admin_account_id = %d: %d объектов\n", i+1, *stat.AdminAccountID, stat.Count)
		}
	}

	// Проверяем, есть ли объекты, которые были удалены (soft delete)
	var deletedObjects int64
	tenantDB.Unscoped().Model(&models.AxentaObjectSnapshot{}).
		Where("deleted_at IS NOT NULL").
		Count(&deletedObjects)
	fmt.Printf("\n   - Мягко удаленных объектов (deleted_at IS NOT NULL): %d\n", deletedObjects)

	// Проверяем последнюю синхронизацию
	fmt.Println("\n🔍 Информация о последней синхронизации:")

	var latestSync struct {
		LastSyncedAt string
		Count        int64
	}
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at) as last_synced_at, COUNT(*) as count").
		Scan(&latestSync)

	fmt.Printf("   - Последняя синхронизация: %s\n", latestSync.LastSyncedAt)
	fmt.Printf("   - Объектов с этой датой синхронизации: %d\n", latestSync.Count)

	// Проверяем, может быть некоторые объекты не сохранились из-за ошибок
	// или были отфильтрованы
	fmt.Println("\n💡 Возможные причины расхождения:")
	fmt.Println("   1. Объекты с одинаковым external_object_id обновляются, а не создаются заново")
	fmt.Println("   2. Некоторые объекты могли быть удалены при очистке устаревших записей")
	fmt.Println("   3. Возможны ошибки при сохранении некоторых объектов")
	fmt.Println("   4. API может возвращать дубликаты объектов")
	fmt.Println("   5. Объекты могут фильтроваться по каким-то критериям")

	// Проверяем, сколько уникальных external_object_id
	var uniqueExternalIDs int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("external_object_id").
		Count(&uniqueExternalIDs)
	fmt.Printf("\n📊 Уникальных external_object_id в БД: %d\n", uniqueExternalIDs)
	fmt.Printf("📊 Всего записей в БД: %d\n", dbTotalObjects)

	if uniqueExternalIDs < int64(apiTotalObjects) {
		fmt.Printf("❌ В БД меньше уникальных объектов (%d) чем в API (%d)\n", uniqueExternalIDs, apiTotalObjects)
		fmt.Printf("   Разница: %d объектов не загружены\n", apiTotalObjects-int(uniqueExternalIDs))
	} else if uniqueExternalIDs == int64(apiTotalObjects) {
		fmt.Printf("✅ Количество уникальных объектов совпадает с API!\n")
		fmt.Printf("   Разница в общем количестве (%d) может быть из-за дубликатов в API\n",
			apiTotalObjects-int(dbTotalObjects))
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Анализ завершен")
	fmt.Println(strings.Repeat("=", 60))
}
