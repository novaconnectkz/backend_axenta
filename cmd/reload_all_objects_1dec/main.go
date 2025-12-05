package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"fmt"
	"log"
	"strings"
	"time"
)

func main() {
	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		log.Fatalf("Ошибка подключения к БД: %v", err)
	}

	// Дата снимка: 1 декабря 2025 года
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	startTime := time.Now()
	fmt.Printf("📸 Перезагрузка всех объектов за %s в БД\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("⏰ Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		log.Fatalf("Ошибка получения компаний: %v", err)
	}

	// Создаем сервисы
	snapshotService := services.NewPartnerSnapshotService()

	totalObjectsSaved := 0
	totalPartnersProcessed := 0

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		fmt.Printf("🏢 Компания: %s\n", company.Name)

		// Получаем токен для компании (проверяем в разных полях)
		var snapshotSettings models.SnapshotSettings
		var token string

		// Пробуем найти по admin_account_id
		if err := tenantDB.Where("admin_account_id = ?", company.ID).First(&snapshotSettings).Error; err == nil && snapshotSettings.AxentaToken != "" {
			token = snapshotSettings.AxentaToken
		} else {
			// Пробуем найти по company_id
			if err := tenantDB.Where("company_id = ?", company.ID).First(&snapshotSettings).Error; err == nil && snapshotSettings.AxentaToken != "" {
				token = snapshotSettings.AxentaToken
			} else {
				// Пробуем найти любой токен в схеме
				if err := tenantDB.First(&snapshotSettings).Error; err == nil && snapshotSettings.AxentaToken != "" {
					token = snapshotSettings.AxentaToken
				}
			}
		}

		if token == "" {
			fmt.Printf("   ⚠️ Токен не найден, пропускаем\n\n")
			continue
		}

		// Получаем все снимки за эту дату
		var snapshots []models.PartnerDailySnapshot
		if err := tenantDB.
			Where("DATE(snapshot_date AT TIME ZONE 'UTC') = ?", snapshotDate.Format("2006-01-02")).
			Find(&snapshots).Error; err != nil {
			fmt.Printf("   ⚠️ Ошибка получения снимков: %v\n\n", err)
			continue
		}

		if len(snapshots) == 0 {
			fmt.Printf("   ℹ️ Нет снимков за эту дату\n\n")
			continue
		}

		fmt.Printf("   Найдено снимков: %d\n", len(snapshots))

		// Загружаем иерархию для поиска партнеров
		hierarchyService := services.NewAccountHierarchyService(token)
		if err := hierarchyService.LoadAllAccounts(); err != nil {
			fmt.Printf("   ⚠️ Ошибка загрузки иерархии: %v\n\n", err)
			continue
		}

		// Для каждого снимка загружаем и сохраняем объекты
		for _, snapshot := range snapshots {
			partnerID := int64(snapshot.PartnerCompanyID)
			expectedCount := snapshot.TotalObjectsCount

			fmt.Printf("   📦 Партнер %d: ожидается объектов=%d\n", partnerID, expectedCount)

			// Сохраняем объекты для этого партнера
			if err := snapshotService.SavePartnerObjectsToDBForSnapshot(
				company.ID,
				snapshot.PartnerCompanyID,
				token,
				snapshotDate,
				tenantDB,
			); err != nil {
				fmt.Printf("      ❌ Ошибка: %v\n", err)
				continue
			}

			// Проверяем сколько объектов сохранилось
			var objectsCount int64
			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Where("account_external_id = ? AND DATE(last_synced_at AT TIME ZONE 'UTC') = ?", partnerID, snapshotDate.Format("2006-01-02")).
				Count(&objectsCount)

			savedCount := int(objectsCount)
			totalObjectsSaved += savedCount
			totalPartnersProcessed++

			if savedCount >= expectedCount {
				fmt.Printf("      ✅ Сохранено объектов: %d (ожидалось: %d)\n", savedCount, expectedCount)
			} else {
				fmt.Printf("      ⚠️ Сохранено объектов: %d (ожидалось: %d)\n", savedCount, expectedCount)
			}
		}

		fmt.Printf("\n")
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Дата: %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("   - Партнеров обработано: %d\n", totalPartnersProcessed)
	fmt.Printf("   - Всего объектов сохранено: %d\n", totalObjectsSaved)
	fmt.Printf("   - Время выполнения: %v\n", duration.Round(time.Second))
	fmt.Printf("   - Начало: %s\n", startTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("   - Конец: %s\n", endTime.Format("2006-01-02 15:04:05"))
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
