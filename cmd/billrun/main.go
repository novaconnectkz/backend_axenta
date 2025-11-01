package main

import (
	"backend_axenta/config"
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func main() {
	// Флаги командной строки
	companyID := flag.Int("company", 0, "ID компании для генерации счетов")
	contractID := flag.Int("contract", 0, "ID договора для генерации счетов (опционально)")
	period := flag.String("period", "", "Период биллинга в формате YYYY-MM (например, 2025-10)")
	month := flag.String("month", "", "Месяц биллинга в формате YYYY-MM (алиас для period)")
	dryRun := flag.Bool("dry-run", false, "Режим сухого прогона (не создавать счета, только показать расчеты)")
	help := flag.Bool("help", false, "Показать справку")

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Валидация аргументов
	if *companyID == 0 {
		log.Fatalf("❌ Ошибка: необходимо указать ID компании (--company)")
	}

	var periodStr string
	if *period != "" {
		periodStr = *period
	} else if *month != "" {
		periodStr = *month
	} else {
		// Используем текущий месяц
		periodStr = time.Now().Format("2006-01")
		log.Printf("📅 Период не указан, используем текущий месяц: %s", periodStr)
	}

	log.Printf("🚀 Запуск billrun для компании %d, период: %s", *companyID, periodStr)

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("❌ Не удалось загрузить конфигурацию: %v", err)
	}

	log.Printf("🔧 Подключение к базе данных: %s@%s:%s/%s",
		cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)

	// Подключаемся к базе данных без миграций (они уже должны быть выполнены)
	if err := database.ConnectDatabaseWithoutMigrations(); err != nil {
		log.Fatalf("❌ Не удалось подключиться к базе данных: %v", err)
	}

	log.Println("✅ Подключение к базе данных установлено")

	db := database.GetDB()

	// Проверяем существование компании
	var company models.Company
	if err := db.First(&company, *companyID).Error; err != nil {
		log.Fatalf("❌ Компания с ID %d не найдена: %v", *companyID, err)
	}

	log.Printf("🏢 Компания: %s", company.Name)

	// Парсим период
	periodTime, err := time.Parse("2006-01", periodStr)
	if err != nil {
		log.Fatalf("❌ Неправильный формат периода: %v (используйте формат YYYY-MM, например 2025-10)", err)
	}

	startDate := time.Date(periodTime.Year(), periodTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Second) // Последний день месяца

	log.Printf("📅 Период: %s - %s", startDate.Format("02.01.2006"), endDate.Format("02.01.2006"))

	// Создаем сервисы
	billingService := services.NewBillingService()

	// Если указан конкретный договор
	if *contractID > 0 {
		log.Printf("📝 Обработка договора ID: %d", *contractID)
		processContract(billingService, db, uint(*contractID), startDate, endDate, *dryRun)
	} else {
		// Обрабатываем все активные договоры компании
		log.Println("📋 Поиск активных договоров компании...")

		var contracts []models.Contract
		if err := db.Where("company_id = ? AND status = ? AND is_active = true", *companyID, "active").
			Preload("TariffPlan").
			Find(&contracts).Error; err != nil {
			log.Fatalf("❌ Ошибка получения договоров: %v", err)
		}

		if len(contracts) == 0 {
			log.Printf("⚠️ Активные договоры не найдены для компании %d", *companyID)
			return
		}

		log.Printf("✅ Найдено активных договоров: %d", len(contracts))

		// Обрабатываем каждый договор
		for i, contract := range contracts {
			log.Printf("")
			log.Printf("📄 Договор %d/%d: %s", i+1, len(contracts), contract.Title)
			processContract(billingService, db, contract.ID, startDate, endDate, *dryRun)
		}
	}

	log.Println("")
	log.Println("🎉 Генерация счетов завершена!")
}

func processContract(billingService *services.BillingService, db *gorm.DB, contractID uint, startDate, endDate time.Time, dryRun bool) {
	// Проверяем, не создан ли уже счет за этот период
	var existingInvoice models.Invoice
	err := db.Where("contract_id = ? AND billing_period_start = ? AND billing_period_end = ? AND deleted_at IS NULL",
		contractID, startDate, endDate).First(&existingInvoice).Error
	
	if err == nil {
		log.Printf("⏭️ Счет за период %s уже существует: %s (статус: %s)",
			startDate.Format("01.2006"), existingInvoice.Number, existingInvoice.Status)
		return
	}

	// Рассчитываем биллинг
	log.Println("🧮 Расчет биллинга...")
	calculation, err := billingService.CalculateBillingForContract(contractID, startDate, endDate)
	if err != nil {
		log.Printf("❌ Ошибка расчета биллинга для договора %d: %v", contractID, err)
		return
	}

	// Выводим результаты расчета
	log.Printf("📊 Результаты расчета:")
	log.Printf("   Активных объектов: %d", calculation.ActiveObjects)
	log.Printf("   Неактивных объектов: %d", calculation.InactiveObjects)
	log.Printf("   Запланировано к удалению: %d", calculation.ScheduledDeletes)
	log.Printf("   Базовая сумма: %s", formatCurrency(calculation.BaseAmount))
	log.Printf("   Сумма за объекты: %s", formatCurrency(calculation.ObjectsAmount))
	log.Printf("   Скидка: %s", formatCurrency(calculation.DiscountAmount))
	log.Printf("   НДС: %s", formatCurrency(calculation.TaxAmount))
	log.Printf("   ИТОГО: %s", formatCurrency(calculation.TotalAmount))

	if len(calculation.Items) > 0 {
		log.Printf("   Позиций в счете: %d", len(calculation.Items))
	}

	// Если dry-run, не создаем счета
	if dryRun {
		log.Printf("🔍 Режим dry-run: счета не будут созданы")
		return
	}

	// Создаем счет
	log.Println("📝 Создание счета...")
	invoice, err := billingService.GenerateInvoiceForContract(contractID, startDate, endDate)
	if err != nil {
		log.Printf("❌ Ошибка создания счета: %v", err)
		return
	}

	log.Printf("✅ Счет успешно создан: %s", invoice.Number)
	log.Printf("   Статус: %s", invoice.Status)
	log.Printf("   Сумма: %s", formatCurrency(invoice.TotalAmount))
	log.Printf("   Срок оплаты: %s", invoice.DueDate.Format("02.01.2006"))
}

func formatCurrency(amount decimal.Decimal) string {
	return fmt.Sprintf("%.2f ₽", amount.InexactFloat64())
}

func printHelp() {
	fmt.Println("🚀 BillRun - Генератор счетов для биллинга")
	fmt.Println("")
	fmt.Println("Использование:")
	fmt.Println("  go run cmd/billrun/main.go [флаги]")
	fmt.Println("")
	fmt.Println("Флаги:")
	fmt.Println("  --company ID     ID компании (обязательно)")
	fmt.Println("  --contract ID    ID договора (опционально, по умолчанию обрабатываются все активные)")
	fmt.Println("  --period YYYY-MM Период биллинга (например, 2025-10)")
	fmt.Println("  --month YYYY-MM  Алиас для --period")
	fmt.Println("  --dry-run         Режим сухого прогона (показать расчеты без создания счетов)")
	fmt.Println("  --help            Показать эту справку")
	fmt.Println("")
	fmt.Println("Примеры:")
	fmt.Println("  # Генерация счетов за текущий месяц для всех договоров компании 1")
	fmt.Println("  go run cmd/billrun/main.go --company 1")
	fmt.Println("")
	fmt.Println("  # Генерация счетов за октябрь 2025 для компании 1")
	fmt.Println("  go run cmd/billrun/main.go --company 1 --period 2025-10")
	fmt.Println("")
	fmt.Println("  # Генерация счета для конкретного договора")
	fmt.Println("  go run cmd/billrun/main.go --company 1 --contract 5 --period 2025-10")
	fmt.Println("")
	fmt.Println("  # Проверка расчетов без создания счетов")
	fmt.Println("  go run cmd/billrun/main.go --company 1 --period 2025-10 --dry-run")
}
