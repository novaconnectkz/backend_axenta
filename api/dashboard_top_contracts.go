package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"backend_axenta/middleware"
)

// TopContractRow — строка таблицы «Топ-контракты по выручке» на дашборде.
type TopContractRow struct {
	ID             uint    `json:"id"`
	ClientName     string  `json:"client_name"`
	ContractNumber string  `json:"contract_number"`
	Objects        int     `json:"objects"`
	Active         int     `json:"active"`
	Overdue        int     `json:"overdue"`
	MRR            float64 `json:"mrr"`
}

// TopContractsResponse — payload ответа /api/auth/dashboard/top-contracts.
type TopContractsResponse struct {
	Data        []TopContractRow `json:"data"`
	Period      string           `json:"period"`
	From        time.Time        `json:"from"`
	To          time.Time        `json:"to"`
	GeneratedAt time.Time        `json:"generated_at"`
}

// GetDashboardTopContracts возвращает топ контрактов по выручке за период.
//
// GET /api/auth/dashboard/top-contracts?period=month&limit=10
//
// MRR = SUM(invoices.total_amount) WHERE billing_period_start попадает в период.
// Контракты без invoice'ов в периоде в выдачу не попадают.
//
// period: month (default) | quarter | year
// limit:  1..50, default 10
func GetDashboardTopContracts(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подключения к базе данных компании",
		})
		return
	}

	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}

	period := c.DefaultQuery("period", "month")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit < 1 || limit > 50 {
		limit = 10
	}

	from, to := periodWindow(period, time.Now())
	publicDB := publicDBOrTenant(tenantDB)

	// 1) Группируем invoices по contract_id за период
	type invAgg struct {
		ContractID uint            `gorm:"column:contract_id"`
		MRR        decimal.Decimal `gorm:"column:mrr"`
		Overdue    int64           `gorm:"column:overdue"`
	}
	var aggs []invAgg
	invoicesTable := publicTable(publicDB, "invoices")
	publicDB.Raw(`
		SELECT contract_id,
		       COALESCE(SUM(total_amount), 0) AS mrr,
		       COUNT(*) FILTER (WHERE status = 'overdue' OR (status NOT IN ('paid','cancelled') AND due_date < NOW())) AS overdue
		FROM `+invoicesTable+`
		WHERE admin_account_id = ?
		  AND contract_id IS NOT NULL
		  AND billing_period_start >= ?
		  AND billing_period_start < ?
		GROUP BY contract_id
		ORDER BY mrr DESC
		LIMIT ?
	`, adminAccountID, from, to, limit).Scan(&aggs)

	if len(aggs) == 0 {
		c.JSON(http.StatusOK, TopContractsResponse{
			Data:        []TopContractRow{},
			Period:      period,
			From:        from,
			To:          to,
			GeneratedAt: time.Now(),
		})
		return
	}

	contractIDs := make([]uint, 0, len(aggs))
	for _, a := range aggs {
		contractIDs = append(contractIDs, a.ContractID)
	}

	// 2) Подтягиваем номер + клиента из contracts (тенантная схема)
	type contractMeta struct {
		ID         uint
		Number     string
		ClientName string
	}
	var metas []contractMeta
	tenantDB.Table("contracts").
		Select("id, number, client_name").
		Where("id IN ? AND admin_account_id = ?", contractIDs, adminAccountID).
		Scan(&metas)
	metaByID := make(map[uint]contractMeta, len(metas))
	for _, m := range metas {
		metaByID[m.ID] = m
	}

	// 3) Считаем objects/active по contract_id из tenant.objects
	type objAgg struct {
		ContractID uint
		Total      int64
		Active     int64
	}
	var objs []objAgg
	tenantDB.Table("objects").
		Select(`contract_id,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'active' OR is_active = true) AS active`).
		Where("contract_id IN ? AND deleted_at IS NULL", contractIDs).
		Group("contract_id").
		Scan(&objs)
	objByID := make(map[uint]objAgg, len(objs))
	for _, o := range objs {
		objByID[o.ContractID] = o
	}

	// 4) Собираем ответ в исходном порядке (MRR DESC)
	rows := make([]TopContractRow, 0, len(aggs))
	for _, a := range aggs {
		meta := metaByID[a.ContractID]
		oa := objByID[a.ContractID]
		mrrFloat, _ := a.MRR.Float64()
		rows = append(rows, TopContractRow{
			ID:             a.ContractID,
			ClientName:     meta.ClientName,
			ContractNumber: meta.Number,
			Objects:        int(oa.Total),
			Active:         int(oa.Active),
			Overdue:        int(a.Overdue),
			MRR:            mrrFloat,
		})
	}

	c.JSON(http.StatusOK, TopContractsResponse{
		Data:        rows,
		Period:      period,
		From:        from,
		To:          to,
		GeneratedAt: time.Now(),
	})
}

// periodWindow возвращает [from, to) для period: month/quarter/year.
func periodWindow(period string, now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	y, m, _ := now.Date()
	switch period {
	case "quarter":
		qStartMonth := time.Month(((int(m)-1)/3)*3 + 1)
		from := time.Date(y, qStartMonth, 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(0, 3, 0)
	case "year":
		from := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(1, 0, 0)
	default: // month
		from := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return from, from.AddDate(0, 1, 0)
	}
}
