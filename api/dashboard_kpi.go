package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
)

// SourceBreakdown — разбивка значения метрики по partner_source (axenta/skif/wialon/gelios).
// Используется в monthly_revenue для отображения вклада каждой GPS-системы.
type SourceBreakdown struct {
	Source   string  `json:"source"`    // "axenta" | "skif" | "wialon" | "gelios"
	Label    string  `json:"label"`     // "Axenta" | "SKIF" | "Wialon" | "GELIOS"
	Amount   string  `json:"amount"`    // "145 100 ₽" — отформатировано
	RawValue float64 `json:"raw_value"` // числовое для сортировки/процента
}

// KPIMetric — одна метрика верхнего бара дашборда с delta vs предыдущий период.
type KPIMetric struct {
	ID              string            `json:"id"`                  // active_objects, monthly_revenue, today_installations, alert
	Title           string            `json:"title"`               // "Активные объекты"
	Value           string            `json:"value"`               // "9710" / "25 500 ₽" — отформатировано для UI
	RawValue        float64           `json:"raw_value"`           // числовое значение для сортировки/чартов
	Delta           string            `json:"delta"`               // "+12 за неделю" — текст для отображения
	DeltaDirection  string            `json:"delta_direction"`     // up, down, flat — для цвета
	DeltaValue      float64           `json:"delta_value"`         // числовая дельта (может быть отрицательной)
	DeltaPercentage float64           `json:"delta_percentage"`    // % изменения, если применимо
	ActionURL       string            `json:"action_url"`          // CTA → frontend route
	Breakdown       []SourceBreakdown `json:"breakdown,omitempty"` // Опциональная разбивка per partner_source (только monthly_revenue, >0)
}

// KPIResponse — payload ответа /api/auth/dashboard/kpi.
type KPIResponse struct {
	Metrics     []KPIMetric `json:"metrics"`
	GeneratedAt time.Time   `json:"generated_at"`
}

// GetDashboardKPI возвращает 4 ключевые метрики верхнего бара дашборда
// с дельтами vs предыдущий сравнимый период.
//
// GET /api/auth/dashboard/kpi
//
// Метрики (фиксированные, без кастомизации):
//  1. active_objects — текущее число активных объектов + delta vs неделю назад (snapshot)
//  2. monthly_revenue — выручка за текущий месяц + delta vs прошлый месяц
//  3. today_installations — запланированные на сегодня монтажи
//  4. alert — самый severe из активных алертов (для top-of-mind)
func GetDashboardKPI(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных компании",
		})
		return
	}

	now := time.Now()
	companyID := middleware.GetCompanyID(c)
	publicDB := publicDBOrTenant(tenantDB)

	metrics := []KPIMetric{
		buildActiveObjectsKPI(tenantDB, now),
		buildMonthlyRevenueKPI(publicDB, tenantDB, companyID, now),
		buildTodayInstallationsKPI(tenantDB, now),
		buildAlertKPI(tenantDB, publicDB, companyID, now),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": KPIResponse{
			Metrics:     metrics,
			GeneratedAt: now,
		},
	})
}

// =====================================================================
// Метрика 1: Активные объекты
// =====================================================================
//
// Объекты живут в Axenta Cloud (внешняя система), tenant.objects обычно
// пуст. Берём актуальные числа из partner_daily_snapshots — агрегаты
// по контрактам, наполняются cron Partner Snapshot (00:30 UTC).
//
// Current = SUM(active_objects_count) на последний snapshot_date.
// Delta = current - SUM на (latest_date - 7 days).

func buildActiveObjectsKPI(db *gorm.DB, _ time.Time) KPIMetric {
	// Один источник истины для агрегатов partner_daily_snapshots — services.AnalyticsService.
	// Раньше SQL дублировался здесь и в partner_snapshot_scheduler.go.
	analytics := services.NewAnalyticsService(db)

	_, ok := analytics.GetLatestSnapshotDate()
	if !ok {
		return KPIMetric{
			ID: "active_objects", Title: "Активные объекты",
			Value: "0", RawValue: 0,
			Delta: "нет данных snapshot", DeltaDirection: "flat",
			ActionURL: "/objects",
		}
	}

	// Per-source latest: каждый источник (axenta/skif/...) на свою последнюю дату.
	// Единая latestDate недосчитывала бы источники без снимка на этот день
	// (разная частота снимков). latestDate оставляем только для «as of»-метки.
	curActive := analytics.GetActiveCountLatestPerSource(0)
	prevActive := analytics.GetActiveCountLatestPerSource(-7)

	delta := curActive - prevActive
	dir, deltaText := formatCountDelta(delta, "за неделю")

	return KPIMetric{
		ID:             "active_objects",
		Title:          "Активные объекты",
		Value:          fmt.Sprintf("%d", curActive),
		RawValue:       float64(curActive),
		Delta:          deltaText,
		DeltaDirection: dir,
		DeltaValue:     float64(delta),
		ActionURL:      "/objects",
	}
}

// =====================================================================
// Метрика 2: Выручка за месяц
// =====================================================================
//
// Сумма paid_amount по invoices с paid_at в текущем месяце. Delta vs
// прошлый месяц (та же арифметика для предыдущего month-range).

func buildMonthlyRevenueKPI(db *gorm.DB, tenantDB *gorm.DB, companyID uint, now time.Time) KPIMetric {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	prevMonthStart := monthStart.AddDate(0, -1, 0)

	current := sumPaid(db, companyID, monthStart, monthStart.AddDate(0, 1, 0))
	prev := sumPaid(db, companyID, prevMonthStart, monthStart)

	delta := current.Sub(prev)
	dir, deltaText, pct := formatRublesDelta(current, delta, prev, "vs прошлый месяц")

	// Per-source breakdown (для tooltip на FE). Σ amount должна совпадать с current.
	// Скрываем нулевые источники — на UI tooltip покажет только non-zero.
	breakdown := sumPaidPerSource(tenantDB, companyID, monthStart, monthStart.AddDate(0, 1, 0))

	return KPIMetric{
		ID:              "monthly_revenue",
		Title:           "Выручка за месяц",
		Value:           formatRublesValue(current),
		RawValue:        rublesAsFloat(current),
		Delta:           deltaText,
		DeltaDirection:  dir,
		DeltaValue:      rublesAsFloat(delta),
		DeltaPercentage: pct,
		ActionURL:       "/billing",
		Breakdown:       breakdown,
	}
}

func sumPaid(db *gorm.DB, companyID uint, from, to time.Time) decimal.Decimal {
	var row struct {
		Sum decimal.Decimal
	}
	db.Table(publicTable(db, "invoices")).
		Select("COALESCE(SUM(paid_amount), 0) as sum").
		Where("company_id = ? AND paid_at >= ? AND paid_at < ?", companyID, from, to).
		Scan(&row)
	return row.Sum
}

// sourceLabels — отображаемые имена GPS-систем для UI breakdown.
var sourceLabels = map[string]string{
	"axenta": "Axenta",
	"skif":   "SKIF",
	"wialon": "Wialon",
	"gelios": "GELIOS",
}

// sumPaidPerSource — разбивка sumPaid по partner_source через JOIN
// public.invoices ↔ tenant.contracts (search_path tenantDB переключён на схему
// текущего тенанта, contracts резолвится туда). Возвращает только источники с
// amount > 0 (нулевые скрываются — для tooltip без UI-шума).
//
// Σ amount по результату должна совпадать с sumPaid за тот же период
// (если paid_amount распределяется по contract_id; invoice'ы без contract_id
// или без partner_source попадут в "other" — отбрасываются).
func sumPaidPerSource(tenantDB *gorm.DB, companyID uint, from, to time.Time) []SourceBreakdown {
	if tenantDB == nil {
		return nil
	}
	type row struct {
		PartnerSource string          `gorm:"column:partner_source"`
		Sum           decimal.Decimal `gorm:"column:sum"`
	}
	var rows []row
	// SQLite (тесты) не поддерживает кросс-schema JOIN с public-префиксом —
	// при postgres JOIN public.invoices + contracts (резолвится из tenant
	// search_path); при SQLite — обе таблицы в одной БД.
	invoicesTable := publicTable(tenantDB, "invoices")
	err := tenantDB.Table(invoicesTable+" AS i").
		Select("c.partner_source AS partner_source, COALESCE(SUM(i.paid_amount), 0) AS sum").
		Joins("JOIN contracts c ON c.id = i.contract_id").
		Where("i.company_id = ? AND i.paid_at >= ? AND i.paid_at < ?", companyID, from, to).
		Where("c.partner_source <> ''").
		Group("c.partner_source").
		Scan(&rows).Error
	if err != nil {
		return nil
	}

	out := make([]SourceBreakdown, 0, len(rows))
	for _, r := range rows {
		if !r.Sum.GreaterThan(decimal.Zero) {
			continue
		}
		label, ok := sourceLabels[r.PartnerSource]
		if !ok {
			label = r.PartnerSource
		}
		out = append(out, SourceBreakdown{
			Source:   r.PartnerSource,
			Label:    label,
			Amount:   formatRublesValue(r.Sum),
			RawValue: rublesAsFloat(r.Sum),
		})
	}
	return out
}

// =====================================================================
// Метрика 3: Монтажи на сегодня
// =====================================================================
//
// Count installations со scheduled_at в окне [today_start, today_end).
// Delta vs того же дня прошлой недели (Mon→Mon, Tue→Tue) — учитывает
// недельную сезонность.

func buildTodayInstallationsKPI(db *gorm.DB, now time.Time) KPIMetric {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	prevDayStart := dayStart.AddDate(0, 0, -7)
	prevDayEnd := dayEnd.AddDate(0, 0, -7)

	var current, prev int64
	db.Model(&models.Installation{}).
		Where("scheduled_at >= ? AND scheduled_at < ?", dayStart, dayEnd).
		Count(&current)
	db.Model(&models.Installation{}).
		Where("scheduled_at >= ? AND scheduled_at < ?", prevDayStart, prevDayEnd).
		Count(&prev)

	delta := current - prev
	dir, deltaText := formatCountDelta(delta, "vs прошлая неделя")

	return KPIMetric{
		ID:             "today_installations",
		Title:          "Монтажи сегодня",
		Value:          fmt.Sprintf("%d", current),
		RawValue:       float64(current),
		Delta:          deltaText,
		DeltaDirection: dir,
		DeltaValue:     float64(delta),
		ActionURL:      "/installations",
	}
}

// =====================================================================
// Метрика 4: Самый severe алерт (для top-of-mind)
// =====================================================================
//
// Берём count из тех же 4 источников что и Alerts row, выбираем тот, у
// которого максимальный severity. Если активных алертов нет — показываем
// "Всё в норме" в качестве fallback.

func buildAlertKPI(tenantDB, publicDB *gorm.DB, companyID uint, now time.Time) KPIMetric {
	type candidate struct {
		title    string
		value    string
		count    int64
		severity string
		url      string
	}

	candidates := []candidate{}

	// Просроченные счета (public.invoices)
	var overdue int64
	publicDB.Table(publicTable(publicDB, "invoices")).
		Where("company_id = ? AND due_date < ? AND status NOT IN ?", companyID, now, []string{"paid", "cancelled"}).
		Count(&overdue)
	if overdue > 0 {
		sev := "high"
		if overdue >= 10 {
			sev = "critical"
		}
		candidates = append(candidates, candidate{
			title:    "Просроченные счета",
			value:    fmt.Sprintf("%d", overdue),
			count:    overdue,
			severity: sev,
			url:      "/billing/overdue",
		})
	}

	// Низкие остатки (tenant.stock_alerts)
	var lowStock int64
	tenantDB.Model(&models.StockAlert{}).
		Where("status = ?", "active").
		Count(&lowStock)
	if lowStock > 0 {
		sev := "medium"
		if lowStock >= 20 {
			sev = "critical"
		}
		candidates = append(candidates, candidate{
			title:    "Низкие остатки",
			value:    fmt.Sprintf("%d", lowStock),
			count:    lowStock,
			severity: sev,
			url:      "/warehouse/alerts",
		})
	}

	// Просроченные монтажи (tenant.installations)
	var overdueInst int64
	tenantDB.Model(&models.Installation{}).
		Where("scheduled_at < ? AND status IN ?", now, []string{"planned", "in_progress"}).
		Count(&overdueInst)
	if overdueInst > 0 {
		sev := "medium"
		if overdueInst >= 5 {
			sev = "high"
		}
		candidates = append(candidates, candidate{
			title:    "Просроченные монтажи",
			value:    fmt.Sprintf("%d", overdueInst),
			count:    overdueInst,
			severity: sev,
			url:      "/installations?filter=overdue",
		})
	}

	if len(candidates) == 0 {
		return KPIMetric{
			ID:             "alert",
			Title:          "Состояние",
			Value:          "OK",
			RawValue:       0,
			Delta:          "Нет активных алертов",
			DeltaDirection: "flat",
			ActionURL:      "/notification-logs",
		}
	}

	// Выбираем максимально severe (при равенстве — больший count)
	best := candidates[0]
	for _, c := range candidates[1:] {
		if severityRank(c.severity) > severityRank(best.severity) ||
			(severityRank(c.severity) == severityRank(best.severity) && c.count > best.count) {
			best = c
		}
	}

	return KPIMetric{
		ID:             "alert",
		Title:          best.title,
		Value:          best.value,
		RawValue:       float64(best.count),
		Delta:          "Требует внимания · " + best.severity,
		DeltaDirection: "down",
		DeltaValue:     float64(best.count),
		ActionURL:      best.url,
	}
}

// =====================================================================
// Форматтеры
// =====================================================================

// formatCountDelta — для целочисленных метрик (объекты, монтажи).
// Возвращает direction (up/down/flat) и текст вида "▲ +12 за неделю".
// Стрелки добавляет frontend по direction — здесь только число.
func formatCountDelta(delta int64, suffix string) (direction, text string) {
	switch {
	case delta > 0:
		return "up", fmt.Sprintf("+%d %s", delta, suffix)
	case delta < 0:
		return "down", fmt.Sprintf("%d %s", delta, suffix)
	default:
		return "flat", "без изменений " + suffix
	}
}

// formatRublesDelta — для денежных метрик. Возвращает direction, текст и %.
// Если предыдущий период == 0, percentage = 0 (избегаем деления на ноль).
func formatRublesDelta(current, delta, prev decimal.Decimal, suffix string) (direction, text string, pct float64) {
	zero := decimal.NewFromInt(0)
	cmp := delta.Cmp(zero)

	if !prev.IsZero() {
		pctDec := delta.Mul(decimal.NewFromInt(100)).Div(prev)
		pct, _ = pctDec.Float64()
	}

	switch {
	case cmp > 0:
		return "up", fmt.Sprintf("+%s %s", formatRublesValue(delta), suffix), pct
	case cmp < 0:
		return "down", fmt.Sprintf("%s %s", formatRublesValue(delta), suffix), pct
	default:
		return "flat", "без изменений " + suffix, 0
	}
}

func formatRublesValue(d decimal.Decimal) string {
	return d.Round(0).String() + " ₽"
}

func rublesAsFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}
