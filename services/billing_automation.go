package services

import (
	"fmt"
	"log"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BillingAutomationService предоставляет функции для автоматизации биллинга
type BillingAutomationService struct {
	db             *gorm.DB
	billingService *BillingService
	adminAccountID uint
}

// NewBillingAutomationService создает новый экземпляр BillingAutomationService
func NewBillingAutomationService(adminAccountID uint) *BillingAutomationService {
	return &BillingAutomationService{
		db:             database.DB,
		billingService: NewBillingService(adminAccountID),
		adminAccountID: adminAccountID,
	}
}

// BillingStatistics содержит статистику биллинга
type BillingStatistics struct {
	CompanyID         uint            `json:"company_id"`
	Year              int             `json:"year"`
	Month             *int            `json:"month,omitempty"`
	TotalInvoices     int             `json:"total_invoices"`
	PaidInvoices      int             `json:"paid_invoices"`
	PendingInvoices   int             `json:"pending_invoices"`
	OverdueInvoices   int             `json:"overdue_invoices"`
	CancelledInvoices int             `json:"cancelled_invoices"`
	TotalAmount       decimal.Decimal `json:"total_amount"`
	PaidAmount        decimal.Decimal `json:"paid_amount"`
	PendingAmount     decimal.Decimal `json:"pending_amount"`
	OverdueAmount     decimal.Decimal `json:"overdue_amount"`
}

// AutoGenerateInvoicesForMonth автоматически генерирует счета за месяц для всех активных договоров
func (bas *BillingAutomationService) AutoGenerateInvoicesForMonth(year int, month int) error {
	// Получаем все активные договоры
	var contracts []models.Contract
	if err := bas.db.Where("status = ? AND start_date <= ? AND end_date >= ? AND admin_account_id = ?",
		"active",
		time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
		time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
		bas.adminAccountID).
		Find(&contracts).Error; err != nil {
		return fmt.Errorf("ошибка получения активных договоров: %w", err)
	}

	// Определяем период биллинга
	periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	successCount := 0
	errors := make([]error, 0)

	for _, contract := range contracts {
		// Проверяем, не создан ли уже счет за этот период
		var existingInvoice models.Invoice
		if err := bas.db.Where("contract_id = ? AND billing_period_start = ? AND billing_period_end = ? AND admin_account_id = ?",
			contract.ID, periodStart, periodEnd, bas.adminAccountID).First(&existingInvoice).Error; err == nil {
			// Счет уже существует, пропускаем
			continue
		}

		// Генерируем счет
		_, err := bas.billingService.GenerateInvoiceForContract(contract.ID, periodStart, periodEnd)
		if err != nil {
			errors = append(errors, fmt.Errorf("ошибка генерации счета для договора %s: %w", contract.Number, err))
			continue
		}

		successCount++
	}

	if len(errors) > 0 {
		return fmt.Errorf("генерация завершена с ошибками. Успешно создано счетов: %d, ошибок: %d. Первая ошибка: %v",
			successCount, len(errors), errors[0])
	}

	return nil
}

// ProcessScheduledDeletions обрабатывает плановые удаления объектов и корректирует биллинг
func (bas *BillingAutomationService) ProcessScheduledDeletions() error {
	// Получаем объекты с плановым удалением на сегодня или раньше
	var objectsToDelete []models.Object
	if err := bas.db.Where("scheduled_delete_at IS NOT NULL AND scheduled_delete_at <= ?", time.Now()).
		Find(&objectsToDelete).Error; err != nil {
		return fmt.Errorf("ошибка получения объектов для планового удаления: %w", err)
	}

	if len(objectsToDelete) == 0 {
		return nil // Нет объектов для удаления
	}

	processedCount := 0
	errors := make([]error, 0)

	for _, obj := range objectsToDelete {
		// Создаем запись в истории биллинга о плановом удалении
		history := &models.BillingHistory{
			ContractID:  obj.ContractID, // ContractID теперь *uint, не нужно брать адрес
			Operation:   "object_scheduled_deletion",
			Amount:      decimal.Zero,
			Currency:    "RUB",
			Description: fmt.Sprintf("Плановое удаление объекта \"%s\" (ID: %d)", obj.Name, obj.ID),
			Status:      "completed",
		}

		// Получаем CompanyID из договора
		var contract models.Contract
		// Проверяем, что ContractID не nil
		if obj.ContractID == nil {
			continue
		}
		if err := bas.db.Where("id = ? AND admin_account_id = ?", *obj.ContractID, bas.adminAccountID).
			First(&contract).Error; err == nil {
			history.CompanyID = contract.CompanyID
			history.AdminAccountID = bas.adminAccountID
		} else {
			// Договор не принадлежит текущему администратору, пропускаем объект
			continue
		}

		if err := bas.db.Create(history).Error; err != nil {
			errors = append(errors, fmt.Errorf("ошибка создания записи в истории для объекта %d: %w", obj.ID, err))
			continue
		}

		// Помечаем объект как неактивный вместо удаления
		if err := bas.db.Model(&obj).Updates(map[string]interface{}{
			"is_active": false,
			"status":    "scheduled_deleted",
		}).Error; err != nil {
			errors = append(errors, fmt.Errorf("ошибка обновления статуса объекта %d: %w", obj.ID, err))
			continue
		}

		processedCount++
	}

	if len(errors) > 0 {
		return fmt.Errorf("обработка плановых удалений завершена с ошибками. Обработано объектов: %d, ошибок: %d. Первая ошибка: %v",
			processedCount, len(errors), errors[0])
	}

	return nil
}

// ActivateScheduledSubscriptions активирует запланированные подписки, которые должны начаться сейчас
func (bas *BillingAutomationService) ActivateScheduledSubscriptions() error {
	// Получаем текущую дату и время
	now := time.Now()

	// Получаем подписки со статусом "scheduled", которые должны активироваться сейчас
	var subscriptions []models.Subscription
	if err := bas.db.Where("status = ? AND start_date <= ? AND admin_account_id = ?",
		"scheduled", now, bas.adminAccountID).
		Find(&subscriptions).Error; err != nil {
		return fmt.Errorf("ошибка получения запланированных подписок: %w", err)
	}

	if len(subscriptions) == 0 {
		return nil // Нет подписок для активации
	}

	activatedCount := 0
	errors := make([]error, 0)

	for _, subscription := range subscriptions {
		// Активируем подписку
		subscription.Status = "active"

		if err := bas.db.Save(&subscription).Error; err != nil {
			errors = append(errors, fmt.Errorf("ошибка активации подписки %d: %w", subscription.ID, err))
			continue
		}

		activatedCount++
	}

	if len(errors) > 0 {
		return fmt.Errorf("активация завершена с ошибками. Активировано подписок: %d, ошибок: %d. Первая ошибка: %v",
			activatedCount, len(errors), errors[0])
	}

	return nil
}

// GetInvoicesByPeriod получает счета за определенный период
func (bas *BillingAutomationService) GetInvoicesByPeriod(companyID *uint, startDate, endDate time.Time) ([]models.Invoice, error) {
	query := bas.db.Where("invoice_date >= ? AND invoice_date <= ? AND admin_account_id = ?", startDate, endDate, bas.adminAccountID).
		Preload("Contract", "admin_account_id = ?", bas.adminAccountID).
		Preload("TariffPlan", "admin_account_id = ?", bas.adminAccountID).
		Preload("Items").
		Order("invoice_date DESC")

	if companyID != nil {
		query = query.Where("company_id = ?", *companyID)
	}

	var invoices []models.Invoice
	if err := query.Find(&invoices).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения счетов за период: %w", err)
	}

	return invoices, nil
}

// GetBillingStatistics возвращает статистику биллинга
func (bas *BillingAutomationService) GetBillingStatistics(companyID uint, year int, month *int) (*BillingStatistics, error) {
	stats := &BillingStatistics{
		CompanyID: companyID,
		Year:      year,
		Month:     month,
	}

	// Определяем период
	var startDate, endDate time.Time
	if month != nil {
		startDate = time.Date(year, time.Month(*month), 1, 0, 0, 0, 0, time.UTC)
		endDate = startDate.AddDate(0, 1, -1).Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	} else {
		startDate = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		endDate = time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)
	}

	// Получаем счета за период
	var invoices []models.Invoice
	if err := bas.db.Where("company_id = ? AND invoice_date >= ? AND invoice_date <= ? AND admin_account_id = ?",
		companyID, startDate, endDate, bas.adminAccountID).Find(&invoices).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения счетов для статистики: %w", err)
	}

	// Подсчитываем статистику
	for _, invoice := range invoices {
		stats.TotalInvoices++
		stats.TotalAmount = stats.TotalAmount.Add(invoice.TotalAmount)

		switch invoice.Status {
		case "paid":
			stats.PaidInvoices++
			stats.PaidAmount = stats.PaidAmount.Add(invoice.TotalAmount)
		case "overdue":
			if invoice.IsOverdue() {
				stats.OverdueInvoices++
				stats.OverdueAmount = stats.OverdueAmount.Add(invoice.GetRemainingAmount())
			}
		case "cancelled":
			stats.CancelledInvoices++
		default:
			stats.PendingInvoices++
			stats.PendingAmount = stats.PendingAmount.Add(invoice.GetRemainingAmount())
		}
	}

	return stats, nil
}

// SendInvoiceReminders отправляет напоминания о счетах
func (bas *BillingAutomationService) SendInvoiceReminders() error {
	// Получаем настройки биллинга для всех компаний
	var settingsList []models.BillingSettings
	if err := bas.db.Where("admin_account_id = ?", bas.adminAccountID).Find(&settingsList).Error; err != nil {
		return fmt.Errorf("ошибка получения настроек биллинга: %w", err)
	}

	for _, settings := range settingsList {
		// Получаем счета, требующие напоминаний
		var invoices []models.Invoice

		// Счета, по которым скоро наступает срок оплаты
		dueSoonDate := time.Now().AddDate(0, 0, settings.NotifyBeforeDue)
		if err := bas.db.Where("company_id = ? AND due_date <= ? AND due_date > ? AND status NOT IN ? AND admin_account_id = ?",
			settings.CompanyID, dueSoonDate, time.Now(), []string{"paid", "cancelled"}, bas.adminAccountID).
			Find(&invoices).Error; err != nil {
			continue // Пропускаем эту компанию при ошибке
		}

		// Просроченные счета
		var overdueInvoices []models.Invoice
		overdueDate := time.Now().AddDate(0, 0, -settings.NotifyOverdue)
		if err := bas.db.Where("company_id = ? AND due_date < ? AND status NOT IN ? AND admin_account_id = ?",
			settings.CompanyID, overdueDate, []string{"paid", "cancelled"}, bas.adminAccountID).
			Find(&overdueInvoices).Error; err != nil {
			continue
		}

		invoices = append(invoices, overdueInvoices...)

		// Здесь должна быть логика отправки уведомлений
		// Пока что просто логируем
		if len(invoices) > 0 {
			fmt.Printf("Компания %d: найдено %d счетов для напоминаний\n", settings.CompanyID, len(invoices))

			// Создаем записи в истории о напоминаниях
			for _, invoice := range invoices {
				history := &models.BillingHistory{
					AdminAccountID: bas.adminAccountID,
					CompanyID:      invoice.CompanyID,
					InvoiceID:      &invoice.ID,
					ContractID:     invoice.ContractID,
					Operation:      "reminder_sent",
					Amount:         decimal.Zero,
					Currency:       invoice.Currency,
					Description:    fmt.Sprintf("Отправлено напоминание по счету %s", invoice.Number),
					Status:         "completed",
				}

				bas.db.Create(history) // Игнорируем ошибки для логирования
			}
		}
	}

	return nil
}

// UpdateInvoiceStatuses обновляет статусы счетов (например, помечает как просроченные)
func (bas *BillingAutomationService) UpdateInvoiceStatuses() error {
	// Помечаем просроченные счета
	if err := bas.db.Model(&models.Invoice{}).
		Where("due_date < ? AND status NOT IN ?", time.Now(), []string{"paid", "cancelled", "overdue"}).
		Update("status", "overdue").Error; err != nil {
		return fmt.Errorf("ошибка обновления статусов просроченных счетов: %w", err)
	}

	// Можно добавить другую логику обновления статусов
	return nil
}

// GenerateMonthlyReports генерирует месячные отчеты по биллингу
func (bas *BillingAutomationService) GenerateMonthlyReports(year int, month int) error {
	// Получаем все компании
	var companies []models.Company
	if err := bas.db.Find(&companies).Error; err != nil {
		return fmt.Errorf("ошибка получения списка компаний: %w", err)
	}

	for _, company := range companies {
		// Генерируем статистику для каждой компании
		// Временно пропускаем статистику из-за несовместимости типов
		// stats, err := bas.GetBillingStatistics(company.ID, year, &month)
		// Просто пропускаем эту компанию
		fmt.Printf("Пропускаем компанию %d из-за несовместимости типов\n", company.ID)
		continue

		// Создаем запись в истории о генерации отчета
		// Временно отключено из-за несовместимости типов
		/*
			history := &models.BillingHistory{
				CompanyID: company.ID,
				Operation: "monthly_report_generated",
				Amount:    stats.TotalAmount,
				Currency:  "RUB",
				Description: fmt.Sprintf("Сгенерирован месячный отчет за %d-%02d. Всего счетов: %d, сумма: %s",
					year, month, stats.TotalInvoices, stats.TotalAmount.String()),
				Status: "completed",
			}

			if err := bas.db.Create(history).Error; err != nil {
				fmt.Printf("Ошибка создания записи в истории для компании %d: %v\n", company.ID, err)
			}
		*/
	}

	return nil
}

// AutoRenewMonthlyContracts автоматически продлевает договоры с включенной автопролонгацией
// Продлевает договоры со статусом "active" и is_auto_renew = true
// Продлевает end_date на указанный период (contract_period_months или период из тарифа)
// если end_date уже наступил или близок (в течение 2 дней)
func (bas *BillingAutomationService) AutoRenewMonthlyContracts() error {
	now := time.Now()
	// Проверяем договоры, у которых end_date уже наступил или наступит в течение 2 дней
	renewalThreshold := now.AddDate(0, 0, 2)

	// Получаем все активные договоры с включенной автопролонгацией, которые нужно продлить
	var contracts []models.Contract
	if err := bas.db.Where("status = ? AND is_active = ? AND is_auto_renew = ? AND end_date <= ? AND admin_account_id = ?",
		"active", true, true, renewalThreshold, bas.adminAccountID).
		Preload("TariffPlan").
		Find(&contracts).Error; err != nil {
		return fmt.Errorf("ошибка получения договоров для пролонгации: %w", err)
	}

	if len(contracts) == 0 {
		return nil // Нет договоров для продления
	}

	renewedCount := 0
	skippedCount := 0
	errors := make([]error, 0)

	for _, contract := range contracts {
		// Проверяем, что у договора есть тарифный план
		if contract.TariffPlanID == nil || *contract.TariffPlanID == 0 {
			skippedCount++
			continue
		}

		// Проверяем, что договор еще активен (на случай, если статус изменился)
		if contract.Status != "active" || !contract.IsActive {
			skippedCount++
			continue
		}

		// Проверяем, включена ли автоматическая пролонгация
		if !contract.IsAutoRenew {
			skippedCount++
			log.Printf("⏭️ Договор %s (ID: %d) пропущен: автоматическая пролонгация отключена", contract.Number, contract.ID)
			continue
		}

		// Определяем период продления
		var renewalMonths int
		if contract.ContractPeriodMonths != nil && *contract.ContractPeriodMonths > 0 {
			// Используем период из договора
			renewalMonths = *contract.ContractPeriodMonths
		} else {
			// Используем период из тарифа
			if contract.TariffPlan.BillingPeriod == "monthly" {
				renewalMonths = 1
			} else if contract.TariffPlan.BillingPeriod == "yearly" {
				renewalMonths = 12
			} else {
				// Для one-time или других типов используем 1 месяц по умолчанию
				renewalMonths = 1
			}
		}

		// Продлеваем договор на указанное количество месяцев от текущей end_date
		newEndDate := contract.EndDate.AddDate(0, renewalMonths, 0)

		// Обновляем end_date договора
		if err := bas.db.Model(&contract).Update("end_date", newEndDate).Error; err != nil {
			errors = append(errors, fmt.Errorf("ошибка продления договора %s (ID: %d): %w", contract.Number, contract.ID, err))
			continue
		}

		// Создаем запись в истории биллинга о пролонгации
		history := &models.BillingHistory{
			AdminAccountID: bas.adminAccountID,
			CompanyID:      contract.CompanyID,
			ContractID:     &contract.ID,
			Operation:      "contract_auto_renewed",
			Amount:         decimal.Zero,
			Currency:       contract.Currency,
			Description:    fmt.Sprintf("Автоматическая пролонгация договора %s на %d месяц(а/ев). Новый срок действия: %s", contract.Number, renewalMonths, newEndDate.Format("02.01.2006")),
			Status:         "completed",
		}

		if err := bas.db.Create(history).Error; err != nil {
			// Логируем ошибку, но не прерываем процесс
			log.Printf("⚠️ Ошибка создания записи в истории для договора %d: %v", contract.ID, err)
		}

		renewedCount++
		log.Printf("✅ Договор %s (ID: %d) продлен до %s", contract.Number, contract.ID, newEndDate.Format("02.01.2006"))
	}

	if len(errors) > 0 {
		return fmt.Errorf("пролонгация завершена с ошибками. Продлено договоров: %d, пропущено: %d, ошибок: %d. Первая ошибка: %v",
			renewedCount, skippedCount, len(errors), errors[0])
	}

	if renewedCount > 0 {
		log.Printf("✅ Автоматическая пролонгация завершена. Продлено договоров: %d, пропущено: %d", renewedCount, skippedCount)
	}

	return nil
}
