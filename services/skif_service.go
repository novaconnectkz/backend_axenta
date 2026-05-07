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

// SkifUnitDTO — минимальный набор полей юнита из /api_v1/units.
// Реальная схема в Postman: см. wiki/sources/skif-api/obekty.md.
type SkifUnitDTO struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	IMEI         string `json:"imei"`
	Phone        string `json:"phoneNumber"`
	Model        string `json:"model"`
	IsActive     bool   `json:"isActive"`
	CompanyName  string `json:"companyName"`
}

// SyncUnits загружает все объекты из SKIF и upsert'ит в skif_units.
// Возвращает количество upsert'нутых записей.
func (s *SkifService) SyncUnits(conn *models.SkifConnection) (int, error) {
	rawBody, status, err := s.authedRequest(conn, "GET", "/api_v1/units", nil)
	if err != nil {
		s.recordError(conn, fmt.Sprintf("sync units: %v", err))
		return 0, err
	}
	if status != 200 {
		err := fmt.Errorf("/units status=%d body=%s", status, truncate(string(rawBody), 300))
		s.recordError(conn, err.Error())
		return 0, err
	}

	// SKIF возвращает массив или объект с полем data — попробуем оба варианта.
	var units []SkifUnitDTO
	if err := json.Unmarshal(rawBody, &units); err != nil {
		var wrapper struct {
			Data []SkifUnitDTO `json:"data"`
		}
		if err2 := json.Unmarshal(rawBody, &wrapper); err2 == nil {
			units = wrapper.Data
		} else {
			s.recordError(conn, fmt.Sprintf("parse units: %v", err))
			return 0, fmt.Errorf("parse units: %w", err)
		}
	}

	now := time.Now()
	saved := 0
	for _, u := range units {
		row := models.SkifUnit{
			ConnectionID:    conn.ID,
			SkifUnitID:      u.ID,
			Name:            u.Name,
			IMEI:            u.IMEI,
			Phone:           u.Phone,
			Model:           u.Model,
			IsActive:        u.IsActive,
			CompanyID:       conn.CompanyID,
			SkifCompany:     u.CompanyName,
			LastCollectedAt: now,
		}
		if err := s.db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "connection_id"}, {Name: "skif_unit_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "imei", "phone", "model", "is_active",
				"company_id", "skif_company", "last_collected_at", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			log.Printf("⚠️ SKIF upsert unit %d: %v", u.ID, err)
			continue
		}
		saved++
	}

	// Обновляем счётчики в connection
	s.db.Model(conn).Updates(map[string]interface{}{
		"units_count":   saved,
		"last_sync_at":  &now,
		"error_message": "",
		"error_count":   0,
		"last_error_at": nil,
	})

	log.Printf("✅ SKIF sync units: conn=%d upserted=%d (raw=%d)", conn.ID, saved, len(units))
	return saved, nil
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
