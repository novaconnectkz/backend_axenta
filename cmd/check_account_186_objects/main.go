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
	log.Println("🔍 Проверка объектов для партнера 186 и его клиентов...")

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

	// Проверяем аккаунт 186 в axenta_account_snapshots
	fmt.Println("🔍 Поиск аккаунта 186 в axenta_account_snapshots...")
	var account186 models.AxentaAccountSnapshot
	
	// Пробуем найти по external_account_id = 186
	if err := tenantDB.Where("external_account_id = ?", 186).First(&account186).Error; err != nil {
		fmt.Printf("   ❌ Аккаунт с external_account_id = 186 не найден\n")
		
		// Ищем аккаунты, в иерархии которых упоминается 186
		fmt.Println("\n🔍 Поиск аккаунтов, связанных с 186 через иерархию...")
		var relatedAccounts []models.AxentaAccountSnapshot
		tenantDB.Where("hierarchy LIKE ? OR hierarchy LIKE ? OR hierarchy LIKE ?", 
			"%/186/%",  // формат: /.../186/...
			"%/186",    // формат: /.../186
			"/186/%").  // формат: /186/...
			Find(&relatedAccounts)
		
		fmt.Printf("   Найдено аккаунтов с упоминанием 186 в иерархии: %d\n", len(relatedAccounts))
		
		if len(relatedAccounts) > 0 {
			fmt.Println("\n   Первые 10 связанных аккаунтов:")
			for i, acc := range relatedAccounts {
				if i >= 10 {
					break
				}
				fmt.Printf("   %d. ID=%d, Name=%s, Hierarchy=%s\n", 
					i+1, acc.ExternalAccountID, acc.AccountName, acc.Hierarchy)
			}
		}
	} else {
		fmt.Printf("   ✅ Аккаунт 186 найден:\n")
		fmt.Printf("      - ID: %d\n", account186.ExternalAccountID)
		fmt.Printf("      - Название: %s\n", account186.AccountName)
		fmt.Printf("      - Тип: %s\n", account186.AccountType)
		fmt.Printf("      - Иерархия: %s\n", account186.Hierarchy)
		fmt.Printf("      - Активен: %v\n", account186.IsActive)
		fmt.Printf("      - Объектов (по данным аккаунта): всего=%d, активных=%d\n", 
			account186.ObjectsTotal, account186.ObjectsActive)
	}

	// Ищем объекты, которые могут принадлежать партнеру 186 или его клиентам
	fmt.Println("\n📊 Поиск объектов, связанных с партнером 186...")
	
	// Вариант 1: Объекты с account_external_id = 186
	var directObjects int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("account_external_id = ?", 186).
		Count(&directObjects)
	fmt.Printf("   - Объекты с account_external_id = 186: %d\n", directObjects)

	// Вариант 2: Ищем аккаунты, которые являются дочерними для 186
	fmt.Println("\n🔍 Поиск дочерних аккаунтов партнера 186...")
	var childAccounts []models.AxentaAccountSnapshot
	
	// Ищем аккаунты, у которых в иерархии есть /186/ или начинается с /186/
	tenantDB.Where("(hierarchy LIKE ? OR hierarchy LIKE ? OR hierarchy LIKE ? OR external_account_id = ?)",
		"%/186/%",      // формат: /.../186/...
		"/186/%",       // формат: /186/...
		"%/186",        // формат: /.../186
		186).           // сам аккаунт 186
		Find(&childAccounts)
	
	fmt.Printf("   Найдено дочерних аккаунтов (включая сам 186): %d\n", len(childAccounts))
	
	if len(childAccounts) > 0 {
		// Собираем ID всех дочерних аккаунтов
		childAccountIDs := make([]int64, len(childAccounts))
		for i, acc := range childAccounts {
			childAccountIDs[i] = acc.ExternalAccountID
			if i < 10 {
				fmt.Printf("   %d. Account ID %d: %s (hierarchy: %s)\n", 
					i+1, acc.ExternalAccountID, acc.AccountName, acc.Hierarchy)
			}
		}
		if len(childAccounts) > 10 {
			fmt.Printf("   ... и еще %d аккаунтов\n", len(childAccounts)-10)
		}
		
		// Подсчитываем объекты для всех дочерних аккаунтов
		var childObjects int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("account_external_id IN ?", childAccountIDs).
			Count(&childObjects)
		
		var activeChildObjects int64
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("account_external_id IN ? AND is_active = ?", childAccountIDs, true).
			Count(&activeChildObjects)
		
		fmt.Printf("\n   📊 Объекты дочерних аккаунтов партнера 186:\n")
		fmt.Printf("      - Всего объектов: %d\n", childObjects)
		fmt.Printf("      - Активных объектов: %d\n", activeChildObjects)
		
		// Распределение по аккаунтам
		type AccountObjectCount struct {
			AccountExternalID int64
			AccountName       string
			TotalObjects      int64
			ActiveObjects     int64
		}
		
		var accountCounts []AccountObjectCount
		tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Select("account_external_id, account_name, COUNT(*) as total_objects, SUM(CASE WHEN is_active THEN 1 ELSE 0 END) as active_objects").
			Where("account_external_id IN ?", childAccountIDs).
			Group("account_external_id, account_name").
			Order("total_objects DESC").
			Limit(20).
			Scan(&accountCounts)
		
		fmt.Printf("\n   Топ-20 дочерних аккаунтов по количеству объектов:\n")
		for i, count := range accountCounts {
			fmt.Printf("   %2d. Account ID %d (%s): всего=%d, активных=%d\n", 
				i+1, count.AccountExternalID, count.AccountName, count.TotalObjects, count.ActiveObjects)
		}
	}

	// Проверяем, есть ли объекты партнеров вообще
	fmt.Println("\n📊 Общая статистика по объектам партнеров:")
	var partnerObjectsTotal int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Joins("JOIN axenta_account_snapshots ON axenta_object_snapshots.account_external_id = axenta_account_snapshots.external_account_id").
		Where("axenta_account_snapshots.account_type LIKE ? OR axenta_account_snapshots.account_type LIKE ?", 
			"%partner%", "%партнер%").
		Count(&partnerObjectsTotal)
	fmt.Printf("   - Всего объектов партнерских аккаунтов: %d\n", partnerObjectsTotal)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Проверка завершена")
	fmt.Println(strings.Repeat("=", 60))
}
