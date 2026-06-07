package models

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// PartnerDailySnapshot представляет ежедневный снимок объектов партнера
// Снимки делаются каждый день в 00:00 UTC для тарификации партнерских договоров
type PartnerDailySnapshot struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Привязка
	AdminAccountID uint `json:"admin_account_id" gorm:"not null;index:idx_partner_snapshots_admin"`
	CompanyID      uint `json:"company_id" gorm:"not null;index:idx_partner_snapshots_company"`
	ContractID     uint `json:"contract_id" gorm:"not null;index:idx_partner_snapshots_contract;uniqueIndex:idx_partner_snapshot_unique,priority:4"`

	// Дата снимка (начало дня в UTC)
	SnapshotDate time.Time `json:"snapshot_date" gorm:"not null;index:idx_partner_snapshots_date;uniqueIndex:idx_partner_snapshot_unique,priority:5"`

	// ID учетной записи партнера
	PartnerCompanyID uint `json:"partner_company_id" gorm:"not null;index:idx_partner_snapshots_partner"`

	// Мульти-системный партнёр (Ф0). Составной uniqueIndex idx_partner_snapshot_unique =
	// (partner_source, connection_id, partner_external_id, contract_id, snapshot_date).
	// Для Axenta: source='axenta', connection_id=0, partner_external_id=partner_company_id.
	// NON-partial (GORM не умеет WHERE deleted_at) — данные чистые, soft-delete+recreate
	// в штатном потоке не происходит (scheduler пропускает существующие через Unscoped).
	PartnerSource     string `json:"partner_source" gorm:"type:varchar(20);not null;default:'axenta';uniqueIndex:idx_partner_snapshot_unique,priority:1"`
	ConnectionID      uint   `json:"connection_id" gorm:"not null;default:0;uniqueIndex:idx_partner_snapshot_unique,priority:2"`
	PartnerExternalID string `json:"partner_external_id" gorm:"type:varchar(128);not null;default:'';uniqueIndex:idx_partner_snapshot_unique,priority:3"`

	// Тарифный план на момент снимка
	TariffPlanID uint        `json:"tariff_plan_id" gorm:"not null"`
	TariffPlan   BillingPlan `json:"tariff_plan,omitempty" gorm:"foreignKey:TariffPlanID;references:ID;constraint:-"`

	// Месячная цена тарифа (сохраняем для истории)
	MonthlyPrice decimal.Decimal `json:"monthly_price" gorm:"type:decimal(10,2);not null"`

	// Дневная цена = monthly_price / 30 (храним с высокой точностью для расчетов)
	DailyPrice decimal.Decimal `json:"daily_price" gorm:"type:decimal(12,6);not null"`

	// Количество объектов
	TotalObjectsCount  int `json:"total_objects_count" gorm:"not null;default:0"`  // Всего объектов в учетной записи
	ActiveObjectsCount int `json:"active_objects_count" gorm:"not null;default:0"` // Активных объектов (для тарификации)

	// Скидки (используется только один тип: либо процент, либо фиксированная сумма)
	DiscountType    string          `json:"discount_type" gorm:"type:varchar(20);default:'none'"` // none, manual, auto
	DiscountPercent decimal.Decimal `json:"discount_percent" gorm:"type:decimal(5,2);default:0"`  // Процент скидки (0-100)
	DiscountFixed   decimal.Decimal `json:"discount_fixed" gorm:"type:decimal(12,2);default:0"`   // Фиксированная скидка в рублях

	// Стоимость до скидки = daily_price * active_objects_count
	CostBeforeDiscount decimal.Decimal `json:"cost_before_discount" gorm:"type:decimal(12,4);not null;default:0"`

	// Сумма скидки
	DiscountAmount decimal.Decimal `json:"discount_amount" gorm:"type:decimal(12,4);not null;default:0"`

	// Стоимость за день после применения скидки
	DailyCost decimal.Decimal `json:"daily_cost" gorm:"type:decimal(12,4);not null"`

	// Статус снимка
	Status string `json:"status" gorm:"type:varchar(20);default:'completed'"` // pending, completed, failed

	// Ф1 billing-gate: двойная сверка данных снимка.
	// VerifyStatus — итог сверки: verified / needs_review / estimated / manual_approved.
	// DEFAULT 'verified' grandfather'ит исторические строки; guard понижает при детекте.
	VerifyStatus string `json:"verify_status" gorm:"type:varchar(20);not null;default:'verified';index:idx_partner_snapshots_verify"`
	// VerifySecondaryCount — независимый второй счёт активных (cross-source). -1 = не считался.
	VerifySecondaryCount int `json:"verify_secondary_count" gorm:"not null;default:-1"`
	// PrevActiveCount — активные за предыдущий день (baseline continuity-guard). -1 = нет baseline.
	PrevActiveCount int `json:"prev_active_count" gorm:"not null;default:-1"`
	// DeltaPct — относительное изменение active vs предыдущий день, %.
	DeltaPct decimal.Decimal `json:"delta_pct" gorm:"type:decimal(7,2);not null;default:0"`
	// AmountAtRisk — сумма под риском (₽) при needs_review (|Δ daily_cost| или потерянный day-cost).
	AmountAtRisk decimal.Decimal `json:"amount_at_risk" gorm:"type:decimal(14,2);not null;default:0"`
	// IsEstimated — снимок восстановлен текущим состоянием (backfill без истории провайдера).
	IsEstimated bool `json:"is_estimated" gorm:"not null;default:false"`
	// VerifyNotes — человекочитаемая причина статуса сверки.
	VerifyNotes string `json:"verify_notes" gorm:"type:text"`
	// VerifiedAt — момент успешной сверки (или ручного подтверждения).
	VerifiedAt *time.Time `json:"verified_at" gorm:"index"`
	// ApprovedBy — ID пользователя, вручную подтвердившего needs_review/estimated.
	ApprovedBy uint `json:"approved_by" gorm:"default:0"`

	// SourceWarn — транзиентный вход guard'а: непустой = источник дал warning
	// (нет данных, force-sync упал) → снимок уйдёт в needs_review. Не персистится.
	SourceWarn string `json:"-" gorm:"-"`

	// ContractNumber — человекочитаемый № договора (резолвится в API из tenant.contracts,
	// не персистится). В справочнике показываем его вместо внутреннего contract_id.
	ContractNumber string `json:"contract_number" gorm:"-"`

	// PartnerName — имя партнёрского аккаунта/дилера (резолвится в API, не персистится).
	// Чтобы по строкам «без договора» было видно, чьи это объекты.
	PartnerName string `json:"partner_name" gorm:"-"`

	// ConnectionName — имя подключения-сервиса (glomoskz/glomosuz/app.gpsnetwork…),
	// резолвится в API из public.{wialon,skif,gelios}_connections по connection_id.
	// Для axenta (connection_id=0) пусто — одно облако. Не персистится.
	ConnectionName string `json:"connection_name" gorm:"-"`

	// IsHidden — дилер-ориентир скрыт из справочника (резолвится в API при show_hidden,
	// не персистится; источник истины — таблица partner_hidden_dealers).
	IsHidden bool `json:"is_hidden" gorm:"-"`

	// Дополнительная информация
	Notes string `json:"notes" gorm:"type:text"` // Примечания (например, ошибки)
}

// TableName задает имя таблицы для модели PartnerDailySnapshot
func (PartnerDailySnapshot) TableName() string {
	return "partner_daily_snapshots"
}

// Пороги continuity-guard. needs_review срабатывает только если ОБА порога превышены —
// гасит шум на малых аккаунтах (1→2 = +100%, но абс=1 < порога → не флагаем).
const (
	verifyDeltaAbsThreshold = 15   // абсолютное изменение active за день
	verifyDeltaPctThreshold = 25.0 // относительное изменение active, %
)

// VerifyStatus-значения.
const (
	VerifyStatusVerified  = "verified"
	VerifyStatusNeedsRev  = "needs_review"
	VerifyStatusEstimated = "estimated"
	VerifyStatusApproved  = "manual_approved"
)

// applyVerifyGuard проставляет поля сверки (continuity + cross-source) ДО записи.
// tx — соединение записи (search_path = целевая tenant-схема), поэтому baseline-снимок
// читается из той же схемы. Вызывается из BeforeCreate ПОСЛЕ ComputeCosts (нужен daily_cost).
// Входы от сервиса (через поля структуры): VerifySecondaryCount (-1=не считался),
// IsEstimated (backfill без истории), SourceWarn (источник дал warning).
func (s *PartnerDailySnapshot) applyVerifyGuard(tx *gorm.DB) {
	now := time.Now().UTC()

	// 1) Источник дал warning (нет данных / force-sync упал) → сразу needs_review.
	if s.SourceWarn != "" {
		s.VerifyStatus = VerifyStatusNeedsRev
		s.VerifyNotes = "источник: " + s.SourceWarn
		s.AmountAtRisk = s.DailyCost.Abs()
		return
	}

	// 2) baseline — предыдущий доверенный снимок (snapshot_date-1, тот же ключ).
	prevDay := s.SnapshotDate.AddDate(0, 0, -1)
	var prev PartnerDailySnapshot
	hasPrev := tx.Where(
		"partner_source = ? AND connection_id = ? AND partner_external_id = ? AND contract_id = ? AND snapshot_date = ?",
		s.PartnerSource, s.ConnectionID, s.PartnerExternalID, s.ContractID, prevDay,
	).Where("verify_status IN ?", []string{VerifyStatusVerified, VerifyStatusApproved}).
		Order("snapshot_date DESC").First(&prev).Error == nil

	if hasPrev {
		s.PrevActiveCount = prev.ActiveObjectsCount
		s.DeltaPct = pctDelta(prev.ActiveObjectsCount, s.ActiveObjectsCount)
	} else {
		s.PrevActiveCount = -1
		s.DeltaPct = decimal.Zero
	}

	// 3) cross-source: независимый второй счёт активных сильно расходится → needs_review.
	if s.VerifySecondaryCount >= 0 && countsDiverge(s.ActiveObjectsCount, s.VerifySecondaryCount) {
		s.VerifyStatus = VerifyStatusNeedsRev
		s.VerifyNotes = fmt.Sprintf("cross-source расхождение: основной=%d, второй=%d", s.ActiveObjectsCount, s.VerifySecondaryCount)
		s.AmountAtRisk = s.DailyCost.Abs()
		return
	}

	if hasPrev {
		// 4) Обнуление где раньше было ненулевым → подозрительно (сигнал circuit).
		if prev.ActiveObjectsCount > 0 && s.ActiveObjectsCount == 0 {
			s.VerifyStatus = VerifyStatusNeedsRev
			s.VerifyNotes = fmt.Sprintf("обнуление: вчера %d активных, сегодня 0", prev.ActiveObjectsCount)
			s.AmountAtRisk = prev.DailyCost.Abs()
			return
		}
		// 5) Скачок active > порога (абс И отн одновременно) → needs_review.
		absDelta := absInt(s.ActiveObjectsCount - prev.ActiveObjectsCount)
		if absDelta >= verifyDeltaAbsThreshold && s.DeltaPct.Abs().GreaterThanOrEqual(decimal.NewFromFloat(verifyDeltaPctThreshold)) {
			s.VerifyStatus = VerifyStatusNeedsRev
			s.VerifyNotes = fmt.Sprintf("скачок active: %d→%d (%s%%)", prev.ActiveObjectsCount, s.ActiveObjectsCount, s.DeltaPct.StringFixed(1))
			s.AmountAtRisk = s.DailyCost.Sub(prev.DailyCost).Abs()
			return
		}
	}

	// 6) Backfill прошлого дня текущим состоянием — честно estimated (блокируется в billing),
	//    но continuity пройден → фиксируем как estimated, не verified.
	if s.IsEstimated {
		s.VerifyStatus = VerifyStatusEstimated
		if s.VerifyNotes == "" {
			s.VerifyNotes = "backfill прошлого дня текущим состоянием (нет истории провайдера)"
		}
		s.AmountAtRisk = s.DailyCost.Abs()
		return
	}

	// 7) Всё чисто → verified.
	s.VerifyStatus = VerifyStatusVerified
	s.VerifyNotes = ""
	s.AmountAtRisk = decimal.Zero
	s.VerifiedAt = &now
}

// pctDelta — относительное изменение cur vs prev в процентах (prev=0 → 100% если cur>0).
func pctDelta(prev, cur int) decimal.Decimal {
	if prev == 0 {
		if cur == 0 {
			return decimal.Zero
		}
		return decimal.NewFromInt(100)
	}
	return decimal.NewFromInt(int64(cur - prev)).
		Div(decimal.NewFromInt(int64(prev))).
		Mul(decimal.NewFromInt(100)).Round(2)
}

// countsDiverge — расходятся ли два счёта активных сверх порога (абс И отн).
func countsDiverge(primary, secondary int) bool {
	absDiff := absInt(primary - secondary)
	if absDiff < verifyDeltaAbsThreshold {
		return false
	}
	base := primary
	if base == 0 {
		base = secondary
	}
	if base == 0 {
		return false
	}
	pct := float64(absDiff) / float64(base) * 100
	return pct >= verifyDeltaPctThreshold
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// BeforeCreate устанавливает дефолтные значения перед созданием.
// Покрывает ВСЕ пути создания снимка (cron всех 4 источников через OnConflict.Create +
// raw db.Create Axenta), поэтому ценовой расчёт И сверка централизованы здесь.
func (s *PartnerDailySnapshot) BeforeCreate(tx *gorm.DB) error {
	// Ф0: гарантируем заполнение мульти-системных полей на ВСЕХ путях создания.
	// Без этого Axenta-снимки писались бы с partner_external_id='' → orphan-строки
	// разных партнёров (contract_id=0) схлопнулись бы по составному unique-индексу.
	if s.PartnerSource == "" {
		s.PartnerSource = "axenta"
	}
	if s.PartnerExternalID == "" && s.PartnerCompanyID > 0 {
		s.PartnerExternalID = fmt.Sprintf("%d", s.PartnerCompanyID)
	}

	s.ComputeCosts()       // цена/скидка → daily_cost (детерминированно)
	s.applyVerifyGuard(tx) // Ф1: continuity + cross-source сверка (нужен daily_cost выше)

	return nil
}

// Recompute пересчитывает цены И сверку для существующего снимка. Нужен на update-пути
// Axenta (dup-key fallback через Save), где BeforeCreate не срабатывает. tx — соединение
// записи (tenant search_path), чтобы baseline-снимок читался из правильной схемы.
func (s *PartnerDailySnapshot) Recompute(tx *gorm.DB) {
	s.ComputeCosts()
	s.applyVerifyGuard(tx)
}

// ComputeCosts рассчитывает daily_price/cost_before_discount/discount_amount/daily_cost
// из monthly_price, скидок и числа дней месяца снимка. Детерминированно и идемпотентно.
//
// daily_price = monthly_price / days_in_month(snapshot_date). Делитель — фактическое
// число дней в месяце снимка (28/29/30/31), не хардкод 30. Гарантирует
// Σ(daily_cost за месяц) == monthly_price * active_objects (точно). monthly_price НЕ меняется.
func (s *PartnerDailySnapshot) ComputeCosts() {
	daysInMonth := decimal.NewFromInt(int64(time.Date(
		s.SnapshotDate.Year(), s.SnapshotDate.Month()+1, 0, 0, 0, 0, 0, time.UTC,
	).Day()))

	if s.DiscountFixed.GreaterThan(decimal.Zero) {
		// Фиксированная скидка применяется к МЕСЯЧНОМУ тарифу
		effectiveMonthlyPrice := s.MonthlyPrice.Sub(s.DiscountFixed)
		if effectiveMonthlyPrice.IsNegative() {
			effectiveMonthlyPrice = decimal.Zero
		}
		effectiveDailyPrice := effectiveMonthlyPrice.Div(daysInMonth).Round(4)
		s.DailyPrice = effectiveDailyPrice
		s.CostBeforeDiscount = s.MonthlyPrice.Div(daysInMonth).Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
		s.DiscountAmount = s.DiscountFixed.Div(daysInMonth).Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
		s.DailyCost = effectiveDailyPrice.Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
		return
	}

	// Базовая дневная цена (без скидки) + процентная скидка к стоимости
	baseDailyPrice := s.MonthlyPrice.Div(daysInMonth).Round(4)
	s.DailyPrice = baseDailyPrice
	s.CostBeforeDiscount = baseDailyPrice.Mul(decimal.NewFromInt(int64(s.ActiveObjectsCount))).Round(2)
	if s.DiscountPercent.GreaterThan(decimal.Zero) {
		discountMultiplier := s.DiscountPercent.Div(decimal.NewFromInt(100))
		s.DiscountAmount = s.CostBeforeDiscount.Mul(discountMultiplier).Round(2)
	} else {
		s.DiscountAmount = decimal.Zero
	}
	s.DailyCost = s.CostBeforeDiscount.Sub(s.DiscountAmount).Round(2)
}
