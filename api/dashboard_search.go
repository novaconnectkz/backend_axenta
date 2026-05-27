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
	Type     string `json:"type"`     // object, client, contract, invoice, user, installation
	Title    string `json:"title"`    // основная строка
	Subtitle string `json:"subtitle"` // вторичный контекст
	URL      string `json:"url"`      // куда переходить по клику
}

// SearchResponse — группы результатов по типу.
type SearchResponse struct {
	Objects       []SearchResultItem `json:"objects"`
	Clients       []SearchResultItem `json:"clients"`
	Contracts     []SearchResultItem `json:"contracts"`
	Invoices      []SearchResultItem `json:"invoices"`
	Users         []SearchResultItem `json:"users"`
	Installations []SearchResultItem `json:"installations"`
	Query         string             `json:"query"`
}

// validScopes — допустимые значения scope-фильтра (Б24-style chips).
var validScopes = map[string]struct{}{
	"objects":       {},
	"clients":       {},
	"contracts":     {},
	"invoices":      {},
	"users":         {},
	"installations": {},
}

// parseScope разбирает CSV scope-параметр в map. Пустой scope = все группы.
func parseScope(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(strings.ToLower(s))
		if _, ok := validScopes[s]; ok {
			out[s] = struct{}{}
		}
	}
	return out
}

// inScope — true если scope пустой (всё) или содержит ключ.
func inScope(scope map[string]struct{}, key string) bool {
	if len(scope) == 0 {
		return true
	}
	_, ok := scope[key]
	return ok
}

// GetGlobalSearch — глобальный поиск по объектам, клиентам, контрактам, счетам,
// пользователям и монтажам с опциональным scope-фильтром (Б24-style).
//
// GET /api/auth/search?q=<term>&limit=10&scope=objects,contracts
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

	scope := parseScope(c.Query("scope"))
	publicDB := publicDBOrTenant(tenantDB)
	pattern := "%" + q + "%"
	companyID := middleware.GetCompanyID(c)

	// Все поля инициализируем пустыми слайсами — Go marshallит nil как null,
	// а FE падает на .length у не-возвращённой группы при scope-фильтре.
	resp := SearchResponse{
		Query:         q,
		Objects:       []SearchResultItem{},
		Clients:       []SearchResultItem{},
		Contracts:     []SearchResultItem{},
		Invoices:      []SearchResultItem{},
		Users:         []SearchResultItem{},
		Installations: []SearchResultItem{},
	}
	if inScope(scope, "objects") {
		resp.Objects = searchObjects(tenantDB, pattern, limit)
	}
	if inScope(scope, "clients") {
		resp.Clients = searchClients(publicDB, tenantDB, companyID, pattern, limit)
	}
	if inScope(scope, "contracts") {
		resp.Contracts = searchContracts(tenantDB, pattern, limit)
	}
	if inScope(scope, "invoices") {
		resp.Invoices = searchInvoices(publicDB, companyID, pattern, limit)
	}
	if inScope(scope, "users") {
		resp.Users = searchUsers(publicDB, tenantDB, companyID, pattern, limit)
	}
	if inScope(scope, "installations") {
		resp.Installations = searchInstallations(tenantDB, pattern, limit)
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
			URL:      "/contracts/edit/" + strconv.FormatUint(uint64(r.ID), 10),
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
			URL:      "/billing?tab=invoices",
		})
	}
	return out
}

// searchClients — объединённый поиск «учётных записей» по 5 источникам:
//   - companies (public) — ACRM
//   - axenta_account_snapshots (tenant) — Axenta
//   - gelios_users (public, is_admin OR units_count>0) — GELIOS играет роль аккаунта
//   - skif_dealers (public, JOIN skif_connections.company_id) — SKIF
//   - wialon_account_statuses (public, JOIN wialon_connections.company_id) — Wialon
func searchClients(publicDB, tenantDB *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	perSource := limit/2 + 1
	out := make([]SearchResultItem, 0, limit*3)
	out = append(out, searchAcrmCompanies(publicDB, pattern, perSource)...)
	out = append(out, searchAxentaAccounts(tenantDB, pattern, perSource)...)
	out = append(out, searchGeliosAccounts(publicDB, companyID, pattern, perSource)...)
	out = append(out, searchSkifDealers(publicDB, companyID, pattern, perSource)...)
	out = append(out, searchWialonAccounts(publicDB, companyID, pattern, perSource)...)
	if len(out) > limit*3 {
		out = out[:limit*3]
	}
	return out
}

func searchAcrmCompanies(db *gorm.DB, pattern string, limit int) []SearchResultItem {
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
			ID:       "client-acrm:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    r.Name,
			Subtitle: "ACRM · учётная запись",
			URL:      "/accounts?search=" + r.Name + "&source=axenta",
		})
	}
	return out
}

func searchAxentaAccounts(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID            uint
		AccountName   string
		AdminFullname string
		IsActive      bool
	}
	var rows []row
	db.Table("axenta_account_snapshots").
		Select("id, account_name, admin_fullname, is_active").
		Where("(account_name ILIKE ? OR admin_fullname ILIKE ?) AND deleted_at IS NULL",
			pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		subtitle := "Axenta"
		if r.AdminFullname != "" {
			subtitle += " · " + r.AdminFullname
		}
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "client-axenta:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    r.AccountName,
			Subtitle: subtitle,
			URL:      "/accounts?search=" + r.AccountName + "&source=axenta",
		})
	}
	return out
}

func searchGeliosAccounts(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID        uint
		Login     string
		Email     string
		LegalName string
		IsBlock   bool
	}
	var rows []row
	db.Table(publicTable(db, "gelios_users")).
		Select("id, login, email, legal_name, is_block").
		Where("company_id = ? AND (is_admin = true OR units_count > 0) AND (login ILIKE ? OR email ILIKE ? OR legal_name ILIKE ?) AND gelios_deleted_at IS NULL",
			companyID, pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.LegalName
		if title == "" {
			title = r.Login
		}
		subtitle := "GELIOS · учётная запись"
		if r.Login != "" && r.Login != title {
			subtitle += " · " + r.Login
		}
		if r.IsBlock {
			subtitle += " · заблокирован"
		}
		out = append(out, SearchResultItem{
			ID:       "client-gelios:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/accounts?search=" + r.Login + "&source=gelios",
		})
	}
	return out
}

func searchSkifDealers(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID    uint
		Name  string
		Email string
	}
	var rows []row
	db.Table(publicTable(db, "skif_dealers")+" AS sd").
		Joins("JOIN "+publicTable(db, "skif_connections")+" AS sc ON sc.id = sd.connection_id").
		Select("sd.id, sd.name, sd.email").
		Where("sc.company_id = ? AND sd.hidden = false AND (sd.name ILIKE ? OR sd.email ILIKE ?)",
			companyID, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		subtitle := "SKIF · дилер"
		if r.Email != "" {
			subtitle += " · " + r.Email
		}
		out = append(out, SearchResultItem{
			ID:       "client-skif:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    r.Name,
			Subtitle: subtitle,
			URL:      "/accounts?search=" + r.Name + "&source=skif",
		})
	}
	return out
}

func searchWialonAccounts(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID       uint
		Name     string
		IsActive bool
	}
	var rows []row
	db.Table(publicTable(db, "wialon_account_statuses")+" AS was").
		Joins("JOIN "+publicTable(db, "wialon_connections")+" AS wc ON wc.id = was.connection_id").
		Select("was.id, was.name, was.is_active").
		Where("wc.company_id = ? AND was.name ILIKE ?", companyID, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		subtitle := "Wialon · учётная запись"
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "client-wialon:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "client",
			Title:    r.Name,
			Subtitle: subtitle,
			URL:      "/accounts?search=" + r.Name + "&source=wialon",
		})
	}
	return out
}

// searchUsers — объединённый поиск по 5 источникам:
//   - local_users (public, company_id фильтр)
//   - axenta_user_snapshots (tenant)
//   - gelios_users (public, company_id)
//   - skif_users (public, company_id)
//   - wialon_users (public, JOIN wialon_connections по company_id)
//
// Каждый источник возвращает до limit/2 результатов, чтобы UI не перегружать.
func searchUsers(publicDB, tenantDB *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	perSource := limit/2 + 1
	out := make([]SearchResultItem, 0, limit*3)
	out = append(out, searchLocalUsers(publicDB, companyID, pattern, perSource)...)
	out = append(out, searchAxentaUsers(tenantDB, pattern, perSource)...)
	out = append(out, searchGeliosUsers(publicDB, companyID, pattern, perSource)...)
	out = append(out, searchSkifUsers(publicDB, companyID, pattern, perSource)...)
	out = append(out, searchWialonUsers(publicDB, companyID, pattern, perSource)...)
	if len(out) > limit*3 {
		out = out[:limit*3]
	}
	return out
}

func searchLocalUsers(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID       uint
		Username string
		Email    string
		Name     string
		Role     string
		IsActive bool
	}
	var rows []row
	db.Table(publicTable(db, "local_users")).
		Select("id, username, email, name, role, is_active").
		Where("company_id = ? AND (username ILIKE ? OR email ILIKE ? OR name ILIKE ?) AND deleted_at IS NULL",
			strconv.FormatUint(uint64(companyID), 10), pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.Name
		if title == "" {
			title = r.Username
		}
		subtitle := "ACRM · " + r.Username
		if r.Email != "" {
			subtitle += " · " + r.Email
		}
		if r.Role != "" {
			subtitle += " · " + r.Role
		}
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "user-local:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "user",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/users?search=" + r.Username + "&source=axenta",
		})
	}
	return out
}

func searchAxentaUsers(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID       uint
		Username string
		Name     string
		Email    string
		IsActive bool
	}
	var rows []row
	db.Table("axenta_user_snapshots").
		Select("id, username, name, email, is_active").
		Where("(username ILIKE ? OR name ILIKE ? OR email ILIKE ?) AND deleted_at IS NULL",
			pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.Name
		if title == "" {
			title = r.Username
		}
		subtitle := "Axenta · " + r.Username
		if r.Email != "" {
			subtitle += " · " + r.Email
		}
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "user-axenta:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "user",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/users?search=" + r.Username + "&source=axenta",
		})
	}
	return out
}

func searchGeliosUsers(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID        uint
		Login     string
		Email     string
		LegalName string
		IsBlock   bool
	}
	var rows []row
	db.Table(publicTable(db, "gelios_users")).
		Select("id, login, email, legal_name, is_block").
		Where("company_id = ? AND (login ILIKE ? OR email ILIKE ? OR legal_name ILIKE ?) AND gelios_deleted_at IS NULL",
			companyID, pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.LegalName
		if title == "" {
			title = r.Login
		}
		subtitle := "GELIOS · " + r.Login
		if r.Email != "" {
			subtitle += " · " + r.Email
		}
		if r.IsBlock {
			subtitle += " · заблокирован"
		}
		out = append(out, SearchResultItem{
			ID:       "user-gelios:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "user",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/users?search=" + r.Login + "&source=gelios",
		})
	}
	return out
}

func searchSkifUsers(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID       uint
		Name     string
		Email    string
		RoleKey  string
		IsActive bool
	}
	var rows []row
	db.Table(publicTable(db, "skif_users")).
		Select("id, name, email, role_key, is_active").
		Where("company_id = ? AND (name ILIKE ? OR email ILIKE ?) AND skif_deleted_at IS NULL",
			companyID, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.Name
		if title == "" {
			title = r.Email
		}
		subtitle := "SKIF"
		if r.Email != "" {
			subtitle += " · " + r.Email
		}
		if r.RoleKey != "" {
			subtitle += " · " + r.RoleKey
		}
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "user-skif:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "user",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/users?search=" + r.Email + "&source=skif",
		})
	}
	return out
}

func searchWialonUsers(db *gorm.DB, companyID uint, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID        uint
		Name      string
		ShortName string
		IsActive  bool
	}
	var rows []row
	db.Table(publicTable(db, "wialon_users")+" AS wu").
		Joins("JOIN "+publicTable(db, "wialon_connections")+" AS wc ON wc.id = wu.connection_id").
		Select("wu.id, wu.name, wu.short_name, wu.is_active").
		Where("wc.company_id = ? AND (wu.name ILIKE ? OR wu.short_name ILIKE ?)",
			companyID, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.Name
		if title == "" {
			title = r.ShortName
		}
		subtitle := "Wialon"
		if r.ShortName != "" && r.ShortName != title {
			subtitle += " · " + r.ShortName
		}
		if !r.IsActive {
			subtitle += " · неактивен"
		}
		out = append(out, SearchResultItem{
			ID:       "user-wialon:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "user",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/users?search=" + title + "&source=wialon",
		})
	}
	return out
}

// searchInstallations — по installations (description, address, client_contact).
func searchInstallations(db *gorm.DB, pattern string, limit int) []SearchResultItem {
	type row struct {
		ID            uint
		Type          string
		Status        string
		Description   string
		Address       string
		ClientContact string
	}
	var rows []row
	db.Table("installations").
		Select("id, type, status, description, address, client_contact").
		Where("(description ILIKE ? OR address ILIKE ? OR client_contact ILIKE ?) AND deleted_at IS NULL",
			pattern, pattern, pattern).
		Limit(limit).
		Scan(&rows)

	out := make([]SearchResultItem, 0, len(rows))
	for _, r := range rows {
		title := r.Description
		if title == "" {
			title = r.Address
		}
		if title == "" {
			title = "Монтаж #" + strconv.FormatUint(uint64(r.ID), 10)
		}
		subtitle := r.Type
		if r.Status != "" {
			subtitle += " · " + r.Status
		}
		if r.Address != "" && r.Address != title {
			subtitle += " · " + r.Address
		}
		out = append(out, SearchResultItem{
			ID:       "installation:" + strconv.FormatUint(uint64(r.ID), 10),
			Type:     "installation",
			Title:    title,
			Subtitle: subtitle,
			URL:      "/installations",
		})
	}
	return out
}
