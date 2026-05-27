package api

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
)

// DashboardAlert — единый формат алерта для top-level Alerts row на дашборде.
// Источники собираются в GetDashboardAlerts и сортируются по severity.
type DashboardAlert struct {
	ID          string    `json:"id"`           // уникальный ключ для key= во Vue
	Severity    string    `json:"severity"`     // critical, high, medium, low
	Category    string    `json:"category"`     // billing, warehouse, installations, notifications
	Title       string    `json:"title"`        // короткий, для строки списка
	Description string    `json:"description"`  // полный текст с цифрами
	Count       int       `json:"count"`        // число агрегированных элементов
	ActionURL   string    `json:"action_url"`   // CTA → frontend route
	ActionLabel string    `json:"action_label"` // текст CTA-кнопки
	CreatedAt   time.Time `json:"created_at"`   // момент агрегации (now)
}

// publicDBOrTenant возвращает database.DB (public schema) если он доступен,
// иначе fallback на tenant DB. Нужно для запросов к глобальным таблицам
// (invoices, notification_logs) при работающем tenant search_path.
func publicDBOrTenant(tenantDB *gorm.DB) *gorm.DB {
	if database.DB != nil {
		return database.DB
	}
	return tenantDB
}

// publicTable возвращает имя таблицы с public-prefix для PostgreSQL,
// иначе (SQLite в тестах) — без префикса. Работает в обход tenant
// search_path для глобальных таблиц.
func publicTable(db *gorm.DB, name string) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return "public." + name
	}
	return name
}

// severityRank возвращает приоритет для сортировки. Чем больше — тем выше показывается.
func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// GetDashboardAlerts возвращает unified-список активных алертов для текущего тенанта.
// Источники: просроченные счета, low-stock, просроченные монтажи, failed уведомления (24ч).
//
// GET /api/auth/dashboard/alerts
//
// Ответ: { "data": [DashboardAlert, ...] } — отсортированы по severity DESC.
// Если алертов нет — `data: []` (frontend скрывает строку).
func GetDashboardAlerts(c *gin.Context) {
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
	publicDB := publicDBOrTenant(tenantDB) // invoices живут в public, не в tenant schema
	alerts := []DashboardAlert{}

	// 1. Просроченные счета — Invoice с DueDate < now() и не оплачен/не отменён.
	var overdueCount int64
	var overdueSum decimal.Decimal
	publicDB.Table(publicTable(publicDB, "invoices")).
		Where("company_id = ? AND due_date < ? AND status NOT IN ?", companyID, now, []string{"paid", "cancelled"}).
		Count(&overdueCount)
	if overdueCount > 0 {
		// Суммируем remaining = total_amount - paid_amount
		var sumRow struct {
			Sum decimal.Decimal
		}
		publicDB.Table(publicTable(publicDB, "invoices")).
			Select("COALESCE(SUM(total_amount - paid_amount), 0) as sum").
			Where("company_id = ? AND due_date < ? AND status NOT IN ?", companyID, now, []string{"paid", "cancelled"}).
			Scan(&sumRow)
		overdueSum = sumRow.Sum

		severity := "high"
		if overdueCount >= 10 {
			severity = "critical"
		}
		alerts = append(alerts, DashboardAlert{
			ID:          "billing.overdue",
			Severity:    severity,
			Category:    "billing",
			Title:       "Просроченные счета",
			Description: formatOverdueDescription(overdueCount, overdueSum),
			Count:       int(overdueCount),
			ActionURL:   "/billing/overdue",
			ActionLabel: "Просмотреть",
			CreatedAt:   now,
		})
	}

	// 2. Low-stock — активные StockAlert.
	var stockRows []struct {
		Severity string
		Count    int64
	}
	tenantDB.Model(&models.StockAlert{}).
		Select("severity, COUNT(*) as count").
		Where("status = ?", "active").
		Group("severity").
		Scan(&stockRows)
	var stockTotal int64
	stockSeverity := "medium"
	for _, r := range stockRows {
		stockTotal += r.Count
		if r.Severity == "critical" || r.Severity == "high" {
			stockSeverity = "high"
		}
	}
	if stockTotal > 0 {
		if stockTotal >= 20 {
			stockSeverity = "critical"
		}
		alerts = append(alerts, DashboardAlert{
			ID:          "warehouse.low_stock",
			Severity:    stockSeverity,
			Category:    "warehouse",
			Title:       "Низкие остатки на складе",
			Description: formatLowStockDescription(stockTotal),
			Count:       int(stockTotal),
			ActionURL:   "/warehouse/alerts",
			ActionLabel: "Склад",
			CreatedAt:   now,
		})
	}

	// 3. Просроченные монтажи — scheduled_at < now() и статус активный.
	var overdueInst int64
	tenantDB.Model(&models.Installation{}).
		Where("scheduled_at < ? AND status IN ?", now, []string{"planned", "in_progress"}).
		Count(&overdueInst)
	if overdueInst > 0 {
		severity := "medium"
		if overdueInst >= 5 {
			severity = "high"
		}
		alerts = append(alerts, DashboardAlert{
			ID:          "installations.overdue",
			Severity:    severity,
			Category:    "installations",
			Title:       "Просроченные монтажи",
			Description: formatOverdueInstallationsDescription(overdueInst),
			Count:       int(overdueInst),
			ActionURL:   "/installations?filter=overdue",
			ActionLabel: "Монтажи",
			CreatedAt:   now,
		})
	}

	// 4. Failed уведомления за последние 24ч (notification_logs в public).
	since := now.Add(-24 * time.Hour)
	var failedNotif int64
	publicDB.Table(publicTable(publicDB, "notification_logs")).
		Where("company_id = ? AND status IN ? AND created_at >= ?", companyID, []string{"failed", "failed_final"}, since).
		Count(&failedNotif)
	if failedNotif > 0 {
		severity := "low"
		if failedNotif >= 10 {
			severity = "medium"
		}
		if failedNotif >= 50 {
			severity = "high"
		}
		alerts = append(alerts, DashboardAlert{
			ID:          "notifications.failed_24h",
			Severity:    severity,
			Category:    "notifications",
			Title:       "Ошибки отправки уведомлений",
			Description: formatFailedNotifDescription(failedNotif),
			Count:       int(failedNotif),
			ActionURL:   "/notification-logs?status=failed",
			ActionLabel: "Журнал",
			CreatedAt:   now,
		})
	}

	// Сортировка: по severity DESC, при равенстве — по count DESC.
	sort.SliceStable(alerts, func(i, j int) bool {
		ri, rj := severityRank(alerts[i].Severity), severityRank(alerts[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return alerts[i].Count > alerts[j].Count
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   alerts,
	})
}

// =====================================================================
// Форматтеры описаний — выделены для тестирования и переиспользования.
// =====================================================================

func formatOverdueDescription(count int64, sum decimal.Decimal) string {
	if sum.IsZero() {
		return pluralize(count, "просроченный счёт", "просроченных счёта", "просроченных счетов")
	}
	return pluralize(count, "просроченный счёт на ", "просроченных счёта на ", "просроченных счетов на ") +
		formatRubles(sum)
}

func formatLowStockDescription(count int64) string {
	return pluralize(count, "позиция оборудования с критически низким остатком",
		"позиции оборудования с критически низким остатком",
		"позиций оборудования с критически низким остатком")
}

func formatOverdueInstallationsDescription(count int64) string {
	return pluralize(count, "просроченный монтаж требует внимания",
		"просроченных монтажа требуют внимания",
		"просроченных монтажей требуют внимания")
}

func formatFailedNotifDescription(count int64) string {
	return pluralize(count, "уведомление не доставлено за последние 24 часа",
		"уведомления не доставлены за последние 24 часа",
		"уведомлений не доставлено за последние 24 часа")
}

// pluralize выбирает форму русского слова по числу: 1 → form1, 2-4 → form2, 5+ → form5.
// Учитывает исключения 11-19 (всегда form5).
func pluralize(n int64, form1, form2, form5 string) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	mod100 := abs % 100
	if mod100 >= 11 && mod100 <= 19 {
		return formatCount(n) + " " + form5
	}
	switch abs % 10 {
	case 1:
		return formatCount(n) + " " + form1
	case 2, 3, 4:
		return formatCount(n) + " " + form2
	default:
		return formatCount(n) + " " + form5
	}
}

func formatCount(n int64) string {
	// Без разделителей для дашборда — числа малые.
	return decimal.NewFromInt(n).String()
}

func formatRubles(d decimal.Decimal) string {
	// Округляем до целых + пробелы-разделители тысяч (русская типография).
	// Делегируем в formatRublesValue (dashboard_kpi.go) — один helper на пакет.
	return formatRublesValue(d)
}
