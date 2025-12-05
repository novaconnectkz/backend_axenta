package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"log"
	"os"
	"strings"
)

// NullWriter для подавления логов
type NullWriter struct{}

func (w *NullWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func main() {
	// Отключаем лишние логи для чистого вывода
	log.SetOutput(&NullWriter{})

	// Загружаем конфигурацию
	config.LoadConfig()

	// Подключаемся к базе данных
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Не удалось подключиться к базе данных: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		sqlDB, _ := database.DB.DB()
		sqlDB.Close()
	}()

	// Иерархия аккаунтов из Excel файла для Partner Company ID 17153
	// Партнер: 17153 - 97 объектов
	// Дочерние клиенты:
	accountHierarchy := map[int64]struct {
		Name        string
		Type        string
		ParentID    int64
		ExpectedObj int
	}{
		17153: {"Черкасов Валихан Самигуллаевич ИП", "Партнер", 0, 97},
		22911: {"Карповка", "Клиент", 17153, 38},
		24188: {"Сидоренко", "Клиент", 17153, 19},
		23660: {"Булат", "Клиент", 17153, 11},
		23662: {"Зонов", "Клиент", 17153, 10},
		23752: {"Попова екатерина", "Клиент", 17153, 5},
		25514: {"Шакман", "Клиент", 17153, 5},
		25492: {"Жанна Лорд", "Клиент", 17153, 4},
		25671: {"Сергей", "Клиент", 17153, 3},
		25672: {"Барахтенко", "Клиент", 17153, 2},
	}

	partnerCompanyID := int64(17153)

	fmt.Printf("🔍 Проверка объектов для Partner Company ID: %d\n", partnerCompanyID)
	fmt.Printf("📋 Иерархия из Excel файла:\n\n")

	// Выводим иерархию
	fmt.Printf("Партнер:\n")
	fmt.Printf("  ID: %d - %s (%s) - ожидается %d объектов\n", partnerCompanyID,
		accountHierarchy[partnerCompanyID].Name,
		accountHierarchy[partnerCompanyID].Type,
		accountHierarchy[partnerCompanyID].ExpectedObj)

	fmt.Printf("\nДочерние клиенты:\n")
	for accountID, info := range accountHierarchy {
		if accountID != partnerCompanyID {
			fmt.Printf("  ID: %d - %s (%s) - ожидается %d объектов\n", accountID, info.Name, info.Type, info.ExpectedObj)
		}
	}

	fmt.Printf("\n" + strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании для поиска во всех tenant схемах
	var allCompanies []models.Company
	if err := database.DB.Find(&allCompanies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "❌ Ошибка получения списка компаний: %v\n", err)
		os.Exit(1)
	}

	// Собираем все account_external_id для поиска
	allAccountIDs := make([]int64, 0, len(accountHierarchy))
	for accountID := range accountHierarchy {
		allAccountIDs = append(allAccountIDs, accountID)
	}

	totalFoundInDB := 0
	totalActiveInDB := 0
	totalExpected := 0

	// Проверяем каждый аккаунт
	for accountID, info := range accountHierarchy {
		totalExpected += info.ExpectedObj

		fmt.Printf("📊 Проверка аккаунта ID=%d (%s):\n", accountID, info.Name)

		// Ищем объекты во всех tenant схемах
		allObjectIDs := make(map[int64]bool)
		activeObjectIDs := make(map[int64]bool)

		for _, company := range allCompanies {
			tenantDB := database.GetTenantDBByID(company.ID)
			if tenantDB == nil {
				continue
			}

			var objects []models.AxentaObjectSnapshot
			if err := tenantDB.Where("account_external_id = ?", accountID).
				Order("last_synced_at DESC").
				Find(&objects).Error; err == nil {
				for _, obj := range objects {
					allObjectIDs[obj.ExternalObjectID] = true
					if obj.IsActive {
						activeObjectIDs[obj.ExternalObjectID] = true
					}
				}
			}
		}

		foundCount := len(allObjectIDs)
		activeCount := len(activeObjectIDs)
		totalFoundInDB += foundCount
		totalActiveInDB += activeCount

		fmt.Printf("   Ожидается объектов: %d\n", info.ExpectedObj)
		fmt.Printf("   Найдено в БД: %d (активных: %d)\n", foundCount, activeCount)

		if foundCount == info.ExpectedObj {
			fmt.Printf("   ✅ Количество совпадает!\n")
		} else if foundCount < info.ExpectedObj {
			fmt.Printf("   ⚠️ Не хватает: %d объектов\n", info.ExpectedObj-foundCount)
		} else {
			fmt.Printf("   ⚠️ Больше чем ожидалось: +%d объектов\n", foundCount-info.ExpectedObj)
		}

		// Показываем первые 5 объектов
		if foundCount > 0 {
			fmt.Printf("   Примеры объектов:\n")
			count := 0
			for _, company := range allCompanies {
				if count >= 5 {
					break
				}
				tenantDB := database.GetTenantDBByID(company.ID)
				if tenantDB == nil {
					continue
				}
				var objects []models.AxentaObjectSnapshot
				tenantDB.Where("account_external_id = ?", accountID).
					Order("last_synced_at DESC").
					Limit(5 - count).
					Find(&objects)
				for _, obj := range objects {
					if count >= 5 {
						break
					}
					fmt.Printf("      - %s (ID: %d, активен: %v)\n", obj.ObjectName, obj.ExternalObjectID, obj.IsActive)
					count++
				}
			}
		}
		fmt.Println()
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n\n")
	fmt.Printf("   Ожидается объектов (из Excel): %d\n", totalExpected)
	fmt.Printf("   Найдено в БД: %d (активных: %d)\n", totalFoundInDB, totalActiveInDB)

	if totalFoundInDB == totalExpected {
		fmt.Printf("   ✅ Все объекты найдены!\n")
	} else if totalFoundInDB < totalExpected {
		fmt.Printf("   ⚠️ Не хватает: %d объектов\n", totalExpected-totalFoundInDB)
	} else {
		fmt.Printf("   ⚠️ Больше чем ожидалось: +%d объектов\n", totalFoundInDB-totalExpected)
	}

	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
