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

// GetBillingPlans возвращает список доступных тарифов для подключения.
// Используется фронтом при создании аккаунта — селектор «Тарифный план».
func (s *WialonAccountService) GetBillingPlans(connectionID uint) ([]WialonBillingPlan, error) {
	var conn models.WialonConnection
	if err := s.db.Where("id = ? AND is_active = ?", connectionID, true).First(&conn).Error; err != nil {
		return nil, fmt.Errorf("connection %d не найден: %w", connectionID, err)
	}

	loginResp, err := s.wialonService.LoginWithHost(conn.Host, conn.Token)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer func() { _ = s.wialonService.LogoutWithHost(conn.Host, loginResp.Eid) }()

	// userId — login user. По SDK params {"userId": <id>}; userId=0 не всегда работает (зависит от прав)
	currentUserID := int64(0)
	if loginResp.User != nil {
		currentUserID = loginResp.User.ID
	}

	// Wialon SDK: params={} (БЕЗ userId) для получения собственного плана + всех подчинённых.
	// Если шлём userId — сужает до 1 плана. Запрос доступен ТОЛЬКО для top-level dealer
	// (на WL не сработает — фолбэк через get_account_data).
	body, err := s.callRaw(conn.Host, loginResp.Eid, "account/get_billing_plans", map[string]interface{}{})
	if err != nil {
		log.Printf("⚠️ get_billing_plans не доступен (%v), фолбэк через account/get_account_data", err)
		return s.getCurrentPlanFallback(conn.Host, loginResp.Eid, currentUserID)
	}

	// Ответ Wialon SDK: { plan: {name, ...}, subPlans: [{name, ...}, ...] }
	// Собираем имя своего плана + всех подчинённых.
	var resp struct {
		Plan     map[string]interface{}   `json:"plan"`
		SubPlans []map[string]interface{} `json:"subPlans"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("парсинг billing_plans: %w (raw: %s)", err, string(body)[:min(300, len(body))])
	}

	plans := make([]WialonBillingPlan, 0)
	if resp.Plan != nil {
		if name, ok := resp.Plan["name"].(string); ok && name != "" {
			plans = append(plans, WialonBillingPlan{Name: name})
		}
	}
	for _, p := range resp.SubPlans {
		if name, ok := p["name"].(string); ok && name != "" {
			plans = append(plans, WialonBillingPlan{Name: name})
		}
	}
	log.Printf("⚡ Wialon billing_plans: получено %d тарифов (plan + subPlans) для %s", len(plans), conn.Name)
	return plans, nil
}

// getCurrentPlanFallback — когда get_billing_plans не доступен, читаем план текущего юзера
// через account/get_account_data type=2. На Wialon Local билинга может не быть вообще —
// в этом случае возвращаем пустой массив без ошибки.
func (s *WialonAccountService) getCurrentPlanFallback(host, eid string, userID int64) ([]WialonBillingPlan, error) {
	body, err := s.callRaw(host, eid, "account/get_account_data", map[string]interface{}{
		"itemId": userID,
		"type":   2,
	})
	if err != nil {
		log.Printf("⚠️ get_account_data fallback не доступен (%v) — возвращаем пустой список тарифов (Wialon Local без билинга)", err)
		return []WialonBillingPlan{}, nil
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if plan, ok := data["plan"].(map[string]interface{}); ok {
		if name, ok := plan["name"].(string); ok && name != "" {
			return []WialonBillingPlan{{Name: name}}, nil
		}
	}
	return []WialonBillingPlan{}, nil
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

	// Шаг 2: create_resource
	resourceID, err := s.callCreateResource(conn.Host, loginResp.Eid, creatorID, req.Name)
	if err != nil {
		return nil, fmt.Errorf("create_resource: %w (юзер %d создан, нужна ручная очистка)", err, userID)
	}
	log.Printf("⚡ Wialon CreateAccount: resource '%s' создан id=%d", req.Name, resourceID)

	// Шаг 3: update_billing_properties — ресурс → billing account
	if err := s.callUpdateBillingProperties(conn.Host, loginResp.Eid, resourceID); err != nil {
		return nil, fmt.Errorf("update_billing_properties: %w (юзер=%d, ресурс=%d)", err, userID, resourceID)
	}

	// Шаг 4: change_billing_plan (если задан — на Wialon Local плана может не быть)
	if req.BillingPlan != "" {
		if err := s.callChangeBillingPlan(conn.Host, loginResp.Eid, resourceID, req.BillingPlan); err != nil {
			log.Printf("⚠️ change_billing_plan '%s' не сработал: %v (продолжаем — для WL билинг может быть не нужен)", req.BillingPlan, err)
		}
	}

	// Шаг 5: email
	if err := s.callUpdateUserProperty(conn.Host, loginResp.Eid, userID, "email", req.Email); err != nil {
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

	// Также сохранить bact = resourceID на user (важно для frontend identity)
	if err := s.callUpdateUserProperty(conn.Host, loginResp.Eid, userID, "bact", fmt.Sprintf("%d", resourceID)); err != nil {
		log.Printf("⚠️ Wialon CreateAccount: не удалось установить bact: %v", err)
	}

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

// callUpdateBillingProperties — нулевые балансы, начало биллинга
func (s *WialonAccountService) callUpdateBillingProperties(host, eid string, resourceID int64) error {
	params := map[string]interface{}{
		"itemId":      resourceID,
		"balance":     "0",
		"days":        0,
		"blockBalance": "0",
		"blockDays":    0,
		"denyBalance":  "0",
		"denyDays":     0,
	}
	_, err := s.callRaw(host, eid, "account/update_billing_properties", params)
	return err
}

func (s *WialonAccountService) callChangeBillingPlan(host, eid string, resourceID int64, plan string) error {
	params := map[string]interface{}{
		"itemId": resourceID,
		"plan":   plan,
	}
	_, err := s.callRaw(host, eid, "account/change_billing_plan", params)
	return err
}

func (s *WialonAccountService) callUpdateUserProperty(host, eid string, userID int64, name, value string) error {
	params := map[string]interface{}{
		"userId": userID,
		"name":   name,
		"value":  value,
	}
	_, err := s.callRaw(host, eid, "core/update_user_property", params)
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
