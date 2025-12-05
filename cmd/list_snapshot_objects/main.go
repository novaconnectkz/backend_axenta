package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// NullWriter для подавления логов
type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	// Отключаем лишние логи для чистого вывода
	log.SetOutput(&NullWriter{})

	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	// Параметры снимка
	snapshotDate := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	partnerCompanyID := int64(1938) // Partner Company ID из снимка

	fmt.Printf("📋 Список объектов из снимка за %s\n", snapshotDate.Format("2006-01-02"))
	fmt.Printf("Partner Company ID: %d\n\n", partnerCompanyID)

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	var allObjects []models.AxentaObjectSnapshot
	var companyName string

	// Ищем объекты во всех компаниях
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Ищем объекты для этого партнера
		// Используем дату конца дня для фильтрации (объекты синхронизированные до или в этот день)
		snapshotEndOfDay := time.Date(snapshotDate.Year(), snapshotDate.Month(), snapshotDate.Day(), 23, 59, 59, 999999999, time.UTC)

		// Пробуем несколько вариантов поиска
		var objects []models.AxentaObjectSnapshot

		// Вариант 1: поиск по account_external_id (как int64)
		if err := tenantDB.
			Where("account_external_id = ? AND last_synced_at <= ?", partnerCompanyID, snapshotEndOfDay).
			Order("object_name ASC").
			Find(&objects).Error; err == nil && len(objects) > 0 {
			allObjects = append(allObjects, objects...)
			companyName = company.Name
			break
		}

		// Вариант 2: поиск по account_external_id без фильтра по дате
		if len(objects) == 0 {
			if err := tenantDB.
				Where("account_external_id = ?", partnerCompanyID).
				Order("object_name ASC").
				Find(&objects).Error; err == nil && len(objects) > 0 {
				allObjects = append(allObjects, objects...)
				companyName = company.Name
				break
			}
		}

		// Вариант 3: поиск всех объектов и фильтрация по дате
		if len(objects) == 0 {
			var allCompanyObjects []models.AxentaObjectSnapshot
			if err := tenantDB.
				Where("last_synced_at <= ?", snapshotEndOfDay).
				Order("object_name ASC").
				Find(&allCompanyObjects).Error; err == nil {
				// Фильтруем по account_external_id в памяти
				for _, obj := range allCompanyObjects {
					if int64(obj.AccountExternalID) == partnerCompanyID {
						objects = append(objects, obj)
					}
				}
				if len(objects) > 0 {
					allObjects = append(allObjects, objects...)
					companyName = company.Name
					break
				}
			}
		}
	}

	if len(allObjects) == 0 {
		fmt.Printf("⚠️ Объекты не найдены для Partner Company ID %d за дату %s\n\n", partnerCompanyID, snapshotDate.Format("2006-01-02"))

		// Пробуем найти, какие account_external_id есть в базе
		fmt.Printf("🔍 Проверяем доступные account_external_id в базе данных...\n\n")
		for _, company := range companies {
			tenantDB := database.GetTenantDBByID(company.ID)
			if tenantDB == nil {
				continue
			}

			var uniqueAccounts []struct {
				AccountExternalID int64 `gorm:"column:account_external_id"`
				Count             int64
			}

			tenantDB.Model(&models.AxentaObjectSnapshot{}).
				Select("account_external_id, COUNT(*) as count").
				Group("account_external_id").
				Order("count DESC").
				Limit(10).
				Find(&uniqueAccounts)

			if len(uniqueAccounts) > 0 {
				fmt.Printf("Компания %s (схема: %s):\n", company.Name, company.DatabaseSchema)
				for _, acc := range uniqueAccounts {
					fmt.Printf("   - account_external_id: %d (объектов: %d)\n", acc.AccountExternalID, acc.Count)
				}
				fmt.Printf("\n")
			}
		}

		fmt.Printf("💡 Примечание: Объекты могут загружаться из Axenta Cloud API при создании снимка,\n")
		fmt.Printf("   а не храниться в базе данных. Для получения списка объектов может потребоваться\n")
		fmt.Printf("   запрос к Axenta Cloud API с токеном авторизации.\n")

		os.Exit(1)
	}

	fmt.Printf("Компания: %s\n", companyName)
	fmt.Printf("Всего найдено объектов: %d\n\n", len(allObjects))
	fmt.Printf(strings.Repeat("=", 100) + "\n")

	// Подсчитываем активные и неактивные
	activeCount := 0
	inactiveCount := 0

	// Выводим список объектов
	for i, obj := range allObjects {
		statusIcon := "❌"
		if obj.IsActive {
			statusIcon = "✅"
			activeCount++
		} else {
			inactiveCount++
		}

		fmt.Printf("%3d. %s %s\n", i+1, statusIcon, obj.ObjectName)
		fmt.Printf("     ID: %d | External ID: %d | Account External ID: %d\n", obj.ID, obj.ExternalObjectID, obj.AccountExternalID)
		fmt.Printf("     Статус: %s | Тип устройства: %s\n", obj.Status, obj.DeviceTypeName)
		fmt.Printf("     Уникальный ID: %s\n", obj.UniqueID)

		if obj.LastCommunicationAt != nil {
			fmt.Printf("     Последняя связь: %s\n", obj.LastCommunicationAt.Format("2006-01-02 15:04:05"))
		}

		if obj.ScheduledDeleteAt != nil {
			fmt.Printf("     ⚠️ Запланировано удаление: %s\n", obj.ScheduledDeleteAt.Format("2006-01-02 15:04:05"))
		}

		if obj.CreatorName != nil && *obj.CreatorName != "" {
			fmt.Printf("     Создатель: %s", *obj.CreatorName)
			if obj.CreatorID != nil {
				fmt.Printf(" (ID: %d)", *obj.CreatorID)
			}
			fmt.Printf("\n")
		}

		if obj.PhoneNumbers != nil && *obj.PhoneNumbers != "" && *obj.PhoneNumbers != "null" {
			fmt.Printf("     Телефоны: %s\n", *obj.PhoneNumbers)
		}

		if obj.AxentaCreatedAt != nil {
			fmt.Printf("     Создан в Axenta: %s\n", obj.AxentaCreatedAt.Format("2006-01-02 15:04:05"))
		}

		fmt.Printf("     Последняя синхронизация: %s\n", obj.LastSyncedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("\n")
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n")
	fmt.Printf("   - Найдено объектов в базе данных: %d\n", len(allObjects))
	fmt.Printf("   - ✅ Активных: %d\n", activeCount)
	fmt.Printf("   - ❌ Неактивных: %d\n", inactiveCount)

	// Проверяем, что показано в снимке
	var snapshot models.PartnerDailySnapshot

	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		if err := tenantDB.
			Where("partner_company_id = ? AND DATE(snapshot_date AT TIME ZONE 'UTC') = ?", partnerCompanyID, snapshotDate.Format("2006-01-02")).
			First(&snapshot).Error; err == nil {
			fmt.Printf("\n📸 ДАННЫЕ ИЗ СНИМКА:\n")
			fmt.Printf("   - Всего объектов в снимке: %d\n", snapshot.TotalObjectsCount)
			fmt.Printf("   - Активных объектов в снимке: %d\n", snapshot.ActiveObjectsCount)

			if len(allObjects) < snapshot.TotalObjectsCount {
				fmt.Printf("\n⚠️ ВНИМАНИЕ: В базе данных найдено только %d объектов из %d, указанных в снимке.\n", len(allObjects), snapshot.TotalObjectsCount)
				fmt.Printf("   Остальные %d объектов не синхронизированы в таблицу axenta_object_snapshots.\n", snapshot.TotalObjectsCount-len(allObjects))
				fmt.Printf("   Они были учтены при создании снимка через Axenta Cloud API.\n")
			}
			break
		}
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
