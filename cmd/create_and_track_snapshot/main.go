package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
	"time"
)

func main() {
	log.Println("📸 Создание нового снимка и отслеживание записи в БД...")

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
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		log.Fatalf("❌ Ошибка получения списка компаний: %v", err)
	}

	if len(companies) == 0 {
		log.Fatalf("❌ Не найдено активных компаний")
	}

	fmt.Printf("\n📋 Найдено активных компаний: %d\n\n", len(companies))

	// Используем первую активную компанию
	firstCompany := companies[0]
	fmt.Printf("🏢 Используем компанию: %s (ID=%d, схема: %s)\n\n", firstCompany.Name, firstCompany.ID, firstCompany.DatabaseSchema)

	// Получаем tenant DB
	tenantDB := database.GetTenantDBByID(firstCompany.ID)
	if tenantDB == nil {
		log.Fatalf("❌ Не удалось получить tenant DB для компании %d", firstCompany.ID)
	}

	// Проверяем настройки снимков и получаем токен
	var snapshotSettings models.SnapshotSettings
	if err := tenantDB.Where("company_id = ? AND is_active = ?", 1, true).First(&snapshotSettings).Error; err != nil {
		log.Fatalf("❌ Токен Axenta не настроен: %v", err)
	}

	if snapshotSettings.AxentaToken == "" {
		log.Fatalf("❌ Токен Axenta не установлен")
	}

	fmt.Printf("✅ Токен найден (длина: %d символов)\n\n", len(snapshotSettings.AxentaToken))

	// Проверяем текущее состояние БД ДО загрузки
	fmt.Println("📊 Состояние БД ДО загрузки:")
	var countBefore int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&countBefore)
	fmt.Printf("   - Объектов в axenta_object_snapshots: %d\n", countBefore)

	var countWithDateBefore int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL").
		Count(&countWithDateBefore)
	fmt.Printf("   - Объектов с axenta_created_at: %d\n\n", countWithDateBefore)

	// Создаем сервис накопления
	accumulationService := services.NewSnapshotAccumulationService()

	// Загружаем все текущие объекты
	fmt.Println("🔄 Загружаем все текущие объекты из Axenta...")
	startTime := time.Now()
	if err := accumulationService.LoadAllCurrentObjects(snapshotSettings.AxentaToken); err != nil {
		log.Fatalf("❌ Ошибка загрузки объектов: %v", err)
	}
	duration := time.Since(startTime)
	fmt.Printf("✅ Загрузка завершена за %v\n\n", duration)

	// Ждем немного, чтобы убедиться, что данные записались
	time.Sleep(2 * time.Second)

	// Проверяем состояние БД ПОСЛЕ загрузки
	fmt.Println("📊 Состояние БД ПОСЛЕ загрузки:")
	var countAfter int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).Count(&countAfter)
	fmt.Printf("   - Объектов в axenta_object_snapshots: %d (было: %d, добавлено: %d)\n", countAfter, countBefore, countAfter-countBefore)

	var countWithDateAfter int64
	tenantDB.Model(&models.AxentaObjectSnapshot{}).
		Where("axenta_created_at IS NOT NULL").
		Count(&countWithDateAfter)
	fmt.Printf("   - Объектов с axenta_created_at: %d (было: %d, добавлено: %d)\n\n", countWithDateAfter, countWithDateBefore, countWithDateAfter-countWithDateBefore)

	// Ищем объект с самой ранней датой создания
	fmt.Println("🔍 Поиск объекта с самой ранней датой создания...")
	var earliestObject models.AxentaObjectSnapshot
	query := tenantDB.
		Where("axenta_created_at IS NOT NULL").
		Order("axenta_created_at ASC").
		Limit(1)

	if err := query.First(&earliestObject).Error; err == nil && earliestObject.AxentaCreatedAt != nil {
		fmt.Println("✅ Найден объект с самой ранней датой создания:")
		fmt.Println()
		fmt.Printf("   📦 ID объекта: %d\n", earliestObject.ExternalObjectID)
		fmt.Printf("   📅 Дата создания: %s\n", earliestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("   📅 Дата создания (только дата): %s\n", earliestObject.AxentaCreatedAt.Format("2006-01-02"))
		fmt.Println()
		fmt.Printf("   📋 Дополнительная информация:\n")
		fmt.Printf("      Название: %s\n", earliestObject.ObjectName)
		fmt.Printf("      Компания: %s (схема: %s)\n", firstCompany.Name, firstCompany.DatabaseSchema)
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
	} else {
		fmt.Println("⚠️ Объекты с axenta_created_at не найдены, проверяем по created_at...")
		var fallbackObject models.AxentaObjectSnapshot
		fallbackQuery := tenantDB.
			Order("created_at ASC").
			Limit(1)

		if err := fallbackQuery.First(&fallbackObject).Error; err == nil {
			fmt.Println("✅ Найден объект по created_at:")
			fmt.Println()
			fmt.Printf("   📦 ID объекта: %d\n", fallbackObject.ExternalObjectID)
			fmt.Printf("   📅 Дата создания записи в БД: %s\n", fallbackObject.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("   📅 Дата создания записи (только дата): %s\n", fallbackObject.CreatedAt.Format("2006-01-02"))
			fmt.Println()
			fmt.Printf("   📋 Дополнительная информация:\n")
			fmt.Printf("      Название: %s\n", fallbackObject.ObjectName)
			fmt.Printf("      Компания: %s (схема: %s)\n", firstCompany.Name, firstCompany.DatabaseSchema)
			if fallbackObject.AxentaCreatedAt != nil {
				fmt.Printf("      Дата создания в Axenta: %s\n", fallbackObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
			}
			fmt.Println()
		} else {
			fmt.Println("❌ Объекты не найдены")
		}
	}

	// Показываем информацию о схеме и таблице
	fmt.Println("📌 Информация о записи в БД:")
	fmt.Printf("   - Схема: %s\n", firstCompany.DatabaseSchema)
	fmt.Printf("   - Таблица: axenta_object_snapshots\n")
	fmt.Printf("   - Всего объектов в таблице: %d\n", countAfter)

	fmt.Println("\n✅ Готово!")
}
