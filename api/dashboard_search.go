package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/middleware"
)

// SearchResultItem — один элемент в результатах глобального поиска.
type SearchResultItem struct {
	ID       string `json:"id"`       // составной ID для key=
	Type     string `json:"type"`     // object, client, contract, invoice
	Title    string `json:"title"`    // основная строка
	Subtitle string `json:"subtitle"` // вторичный контекст
	URL      string `json:"url"`      // куда переходить по клику
}

// SearchResponse — группы результатов по типу.
// Контракты и счета пока скрыты — расширим scope позже.
type SearchResponse struct {
	Objects []SearchResultItem `json:"objects"`
	Clients []SearchResultItem `json:"clients"`
	Query   string             `json:"query"`
}

// GetGlobalSearch — глобальный поиск по объектам, клиентам, контрактам, счетам.
//
// GET /api/auth/search?q=<term>&limit=10
//
// Источники:
//   - axenta_object_snapshots (tenant) — object_name, unique_id, account_name
//   - contracts (tenant) — number, client_name
//   - companies (public) — name (для клиентов partner-аккаунта)
//   - invoices (public) — number, по company_id текущей компании
func GetGlobalSearch(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error", "error": "Ошибка подключения к базе данных компании",
		})
		return
	}

	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   SearchResponse{Query: q},
		})
		return
	}

	limit := 10
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 50 {
		limit = l
	}

	publicDB := publicDBOrTenant(tenantDB)
	pattern := "%" + q + "%"

	resp := SearchResponse{
		Query:   q,
		Objects: searchObjects(tenantDB, pattern, limit),
		Clients: searchClients(publicDB, pattern, limit),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   resp,
	})
}

// searchObjects — по axenta_object_snapshots (object_name, unique_id, account_name).
func searchObjects(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ExternalObjectID string
		ObjectName       string
		UniqueID         string
		AccountName      string
		IsActive         bool
	}
	var rows []row
	db.Table("axenta_object_snapshots").
		Select("external_object_id, object_name, unique_id, account_name, is_active").
		Where("(object_name ILIKE ? OR unique_id ILIKE ? OR account_name ILIKE ?) AND deleted_at IS NULL",
			pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		statusBadge := "активный"
		if !r.IsActive {
			statusBadge = "неактивный"
		}
		subtitle := r.AccountName
		if subtitle != "" {
			subtitle += " · "
		}
		subtitle += "ID: " + r.UniqueID + " · " + statusBadge
		out = append(out, SearchResultItem{
			ID:       "object:" + r.ExternalObjectID,
			Type:     "object",
			Title:    r.ObjectName,
			Subtitle: subtitle,
			URL:      "/objects?search=" + r.UniqueID,
		})
	}
	return out
}

// searchContracts — по contracts (number, client_name).
func searchContracts(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID         uint
		Number     string
		ClientName string
	}
	var rows []row
	db.Table("contracts").
		Select("id, number, client_name").
		Where("(number ILIKE ? OR client_name ILIKE ?) AND deleted_at IS NULL", pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchResultItem{
			ID:       "contract:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "contract",
			Title:    r.Number,
			Subtitle: r.ClientName,
			URL:      "/contracts/" + strconv.FormatUint(uint64(r.ID), 10),
		})
	}
	return out
}

// searchInvoices — по invoices (number) для текущей компании.
func searchInvoices(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID     uint
		Number string
		Status string
	}
	var rows []row
	db.Table(publicTable(db, "invoices")).
		Select("id, number, status").
		Where("company_id = ? AND number ILIKE ? AND deleted_at IS NULL", companyID, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchResultItem{
			ID:       "invoice:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "invoice",
			Title:    r.Number,
			Subtitle: "Счёт · " + r.Status,
			URL:      "/billing/invoices/" + strconv.FormatUint(uint64(r.ID), 10),
		})
	}
	return out
}

// searchClients — по companies.name (учётные записи / клиенты).
func searchClients(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID   uint
		Name string
	}
	var rows []row
	db.Table(publicTable(db, "companies")).
		Select("id, name").
		Where("name ILIKE ? AND deleted_at IS NULL", pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, SearchResultItem{
			ID:       "client:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    r.Name,
			Subtitle: "Учётная запись",
			URL:      "/accounts?search=" + r.Name,
		})
	}
	return out
}
