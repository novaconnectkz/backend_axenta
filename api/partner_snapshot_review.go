package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend_axenta/audit"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Ф1 billing-gate: справочник снимков + очередь ручного подтверждения.
// Снимки живут в tenant-схеме (там их пишут все 4 источника), поэтому читаем/пишем
// через tenant_db из контекста.

func partnerSnapshotTenantDB(c *gin.Context) (*gorm.DB, bool) {
	v, ok := c.Get("tenant_db")
	if !ok {
		return nil, false
	}
	db, ok := v.(*gorm.DB)
	return db, ok
}

// resolveContractNumbers проставляет человекочитаемый № договора (tenant.contracts.number)
// по contract_id — чтобы в справочнике показывать «AX-…/442» вместо внутреннего «#97».
func resolveContractNumbers(db *gorm.DB, rows []models.PartnerDailySnapshot) {
	ids := map[uint]struct{}{}
	for i := range rows {
		if rows[i].ContractID > 0 {
			ids[rows[i].ContractID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return
	}
	idList := make([]uint, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	var cs []struct {
		ID     uint
		Number string
	}
	if err := db.Table("contracts").Select("id, number").
		Where("id IN ?", idList).Find(&cs).Error; err != nil {
		return // best-effort: FE покажет внутренний id
	}
	numByID := make(map[uint]string, len(cs))
	for _, x := range cs {
		numByID[x.ID] = x.Number
	}
	for i := range rows {
		if n := numByID[rows[i].ContractID]; n != "" {
			rows[i].ContractNumber = n
		}
	}
}

// resolvePartnerNames best-effort проставляет имя владельца (аккаунт/дилер/юзер) в строки
// снимков — чтобы по «без договора» было видно, чьи объекты. Резолвит ВСЕ 4 источника:
//
//	axenta  — axenta_account_snapshots.external_account_id → account_name (tenant, conn=0)
//	skif    — public.skif_dealers.skif_dealer_id → name (по connection_id)
//	wialon  — public.wialon_account_statuses.wialon_user_id → name (по connection_id)
//	gelios  — public.gelios_users.gelios_user_id → legal_name||login (по connection_id)
//
// Имена скоупятся по (connection_id, external_id), т.к. внешний id уникален лишь в рамках conn.
func resolvePartnerNames(db *gorm.DB, rows []models.PartnerDailySnapshot) {
	key := func(conn uint, ext string) string { return fmt.Sprintf("%d|%s", conn, ext) }
	names := map[string]string{}

	type bucket struct {
		conns map[uint]struct{}
		exts  map[string]struct{}
	}
	bySrc := map[string]*bucket{}
	for i := range rows {
		ext := rows[i].PartnerExternalID
		if ext == "" {
			continue
		}
		s := rows[i].PartnerSource
		if bySrc[s] == nil {
			bySrc[s] = &bucket{conns: map[uint]struct{}{}, exts: map[string]struct{}{}}
		}
		bySrc[s].conns[rows[i].ConnectionID] = struct{}{}
		bySrc[s].exts[ext] = struct{}{}
	}
	lists := func(b *bucket) ([]uint, []string) {
		cs := make([]uint, 0, len(b.conns))
		for c := range b.conns {
			cs = append(cs, c)
		}
		es := make([]string, 0, len(b.exts))
		for e := range b.exts {
			es = append(es, e)
		}
		return cs, es
	}

	if b := bySrc["axenta"]; b != nil {
		_, es := lists(b)
		var rr []struct {
			ExternalAccountID int64
			AccountName       string
		}
		db.Table("axenta_account_snapshots").Select("external_account_id, account_name").
			Where("deleted_at IS NULL AND external_account_id::text IN ?", es).Find(&rr)
		for _, x := range rr {
			names[key(0, fmt.Sprintf("%d", x.ExternalAccountID))] = x.AccountName
		}
	}
	if b := bySrc["skif"]; b != nil {
		cs, es := lists(b)
		var rr []struct {
			ConnectionID uint
			SkifDealerID string
			Name         string
		}
		db.Table("public.skif_dealers").Select("connection_id, skif_dealer_id, name").
			Where("connection_id IN ? AND skif_dealer_id IN ?", cs, es).Find(&rr)
		for _, x := range rr {
			names[key(x.ConnectionID, x.SkifDealerID)] = x.Name
		}
	}
	if b := bySrc["wialon"]; b != nil {
		cs, es := lists(b)
		var rr []struct {
			ConnectionID uint
			WialonUserID int64
			Name         string
		}
		db.Table("public.wialon_account_statuses").Select("connection_id, wialon_user_id, name").
			Where("connection_id IN ? AND wialon_user_id::text IN ?", cs, es).Find(&rr)
		for _, x := range rr {
			names[key(x.ConnectionID, fmt.Sprintf("%d", x.WialonUserID))] = x.Name
		}
	}
	if b := bySrc["gelios"]; b != nil {
		cs, es := lists(b)
		var rr []struct {
			ConnectionID uint
			GeliosUserID string
			LegalName    string
			Login        string
		}
		db.Table("public.gelios_users").Select("connection_id, gelios_user_id, legal_name, login").
			Where("connection_id IN ? AND gelios_user_id IN ?", cs, es).Find(&rr)
		for _, x := range rr {
			n := x.LegalName
			if n == "" {
				n = x.Login
			}
			names[key(x.ConnectionID, x.GeliosUserID)] = n
		}
	}

	for i := range rows {
		conn := rows[i].ConnectionID
		if rows[i].PartnerSource == "axenta" {
			conn = 0
		}
		if n := names[key(conn, rows[i].PartnerExternalID)]; n != "" {
			rows[i].PartnerName = n
		}
	}
}

// applyDirectPartnerFilter ограничивает выборку ПРЯМЫМИ партнёрами (Axenta-иерархия глубина 1:
// «ROOT > X»). Суб-партнёры (партнёры наших партнёров, глубина ≥2: «ROOT > Партнёр > X») —
// в счёт не идут (биллинг прямого партнёра уже включает поддерево через Axenta API), это шум.
// НЕ трогаем: договорные строки (cid>0, осознанный биллинг) и не-Axenta источники.
// Bypass: ?include_subpartners=true. Глубина = число '>' в hierarchy.
func applyDirectPartnerFilter(c *gin.Context, db, q *gorm.DB) *gorm.DB {
	if c.Query("include_subpartners") == "true" {
		return q
	}
	directIDs := db.Table("axenta_account_snapshots").
		Select("external_account_id::text").
		Where("deleted_at IS NULL AND (length(hierarchy) - length(replace(hierarchy, '>', ''))) = 1")
	return q.Where("partner_source <> ? OR contract_id > 0 OR partner_external_id IN (?)", "axenta", directIDs)
}

// ListPartnerSnapshots — read-only справочник снимков с фильтрами (Ф4 «История снимков»).
// GET /api/auth/partner-snapshots/list?source=&verify_status=&contract_id=&start_date=&end_date=&limit=&offset=
func ListPartnerSnapshots(c *gin.Context) {
	if _, err := middleware.GetAdminAccountID(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	db, ok := partnerSnapshotTenantDB(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Tenant DB не найдена"})
		return
	}

	q := db.Model(&models.PartnerDailySnapshot{})
	if src := c.Query("source"); src != "" {
		q = q.Where("partner_source = ?", src)
	}
	if vs := c.Query("verify_status"); vs != "" {
		q = q.Where("verify_status = ?", vs)
	}
	if cid := c.Query("contract_id"); cid != "" {
		if id, err := strconv.ParseUint(cid, 10, 64); err == nil {
			q = q.Where("contract_id = ?", id)
		}
	}
	if sd := c.Query("start_date"); sd != "" {
		if t, err := time.Parse("2006-01-02", sd); err == nil {
			q = q.Where("snapshot_date >= ?", t)
		}
	}
	if ed := c.Query("end_date"); ed != "" {
		if t, err := time.Parse("2006-01-02", ed); err == nil {
			q = q.Where("snapshot_date <= ?", t)
		}
	}

	// Поиск по колонке «Договор»: номер договора ИЛИ имя партнёра (4 источника).
	// partner_daily_snapshots не хранит номер/имя — резолвим matching id'шники и фильтруем
	// по contract_id / partner_company_id / partner_external_id. COLLATE "und-x-icu" — для
	// регистронезависимого поиска по кириллице под lc_collate=C (голый ILIKE не сфолдит).
	if qstr := strings.TrimSpace(c.Query("q")); qstr != "" {
		like := "%" + qstr + "%"
		var cids []uint
		db.Table("contracts").Where("number ILIKE ?", like).Pluck("id", &cids)
		var axIDs []int64
		db.Table("axenta_account_snapshots").
			Where("deleted_at IS NULL AND account_name ILIKE ? COLLATE \"und-x-icu\"", like).
			Pluck("external_account_id", &axIDs)
		var skifIDs []string
		db.Table("public.skif_dealers").
			Where("name ILIKE ? COLLATE \"und-x-icu\"", like).Pluck("skif_dealer_id", &skifIDs)
		var wlIDs []int64
		db.Table("public.wialon_account_statuses").
			Where("name ILIKE ? COLLATE \"und-x-icu\"", like).Pluck("wialon_user_id", &wlIDs)
		wlStr := make([]string, 0, len(wlIDs))
		for _, id := range wlIDs {
			wlStr = append(wlStr, fmt.Sprintf("%d", id))
		}
		var gIDs []string
		db.Table("public.gelios_users").
			Where("legal_name ILIKE ? COLLATE \"und-x-icu\" OR login ILIKE ? COLLATE \"und-x-icu\"", like, like).
			Pluck("gelios_user_id", &gIDs)

		// OR-группа: пустые IN-наборы дают IN (NULL) → matches nothing (безопасно).
		grp := db.Where("contract_id IN ?", cids).
			Or("partner_company_id IN ?", axIDs).
			Or("partner_source = ? AND partner_external_id IN ?", "skif", skifIDs).
			Or("partner_source = ? AND partner_external_id IN ?", "wialon", wlStr).
			Or("partner_source = ? AND partner_external_id IN ?", "gelios", gIDs)
		q = q.Where(grp)
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	q = applyDirectPartnerFilter(c, db, q) // только прямые партнёры (без партнёров наших партнёров)

	var total int64
	q.Count(&total)

	var rows []models.PartnerDailySnapshot
	if err := q.Order("snapshot_date DESC, partner_source ASC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	resolvePartnerNames(db, rows)
	resolveContractNumbers(db, rows)

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"snapshots": rows,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetPartnerSnapshotReviewQueue — очередь снимков, заблокированных гейтом (needs_review/estimated).
// GET /api/auth/partner-snapshots/review-queue?source=&start_date=&end_date=
func GetPartnerSnapshotReviewQueue(c *gin.Context) {
	if _, err := middleware.GetAdminAccountID(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	db, ok := partnerSnapshotTenantDB(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Tenant DB не найдена"})
		return
	}

	q := db.Model(&models.PartnerDailySnapshot{}).
		Where("verify_status IN ?", []string{models.VerifyStatusNeedsRev, models.VerifyStatusEstimated})
	if src := c.Query("source"); src != "" {
		q = q.Where("partner_source = ?", src)
	}
	if sd := c.Query("start_date"); sd != "" {
		if t, err := time.Parse("2006-01-02", sd); err == nil {
			q = q.Where("snapshot_date >= ?", t)
		}
	}
	if ed := c.Query("end_date"); ed != "" {
		if t, err := time.Parse("2006-01-02", ed); err == nil {
			q = q.Where("snapshot_date <= ?", t)
		}
	}

	q = applyDirectPartnerFilter(c, db, q) // только прямые партнёры

	var rows []models.PartnerDailySnapshot
	if err := q.Order("amount_at_risk DESC, snapshot_date DESC").Limit(1000).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	resolvePartnerNames(db, rows)
	resolveContractNumbers(db, rows)

	totalRisk := decimal.Zero
	for i := range rows {
		totalRisk = totalRisk.Add(rows[i].AmountAtRisk)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"snapshots":     rows,
		"count":         len(rows),
		"total_at_risk": totalRisk.StringFixed(2),
	})
}

// ApprovePartnerSnapshot — ручное подтверждение снимка из очереди (needs_review/estimated → manual_approved).
// POST /api/auth/partner-snapshots/:id/approve  body {comment}
// Обновляет ТОЛЬКО verify-поля (Updates по map, без хуков) — daily_cost не трогается.
func ApprovePartnerSnapshot(c *gin.Context) {
	if _, err := middleware.GetAdminAccountID(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	db, ok := partnerSnapshotTenantDB(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Tenant DB не найдена"})
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректный id снимка"})
		return
	}

	var body struct {
		Comment string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&body)

	var snap models.PartnerDailySnapshot
	if err := db.Where("id = ?", id).First(&snap).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "снимок не найден"})
		return
	}

	// Подтверждать имеет смысл только заблокированные снимки.
	if snap.VerifyStatus != models.VerifyStatusNeedsRev && snap.VerifyStatus != models.VerifyStatusEstimated {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "снимок не в статусе needs_review/estimated (текущий: " + snap.VerifyStatus + ")",
		})
		return
	}

	var userID uint
	if v, ok := c.Get("user_id"); ok {
		switch t := v.(type) {
		case uint:
			userID = t
		case int:
			userID = uint(t)
		case float64:
			userID = uint(t)
		}
	}

	now := time.Now().UTC()
	note := snap.VerifyNotes + " | подтверждено вручную"
	if body.Comment != "" {
		note += ": " + body.Comment
	}

	// Условный апдейт: только если снимок ВСЁ ЕЩЁ заблокирован (защита от гонки с ночным
	// cron, который мог между read и update пересоздать строку с новыми числами).
	res := db.Model(&models.PartnerDailySnapshot{}).
		Where("id = ? AND verify_status IN ?", id, []string{models.VerifyStatusNeedsRev, models.VerifyStatusEstimated}).
		Updates(map[string]interface{}{
			"verify_status": models.VerifyStatusApproved,
			"approved_by":   userID,
			"verified_at":   now,
			"verify_notes":  note,
		})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": res.Error.Error()})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{
			"status": "error",
			"error":  "статус снимка изменился (возможно, пересчитан кроном) — обновите очередь и проверьте заново",
		})
		return
	}

	// Audit trail: кто/когда/что подтвердил + оригинальное значение.
	audit.LogSuccess(c, "partner_snapshot.manual_approve", gin.H{
		"snapshot_id":    id,
		"contract_id":    snap.ContractID,
		"partner_source": snap.PartnerSource,
		"snapshot_date":  snap.SnapshotDate.Format("2006-01-02"),
		"prev_status":    snap.VerifyStatus,
		"active_objects": snap.ActiveObjectsCount,
		"amount_at_risk": snap.AmountAtRisk.StringFixed(2),
		"approved_by":    userID,
		"comment":        body.Comment,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Снимок подтверждён и включён в биллинг",
		"id":      id,
	})
}

// ApprovePartnerSnapshotsBulk — массовое подтверждение всех заблокированных (needs_review/
// estimated) снимков по фильтрам (source/contract_id/период/q — те же что в списке, чтобы
// «Подтвердить всё» совпадало с видимым). Атомарный UPDATE → manual_approved (WHERE forces
// статус → гонка с cron безопасна: пересчитанные verified/уже approved не трогаются).
// POST /api/auth/partner-snapshots/approve-bulk
func ApprovePartnerSnapshotsBulk(c *gin.Context) {
	if _, err := middleware.GetAdminAccountID(c); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": err.Error()})
		return
	}
	db, ok := partnerSnapshotTenantDB(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Tenant DB не найдена"})
		return
	}
	// Codex C1: массовое подтверждение (деньги, скопом) — только admin/superadmin.
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "массовое подтверждение доступно только admin/superadmin"})
		return
	}

	var body struct {
		Source     string `json:"source"`
		ContractID uint   `json:"contract_id"`
		StartDate  string `json:"start_date"`
		EndDate    string `json:"end_date"`
		Q          string `json:"q"`
		Comment    string `json:"comment"`
		ApproveAll bool   `json:"approve_all"` // явное согласие на тенант-wide (без фильтров)
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		// Codex C2: НЕ глотаем ошибку — кривой JSON не должен молча расшириться до «всех».
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "неверный формат запроса"})
		return
	}

	var userID uint
	if v, ok := c.Get("user_id"); ok {
		switch t := v.(type) {
		case uint:
			userID = t
		case int:
			userID = uint(t)
		case float64:
			userID = uint(t)
		}
	}
	// Codex H: финансовое одобрение без ответственного актора недопустимо.
	if userID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "не удалось определить пользователя-подтверждающего"})
		return
	}

	// Только заблокированные снимки — форсим в WHERE (race-safe, как в single approve).
	q := db.Model(&models.PartnerDailySnapshot{}).
		Where("verify_status IN ?", []string{models.VerifyStatusNeedsRev, models.VerifyStatusEstimated})
	hasFilter := false
	if body.Source != "" {
		q = q.Where("partner_source = ?", body.Source)
		hasFilter = true
	}
	if body.ContractID > 0 {
		q = q.Where("contract_id = ?", body.ContractID)
		hasFilter = true
	}
	if body.StartDate != "" {
		// Codex C2: переданная-но-кривая дата → 400, не молчаливое расширение скоупа.
		t, err := time.Parse("2006-01-02", body.StartDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "неверный start_date (YYYY-MM-DD)"})
			return
		}
		q = q.Where("snapshot_date >= ?", t)
		hasFilter = true
	}
	if body.EndDate != "" {
		t, err := time.Parse("2006-01-02", body.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "неверный end_date (YYYY-MM-DD)"})
			return
		}
		q = q.Where("snapshot_date <= ?", t)
		hasFilter = true
	}
	// Поиск по «Договору» (номер/имя партнёра) — зеркало ListPartnerSnapshots, чтобы bulk
	// совпадал с отфильтрованным видом.
	if qstr := strings.TrimSpace(body.Q); qstr != "" {
		like := "%" + qstr + "%"
		var cids []uint
		db.Table("contracts").Where("number ILIKE ?", like).Pluck("id", &cids)
		var axIDs []int64
		db.Table("axenta_account_snapshots").
			Where("deleted_at IS NULL AND account_name ILIKE ? COLLATE \"und-x-icu\"", like).
			Pluck("external_account_id", &axIDs)
		var skifIDs []string
		db.Table("public.skif_dealers").
			Where("name ILIKE ? COLLATE \"und-x-icu\"", like).Pluck("skif_dealer_id", &skifIDs)
		var wlIDs []int64
		db.Table("public.wialon_account_statuses").
			Where("name ILIKE ? COLLATE \"und-x-icu\"", like).Pluck("wialon_user_id", &wlIDs)
		wlStr := make([]string, 0, len(wlIDs))
		for _, id := range wlIDs {
			wlStr = append(wlStr, fmt.Sprintf("%d", id))
		}
		var gIDs []string
		db.Table("public.gelios_users").
			Where("legal_name ILIKE ? COLLATE \"und-x-icu\" OR login ILIKE ? COLLATE \"und-x-icu\"", like, like).
			Pluck("gelios_user_id", &gIDs)
		grp := db.Where("contract_id IN ?", cids).
			Or("partner_company_id IN ?", axIDs).
			Or("partner_source = ? AND partner_external_id IN ?", "skif", skifIDs).
			Or("partner_source = ? AND partner_external_id IN ?", "wialon", wlStr).
			Or("partner_source = ? AND partner_external_id IN ?", "gelios", gIDs)
		q = q.Where(grp)
		hasFilter = true
	}

	// Codex C2/H: без фильтров операция = ВЕСЬ заблокированный биллинг тенанта (скопом обходит
	// гейт, который мог заблокировать по реальной причине — обнуление/скачок/cross-source).
	// Требуем явное approve_all=true для тенант-wide, иначе ≥1 фильтр.
	if !hasFilter && !body.ApproveAll {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "укажите хотя бы один фильтр (source/contract_id/период/q) либо approve_all=true для всего тенанта",
		})
		return
	}

	q = applyDirectPartnerFilter(c, db, q) // только прямые партнёры

	// Сумма под риском ДО апдейта (для отчёта оператору).
	var agg struct {
		Cnt int64
		Sum decimal.Decimal
	}
	q.Session(&gorm.Session{}).
		Select("COUNT(*) as cnt, COALESCE(SUM(daily_cost),0) as sum").Scan(&agg)

	now := time.Now().UTC()
	note := "массово подтверждено вручную"
	if body.Comment != "" {
		note += ": " + body.Comment
	}
	res := q.Updates(map[string]interface{}{
		"verify_status": models.VerifyStatusApproved,
		"approved_by":   userID,
		"verified_at":   now,
		"verify_notes":  gorm.Expr("COALESCE(verify_notes, '') || ?", " | "+note),
	})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": res.Error.Error()})
		return
	}

	audit.LogSuccess(c, "partner_snapshot.manual_approve_bulk", gin.H{
		"approved_count": res.RowsAffected,
		"amount_total":   agg.Sum.StringFixed(2),
		"source":         body.Source,
		"contract_id":    body.ContractID,
		"start_date":     body.StartDate,
		"end_date":       body.EndDate,
		"q":              body.Q,
		"approved_by":    userID,
		"comment":        body.Comment,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"message":        fmt.Sprintf("Подтверждено снимков: %d (на сумму %s ₽)", res.RowsAffected, agg.Sum.StringFixed(2)),
		"approved_count": res.RowsAffected,
		"amount_total":   agg.Sum.InexactFloat64(),
	})
}
