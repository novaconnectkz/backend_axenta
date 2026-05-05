package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"backend_axenta/middleware"
	"backend_axenta/models"
)

// =====================================================================
// /api/auth/dashboard/today-installations — монтажи на сегодня
// =====================================================================

type TodayInstallationItem struct {
	ID            uint      `json:"id"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	TimeLabel     string    `json:"time_label"` // "09:00" — для UI
	Status        string    `json:"status"`     // planned, in_progress, completed
	Type          string    `json:"type"`
	Address       string    `json:"address"`
	InstallerID   uint      `json:"installer_id"`
	InstallerName string    `json:"installer_name"`
	ObjectID      uint      `json:"object_id"`
	ObjectName    string    `json:"object_name"`
}

// GetTodayInstallations возвращает монтажи на сегодня, отсортированные по
// времени. Limit 20 (на дашборде показываем верхние 5 — лимит для запаса).
func GetTodayInstallations(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "error": "Ошибка подключения к базе данных компании",
		})
		return
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.Add(24 * time.Hour)

	var rows []models.Installation
	if err := tenantDB.
		Preload("Installer").Preload("Object").
		Where("scheduled_at >= ? AND scheduled_at < ?", dayStart, dayEnd).
		Order("scheduled_at ASC").
		Limit(20).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "error": "Ошибка получения монтажей: " + err.Error(),
		})
		return
	}

	items := make([]TodayInstallationItem, 0, len(rows))
	for _, r := range rows {
		item := TodayInstallationItem{
			ID:          r.ID,
			ScheduledAt: r.ScheduledAt,
			TimeLabel:   r.ScheduledAt.Format("15:04"),
			Status:      r.Status,
			Type:        r.Type,
			Address:     r.Address,
			InstallerID: r.InstallerID,
			ObjectID:    r.ObjectID,
		}
		if r.Installer != nil {
			item.InstallerName = installerDisplayName(r.Installer)
		}
		if r.Object != nil {
			item.ObjectName = r.Object.Name
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   items,
	})
}

func installerDisplayName(i *models.Installer) string {
	full := i.LastName
	if i.FirstName != "" {
		if full != "" {
			full += " "
		}
		full += string([]rune(i.FirstName)[0]) + "."
	}
	return full
}

// =====================================================================
// /api/auth/dashboard/recent-invoices — последние счета
// =====================================================================

type RecentInvoiceItem struct {
	ID          uint            `json:"id"`
	Number      string          `json:"number"`
	Status      string          `json:"status"`
	TotalAmount decimal.Decimal `json:"total_amount"`
	PaidAmount  decimal.Decimal `json:"paid_amount"`
	DueDate     time.Time       `json:"due_date"`
	CreatedAt   time.Time       `json:"created_at"`
	ClientName  string          `json:"client_name"` // best-effort из Contract
	IsOverdue   bool            `json:"is_overdue"`
}

// GetRecentInvoices возвращает последние 10 счетов компании (по created_at DESC).
// Status и сумма — для отображения в Today row дашборда.
//
// Invoices живут в public schema (не в tenant) — используем публичную DB
// с фильтром по company_id. Contract тоже в public.
func GetRecentInvoices(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "error": "Ошибка подключения к базе данных компании",
		})
		return
	}
	companyID := middleware.GetCompanyID(c)
	publicDB := publicDBOrTenant(tenantDB)

	limit := 10
	var rows []models.Invoice
	// Явный schema-prefix через Table("public.invoices") — иначе при tenant
	// search_path GORM ищет invoices в tenant_NNN, где её нет.
	// TODO: вернуть Preload("Contract") когда выясним почему он
	// возвращает []. Возможно GORM игнорирует Table() override и
	// пытается найти invoices в tenant schema.
	if err := publicDB.
		Table(publicTable(publicDB, "invoices")).
		Where("company_id = ?", companyID).
		Order("created_at DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "error": "Ошибка получения счетов: " + err.Error(),
		})
		return
	}

	items := make([]RecentInvoiceItem, 0, len(rows))
	now := time.Now()
	for _, r := range rows {
		item := RecentInvoiceItem{
			ID:          r.ID,
			Number:      r.Number,
			Status:      r.Status,
			TotalAmount: r.TotalAmount,
			PaidAmount:  r.PaidAmount,
			DueDate:     r.DueDate,
			CreatedAt:   r.CreatedAt,
			IsOverdue:   r.Status != "paid" && r.Status != "cancelled" && now.After(r.DueDate),
		}
		if r.Contract != nil {
			item.ClientName = contractClientName(r.Contract)
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   items,
	})
}

// contractClientName — best-effort извлечение имени клиента из Contract.
// Поле может называться по-разному в исторических версиях модели; здесь
// читаем самые распространённые имена через рефлексию-альтернативу — простую
// проверку. Если не нашли — возвращаем пустую строку (фронт выводит "—").
func contractClientName(c *models.Contract) string {
	// На текущей модели — Contract.ClientName.
	if c.ClientName != "" {
		return c.ClientName
	}
	return ""
}

// =====================================================================
// Helper для тестов: позволяет вытащить структуры без публичной зависимости
// =====================================================================

// Не экспортируем дополнительных API — только handlers для роутера.
// Регистрация в main.go рядом с alerts/kpi.
var _ *gorm.DB = nil // suppress unused gorm import in some build configs
