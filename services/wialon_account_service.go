package services

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WialonAccountService — создание новых учётных записей в Wialon (user + resource + billing).
//
// Поток создания (Wialon SDK):
//  1. core/create_user             — создать юзера (creatorId = login user)
//  2. core/create_resource         — создать ресурс (creatorId = login user)
//  3. account/update_billing_properties — превратить ресурс в биллинг-аккаунт
//  4. account/change_billing_plan  — назначить тариф
//  5. core/update_user_property    — установить email юзеру
//  6. core/update_custom_property  — sys_account_enable_parent=1 если type=partner
//  7. core/update_item_access      — выдать юзеру полный доступ к своему ресурсу
type WialonAccountService struct {
	db            *gorm.DB
	wialonService *WialonService
	statsService  *WialonStatsService
}

func NewWialonAccountService() *WialonAccountService {
	return &WialonAccountService{
		db:            database.DB,
		wialonService: NewWialonService(),
		statsService:  NewWialonStatsService(),
	}
}

// WialonBillingPlan — пункт списка тарифов из account/get_billing_plans
type WialonBillingPlan struct {
	Name string `json:"name"`
}

// GetBillingPlans возвращает список тарифов из БД-кэша. Если кэш пустой — auto-sync с Wialon (lazy-init).
// Используется фронтом для селектора «Тарифный план».
func (s *WialonAccountService) GetBillingPlans(connectionID uint) ([]WialonBillingPlan, error) {
	cached, err := s.getCachedPlans(connectionID)
	if err != nil {
		return nil, err
	}
	if len(cached) > 0 {
		return cached, nil
	}

	// Cache miss → синхронный sync, потом возвращаем
	log.Printf("⚡ billing-plans cache miss для conn=%d, lazy-init", connectionID)
	if _, err := s.SyncBillingPlans(connectionID); err != nil {
		return nil, err
	}
	return s.getCachedPlans(connectionID)
}

// getCachedPlans читает из таблицы wialon_billing_plans
func (s *WialonAccountService) getCachedPlans(connectionID uint) ([]WialonBillingPlan, error) {
	var rows []models.WialonBillingPlan
	if err := s.db.Where("connection_id = ?", connectionID).Order("plan_name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	plans := make([]WialonBillingPlan, 0, len(rows))
	for _, r := range rows {
		plans = append(plans, WialonBillingPlan{Name: r.PlanName})
	}
	return plans, nil
}

// SyncBillingPlans — fetch из Wialon → upsert в БД → diff (added/removed) → лог.
// Возвращает список свежих тарифов. Используется WialonBillingPlansScheduler и при ?force_refresh=true.
func (s *WialonAccountService) SyncBillingPlans(connectionID uint) ([]WialonBillingPlan, error) {
	t0 := time.Now()
	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден: %w", connectionID, err)
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	currentUserID := int64(0)
	if loginResp.User != nil {
		currentUserID = loginResp.User.ID
	}

	// Wialon SDK: params={} (БЕЗ userId) для получения собственного плана + subPlans.
	// На WL get_billing_plans закрыт → фолбэк через get_account_data (отдаёт текущий план).
	body, err := s.callRaw(conn.Host, loginResp.Eid, "account/get_billing_plans", map[string]interface{}{})
	if err != nil {
		log.Printf("⚠️ get_billing_plans не доступен (%v), фолбэк через account/get_account_data", err)
		fallback, ferr := s.getCurrentPlanFallback(conn.Host, loginResp.Eid, currentUserID)
		if ferr != nil {
			return nil, ferr
		}
		return s.upsertPlans(connectionID, fallback, nil)
	}

	// Ответ: { plan: {...}, subPlans: [...] }. Сохраняем raw_payload для diagnostics.
	var resp struct {
		Plan     map[string]interface{}   `json:"plan"`
		SubPlans []map[string]interface{} `json:"subPlans"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("парсинг billing_plans: %w (raw: %s)", err, string(body)[:min(300, len(body))])
	}

	plans := make([]WialonBillingPlan, 0)
	rawByName := make(map[string][]byte)
	seenNames := map[string]bool{}
	addPlan := func(name string, raw []byte) {
		if name == "" || seenNames[name] {
			return
		}
		seenNames[name] = true
		plans = append(plans, WialonBillingPlan{Name: name})
		if raw != nil {
			rawByName[name] = raw
		}
	}
	if resp.Plan != nil {
		if name, ok := resp.Plan["name"].(string); ok {
			raw, _ := json.Marshal(resp.Plan)
			addPlan(name, raw)
		}
	}
	for _, p := range resp.SubPlans {
		if name, ok := p["name"].(string); ok {
			raw, _ := json.Marshal(p)
			addPlan(name, raw)
		}
	}
	log.Printf("⚡ billing-plans sync: получено %d тарифов из Wialon (conn=%d %s) за %s",
		len(plans), connectionID, conn.Name, time.Since(t0))
	return s.upsertPlans(connectionID, plans, rawByName)
}

// upsertPlans — записывает план-имена в БД и удаляет те, что Wialon больше не возвращает.
// Лог diff (added/removed) полезен для отладки изменений тарифной сетки у дилера.
func (s *WialonAccountService) upsertPlans(connectionID uint, plans []WialonBillingPlan, rawByName map[string][]byte) ([]WialonBillingPlan, error) {
	now := time.Now()

	// Текущие в БД для diff
	var existing []models.WialonBillingPlan
	if err := s.db.Where("connection_id = ?", connectionID).Find(&existing).Error; err != nil {
		return nil, fmt.Errorf("read existing: %w", err)
	}
	existingNames := map[string]bool{}
	for _, e := range existing {
		existingNames[e.PlanName] = true
	}
	freshNames := map[string]bool{}
	for _, p := range plans {
		freshNames[p.Name] = true
	}

	// Upsert свежие. Дедуплицируем — Wialon может вернуть один и тот же plan и в plan, и в subPlans
	// (например, "glomoskz" фигурирует дважды). PostgreSQL ON CONFLICT не любит дубликаты в одном batch.
	if len(plans) > 0 {
		seen := map[string]bool{}
		rows := make([]models.WialonBillingPlan, 0, len(plans))
		for _, p := range plans {
			if seen[p.Name] {
				continue
			}
			seen[p.Name] = true
			row := models.WialonBillingPlan{
				ConnectionID: connectionID,
				PlanName:     p.Name,
				LastSyncedAt: now,
			}
			if raw, ok := rawByName[p.Name]; ok && len(raw) > 0 {
				row.RawPayload = string(raw)
			} else {
				// пустая строка ломает jsonb → пишем валидный пустой объект
				row.RawPayload = "{}"
			}
			rows = append(rows, row)
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "plan_name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"raw_payload", "last_synced_at", "updated_at",
			}),
		}).Create(rows).Error; err != nil {
			return nil, fmt.Errorf("upsert: %w", err)
		}
	}

	// Удаляем устаревшие (которых больше нет в Wialon-ответе)
	added, removed := 0, 0
	for name := range freshNames {
		if !existingNames[name] {
			added++
		}
	}
	toRemove := []string{}
	for name := range existingNames {
		if !freshNames[name] {
			toRemove = append(toRemove, name)
			removed++
		}
	}
	if len(toRemove) > 0 {
		if err := s.db.Where("connection_id = ? AND plan_name IN ?", connectionID, toRemove).
			Delete(&models.WialonBillingPlan{}).Error; err != nil {
			log.Printf("⚠️ billing-plans: не смогли удалить устаревшие %v: %v", toRemove, err)
		}
	}
	if added > 0 || removed > 0 {
		log.Printf("📊 billing-plans diff (conn=%d): +%d added, -%d removed", connectionID, added, removed)
	}

	return plans, nil
}

// getCurrentPlanFallback — когда account/get_billing_plans не доступен (типично для Wialon Local
// и не-root dealer), используем core/get_account_data с params={"type":1} (БЕЗ itemId).
// Этот метод работает на любом юзере и возвращает {plan, subPlans:[...]} — собственный план
// + массив доступных подчинённых планов. Именно его дёргает Wialon CMS Manager при открытии
// диалога "Создать учётную запись".
func (s *WialonAccountService) getCurrentPlanFallback(host, eid string, userID int64) ([]WialonBillingPlan, error) {
	body, err := s.callRaw(host, eid, "core/get_account_data", map[string]interface{}{
		"type": 1,
	})
	if err != nil {
		log.Printf("⚠️ get_account_data fallback не доступен (%v) — возвращаем пустой список тарифов", err)
		return []WialonBillingPlan{}, nil
	}
	var data struct {
		Plan     string   `json:"plan"`
		SubPlans []string `json:"subPlans"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("парсинг get_account_data: %w", err)
	}

	plans := make([]WialonBillingPlan, 0, len(data.SubPlans)+1)
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		plans = append(plans, WialonBillingPlan{Name: name})
	}
	add(data.Plan)
	for _, name := range data.SubPlans {
		add(name)
	}
	log.Printf("⚡ get_account_data fallback: получено %d планов (plan + subPlans)", len(plans))
	return plans, nil
}

// CreateAccountRequest — тело запроса на создание аккаунта в Wialon
type CreateWialonAccountRequest struct {
	Name         string `json:"name" binding:"required"`         // имя аккаунта (= имя ресурса)
	Username     string `json:"username" binding:"required"`     // логин юзера (admin)
	Password     string `json:"password" binding:"required,min=4"` // пароль
	Email        string `json:"email" binding:"required,email"`  // email юзера
	Type         string `json:"type" binding:"required,oneof=client partner"` // partner = dealer rights
	BillingPlan  string `json:"billingPlan"`                     // имя тарифа из get_billing_plans (опц.: пусто для WL без билинга)
}

// CreateAccountResult — данные созданного аккаунта (для возврата фронту)
type CreateWialonAccountResult struct {
	UserID        int64  `json:"userId"`
	ResourceID    int64  `json:"resourceId"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	Type          string `json:"type"`
	BillingPlan   string `json:"billingPlan"`
	DealerRights  bool   `json:"dealerRights"`
	ConnectionID  uint   `json:"connectionId"`
	SourceLabel   string `json:"sourceLabel"`
	Hierarchy     string `json:"hierarchy"`
}

// CreateAccount — создание Wialon-аккаунта. 5-step flow:
//  1. core/create_user  → userId
//  2. core/create_resource → resourceId
//  3. account/update_billing_properties — ресурс становится billing-аккаунтом
//  4. account/change_billing_plan
//  5. core/update_user_property (email)
//  6. core/update_custom_property (sys_account_enable_parent=1 если partner)
//  7. core/update_item_access (full access юзера к своему ресурсу)
//
// При ошибке на шагах 2+ — НЕ откатываем юзера. Wialon без транзакций. Логируем,
// возвращаем подробную ошибку фронту, дальше — ручная очистка через Wialon UI или
// /wialon/users/{id} DELETE.
func (s *WialonAccountService) CreateAccount(connectionID uint, req CreateWialonAccountRequest) (*CreateWialonAccountResult, error) {
	t0 := time.Now()

	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден: %w", connectionID, err)
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	creatorID := int64(0)
	if loginResp.User != nil && loginResp.User.ID > 0 {
		creatorID = loginResp.User.ID
	}
	if creatorID == 0 {
		return nil, fmt.Errorf("не удалось получить ID текущего юзера из login response")
	}

	// Шаг 1: create_user
	userID, err := s.callCreateUser(conn.Host, loginResp.Eid, creatorID, req.Username, req.Password)
	if err != nil {
		return nil, fmt.Errorf("create_user: %w", err)
	}
	log.Printf("⚡ Wialon CreateAccount: user '%s' создан id=%d", req.Username, userID)

	// Шаг 2: create_resource. ВАЖНО: creatorId должен быть НОВЫЙ юзер (userID), а НЕ login-юзер.
	// Wialon error 2014 «выбранный пользователь является создателем каких-либо элементов системы»
	// возникает если creatorId = существующий dealer (glomoskz). Ресурс должен принадлежать
	// своему юзеру.
	resourceID, err := s.callCreateResource(conn.Host, loginResp.Eid, userID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("create_resource: %w (юзер %d создан, нужна ручная очистка)", err, userID)
	}
	log.Printf("⚡ Wialon CreateAccount: resource '%s' создан id=%d", req.Name, resourceID)

	// Шаг 3: account/create_account — превращает ресурс в биллинг-аккаунт + сразу назначает план.
	// Заменяет связку update_billing_properties + change_billing_plan (метод update_billing_properties
	// не существует, выдаёт error 2 Invalid service).
	if req.BillingPlan != "" {
		if err := s.callCreateBillingAccount(conn.Host, loginResp.Eid, resourceID, req.BillingPlan); err != nil {
			return nil, fmt.Errorf("create_account billing '%s': %w (юзер=%d, ресурс=%d)", req.BillingPlan, err, userID, resourceID)
		}
	}

	// Шаг 5: email — это user property prp.email, ставится через item/update_custom_property с itemId=userID
	if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, userID, "email", req.Email); err != nil {
		log.Printf("⚠️ Wialon CreateAccount: не удалось установить email: %v (продолжаем)", err)
	}

	// Шаг 6: dealer rights если partner
	dealerRights := req.Type == "partner"
	if dealerRights {
		if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, resourceID, "sys_account_enable_parent", "1"); err != nil {
			return nil, fmt.Errorf("update_custom_property dealer: %w", err)
		}
	}

	// Шаг 7: full access юзеру на свой ресурс. Маска 0xFFFFFFFF (32 бита) — полный доступ; для большинства задач достаточно.
	if err := s.callUpdateItemAccess(conn.Host, loginResp.Eid, userID, resourceID); err != nil {
		log.Printf("⚠️ Wialon CreateAccount: не удалось выдать access: %v (продолжаем — Wialon обычно даёт права автоматом если creatorId совпадает)", err)
	}

	// bact (billing_account_id) на user — Wialon сам устанавливает его при account/create_account.
	// Ручное обновление не требуется.

	// Сохраняем в БД-кэш чтобы запись сразу появилась в /accounts UI без ожидания scheduler
	now := time.Now()
	stat := models.WialonObjectStat{
		ConnectionID:    connectionID,
		ResourceID:      resourceID,
		UserID:          userID,
		ObjectsTotal:    0,
		ObjectsActive:   0,
		LastCollectedAt: now,
	}
	_ = s.db.Create(&stat).Error // dup-key игнорируем

	sourceLabel := "WL(" + conn.UserName + ")"
	if conn.ConnectionType == models.WialonConnectionTypeHosting {
		sourceLabel = "WH(" + conn.UserName + ")"
	}

	result := &CreateWialonAccountResult{
		UserID:       userID,
		ResourceID:   resourceID,
		Name:         req.Name,
		Username:     req.Username,
		Type:         req.Type,
		BillingPlan:  req.BillingPlan,
		DealerRights: dealerRights,
		ConnectionID: connectionID,
		SourceLabel:  sourceLabel,
		Hierarchy:    sourceLabel + " > " + req.Name,
	}
	log.Printf("✅ Wialon CreateAccount: '%s' (user=%d, resource=%d, plan='%s', dealer=%v) за %s",
		req.Name, userID, resourceID, req.BillingPlan, dealerRights, time.Since(t0))
	return result, nil
}

// CreateWialonUserRequest — параметры создания одиночного юзера (без ресурса/билинга).
type CreateWialonUserRequest struct {
	Username  string `json:"username"`            // login юзера
	Password  string `json:"password"`            // пароль (Wialon ≥4 символов, латиница+цифры+спецсимволы)
	Email     string `json:"email,omitempty"`     // опционально
	CreatorID int64  `json:"creatorId,omitempty"` // ID учётной записи (билинг-аккаунта/юзера) под которой создаём; если 0 — текущий login-юзер
}

// CreateWialonUserResult — ответ create-user
type CreateWialonUserResult struct {
	UserID       int64  `json:"userId"`
	Username     string `json:"username"`
	Email        string `json:"email,omitempty"`
	ConnectionID uint   `json:"connectionId"`
	SourceLabel  string `json:"sourceLabel"`
	CreatorID    int64  `json:"creatorId"`
}

// CreateUser — создание одиночного Wialon-юзера (без ресурса/биллинга).
// Используется со страницы /users/create. creatorId — это ID существующего юзера/аккаунта
// в Wialon под которым будет числиться новый. Если 0 — берём login-юзера connection-токена.
func (s *WialonAccountService) CreateUser(connectionID uint, req CreateWialonUserRequest) (*CreateWialonUserResult, error) {
	t0 := time.Now()

	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден: %w", connectionID, err)
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	creatorID := req.CreatorID
	if creatorID == 0 {
		if loginResp.User != nil && loginResp.User.ID > 0 {
			creatorID = loginResp.User.ID
		}
	}
	if creatorID == 0 {
		return nil, fmt.Errorf("не удалось определить creator_id (ни в request, ни в login response)")
	}

	userID, err := s.callCreateUser(conn.Host, loginResp.Eid, creatorID, req.Username, req.Password)
	if err != nil {
		return nil, fmt.Errorf("create_user: %w", err)
	}
	log.Printf("⚡ Wialon CreateUser: user '%s' создан id=%d (creator=%d)", req.Username, userID, creatorID)

	if req.Email != "" {
		if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, userID, "email", req.Email); err != nil {
			log.Printf("⚠️ Wialon CreateUser: не удалось установить email: %v (user уже создан id=%d)", err, userID)
		}
	}

	sourceLabel := "WL(" + conn.UserName + ")"
	if conn.ConnectionType == models.WialonConnectionTypeHosting {
		sourceLabel = "WH(" + conn.UserName + ")"
	}

	log.Printf("✅ Wialon CreateUser: '%s' (id=%d) за %s", req.Username, userID, time.Since(t0))
	return &CreateWialonUserResult{
		UserID:       userID,
		Username:     req.Username,
		Email:        req.Email,
		ConnectionID: connectionID,
		SourceLabel:  sourceLabel,
		CreatorID:    creatorID,
	}, nil
}

// callCreateUser — core/create_user. Возвращает item.id.
func (s *WialonAccountService) callCreateUser(host, eid string, creatorID int64, username, password string) (int64, error) {
	params := map[string]interface{}{
		"creatorId": creatorID,
		"name":      username,
		"password":  password,
		"dataFlags": 1,
	}
	body, err := s.callRaw(host, eid, "core/create_user", params)
	if err != nil {
		return 0, err
	}
	return extractItemID(body)
}

// callCreateResource — core/create_resource (skipCreatorCheck=0). Возвращает item.id.
func (s *WialonAccountService) callCreateResource(host, eid string, creatorID int64, name string) (int64, error) {
	params := map[string]interface{}{
		"creatorId":        creatorID,
		"name":             name,
		"dataFlags":        1,
		"skipCreatorCheck": 0,
	}
	body, err := s.callRaw(host, eid, "core/create_resource", params)
	if err != nil {
		return 0, err
	}
	return extractItemID(body)
}

// callCreateBillingAccount — превращает avl_resource в биллинговый аккаунт с указанным планом.
// account/create_account params={itemId, plan}, response={} при успехе.
func (s *WialonAccountService) callCreateBillingAccount(host, eid string, resourceID int64, plan string) error {
	params := map[string]interface{}{
		"itemId": resourceID,
		"plan":   plan,
	}
	_, err := s.callRaw(host, eid, "account/create_account", params)
	return err
}

// UpdateWialonUserRequest — точечное редактирование Wialon-юзера (name + custom props).
// Поля nil-указатели: nil = не менять, "" = очистить.
type UpdateWialonUserRequest struct {
	Email    *string `json:"email,omitempty"`
	Name     *string `json:"name,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Telegram *string `json:"telegram,omitempty"`
}

// UpdateUser — изменяет Wialon-юзера. Email через item/update_custom_property,
// Name через item/update_name. Возвращает финальные значения.
func (s *WialonAccountService) UpdateUser(connectionID uint, userID int64, req UpdateWialonUserRequest) (*UpdateWialonUserResult, error) {
	t0 := time.Now()
	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден: %w", connectionID, err)
	}
	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	res := &UpdateWialonUserResult{UserID: userID, ConnectionID: connectionID}

	if req.Name != nil {
		if err := s.callRenameItem(conn.Host, loginResp.Eid, userID, *req.Name); err != nil {
			return nil, fmt.Errorf("update_name: %w", err)
		}
		res.Name = *req.Name
	}
	if req.Email != nil {
		if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, userID, "email", *req.Email); err != nil {
			return nil, fmt.Errorf("update email: %w", err)
		}
		res.Email = *req.Email
	}
	if req.Phone != nil {
		if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, userID, "phone", *req.Phone); err != nil {
			return nil, fmt.Errorf("update phone: %w", err)
		}
		res.Phone = *req.Phone
	}
	if req.Telegram != nil {
		if err := s.callUpdateCustomProperty(conn.Host, loginResp.Eid, userID, "telegram", *req.Telegram); err != nil {
			return nil, fmt.Errorf("update telegram: %w", err)
		}
		res.Telegram = *req.Telegram
	}

	log.Printf("✅ Wialon UpdateUser: id=%d за %s", userID, time.Since(t0))
	return res, nil
}

// UpdateWialonUserResult — ответ на PUT /wialon/connections/:id/users/:user_id.
type UpdateWialonUserResult struct {
	UserID       int64  `json:"user_id"`
	ConnectionID uint   `json:"connection_id"`
	Email        string `json:"email,omitempty"`
	Name         string `json:"name,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Telegram     string `json:"telegram,omitempty"`
}

// callRenameItem — item/update_name. itemId, name.
func (s *WialonAccountService) callRenameItem(host, eid string, itemID int64, name string) error {
	params := map[string]interface{}{
		"itemId": itemID,
		"name":   name,
	}
	_, err := s.callRaw(host, eid, "item/update_name", params)
	return err
}

func (s *WialonAccountService) callUpdateCustomProperty(host, eid string, itemID int64, name, value string) error {
	params := map[string]interface{}{
		"itemId": itemID,
		"name":   name,
		"value":  value,
	}
	_, err := s.callRaw(host, eid, "item/update_custom_property", params)
	return err
}

func (s *WialonAccountService) callUpdateItemAccess(host, eid string, userID, itemID int64) error {
	// 0xFFFFFFFF — полные права (32-bit max). Для accountId есть отдельный битмаск, но 0xFFFFFFFF ≈ всё.
	params := map[string]interface{}{
		"userId":     userID,
		"itemId":     itemID,
		"accessMask": 0xFFFFFFFF,
	}
	_, err := s.callRaw(host, eid, "user/update_item_access", params)
	return err
}

// callRaw — низкоуровневый вызов одного svc. В отличие от callBatch, тут не нужен массив.
func (s *WialonAccountService) callRaw(host, eid, svc string, params map[string]interface{}) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	q := url.Values{}
	q.Set("svc", svc)
	q.Set("sid", eid)
	q.Set("params", string(paramsJSON))

	resp, err := s.wialonService.httpClient.Post(apiURL+"?"+q.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	body, err := readAllBytes(resp.Body, 0)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Wialon при ошибке отдаёт {"error": N, "reason": "..."}
	var errResp struct {
		Error  int    `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != 0 {
		return body, fmt.Errorf("wialon %s error %d: %s", svc, errResp.Error, errResp.Reason)
	}
	if strings.Contains(string(body), `"error"`) {
		// На случай если parsed-error пуст из-за регистра/типа — логируем тело
		log.Printf("⚠️ Wialon %s suspicious response: %s", svc, string(body)[:min(200, len(body))])
	}
	return body, nil
}

// extractItemID — общий парсер ответа на create_user/create_resource: {"item": {"id": N, ...}, ...}
func extractItemID(body []byte) (int64, error) {
	var resp struct {
		Item map[string]interface{} `json:"item"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("парсинг ответа: %w (raw: %s)", err, string(body)[:min(200, len(body))])
	}
	if resp.Item == nil {
		return 0, fmt.Errorf("пустой item в ответе: %s", string(body)[:min(200, len(body))])
	}
	if id, ok := resp.Item["id"].(float64); ok {
		return int64(id), nil
	}
	return 0, fmt.Errorf("нет item.id в ответе: %s", string(body)[:min(200, len(body))])
}

// readAllBytes — обёртка для io.ReadAll. Lim=0 → без лимита.
func readAllBytes(r interface{ Read(p []byte) (n int, err error) }, lim int) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if lim > 0 && len(buf) > lim {
				return buf[:lim], nil
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
