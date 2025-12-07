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
	log.Println("🔍 Проверка структуры таблицы axenta_object_snapshots...")

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

	// Проверяем уникальные индексы в таблице
	fmt.Println("🔍 Проверка уникальных индексов в таблице axenta_object_snapshots...")

	var indexes []struct {
		IndexName string
		Columns   string
	}

	// Получаем информацию об индексах из PostgreSQL
	schemaName := firstCompany.DatabaseSchema
	query := fmt.Sprintf(`
		SELECT 
			i.indexname as index_name,
			string_agg(a.attname, ', ' ORDER BY a.attnum) as columns
		FROM pg_indexes i
		JOIN pg_class c ON c.relname = i.indexname
		JOIN pg_index idx ON idx.indexrelid = c.oid
		JOIN pg_attribute a ON a.attrelid = idx.indrelid AND a.attnum = ANY(idx.indkey)
		WHERE i.schemaname = '%s' 
			AND i.tablename = 'axenta_object_snapshots'
			AND idx.indisunique = true
		GROUP BY i.indexname
		ORDER BY i.indexname
	`, schemaName)

	tenantDB.Raw(query).Scan(&indexes)

	fmt.Printf("   Найдено уникальных индексов: %d\n", len(indexes))
	for _, idx := range indexes {
		fmt.Printf("   - %s: колонки (%s)\n", idx.IndexName, idx.Columns)
	}

	// Проверяем количество объектов
	var totalObjects int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&totalObjects)
	fmt.Printf("\n📊 Всего объектов в таблице: %d\n", totalObjects)

	// Проверяем, сколько уникальных external_object_id
	var uniqueExternalIDs int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("external_object_id").
		Count(&uniqueExternalIDs)
	fmt.Printf("📊 Уникальных external_object_id: %d\n", uniqueExternalIDs)

	// Проверяем, сколько уникальных пар (admin_account_id, external_object_id)
	var uniquePairs int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Distinct("admin_account_id", "external_object_id").
		Count(&uniquePairs)
	fmt.Printf("📊 Уникальных пар (admin_account_id, external_object_id): %d\n", uniquePairs)

	// Проверяем дубликаты по external_object_id
	fmt.Println("\n🔍 Поиск дубликатов по external_object_id...")

	type DuplicateInfo struct {
		ExternalObjectID int64
		Count            int64
		AdminAccountIDs  string
	}

	var duplicates []DuplicateInfo
	tenantDB.Raw(`
		SELECT 
			external_object_id,
			COUNT(*) as count,
			string_agg(DISTINCT COALESCE(admin_account_id::text, 'NULL'), ', ') as admin_account_ids
		FROM axenta_object_snapshots
		GROUP BY external_object_id
		HAVING COUNT(*) > 1
		ORDER BY count DESC
		LIMIT 10
	`).Scan(&duplicates)

	if len(duplicates) > 0 {
		fmt.Printf("   ⚠️ Найдено дубликатов: %d\n", len(duplicates))
		fmt.Printf("   Первые 10 дубликатов:\n")
		for i, dup := range duplicates {
			fmt.Printf("   %d. Object ID %d встречается %d раз (admin_account_ids: %s)\n",
				i+1, dup.ExternalObjectID, dup.Count, dup.AdminAccountIDs)
		}
	} else {
		fmt.Printf("   ✅ Дубликатов по external_object_id не найдено\n")
	}

	// Проверяем, может быть проблема в том, что объекты с одинаковым external_object_id
	// но разными admin_account_id не могут быть сохранены из-за уникального индекса
	fmt.Println("\n💡 Анализ проблемы:")

	if uniqueExternalIDs < totalObjects {
		fmt.Printf("   ❌ Проблема: уникальных external_object_id (%d) меньше чем записей (%d)\n",
			uniqueExternalIDs, totalObjects)
		fmt.Printf("   Это означает, что есть дубликаты, что не должно быть при уникальном индексе\n")
	} else if uniqueExternalIDs == totalObjects {
		fmt.Printf("   ✅ Все external_object_id уникальны\n")
	}

	if uniquePairs < totalObjects {
		fmt.Printf("   ⚠️ Уникальных пар (admin_account_id, external_object_id) меньше чем записей\n")
		fmt.Printf("   Это может указывать на проблему с логикой сохранения\n")
	}

	// Проверяем, сколько объектов было загружено из API
	fmt.Println("\n📊 Сравнение с данными из API:")
	fmt.Printf("   - Ожидалось из API: 10934 объектов\n")
	fmt.Printf("   - Уникальных external_object_id в БД: %d\n", uniqueExternalIDs)
	fmt.Printf("   - Всего записей в БД: %d\n", totalObjects)

	missingUnique := 10934 - int(uniqueExternalIDs)
	if missingUnique > 0 {
		fmt.Printf("   ❌ Не хватает уникальных объектов: %d\n", missingUnique)
		fmt.Printf("   💡 Возможные причины:\n")
		fmt.Printf("      1. Объекты не были загружены из-за ошибок API\n")
		fmt.Printf("      2. Объекты были отфильтрованы при сохранении\n")
		fmt.Printf("      3. Объекты были удалены при очистке\n")
		fmt.Printf("      4. Проблема с уникальным индексом - дубликаты не сохраняются\n")
	} else if missingUnique == 0 {
		fmt.Printf("   ✅ Все уникальные объекты загружены!\n")
		if totalObjects > uniqueExternalIDs {
			fmt.Printf("   ℹ️ В БД больше записей (%d) чем уникальных объектов (%d)\n",
				totalObjects, uniqueExternalIDs)
			fmt.Printf("   Это может быть из-за дубликатов или разных admin_account_id\n")
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Проверка завершена")
	fmt.Println(strings.Repeat("=", 60))
}
