package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============================================================================
// Контрагенты (Ф1). Единый лицевой счёт per контрагент: N договоров = ОДИН баланс.
//
// Таблица counterparties — глобальная (public), как ledger_entries. Скоупится по
// (admin_account_id, company_id). В Ф1 ВСЕ endpoint'ы под admin-гейтом
// (requireContractAssignAccess) — FE-формы (договор/список/ЛС-вью) подключат их в Ф4,
// тогда search ослабнет до менеджеров через managerScopedCounterparties.
// ============================================================================

// validCounterpartyIDTypes — допустимые типы идентификатора (страно/тип-зависимые).
var validCounterpartyIDTypes = map[string]bool{
	"inn": true, "bin": true, "iin": true, "passport": true, "other": true,
}

// ciLikeExpr — выражение регистронезависимого LIKE для кириллицы.
// На prod PG lc_collate=C → голый ILIKE не downcase'ит кириллицу, нужен COLLATE "und-x-icu"
// (см. dashboard_search.go). На sqlite (тесты) COLLATE-синтаксиса нет → обычный LOWER.
// Паттерн в аргументе должен быть уже в нижнем регистре.
func ciLikeExpr(col string) string {
	if database.DB != nil && database.DB.Dialector.Name() == "postgres" {
		return fmt.Sprintf(`LOWER(%s COLLATE "und-x-icu") LIKE ?`, col)
	}
	return fmt.Sprintf(`LOWER(%s) LIKE ?`, col)
}

// counterpartyScope — базовый запрос, ограниченный admin+company текущего оператора.
// Возвращает (query, adminAccountID, companyID, ok). При ok=false ответ уже отправлен.
func counterpartyScope(c *gin.Context) (*gorm.DB, uint, uint, bool) {
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return nil, 0, 0, false
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		// Денежная сущность: без определённой компании скоуп неполон → fail-closed.
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return nil, 0, 0, false
	}
	q := database.DB.Model(&models.Counterparty{}).
		Where("admin_account_id = ? AND company_id = ?", adminAccountID, companyID)
	return q, adminAccountID, companyID, true
}

// GetCounterparties — GET /api/auth/counterparties
// Список контрагентов компании. Фильтры: ?q= (имя/tax_id), ?manual_review=1, пагинация limit/offset.
func GetCounterparties(c *gin.Context) {
	// Ф4: менеджер видит ТОЛЬКО контрагентов своих договоров; admin/superadmin — всех (scope admin+company).
	q, _, _, ok := counterpartyScope(c)
	if !ok {
		return
	}
	q = applyCounterpartyManagerScope(c, q)

	if s := strings.TrimSpace(c.Query("q")); s != "" {
		pattern := "%" + strings.ToLower(s) + "%"
		// Скобки обязательны: иначе OR на верхнем уровне обходит admin/company-scope.
		q = q.Where("("+ciLikeExpr("name")+" OR "+ciLikeExpr("tax_id")+")", pattern, pattern)
	}
	if c.Query("manual_review") == "1" {
		q = q.Where("manual_review = ?", true)
	}

	var total int64
	q.Count(&total)

	limit := 100
	if v, e := strconv.Atoi(c.Query("limit")); e == nil && v > 0 && v <= 1000 {
		limit = v
	}
	offset := 0
	if v, e := strconv.Atoi(c.Query("offset")); e == nil && v > 0 {
		offset = v
	}

	var items []models.Counterparty
	if err := q.Order("name ASC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не удалось получить контрагентов"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": items, "count": len(items), "total": total})
}

// SearchCounterparties — GET /api/auth/counterparties/search?q=...
// Лёгкий autocomplete для формы договора (Ф4): id, name, tax_id, id_type, manual_review.
func SearchCounterparties(c *gin.Context) {
	q, _, _, ok := counterpartyScope(c)
	if !ok {
		return
	}
	q = applyCounterpartyManagerScope(c, q) // Ф4: менеджер — только свои контрагенты
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		pattern := "%" + strings.ToLower(s) + "%"
		// Скобки обязательны: иначе OR на верхнем уровне обходит admin/company-scope.
		q = q.Where("("+ciLikeExpr("name")+" OR "+ciLikeExpr("tax_id")+")", pattern, pattern)
	}
	type row struct {
		ID           uint   `json:"id"`
		Name         string `json:"name"`
		TaxID        string `json:"tax_id"`
		IDType       string `json:"id_type"`
		ManualReview bool   `json:"manual_review"`
	}
	var rows []row
	if err := q.Select("id, name, tax_id, id_type, manual_review").
		Order("name ASC").Limit(20).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "ошибка поиска"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": rows})
}

// GetCounterparty — GET /api/auth/counterparties/:id
func GetCounterparty(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	q, _, _, ok := counterpartyScope(c)
	if !ok {
		return
	}
	q = applyCounterpartyManagerScope(c, q) // Ф4: менеджер — только свои контрагенты
	var cp models.Counterparty
	if err := q.First(&cp, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "контрагент не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": cp})
}

// CreateCounterparty — POST /api/auth/counterparties
func CreateCounterparty(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "компания не определена в контексте"})
		return
	}

	var cp models.Counterparty
	if err := c.ShouldBindJSON(&cp); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные контрагента"})
		return
	}
	cp.Name = strings.TrimSpace(cp.Name)
	if cp.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "имя контрагента обязательно"})
		return
	}
	if cp.Country == "" {
		cp.Country = "ru"
	}
	if cp.IDType == "" {
		cp.IDType = "inn"
	}
	if !validCounterpartyIDTypes[cp.IDType] {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимый id_type"})
		return
	}
	cp.TaxID = strings.TrimSpace(cp.TaxID)
	if cp.BillingMode == "" {
		cp.BillingMode = "prepaid"
	}
	// Скоуп и аудит проставляем сервером — не доверяем телу запроса.
	cp.ID = 0
	cp.AdminAccountID = adminAccountID
	cp.CompanyID = companyID
	username, _ := c.Get("username")
	cp.CreatedBy, _ = username.(string)
	// Без tax_id — пометить на ручную проверку (как контрагенты из миграции по имени).
	if cp.TaxID == "" {
		cp.ManualReview = true
	}

	// Идемпотентность/дубль по идентичности (при непустом tax_id ловим до INSERT,
	// а гонку добивает партиальный uniqueIndex idx_cp_identity → 409).
	if cp.TaxID != "" {
		var existing models.Counterparty
		dup := database.DB.Where("admin_account_id = ? AND company_id = ? AND id_type = ? AND tax_id = ?",
			adminAccountID, companyID, cp.IDType, cp.TaxID).First(&existing)
		if dup.Error == nil {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "контрагент с таким идентификатором уже существует", "data": existing})
			return
		}
	}

	if err := database.DB.Create(&cp).Error; err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "контрагент с таким идентификатором уже существует"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не удалось создать контрагента"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": cp})
}

// UpdateCounterparty — PUT /api/auth/counterparties/:id
// Правит реквизиты/лимит/режим/идентификатор. Скоуп-ключи (admin/company) неизменны.
func UpdateCounterparty(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	q, adminAccountID, companyID, ok := counterpartyScope(c)
	if !ok {
		return
	}
	var existing models.Counterparty
	if err := q.First(&existing, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "контрагент не найден"})
		return
	}

	var in models.Counterparty
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректные данные"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "имя контрагента обязательно"})
		return
	}
	if in.IDType == "" {
		in.IDType = existing.IDType
	}
	if !validCounterpartyIDTypes[in.IDType] {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимый id_type"})
		return
	}
	// Защищаем неизменяемые/серверные поля.
	in.ID = existing.ID
	in.AdminAccountID = adminAccountID
	in.CompanyID = companyID
	in.CreatedBy = existing.CreatedBy
	in.CreatedAt = existing.CreatedAt
	in.TaxID = strings.TrimSpace(in.TaxID)
	if in.TaxID == "" {
		in.ManualReview = true
	}

	if err := database.DB.Model(&existing).Select("*").
		Omit("id", "admin_account_id", "company_id", "created_by", "created_at", "deleted_at").
		Updates(&in).Error; err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "контрагент с таким идентификатором уже существует"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не удалось обновить контрагента"})
		return
	}
	var updated models.Counterparty
	database.DB.First(&updated, existing.ID)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": updated})
}

// DeleteCounterparty — DELETE /api/auth/counterparties/:id
// Soft-delete. Запрещено, если на контрагента ссылаются договоры (tenant) или проводки (public):
// иначе осиротеет баланс. Сначала переназначь/удали договоры.
func DeleteCounterparty(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	q, adminAccountID, _, ok := counterpartyScope(c)
	if !ok {
		return
	}
	var cp models.Counterparty
	if err := q.First(&cp, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "контрагент не найден"})
		return
	}

	// Ссылки из проводок (public, scope по admin).
	var ledgerRefs int64
	database.DB.Model(&models.LedgerEntry{}).
		Where("counterparty_id = ? AND admin_account_id = ? AND deleted_at IS NULL", cp.ID, adminAccountID).
		Count(&ledgerRefs)
	if ledgerRefs > 0 {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "у контрагента есть проводки лицевого счёта — удаление запрещено"})
		return
	}
	// Ссылки из договоров (tenant-схема текущей компании).
	if tenantDB := middleware.GetTenantDB(c); tenantDB != nil {
		var contractRefs int64
		tenantDB.Model(&models.Contract{}).
			Where("counterparty_id = ?", cp.ID).Count(&contractRefs)
		if contractRefs > 0 {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "на контрагента ссылаются договоры — сначала переназначь их"})
			return
		}
	}

	if err := database.DB.Delete(&cp).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не удалось удалить контрагента"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "контрагент удалён"})
}

// cpNormalizeName — каноничное имя контрагента (схлопывание пробелов), как в datafix.
func cpNormalizeName(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// resolveOrCreateCounterparty — Ф4: находит контрагента по идентичности договора или создаёт
// нового (та же логика, что datafix Ф1). Закрывает Codex HIGH-3 — новые договоры входят в
// единый ЛС автоматически. Идентичность: есть ИНН → id_type=inn по ИНН; иначе по имени +
// manual_review. Возвращает counterparty_id. Гонку (две параллельные вставки) ловит
// партиальный uniqueIndex → повторный SELECT.
func resolveOrCreateCounterparty(adminAccountID, companyID uint, ct *models.Contract) (uint, error) {
	if adminAccountID == 0 || companyID == 0 {
		return 0, fmt.Errorf("counterparty: пустой admin/company scope")
	}
	inn := strings.TrimSpace(ct.ClientINN)
	name := cpNormalizeName(ct.ClientName)
	idType := "other"
	manualReview := true
	if inn != "" {
		idType = "inn"
		manualReview = false
	}

	find := func() (uint, bool) {
		var id uint
		q := database.DB.Model(&models.Counterparty{}).Select("id").
			Where("admin_account_id = ? AND company_id = ? AND deleted_at IS NULL", adminAccountID, companyID)
		if inn != "" {
			q = q.Where("id_type = ? AND tax_id = ?", idType, inn)
		} else {
			// Имя exact-match (case-preserved) — обходит lc_collate=C; зеркало datafix lookup.
			q = q.Where("(tax_id = '' OR tax_id IS NULL) AND name = ?", name)
		}
		q.Scan(&id)
		return id, id != 0
	}

	if id, ok := find(); ok {
		return id, nil
	}
	cp := models.Counterparty{
		AdminAccountID: adminAccountID, CompanyID: companyID, Country: "ru",
		IDType: idType, TaxID: inn, Name: name, ClientType: ct.ClientType,
		ShortName: ct.ClientShortName, KPP: ct.ClientKPP, Email: ct.ClientEmail, Phone: ct.ClientPhone,
		Address: ct.ClientAddress, LegalAddress: ct.ClientLegalAddress, PostalAddress: ct.ClientPostalAddress,
		OGRN: ct.ClientOGRN, OKPO: ct.ClientOKPO, Director: ct.ClientDirector, BasedOn: ct.ClientBasedOn,
		Website: ct.ClientWebsite, BankName: ct.ClientBankName, BankBIK: ct.ClientBankBIK,
		BankCorrespondentAccount: ct.ClientBankCorrespondentAccount, BankAccount: ct.ClientBankAccount,
		BankRecipient: ct.ClientBankRecipient, PassportSeries: ct.ClientPassportSeries,
		PassportNumber: ct.ClientPassportNumber, PassportIssuedBy: ct.ClientPassportIssuedBy,
		PassportIssueDate: ct.ClientPassportIssueDate, PassportDepartmentCode: ct.ClientPassportDepartmentCode,
		RegistrationAddress: ct.ClientRegistrationAddress, ActualAddress: ct.ClientActualAddress,
		SNILS: ct.ClientSNILS, OGRNIP: ct.ClientOGRNIP,
		BillingMode: defaultBillingMode(ct.BillingMode), CreditLimit: ct.CreditLimit,
		ManualReview: manualReview, CreatedBy: "auto:contract",
	}
	if name == "" {
		return 0, fmt.Errorf("counterparty: пустое имя клиента")
	}
	err := database.DB.Create(&cp).Error
	if err != nil {
		if isUniqueViolation(err) {
			// Гонка: кто-то создал того же контрагента параллельно → перечитать.
			if id, ok := find(); ok {
				return id, nil
			}
		}
		return 0, err
	}
	return cp.ID, nil
}

func defaultBillingMode(m string) string {
	if strings.TrimSpace(m) == "" {
		return "prepaid"
	}
	return m
}

// mirrorBillingToCounterparty — Ф2: при изменении billing_mode/credit_limit договора
// зеркалим значения на его контрагента (авторитетный источник для sweep/charge в единый ЛС).
// cp=0 → no-op (договор вне модели контрагентов). Полная миграция write-пути на форму
// контрагента — Ф4. credit_limit при prepaid должен приходить нулевым (контролирует вызывающий).
func mirrorBillingToCounterparty(counterpartyID, adminAccountID, companyID uint, billingMode string, creditLimit decimal.Decimal) {
	if counterpartyID == 0 || billingMode == "" {
		return
	}
	// Скоуп id+admin+company (id — PK, но фиксируем инвариант шардинга; admin общий по компаниям).
	database.DB.Model(&models.Counterparty{}).
		Where("id = ? AND admin_account_id = ? AND company_id = ?", counterpartyID, adminAccountID, companyID).
		Updates(map[string]interface{}{"billing_mode": billingMode, "credit_limit": creditLimit})
}

// isUniqueViolation — грубая проверка нарушения уникального индекса (PG: SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "23505") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique constraint")
}
