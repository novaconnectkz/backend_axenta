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
	log.Println("🔍 Проверка загруженных объектов в БД...")

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

	// Общая статистика по объектам
	fmt.Println("📊 Общая статистика по объектам:")
	var totalObjects int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&totalObjects)
	fmt.Printf("   - Всего объектов в БД: %d\n", totalObjects)

	var activeObjects int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Where("is_active = ?", true).Count(&activeObjects)
	fmt.Printf("   - Активных объектов: %d\n", activeObjects)

	var objectsWithDate int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL").
		Count(&objectsWithDate)
	fmt.Printf("   - Объектов с датой создания: %d\n\n", objectsWithDate)

	// Проверяем распределение объектов по account_external_id
	fmt.Println("📊 Распределение объектов по account_external_id:")
	
	type AccountStats struct {
		AccountExternalID int64
		TotalObjects      int64
		ActiveObjects     int64
	}
	
	var accountStats []AccountStats
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Select("account_external_id, COUNT(*) as total_objects, SUM(CASE WHEN is_active THEN 1 ELSE 0 END) as active_objects").
		Group("account_external_id").
		Order("total_objects DESC").
		Limit(20).
		Scan(&accountStats)

	fmt.Printf("   Топ-20 аккаунтов по количеству объектов:\n")
	for i, stat := range accountStats {
		fmt.Printf("   %2d. Account ID %d: всего=%d, активных=%d\n", 
			i+1, stat.AccountExternalID, stat.TotalObjects, stat.ActiveObjects)
	}

	// Проверяем, есть ли объекты с account_external_id = 186
	fmt.Println("\n🔍 Проверка объектов для account_external_id = 186:")
	var objects186 int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("account_external_id = ?", 186).
		Count(&objects186)
	fmt.Printf("   - Всего объектов с account_external_id = 186: %d\n", objects186)

	var activeObjects186 int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("account_external_id = ? AND is_active = ?", 186, true).
		Count(&activeObjects186)
	fmt.Printf("   - Активных объектов с account_external_id = 186: %d\n", activeObjects186)

	// Проверяем уникальные account_external_id
	var uniqueAccounts int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("account_external_id").
		Count(&uniqueAccounts)
	fmt.Printf("\n📊 Всего уникальных account_external_id: %d\n", uniqueAccounts)

	// Получаем список всех уникальных account_external_id
	var accountIDs []int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("account_external_id").
		Pluck("account_external_id", &accountIDs)

	fmt.Printf("\n📋 Список всех account_external_id (первые 50):\n")
	for i, id := range accountIDs {
		if i >= 50 {
			fmt.Printf("   ... и еще %d аккаунтов\n", len(accountIDs)-50)
			break
		}
		var count int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("account_external_id = ?", id).
			Count(&count)
		fmt.Printf("   - Account ID %d: %d объектов\n", id, count)
	}

	// Проверяем, какие аккаунты есть в axenta_account_snapshots
	fmt.Println("\n📊 Аккаунты в axenta_account_snapshots:")
	var totalAccounts int64
	tenantDB.Model(&models.AxentaAccountSnapshot{}).Count(&totalAccounts)
	fmt.Printf("   - Всего аккаунтов: %d\n", totalAccounts)

	// Проверяем, есть ли аккаунт с ID 186
	var account186 models.AxentaAccountSnapshot
	if err := tenantDB.Where("external_account_id = ?", 186).First(&account186).Error; err == nil {
		fmt.Printf("   ✅ Аккаунт 186 найден: %s (тип: %s, активен: %v)\n", 
			account186.AccountName, account186.AccountType, account186.IsActive)
	} else {
		fmt.Printf("   ❌ Аккаунт 186 не найден в axenta_account_snapshots\n")
	}

	// Проверяем объекты партнеров (обычно это аккаунты с определенными типами или иерархией)
	fmt.Println("\n🔍 Анализ объектов партнеров:")
	
	// Ищем аккаунты, которые могут быть партнерами
	var partnerAccounts []models.AxentaAccountSnapshot
	tenantDB.Where("account_type LIKE ? OR account_type LIKE ?", "%partner%", "%партнер%").
		Find(&partnerAccounts)
	
	fmt.Printf("   - Найдено аккаунтов с типом 'partner': %d\n", len(partnerAccounts))
	
	// Подсчитываем объекты для партнерских аккаунтов
	if len(partnerAccounts) > 0 {
		partnerAccountIDs := make([]int64, len(partnerAccounts))
		for i, acc := range partnerAccounts {
			partnerAccountIDs[i] = acc.ExternalAccountID
		}
		
		var partnerObjects int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("account_external_id IN ?", partnerAccountIDs).
			Count(&partnerObjects)
		fmt.Printf("   - Объектов партнерских аккаунтов: %d\n", partnerObjects)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Проверка завершена")
	fmt.Println(strings.Repeat("=", 60))
}
