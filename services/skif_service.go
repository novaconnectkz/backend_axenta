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
)

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
	body, _ := json.Marshal(map[string]string{
		"userProviderId": conn.Login,
		"provider_key":   "TEXT",
		"password":       conn.Password,
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
	body, _ := json.Marshal(map[string]string{
		"userProviderId": conn.Login,
		"provider_key":   "TEXT",
		"password":       conn.Password,
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
	body, _ := json.Marshal(map[string]interface{}{
		"userProviderId":  conn.Login,
		"provider_key":    "EMAIL",
		"password":        conn.Password,
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
