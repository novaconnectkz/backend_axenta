package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend_axenta/models"
	"backend_axenta/utils"
)

// plainPassword — расшифровка SkifConnection.Password (AES-GCM, format "enc:v1:...").
// Legacy plaintext (без префикса) возвращается as-is — для постепенной миграции.
func (s *SkifService) plainPassword(conn *models.SkifConnection) (string, error) {
	return utils.DecryptString(conn.Password)
}

// SkifService — клиент SKIF.PRO API. Cookie session-based auth.
//
// Документация: ACRM-Brain/wiki/sources/skif-api.md + skif-api/*.md
type SkifService struct {
	db *gorm.DB
}

func NewSkifService(db *gorm.DB) *SkifService {
	return &SkifService{db: db}
}

// HTTPClient создаёт http.Client с cookie jar и опционально пред-загруженной cookie из БД.
// При успешном login обновляет SessionCookie в БД.
func (s *SkifService) httpClient(conn *models.SkifConnection) (*http.Client, *cookiejar.Jar) {
	jar, _ := cookiejar.New(nil)
	if conn.SessionCookie != "" {
		if u, err := url.Parse(conn.BaseURL); err == nil {
			cookies := parseCookieString(conn.SessionCookie, u)
			jar.SetCookies(u, cookies)
		}
	}
	return &http.Client{
		Jar:     jar,
		Timeout: 30 * time.Second,
	}, jar
}

// Login выполняет POST /api_v1/login и сохраняет cookie session в БД.
// При повторных вызовах перезаписывает существующую сессию.
func (s *SkifService) Login(conn *models.SkifConnection) error {
	client, jar := s.httpClient(conn)
	plainPwd, err := s.plainPassword(conn)
	if err != nil {
		return fmt.Errorf("decrypt password: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"userProviderId": conn.Login,
		"provider_key":   "TEXT",
		"password":       plainPwd,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("login http: %v", err))
		return err
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		errMsg := fmt.Sprintf("login status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 300))
		s.recordError(conn, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	u, _ := url.Parse(conn.BaseURL)
	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		s.recordError(conn, "login: server did not set session cookie")
		return fmt.Errorf("no session cookie in response")
	}

	cookieStr := serializeCookies(cookies)
	now := time.Now()
	updates := map[string]interface{}{
		"session_cookie": cookieStr,
		"last_login_at":  &now,
		"error_message":  "",
		"error_count":    0,
		"last_error_at":  nil,
	}
	if err := s.db.Model(conn).Updates(updates).Error; err != nil {
		log.Printf("⚠️ SkifService: не удалось сохранить session_cookie для conn=%d: %v", conn.ID, err)
	}
	conn.SessionCookie = cookieStr
	conn.LastLoginAt = &now
	log.Printf("✅ SKIF login OK: conn=%d (%s) login=%s", conn.ID, conn.Name, conn.Login)
	return nil
}

// authedRequest выполняет HTTP-запрос с cookie session, авто-релогинит при 401/422.
func (s *SkifService) authedRequest(conn *models.SkifConnection, method, path string, body io.Reader) ([]byte, int, error) {
	if conn.SessionCookie == "" {
		if err := s.Login(conn); err != nil {
			return nil, 0, fmt.Errorf("auto-login: %w", err)
		}
	}
	rawBody, status, err := s.doRequest(conn, method, path, body)
	if err == nil && status != 401 && status != 422 {
		return rawBody, status, nil
	}
	if status == 401 || status == 422 {
		log.Printf("🔄 SKIF re-login (conn=%d, status=%d)", conn.ID, status)
		if err := s.Login(conn); err != nil {
			return rawBody, status, fmt.Errorf("re-login: %w", err)
		}
		return s.doRequest(conn, method, path, body)
	}
	return rawBody, status, err
}

func (s *SkifService) doRequest(conn *models.SkifConnection, method, path string, body io.Reader) ([]byte, int, error) {
	client, _ := s.httpClient(conn)
	url := strings.TrimRight(conn.BaseURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	return rawBody, resp.StatusCode, nil
}

// TestConnection выполняет login + GET /api_v1/me для верификации creds.
func (s *SkifService) TestConnection(conn *models.SkifConnection) (map[string]interface{}, error) {
	if err := s.Login(conn); err != nil {
		return nil, err
	}
	rawBody, status, err := s.authedRequest(conn, "GET", "/api_v1/me", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("/me status=%d body=%s", status, truncate(string(rawBody), 200))
	}
	var me map[string]interface{}
	_ = json.Unmarshal(rawBody, &me)
	return me, nil
}

// SkifUnitDTO — минимальный набор полей юнита из POST /api_v1/units/list.
// id — UUID.
// SKIF /units/list НЕ возвращает поле `created` (даже при явном запросе в fields).
// Дату создания берём из states[0].date_from (первое состояние = когда юнит появился),
// фильтруя epoch zero "1970-01-01" как невалидное значение.
// См. wiki/sources/skif-api/obekty.md.
type SkifUnitDTO struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	IMEI        string          `json:"imei"`
	Phone       string          `json:"phoneNumber"`
	Model       string          `json:"model"`
	IsActive    bool            `json:"isActive"`
	CompanyName string          `json:"companyName"`
	States      []skifUnitState `json:"states"`
}

type skifUnitState struct {
	Name     string `json:"name"`
	DateFrom string `json:"date_from"`
}

type skifUnitsListResponse struct {
	Max  int           `json:"max"`
	List []SkifUnitDTO `json:"list"`
}

// skifMeResponse — упрощённый ответ GET /api_v1/me с companies[].
type skifMeResponse struct {
	ID            string             `json:"id"`
	Email         string             `json:"email"`
	Name          string             `json:"name"`
	Companies     []skifCompanyBrief `json:"companies"`
	ActiveCompany skifCompanyBrief   `json:"active_company"`
}

type skifCompanyBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SyncUnits загружает все объекты из SKIF через все доступные компании пользователя.
//
// Pattern для интегратора:
//  1. login (если cookie session невалиден)
//  2. GET /api_v1/me → companies[]
//  3. для каждой company:
//     - POST /api_v1/company/change/:id (cookie session переключается)
//     - POST /api_v1/units/list с pagination → объекты этой компании
//     - upsert в skif_units (skif_company_id = company.ID)
//  4. сохранить финальную cookie session в БД
//
// Используется один http.Client с persistent cookie jar — все шаги в одной сессии.
func (s *SkifService) SyncUnits(conn *models.SkifConnection) (int, error) {
	now := time.Now()
	client, jar := s.httpClient(conn)

	// 1. Ensure logged in: если cookie уже есть, /me попробует с ней; если 401/422 — relogin.
	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, err
		}
	}

	// 2. Получаем /me → список доступных компаний.
	me, status, err := s.fetchMe(conn, client)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("/me: %v", err))
		return 0, err
	}
	if status == 401 || status == 422 {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, err
		}
		me, status, err = s.fetchMe(conn, client)
		if err != nil || status != 200 {
			s.recordError(conn, fmt.Sprintf("/me retry status=%d err=%v", status, err))
			return 0, fmt.Errorf("/me retry: %w", err)
		}
	}
	if status != 200 {
		s.recordError(conn, fmt.Sprintf("/me status=%d", status))
		return 0, fmt.Errorf("/me status=%d", status)
	}

	companies := me.Companies
	if len(companies) == 0 {
		// Fallback: hayır companies — синкаем активную компанию (стандартное поведение).
		companies = []skifCompanyBrief{me.ActiveCompany}
	}

	log.Printf("🔄 SKIF conn=%d: %d companies для sync", conn.ID, len(companies))

	saved := 0
	failedCompanies := 0
	seenUnitIDs := make([]string, 0, 2000)
	// Rate limit SKIF: ~60 req/min. Между companies минимум 1s + retry на 429.
	const throttle = 1100 * time.Millisecond
	for i, comp := range companies {
		if i > 0 {
			time.Sleep(throttle)
		}
		if err := s.switchWithRetry(conn, client, comp.ID); err != nil {
			log.Printf("⚠️ SKIF conn=%d company switch %s (%s): %v", conn.ID, comp.Name, comp.ID, err)
			failedCompanies++
			continue
		}
		n, ids, err := s.fetchUnitsForCompany(conn, client, comp, now)
		if err != nil {
			log.Printf("⚠️ SKIF conn=%d company %s units: %v", conn.ID, comp.Name, err)
			failedCompanies++
			continue
		}
		saved += n
		seenUnitIDs = append(seenUnitIDs, ids...)
		if (i+1)%10 == 0 || i == len(companies)-1 {
			log.Printf("   📦 SKIF: %d/%d компаний обработано (saved=%d)", i+1, len(companies), saved)
		}
	}

	// Mark deleted: юниты которых нет в выгрузке (только если errorRate приемлем).
	// Если упало больше половины компаний — пропускаем (риск false-positive).
	if len(seenUnitIDs) > 0 && failedCompanies*2 < len(companies) {
		res := s.db.Model(&models.SkifUnit{}).
			Where("connection_id = ? AND skif_deleted_at IS NULL AND skif_unit_id NOT IN ?", conn.ID, seenUnitIDs).
			Update("skif_deleted_at", now)
		if res.Error != nil {
			log.Printf("⚠️ SKIF mark deleted: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("🗑 SKIF conn=%d: помечено как удалённые %d юнитов", conn.ID, res.RowsAffected)
		}
	}

	// 4. Финальная cookie session → в БД.
	s.saveSessionFromJar(conn, jar)

	if failedCompanies > 0 {
		log.Printf("⚠️ SKIF sync conn=%d: %d/%d компаний упали", conn.ID, failedCompanies, len(companies))
	}

	// Обновляем счётчики в connection
	s.db.Model(conn).Updates(map[string]interface{}{
		"units_count":   saved,
		"last_sync_at":  &now,
		"error_message": "",
		"error_count":   0,
		"last_error_at": nil,
	})

	log.Printf("✅ SKIF sync units: conn=%d upserted=%d (companies=%d, failed=%d)", conn.ID, saved, len(companies), failedCompanies)
	return saved, nil
}

// loginWithClient — login через переданный client (общий cookie jar для всего sync run).
// Дублирует Login(), но без создания нового client'а.
func (s *SkifService) loginWithClient(conn *models.SkifConnection, client *http.Client) error {
	plainPwd, err := s.plainPassword(conn)
	if err != nil {
		return fmt.Errorf("decrypt password: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"userProviderId": conn.Login,
		"provider_key":   "TEXT",
		"password":       plainPwd,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("login req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		errMsg := fmt.Sprintf("login status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 300))
		s.recordError(conn, errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	now := time.Now()
	conn.LastLoginAt = &now
	return nil
}

// fetchMe — GET /api_v1/me с переданным client (использует существующий cookie jar).
func (s *SkifService) fetchMe(conn *models.SkifConnection, client *http.Client) (*skifMeResponse, int, error) {
	req, err := http.NewRequest("GET", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/me", nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, nil
	}
	var me skifMeResponse
	if err := json.Unmarshal(rawBody, &me); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse /me: %w", err)
	}
	return &me, resp.StatusCode, nil
}

// switchWithRetry — switchCompany с retry на 429 (rate limit) и 401 (session expired).
// 429: 3 попытки экспоненциально 6s → 12s → 24s.
// 401: один re-login + повтор switch.
func (s *SkifService) switchWithRetry(conn *models.SkifConnection, client *http.Client, companyID string) error {
	backoffs := []time.Duration{6 * time.Second, 12 * time.Second, 24 * time.Second}
	reloggedIn := false
	var lastErr error
	for i := 0; i <= len(backoffs); i++ {
		err := s.switchCompany(conn, client, companyID)
		if err == nil {
			return nil
		}
		lastErr = err
		if strings.Contains(err.Error(), "status=401") && !reloggedIn {
			if relErr := s.loginWithClient(conn, client); relErr != nil {
				return fmt.Errorf("re-login on 401: %w", relErr)
			}
			reloggedIn = true
			continue
		}
		if !strings.Contains(err.Error(), "status=429") {
			return err
		}
		if i < len(backoffs) {
			time.Sleep(backoffs[i])
		}
	}
	return lastErr
}

// switchCompany — POST /api_v1/company/change/:id, обновляет cookie session client'а.
func (s *SkifService) switchCompany(conn *models.SkifConnection, client *http.Client, companyID string) error {
	body, _ := json.Marshal(map[string]string{"id": companyID})
	req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/company/change/"+companyID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("change status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 200))
	}
	return nil
}

// fetchUnitsForCompany — POST /api_v1/units/list для текущей (после switch) компании
// + upsert в skif_units. Возвращает кол-во upserted и список skif_unit_id (для mark-deleted).
func (s *SkifService) fetchUnitsForCompany(conn *models.SkifConnection, client *http.Client, comp skifCompanyBrief, now time.Time) (int, []string, error) {
	const pageSize = 500
	from := 0
	saved := 0
	seenIDs := make([]string, 0, 64)
	for {
		body, _ := json.Marshal(map[string]interface{}{
			"from":       from,
			"count":      pageSize,
			"sortField":  "name",
			"sortDesc":   "false",
			"conditions": []interface{}{},
			// "created" SKIF не отдаёт. Берём states[0].date_from как proxy.
			"fields": []string{"name", "imei", "phoneNumber", "model", "isActive", "companyName", "states"},
		})
		req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/units/list", bytes.NewReader(body))
		if err != nil {
			return saved, seenIDs, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return saved, seenIDs, err
		}
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return saved, seenIDs, fmt.Errorf("/units/list status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 200))
		}
		var page skifUnitsListResponse
		if err := json.Unmarshal(rawBody, &page); err != nil {
			return saved, seenIDs, fmt.Errorf("parse: %w", err)
		}
		if len(page.List) == 0 {
			break
		}
		for _, u := range page.List {
			row := models.SkifUnit{
				ConnectionID:    conn.ID,
				SkifUnitID:      u.ID,
				Name:            u.Name,
				IMEI:            u.IMEI,
				Phone:           u.Phone,
				Model:           u.Model,
				IsActive:        u.IsActive,
				CompanyID:       conn.CompanyID,
				SkifCompanyID:   comp.ID,
				SkifCompany:     comp.Name,
				LastCollectedAt: now,
				SkifCreatedAt:   extractSkifCreatedAt(u.States),
			}
			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_unit_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"name":              row.Name,
					"imei":              row.IMEI,
					"phone":             row.Phone,
					"model":             row.Model,
					"is_active":         row.IsActive,
					"company_id":        row.CompanyID,
					"skif_company_id":   row.SkifCompanyID,
					"skif_company":      row.SkifCompany,
					"skif_created_at":   row.SkifCreatedAt,
					"last_collected_at": row.LastCollectedAt,
					"skif_deleted_at":   nil, // юнит вернулся → сброс mark
					"updated_at":        time.Now(),
				}),
			}).Create(&row).Error; err != nil {
				log.Printf("⚠️ SKIF upsert unit %s: %v", u.ID, err)
				continue
			}
			seenIDs = append(seenIDs, u.ID)
			saved++
		}
		from += pageSize
		if page.Max > 0 && from >= page.Max {
			break
		}
		if from >= 50000 {
			break
		}
	}
	return saved, seenIDs, nil
}

// SkifUserDTO — поля юзера из POST /api_v1/users/query (response = массив объектов /me).
// "created" SKIF не возвращает в /users/query, аналогично unit'ам.
type SkifUserDTO struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Email          string             `json:"email"`
	Phone          string             `json:"phone"`
	Role           skifUserRole       `json:"role"`
	IsApproved     bool               `json:"is_approved"`
	IsDriver       bool               `json:"is_driver"`
	Code           interface{}        `json:"code"`             // в API может быть number или string (RFID id)
	Language       skifUserLanguage   `json:"language"`
	Details        string             `json:"details"`
	TelegramChatID interface{}        `json:"telegram_chat_id"` // в API number, но безопаснее interface{}
	Companies      []skifCompanyBrief `json:"companies"`
}

type skifUserRole struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type skifUserLanguage struct {
	Key string `json:"key"`
}

type skifUsersQueryResponse struct {
	List []SkifUserDTO `json:"list"`
	Max  int           `json:"max"`
}

// SyncUsers загружает всех пользователей SKIF через все доступные компании.
//
// Pattern идентичен SyncUnits:
//  1. login (если cookie невалиден)
//  2. GET /me → companies[]
//  3. для каждой company: switch + POST /api_v1/users/query (paginated)
//  4. upsert в skif_users (один email × N companies = N строк)
//  5. mark deleted: записи которых нет в выгрузке
//  6. сохранение cookie session
//
// Rate limit SKIF ~60/min: throttle 1.1s между companies.
func (s *SkifService) SyncUsers(conn *models.SkifConnection) (int, error) {
	now := time.Now()
	client, jar := s.httpClient(conn)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, err
		}
	}

	me, status, err := s.fetchMe(conn, client)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("/me users: %v", err))
		return 0, err
	}
	if status == 401 || status == 422 {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, err
		}
		me, status, err = s.fetchMe(conn, client)
		if err != nil || status != 200 {
			s.recordError(conn, fmt.Sprintf("/me users retry status=%d err=%v", status, err))
			return 0, fmt.Errorf("/me retry: %w", err)
		}
	}
	if status != 200 {
		s.recordError(conn, fmt.Sprintf("/me users status=%d", status))
		return 0, fmt.Errorf("/me users status=%d", status)
	}

	companies := me.Companies
	if len(companies) == 0 {
		companies = []skifCompanyBrief{me.ActiveCompany}
	}

	log.Printf("🔄 SKIF SyncUsers conn=%d: %d companies", conn.ID, len(companies))

	saved := 0
	failedCompanies := 0
	type seenKey struct {
		userID    string
		companyID string
	}
	seen := make([]seenKey, 0, 256)
	const throttle = 1100 * time.Millisecond
	for i, comp := range companies {
		if i > 0 {
			time.Sleep(throttle)
		}
		if err := s.switchWithRetry(conn, client, comp.ID); err != nil {
			log.Printf("⚠️ SKIF SyncUsers conn=%d switch %s (%s): %v", conn.ID, comp.Name, comp.ID, err)
			failedCompanies++
			continue
		}
		n, ids, err := s.fetchUsersForCompany(conn, client, comp, now)
		if err != nil {
			log.Printf("⚠️ SKIF SyncUsers conn=%d company %s: %v", conn.ID, comp.Name, err)
			failedCompanies++
			continue
		}
		saved += n
		for _, id := range ids {
			seen = append(seen, seenKey{userID: id, companyID: comp.ID})
		}
		if (i+1)%10 == 0 || i == len(companies)-1 {
			log.Printf("   👥 SKIF users: %d/%d компаний (saved=%d)", i+1, len(companies), saved)
		}
	}

	// Mark deleted: записи которых нет в выгрузке. Делаем per-company чтобы не зацепить
	// чужие записи если одна company упала.
	if failedCompanies*2 < len(companies) {
		seenByCompany := make(map[string][]string)
		for _, k := range seen {
			seenByCompany[k.companyID] = append(seenByCompany[k.companyID], k.userID)
		}
		for companyID, userIDs := range seenByCompany {
			if len(userIDs) == 0 {
				continue
			}
			res := s.db.Model(&models.SkifUser{}).
				Where("connection_id = ? AND skif_company_id = ? AND skif_deleted_at IS NULL AND skif_user_id NOT IN ?",
					conn.ID, companyID, userIDs).
				Update("skif_deleted_at", now)
			if res.Error != nil {
				log.Printf("⚠️ SKIF mark deleted users: %v", res.Error)
			} else if res.RowsAffected > 0 {
				log.Printf("🗑 SKIF SyncUsers conn=%d company=%s: помечено %d юзеров", conn.ID, companyID, res.RowsAffected)
			}
		}
	}

	s.saveSessionFromJar(conn, jar)

	if failedCompanies > 0 {
		log.Printf("⚠️ SKIF SyncUsers conn=%d: %d/%d компаний упали", conn.ID, failedCompanies, len(companies))
	}

	s.db.Model(conn).Updates(map[string]interface{}{
		"users_count": saved,
		"updated_at":  time.Now(),
	})

	log.Printf("✅ SKIF SyncUsers: conn=%d upserted=%d (companies=%d, failed=%d)", conn.ID, saved, len(companies), failedCompanies)
	return saved, nil
}

// fetchUsersForCompany — POST /api_v1/users/query для текущей (после switch) компании.
// Пагинация from+limit. Sortfield=name. Возвращает кол-во upserted и список skif_user_id.
func (s *SkifService) fetchUsersForCompany(conn *models.SkifConnection, client *http.Client, comp skifCompanyBrief, now time.Time) (int, []string, error) {
	const pageSize = 500
	from := 0
	saved := 0
	seenIDs := make([]string, 0, 64)
	for {
		body, _ := json.Marshal(map[string]interface{}{
			"from":      from,
			"limit":     pageSize,
			"sortField": "name",
			"sortDesc":  false,
		})
		req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/users/query", bytes.NewReader(body))
		if err != nil {
			return saved, seenIDs, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return saved, seenIDs, err
		}
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return saved, seenIDs, fmt.Errorf("/users/query status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 200))
		}

		// SKIF может вернуть либо {list:[], max} либо просто массив — пробуем оба.
		var page skifUsersQueryResponse
		if err := json.Unmarshal(rawBody, &page); err != nil || len(page.List) == 0 {
			var arr []SkifUserDTO
			if errArr := json.Unmarshal(rawBody, &arr); errArr == nil && len(arr) > 0 {
				page.List = arr
			} else if err != nil {
				return saved, seenIDs, fmt.Errorf("parse: %w", err)
			}
		}

		if len(page.List) == 0 {
			break
		}

		for _, u := range page.List {
			tgChatID := ""
			if u.TelegramChatID != nil {
				tgChatID = fmt.Sprintf("%v", u.TelegramChatID)
			}
			code := ""
			if u.Code != nil {
				code = fmt.Sprintf("%v", u.Code)
			}
			row := models.SkifUser{
				ConnectionID:    conn.ID,
				SkifUserID:      u.ID,
				SkifCompanyID:   comp.ID,
				Name:            u.Name,
				Email:           u.Email,
				Phone:           u.Phone,
				RoleKey:         u.Role.Key,
				RoleName:        u.Role.Value,
				IsActive:        u.Role.Key != "" && u.Role.Key != "NoAccess",
				IsApproved:      u.IsApproved,
				IsDriver:        u.IsDriver,
				Code:            code,
				Language:        u.Language.Key,
				Details:         u.Details,
				TelegramChatID:  tgChatID,
				CompanyID:       conn.CompanyID,
				SkifCompany:     comp.Name,
				LastCollectedAt: now,
			}
			if err := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_user_id"}, {Name: "skif_company_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"name":              row.Name,
					"email":             row.Email,
					"phone":             row.Phone,
					"role_key":          row.RoleKey,
					"role_name":         row.RoleName,
					"is_active":         row.IsActive,
					"is_approved":       row.IsApproved,
					"is_driver":         row.IsDriver,
					"code":              row.Code,
					"language":          row.Language,
					"details":           row.Details,
					"telegram_chat_id":  row.TelegramChatID,
					"company_id":        row.CompanyID,
					"skif_company":     row.SkifCompany,
					"last_collected_at": row.LastCollectedAt,
					"skif_deleted_at":   nil,
					"updated_at":        time.Now(),
				}),
			}).Create(&row).Error; err != nil {
				log.Printf("⚠️ SKIF upsert user %s: %v", u.ID, err)
				continue
			}
			seenIDs = append(seenIDs, u.ID)
			saved++
		}

		from += pageSize
		if page.Max > 0 && from >= page.Max {
			break
		}
		if len(page.List) < pageSize {
			break
		}
		if from >= 50000 {
			break
		}
	}
	return saved, seenIDs, nil
}

// UpdateSkifUserRequest — поля юзера для PUT /api_v1/users/:id.
// nil = поле не передаём (не меняем). Минимум одно поле обязательно.
type UpdateSkifUserRequest struct {
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Phone    *string `json:"phone,omitempty"`
	Password *string `json:"password,omitempty"`
	IsDriver *bool   `json:"is_driver,omitempty"`
	Code     *string `json:"code,omitempty"`
	Details  *string `json:"details,omitempty"`
	Language *string `json:"language,omitempty"` // ключ языка ("ru" / "en")
}

// UpdateUser — точечное редактирование юзера SKIF в контексте конкретной company.
// PUT /api_v1/users/:userId. Перед запросом делаем switchCompany(skifCompanyID),
// потому что SKIF проверяет роль текущей активной компании.
//
// Возвращает обновлённый объект юзера (для UI обновления карточки).
func (s *SkifService) UpdateUser(conn *models.SkifConnection, skifCompanyID, skifUserID string, req UpdateSkifUserRequest) (map[string]interface{}, error) {
	client, jar := s.httpClient(conn)
	defer s.saveSessionFromJar(conn, jar)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return nil, err
		}
	}

	if err := s.switchWithRetry(conn, client, skifCompanyID); err != nil {
		return nil, fmt.Errorf("switch company: %w", err)
	}

	body := map[string]interface{}{}
	if req.Name != nil {
		body["name"] = *req.Name
	}
	if req.Email != nil {
		body["email"] = *req.Email
	}
	if req.Phone != nil {
		body["phone"] = *req.Phone
	}
	if req.Password != nil {
		body["password"] = *req.Password
	}
	if req.IsDriver != nil {
		body["is_driver"] = *req.IsDriver
	}
	if req.Code != nil {
		body["code"] = *req.Code
	}
	if req.Details != nil {
		body["details"] = *req.Details
	}
	if req.Language != nil {
		body["language"] = map[string]string{"key": *req.Language}
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	jsonBody, _ := json.Marshal(body)
	req2, err := http.NewRequest("PUT", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/users/"+skifUserID, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json")
	resp, err := client.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PUT /users/%s status=%d body=%s", skifUserID, resp.StatusCode, truncate(string(rawBody), 300))
	}

	var result map[string]interface{}
	_ = json.Unmarshal(rawBody, &result)

	// Обновляем skif_users-запись локально (минимум — те поля что отправляли)
	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.Code != nil {
		updates["code"] = *req.Code
	}
	if req.Details != nil {
		updates["details"] = *req.Details
	}
	if req.Language != nil {
		updates["language"] = *req.Language
	}
	if req.IsDriver != nil {
		updates["is_driver"] = *req.IsDriver
	}
	s.db.Model(&models.SkifUser{}).
		Where("connection_id = ? AND skif_user_id = ? AND skif_company_id = ?", conn.ID, skifUserID, skifCompanyID).
		Updates(updates)

	return result, nil
}

// CreateSkifUserRequest — поля для POST /api_v1/users.
// Name+Password обязательны. RoleKey: EDITOR / ADMIN / READER / SUPERVISOR / NoAccess.
type CreateSkifUserRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Password string `json:"password"`
	Phone    string `json:"phone,omitempty"`
	RoleKey  string `json:"role_key,omitempty"` // дефолт EDITOR если пусто
	Code     string `json:"code,omitempty"`
	Details  string `json:"details,omitempty"`
	Language string `json:"language,omitempty"` // "ru" / "en", дефолт "ru"
	IsDriver bool   `json:"is_driver,omitempty"`
}

// skifRoleResp — упрощённый ответ GET /api_v1/roles.
type skifRoleResp struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// fetchRoleIDByKey — резолвит role.id по role.key через GET /api_v1/roles.
// Кэширование не делаем — endpoint вызывается раз на create, ролей <10.
func (s *SkifService) fetchRoleIDByKey(conn *models.SkifConnection, client *http.Client, roleKey string) (string, string, error) {
	req, err := http.NewRequest("GET", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/roles", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("/roles status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 200))
	}
	// SKIF может вернуть и массив, и {list: []}
	var arr []skifRoleResp
	if err := json.Unmarshal(rawBody, &arr); err != nil || len(arr) == 0 {
		var wrap struct {
			List []skifRoleResp `json:"list"`
		}
		if err := json.Unmarshal(rawBody, &wrap); err != nil {
			return "", "", fmt.Errorf("parse /roles: %w", err)
		}
		arr = wrap.List
	}
	for _, r := range arr {
		if strings.EqualFold(r.Key, roleKey) {
			return r.ID, r.Value, nil
		}
	}
	return "", "", fmt.Errorf("role with key=%s not found in /roles", roleKey)
}

// CreateUserInCompany — создание юзера в конкретной company SKIF.
// Workflow: login → switch(skifCompanyID) → GET /roles → POST /api_v1/users.
//
// Возвращает (skifUserID, response, error). При успехе создаём локальную snapshot-row.
func (s *SkifService) CreateUserInCompany(conn *models.SkifConnection, skifCompanyID, skifCompanyName string, req CreateSkifUserRequest) (string, map[string]interface{}, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Password) == "" {
		return "", nil, fmt.Errorf("name и password обязательны")
	}

	client, jar := s.httpClient(conn)
	defer s.saveSessionFromJar(conn, jar)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return "", nil, err
		}
	}
	if err := s.switchWithRetry(conn, client, skifCompanyID); err != nil {
		return "", nil, fmt.Errorf("switch company: %w", err)
	}

	roleKey := strings.ToUpper(strings.TrimSpace(req.RoleKey))
	if roleKey == "" {
		roleKey = "EDITOR"
	}
	roleID, roleValue, err := s.fetchRoleIDByKey(conn, client, roleKey)
	if err != nil {
		return "", nil, fmt.Errorf("resolve role: %w", err)
	}

	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = "ru"
	}

	body := map[string]interface{}{
		"role":     map[string]string{"key": roleKey, "value": roleValue, "id": roleID},
		"name":     req.Name,
		"password": req.Password,
		"language": map[string]string{"key": lang},
		"is_driver": req.IsDriver,
	}
	if req.Email != "" {
		body["email"] = req.Email
	}
	if req.Phone != "" {
		body["phone"] = req.Phone
	}
	if req.Code != "" {
		body["code"] = req.Code
	}
	if req.Details != "" {
		body["details"] = req.Details
	}

	jsonBody, _ := json.Marshal(body)
	req2, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/users", bytes.NewReader(jsonBody))
	if err != nil {
		return "", nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Accept", "application/json")
	resp, err := client.Do(req2)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", nil, fmt.Errorf("POST /users status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 300))
	}

	var result map[string]interface{}
	_ = json.Unmarshal(rawBody, &result)

	skifUserID := ""
	if v, ok := result["id"].(string); ok {
		skifUserID = v
	}

	// Локальный upsert (чтобы юзер сразу появился в /unified/users без полного re-sync)
	if skifUserID != "" {
		now := time.Now()
		row := models.SkifUser{
			ConnectionID:    conn.ID,
			SkifUserID:      skifUserID,
			SkifCompanyID:   skifCompanyID,
			Name:            req.Name,
			Email:           req.Email,
			Phone:           req.Phone,
			RoleKey:         roleKey,
			RoleName:        roleValue,
			IsActive:        true,
			IsDriver:        req.IsDriver,
			Code:            req.Code,
			Language:        lang,
			Details:         req.Details,
			CompanyID:       conn.CompanyID,
			SkifCompany:     skifCompanyName,
			LastCollectedAt: now,
			SkifCreatedAt:   &now,
		}
		s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_user_id"}, {Name: "skif_company_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "email", "phone", "role_key", "role_name", "is_active", "is_driver",
				"code", "language", "details", "company_id", "skif_company",
				"last_collected_at", "skif_deleted_at", "updated_at",
			}),
		}).Create(&row)
	}

	return skifUserID, result, nil
}

// DeleteUserFromCompany — снять юзера SKIF с конкретной компании.
// DELETE /api_v1/users/:userId/companies, body {companies: [{id: skifCompanyID}]}.
//
// Полное удаление юзера (когда он остаётся без компаний) делается через
// schedule_delete и здесь не реализовано — пусть админ сделает в SKIF UI.
func (s *SkifService) DeleteUserFromCompany(conn *models.SkifConnection, skifCompanyID, skifUserID string) error {
	client, jar := s.httpClient(conn)
	defer s.saveSessionFromJar(conn, jar)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return err
		}
	}

	if err := s.switchWithRetry(conn, client, skifCompanyID); err != nil {
		return fmt.Errorf("switch company: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"companies": []map[string]string{{"id": skifCompanyID}},
	})
	req, err := http.NewRequest("DELETE", strings.TrimRight(conn.BaseURL, "/")+"/api_v1/users/"+skifUserID+"/companies", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("DELETE /users/%s/companies status=%d body=%s", skifUserID, resp.StatusCode, truncate(string(rawBody), 300))
	}

	// Mark deleted локально (одна row на (user, company))
	now := time.Now()
	s.db.Model(&models.SkifUser{}).
		Where("connection_id = ? AND skif_user_id = ? AND skif_company_id = ?", conn.ID, skifUserID, skifCompanyID).
		Updates(map[string]interface{}{"skif_deleted_at": now, "updated_at": now})

	return nil
}

// updateRow — одна запись из POST /api_v1/company/updates/query.
type skifUpdateRow struct {
	Created   string `json:"created"`   // "2020-11-11 15:55:33" (Asia/Moscow по факту, но для агрегации не критично)
	Objects   string `json:"objects"`   // "units" / "company" / ...
	Operation string `json:"operation"` // POST / PUT / DELETE
	ObjectID  string `json:"object_id"`
}

type skifUpdatesQueryResp struct {
	List    []skifUpdateRow `json:"list"`
	MaxRows int             `json:"max_rows"`
}

// BackfillHistory — синхронный backfill истории units через
// POST /api_v1/company/updates/query foreach company.
//
// Параметры:
//   - from/to: окно (макс 1 год по SKIF-API, разбивает автоматически)
//
// Логика:
//  1. login (если нет cookie)
//  2. GET /me → companies[]
//  3. foreach company:
//     - switch
//     - POST /updates/query {objects: "units", from, to, sortField: "created", first_row: page*1000, max_rows: 1000}
//     - paginate пока List < max_rows и first_row < limit
//     - в памяти агрегируем per (company_id, day, operation) → counts
//  4. upsert в skif_daily_snapshots
//
// Возвращает: total events processed, snapshots upserted, error.
func (s *SkifService) BackfillHistory(conn *models.SkifConnection, from, to time.Time) (int, int, error) {
	const pageSize = 1000
	const skifMaxPeriod = 365 * 24 * time.Hour // SKIF docs: запрос ограничен 1 годом

	now := time.Now()
	client, jar := s.httpClient(conn)
	defer s.saveSessionFromJar(conn, jar)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, 0, err
		}
	}

	me, status, err := s.fetchMe(conn, client)
	if err != nil || status != 200 {
		if status == 401 || status == 422 {
			if err := s.loginWithClient(conn, client); err != nil {
				return 0, 0, err
			}
			me, status, err = s.fetchMe(conn, client)
		}
		if err != nil || status != 200 {
			return 0, 0, fmt.Errorf("/me status=%d err=%v", status, err)
		}
	}
	companies := me.Companies
	if len(companies) == 0 {
		companies = []skifCompanyBrief{me.ActiveCompany}
	}

	// Aggregator: (companyID, day) → {created, deleted, updated}
	type aggKey struct {
		CompanyID string
		Day       string // YYYY-MM-DD
	}
	type counts struct{ Created, Deleted, Updated int }
	agg := make(map[aggKey]*counts, 4096)

	totalEvents := 0
	// Throttle 2.2s между компаниями: 2 запроса (switch+query) на компанию ≤ ~1 req/sec под лимит SKIF 60/min.
	const throttle = 2200 * time.Millisecond

	// Разбиваем большое окно на куски по году (SKIF limit).
	type window struct{ from, to time.Time }
	var windows []window
	cursor := from
	for cursor.Before(to) {
		end := cursor.Add(skifMaxPeriod)
		if end.After(to) {
			end = to
		}
		windows = append(windows, window{cursor, end})
		cursor = end
	}

	// processCompany — обрабатывает одну компанию, возвращает true при успехе.
	processCompany := func(comp skifCompanyBrief) bool {
		if err := s.switchWithRetry(conn, client, comp.ID); err != nil {
			log.Printf("⚠️ SKIF backfill switch %s: %v", comp.Name, err)
			return false
		}
		for _, w := range windows {
			firstRow := 0
			reloggedIn := false
			for {
				body, _ := json.Marshal(map[string]interface{}{
					"objects":   "units",
					"from":      w.from.Format("2006-01-02 15:04:05"),
					"to":        w.to.Format("2006-01-02 15:04:05"),
					"first_row": firstRow,
					"max_rows":  pageSize,
					"sortField": "created",
					"sortDesc":  false,
				})
				rawBody, code, err := s.postJSON(conn, client, "/api_v1/company/updates/query", body)
				if err != nil {
					log.Printf("⚠️ SKIF backfill /updates/query company=%s: %v", comp.Name, err)
					return false
				}
				if code == 429 {
					time.Sleep(6 * time.Second)
					continue
				}
				if code == 401 && !reloggedIn {
					log.Printf("🔑 SKIF backfill 401 на /updates/query company=%s, re-login", comp.Name)
					if relErr := s.loginWithClient(conn, client); relErr != nil {
						log.Printf("⚠️ SKIF backfill re-login fail: %v", relErr)
						return false
					}
					if swErr := s.switchCompany(conn, client, comp.ID); swErr != nil {
						log.Printf("⚠️ SKIF backfill re-switch %s: %v", comp.Name, swErr)
						return false
					}
					reloggedIn = true
					continue
				}
				if code != 200 {
					log.Printf("⚠️ SKIF backfill /updates/query company=%s status=%d body=%s", comp.Name, code, truncate(string(rawBody), 200))
					return false
				}
				var resp skifUpdatesQueryResp
				if err := json.Unmarshal(rawBody, &resp); err != nil {
					log.Printf("⚠️ SKIF backfill parse company=%s: %v", comp.Name, err)
					return false
				}
				if len(resp.List) == 0 {
					break
				}
				for _, ev := range resp.List {
					if ev.Objects != "units" {
						continue
					}
					day := ev.Created
					if len(day) >= 10 {
						day = day[:10]
					}
					k := aggKey{CompanyID: comp.ID, Day: day}
					c, ok := agg[k]
					if !ok {
						c = &counts{}
						agg[k] = c
					}
					switch strings.ToUpper(ev.Operation) {
					case "POST":
						c.Created++
					case "DELETE":
						c.Deleted++
					case "PUT":
						c.Updated++
					}
					totalEvents++
				}
				firstRow += pageSize
				if len(resp.List) < pageSize {
					break
				}
				if firstRow >= 50000 { // hard cap per company per window
					break
				}
				time.Sleep(throttle)
			}
		}
		return true
	}

	// Pass 1: основной проход по всем компаниям.
	var failed []skifCompanyBrief
	for ci, comp := range companies {
		if ci > 0 {
			time.Sleep(throttle)
		}
		if !processCompany(comp) {
			failed = append(failed, comp)
		}
	}

	// Pass 2: retry для упавших компаний с увеличенной паузой 30s между ними.
	var stillFailed []string
	if len(failed) > 0 {
		log.Printf("🔄 SKIF backfill retry pass: %d failed companies", len(failed))
		time.Sleep(30 * time.Second)
		for ri, comp := range failed {
			if ri > 0 {
				time.Sleep(30 * time.Second)
			}
			if !processCompany(comp) {
				stillFailed = append(stillFailed, comp.Name)
			}
		}
	}

	// Upsert агрегатов в БД.
	upserted := 0
	for k, c := range agg {
		day, err := time.Parse("2006-01-02", k.Day)
		if err != nil {
			continue
		}
		row := models.SkifDailySnapshot{
			ConnectionID:  conn.ID,
			SkifCompanyID: k.CompanyID,
			SnapshotDate:  day,
			UnitsCreated:  c.Created,
			UnitsDeleted:  c.Deleted,
			UnitsUpdated:  c.Updated,
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_company_id"}, {Name: "snapshot_date"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"units_created", "units_deleted", "units_updated", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			log.Printf("⚠️ SKIF snapshot upsert: %v", err)
			continue
		}
		upserted++
	}

	skippedSummary := ""
	if len(stillFailed) > 0 {
		skippedSummary = fmt.Sprintf(" skipped=%d (%s)", len(stillFailed), strings.Join(stillFailed, ", "))
	}
	log.Printf("✅ SKIF backfill conn=%d companies=%d windows=%d events=%d upserted=%d за %v%s",
		conn.ID, len(companies), len(windows), totalEvents, upserted, time.Since(now).Round(time.Second), skippedSummary)
	return totalEvents, upserted, nil
}

// postJSON — вспомогательный метод для POST с JSON body.
func (s *SkifService) postJSON(conn *models.SkifConnection, client *http.Client, path string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequest("POST", strings.TrimRight(conn.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	return rawBody, resp.StatusCode, nil
}

// saveSessionFromJar — забирает текущие cookie из jar и сохраняет в conn.SessionCookie.
func (s *SkifService) saveSessionFromJar(conn *models.SkifConnection, jar *cookiejar.Jar) {
	u, err := url.Parse(conn.BaseURL)
	if err != nil {
		return
	}
	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		return
	}
	cookieStr := serializeCookies(cookies)
	now := time.Now()
	s.db.Model(conn).Updates(map[string]interface{}{
		"session_cookie": cookieStr,
		"last_login_at":  &now,
	})
	conn.SessionCookie = cookieStr
	conn.LastLoginAt = &now
}

// SkifCreateCompanyParams — параметры для POST /api_v1/company/create.
//
// Без поля User создаётся только компания. С User — компания + первый admin-пользователь
// (см. wiki/sources/skif-api/kompaniya.md).
type SkifCreateCompanyParams struct {
	CompanyName string                  `json:"company_name"`
	Timezone    string                  `json:"timezone"`
	DateFormat  string                  `json:"dateformat,omitempty"`
	TimeFormat  string                  `json:"timeformat,omitempty"`
	User        *SkifCreateCompanyUser  `json:"-"` // флатим в body вручную ниже
}

// SkifCreateCompanyUser — опциональные поля для создания первого пользователя компании.
type SkifCreateCompanyUser struct {
	Email    string `json:"userProviderId"`
	Type     string `json:"type"` // "EMAIL"
	Password string `json:"password"`
	Name     string `json:"name"`
}

// adminBaseURL возвращает admin-host SKIF на основе app-host из conn.BaseURL.
// app.skif.pro → admin.skif.pro. Если хост уже admin — возвращает его.
func adminBaseURL(conn *models.SkifConnection) string {
	base := strings.TrimRight(conn.BaseURL, "/")
	// Замена app.skif.pro → admin.skif.pro
	if strings.Contains(base, "://app.skif.pro") {
		return strings.Replace(base, "://app.skif.pro", "://admin.skif.pro", 1)
	}
	if strings.Contains(base, "://admin.") {
		return base
	}
	// Fallback: попробуем заменить app. → admin.
	return strings.Replace(base, "://app.", "://admin.", 1)
}

// skifTimezoneMap — IANA → SKIF timezone-объект. SKIF использует свои ключи "UTC±N".
// value — человекочитаемое описание (берётся из SKIF /api_v1/dictionaries).
var skifTimezoneMap = map[string]struct {
	Key   string
	Value string
}{
	"Europe/Moscow":      {"UTC+3", "(GMT+03:00) Москва, Санкт-Петербург, Волгоград"},
	"Europe/Kaliningrad": {"UTC+2", "(GMT+02:00) Хельсинки, Киев, Рига, Стамбул, Минск"},
	"Europe/Kiev":        {"UTC+2", "(GMT+02:00) Хельсинки, Киев, Рига, Стамбул, Минск"},
	"Europe/Samara":      {"UTC+4", "(GMT+04:00) Баку, Тбилиси, Ереван"},
	"Asia/Yekaterinburg": {"UTC+5", "(GMT+05:00) Екатеринбург, Астана, Алматы, Ташкент"},
	"Asia/Almaty":        {"UTC+5", "(GMT+05:00) Екатеринбург, Астана, Алматы, Ташкент"},
	"Asia/Omsk":          {"UTC+6", "(GMT+06:00) Республика Саха (Якутия)"},
	"Asia/Krasnoyarsk":   {"UTC+7", "(GMT+07:00) Новосибирск, Красноярск"},
	"Asia/Irkutsk":       {"UTC+8", "(GMT+08:00) Иркутск, Улан-Батор"},
	"Asia/Yakutsk":       {"UTC+9", "(GMT+09:00) Якутск, Осака, Саппоро, Токио, Сеул"},
	"Asia/Vladivostok":   {"UTC+10", "(GMT+10:00) Владивосток"},
	"UTC":                {"UTC", "(GMT) Дублин, Эдинбург, Лиссабон, Лондон"},
}

// resolveSkifTimezone преобразует IANA-id или SKIF-key в SKIF timezone-объект.
// Принимает "Europe/Moscow", "UTC+3", "(GMT+03:00) ..." — возвращает {key, value}.
func resolveSkifTimezone(tz string) (key, value string, ok bool) {
	if v, found := skifTimezoneMap[tz]; found {
		return v.Key, v.Value, true
	}
	// Если уже SKIF-key (UTC+3) — возвращаем как есть с пустым value (SKIF проставит).
	if strings.HasPrefix(tz, "UTC") {
		return tz, "", true
	}
	return "", "", false
}

// adminLogin делает POST admin/api_v1/login и возвращает PLAY_SESSION cookie-string.
// Отличается от обычного Login: provider_key="EMAIL", is_admin_panel=true, host=admin.skif.pro.
func (s *SkifService) adminLogin(conn *models.SkifConnection) (string, error) {
	plainPwd, err := s.plainPassword(conn)
	if err != nil {
		return "", fmt.Errorf("decrypt password: %w", err)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"userProviderId":  conn.Login,
		"provider_key":    "EMAIL",
		"password":        plainPwd,
		"is_admin_panel":  true,
		"timezone_key":    "UTC+3",
	})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	url := adminBaseURL(conn) + "/api_v1/login"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("admin login build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("admin login http: %w", err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("admin login status=%d body=%s", resp.StatusCode, truncate(string(rawBody), 300))
	}
	u, _ := neturl(adminBaseURL(conn))
	cookies := jar.Cookies(u)
	if len(cookies) == 0 {
		return "", fmt.Errorf("admin login: no cookie set")
	}
	return serializeCookies(cookies), nil
}

// neturl wraps url.Parse для вызова после строкового хоста — отдельная функция чтобы не путаться с переменной url.
func neturl(s string) (*url.URL, error) {
	return url.Parse(s)
}

// adminPOST делает POST на admin-host с указанным cookie-string, возвращает raw body + status.
func (s *SkifService) adminPOST(conn *models.SkifConnection, cookieStr, path string, body []byte) ([]byte, int, error) {
	u, _ := neturl(adminBaseURL(conn) + path)
	jar, _ := cookiejar.New(nil)
	if cookieStr != "" {
		base, _ := neturl(adminBaseURL(conn))
		jar.SetCookies(base, parseCookieString(cookieStr, base))
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	return rawBody, resp.StatusCode, nil
}

// resolveDealerForLogin находит dealer'а по email == conn.Login через POST /api_v1/dealers_admin_query.
// SKIF возвращает list дилеров; нам нужен root-level (без parent_id) с совпадающим email.
func (s *SkifService) resolveDealerForLogin(conn *models.SkifConnection, cookieStr string) (map[string]interface{}, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"from":         0,
		"count":        100,
		"value":        "",
		"timezone_key": "UTC+3",
	})
	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/dealers_admin_query", body)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("dealers_admin_query status=%d body=%s", status, truncate(string(rawBody), 300))
	}
	var resp struct {
		List []map[string]interface{} `json:"list"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return nil, fmt.Errorf("dealers parse: %w", err)
	}
	// Сначала ищем root-dealer (без parent_id) с совпадающим email.
	for _, d := range resp.List {
		if email, _ := d["email"].(string); strings.EqualFold(email, conn.Login) {
			if _, hasParent := d["parent_id"]; !hasParent {
				return d, nil
			}
		}
	}
	// Fallback: любой dealer с совпадающим email.
	for _, d := range resp.List {
		if email, _ := d["email"].(string); strings.EqualFold(email, conn.Login) {
			return d, nil
		}
	}
	return nil, fmt.Errorf("dealer для login=%s не найден среди %d", conn.Login, len(resp.List))
}

// CreateCompany создаёт новую компанию-клиента в SKIF через admin API.
//
// Ходит на admin.skif.pro: login → dealers_admin_query (резолв dealer по email) → company/create.
// Не использует app.skif.pro session (conn.SessionCookie не задействован).
//
// Возвращает body ответа SKIF (включая id новой компании).
//
// Примечание: создание admin-юзера вместе с компанией (params.User) пока не реализовано —
// admin-API skif требует другую схему чем app-API. См. wiki/sources/skif-api/kompaniya.md.
func (s *SkifService) CreateCompany(conn *models.SkifConnection, params SkifCreateCompanyParams) (map[string]interface{}, error) {
	if params.CompanyName == "" {
		return nil, fmt.Errorf("company_name обязателен")
	}
	if params.Timezone == "" {
		return nil, fmt.Errorf("timezone обязателен")
	}

	tzKey, tzValue, ok := resolveSkifTimezone(params.Timezone)
	if !ok {
		return nil, fmt.Errorf("неизвестный timezone %q (поддерживаются IANA Europe/Moscow и т.п. либо UTC+N)", params.Timezone)
	}

	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("admin login: %v", err))
		return nil, fmt.Errorf("admin login: %w", err)
	}

	dealer, err := s.resolveDealerForLogin(conn, cookieStr)
	if err != nil {
		return nil, fmt.Errorf("resolve dealer: %w", err)
	}

	// Чистим dealer от лишних полей (оставляем что нужно SKIF /company/create).
	cleanDealer := map[string]interface{}{
		"id":                dealer["id"],
		"name":              dealer["name"],
		"blocked":           dealer["blocked"],
		"skif_support_type": dealer["skif_support_type"],
		"is_default":        dealer["is_default"],
	}

	body := map[string]interface{}{
		"company_name": params.CompanyName,
		"imeis":        []string{""},
		"timezone": map[string]interface{}{
			"key":   tzKey,
			"type":  "timezones",
			"value": tzValue,
		},
		"timezone_key":     tzKey,
		"dealer":           cleanDealer,
		"notify_users_ids": []interface{}{},
		"support_info":     nil,
		"details":          nil,
		"properties": map[string]interface{}{
			"available_maps": []map[string]interface{}{
				{"os": "web", "maps_keys": []string{"yandex", "google_satellite", "google_road", "google_hybrid", "google_traffic", "google_terrain", "osm_scheme", "here", "bing", "bing_satellite"}},
				{"os": "ios", "maps_keys": []string{"yandex", "google_satellite", "google_road", "google_hybrid", "osm_scheme", "bing_satellite"}},
				{"os": "android", "maps_keys": []string{"yandex", "google_satellite", "google_road", "google_hybrid", "osm_scheme", "bing", "bing_satellite", "usgs_topo", "usgs_sat", "wikimedia", "open_topo"}},
			},
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/company/create", bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("/company/create http: %w", err)
	}
	if status != 200 && status != 201 {
		return nil, fmt.Errorf("/company/create status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		log.Printf("⚠️ SKIF create company: parse response: %v body=%s", err, truncate(string(rawBody), 200))
		return map[string]interface{}{"raw": string(rawBody)}, nil
	}
	log.Printf("✅ SKIF создана компания (admin): conn=%d name=%q dealer=%v id=%v", conn.ID, params.CompanyName, dealer["name"], resp["id"])
	return resp, nil
}

// DeleteCompany планирует удаление SKIF-компании через admin API.
//
// SKIF не удаляет сразу — schedule_delete ставит задачу на удаление с возможностью
// отмены через POST /company/cancel_delete. По умолчанию задача исполняется через 14 дней.
//
// Принимает skifCompanyID (UUID компании в SKIF — поле SkifUnit.SkifCompanyID).
//
// Особенности admin endpoint:
//   - метод DELETE
//   - body {"ids":["<uuid>"],"timezone_key":"UTC+3"} — массив, но **только один id за запрос**
//   - host admin.skif.pro, отдельная PLAY_SESSION
func (s *SkifService) DeleteCompany(conn *models.SkifConnection, skifCompanyID string) error {
	if skifCompanyID == "" {
		return fmt.Errorf("skif_company_id обязателен")
	}

	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("admin login: %v", err))
		return fmt.Errorf("admin login: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"ids":          []string{skifCompanyID},
		"timezone_key": "UTC+3",
	})

	rawBody, status, err := s.adminDELETE(conn, cookieStr, "/api_v1/company/schedule_delete", body)
	if err != nil {
		return fmt.Errorf("/company/schedule_delete http: %w", err)
	}
	if status != 200 && status != 204 {
		return fmt.Errorf("/company/schedule_delete status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	// Локально фиксируем pending-delete для UI countdown.
	// SKIF удаляет через 14 дней — до этого момента можно отменить через cancel_delete.
	now := time.Now()
	scheduledFor := now.Add(14 * 24 * time.Hour)
	var companyName string
	var nameRow struct{ Name string }
	if err := s.db.Raw("SELECT MAX(skif_company) AS name FROM skif_units WHERE connection_id = ? AND skif_company_id = ?", conn.ID, skifCompanyID).Scan(&nameRow).Error; err == nil {
		companyName = nameRow.Name
	}
	pending := models.SkifPendingDelete{
		ConnectionID:  conn.ID,
		SkifCompanyID: skifCompanyID,
		CompanyName:   companyName,
		ScheduledAt:   now,
		ScheduledFor:  scheduledFor,
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_company_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"company_name":  companyName,
			"scheduled_at":  now,
			"scheduled_for": scheduledFor,
			"updated_at":    now,
		}),
	}).Create(&pending).Error; err != nil {
		log.Printf("⚠️ SKIF DeleteCompany: failed to upsert pending_delete: %v", err)
	}

	log.Printf("✅ SKIF schedule_delete OK: conn=%d company=%s scheduled_for=%s", conn.ID, skifCompanyID, scheduledFor.Format(time.RFC3339))
	return nil
}

// adminDELETE — DELETE на admin-host с указанным cookie-string.
// Параллель к adminPOST, но DELETE с body (SKIF поддерживает body для DELETE).
func (s *SkifService) adminDELETE(conn *models.SkifConnection, cookieStr, path string, body []byte) ([]byte, int, error) {
	u, _ := neturl(adminBaseURL(conn) + path)
	jar, _ := cookiejar.New(nil)
	if cookieStr != "" {
		base, _ := neturl(adminBaseURL(conn))
		jar.SetCookies(base, parseCookieString(cookieStr, base))
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	req, err := http.NewRequest("DELETE", u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	return rawBody, resp.StatusCode, nil
}

// BackfillObjectCreated тянет POST events из /company/updates/query для указанных
// objectTypes и сохраняет точные даты создания в skif_object_created.
//
// SKIF не отдаёт `created` в response объектов, поэтому это единственный способ
// получить дату создания (через operation log). Окно макс 1 год — для старых
// сущностей нужен повторный запуск с разными from/to.
//
// objectTypes: ["units", "company", "users", "geozones", "unitsgroup", ...]
// Возвращает: total events, total upserted, ошибка.
func (s *SkifService) BackfillObjectCreated(conn *models.SkifConnection, objectTypes []string, from, to time.Time) (int, int, error) {
	const pageSize = 1000
	const throttle = 2200 * time.Millisecond
	const skifMaxPeriod = 365 * 24 * time.Hour

	if len(objectTypes) == 0 {
		objectTypes = []string{"units", "company", "users"}
	}
	if to.Sub(from) > skifMaxPeriod {
		return 0, 0, fmt.Errorf("окно > 1 года не поддерживается, разбейте на куски")
	}

	now := time.Now()
	client, jar := s.httpClient(conn)
	defer s.saveSessionFromJar(conn, jar)

	if conn.SessionCookie == "" {
		if err := s.loginWithClient(conn, client); err != nil {
			return 0, 0, err
		}
	}

	me, status, err := s.fetchMe(conn, client)
	if err != nil || status != 200 {
		if status == 401 || status == 422 {
			if err := s.loginWithClient(conn, client); err != nil {
				return 0, 0, err
			}
			me, status, err = s.fetchMe(conn, client)
		}
		if err != nil || status != 200 {
			return 0, 0, fmt.Errorf("/me status=%d err=%v", status, err)
		}
	}
	companies := me.Companies
	if len(companies) == 0 {
		companies = []skifCompanyBrief{me.ActiveCompany}
	}

	totalEvents := 0
	totalUpserted := 0

	processOne := func(comp skifCompanyBrief, objType string) error {
		if err := s.switchWithRetry(conn, client, comp.ID); err != nil {
			return fmt.Errorf("switch: %w", err)
		}
		firstRow := 0
		reloggedIn := false
		for {
			body, _ := json.Marshal(map[string]interface{}{
				"objects":   objType,
				"from":      from.Format("2006-01-02 15:04:05"),
				"to":        to.Format("2006-01-02 15:04:05"),
				"first_row": firstRow,
				"max_rows":  pageSize,
				"sortField": "created",
				"sortDesc":  false,
			})
			rawBody, code, err := s.postJSON(conn, client, "/api_v1/company/updates/query", body)
			if err != nil {
				return err
			}
			if code == 429 {
				time.Sleep(6 * time.Second)
				continue
			}
			if code == 401 && !reloggedIn {
				if relErr := s.loginWithClient(conn, client); relErr != nil {
					return relErr
				}
				if swErr := s.switchCompany(conn, client, comp.ID); swErr != nil {
					return swErr
				}
				reloggedIn = true
				continue
			}
			if code != 200 {
				return fmt.Errorf("status=%d body=%s", code, truncate(string(rawBody), 200))
			}
			var resp skifUpdatesQueryResp
			if err := json.Unmarshal(rawBody, &resp); err != nil {
				return err
			}
			if len(resp.List) == 0 {
				return nil
			}
			for _, ev := range resp.List {
				if ev.Objects != objType {
					continue
				}
				if !strings.EqualFold(ev.Operation, "POST") {
					continue
				}
				totalEvents++
				createdAt, perr := time.Parse("2006-01-02 15:04:05", ev.Created)
				if perr != nil {
					createdAt, perr = time.Parse(time.RFC3339, ev.Created)
				}
				if perr != nil || createdAt.IsZero() {
					continue
				}
				row := models.SkifObjectCreated{
					ConnectionID:     conn.ID,
					ObjectType:       objType,
					ObjectID:         ev.ObjectID,
					CompanyContextID: comp.ID,
					CreatedSkifAt:    createdAt,
				}
				if err := s.db.Clauses(clause.OnConflict{
					Columns: []clause.Column{{Name: "connection_id"}, {Name: "object_type"}, {Name: "object_id"}},
					DoUpdates: clause.Assignments(map[string]interface{}{
						"company_context_id": comp.ID,
						"created_skif_at":    createdAt,
						"updated_at":         time.Now(),
					}),
				}).Create(&row).Error; err != nil {
					log.Printf("⚠️ SKIF skif_object_created upsert %s/%s: %v", objType, ev.ObjectID, err)
					continue
				}
				totalUpserted++
			}
			firstRow += pageSize
			if len(resp.List) < pageSize {
				return nil
			}
			if firstRow >= 50000 {
				return nil
			}
			time.Sleep(throttle)
		}
	}

	for ci, comp := range companies {
		if ci > 0 {
			time.Sleep(throttle)
		}
		for _, ot := range objectTypes {
			if err := processOne(comp, ot); err != nil {
				log.Printf("⚠️ SKIF backfill_created company=%s type=%s: %v", comp.Name, ot, err)
			}
			time.Sleep(throttle)
		}
	}

	log.Printf("✅ SKIF backfill_created conn=%d types=%v companies=%d events=%d upserted=%d за %v",
		conn.ID, objectTypes, len(companies), totalEvents, totalUpserted, time.Since(now).Round(time.Second))
	return totalEvents, totalUpserted, nil
}

// SkifRegisterSubdealerParams — параметры регистрации субинтегратора через
// POST app/api_v1/registrate_dealer.
type SkifRegisterSubdealerParams struct {
	TypeKey       string `json:"type_key"`       // legal_entity | individual_entrepreneur
	Name          string `json:"name"`
	Email         string `json:"email"`
	INN           string `json:"inn"`
	Phone         string `json:"phone"`
	ContactPerson string `json:"contact_person"`
	Password      string `json:"password"`
	Address       string `json:"address"`
}

// RegisterSubdealer регистрирует нового субинтегратора через app-API.
//
// Использует app-host session (s.Login обычный, не admin). Создаёт сразу
// и dealer entity, и auto-company с тем же именем (см. wiki/sources/skif-api/integratory.md).
//
// Возвращает {id, name, link} response от SKIF + ошибку.
func (s *SkifService) RegisterSubdealer(conn *models.SkifConnection, params SkifRegisterSubdealerParams) (map[string]interface{}, error) {
	if params.Name == "" || params.Email == "" || params.INN == "" || params.Phone == "" || params.ContactPerson == "" || params.Password == "" || params.Address == "" {
		return nil, fmt.Errorf("name/email/inn/phone/contact_person/password/address обязательны")
	}
	if params.TypeKey == "" {
		params.TypeKey = "legal_entity"
	}
	if params.TypeKey != "legal_entity" && params.TypeKey != "individual_entrepreneur" {
		return nil, fmt.Errorf("type_key должен быть legal_entity или individual_entrepreneur")
	}

	body, _ := json.Marshal(map[string]interface{}{
		"type":           map[string]string{"key": params.TypeKey},
		"name":           params.Name,
		"email":          params.Email,
		"inn":            params.INN,
		"phone":          params.Phone,
		"contact_person": params.ContactPerson,
		"password":       params.Password,
		"address":        params.Address,
	})

	rawBody, status, err := s.authedRequest(conn, "POST", "/api_v1/registrate_dealer", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("/registrate_dealer http: %w", err)
	}
	if status != 200 && status != 201 {
		return nil, fmt.Errorf("/registrate_dealer status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		log.Printf("⚠️ SKIF RegisterSubdealer: parse response: %v body=%s", err, truncate(string(rawBody), 200))
		return map[string]interface{}{"raw": string(rawBody)}, nil
	}
	log.Printf("✅ SKIF subdealer создан: conn=%d name=%q id=%v", conn.ID, params.Name, resp["id"])

	// Async sync чтобы новый dealer попал в локальный реестр.
	go func() {
		if _, err := s.SyncSubdealers(conn); err != nil {
			log.Printf("⚠️ SKIF post-register subdealers sync: %v", err)
		}
	}()
	return resp, nil
}

// SyncSubdealers тянет список dealers через admin dealers_admin_query
// и upsert'ит subdealers (только тех у кого parent_id != "") в локальную таблицу.
//
// Root-dealer (текущий интегратор без parent_id) — игнорируется.
func (s *SkifService) SyncSubdealers(conn *models.SkifConnection) (int, error) {
	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		return 0, fmt.Errorf("admin login: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"from":         0,
		"count":        500,
		"value":        "",
		"timezone_key": "UTC+3",
	})

	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/dealers_admin_query", body)
	if err != nil {
		return 0, fmt.Errorf("dealers_admin_query http: %w", err)
	}
	if status != 200 {
		return 0, fmt.Errorf("dealers_admin_query status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	var resp struct {
		List []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			INN           string `json:"inn"`
			Phone         string `json:"phone"`
			Email         string `json:"email"`
			ContactPerson string `json:"contact_person"`
			Address       string `json:"address"`
			Units         int    `json:"units"`
			ParentID      string `json:"parent_id"`
			ParentName    string `json:"parent_name"`
			Blocked       bool   `json:"blocked"`
			IsDefault     bool   `json:"is_default"`
			Type          struct {
				Key string `json:"key"`
			} `json:"type"`
			SkifSupportType struct {
				Key string `json:"key"`
			} `json:"skif_support_type"`
		} `json:"list"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return 0, fmt.Errorf("dealers parse: %w", err)
	}

	now := time.Now()
	upserted := 0
	for _, d := range resp.List {
		if d.ID == "" || d.ParentID == "" {
			continue // root-dealer без parent_id — пропускаем
		}
		row := models.SkifDealer{
			ConnectionID:       conn.ID,
			SkifDealerID:       d.ID,
			Name:               d.Name,
			Type:               d.Type.Key,
			INN:                d.INN,
			Phone:              d.Phone,
			Email:              d.Email,
			ContactPerson:      d.ContactPerson,
			Address:            d.Address,
			ParentID:           d.ParentID,
			ParentName:         d.ParentName,
			UnitsCount:         d.Units,
			Blocked:            d.Blocked,
			IsDefault:          d.IsDefault,
			SkifSupportTypeKey: d.SkifSupportType.Key,
			LastSyncedAt:       now,
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_dealer_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"name":                  d.Name,
				"type":                  d.Type.Key,
				"inn":                   d.INN,
				"phone":                 d.Phone,
				"email":                 d.Email,
				"contact_person":        d.ContactPerson,
				"address":               d.Address,
				"parent_id":             d.ParentID,
				"parent_name":           d.ParentName,
				"units_count":           d.Units,
				"blocked":               d.Blocked,
				"is_default":            d.IsDefault,
				"skif_support_type_key": d.SkifSupportType.Key,
				"last_synced_at":        now,
				"updated_at":            now,
			}),
		}).Create(&row).Error; err != nil {
			log.Printf("⚠️ SKIF SyncSubdealers upsert %s: %v", d.ID, err)
			continue
		}
		upserted++
	}
	log.Printf("🔁 SKIF SyncSubdealers: conn=%d upserted=%d (root и subdealers всего %d)", conn.ID, upserted, len(resp.List))
	return upserted, nil
}

// SyncCompanyStatuses тянет список компаний через admin auth_admin_query
// и обновляет локальную таблицу skif_company_statuses.
//
// Запрашивает поля id/name/users_count/dealer_id/billing/units. Игнорирует pending
// "blocked" companies — billing.company_status содержит "ACTIVE" / "BLOCKED" / "DELETING".
//
// Возвращает количество upsert'ов и ошибку.
func (s *SkifService) SyncCompanyStatuses(conn *models.SkifConnection) (int, error) {
	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		return 0, fmt.Errorf("admin login: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"model":        "companies",
		"fields":       []string{"id", "name", "timezone_key", "users_count", "creator", "amount", "tariff.units", "dealer_id", "billing", "units"},
		"from":         0,
		"count":        1000,
		"value":        "",
		"field":        "",
		"conditions":   []map[string]interface{}{},
		"companies":    []interface{}{},
		"timezone_key": "UTC+3",
	})

	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/auth_admin_query", body)
	if err != nil {
		return 0, fmt.Errorf("auth_admin_query http: %w", err)
	}
	if status != 200 {
		return 0, fmt.Errorf("auth_admin_query status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	var resp struct {
		List []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Units      int    `json:"units"`
			UsersCount int    `json:"users_count"`
			Billing    struct {
				CompanyStatus     string `json:"company_status"`
				TerminalBlockType string `json:"terminal_block_type"`
			} `json:"billing"`
		} `json:"list"`
	}
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return 0, fmt.Errorf("auth_admin_query parse: %w", err)
	}

	now := time.Now()
	upserted := 0
	for _, c := range resp.List {
		if c.ID == "" {
			continue
		}
		row := models.SkifCompanyStatus{
			ConnectionID:      conn.ID,
			SkifCompanyID:     c.ID,
			CompanyName:       c.Name,
			CompanyStatus:     c.Billing.CompanyStatus,
			TerminalBlockType: c.Billing.TerminalBlockType,
			UnitsCount:        c.Units,
			UsersCount:        c.UsersCount,
			LastSyncedAt:      now,
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_company_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"company_name":        c.Name,
				"company_status":      c.Billing.CompanyStatus,
				"terminal_block_type": c.Billing.TerminalBlockType,
				"units_count":         c.Units,
				"users_count":         c.UsersCount,
				"last_synced_at":      now,
				"updated_at":          now,
			}),
		}).Create(&row).Error; err != nil {
			log.Printf("⚠️ SKIF SyncCompanyStatuses upsert %s: %v", c.ID, err)
			continue
		}
		upserted++
	}
	log.Printf("🔁 SKIF SyncCompanyStatuses: conn=%d upserted=%d из %d", conn.ID, upserted, len(resp.List))
	return upserted, nil
}

// BlockCompany блокирует компанию через POST admin/api_v1/company/block.
//
// blockType: "terminals_not_block" (обычная — юзеры не входят, терминалы шлют) или
//            "terminals_block" (полная — терминалы тоже блокируются).
// pending=true для запланированной блокировки.
func (s *SkifService) BlockCompany(conn *models.SkifConnection, skifCompanyID, blockType string, pending bool) error {
	return s.toggleBlock(conn, skifCompanyID, blockType, true, pending)
}

// UnblockCompany снимает блокировку компании. Использует тот же endpoint /company/block.
func (s *SkifService) UnblockCompany(conn *models.SkifConnection, skifCompanyID string) error {
	return s.toggleBlock(conn, skifCompanyID, "terminals_not_block", false, false)
}

// toggleBlock — общая логика для block/unblock.
func (s *SkifService) toggleBlock(conn *models.SkifConnection, skifCompanyID, blockType string, block, pending bool) error {
	if skifCompanyID == "" {
		return fmt.Errorf("skif_company_id обязателен")
	}
	if blockType == "" {
		blockType = "terminals_not_block"
	}
	if blockType != "terminals_not_block" && blockType != "terminals_block" {
		return fmt.Errorf("block_type должен быть terminals_not_block или terminals_block")
	}

	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"companies":    []map[string]string{{"id": skifCompanyID}},
		"block_type":   blockType,
		"block":        block,
		"pending":      pending,
		"timezone_key": "UTC+3",
	})

	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/company/block", body)
	if err != nil {
		return fmt.Errorf("/company/block http: %w", err)
	}
	if status != 200 && status != 204 {
		return fmt.Errorf("/company/block status=%d body=%s", status, truncate(string(rawBody), 300))
	}
	action := "block"
	if !block {
		action = "unblock"
	}
	log.Printf("✅ SKIF %s OK: conn=%d company=%s type=%s pending=%v", action, conn.ID, skifCompanyID, blockType, pending)

	// Refresh статусов после действия чтобы UI получил актуальное состояние.
	go func() {
		if _, err := s.SyncCompanyStatuses(conn); err != nil {
			log.Printf("⚠️ SKIF post-%s status sync: %v", action, err)
		}
	}()
	return nil
}

// CancelDeleteCompany отменяет ранее запланированное удаление компании
// (POST /api_v1/company/cancel_delete). Полезно если пользователь передумал в течение 14 дней.
func (s *SkifService) CancelDeleteCompany(conn *models.SkifConnection, skifCompanyID string) error {
	if skifCompanyID == "" {
		return fmt.Errorf("skif_company_id обязателен")
	}

	cookieStr, err := s.adminLogin(conn)
	if err != nil {
		return fmt.Errorf("admin login: %w", err)
	}

	body, _ := json.Marshal(map[string]interface{}{
		"ids":          []string{skifCompanyID},
		"timezone_key": "UTC+3",
	})

	rawBody, status, err := s.adminPOST(conn, cookieStr, "/api_v1/company/cancel_delete", body)
	if err != nil {
		return fmt.Errorf("/company/cancel_delete http: %w", err)
	}
	if status != 200 && status != 204 {
		return fmt.Errorf("/company/cancel_delete status=%d body=%s", status, truncate(string(rawBody), 300))
	}

	// Удаляем локальную pending-запись.
	if err := s.db.Where("connection_id = ? AND skif_company_id = ?", conn.ID, skifCompanyID).
		Delete(&models.SkifPendingDelete{}).Error; err != nil {
		log.Printf("⚠️ SKIF CancelDeleteCompany: failed to delete pending_delete: %v", err)
	}

	log.Printf("✅ SKIF cancel_delete OK: conn=%d company=%s", conn.ID, skifCompanyID)
	return nil
}

func (s *SkifService) recordError(conn *models.SkifConnection, msg string) {
	now := time.Now()
	s.db.Model(conn).Updates(map[string]interface{}{
		"error_message": msg,
		"last_error_at": &now,
		"error_count":   gorm.Expr("error_count + 1"),
	})
	log.Printf("❌ SKIF conn=%d: %s", conn.ID, truncate(msg, 200))
}

// ----- helpers -----

func parseCookieString(s string, u *url.URL) []*http.Cookie {
	header := http.Header{}
	header.Add("Cookie", s)
	req := http.Request{Header: header}
	return req.Cookies()
}

func serializeCookies(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
	}
	return strings.Join(parts, "; ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// extractSkifCreatedAt — берёт дату создания юнита из массива states.
// Логика: первое состояние («Начальное состояние») с date_from != "1970-01-01..."
// = когда юнит реально появился в SKIF. Эпоху отфильтровываем.
func extractSkifCreatedAt(states []skifUnitState) *time.Time {
	for _, st := range states {
		if strings.HasPrefix(st.DateFrom, "1970-01-01") {
			continue
		}
		if t := parseSkifTime(st.DateFrom); t != nil {
			return t
		}
	}
	return nil
}

// parseSkifTime парсит SKIF timestamp "2024-03-14 10:00:00" в time.Time.
// Возвращает nil если parse fail или строка пустая.
func parseSkifTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}
