package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// blockAxentaAccount — метод сервиса (вызываем из крона, без gin.Context). Токен — из
// инжектированного s.serverTok (server-токен компании, axenta_server_token.go). body {"state":bool}:
// false = заблокировать учётку, true = разблокировать. Зеркало api/accounts.go ToggleAccountStatus,
// но контекстно-нейтрально. 401/403 → invalidate + 1 retry.
func (s *EnforcementService) blockAxentaAccount(ctx context.Context, companyID uint, accountExternalID string, enable bool) error {
	if s.serverTok == nil {
		return fmt.Errorf("AxentaServerToken не инжектирован")
	}
	token, err := s.serverTok.Token(ctx, companyID)
	if err != nil {
		return fmt.Errorf("axenta server token company=%d: %w", companyID, err)
	}
	_ = waitAxentaToken(ctx, token) // rate-limit 500/мин/токен; крон бьёт пачкой (handler этого не делает — латентный долг)

	url := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/%s/activate/", accountExternalID)
	body := []byte(fmt.Sprintf(`{"state":%t}`, enable))
	client := &http.Client{Timeout: 30 * time.Second}

	doReq := func(tk string) (int, []byte, error) {
		req, e := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
		if e != nil {
			return 0, nil, e
		}
		req.Header.Set("Authorization", "Token "+tk)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rsp, e := client.Do(req)
		if e != nil {
			return 0, nil, e
		}
		defer rsp.Body.Close()
		b, e := io.ReadAll(rsp.Body)
		return rsp.StatusCode, b, e
	}

	code, resp, err := doReq(token)
	if err != nil {
		return err
	}
	if isAxentaAuthErr(code) {
		s.serverTok.Invalidate(companyID)
		token, err = s.serverTok.Token(ctx, companyID)
		if err != nil {
			return err
		}
		_ = waitAxentaToken(ctx, token)
		code, resp, err = doReq(token)
		if err != nil {
			return err
		}
	}
	if code >= 400 { // успех по spec = 201
		return fmt.Errorf("axenta activate %s: HTTP %d: %s", accountExternalID, code, string(resp))
	}
	return nil
}

// isAxentaAuthErr — локальный helper (isAxentaAuthError живёт в пакете api, недоступна из services).
func isAxentaAuthErr(code int) bool { return code == 401 || code == 403 }
