package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
)

func main() {
	log.Println("🧹 Очистка всех договоров и связанных таблиц")
	log.Println("==========================================")

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	// Получаем подключение к БД
	db := database.GetDB()

	// Убеждаемся, что мы в схеме public для работы с таблицей companies
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	} else {
		log.Println("✅ Переключились на схему public")
	}

	// Получаем список всех компаний
	var companies []models.Company
	if err := db.Find(&companies).Error; err != nil {
		log.Fatalf("❌ Не удалось получить список компаний: %v", err)
	}

	log.Printf("📋 Найдено компаний: %d", len(companies))

	totalDeleted := 0

	// Очищаем данные в каждой tenant схеме
	for _, company := range companies {
		if !company.IsActive {
			log.Printf("⏭️  Пропускаем неактивную компанию: %s (ID: %d)", company.Name, company.ID)
			continue
		}

		log.Printf("\n🏢 Очистка данных для компании: %s (ID: %d, схема: %s)", company.Name, company.ID, company.DatabaseSchema)

		// Переключаемся на схему компании
		if err := db.Exec(fmt.Sprintf("SET search_path TO %s", company.DatabaseSchema)).Error; err != nil {
			log.Printf("⚠️ Не удалось переключиться на схему %s: %v", company.DatabaseSchema, err)
			continue
		}

		// 1. Удаляем связи объектов с договорами (junction table)
		result := db.Exec("DELETE FROM contract_objects")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка удаления contract_objects: %v", result.Error)
		} else {
			log.Printf("   ✅ Удалено записей из contract_objects: %d", result.RowsAffected)
			totalDeleted += int(result.RowsAffected)
		}

		// 2. Удаляем приложения к договорам
		result = db.Exec("DELETE FROM contract_appendices")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка удаления contract_appendices: %v", result.Error)
		} else {
			log.Printf("   ✅ Удалено записей из contract_appendices: %d", result.RowsAffected)
			totalDeleted += int(result.RowsAffected)
		}

		// 3. Обнуляем contract_id в объектах (для обратной совместимости)
		result = db.Exec("UPDATE objects SET contract_id = NULL WHERE contract_id IS NOT NULL")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка обновления objects: %v", result.Error)
		} else {
			log.Printf("   ✅ Обновлено объектов (обнулен contract_id): %d", result.RowsAffected)
		}

		// 4. Удаляем историю биллинга, связанную с договорами
		result = db.Exec("DELETE FROM billing_history WHERE contract_id IS NOT NULL")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка удаления billing_history: %v", result.Error)
		} else {
			log.Printf("   ✅ Удалено записей из billing_history: %d", result.RowsAffected)
			totalDeleted += int(result.RowsAffected)
		}

		// 5. Обнуляем contract_id в счетах (не удаляем сами счета)
		result = db.Exec("UPDATE invoices SET contract_id = NULL WHERE contract_id IS NOT NULL")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка обновления invoices: %v", result.Error)
		} else {
			log.Printf("   ✅ Обновлено счетов (обнулен contract_id): %d", result.RowsAffected)
		}

		// 6. Удаляем сами договоры
		result = db.Exec("DELETE FROM contracts")
		if result.Error != nil {
			log.Printf("⚠️ Ошибка удаления contracts: %v", result.Error)
		} else {
			log.Printf("   ✅ Удалено договоров: %d", result.RowsAffected)
			totalDeleted += int(result.RowsAffected)
		}

		// Проверка: подсчет оставшихся записей
		var counts struct {
			ContractsCount            int64
			ContractObjectsCount      int64
			AppendicesCount           int64
			InvoicesWithContractCount int64
			BillingHistoryCount       int64
			ObjectsWithContractCount  int64
		}

		db.Raw(`
			SELECT 
				(SELECT COUNT(*) FROM contracts) as contracts_count,
				(SELECT COUNT(*) FROM contract_objects) as contract_objects_count,
				(SELECT COUNT(*) FROM contract_appendices) as appendices_count,
				(SELECT COUNT(*) FROM invoices WHERE contract_id IS NOT NULL) as invoices_with_contract_count,
				(SELECT COUNT(*) FROM billing_history WHERE contract_id IS NOT NULL) as billing_history_count,
				(SELECT COUNT(*) FROM objects WHERE contract_id IS NOT NULL) as objects_with_contract_count
		`).Scan(&counts)

		log.Printf("   📊 Проверка после очистки:")
		log.Printf("      - Договоры: %d", counts.ContractsCount)
		log.Printf("      - Связи объектов: %d", counts.ContractObjectsCount)
		log.Printf("      - Приложения: %d", counts.AppendicesCount)
		log.Printf("      - Счета с contract_id: %d", counts.InvoicesWithContractCount)
		log.Printf("      - История биллинга: %d", counts.BillingHistoryCount)
		log.Printf("      - Объекты с contract_id: %d", counts.ObjectsWithContractCount)
	}

	// Возвращаемся к схеме public
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось вернуться к схеме public: %v", err)
	}

	log.Printf("\n✅ Очистка завершена. Всего удалено записей: %d", totalDeleted)
	log.Println("==========================================")
}

