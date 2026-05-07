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

// switchWithRetry — switchCompany с retry на 429 rate limit.
// При 429 — sleep 6s, повтор 1 раз. Если опять 429 — fail.
func (s *SkifService) switchWithRetry(conn *models.SkifConnection, client *http.Client, companyID string) error {
	if err := s.switchCompany(conn, client, companyID); err != nil {
		if strings.Contains(err.Error(), "status=429") {
			time.Sleep(6 * time.Second)
			return s.switchCompany(conn, client, companyID)
		}
		return err
	}
	return nil
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
	const throttle = 1100 * time.Millisecond

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

	for ci, comp := range companies {
		if ci > 0 {
			time.Sleep(throttle)
		}
		if err := s.switchWithRetry(conn, client, comp.ID); err != nil {
			log.Printf("⚠️ SKIF backfill switch %s: %v", comp.Name, err)
			continue
		}

		for _, w := range windows {
			firstRow := 0
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
					break
				}
				if code == 429 {
					time.Sleep(6 * time.Second)
					continue
				}
				if code != 200 {
					log.Printf("⚠️ SKIF backfill /updates/query company=%s status=%d body=%s", comp.Name, code, truncate(string(rawBody), 200))
					break
				}
				var resp skifUpdatesQueryResp
				if err := json.Unmarshal(rawBody, &resp); err != nil {
					log.Printf("⚠️ SKIF backfill parse company=%s: %v", comp.Name, err)
					break
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

	log.Printf("✅ SKIF backfill conn=%d companies=%d windows=%d events=%d upserted=%d за %v",
		conn.ID, len(companies), len(windows), totalEvents, upserted, time.Since(now).Round(time.Second))
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
