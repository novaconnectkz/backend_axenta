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

	// Загружаем конфиг
	config.LoadConfig()

	// Инициализируем БД
	if err := database.ConnectDatabase(); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка подключения к БД: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📊 Подсчет объектов в таблице axenta_object_snapshots\n")
	fmt.Printf(strings.Repeat("=", 100) + "\n\n")

	// Получаем все компании
	var companies []models.Company
	if err := database.DB.Find(&companies).Error; err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка получения компаний: %v\n", err)
		os.Exit(1)
	}

	totalObjects := 0
	totalActive := 0
	totalInactive := 0
	companiesWithObjects := 0

	// Обрабатываем каждую компанию
	for _, company := range companies {
		tenantDB := database.GetTenantDBByID(company.ID)
		if tenantDB == nil {
			continue
		}

		// Подсчитываем все объекты
		var objectCount int64
		var activeCount int64
		var inactiveCount int64

		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Count(&objectCount).Error; err != nil {
			continue
		}

		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("is_active = ?", true).
			Count(&activeCount).Error; err != nil {
			continue
		}

		if err := tenantDB.Model(&models.AxentaObjectSnapshot{}).
			Where("is_active = ?", false).
			Count(&inactiveCount).Error; err != nil {
			continue
		}

		if objectCount > 0 {
			companiesWithObjects++
			totalObjects += int(objectCount)
			totalActive += int(activeCount)
			totalInactive += int(inactiveCount)

			fmt.Printf("🏢 Компания: %s (схема: %s)\n", company.Name, company.DatabaseSchema)
			fmt.Printf("   - Всего объектов: %d\n", objectCount)
			fmt.Printf("   - Активных: %d\n", activeCount)
			fmt.Printf("   - Неактивных: %d\n", inactiveCount)
			fmt.Printf("\n")
		}
	}

	// Итоговая статистика
	fmt.Printf(strings.Repeat("=", 100) + "\n")
	fmt.Printf("📊 ИТОГОВАЯ СТАТИСТИКА:\n\n")
	fmt.Printf("   - Всего компаний проверено: %d\n", len(companies))
	fmt.Printf("   - Компаний с объектами: %d\n", companiesWithObjects)
	fmt.Printf("   - Всего объектов в axenta_object_snapshots: %d\n", totalObjects)
	fmt.Printf("   - Активных объектов: %d\n", totalActive)
	fmt.Printf("   - Неактивных объектов: %d\n", totalInactive)
	fmt.Printf(strings.Repeat("=", 100) + "\n")
}
