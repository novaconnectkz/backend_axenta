package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"backend_axenta/database"
	"backend_axenta/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// BillingService предоставляет функции для работы с биллингом
type BillingService struct {
	db             *gorm.DB
	adminAccountID uint
}

// NewBillingService создает новый экземпляр BillingService
func NewBillingService(adminAccountID uint) *BillingService {
	return &BillingService{
		db:             database.DB,
		adminAccountID: adminAccountID,
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
	return bs.CalculateBillingForContractWithTenantDB(contractID, periodStart, periodEnd, bs.db)
}

// CalculateBillingForContractWithTenantDB рассчитывает биллинг для конкретного договора с использованием tenant DB
func (bs *BillingService) CalculateBillingForContractWithTenantDB(contractID uint, periodStart, periodEnd time.Time, tenantDB *gorm.DB) (*BillingCalculationResult, error) {
	// Получаем договор из tenant DB (без admin_account_id, так как в tenant схеме его нет)
	var contract models.Contract
	if err := tenantDB.
		Where("id = ?", contractID).
		First(&contract).Error; err != nil {
		return nil, fmt.Errorf("договор не найден: %w", err)
	}

	// Проверяем тип договора
	if contract.ContractType == "partner" {
		// Для партнерских договоров используем специальную логику
		return bs.CalculateBillingForPartnerContract(&contract, periodStart, periodEnd, tenantDB)
	}

	// Загружаем тарифный план из public схемы
	if contract.TariffPlanID != nil && *contract.TariffPlanID > 0 {
		publicDB := bs.db.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
			log.Printf("⚠️ Не удалось переключиться на public: %v", err)
		}

		var tariffPlan models.BillingPlan
		if err := publicDB.
			Where("id = ? AND admin_account_id = ?", *contract.TariffPlanID, bs.adminAccountID).
			First(&tariffPlan).Error; err == nil {
			contract.TariffPlan = tariffPlan
		}
	}

	// Получаем настройки биллинга для компании из public схемы
	publicDB := bs.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на public: %v", err)
	}

	var settings models.BillingSettings
	if err := publicDB.
		Where("company_id = ? AND admin_account_id = ?", contract.CompanyID, bs.adminAccountID).
		First(&settings).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("ошибка получения настроек биллинга: %w", err)
		}

		// Пробуем найти настройки для компании без учета admin_account_id
		var companySettings models.BillingSettings
		if err := publicDB.Where("company_id = ?", contract.CompanyID).First(&companySettings).Error; err == nil {
			if companySettings.AdminAccountID != bs.adminAccountID {
				companySettings.AdminAccountID = bs.adminAccountID
				if saveErr := publicDB.Save(&companySettings).Error; saveErr != nil {
					return nil, fmt.Errorf("ошибка обновления настроек биллинга: %w", saveErr)
				}
			}
			settings = companySettings
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// Создаем настройки по умолчанию, если их нет вообще
			settings = models.BillingSettings{
				AdminAccountID:          bs.adminAccountID,
				CompanyID:               contract.CompanyID,
				DefaultTaxRate:          decimal.NewFromFloat(20),
				TaxIncluded:             false,
				EnableInactiveDiscounts: true,
				InactiveDiscountRatio:   decimal.NewFromFloat(0.5),
				MinDaysForFullMonth:     5,
			}
			if createErr := publicDB.Create(&settings).Error; createErr != nil {
				if isUniqueConstraintError(createErr) {
					if err := publicDB.Where("company_id = ?", contract.CompanyID).First(&companySettings).Error; err == nil {
						if companySettings.AdminAccountID != bs.adminAccountID {
							companySettings.AdminAccountID = bs.adminAccountID
							if saveErr := publicDB.Save(&companySettings).Error; saveErr != nil {
								return nil, fmt.Errorf("ошибка обновления настроек биллинга: %w", saveErr)
							}
						}
						settings = companySettings
					} else {
						return nil, fmt.Errorf("ошибка получения настроек биллинга после конфликта: %w", err)
					}
				} else {
					return nil, fmt.Errorf("ошибка создания настроек биллинга: %w", createErr)
				}
			}
		} else {
			return nil, fmt.Errorf("ошибка получения настроек биллинга: %w", err)
		}
	}

	// Получаем настройки НДС из таблицы companies
	var company models.Company
	if err := publicDB.Where("id = ?", contract.CompanyID).First(&company).Error; err == nil {
		// Используем настройки НДС из компании, если они есть
		if company.DefaultTaxRate.GreaterThan(decimal.Zero) {
			settings.DefaultTaxRate = company.DefaultTaxRate
			settings.TaxIncluded = company.TaxIncluded
			log.Printf("✅ [CalculateBilling] Используем настройки НДС из компании: rate=%s%%, included=%v", company.DefaultTaxRate.StringFixed(2), company.TaxIncluded)
		}
	}

	// Получаем объекты по договору из tenant DB
	var objects []models.Object
	if err := tenantDB.Where("contract_id = ?", contractID).Find(&objects).Error; err != nil {
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

	// Проверяем, что у договора есть тарифный план
	if contract.TariffPlanID == nil {
		return nil, fmt.Errorf("у договора не указан тарифный план")
	}

	// Создаем результат расчета
	result := &BillingCalculationResult{
		CompanyID:          contract.CompanyID,
		ContractID:         contractID,
		TariffPlanID:       *contract.TariffPlanID,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		ActiveObjects:      activeCount,
		InactiveObjects:    inactiveCount,
		ScheduledDeletes:   scheduledDeleteCount,
		Items:              make([]InvoiceItemData, 0),
	}

	// Получаем тарифный план из public схемы
	publicDB2 := bs.db.Session(&gorm.Session{})
	if err := publicDB2.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на public для тарифного плана: %v", err)
	}

	tariffPlan := &models.BillingPlan{}
	if err := publicDB2.
		Where("id = ? AND admin_account_id = ?", contract.TariffPlanID, bs.adminAccountID).
		First(tariffPlan).Error; err != nil {
		return nil, fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Рассчитываем базовую стоимость подписки
	result.BaseAmount = tariffPlan.Price
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
	// Для BillingPlan используем упрощенный расчет (без дополнительных полей из TariffPlan)
	result.ObjectsAmount = decimal.Zero

	// Добавляем позиции для объектов (если есть)
	if activeCount > 0 {
		// По умолчанию все объекты бесплатны если нет специфичного тарифного плана
		log.Printf("ℹ️ Активных объектов: %d (без дополнительной платы в базовом тарифе)", activeCount)
	}

	// Применяем скидки из таблицы discounts (по уровням иерархии)
	discountService := NewDiscountService()
	ctx := context.Background()

	// Получаем ID для применения скидок
	var objectIDs []*uint
	for i := range objects {
		if objects[i].ID > 0 {
			objID := objects[i].ID
			objectIDs = append(objectIDs, &objID) // nolint:staticcheck
		}
	}

	// Применяем скидки (пока только на уровне договора и тарифа)
	subtotal := result.BaseAmount.Add(result.ObjectsAmount)
	discountedAmount, discountApplications, err := discountService.ApplyDiscounts(
		ctx,
		subtotal,
		nil,                   // objectID - можно добавить по объектам отдельно
		contract.TariffPlanID, // tariffID
		nil,                   // subscriptionID
		nil,                   // appendixID
		&contractID,           // contractID
		periodStart,
	)

	if err != nil {
		// Логируем ошибку, но продолжаем без скидок
		fmt.Printf("⚠️ Ошибка применения скидок: %v\n", err)
		discountedAmount = subtotal
	}

	result.DiscountAmount = subtotal.Sub(discountedAmount)

	// Добавляем позиции скидок в счет
	for _, app := range discountApplications {
		result.Items = append(result.Items, InvoiceItemData{
			Name:        fmt.Sprintf("Скидка (%s)", app.Level),
			Description: fmt.Sprintf("%s: %s", app.Type, app.Reason),
			ItemType:    "discount",
			Quantity:    decimal.NewFromInt(1),
			UnitPrice:   app.Amount.Neg(),
			Amount:      app.Amount.Neg(),
		})
	}

	// Также применяем старую скидку тарифного плана (для обратной совместимости)
	// Примечание: BillingPlan не имеет поля DiscountPercent, пропускаем этот расчет
	// if tariffPlan.DiscountPercent.GreaterThan(decimal.Zero) { ... }

	// Проверяем минимальную оплату (min_commit) для тарифных компонентов из public схемы
	var tariffComponents []models.TariffComponent
	if err := publicDB2.Where("tariff_plan_id = ? AND admin_account_id = ? AND is_active = true AND deleted_at IS NULL", contract.TariffPlanID, bs.adminAccountID).
		Find(&tariffComponents).Error; err == nil {

		for _, component := range tariffComponents {
			if component.MinCommit.GreaterThan(decimal.Zero) {
				// Находим позиции, связанные с этим компонентом
				componentAmount := decimal.Zero
				for _, item := range result.Items {
					// TODO: Добавить связь между InvoiceItem и TariffComponent
					// Пока считаем для всех позиций типа recurring
					if item.ItemType == "subscription" || item.ItemType == "recurring" {
						componentAmount = componentAmount.Add(item.Amount)
					}
				}

				// Если сумма меньше минимальной оплаты, добавляем строку "Минимальная оплата"
				if componentAmount.LessThan(component.MinCommit) {
					minCommitDiff := component.MinCommit.Sub(componentAmount)
					result.Items = append(result.Items, InvoiceItemData{
						Name:        fmt.Sprintf("Минимальная оплата: %s", component.Name),
						Description: fmt.Sprintf("Минимальная оплата за %s (недобор: %s)", component.Name, minCommitDiff.String()),
						ItemType:    "min_commit",
						Quantity:    decimal.NewFromInt(1),
						UnitPrice:   minCommitDiff,
						Amount:      minCommitDiff,
						PeriodStart: &periodStart,
						PeriodEnd:   &periodEnd,
					})
					discountedAmount = discountedAmount.Add(minCommitDiff)
				}
			}
		}
	}

	// Рассчитываем промежуточную сумму
	result.SubtotalAmount = discountedAmount

	// Рассчитываем налог
	if settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
		// НДС выделяется из суммы (включен в цену)
		// Формула: TaxAmount = SubtotalAmount * TaxRate / (100 + TaxRate)
		// Сумма без НДС = SubtotalAmount - TaxAmount
		taxRateDivisor := decimal.NewFromInt(100).Add(settings.DefaultTaxRate)
		result.TaxAmount = result.SubtotalAmount.Mul(settings.DefaultTaxRate).Div(taxRateDivisor)
		result.TotalAmount = result.SubtotalAmount
		result.SubtotalAmount = result.TotalAmount.Sub(result.TaxAmount)
	} else if !settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
		// НДС начисляется сверху
		result.TaxAmount = result.SubtotalAmount.Mul(settings.DefaultTaxRate).Div(decimal.NewFromInt(100))
		result.TotalAmount = result.SubtotalAmount.Add(result.TaxAmount)
	} else {
		// Налог не применяется
		result.TaxAmount = decimal.Zero
		result.TotalAmount = result.SubtotalAmount
	}

	return result, nil
}

// CalculateBillingForPartnerContract рассчитывает биллинг для партнерского договора
// Для партнерских договоров:
// - Используются ежедневные снимки объектов (partner_daily_snapshots)
// - Стоимость = сумма daily_cost из всех снимков за период
// - Формула для каждого дня: (тариф/30) * количество_активных_объектов
// - Автопилот не применяется
func (bs *BillingService) CalculateBillingForPartnerContract(contract *models.Contract, periodStart, periodEnd time.Time, tenantDB *gorm.DB) (*BillingCalculationResult, error) {
	log.Printf("🔧 Расчет биллинга для партнерского договора %d за период %s - %s",
		contract.ID, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))

	// Проверяем, что указан partner_company_id
	if contract.PartnerCompanyID == nil {
		return nil, fmt.Errorf("для партнерского договора не указан partner_company_id")
	}

	// Проверяем, что у договора есть тарифный план
	if contract.TariffPlanID == nil {
		return nil, fmt.Errorf("у партнерского договора не указан тарифный план")
	}

	// Загружаем тарифный план из public схемы
	publicDB := bs.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на public: %v", err)
	}

	var tariffPlan models.BillingPlan
	if err := publicDB.
		Where("id = ? AND admin_account_id = ?", *contract.TariffPlanID, bs.adminAccountID).
		First(&tariffPlan).Error; err != nil {
		return nil, fmt.Errorf("тарифный план не найден: %w", err)
	}

	// Получаем настройки биллинга для компании
	var settings models.BillingSettings
	if err := publicDB.
		Where("company_id = ? AND admin_account_id = ?", contract.CompanyID, bs.adminAccountID).
		First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Создаем настройки по умолчанию
			settings = models.BillingSettings{
				AdminAccountID:          bs.adminAccountID,
				CompanyID:               contract.CompanyID,
				DefaultTaxRate:          decimal.NewFromFloat(20),
				TaxIncluded:             false,
				EnableInactiveDiscounts: false, // Для партнеров не применяем скидки на неактивные объекты
				InactiveDiscountRatio:   decimal.Zero,
				MinDaysForFullMonth:     5,
			}
			if createErr := publicDB.Create(&settings).Error; createErr != nil {
				log.Printf("⚠️ Ошибка создания настроек биллинга: %v", createErr)
			}
		} else {
			return nil, fmt.Errorf("ошибка получения настроек биллинга: %w", err)
		}
	}

	// Получаем настройки НДС из таблицы companies
	var company models.Company
	if err := publicDB.Where("id = ?", contract.CompanyID).First(&company).Error; err == nil {
		if company.DefaultTaxRate.GreaterThan(decimal.Zero) {
			settings.DefaultTaxRate = company.DefaultTaxRate
			settings.TaxIncluded = company.TaxIncluded
			log.Printf("✅ Используем настройки НДС из компании: rate=%s%%, included=%v", company.DefaultTaxRate.StringFixed(2), company.TaxIncluded)
		}
	}

	// Ф1: снимки читаем из ТЕНАНТ-схемы договора (там пишут все 4 источника),
	// а не из public (исторически протухшая копия). Резолвим схему ЯВНО и fail-closed:
	// GetTenantDBByID на ошибке вернул бы main-пул (public) → тихий недосчёт по протухшим
	// данным. Здесь — ошибка вместо молчаливого чтения public (Codex critical).
	var comp struct {
		DatabaseSchema string `gorm:"column:database_schema"`
	}
	if err := bs.db.Table("public.companies").Select("database_schema").
		Where("id = ?", contract.CompanyID).First(&comp).Error; err != nil {
		return nil, fmt.Errorf("billing-gate: компания %d не найдена для резолва схемы: %w", contract.CompanyID, err)
	}
	schema := comp.DatabaseSchema
	if schema == "" {
		schema = fmt.Sprintf("tenant_%d", contract.CompanyID)
	}
	snapDB, err := database.ConnectToTenant(schema)
	if err != nil {
		return nil, fmt.Errorf("billing-gate: не удалось подключиться к tenant-схеме %s: %w", schema, err)
	}

	// Ф1 billing-gate: в расчёт идут ТОЛЬКО доверенные снимки (verified/manual_approved).
	// needs_review/estimated в начисление НЕ попадают (тихий недосчёт исключён —
	// заблокированные дни видны отдельно + алерт), что для денег безопаснее.
	var snapshots []models.PartnerDailySnapshot
	if err := snapDB.
		Where("contract_id = ? AND snapshot_date >= ? AND snapshot_date <= ?", contract.ID, periodStart, periodEnd).
		Where("verify_status IN ?", []string{models.VerifyStatusVerified, models.VerifyStatusApproved}).
		Order("snapshot_date ASC").
		Find(&snapshots).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения снимков: %w", err)
	}

	// Заблокированные гейтом снимки (для наблюдаемости/алертов) — НЕ суммируются.
	var blocked []models.PartnerDailySnapshot
	if err := snapDB.
		Where("contract_id = ? AND snapshot_date >= ? AND snapshot_date <= ?", contract.ID, periodStart, periodEnd).
		Where("verify_status NOT IN ?", []string{models.VerifyStatusVerified, models.VerifyStatusApproved}).
		Order("snapshot_date ASC").
		Find(&blocked).Error; err == nil && len(blocked) > 0 {
		blockedRisk := decimal.Zero
		for _, b := range blocked {
			blockedRisk = blockedRisk.Add(b.AmountAtRisk)
		}
		log.Printf("🚧 billing-gate: договор %d — заблокировано снимков=%d (₽ под риском=%s), требуют ручного подтверждения",
			contract.ID, len(blocked), blockedRisk.StringFixed(2))
	}

	// Missing-day: видимость тихого недосчёта (день без строки вообще). present = доверенные+
	// заблокированные; expected = дней в периоде. Полная readiness-таблица — Ф2.
	expectedDays := int(periodEnd.Sub(periodStart).Hours()/24) + 1
	present := len(snapshots) + len(blocked)
	if expectedDays > 0 && present < expectedDays {
		log.Printf("⚠️ billing-gate: договор %d — пропущено дней без снимка=%d из %d (период %s–%s) — недосчёт не покрыт начислением",
			contract.ID, expectedDays-present, expectedDays,
			periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))
	}

	log.Printf("📊 Найдено доверенных снимков для договора за период: %d", len(snapshots))

	if len(snapshots) == 0 {
		log.Printf("⚠️ Нет снимков для партнерского договора %d за период %s - %s",
			contract.ID, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))
		// Возвращаем нулевую стоимость, если нет снимков
		return &BillingCalculationResult{
			CompanyID:          contract.CompanyID,
			ContractID:         contract.ID,
			TariffPlanID:       *contract.TariffPlanID,
			BillingPeriodStart: periodStart,
			BillingPeriodEnd:   periodEnd,
			ActiveObjects:      0,
			InactiveObjects:    0,
			ScheduledDeletes:   0,
			Items:              make([]InvoiceItemData, 0),
			ObjectsAmount:      decimal.Zero,
			BaseAmount:         decimal.Zero,
			SubtotalAmount:     decimal.Zero,
			TaxAmount:          decimal.Zero,
			TotalAmount:        decimal.Zero,
		}, nil
	}

	// Рассчитываем общую стоимость из снимков
	totalCost := decimal.Zero
	totalObjectsDays := 0 // Сумма объектов за все дни
	daysCount := len(snapshots)

	for _, snapshot := range snapshots {
		totalCost = totalCost.Add(snapshot.DailyCost)
		totalObjectsDays += snapshot.ActiveObjectsCount
	}

	// Средне количество объектов за период
	avgObjectsCount := 0
	if daysCount > 0 {
		avgObjectsCount = totalObjectsDays / daysCount
	}

	log.Printf("💰 Расчет для партнерского договора: снимков=%d, средне объектов=%d, общая стоимость=%s₽",
		daysCount, avgObjectsCount, totalCost.StringFixed(2))

	// Создаем результат расчета на основе снимков
	result := &BillingCalculationResult{
		CompanyID:          contract.CompanyID,
		ContractID:         contract.ID,
		TariffPlanID:       *contract.TariffPlanID,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		ActiveObjects:      avgObjectsCount,
		InactiveObjects:    0, // Для партнерских договоров не учитываем неактивные
		ScheduledDeletes:   0,
		Items:              make([]InvoiceItemData, 0),
		ObjectsAmount:      totalCost,
		BaseAmount:         decimal.Zero, // Нет базовой подписки для партнеров
	}

	// Добавляем позицию в счет
	if totalCost.GreaterThan(decimal.Zero) {
		result.Items = append(result.Items, InvoiceItemData{
			Name:        "Тарификация объектов партнера",
			Description: fmt.Sprintf("Среднее количество объектов: %d, период: %s - %s (%d дн.)", avgObjectsCount, periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006"), daysCount),
			ItemType:    "partner_objects",
			Quantity:    decimal.NewFromInt(int64(avgObjectsCount)),
			UnitPrice:   tariffPlan.Price.Div(decimal.NewFromInt(30)), // Дневная цена
			Amount:      totalCost,
			PeriodStart: &periodStart,
			PeriodEnd:   &periodEnd,
		})
	}

	// Для партнерских договоров НЕ применяем скидки и автопилот
	result.DiscountAmount = decimal.Zero

	// Рассчитываем промежуточную сумму
	result.SubtotalAmount = totalCost

	// Рассчитываем налог
	if settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
		// НДС выделяется из суммы (включен в цену)
		taxRateDivisor := decimal.NewFromInt(100).Add(settings.DefaultTaxRate)
		result.TaxAmount = result.SubtotalAmount.Mul(settings.DefaultTaxRate).Div(taxRateDivisor)
		result.TotalAmount = result.SubtotalAmount
		result.SubtotalAmount = result.TotalAmount.Sub(result.TaxAmount)
	} else if !settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
		// НДС начисляется сверху
		result.TaxAmount = result.SubtotalAmount.Mul(settings.DefaultTaxRate).Div(decimal.NewFromInt(100))
		result.TotalAmount = result.SubtotalAmount.Add(result.TaxAmount)
	} else {
		// Налог не применяется
		result.TaxAmount = decimal.Zero
		result.TotalAmount = result.SubtotalAmount
	}

	log.Printf("💰 Партнерский договор: средне объектов=%d, снимков=%d, сумма=%s", avgObjectsCount, daysCount, result.TotalAmount.String())

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
	return bs.GenerateInvoiceForContractWithTenantDB(contractID, periodStart, periodEnd, bs.db, nil)
}

// GenerateInvoiceForContractWithTenantDB создает счет для договора с использованием tenant DB
func (bs *BillingService) GenerateInvoiceForContractWithTenantDB(contractID uint, periodStart, periodEnd time.Time, tenantDB *gorm.DB, customAmount *float64) (*models.Invoice, error) {
	// Рассчитываем биллинг с tenant DB
	calculation, err := bs.CalculateBillingForContractWithTenantDB(contractID, periodStart, periodEnd, tenantDB)
	if err != nil {
		return nil, fmt.Errorf("ошибка расчета биллинга: %w", err)
	}

	// Если задана ручная сумма для hourly/daily/weekly тарифов, переопределяем расчетную сумму
	if customAmount != nil && *customAmount > 0 {
		customAmountDecimal := decimal.NewFromFloat(*customAmount)
		log.Printf("💰 Используется ручная сумма: %.2f (вместо расчетной %s)",
			*customAmount, calculation.TotalAmount.StringFixed(2))

		// Сохраняем оригинальную структуру расчета, но меняем суммы
		calculation.BaseAmount = customAmountDecimal
		calculation.ObjectsAmount = decimal.Zero
		calculation.DiscountAmount = decimal.Zero
		calculation.SubtotalAmount = customAmountDecimal

		// Пересчитываем налоги, если нужно
		var settings models.BillingSettings
		publicDB := bs.db.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
			if err := publicDB.Where("company_id = ? AND admin_account_id = ?", calculation.CompanyID, bs.adminAccountID).
				First(&settings).Error; err == nil {

				if settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
					// НДС включен в цену
					taxRateDivisor := decimal.NewFromInt(100).Add(settings.DefaultTaxRate)
					calculation.TaxAmount = customAmountDecimal.Mul(settings.DefaultTaxRate).Div(taxRateDivisor)
					calculation.TotalAmount = customAmountDecimal
					calculation.SubtotalAmount = customAmountDecimal.Sub(calculation.TaxAmount)
				} else if !settings.TaxIncluded && settings.DefaultTaxRate.GreaterThan(decimal.Zero) {
					// НДС сверху
					calculation.TaxAmount = customAmountDecimal.Mul(settings.DefaultTaxRate).Div(decimal.NewFromInt(100))
					calculation.TotalAmount = customAmountDecimal.Add(calculation.TaxAmount)
				} else {
					calculation.TaxAmount = decimal.Zero
					calculation.TotalAmount = customAmountDecimal
				}
			}
		}

		// Заменяем позиции счета на одну позицию с ручной суммой
		calculation.Items = []InvoiceItemData{
			{
				Name: "Услуги мониторинга (ручная сумма)",
				Description: fmt.Sprintf("Оплата услуг за период с %s по %s",
					calculation.BillingPeriodStart.Format("02.01.2006"),
					calculation.BillingPeriodEnd.Format("02.01.2006")),
				ItemType:    "subscription",
				Quantity:    decimal.NewFromInt(1),
				UnitPrice:   calculation.SubtotalAmount,
				Amount:      calculation.SubtotalAmount,
				PeriodStart: &calculation.BillingPeriodStart,
				PeriodEnd:   &calculation.BillingPeriodEnd,
			},
		}
		log.Printf("✅ Создана позиция счета с ручной суммой: %s", calculation.SubtotalAmount.StringFixed(2))
	}

	// Получаем настройки биллинга из public схемы
	publicDB := bs.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на public: %v", err)
	}

	var settings models.BillingSettings
	if err := publicDB.
		Where("company_id = ? AND admin_account_id = ?", calculation.CompanyID, bs.adminAccountID).
		First(&settings).Error; err != nil {
		return nil, fmt.Errorf("настройки биллинга не найдены: %w", err)
	}

	// Получаем настройки НДС из таблицы companies
	var company models.Company
	if err := publicDB.Where("id = ?", calculation.CompanyID).First(&company).Error; err == nil {
		// Используем настройки НДС из компании, если они есть
		if company.DefaultTaxRate.GreaterThan(decimal.Zero) {
			settings.DefaultTaxRate = company.DefaultTaxRate
			settings.TaxIncluded = company.TaxIncluded
			log.Printf("✅ Используем настройки НДС из компании: rate=%s%%, included=%v", company.DefaultTaxRate.StringFixed(2), company.TaxIncluded)
		}
	}

	// Генерируем номер счета через нумератор
	var numerator models.InvoiceNumerator
	// Ищем нумератор по умолчанию для этой компании
	if err := publicDB.
		Where("admin_account_id = ? AND company_id = ? AND is_active = ? AND is_default = ?",
			bs.adminAccountID, calculation.CompanyID, true, true).
		First(&numerator).Error; err != nil {
		// Если нет нумератора по умолчанию, ищем любой активный
		if err := publicDB.
			Where("admin_account_id = ? AND company_id = ? AND is_active = ?",
				bs.adminAccountID, calculation.CompanyID, true).
			Order("id ASC").
			First(&numerator).Error; err != nil {
			// Если нумераторов нет совсем, используем fallback на старый метод
			log.Printf("⚠️ Нумератор счетов не найден для company_id=%d, используем fallback", calculation.CompanyID)
			timestamp := time.Now().Unix()
			random := rand.Intn(9999)
			sequenceNumber := int(timestamp%100000)*10000 + random
			invoiceNumber := settings.GetInvoiceNumber(sequenceNumber)

			// Проверяем уникальность
			var existingInvoice models.Invoice
			for i := 0; i < 10; i++ {
				if err := publicDB.Where("number = ?", invoiceNumber).First(&existingInvoice).Error; err != nil {
					break
				}
				random = rand.Intn(9999)
				sequenceNumber = int(timestamp%100000)*10000 + random + i
				invoiceNumber = settings.GetInvoiceNumber(sequenceNumber)
			}

			invoice := &models.Invoice{
				Number:             invoiceNumber,
				Title:              fmt.Sprintf("Счет за услуги мониторинга за период %s - %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
				Description:        fmt.Sprintf("Оплата услуг по договору за период с %s по %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
				InvoiceDate:        time.Now(),
				DueDate:            time.Now().AddDate(0, 0, settings.InvoicePaymentTermDays),
				AdminAccountID:     bs.adminAccountID,
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
			if err := publicDB.Create(invoice).Error; err != nil {
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

				if err := publicDB.Create(item).Error; err != nil {
					return nil, fmt.Errorf("ошибка создания позиции счета: %w", err)
				}
			}

			return invoice, nil
		}
	}

	// Генерируем номер счета по шаблону нумератора
	log.Printf("✅ Используем нумератор: id=%d, name=%s, template=%s", numerator.ID, numerator.Name, numerator.Template)

	// Увеличиваем счетчик нумератора
	numerator.CounterValue++
	if err := publicDB.Save(&numerator).Error; err != nil {
		return nil, fmt.Errorf("ошибка обновления нумератора: %w", err)
	}

	// Генерируем номер по шаблону
	invoiceNumber := bs.generateInvoiceNumberFromTemplate(numerator.Template, numerator.Prefix, numerator.CounterValue)
	log.Printf("📋 Сгенерирован номер счета: %s", invoiceNumber)

	// Создаем счет в public схеме
	invoice := &models.Invoice{
		Number:             invoiceNumber,
		Title:              fmt.Sprintf("Счет за услуги мониторинга за период %s - %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
		Description:        fmt.Sprintf("Оплата услуг по договору за период с %s по %s", periodStart.Format("02.01.2006"), periodEnd.Format("02.01.2006")),
		InvoiceDate:        time.Now(),
		DueDate:            time.Now().AddDate(0, 0, settings.InvoicePaymentTermDays),
		AdminAccountID:     bs.adminAccountID,
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
	if err := publicDB.Create(invoice).Error; err != nil {
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

		if err := publicDB.Create(item).Error; err != nil {
			return nil, fmt.Errorf("ошибка создания позиции счета: %w", err)
		}
	}

	// Создаем запись в истории биллинга
	history := &models.BillingHistory{
		AdminAccountID: bs.adminAccountID,
		CompanyID:      calculation.CompanyID,
		InvoiceID:      &invoice.ID,
		ContractID:     &calculation.ContractID,
		Operation:      "invoice_created",
		Amount:         calculation.TotalAmount,
		Currency:       settings.Currency,
		Description:    fmt.Sprintf("Создан счет %s на сумму %s %s", invoice.Number, calculation.TotalAmount.String(), settings.Currency),
		PeriodStart:    &calculation.BillingPeriodStart,
		PeriodEnd:      &calculation.BillingPeriodEnd,
		Status:         "completed",
	}

	if err := publicDB.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		fmt.Printf("Предупреждение: ошибка создания записи в истории биллинга: %v\n", err)
	}

	// Загружаем созданный счет с позициями и тарифным планом (из public схемы)
	// НЕ загружаем Contract через Preload, так как он в tenant-схеме
	if err := publicDB.Preload("Items").Preload("TariffPlan").First(invoice, invoice.ID).Error; err != nil {
		return nil, fmt.Errorf("ошибка загрузки созданного счета: %w", err)
	}

	// Загружаем договор отдельно из tenant-схемы, если contract_id задан
	if invoice.ContractID != nil && *invoice.ContractID > 0 {
		var contract models.Contract
		if err := tenantDB.First(&contract, *invoice.ContractID).Error; err != nil {
			log.Printf("⚠️ Не удалось загрузить договор ID=%d из tenant-схемы: %v", *invoice.ContractID, err)
			// Не прерываем выполнение - счет уже создан
		} else {
			invoice.Contract = &contract
		}
	}

	return invoice, nil
}

// ProcessPayment обрабатывает платеж по счету
func (bs *BillingService) ProcessPayment(invoiceID uint, amount decimal.Decimal, paymentMethod string, notes string) error {
	return bs.ProcessPaymentWithDate(invoiceID, amount, paymentMethod, notes, nil)
}

// ProcessPaymentWithDate обрабатывает платеж по счету с указанной датой
func (bs *BillingService) ProcessPaymentWithDate(invoiceID uint, amount decimal.Decimal, paymentMethod string, notes string, paymentDate *time.Time) error {
	// Устанавливаем схему public для работы с таблицами Invoice и BillingHistory
	publicDB := bs.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		return fmt.Errorf("ошибка установки схемы public: %w", err)
	}

	// Получаем счет
	var invoice models.Invoice
	if err := publicDB.
		Where("id = ? AND admin_account_id = ?", invoiceID, bs.adminAccountID).
		First(&invoice).Error; err != nil {
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
		// Используем указанную дату или текущую дату
		if paymentDate != nil {
			paidAt = paymentDate
		} else {
			now := time.Now()
			paidAt = &now
		}
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

	if err := publicDB.Model(&invoice).Updates(updates).Error; err != nil {
		return fmt.Errorf("ошибка обновления счета: %w", err)
	}

	// Создаем запись в истории биллинга
	history := &models.BillingHistory{
		AdminAccountID: bs.adminAccountID,
		CompanyID:      invoice.CompanyID,
		InvoiceID:      &invoice.ID,
		ContractID:     invoice.ContractID,
		Operation:      "payment_received",
		Amount:         amount,
		Currency:       invoice.Currency,
		Description:    fmt.Sprintf("Получен платеж по счету %s на сумму %s %s. Способ оплаты: %s", invoice.Number, amount.String(), invoice.Currency, paymentMethod),
		Status:         "completed",
	}

	if notes != "" {
		history.Description += fmt.Sprintf(". Примечания: %s", notes)
	}

	if err := publicDB.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		log.Printf("⚠️ Предупреждение: ошибка создания записи в истории биллинга: %v", err)
	}

	return nil
}

// GetBillingHistory возвращает историю биллинга для компании
func (bs *BillingService) GetBillingHistory(companyID uuid.UUID, limit, offset int) ([]models.BillingHistory, int64, error) {
	var history []models.BillingHistory
	var total int64

	// Подсчитываем общее количество записей
	if err := bs.db.Model(&models.BillingHistory{}).
		Where("company_id = ? AND admin_account_id = ?", companyID, bs.adminAccountID).
		Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчета записей истории: %w", err)
	}

	// Получаем записи с пагинацией
	query := bs.db.
		Where("company_id = ? AND admin_account_id = ?", companyID, bs.adminAccountID).
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
		Where("admin_account_id = ?", bs.adminAccountID).
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
	if err := bs.db.
		Where("id = ? AND admin_account_id = ?", invoiceID, bs.adminAccountID).
		First(&invoice).Error; err != nil {
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
		AdminAccountID: bs.adminAccountID,
		CompanyID:      invoice.CompanyID,
		InvoiceID:      &invoice.ID,
		ContractID:     invoice.ContractID,
		Operation:      "invoice_cancelled",
		Amount:         invoice.TotalAmount,
		Currency:       invoice.Currency,
		Description:    fmt.Sprintf("Отменен счет %s. Причина: %s", invoice.Number, reason),
		Status:         "completed",
	}

	if err := bs.db.Create(history).Error; err != nil {
		// Логируем ошибку, но не прерываем выполнение
		fmt.Printf("Предупреждение: ошибка создания записи в истории биллинга: %v\n", err)
	}

	return nil
}

// NextInvoiceNumber генерирует следующий номер счета на основе таблицы invoice_sequences
// Параметры:
//   - countryCode: код страны (например, "RU", "KZ")
//   - prefix: префикс номера (например, "INV", "СЧТ")
//
// Функция:
//  1. Ищет существующую последовательность для countryCode и prefix
//  2. Если не найдена, создает новую с дефолтным шаблоном "%s-%04d"
//  3. Инкрементирует LastNumber
//  4. Возвращает сгенерированный номер по шаблону
func (bs *BillingService) NextInvoiceNumber(countryCode, prefix string) (string, error) {
	if countryCode == "" {
		countryCode = "RU" // По умолчанию Россия
	}
	if prefix == "" {
		prefix = "INV" // По умолчанию INV
	}

	// Ищем существующую последовательность
	var sequence models.InvoiceSequence
	err := bs.db.Where("country_code = ? AND prefix = ? AND is_active = true", countryCode, prefix).First(&sequence).Error

	if err == gorm.ErrRecordNotFound {
		// Создаем новую последовательность
		sequence = models.InvoiceSequence{
			CountryCode: countryCode,
			Prefix:      prefix,
			Pattern:     "%s-%04d", // Дефолтный шаблон
			LastNumber:  0,
			Description: fmt.Sprintf("Последовательность для %s, префикс %s", countryCode, prefix),
			IsActive:    true,
		}
		if err := bs.db.Create(&sequence).Error; err != nil {
			return "", fmt.Errorf("ошибка создания последовательности: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("ошибка поиска последовательности: %w", err)
	}

	// Инкрементируем номер в транзакции
	tx := bs.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Error; err != nil {
		return "", fmt.Errorf("ошибка начала транзакции: %w", err)
	}

	// Блокируем строку для обновления
	var lockedSequence models.InvoiceSequence
	if err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", sequence.ID).
		First(&lockedSequence).Error; err != nil {
		tx.Rollback()
		return "", fmt.Errorf("ошибка блокировки последовательности: %w", err)
	}

	// Инкрементируем номер
	lockedSequence.LastNumber++
	newNumber := lockedSequence.LastNumber

	// Сохраняем обновленную последовательность
	if err := tx.Model(&lockedSequence).Update("last_number", newNumber).Error; err != nil {
		tx.Rollback()
		return "", fmt.Errorf("ошибка обновления последовательности: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return "", fmt.Errorf("ошибка коммита транзакции: %w", err)
	}

	// Генерируем номер по шаблону
	invoiceNumber := fmt.Sprintf(sequence.Pattern, prefix, newNumber)

	return invoiceNumber, nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "UNIQUE constraint") ||
		strings.Contains(errMsg, "unique constraint") ||
		strings.Contains(errMsg, "already exists")
}

// generateInvoiceNumberFromTemplate генерирует номер счета по шаблону нумератора
func (bs *BillingService) generateInvoiceNumberFromTemplate(template, prefix string, counter int) string {
	now := time.Now()

	// Замены для шаблона
	replacements := map[string]string{
		"{PREFIX}":     prefix,
		"{YEAR}":       fmt.Sprintf("%d", now.Year()),
		"{YEAR_SHORT}": fmt.Sprintf("%02d", now.Year()%100),
		"{MONTH}":      fmt.Sprintf("%02d", now.Month()),
		"{DAY}":        fmt.Sprintf("%02d", now.Day()),
		"{SEQ}":        fmt.Sprintf("%04d", counter),
		"{COUNTER}":    fmt.Sprintf("%04d", counter),
	}

	result := template
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}
