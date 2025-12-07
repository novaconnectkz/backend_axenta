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
	log.Println("🔍 Поиск последнего созданного объекта после загрузки...")

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

	// Получаем все активные компании
	var companies []models.Company
	if err := database.DB.Table("public.companies").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		log.Fatalf("❌ Не удалось получить список компаний: %v", err)
	}

	fmt.Printf("📋 Проверяем объекты в %d компаниях...\n\n", len(companies))

	var latestObject *models.AxentaObjectSnapshot
	var latestCompany *models.Company
	var latestDate time.Time

	// Ищем последний объект во всех компаниях
	for _, company := range companies {
		fmt.Printf("🏢 Компания: %s (ID=%d, схема: %s)\n", company.Name, company.ID, company.DatabaseSchema)

		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			fmt.Printf("   ⚠️ Не удалось получить tenant DB, пропускаем\n\n")
			continue
		}

		// Ищем объект с самой поздней датой создания (axenta_created_at)
		var object models.AxentaObjectSnapshot
		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("axenta_created_at IS NOT NULL").
			Order("axenta_created_at DESC").
			First(&object).Error; err != nil {
			fmt.Printf("   ⚠️ Объекты с axenta_created_at не найдены\n")

			// Пробуем найти по created_at (дата создания записи в БД)
			if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Order("created_at DESC").
				First(&object).Error; err != nil {
				fmt.Printf("   ⚠️ Объекты не найдены в этой схеме\n\n")
				continue
			}
			fmt.Printf("   📅 Найден объект по created_at: %s\n", object.CreatedAt.Format("2006-01-02 15:04:05"))
		} else {
			fmt.Printf("   📅 Найден объект по axenta_created_at: %s\n", object.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
		}

		// Определяем дату для сравнения
		var compareDate time.Time
		if object.AxentaCreatedAt != nil && !object.AxentaCreatedAt.IsZero() {
			compareDate = *object.AxentaCreatedAt
		} else {
			compareDate = object.CreatedAt
		}

		// Проверяем, является ли этот объект последним
		if latestObject == nil || compareDate.After(latestDate) {
			latestObject = &object
			latestCompany = &company
			latestDate = compareDate
		}

		fmt.Printf("   📦 ID объекта: %d\n", object.ExternalObjectID)
		fmt.Printf("   📝 Название: %s\n", object.ObjectName)
		if object.AxentaCreatedAt != nil && !object.AxentaCreatedAt.IsZero() {
			fmt.Printf("   📅 Дата создания в Axenta: %s\n", object.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("   📅 Дата создания записи в БД: %s\n", object.CreatedAt.Format("2006-01-02 15:04:05"))
		if !object.LastSyncedAt.IsZero() {
			fmt.Printf("   🔄 Последняя синхронизация: %s\n", object.LastSyncedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}

	// Выводим результат
	fmt.Println(strings.Repeat("=", 60))
	if latestObject != nil {
		fmt.Println("✅ Последний созданный объект найден:")
		fmt.Printf("   🏢 Компания: %s (ID=%d, схема: %s)\n", latestCompany.Name, latestCompany.ID, latestCompany.DatabaseSchema)
		fmt.Printf("   📦 ID объекта: %d\n", latestObject.ExternalObjectID)
		fmt.Printf("   📝 Название: %s\n", latestObject.ObjectName)
		if latestObject.AxentaCreatedAt != nil && !latestObject.AxentaCreatedAt.IsZero() {
			fmt.Printf("   📅 Дата создания в Axenta: %s\n", latestObject.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("   📅 Дата создания (только дата): %s\n", latestObject.AxentaCreatedAt.Format("2006-01-02"))
		}
		fmt.Printf("   📅 Дата создания записи в БД: %s\n", latestObject.CreatedAt.Format("2006-01-02 15:04:05"))
		if !latestObject.LastSyncedAt.IsZero() {
			fmt.Printf("   🔄 Последняя синхронизация: %s\n", latestObject.LastSyncedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Printf("   📊 Статус: %s\n", latestObject.Status)
		fmt.Printf("   ✅ Активен: %v\n", latestObject.IsActive)
		if latestObject.AccountName != "" {
			fmt.Printf("   👤 Аккаунт: %s\n", latestObject.AccountName)
		}
		if latestObject.DeviceTypeName != "" {
			fmt.Printf("   🔧 Тип устройства: %s\n", latestObject.DeviceTypeName)
		}
	} else {
		fmt.Println("❌ Объекты не найдены")
	}
	fmt.Println(strings.Repeat("=", 60))
}
