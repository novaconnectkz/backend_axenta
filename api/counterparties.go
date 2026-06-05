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
	// Phase D: фильтр по роли. ?kind=client|partner. Без параметра — справочник
	// показывает ВСЕХ (и клиентов, и партнёров; FE различает бейджем по полю kind).
	if k := strings.TrimSpace(c.Query("kind")); k == "client" || k == "partner" {
		q = q.Where("kind = ?", k)
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
	// Роль: client-договор → client; партнёрский → all (единый контрагент, любой kind);
	// явный partner → только партнёры. По умолчанию client (обратная совместимость).
	switch strings.TrimSpace(c.Query("kind")) {
	case "partner":
		q = q.Where("kind = ?", "partner")
	case "all":
		// без фильтра — все роли (единый контрагент)
	default:
		q = q.Where("kind = ?", "client")
	}
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

// GetCounterpartyBalance — GET /api/auth/counterparties/:id/balance
// Единый лицевой счёт контрагента: баланс (SUM по всем договорам) + разбивка + кол-во договоров.
func GetCounterpartyBalance(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	q, adminAccountID, companyID, ok := counterpartyScope(c)
	if !ok {
		return
	}
	q = applyCounterpartyManagerScope(c, q) // менеджер — только свои
	var cp models.Counterparty
	if err := q.First(&cp, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "контрагент не найден"})
		return
	}
	// Phase D hard-gate (Codex MEDIUM): партнёр НЕ субъект ЛС — не агрегируем ledger
	// (даже при случайной проводке не показываем как баланс). Отдаём справочный ответ:
	// kind + кол-во договоров, без charged/paid/balance/holds. FE рисует снимок-биллинг.
	if cp.Kind == "partner" {
		var partnerContracts int64
		if tenantDB := middleware.GetTenantDB(c); tenantDB != nil {
			tenantDB.Model(&models.Contract{}).
				Where("counterparty_id = ? AND deleted_at IS NULL", cp.ID).Count(&partnerContracts)
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
			"counterparty_id": cp.ID,
			"name":            cp.Name,
			"kind":            "partner",
			"is_ledger":       false, // партнёр вне лицевого счёта
			"contracts_count": partnerContracts,
		}})
		return
	}
	charged, paid := ledgerBreakdown(0, cp.ID, adminAccountID, companyID) // cp<>0 → агрегат контрагента
	balance := paid.Sub(charged)
	// Кол-во client-договоров контрагента (tenant-схема текущей компании).
	var contractsCount int64
	if tenantDB := middleware.GetTenantDB(c); tenantDB != nil {
		tenantDB.Model(&models.Contract{}).
			Where("counterparty_id = ? AND deleted_at IS NULL", cp.ID).Count(&contractsCount)
	}
	data := gin.H{
		"counterparty_id": cp.ID,
		"name":            cp.Name,
		"kind":            cp.Kind, // Phase D: 'partner' → FE скрывает баланс/платёж/holds (партнёр не субъект ЛС)
		"total_charged":   charged.StringFixed(2),
		"total_paid":      paid.StringFixed(2),
		"credit_limit":    cp.CreditLimit.StringFixed(2), // порог применяется per-currency
		"billing_mode":    cp.BillingMode,
		"contracts_count": contractsCount,
	}
	// Мультивалюта: balance — свёртка по курсу в доминирующую валюту (или суб-баланс этой
	// валюты при stale). Одновалютный (прод) — прежний balance=paid−charged, без новых полей.
	subs := ledgerBreakdownByCurrency(0, cp.ID, adminAccountID, companyID)
	if len(subs) > 1 {
		target := "RUB"
		maxCharged := decimal.Zero
		for _, s := range subs {
			if s.Charged.GreaterThan(maxCharged) {
				maxCharged = s.Charged
				target = contractCurrency(s.Currency)
			}
		}
		data["multicurrency"] = true
		data["sub_balances"] = subs
		data["presentation_currency"] = target
		if pres, ok := presentBalance(subs, target, companyID); ok {
			balance = pres
			data["presentation_only"] = true
		} else {
			balance = decimal.Zero
			for _, s := range subs {
				if contractCurrency(s.Currency) == target {
					balance = s.Balance
				}
			}
		}
	}
	debt := decimal.Zero
	if balance.IsNegative() {
		debt = balance.Abs()
	}
	data["balance"] = balance.StringFixed(2)
	data["is_debt"] = balance.IsNegative()
	data["debt_amount"] = debt.StringFixed(2)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

// Phase C — дом идентичности партнёра: партнёр НЕ субъект лицевого счёта (биллинг
// snapshot-based), его имя/ИНН/реквизиты живут в partner_* ДОГОВОРА, не в Counterparty.
// FE шлёт идентичность в (транзитных) client_*; BE роутит client_*→partner_*.
// Логика — методы models.Contract (runtime и backfill зовут одно). Тонкая обёртка ниже.
// (C4b: snapshot client_*-dual-write удалён — колонки client_* дропнуты, авторитет на Counterparty.)

func applyPartnerIdentityFromContract(ct *models.Contract) {
	if ct != nil {
		ct.ApplyPartnerIdentityFromClient()
	}
}

func partnerIdentityMap(ct *models.Contract) map[string]any {
	if ct == nil {
		return map[string]any{}
	}
	return ct.PartnerIdentityMap()
}

// GetCounterpartyContracts — GET /api/auth/counterparties/:id/contracts
// Договоры контрагента (для карточки контрагента, Фаза A IA-реструктуризации). Read-only.
// Доступ как у balance: видимость контрагента гейтится менеджер-скоупом (видит cp, если есть
// хотя бы один свой договор), но СПИСОК отдаёт ВСЕ договоры контрагента (решение владельца:
// полный ЛС на чтение). Денежные действия по ним остаются под admin-гейтом в своих endpoint'ах.
func GetCounterpartyContracts(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id"})
		return
	}
	q, adminAccountID, companyID, ok := counterpartyScope(c)
	if !ok {
		return
	}
	q = applyCounterpartyManagerScope(c, q) // гейт видимости контрагента
	var cp models.Counterparty
	if err := q.First(&cp, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "контрагент не найден"})
		return
	}
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": []gin.H{}})
		return
	}
	var contracts []models.Contract
	tenantDB.Where("counterparty_id = ? AND deleted_at IS NULL", cp.ID).
		Order("created_at DESC").Find(&contracts)
	attachCounterparties(contracts) // C4a: единый FE-display (cp в public, tenantDB не видит)

	// Тарифные планы — в public-схеме, подгружаем именами одним запросом.
	planNames := map[uint]string{}
	planPrices := map[uint]decimal.Decimal{}
	var planIDs []uint
	for i := range contracts {
		if contracts[i].TariffPlanID != nil && *contracts[i].TariffPlanID > 0 {
			planIDs = append(planIDs, *contracts[i].TariffPlanID)
		}
	}
	if len(planIDs) > 0 {
		var plans []models.BillingPlan
		database.DB.Where("id IN ? AND admin_account_id = ?", planIDs, adminAccountID).Find(&plans)
		for _, p := range plans {
			planNames[p.ID] = p.Name
			planPrices[p.ID] = p.Price
		}
	}

	out := make([]gin.H, 0, len(contracts))
	for i := range contracts {
		ct := &contracts[i]
		bal := balanceForContractCcy(ct.ID, 0, adminAccountID, companyID, ct.Currency)
		row := gin.H{
			"id":            ct.ID,
			"number":        ct.Number,
			"contract_type": ct.ContractType,
			"status":        ct.Status,
			"is_active":     ct.IsActive,
			"currency":      ct.Currency,
			"total_amount":  ct.TotalAmount.StringFixed(2),
			"balance":       bal.StringFixed(2),
			"is_debt":       bal.IsNegative(),
			"start_date":    ct.StartDate,
			"end_date":      ct.EndDate,
			"manager_name":  ct.ManagerName,
		}
		if ct.TariffPlanID != nil {
			row["tariff_name"] = planNames[*ct.TariffPlanID]
			if price, ok := planPrices[*ct.TariffPlanID]; ok {
				row["tariff_price"] = price.StringFixed(2)
			}
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": out})
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
	// Роль (kind): client по умолчанию; явно client|partner из формы.
	if cp.Kind == "" {
		cp.Kind = "client"
	}
	if cp.Kind != "client" && cp.Kind != "partner" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "недопустимая роль контрагента"})
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
	q, _, _, ok := counterpartyScope(c) // scope уже в q (First грузит existing в admin+company)
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
	in.TaxID = strings.TrimSpace(in.TaxID)
	// Появился валидный tax_id → флаг «нет ИНН, ручная проверка» снимается (Codex).
	manualReview := in.TaxID == ""

	// Map-based Updates: пишем ТОЛЬКО явно перечисленные редактируемые колонки реквизитов.
	// Дискриминатор kind (роль client|partner) и серверные поля (id/admin/company/created_*/
	// updated_at/deleted_at) НЕ в карте → структурно защищены (партнёр не затрётся в '').
	// Это заменяет хрупкий Select("*"), который писал ВСЕ колонки, включая незаполненные.
	updates := map[string]any{
		"client_type":                in.ClientType,
		"name":                       in.Name,
		"short_name":                 in.ShortName,
		"country":                    in.Country,
		"id_type":                    in.IDType,
		"tax_id":                     in.TaxID,
		"kpp":                        in.KPP,
		"ogrn":                       in.OGRN,
		"okpo":                       in.OKPO,
		"email":                      in.Email,
		"phone":                      in.Phone,
		"website":                    in.Website,
		"address":                    in.Address,
		"legal_address":              in.LegalAddress,
		"postal_address":             in.PostalAddress,
		"director":                   in.Director,
		"based_on":                   in.BasedOn,
		"bank_name":                  in.BankName,
		"bank_bik":                   in.BankBIK,
		"bank_correspondent_account": in.BankCorrespondentAccount,
		"bank_account":               in.BankAccount,
		"bank_recipient":             in.BankRecipient,
		"passport_series":            in.PassportSeries,
		"passport_number":            in.PassportNumber,
		"passport_issued_by":         in.PassportIssuedBy,
		"passport_issue_date":        in.PassportIssueDate,
		"passport_department_code":   in.PassportDepartmentCode,
		"registration_address":       in.RegistrationAddress,
		"actual_address":             in.ActualAddress,
		"snils":                      in.SNILS,
		"ogrn_ip":                    in.OGRNIP, // колонка-override (поле OGRNIP)
		"billing_mode":               in.BillingMode,
		"credit_limit":               in.CreditLimit,
		"manual_review":              manualReview,
	}
	// Роль (kind) меняется ТОЛЬКО на валидное значение (защита от затирания партнёра в '' старой формой).
	if in.Kind == "client" || in.Kind == "partner" {
		updates["kind"] = in.Kind
	}

	if err := database.DB.Model(&existing).Updates(updates).Error; err != nil {
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

// firstNonEmpty — первая непустая (после trim) строка из переданных.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// resolveOrCreateCounterparty — Ф4: находит контрагента по идентичности договора или создаёт
// нового (та же логика, что datafix Ф1). Закрывает Codex HIGH-3 — новые договоры входят в
// единый ЛС автоматически. Идентичность: есть ИНН → id_type=inn по ИНН; иначе по имени +
// manual_review. Возвращает counterparty_id. Гонку (две параллельные вставки) ловит
// партиальный uniqueIndex → повторный SELECT.
// resolveOrCreatePartnerCounterparty — Phase D: партнёрский договор → контрагент
// kind='partner' (СПРАВОЧНИК идентичности, НЕ субъект ЛС). Идентичность из Partner*
// (дом C-партнёра) с fallback на Client* (snapshot). find() скоупится kind='partner' —
// партнёр НЕ матчит клиентского cp с тем же ИНН (и наоборот). Биллинг НЕ копируется
// (prepaid/0) — баланс/начисления к партнёру не применяются. No-op для не-партнёра.
func resolveOrCreatePartnerCounterparty(adminAccountID, companyID uint, ct *models.Contract) (uint, error) {
	if ct.ContractType != "partner" {
		return 0, nil
	}
	if adminAccountID == 0 || companyID == 0 {
		return 0, fmt.Errorf("partner counterparty: пустой admin/company scope")
	}
	name := cpNormalizeName(firstNonEmpty(ct.PartnerName, ct.ClientName))
	inn := strings.TrimSpace(firstNonEmpty(ct.PartnerINN, ct.ClientINN))
	if name == "" {
		return 0, fmt.Errorf("partner counterparty: пустое имя партнёра")
	}
	idType := "other"
	manualReview := true
	if inn != "" {
		idType = "inn"
		manualReview = false
	}

	find := func() (uint, bool) {
		var id uint
		q := database.DB.Model(&models.Counterparty{}).Select("id").
			Where("admin_account_id = ? AND company_id = ? AND kind = ? AND deleted_at IS NULL", adminAccountID, companyID, "partner")
		if inn != "" {
			q = q.Where("id_type = ? AND tax_id = ?", idType, inn)
		} else {
			q = q.Where("(tax_id = '' OR tax_id IS NULL) AND name = ?", name)
		}
		q.Scan(&id)
		return id, id != 0
	}

	if id, ok := find(); ok {
		return id, nil
	}
	cp := models.Counterparty{
		AdminAccountID: adminAccountID, CompanyID: companyID, Country: "ru", Kind: "partner",
		IDType: idType, TaxID: inn, Name: name, ClientType: ct.ClientType,
		ShortName: ct.ClientShortName, KPP: ct.ClientKPP, Email: ct.ClientEmail, Phone: ct.ClientPhone,
		Address: ct.ClientAddress, LegalAddress: ct.ClientLegalAddress, PostalAddress: ct.ClientPostalAddress,
		OGRN: ct.ClientOGRN, OKPO: ct.ClientOKPO, Director: ct.ClientDirector, BasedOn: ct.ClientBasedOn,
		Website: ct.ClientWebsite, BankName: ct.ClientBankName, BankBIK: ct.ClientBankBIK,
		BankCorrespondentAccount: ct.ClientBankCorrespondentAccount, BankAccount: ct.ClientBankAccount,
		BankRecipient: ct.ClientBankRecipient,
		// Биллинг НЕ копируем: партнёр не субъект ЛС (snapshot-биллинг). prepaid/0 — нейтрально.
		BillingMode: "prepaid", CreditLimit: decimal.Zero,
		ManualReview: manualReview, CreatedBy: "auto:partner-contract",
	}
	if err := database.DB.Create(&cp).Error; err != nil {
		if isUniqueViolation(err) {
			if id, ok := find(); ok {
				return id, nil
			}
		}
		return 0, err
	}
	return cp.ID, nil
}

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
