package api

import (
	"fmt"
	"net/http"
	"strings"
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
		buildAlertKPI(c, tenantDB, publicDB, companyID, now),
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
// Σ daily_cost из partner_daily_snapshots за текущий месяц — плановая выручка
// (биллинг-начисления per-договор, наполняется partner-snapshot cron'ом
// 00:30/00:45/00:50/00:55 UTC). Раньше брали paid_amount из invoices, но
// на проде invoices не помечаются paid → tile всегда 0. Snapshot-источник
// даёт осмысленное значение сразу.
//
// status='completed' — пропускаем warning-снимки (GELIOS Ф3: warning =
// дилер исчез/нулевой). Delta vs прошлый месяц.

func buildMonthlyRevenueKPI(_ *gorm.DB, tenantDB *gorm.DB, _ uint, now time.Time) KPIMetric {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	prevMonthStart := monthStart.AddDate(0, -1, 0)

	current := sumBilledFromSnapshots(tenantDB, monthStart, monthStart.AddDate(0, 1, 0))
	prev := sumBilledFromSnapshots(tenantDB, prevMonthStart, monthStart)

	delta := current.Sub(prev)
	dir, deltaText, pct := formatRublesDelta(current, delta, prev, "vs прошлый месяц")

	// Per-source breakdown (для tooltip). Σ amount совпадает с current.
	// Скрываем нулевые источники — tooltip покажет только non-zero.
	breakdown := sumBilledPerSource(tenantDB, monthStart, monthStart.AddDate(0, 1, 0))

	return KPIMetric{
		ID:              "monthly_revenue",
		Title:           "Начислено за месяц",
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

// sumBilledFromSnapshots — Σ daily_cost из partner_daily_snapshots за период.
// status='completed' — без warning-снимков (нулевые/исчезнувшие дилеры).
func sumBilledFromSnapshots(tenantDB *gorm.DB, from, to time.Time) decimal.Decimal {
	if tenantDB == nil {
		return decimal.Zero
	}
	var row struct {
		Sum decimal.Decimal
	}
	tenantDB.Table("partner_daily_snapshots").
		Select("COALESCE(SUM(daily_cost), 0) as sum").
		Where("snapshot_date >= ? AND snapshot_date < ?", from, to).
		Where("status = ?", "completed").
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

// sumBilledPerSource — разбивка sumBilledFromSnapshots по partner_source.
// tenant-scoped (snapshots в tenant schema), без cross-schema JOIN. Только
// non-zero и status='completed'.
func sumBilledPerSource(tenantDB *gorm.DB, from, to time.Time) []SourceBreakdown {
	if tenantDB == nil {
		return nil
	}
	type row struct {
		PartnerSource string          `gorm:"column:partner_source"`
		Sum           decimal.Decimal `gorm:"column:sum"`
	}
	var rows []row
	err := tenantDB.Table("partner_daily_snapshots").
		Select("partner_source, COALESCE(SUM(daily_cost), 0) AS sum").
		Where("snapshot_date >= ? AND snapshot_date < ?", from, to).
		Where("status = ?", "completed").
		Where("partner_source <> ''").
		Group("partner_source").
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

func buildAlertKPI(c *gin.Context, tenantDB, publicDB *gorm.DB, companyID uint, now time.Time) KPIMetric {
	type candidate struct {
		title    string
		value    string
		count    int64
		severity string
		url      string
	}

	candidates := []candidate{}

	// Просроченные счета (public.invoices) — scoping менеджера (только свои договоры)
	var overdue int64
	applyManagerScope(c, publicDB.Table(publicTable(publicDB, "invoices")).
		Where("company_id = ? AND due_date < ? AND status NOT IN ?", companyID, now, []string{"paid", "cancelled"})).
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

// formatRublesValue форматирует число с пробелами-разделителями тысяч
// (русская типография). 583634 → "583 634 ₽".
func formatRublesValue(d decimal.Decimal) string {
	n := d.Round(0).IntPart()
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return sign + s + " ₽"
	}
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		b.WriteByte(' ')
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(' ')
		}
	}
	return sign + b.String() + " ₽"
}

func rublesAsFloat(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}
