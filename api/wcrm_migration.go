package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============================================================================
// WCRM → ACRM миграция Axenta-договоров (временная фича, super_admin only).
//
// Источник: JSON snapshot /var/imports/wcrm-axenta-import-<date>.json (Этап A).
// Цель: tenant_186.contracts/contract_appendices + public.billing_history (остаток).
//
// UX: GET /preview показывает что и куда мигрируется (per-company), POST /approve
// импортирует одну компанию атомарно. Идемпотентность через external_id-индексы
// (миграция 0069) и metadata для billing_history.
// ============================================================================

const (
	wcrmImportDefaultPath = "/var/imports/wcrm-axenta-import-2026-05-28.json"
	wcrmTargetTenantID    = 186
)

// wcrmImportPath — путь к snapshot (override через env WCRM_IMPORT_PATH).
func wcrmImportPath() string {
	if p := os.Getenv("WCRM_IMPORT_PATH"); p != "" {
		return p
	}
	return wcrmImportDefaultPath
}

// ---- Структуры JSON snapshot ----

type wcrmCompany struct {
	Name           string `json:"name"`
	Name1          string `json:"name1"`
	Type           int    `json:"type"`
	INN            string `json:"inn"`
	KPP            string `json:"kpp"`
	BIK            string `json:"bik"`
	BankName       string `json:"bank_name"`
	CurA           string `json:"cur_a"`
	CorA           string `json:"cor_a"`
	OGRN           string `json:"ogrn"`
	OKPO           string `json:"okpo"`
	OKVED          string `json:"okved"`
	AddressU       string `json:"address_u"`
	AddressF       string `json:"address_f"`
	AddressP       string `json:"address_p"`
	HeadName       string `json:"head_name"`
	AccountantName string `json:"accountant_name"`
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	Description    string `json:"description"`
}

type wcrmObject struct {
	WcrmObjectID int64  `json:"wcrm_object_id"`
	Name         string `json:"name"`
	UID          string `json:"uid"`
	WID          int64  `json:"wid"`
}

type wcrmAppendix struct {
	WcrmAttachmentID int64        `json:"wcrm_attachment_id"`
	StartDate        string       `json:"start_date"`
	EndDate          string       `json:"end_date"`
	Price            float64      `json:"price"`
	Type             int          `json:"type"`
	Period           int          `json:"period"`
	Daycount         *int         `json:"daycount"`
	Name             *string      `json:"name"`
	Enabled          int          `json:"enabled"`
	Objects          []wcrmObject `json:"objects"`
}

type wcrmContract struct {
	WcrmContractID int64          `json:"wcrm_contract_id"`
	Number         string         `json:"number"`
	StartDate      string         `json:"start_date"`
	EndDate        *string        `json:"end_date"`
	Postpaid       int            `json:"postpaid"`
	Appendices     []wcrmAppendix `json:"appendices"`
}

type wcrmRecord struct {
	WcrmCompanyID int64          `json:"wcrm_company_id"`
	Company       wcrmCompany    `json:"company"`
	Balance       float64        `json:"balance"`
	Contracts     []wcrmContract `json:"contracts"`
}

// ---- Preview-ответ ----

type wcrmPreviewAppendix struct {
	WcrmAttachmentID int64    `json:"wcrm_attachment_id"`
	Title            string   `json:"title"`
	Price            string   `json:"price"`
	Period           int      `json:"period"`
	StartDate        string   `json:"start_date"` // дата начала приложения (YYYY-MM-DD), не договора
	EndDate          string   `json:"end_date"`   // дата окончания приложения (YYYY-MM-DD)
	Enabled          bool     `json:"enabled"`
	ObjectCount      int      `json:"object_count"`
	ObjectNames      []string `json:"object_names"` // имена для показа (до 50)
}

type wcrmPreviewContract struct {
	WcrmContractID int64                 `json:"wcrm_contract_id"`
	SourceNumber   string                `json:"source_number"`
	TargetNumber   string                `json:"target_number"` // с суффиксом если конфликт
	NumberConflict bool                  `json:"number_conflict"`
	StartDate      string                `json:"start_date"`
	EndDate        string                `json:"end_date"`
	Status         string                `json:"status"`
	AppendixCount  int                   `json:"appendix_count"`
	BadDates       int                   `json:"bad_dates"` // приложений с невалидными датами
	ObjectCount    int                   `json:"object_count"`
	ObjectsMatched int                   `json:"objects_matched"` // объектов найдено в ACRM по uid
	WillSkip       bool                  `json:"will_skip"`       // нет включённых приложений → не мигрируется
	Appendices     []wcrmPreviewAppendix `json:"appendices"`
}

type wcrmPreviewCandidate struct {
	WcrmCompanyID  int64                 `json:"wcrm_company_id"`
	ClientName     string                `json:"client_name"`
	ClientINN      string                `json:"client_inn"`
	ClientType     string                `json:"client_type"`      // organization|individual_entrepreneur|physical_person
	ClientTypeNote string                `json:"client_type_note"` // "manual_review" если не определён
	ContractsCount int                   `json:"contracts_count"`
	AppendixCount  int                   `json:"appendix_count"`
	ObjectCount    int                   `json:"object_count"`
	Balance        string                `json:"balance"`
	BalanceTarget  string                `json:"balance_target"` // billing_history(payment_received|balance_debt) или "—"
	Contracts      []wcrmPreviewContract `json:"contracts"`
	AlreadyImported bool                 `json:"already_imported"`
	Status         string                `json:"status"` // pending|approved|skipped|error (из state)
	HasIssues      bool                  `json:"has_issues"`
}

type wcrmPreviewResponse struct {
	SnapshotSHA256 string                 `json:"snapshot_sha256"`
	GeneratedAt    string                 `json:"generated_at"`
	TotalCompanies int                    `json:"total_companies"`
	TotalContracts int                    `json:"total_contracts"`
	TotalAppendix  int                    `json:"total_appendix"`
	Approved       int                    `json:"approved"`
	Skipped        int                    `json:"skipped"`
	Conflicts      int                    `json:"conflicts"`
	Candidates     []wcrmPreviewCandidate `json:"candidates"`
}

// ---- Вспомогательные ----

// loadWcrmSnapshot читает + парсит JSON, возвращает записи и sha256 файла.
func loadWcrmSnapshot() ([]wcrmRecord, string, error) {
	raw, err := os.ReadFile(wcrmImportPath())
	if err != nil {
		return nil, "", fmt.Errorf("чтение snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	var recs []wcrmRecord
	if err := json.Unmarshal(raw, &recs); err != nil {
		return nil, sha, fmt.Errorf("парсинг snapshot: %w", err)
	}
	return recs, sha, nil
}

// resolveClientType определяет тип клиента. Возвращает (type, needsManualReview).
//
// В WCRM ИНН/КПП/ОГРН у Axenta-клиентов почти везде пусты (ИНН заполнен у ~4 из 207),
// поэтому основной сигнал — суффикс/форма в названии. ИНН используется как уточнение.
//   - ИНН 10 цифр → organization (точно)
//   - ИНН 12 цифр → individual_entrepreneur (точно)
//   - иначе по названию: ООО/АО/ПАО/... → organization; "ИП"/"предприниматель" → ИП;
//     ФИО (нет орг-формы) → physical_person; пустое имя → manual_review.
func resolveClientType(c wcrmCompany) (string, bool) {
	inn := strings.TrimSpace(c.INN)
	switch len(inn) {
	case 10:
		return "organization", false
	case 12:
		return "individual_entrepreneur", false
	}

	name := strings.ToLower(strings.TrimSpace(c.Name1))
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(c.Name))
	}
	if name == "" {
		return "organization", true // нет данных — ручной выбор
	}

	// Орг-формы юрлица
	for _, f := range []string{"ооо", "оао", "зао", "пао", "ао ", " ао", "нко", "тоо", "общество с ограниченной", "акционерное общество"} {
		if strings.Contains(name, f) {
			return "organization", false
		}
	}
	// ИП
	if strings.Contains(name, "индивидуальный предприниматель") ||
		strings.HasPrefix(name, "ип ") || strings.Contains(name, " ип ") ||
		strings.HasSuffix(name, " ип") {
		return "individual_entrepreneur", false
	}
	// ФИО из 2-3 слов без цифр и форм → физлицо
	words := strings.Fields(name)
	hasDigit := strings.ContainsAny(name, "0123456789")
	if (len(words) == 2 || len(words) == 3) && !hasDigit {
		return "physical_person", false
	}

	// Неоднозначно — по умолчанию организация, без блокировки (юзер approve вручную,
	// видит тип и может переопределить в селекторе).
	return "organization", false
}

// mapClientName выбирает полное имя клиента (name1 приоритетнее короткого name).
func mapClientName(c wcrmCompany) string {
	if strings.TrimSpace(c.Name1) != "" {
		return c.Name1
	}
	return c.Name
}

// parseWcrmDate парсит WCRM timestamp "2025-05-21 06:04:04".
// Возвращает (time, ok). '0000-00-00...' и пустые → ok=false.
func parseWcrmDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "0000-00-00") {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// contractPeriodFromAppendices вычисляет период договора из ВКЛЮЧЁННЫХ приложений:
// StartDate = min(start), EndDate = max(end). Конец приложения = parse(end_date) либо
// start + period месяцев (в WCRM end часто 0000-00-00). Возвращает (start, end, ok);
// ok=false если нет включённых приложений с валидной датой старта → caller берёт даты договора.
// Период договора в WCRM (ct.StartDate) — дата подписания, не отражает реальный срок услуги;
// пользователь хочет видеть период по приложению.
func contractPeriodFromAppendices(ct wcrmContract) (time.Time, *time.Time, bool) {
	var minStart time.Time
	var maxEnd *time.Time
	found := false
	for _, ap := range ct.Appendices {
		if ap.Enabled == 0 {
			continue
		}
		st, okS := parseWcrmDate(ap.StartDate)
		if !okS {
			continue
		}
		if !found || st.Before(minStart) {
			minStart = st
		}
		found = true
		var en *time.Time
		if e, okE := parseWcrmDate(ap.EndDate); okE {
			en = &e
		} else if ap.Period > 0 {
			e := st.AddDate(0, ap.Period, 0)
			en = &e
		}
		if en != nil && (maxEnd == nil || en.After(*maxEnd)) {
			maxEnd = en
		}
	}
	return minStart, maxEnd, found
}

// deriveContractStatus: active если end_date пуст или в будущем, иначе expired.
func deriveContractStatus(endStr *string) (string, bool) {
	if endStr == nil {
		return "active", true
	}
	end, ok := parseWcrmDate(*endStr)
	if !ok {
		return "active", true
	}
	if time.Now().After(end) {
		return "expired", false
	}
	return "active", true
}

// advisoryKey — стабильный int64-ключ для pg_try_advisory_xact_lock.
func advisoryKey(companyID int64) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("wcrm:" + strconv.FormatInt(companyID, 10)))
	return int64(h.Sum64() >> 1) // >>1 чтобы влезло в signed bigint без переполнения
}

// requireSuperadmin — guard: только is_superadmin claim из доверенного JWT.
// Обычный admin НЕ допускается (миграция данных = высший привилегированный доступ).
func requireSuperadmin(c *gin.Context) bool {
	v, _ := c.Get("is_superadmin")
	if b, ok := v.(bool); ok && b {
		return true
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"status": "error",
		"error":  "Доступ только для суперадминистратора",
	})
	return false
}

// ---- Endpoints ----

// GetWcrmMigrationPreview — GET /api/auth/migration/wcrm/preview
// Показывает что и куда мигрируется (read-only).
func GetWcrmMigrationPreview(c *gin.Context) {
	if !requireSuperadmin(c) {
		return
	}
	recs, sha, err := loadWcrmSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	if database.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "БД недоступна"})
		return
	}
	schema := wcrmSchema()

	// Существующие WCRM-импортированные external_id (для already_imported).
	// Fully-qualified имена — не зависим от search_path / connection pooling.
	existingExtIDs := map[string]bool{}
	var existing []struct{ ExternalID string }
	database.DB.Raw(fmt.Sprintf(`SELECT external_id FROM %s.contracts WHERE external_id LIKE 'wcrm:contract:%%' AND deleted_at IS NULL`, schema)).Scan(&existing)
	for _, e := range existing {
		existingExtIDs[e.ExternalID] = true
	}

	// Существующие НЕ-wcrm номера договоров (для конфликтов).
	existingNumbers := map[string]bool{}
	var nums []struct{ Number string }
	database.DB.Raw(fmt.Sprintf(`SELECT number FROM %s.contracts WHERE deleted_at IS NULL AND (external_id IS NULL OR external_id NOT LIKE 'wcrm:contract:%%')`, schema)).Scan(&nums)
	for _, n := range nums {
		existingNumbers[n.Number] = true
	}

	// Состояние из wcrm_migration_state.
	stateByCompany := map[int64]string{}
	var states []struct {
		WcrmCompanyID int64
		Status        string
	}
	database.DB.Raw(fmt.Sprintf(`SELECT wcrm_company_id, status FROM %s.wcrm_migration_state`, schema)).Scan(&states)
	for _, s := range states {
		stateByCompany[s.WcrmCompanyID] = s.Status
	}

	// uid → ACRM Axenta-объект (для подсчёта сматченных объектов в preview).
	axentaObjMap := loadAxentaObjectMap(wcrmSchema())

	resp := wcrmPreviewResponse{
		SnapshotSHA256: sha,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		TotalCompanies: len(recs),
	}

	for _, r := range recs {
		ctype, manual := resolveClientType(r.Company)
		cand := wcrmPreviewCandidate{
			WcrmCompanyID:  r.WcrmCompanyID,
			ClientName:     mapClientName(r.Company),
			ClientINN:      r.Company.INN,
			ClientType:     ctype,
			ContractsCount: len(r.Contracts),
			Balance:        decimal.NewFromFloat(r.Balance).StringFixed(2),
			Status:         stateByCompany[r.WcrmCompanyID],
		}
		if manual {
			cand.ClientTypeNote = "manual_review"
			cand.HasIssues = true
		}

		// Balance target
		switch {
		case r.Balance > 0:
			cand.BalanceTarget = "лицевой счёт (предоплата)"
		case r.Balance < 0:
			cand.BalanceTarget = "лицевой счёт (долг)"
		default:
			cand.BalanceTarget = "—"
		}

		allImported := len(r.Contracts) > 0
		for _, ct := range r.Contracts {
			extID := "wcrm:contract:" + strconv.FormatInt(ct.WcrmContractID, 10)
			if !existingExtIDs[extID] {
				allImported = false
			}
			resp.TotalContracts++

			status, _ := deriveContractStatus(ct.EndDate)
			target := ct.Number
			conflict := false
			if existingNumbers[ct.Number] {
				conflict = true
				target = ct.Number + " (WCRM #" + strconv.FormatInt(ct.WcrmContractID, 10) + ")"
				resp.Conflicts++
				cand.HasIssues = true
			}

			badDates := 0
			ctObjCount := 0
			ctObjMatched := 0
			ctAnyEnabled := false
			previewAppendices := make([]wcrmPreviewAppendix, 0, len(ct.Appendices))
			for _, ap := range ct.Appendices {
				resp.TotalAppendix++
				cand.AppendixCount++
				if ap.Enabled != 0 {
					ctAnyEnabled = true
				}
				apStartT, okS := parseWcrmDate(ap.StartDate)
				apEndT, okE := parseWcrmDate(ap.EndDate)
				// В WCRM end_date часто 0000-00-00 (хранится только start + period).
				// Реальный срок приложения = start + period месяцев (как считает UI WCRM).
				if !okE && okS && ap.Period > 0 {
					apEndT = apStartT.AddDate(0, ap.Period, 0)
					okE = true
				}
				if !okS {
					badDates++
				}
				apStartStr, apEndStr := "", ""
				if okS {
					apStartStr = apStartT.Format("2006-01-02")
				}
				if okE {
					apEndStr = apEndT.Format("2006-01-02")
				}
				apTitle := "Приложение"
				if ap.Name != nil && strings.TrimSpace(*ap.Name) != "" {
					apTitle = *ap.Name
				}
				names := make([]string, 0, len(ap.Objects))
				for _, ob := range ap.Objects {
					if len(names) < 50 {
						names = append(names, ob.Name)
					}
					if _, ok := axentaObjMap[ob.UID]; ok {
						ctObjMatched++
					}
				}
				ctObjCount += len(ap.Objects)
				cand.ObjectCount += len(ap.Objects)
				previewAppendices = append(previewAppendices, wcrmPreviewAppendix{
					WcrmAttachmentID: ap.WcrmAttachmentID,
					Title:            apTitle,
					Price:            decimal.NewFromFloat(ap.Price).StringFixed(2),
					Period:           ap.Period,
					StartDate:        apStartStr,
					EndDate:          apEndStr,
					Enabled:          ap.Enabled != 0,
					ObjectCount:      len(ap.Objects),
					ObjectNames:      names,
				})
			}

			endStr := ""
			if ct.EndDate != nil {
				endStr = *ct.EndDate
			}
			// Период по приложениям (как при import), а не дата подписания договора.
			ctStartStr, ctEndStr := ct.StartDate, endStr
			if s, e, ok := contractPeriodFromAppendices(ct); ok {
				ctStartStr = s.Format("2006-01-02")
				if e != nil {
					ctEndStr = e.Format("2006-01-02")
				} else {
					ctEndStr = ""
				}
			}
			cand.Contracts = append(cand.Contracts, wcrmPreviewContract{
				WcrmContractID: ct.WcrmContractID,
				SourceNumber:   ct.Number,
				TargetNumber:   target,
				NumberConflict: conflict,
				StartDate:      ctStartStr,
				EndDate:        ctEndStr,
				Status:         status,
				AppendixCount:  len(ct.Appendices),
				BadDates:       badDates,
				ObjectCount:    ctObjCount,
				ObjectsMatched: ctObjMatched,
				WillSkip:       !ctAnyEnabled,
				Appendices:     previewAppendices,
			})
		}

		cand.AlreadyImported = allImported && len(r.Contracts) > 0
		if st := stateByCompany[r.WcrmCompanyID]; st == "approved" {
			resp.Approved++
		} else if st == "skipped" {
			resp.Skipped++
		}
		resp.Candidates = append(resp.Candidates, cand)
	}

	// Сортировка: сначала с issues, потом по имени.
	sort.SliceStable(resp.Candidates, func(i, j int) bool {
		if resp.Candidates[i].HasIssues != resp.Candidates[j].HasIssues {
			return resp.Candidates[i].HasIssues
		}
		return resp.Candidates[i].ClientName < resp.Candidates[j].ClientName
	})

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": resp})
}

// wcrmSchema — имя целевой tenant-схемы.
func wcrmSchema() string {
	return "tenant_" + strconv.Itoa(wcrmTargetTenantID)
}

// ---- Approve / Skip / Undo / Status ----

type wcrmApproveRequest struct {
	SnapshotSHA256    string `json:"snapshot_sha256"`
	ClientTypeOverride string `json:"client_type_override"` // для manual_review
}

type wcrmApproveResult struct {
	WcrmCompanyID        int64    `json:"wcrm_company_id"`
	ContractsInserted    int      `json:"contracts_inserted"`
	ContractsUpdated     int      `json:"contracts_updated"`
	SubscriptionsCreated int      `json:"subscriptions_created"`
	ObjectsLinked        int      `json:"objects_linked"`
	ObjectsUnmatched     int      `json:"objects_unmatched"`
	SkippedInactive      int      `json:"skipped_inactive"`     // договоров пропущено (все приложения disabled)
	TariffNotFound       []string `json:"tariff_not_found"`     // цены без тарифа → подписка не создана
	BalanceRecorded      bool     `json:"balance_recorded"`
	CreatedContractIDs   []uint   `json:"created_contract_ids"`
}

// PostWcrmMigrationApprove — POST /api/auth/migration/wcrm/approve/:wcrm_company_id
// Атомарно импортирует одну WCRM-компанию (договоры + приложения + остаток).
func PostWcrmMigrationApprove(c *gin.Context) {
	if !requireSuperadmin(c) {
		return
	}
	companyID, err := strconv.ParseInt(c.Param("wcrm_company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный wcrm_company_id"})
		return
	}
	var req wcrmApproveRequest
	_ = c.ShouldBindJSON(&req)

	recs, sha, err := loadWcrmSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}
	// Защита от рассинхрона preview↔approve (snapshot подменили).
	if req.SnapshotSHA256 != "" && req.SnapshotSHA256 != sha {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "snapshot изменился — обновите preview", "current_sha256": sha})
		return
	}

	var rec *wcrmRecord
	for i := range recs {
		if recs[i].WcrmCompanyID == companyID {
			rec = &recs[i]
			break
		}
	}
	if rec == nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "компания не найдена в snapshot"})
		return
	}

	// admin_account_id = account_id (как GetContracts фильтрует список), НЕ user_id.
	// Иначе импортированные договоры невидимы в /billing (admin=186 vs user_id=3).
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err != nil || adminAccountID == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "не удалось определить admin_account_id"})
		return
	}
	username, _ := c.Get("username")
	approvedBy, _ := username.(string)

	ctype, manual := resolveClientType(rec.Company)
	if manual {
		if req.ClientTypeOverride == "" {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "client_type не определён по ИНН — требуется client_type_override"})
			return
		}
		ctype = req.ClientTypeOverride
	}

	result := wcrmApproveResult{WcrmCompanyID: companyID}

	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		// Изоляция схемы + защита от параллельного approve той же компании.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL search_path TO %s, public", wcrmSchema())).Error; err != nil {
			return err
		}
		if err := tx.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
			return err
		}
		var locked bool
		if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", advisoryKey(companyID)).Scan(&locked).Error; err != nil {
			return err
		}
		if !locked {
			return errWcrmLocked
		}

		clientName := mapClientName(rec.Company)

		// uid → ACRM Axenta-объект (для привязки объектов подписки по uid-матчу).
		// WCRM ax-объекты = Axenta Cloud; uid == axenta_object_snapshots.unique_id.
		axentaObjMap := loadAxentaObjectMap(wcrmSchema())

		// Все обработанные (created+updated) договоры ЭТОЙ компании — для привязки
		// остатка к первому договору компании (не глобального min, который мог быть чужим).
		processedContractIDs := make([]uint, 0, len(rec.Contracts))

		for _, ct := range rec.Contracts {
			extID := "wcrm:contract:" + strconv.FormatInt(ct.WcrmContractID, 10)

			// Skip неактивных: договор без включённых приложений (все enabled=0) не мигрируем.
			anyEnabled := false
			for _, ap := range ct.Appendices {
				if ap.Enabled != 0 {
					anyEnabled = true
					break
				}
			}
			if !anyEnabled {
				result.SkippedInactive++
				continue
			}

			// Резолв номера: конфликт с НЕ-wcrm договором → детерминированный суффикс.
			targetNumber := ct.Number
			var conflictCount int64
			tx.Raw(`SELECT COUNT(*) FROM contracts WHERE number = ? AND deleted_at IS NULL AND (external_id IS NULL OR external_id <> ?)`, ct.Number, extID).Scan(&conflictCount)
			if conflictCount > 0 {
				targetNumber = ct.Number + " (WCRM #" + strconv.FormatInt(ct.WcrmContractID, 10) + ")"
			}

			// Договор активен (есть включённое приложение, прошёл skip выше).
			status := "active"
			isActive := true
			// Период договора = min/max по включённым приложениям (дата старта приложения,
			// не дата подписания договора). Fallback на даты договора если приложений с датой нет.
			var startPtr, endPtr *time.Time
			if s, e, ok := contractPeriodFromAppendices(ct); ok {
				startPtr = &s
				endPtr = e
			} else {
				if t, ok := parseWcrmDate(ct.StartDate); ok {
					startPtr = &t
				}
				if ct.EndDate != nil {
					if t, ok := parseWcrmDate(*ct.EndDate); ok {
						endPtr = &t
					}
				}
			}

			var existing models.Contract
			findErr := tx.Where("external_id = ?", extID).First(&existing).Error
			if findErr == gorm.ErrRecordNotFound {
				nc := models.Contract{
					Number:          targetNumber,
					AdminAccountID:  adminAccountID,
					CompanyID:       wcrmTargetTenantID,
					ContractType:    "client",
					ClientType:      ctype,
					ClientName:      clientName,
					ClientShortName: rec.Company.Name,
					ClientINN:       rec.Company.INN,
					ClientKPP:       rec.Company.KPP,
					ClientEmail:     rec.Company.Email,
					ClientPhone:     rec.Company.Phone,
					ClientAddress:   rec.Company.AddressU,
					ClientLegalAddress:  rec.Company.AddressU,
					ClientPostalAddress: rec.Company.AddressP,
					ClientOGRN:      rec.Company.OGRN,
					ClientOKPO:      rec.Company.OKPO,
					ClientDirector:  rec.Company.HeadName,
					ClientBankName:  rec.Company.BankName,
					ClientBankBIK:   rec.Company.BIK,
					ClientBankCorrespondentAccount: rec.Company.CorA,
					ClientBankAccount: rec.Company.CurA,
					StartDate:       startPtr,
					EndDate:         endPtr,
					Status:          status,
					IsActive:        isActive,
					Currency:        "RUB",
					NotifyBefore:    30,
					ExternalID:      extID,
				}
				// Select("*"): форсим запись ВСЕХ полей включая is_active=false.
				// Иначе GORM (default:true тег) пропускает zero-value bool → БД ставит true.
				if err := tx.Select("*").Create(&nc).Error; err != nil {
					return fmt.Errorf("create contract %s: %w", ct.Number, err)
				}
				result.ContractsInserted++
				result.CreatedContractIDs = append(result.CreatedContractIDs, nc.ID)
				existing = nc
			} else if findErr != nil {
				return fmt.Errorf("lookup contract %s: %w", extID, findErr)
			} else {
				// UPDATE только import-owned поля (не трогаем ручные правки вне allowlist).
				if err := tx.Model(&existing).Updates(map[string]interface{}{
					"number":            targetNumber,
					"client_type":       ctype,
					"client_name":       clientName,
					"client_short_name": rec.Company.Name,
					"client_inn":        rec.Company.INN,
					"client_kpp":        rec.Company.KPP,
					"client_director":   rec.Company.HeadName,
					"start_date":        startPtr,
					"end_date":          endPtr,
					"status":            status,
					"is_active":         isActive,
				}).Error; err != nil {
					return fmt.Errorf("update contract %s: %w", extID, err)
				}
				result.ContractsUpdated++
			}
				processedContractIDs = append(processedContractIDs, existing.ID)

			// Включённые приложения (enabled=1) → Subscription. Выключенные пропускаем
			// (договор уже отфильтрован на активность, но внутри могут быть disabled-приложения).
			for _, ap := range ct.Appendices {
				if ap.Enabled == 0 {
					continue
				}
				apStart, okS := parseWcrmDate(ap.StartDate)
				if !okS {
					if startPtr != nil {
						apStart = *startPtr
					} else {
						apStart = time.Now()
					}
				}
				var apEndPtr *time.Time
				if e, okE := parseWcrmDate(ap.EndDate); okE {
					apEndPtr = &e
				} else if ap.Period > 0 {
					// WCRM end_date = 0000-00-00 (хранит start + period). Реальный срок
					// приложения = start + period месяцев (как считает UI WCRM).
					e := apStart.AddDate(0, ap.Period, 0)
					apEndPtr = &e
				} else if endPtr != nil {
					apEndPtr = endPtr
				}

				// Тариф-матч: BillingPlan по (admin, price, billing_period). WCRM price =
				// цена за объект/мес (проверено: 600₽×48, 500₽×3). WCRM ap.Period — это
				// ДЛИТЕЛЬНОСТЬ приложения в месяцах (→ EndDate выше), а НЕ частота биллинга:
				// в WCRM всё биллится помесячно per object. Поэтому тариф ищем всегда monthly
				// (на проде есть 450/500/600 monthly; 500-yearly не существовало → подписка
				// раньше не создавалась). Тарифа нет → пропуск, оператор создаёт вручную.
				period := "monthly"
				var plan models.BillingPlan
				planErr := tx.Table("public.billing_plans").
					Where("admin_account_id = ? AND price = ? AND billing_period = ? AND is_active = true AND deleted_at IS NULL",
						adminAccountID, decimal.NewFromFloat(ap.Price), period).
					First(&plan).Error
				if planErr == gorm.ErrRecordNotFound {
					result.TariffNotFound = append(result.TariffNotFound,
						decimal.NewFromFloat(ap.Price).StringFixed(2)+"₽/"+period)
					continue
				} else if planErr != nil {
					return fmt.Errorf("lookup billing_plan price=%v: %w", ap.Price, planErr)
				}

				subExtID := "wcrm:sub:" + strconv.FormatInt(ap.WcrmAttachmentID, 10)
				// Следующий платёж = 1-е число следующего месяца. Дата начала приложения
				// в WCRM может быть в прошлом (импорт старых договоров) — берём опорной
				// max(now, apStart), иначе next_payment оказался бы в прошлом.
				nextBase := apStart
				now := time.Now().UTC()
				if now.After(nextBase) {
					nextBase = now
				}
				nextPay := time.Date(nextBase.Year(), nextBase.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
				// Автопродление: ежемесячные приложения (WCRM period=1) продлеваются
				// каждый месяц. Прочие периоды (год и т.п.) — без автопродления.
				autoRenew := ap.Period == 1
				var subID uint
				var exSub models.Subscription
				sErr := tx.Table("public.subscriptions").Where("external_id = ?", subExtID).First(&exSub).Error
				if sErr == gorm.ErrRecordNotFound {
					ns := models.Subscription{
						AdminAccountID:  adminAccountID,
						CompanyID:       wcrmTargetTenantID,
						BillingPlanID:   plan.ID,
						ContractID:      &existing.ID,
						StartDate:       apStart,
						EndDate:         apEndPtr,
						Status:          "active",
						IsAutoRenew:     autoRenew,
						NextPaymentDate: &nextPay,
						ExternalID:      subExtID,
					}
					if err := tx.Table("public.subscriptions").Create(&ns).Error; err != nil {
						return fmt.Errorf("create subscription %d: %w", ap.WcrmAttachmentID, err)
					}
					subID = ns.ID
					// Явный апдейт is_auto_renew: GORM default:true глотает zero-value false.
					if err := tx.Table("public.subscriptions").Where("id = ?", subID).
						Update("is_auto_renew", autoRenew).Error; err != nil {
						return fmt.Errorf("set sub auto_renew %d: %w", ap.WcrmAttachmentID, err)
					}
					result.SubscriptionsCreated++
				} else if sErr != nil {
					return fmt.Errorf("lookup subscription %s: %w", subExtID, sErr)
				} else {
					subID = exSub.ID
					if err := tx.Table("public.subscriptions").Where("id = ?", subID).Updates(map[string]interface{}{
						"billing_plan_id": plan.ID,
						"contract_id":     existing.ID,
						"start_date":      apStart,
						"end_date":        apEndPtr,
						"status":          "active",
						"next_payment_date": nextPay,
						"is_auto_renew":     autoRenew,
					}).Error; err != nil {
						return fmt.Errorf("update subscription %s: %w", subExtID, err)
					}
				}

				// Объекты приложения → ContractObject (uid-матч WCRM↔ACRM Axenta).
				for _, ob := range ap.Objects {
					ref, ok := axentaObjMap[ob.UID]
					if !ok {
						result.ObjectsUnmatched++
						continue
					}
					sid := subID
					var existsCO int64
					tx.Raw(`SELECT COUNT(*) FROM contract_objects WHERE subscription_id = ? AND object_id = ? AND deleted_at IS NULL`, subID, ref.ObjectID).Scan(&existsCO)
					if existsCO == 0 {
						co := models.ContractObject{
							ContractID:      existing.ID,
							SubscriptionID:  &sid,
							ObjectID:        ref.ObjectID,
							ObjectCompanyID: ref.CompanyID,
							ObjectSchema:    wcrmSchema(),
							Status:          "active",
							StartDate:       apStart,
							EndDate:         apEndPtr,
						}
						if err := tx.Create(&co).Error; err != nil {
							return fmt.Errorf("create contract_object uid=%s: %w", ob.UID, err)
						}
						result.ObjectsLinked++
					}
				}
			}
		}

		// Остаток средств → ledger (проводка migration_balance). amount знаковый:
		// >0 предоплата (баланс в плюсе), <0 долг. balance договора = SUM(ledger).
		if rec.Balance != 0 {
			// Остаток привязываем к первому договору ЭТОЙ компании (created или updated),
			// а не к глобальному min wcrm-договору (мог быть чужой компании).
			var primaryContractID uint
			if len(processedContractIDs) > 0 {
				primaryContractID = processedContractIDs[0]
			}
			if primaryContractID > 0 {
				ledExtID := "wcrm:balance:" + strconv.FormatInt(companyID, 10)
				amount := decimal.NewFromFloat(rec.Balance) // знаковый
				var existsCount int64
				tx.Model(&models.LedgerEntry{}).
					Where("admin_account_id = ? AND company_id = ? AND source = 'migration' AND external_id = ? AND deleted_at IS NULL",
						adminAccountID, wcrmTargetTenantID, ledExtID).Count(&existsCount)
				if existsCount == 0 {
					le := models.LedgerEntry{
						AdminAccountID: adminAccountID,
						CompanyID:      wcrmTargetTenantID,
						ContractID:     primaryContractID,
						EntryType:      "migration_balance",
						Amount:         amount,
						Currency:       "RUB",
						Source:         "migration",
						ExternalID:     ledExtID,
						Description:    "Импорт остатка лицевого счёта из WCRM",
						EntryDate:      time.Now().UTC(),
						CreatedBy:      approvedBy,
					}
					if err := tx.Create(&le).Error; err != nil {
						return fmt.Errorf("create ledger migration_balance: %w", err)
					}
				} else {
					if err := tx.Model(&models.LedgerEntry{}).
						Where("admin_account_id = ? AND company_id = ? AND source = 'migration' AND external_id = ? AND deleted_at IS NULL",
							adminAccountID, wcrmTargetTenantID, ledExtID).
						Update("amount", amount).Error; err != nil {
						return fmt.Errorf("update ledger migration_balance: %w", err)
					}
				}
				result.BalanceRecorded = true
			}
		}

		// Состояние
		if err := tx.Exec(`INSERT INTO wcrm_migration_state (wcrm_company_id, status, snapshot_sha256, client_type_override, approved_by, approved_at, updated_at)
			VALUES (?, 'approved', ?, ?, ?, now(), now())
			ON CONFLICT (wcrm_company_id) DO UPDATE SET status='approved', snapshot_sha256=EXCLUDED.snapshot_sha256, client_type_override=EXCLUDED.client_type_override, approved_by=EXCLUDED.approved_by, approved_at=now(), updated_at=now()`,
			companyID, sha, nullStr(req.ClientTypeOverride), approvedBy).Error; err != nil {
			return fmt.Errorf("upsert state: %w", err)
		}

		return nil
	})

	if txErr == errWcrmLocked {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "импорт этой компании уже выполняется"})
		return
	}
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": txErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": result})
}

// PostWcrmMigrationSkip — POST /api/auth/migration/wcrm/skip/:wcrm_company_id
func PostWcrmMigrationSkip(c *gin.Context) {
	if !requireSuperadmin(c) {
		return
	}
	companyID, err := strconv.ParseInt(c.Param("wcrm_company_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный wcrm_company_id"})
		return
	}
	_, sha, _ := loadWcrmSnapshot()
	username, _ := c.Get("username")
	approvedBy, _ := username.(string)
	if err := database.DB.Exec(fmt.Sprintf(`INSERT INTO %s.wcrm_migration_state (wcrm_company_id, status, snapshot_sha256, approved_by, updated_at)
		VALUES (?, 'skipped', ?, ?, now())
		ON CONFLICT (wcrm_company_id) DO UPDATE SET status='skipped', approved_by=EXCLUDED.approved_by, updated_at=now()`, wcrmSchema()),
		companyID, sha, approvedBy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// GetWcrmMigrationStatus — GET /api/auth/migration/wcrm/status
func GetWcrmMigrationStatus(c *gin.Context) {
	if !requireSuperadmin(c) {
		return
	}
	schema := wcrmSchema()
	var counts struct {
		Approved int64
		Skipped  int64
		Pending  int64
	}
	database.DB.Raw(fmt.Sprintf(`SELECT
		COUNT(*) FILTER (WHERE status='approved') AS approved,
		COUNT(*) FILTER (WHERE status='skipped')  AS skipped,
		COUNT(*) FILTER (WHERE status='pending')  AS pending
		FROM %s.wcrm_migration_state`, schema)).Scan(&counts)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": counts})
}

// acrmObjectRef — ссылка на ACRM-объект для ContractObject.
type acrmObjectRef struct {
	ObjectID  uint // external_object_id (Axenta Cloud object id)
	CompanyID uint // account_external_id (Axenta account id)
}

// loadAxentaObjectMap строит map unique_id → ACRM Axenta-объект из axenta_object_snapshots.
// WCRM ax-объекты (companies.ax=1) — это объекты Axenta Cloud; их object.uid совпадает
// с axenta_object_snapshots.unique_id (проверено: Газель uid 179191 = unique_id 179191,
// wid 1000000195397 = external_object_id 195397). Для ContractObject: ObjectID =
// external_object_id, ObjectCompanyID = account_external_id (как трактует billing API).
func loadAxentaObjectMap(schema string) map[string]acrmObjectRef {
	out := map[string]acrmObjectRef{}
	if database.DB == nil {
		return out
	}
	var rows []struct {
		UniqueID          string
		ExternalObjectID  uint
		AccountExternalID uint
	}
	database.DB.Raw(fmt.Sprintf(
		`SELECT unique_id, external_object_id, account_external_id FROM %s.axenta_object_snapshots WHERE unique_id <> '' AND axenta_deleted_at IS NULL`, schema)).
		Scan(&rows)
	for _, r := range rows {
		if r.UniqueID != "" && r.ExternalObjectID > 0 {
			out[r.UniqueID] = acrmObjectRef{ObjectID: r.ExternalObjectID, CompanyID: r.AccountExternalID}
		}
	}
	return out
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

var errWcrmLocked = fmt.Errorf("wcrm company import locked")
