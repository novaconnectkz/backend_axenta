package services

import (
	"fmt"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BillingService предоставляет функции для работы с биллингом
type BillingService struct {
	db *gorm.DB
}

// NewBillingService создает новый экземпляр BillingService
func NewBillingService() *BillingService {
	return &BillingService{
		db: database.DB,
	}
}

// BillingCalculationResult содержит результаты расчета биллинга
type BillingCalculationResult struct {
	CompanyID          uint              `json:"company_id"`
	ContractID         uint              `json:"contract_id"`
	TariffPlanID       uint              `json:"tariff_plan_id"`
	BillingPeriodStart time.Time         `json:"billing_period_start"`
	BillingPeriodEnd   time.Time         `json:"billing_period_end"`
	ActiveObjects      int               `json:"active_objects"`
	InactiveObjects    int               `json:"inactive_objects"`
	ScheduledDeletes   int               `json:"scheduled_deletes"`
	BaseAmount         decimal.Decimal   `json:"base_amount"`
	ObjectsAmount      decimal.Decimal   `json:"objects_amount"`
	DiscountAmount     decimal.Decimal   `json:"discount_amount"`
	SubtotalAmount     decimal.Decimal   `json:"subtotal_amount"`
	TaxAmount          decimal.Decimal   `json:"tax_amount"`
	TotalAmount        decimal.Decimal   `json:"total_amount"`
	Items              []InvoiceItemData `json:"items"`
}

// InvoiceItemData содержит данные для позиции счета
type InvoiceItemData struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	ItemType    string          `json:"item_type"`
	ObjectID    *uint           `json:"object_id"`
	Quantity    decimal.Decimal `json:"quantity"`
	UnitPrice   decimal.Decimal `json:"unit_price"`
	Amount      decimal.Decimal `json:"amount"`
	PeriodStart *time.Time      `json:"period_start"`
	PeriodEnd   *time.Time      `json:"period_end"`
}

// CalculateBillingForContract рассчитывает биллинг для конкретного договора
func (bs *BillingService) CalculateBillingForContract(contractID uint, periodStart, periodEnd time.Time) (*BillingCalculationResult, error) {
	// Получаем договор с тарифным планом
	var contract models.Contract
	if err := bs.db.Preload("TariffPlan").First(&contract, contractID).Error; err != nil {
		return nil, fmt.Errorf("договор не найден: %w", err)
	}

	// Получаем настройки биллинга для компании
	var settings models.BillingSettings
	if err := bs.db.Where("company_id = ?", contract.CompanyID).First(&settings).Error; err != nil {
		// Создаем настройки по умолчанию, если их нет
		settings = models.BillingSettings{
			CompanyID:               contract.CompanyID,
			DefaultTaxRate:          decimal.NewFromFloat(20),
			TaxIncluded:             false,
			EnableInactiveDiscounts: true,
			InactiveDiscountRatio:   decimal.NewFromFloat(0.5),
		}
		bs.db.Create(&settings)
	}

	// Получаем объекты по договору
	var objects []models.Object
	if err := bs.db.Where("contract_id = ?", contractID).Find(&objects).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения объектов: %w", err)
	}

	// Подсчитываем активные и неактивные объекты
	activeCount := 0
	inactiveCount := 0
	scheduledDeleteCount := 0

	for _, obj := range objects {
		if obj.ScheduledDeleteAt != nil && obj.ScheduledDeleteAt.Before(periodEnd) {
			scheduledDeleteCount++
			// Объекты с плановым удалением считаются неактивными
			inactiveCount++
		} else if obj.Status == "active" && obj.IsActive {
			activeCount++
		} else {
			inactiveCount++
		}
	}

	// Создаем результат расчета
	result := &BillingCalculationResult{
		CompanyID:          contract.CompanyID,
		ContractID:         contractID,
		TariffPlanID:       contract.TariffPlanID,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		ActiveObjects:      activeCount,
		InactiveObjects:    inactiveCount,
		ScheduledDeletes:   scheduledDeleteCount,
		Items:              make([]InvoiceItemData, 0),
	}

	// Получаем тарифный план
	tariffPlan := &models.TariffPlan{}
	if err := bs.db.First(tariffPlan, contract.TariffPlanID).Error; err != nil {
		return nil, fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Рассчитываем базовую стоимость подписки
	result.BaseAmount = decimal.NewFromFloat(tariffPlan.Price)
	if result.BaseAmount.GreaterThan(decimal.Zero) {
		result.Items = append(result.Items, InvoiceItemData{
			Name:        fmt.Sprintf("Подписка \"%s\"", tariffPlan.Name),
			Description: fmt.Sprintf("Период: %s - %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
			ItemType:    "subscription",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   result.BaseAmount,
			Amount:      result.BaseAmount,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		})
	}

	// Рассчитываем стоимость объектов
	result.ObjectsAmount = bs.calculateObjectsAmount(tariffPlan, activeCount, inactiveCount, settings.EnableInactiveDiscounts)

	// Добавляем позиции для активных объектов
	if activeCount > tariffPlan.FreeObjectsCount {
		billableActive := activeCount - tariffPlan.FreeObjectsCount
		if billableActive > 0 {
			activeAmount := tariffPlan.PricePerObject.Mul(decimal.NewFromInt(int64(billableActive)))
			result.Items = append(result.Items, InvoiceItemData{
				Name:        "Активные объекты мониторинга",
				Description: fmt.Sprintf("Количество: %d объектов", billableActive),
				ItemType:    "object",
				Quantity:    decimal.NewFromInt(int64(billableActive)),
				UnitPrice:   tariffPlan.PricePerObject,
				Amount:      activeAmount,
				PeriodStart: &periodStart,
				PeriodEnd:   &periodEnd,
			})
		}
	}

	// Добавляем позиции для неактивных объектов (со скидкой)
	if inactiveCount > 0 && settings.EnableInactiveDiscounts {
		inactivePrice := tariffPlan.PricePerObject.Mul(settings.InactiveDiscountRatio)
		inactiveAmount := inactivePrice.Mul(decimal.NewFromInt(int64(inactiveCount)))
		result.Items = append(result.Items, InvoiceItemData{
			Name:        "Неактивные объекты мониторинга",
			Description: fmt.Sprintf("Количество: %d объектов (льготный тариф)", inactiveCount),
			ItemType:    "object",
			Quantity:    decimal.NewFromInt(int64(inactiveCount)),
			UnitPrice:   inactivePrice,
			Amount:      inactiveAmount,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		})
	}

	// Применяем скидку тарифного плана
	result.DiscountAmount = decimal.Zero
	if tariffPlan.DiscountPercent.GreaterThan(decimal.Zero) {
		subtotal := result.BaseAmount.Add(result.ObjectsAmount)
		result.DiscountAmount = subtotal.Mul(tariffPlan.DiscountPercent).Div(decimal.NewFromInt(100))

		result.Items = append(result.Items, InvoiceItemData{
			Name:        fmt.Sprintf("Скидка %s%%", tariffPlan.DiscountPercent.String()),
			Description: "Скидка по тарифному плану",
			ItemType:    "discount",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   result.DiscountAmount.Neg(),
			Amount:      result.DiscountAmount.Neg(),
		})
	}

	// Рассчитываем промежуточную сумму
	result.SubtotalAmount = result.BaseAmount.Add(result.ObjectsAmount).Sub(result.DiscountAmount)

	// Рассчитываем налог
	if !settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
		result.TaxAmount = result.SubtotalAmount.Mul(settings.DefaultTaxRate).Div(decimal.NewFromInt(100))
	}

	// Рассчитываем итоговую сумму
	result.TotalAmount = result.SubtotalAmount.Add(result.TaxAmount)

	return result, nil
}

// calculateObjectsAmount рассчитывает стоимость объектов с учетом льгот
func (bs *BillingService) calculateObjectsAmount(tariff *models.TariffPlan, activeCount, inactiveCount int, enableDiscounts bool) decimal.Decimal {
	totalObjects := activeCount + inactiveCount

	if totalObjects <= tariff.FreeObjectsCount {
		return decimal.Zero
	}

	// Количество объектов к оплате
	billableObjects := totalObjects - tariff.FreeObjectsCount

	// Сначала списываем с активных объектов
	billableActive := activeCount
	billableInactive := inactiveCount

	if billableActive > billableObjects {
		billableActive = billableObjects
		billableInactive = 0
	} else if billableActive+billableInactive > billableObjects {
		billableInactive = billableObjects - billableActive
	}

	// Рассчитываем стоимость активных объектов
	activeAmount := tariff.PricePerObject.Mul(decimal.NewFromInt(int64(billableActive)))

	// Рассчитываем стоимость неактивных объектов со скидкой
	inactiveAmount := decimal.Zero
	if billableInactive > 0 && enableDiscounts {
		inactivePrice := tariff.PricePerObject.Mul(tariff.InactivePriceRatio)
		inactiveAmount = inactivePrice.Mul(decimal.NewFromInt(int64(billableInactive)))
	} else if billableInactive > 0 {
		// Если скидки отключены, считаем по полной цене
		inactiveAmount = tariff.PricePerObject.Mul(decimal.NewFromInt(int64(billableInactive)))
	}

	return activeAmount.Add(inactiveAmount)
}

// GenerateInvoiceForContract создает счет для договора
func (bs *BillingService) GenerateInvoiceForContract(contractID uint, periodStart, periodEnd time.Time) (*models.Invoice, error) {
	// Рассчитываем биллинг
	calculation, err := bs.CalculateBillingForContract(contractID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("ошибка расчета биллинга: %w", err)
	}

	// Получаем настройки биллинга
	var settings models.BillingSettings
	if err := bs.db.Where("company_id = ?", calculation.CompanyID).First(&settings).Error; err != nil {
		return nil, fmt.Errorf("настройки биллинга не найдены: %w", err)
	}

	// Генерируем номер счета
	var lastInvoice models.Invoice
	var sequenceNumber int
	if err := bs.db.Where("company_id = ? AND invoice_date >= ?", calculation.CompanyID, time.Now().Truncate(24*time.Hour).AddDate(0, 0, -time.Now().Day()+1)).
		Order("id DESC").First(&lastInvoice).Error; err == nil {
		sequenceNumber = int(lastInvoice.ID) + 1
	} else {
		sequenceNumber = 1
	}

	invoiceNumber := settings.GetInvoiceNumber(sequenceNumber)

	// Создаем счет
	invoice := &models.Invoice{
		Number:             invoiceNumber,
		Title:              fmt.Sprintf("Счет за услуги мониторинга за период %s - %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
		Description:        fmt.Sprintf("Оплата услуг по договору за период с %s по %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
		InvoiceDate:        time.Now(),
		DueDate:            time.Now().AddDate(0, 0, settings.InvoicePaymentTermDays),
		CompanyID:          calculation.CompanyID,
		ContractID:         &calculation.ContractID,
		TariffPlanID:       calculation.TariffPlanID,
		BillingPeriodStart: calculation.BillingPeriodStart,
		BillingPeriodEnd:   calculation.BillingPeriodEnd,
		SubtotalAmount:     calculation.SubtotalAmount,
		TaxRate:            settings.DefaultTaxRate,
		TaxAmount:          calculation.TaxAmount,
		TotalAmount:        calculation.TotalAmount,
		Currency:           settings.Currency,
		Status:             "draft",
	}

	// Сохраняем счет
	if err := bs.db.Create(invoice).Error; err != nil {
		return nil, fmt.Errorf("ошибка создания счета: %w", err)
	}

	// Создаем позиции счета
	for _, itemData := range calculation.Items {
		item := &models.InvoiceItem{
			InvoiceID:   invoice.ID,
			Name:        itemData.Name,
			Description: itemData.Description,
			ItemType:    itemData.ItemType,
			ObjectID:    itemData.ObjectID,
			Quantity:    itemData.Quantity,
			UnitPrice:   itemData.UnitPrice,
			Amount:      itemData.Amount,
			PeriodStart: itemData.PeriodStart,
			PeriodEnd:   itemData.PeriodEnd,
		}

		if err := bs.db.Create(item).Error; err != nil {
			return nil, fmt.Errorf("ошибка создания позиции счета: %w", err)
		}
	}

	// Создаем запись в истории биллинга
	history := &models.BillingHistory{
		CompanyID:   calculation.CompanyID,
		InvoiceID:   &invoice.ID,
		ContractID:  &calculation.ContractID,
		Operation:   "invoice_created",
		Amount:      calculation.TotalAmount,
		Currency:    settings.Currency,
		Description: fmt.Sprintf("Создан счет %s на сумму %s %s", invoice.Number, calculation.TotalAmount.String(), settings.Currency),
		PeriodStart: &calculation.BillingPeriodStart,
		PeriodEnd:   &calculation.BillingPeriodEnd,
		Status:      "completed",
	}

	if err := bs.db.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		fmt.Printf("Предупреждение: ошибка создания записи в истории биллинга: %v\n", err)
	}

	// Загружаем созданный счет с позициями
	if err := bs.db.Preload("Items").Preload("Contract").Preload("TariffPlan").First(invoice, invoice.ID).Error; err != nil {
		return nil, fmt.Errorf("ошибка загрузки созданного счета: %w", err)
	}

	return invoice, nil
}

// ProcessPayment обрабатывает платеж по счету
func (bs *BillingService) ProcessPayment(invoiceID uint, amount decimal.Decimal, paymentMethod string, notes string) error {
	// Получаем счет
	var invoice models.Invoice
	if err := bs.db.First(&invoice, invoiceID).Error; err != nil {
		return fmt.Errorf("счет не найден: %w", err)
	}

	// Проверяем, что счет не отменен
	if invoice.Status == "cancelled" {
		return fmt.Errorf("нельзя провести платеж по отмененному счету")
	}

	// Обновляем сумму оплаты
	newPaidAmount := invoice.PaidAmount.Add(amount)

	// Определяем новый статус
	newStatus := invoice.Status
	var paidAt *time.Time

	if newPaidAmount.GreaterThanOrEqual(invoice.TotalAmount) {
		newStatus = "paid"
		now := time.Now()
		paidAt = &now
	} else if newPaidAmount.GreaterThan(decimal.Zero) {
		newStatus = "partially_paid"
	}

	// Обновляем счет
	updates := map[string]interface{}{
		"paid_amount": newPaidAmount,
		"status":      newStatus,
	}

	if paidAt != nil {
		updates["paid_at"] = paidAt
	}

	if err := bs.db.Model(&invoice).Updates(updates).Error; err != nil {
		return fmt.Errorf("ошибка обновления счета: %w", err)
	}

	// Создаем запись в истории биллинга
	history := &models.BillingHistory{
		CompanyID:   invoice.CompanyID,
		InvoiceID:   &invoice.ID,
		ContractID:  invoice.ContractID,
		Operation:   "payment_received",
		Amount:      amount,
		Currency:    invoice.Currency,
		Description: fmt.Sprintf("Получен платеж по счету %s на сумму %s %s. Способ оплаты: %s", invoice.Number, amount.String(), invoice.Currency, paymentMethod),
		Status:      "completed",
	}

	if notes != "" {
		history.Description += fmt.Sprintf(". Примечания: %s", notes)
	}

	if err := bs.db.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		fmt.Printf("Предупреждение: ошибка создания записи в истории биллинга: %v\n", err)
	}

	return nil
}

// GetBillingHistory возвращает историю биллинга для компании
func (bs *BillingService) GetBillingHistory(companyID uint, limit, offset int) ([]models.BillingHistory, int64, error) {
	var history []models.BillingHistory
	var total int64

	// Подсчитываем общее количество записей
	if err := bs.db.Model(&models.BillingHistory{}).Where("company_id = ?", companyID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчета записей истории: %w", err)
	}

	// Получаем записи с пагинацией
	query := bs.db.Where("company_id = ?", companyID).
		Preload("Invoice").
		Preload("Contract").
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&history).Error; err != nil {
		return nil, 0, fmt.Errorf("ошибка получения истории биллинга: %w", err)
	}

	return history, total, nil
}

// GetOverdueInvoices возвращает просроченные счета
func (bs *BillingService) GetOverdueInvoices(companyID *uint) ([]models.Invoice, error) {
	query := bs.db.Where("due_date < ? AND status NOT IN ?", time.Now(), []string{"paid", "cancelled"}).
		Preload("Contract").
		Preload("TariffPlan").
		Order("due_date ASC")

	if companyID != nil {
		query = query.Where("company_id = ?", *companyID)
	}

	var invoices []models.Invoice
	if err := query.Find(&invoices).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения просроченных счетов: %w", err)
	}

	return invoices, nil
}

// CancelInvoice отменяет счет
func (bs *BillingService) CancelInvoice(invoiceID uint, reason string) error {
	// Получаем счет
	var invoice models.Invoice
	if err := bs.db.First(&invoice, invoiceID).Error; err != nil {
		return fmt.Errorf("счет не найден: %w", err)
	}

	// Проверяем, что счет можно отменить
	if invoice.Status == "paid" {
		return fmt.Errorf("нельзя отменить оплаченный счет")
	}

	if invoice.Status == "cancelled" {
		return fmt.Errorf("счет уже отменен")
	}

	// Обновляем статус счета
	if err := bs.db.Model(&invoice).Updates(map[string]interface{}{
		"status": "cancelled",
		"notes":  reason,
	}).Error; err != nil {
		return fmt.Errorf("ошибка отмены счета: %w", err)
	}

	// Создаем запись в истории биллинга
	history := &models.BillingHistory{
		CompanyID:   invoice.CompanyID,
		InvoiceID:   &invoice.ID,
		ContractID:  invoice.ContractID,
		Operation:   "invoice_cancelled",
		Amount:      invoice.TotalAmount,
		Currency:    invoice.Currency,
		Description: fmt.Sprintf("Отменен счет %s. Причина: %s", invoice.Number, reason),
		Status:      "completed",
	}

	if err := bs.db.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		fmt.Printf("Предупреждение: ошибка создания записи в истории биллинга: %v\n", err)
	}

	return nil
}
