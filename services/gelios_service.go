package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
	"backend_axenta/utils"
)

// GeliosAllowedBaseURL — GELIOS-инстанс один, base_url НЕ настраиваемый
// пользователем (SSRF-защита: иначе бэкенд ходил бы на произвольный host
// с кредами). Валидируется на create/update.
const GeliosAllowedBaseURL = "https://api.geliospro.com"

// GeliosService — HTTP-клиент GELIOS GPS (api.geliospro.com).
//
// Auth: OAuth2 password grant. POST /api/v1/auth/login,
// Content-Type x-www-form-urlencoded, body
// grant_type=password&username=<urlenc>&password=<...>
// → {access_token, refresh_token, expires_in, token_type}.
// Далее Authorization: Bearer <access_token>.
//
// База знаний: ACRM-Brain/wiki/sources/gelios-api/billing.md
type GeliosService struct {
	db  *gorm.DB
	cli *http.Client
}

func NewGeliosService(db *gorm.DB) *GeliosService {
	return &GeliosService{db: db, cli: &http.Client{Timeout: 30 * time.Second}}
}

func (s *GeliosService) plainPassword(conn *models.GeliosConnection) (string, error) {
	pw, err := utils.DecryptString(conn.Password)
	if err != nil {
		return "", fmt.Errorf("decrypt password: %w", err)
	}
	return pw, nil
}

// baseURL всегда возвращает allowlisted-хост (SSRF-защита) — игнорирует
// потенциально подменённый conn.BaseURL.
func (s *GeliosService) baseURL(_ *models.GeliosConnection) string {
	return GeliosAllowedBaseURL
}

type geliosTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// login выполняет OAuth2 password grant и сохраняет токен в connection.
func (s *GeliosService) login(conn *models.GeliosConnection) (string, error) {
	pw, err := s.plainPassword(conn)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("username", conn.Username)
	form.Set("password", pw)

	req, err := http.NewRequest(http.MethodPost,
		s.baseURL(conn)+"/api/v1/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("gelios login: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gelios login: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr geliosTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("gelios login: parse token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("gelios login: пустой access_token")
	}

	now := time.Now()
	exp := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	updates := map[string]interface{}{
		"access_token":     tr.AccessToken,
		"refresh_token":    tr.RefreshToken,
		"token_expires_at": exp,
		"last_login_at":    now,
	}
	if e := s.db.Model(conn).Updates(updates).Error; e != nil {
		log.Printf("⚠️ GELIOS: не сохранён токен conn=%d: %v", conn.ID, e)
	}
	conn.AccessToken = tr.AccessToken
	conn.RefreshToken = tr.RefreshToken
	conn.TokenExpiresAt = &exp
	return tr.AccessToken, nil
}

// token возвращает валидный Bearer-токен (кешированный или свежий login).
func (s *GeliosService) token(conn *models.GeliosConnection) (string, error) {
	if conn.AccessToken != "" && conn.TokenExpiresAt != nil &&
		time.Until(*conn.TokenExpiresAt) > 60*time.Second {
		return conn.AccessToken, nil
	}
	return s.login(conn)
}

// TestConnection делает login + GET /api/v1/units?limit=1 для проверки кредов.
// Возвращает units_total (paginationMetadata.totalCount).
func (s *GeliosService) TestConnection(conn *models.GeliosConnection) (map[string]interface{}, error) {
	tok, err := s.login(conn)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet,
		s.baseURL(conn)+"/api/v1/units?limit=1&offset=0", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := s.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gelios units: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gelios units: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		PaginationMetadata struct {
			TotalCount int `json:"totalCount"`
		} `json:"paginationMetadata"`
	}
	_ = json.Unmarshal(body, &parsed)
	return map[string]interface{}{
		"username":    conn.Username,
		"units_total": parsed.PaginationMetadata.TotalCount,
	}, nil
}
