package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	// ВНИМАНИЕ: GELIOS отдаёт expires_in как АБСОЛЮТНЫЙ unix-epoch
	// (проверено: now+86401), НЕ относительную длительность.
	ExpiresIn int64 `json:"expires_in"`
}

// persistToken валидирует и сохраняет токен-ответ (общий для login+refresh).
// expires_in трактуется как абсолютный unix-epoch.
func (s *GeliosService) persistToken(conn *models.GeliosConnection, tr geliosTokenResp) (string, error) {
	if tr.AccessToken == "" {
		return "", fmt.Errorf("gelios: пустой access_token")
	}
	if tr.ExpiresIn <= 0 {
		return "", fmt.Errorf("gelios: некорректный expires_in=%d", tr.ExpiresIn)
	}
	now := time.Now()
	exp := time.Unix(tr.ExpiresIn, 0).UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("gelios: токен уже истёк (exp=%s)", exp)
	}
	if e := s.db.Model(conn).Updates(map[string]interface{}{
		"access_token":     tr.AccessToken,
		"refresh_token":    tr.RefreshToken,
		"token_expires_at": exp,
		"last_login_at":    now,
	}).Error; e != nil {
		// Best-effort: in-memory conn-токен валиден для ТЕКУЩЕЙ операции,
		// не валим её из-за DB-сбоя. Не персистнутый ротированный
		// refresh_token само-лечится: следующий тик (scheduler перезагрузит
		// conn из БД) сделает full login (refresh_token пуст/старый → login).
		log.Printf("⚠️ GELIOS: не сохранён токен conn=%d: %v (self-heal: re-login)", conn.ID, e)
	}
	conn.AccessToken = tr.AccessToken
	conn.RefreshToken = tr.RefreshToken
	conn.TokenExpiresAt = &exp
	return tr.AccessToken, nil
}

// refresh — OAuth2 refresh через POST /api/v1/auth/refresh (JSON body).
// GELIOS НЕ принимает grant_type=refresh_token на /auth/login (только
// password); refresh — отдельный JSON-эндпоинт. refresh_token ротируется.
func (s *GeliosService) refresh(conn *models.GeliosConnection) (string, error) {
	if conn.RefreshToken == "" {
		return "", fmt.Errorf("gelios: нет refresh_token")
	}
	bodyJSON, _ := json.Marshal(map[string]string{"refresh_token": conn.RefreshToken})
	req, err := http.NewRequest(http.MethodPost,
		s.baseURL(conn)+"/api/v1/auth/refresh", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.cli.Do(req)
	if err != nil {
		return "", fmt.Errorf("gelios refresh: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// auth-endpoint: raw body НЕ включаем (внешне-контролируемый,
		// попадёт в ErrorMessage→health). Только статус.
		return "", fmt.Errorf("gelios refresh: HTTP %d", resp.StatusCode)
	}
	var tr geliosTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("gelios refresh: parse: %w", err)
	}
	return s.persistToken(conn, tr)
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
		// auth-endpoint: raw body НЕ включаем (внешне-контролируемый,
		// попадёт в ErrorMessage→health). Только статус.
		return "", fmt.Errorf("gelios login: HTTP %d", resp.StatusCode)
	}
	var tr geliosTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("gelios login: parse token: %w", err)
	}
	tok, perr := s.persistToken(conn, tr)
	if perr == nil {
		// login_count++ ТОЛЬКО на full password-login (не refresh):
		// высокий счётчик при фикс #1 = refresh не работает (наблюдаемость).
		s.db.Model(conn).UpdateColumn("login_count", gorm.Expr("login_count + 1"))
	}
	return tok, perr
}

// token возвращает валидный Bearer-токен (кешированный или свежий login).
func (s *GeliosService) token(conn *models.GeliosConnection) (string, error) {
	if conn.AccessToken != "" && conn.TokenExpiresAt != nil &&
		time.Until(*conn.TokenExpiresAt) > 60*time.Second {
		return conn.AccessToken, nil
	}
	// Истёк/нет: сначала refresh (дешевле, без пароля), при неудаче —
	// полный password-login (fallback). refresh_token ротируется.
	if conn.RefreshToken != "" {
		if t, err := s.refresh(conn); err == nil {
			return t, nil
		} else {
			log.Printf("⚠️ GELIOS conn=%d: refresh не удался (%v) → full login", conn.ID, err)
		}
	}
	return s.login(conn)
}

// TestConnection делает login + GET /api/v1/units?limit=1 для проверки кредов.
// Возвращает units_total (paginationMetadata.totalCount).
func (s *GeliosService) TestConnection(conn *models.GeliosConnection) (map[string]interface{}, error) {
	// Под тем же per-conn lock что SyncUnits: login() здесь ротирует
	// refresh_token; параллельный SyncUnits→refresh() иначе = last-write-wins
	// по refresh_token в БД (stale перетёр бы свежий).
	mu := geliosConnLock(conn.ID)
	if !mu.TryLock() {
		return nil, fmt.Errorf("gelios conn=%d занят (sync/test уже выполняется)", conn.ID)
	}
	defer mu.Unlock()

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

type geliosUnitItem struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	IMEI    string `json:"imei"`
	IsBlock bool   `json:"isBlock"`
	Removed bool   `json:"removed"`
	// Unix epoch seconds; null → nil.
	RemovedAt *int64 `json:"removedAt"`
	CreatedAt *int64 `json:"createdAt"`
	Creator   struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"creator"`
	LastMsg *struct {
		Time int64 `json:"time"`
	} `json:"lastMsg"`
}

func geliosUnixPtr(v *int64) *time.Time {
	if v == nil || *v <= 0 {
		return nil
	}
	t := time.Unix(*v, 0).UTC()
	return &t
}

const geliosPageSize = 500

// geliosEmptyConfirmTicks — сколько подряд totalCount=0 синков нужно
// чтобы поверить в реально пустой аккаунт и mass-soft-delete (API-flap).
const geliosEmptyConfirmTicks = 2

// geliosConnLocks — per-connection mutex (prod = single systemd-процесс).
// Защищает от overlap: scheduler-tick + ручной /sync + cron overlap
// иначе бьют по token-кешу, counters и soft-delete окну.
var geliosConnLocks sync.Map // map[uint]*sync.Mutex

func geliosConnLock(id uint) *sync.Mutex {
	m, _ := geliosConnLocks.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// SyncUnits — forward-сбор объектов GELIOS в реестр gelios_units.
// Пагинация GET /api/v1/units?limit&offset, upsert по (connection_id,
// gelios_unit_id), исчезнувшие юниты помечаются gelios_deleted_at ТОЛЬКО
// при полностью успешной выгрузке (нет partial-fail → нет ложного
// soft-delete живого юнита). Прошлое невосстановимо (point-in-time, риск #2).
func (s *GeliosService) SyncUnits(conn *models.GeliosConnection) (int, error) {
	// Per-connection lock: параллельный sync того же conn → ранний выход.
	mu := geliosConnLock(conn.ID)
	if !mu.TryLock() {
		return 0, fmt.Errorf("gelios sync conn=%d уже выполняется", conn.ID)
	}
	defer mu.Unlock()

	now := time.Now()
	tok, err := s.token(conn)
	if err != nil {
		s.recordError(conn, err.Error())
		return 0, err
	}

	saved := 0
	upsertErrs := 0
	seenSet := make(map[string]struct{}, 1024)
	offset := 0
	totalCount := 0
	expectedTotal := -1 // зафиксируем с первой страницы
	for {
		req, e := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/api/v1/units?limit=%d&offset=%d", s.baseURL(conn), geliosPageSize, offset), nil)
		if e != nil {
			s.recordError(conn, e.Error())
			return 0, e
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		resp, e := s.cli.Do(req)
		if e != nil {
			s.recordError(conn, fmt.Sprintf("units: %v", e))
			return 0, fmt.Errorf("gelios units: %w", e)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			s.recordError(conn, fmt.Sprintf("units HTTP %d", resp.StatusCode))
			return 0, fmt.Errorf("gelios units: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var page struct {
			Items              []geliosUnitItem `json:"items"`
			PaginationMetadata struct {
				TotalCount int `json:"totalCount"`
			} `json:"paginationMetadata"`
		}
		if e := json.Unmarshal(body, &page); e != nil {
			s.recordError(conn, "units parse")
			return 0, fmt.Errorf("gelios units: parse: %w", e)
		}
		totalCount = page.PaginationMetadata.TotalCount
		if offset == 0 {
			expectedTotal = totalCount
		}
		// Подозрительная пустая страница при ожидаемых данных = transient,
		// НЕ success (иначе counters/last_sync_at ложно обновятся).
		if len(page.Items) == 0 {
			if offset == 0 && totalCount > 0 {
				s.recordError(conn, "empty page при totalCount>0 (transient)")
				return 0, fmt.Errorf("gelios units: пустая страница при totalCount=%d", totalCount)
			}
			break
		}
		for _, it := range page.Items {
			uid := fmt.Sprintf("%d", it.ID)
			var lastMsg *time.Time
			if it.LastMsg != nil {
				lastMsg = geliosUnixPtr(&it.LastMsg.Time)
			}
			row := models.GeliosUnit{
				ConnectionID:       conn.ID,
				GeliosUnitID:       uid,
				Name:               it.Name,
				IMEI:               it.IMEI,
				IsActive:           !it.Removed && !it.IsBlock,
				CompanyID:          conn.CompanyID,
				GeliosCreatorID:    it.Creator.ID,
				GeliosCreatorLogin: it.Creator.Login,
				LastMsgAt:          lastMsg,
				GeliosCreatedAt:    geliosUnixPtr(it.CreatedAt),
				GeliosRemovedAt:    geliosUnixPtr(it.RemovedAt),
				LastCollectedAt:    now,
			}
			if e := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "gelios_unit_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"name":                 row.Name,
					"imei":                 row.IMEI,
					"is_active":            row.IsActive,
					"company_id":           row.CompanyID,
					"gelios_creator_id":    row.GeliosCreatorID,
					"gelios_creator_login": row.GeliosCreatorLogin,
					"last_msg_at":          row.LastMsgAt,
					"gelios_created_at":    row.GeliosCreatedAt,
					"gelios_removed_at":    row.GeliosRemovedAt,
					"last_collected_at":    row.LastCollectedAt,
					"gelios_deleted_at":    nil, // юнит вернулся → сброс mark
					"updated_at":           time.Now(),
				}),
			}).Create(&row).Error; e != nil {
				log.Printf("⚠️ GELIOS upsert unit %s: %v", uid, e)
				upsertErrs++
				continue
			}
			seenSet[uid] = struct{}{}
			saved++
		}
		offset += geliosPageSize
		if offset >= totalCount {
			break
		}
	}

	// Completeness-gate: mark-deleted и clean-success ТОЛЬКО если выгрузка
	// доказанно полная — нет upsert-ошибок И собрано ровно expectedTotal
	// уникальных юнитов. Иначе (mid-empty, totalCount-drift, partial upsert)
	// = деградированный sync: НЕ success, НЕ трогаем soft-delete (иначе
	// живой юнит ложно пометился бы удалённым). Источник истины.
	complete := upsertErrs == 0 && expectedTotal >= 0 && len(seenSet) == expectedTotal
	if !complete {
		msg := fmt.Sprintf("неполная выгрузка: seen=%d expected=%d upsertErrs=%d", len(seenSet), expectedTotal, upsertErrs)
		s.recordError(conn, msg)
		log.Printf("⚠️ GELIOS conn=%d: %s → mark-deleted+success пропущены", conn.ID, msg)
		return saved, fmt.Errorf("gelios sync conn=%d: %s", conn.ID, msg)
	}

	if expectedTotal == 0 {
		// API-flap защита: чистый totalCount=0 неотличим от сбоя GELIOS.
		// Если были живые юниты — НЕ массово удаляем сразу, ждём
		// geliosEmptyConfirmTicks подтверждений подряд (transient 0 не
		// сносит флот; реально пустой аккаунт чистится через N тиков).
		var liveCount int64
		if e := s.db.Model(&models.GeliosUnit{}).
			Where("connection_id = ? AND gelios_deleted_at IS NULL", conn.ID).
			Count(&liveCount).Error; e != nil {
			// DB-сбой на live-count → НЕ трактуем как пустой аккаунт
			// (иначе success-блок ложно сбросит streak и объявит успех).
			s.recordError(conn, fmt.Sprintf("live-count: %v", e))
			log.Printf("⚠️ GELIOS conn=%d: live-count fail: %v", conn.ID, e)
			return saved, fmt.Errorf("gelios conn=%d: live-count: %w", conn.ID, e)
		}
		if liveCount > 0 {
			// Streak читаем СВЕЖИМ из БД под per-conn lock: scheduler
			// batch-загружает conns заранее, ручной sync мог сбросить
			// empty_sync_streak в БД между load и обработкой — stale
			// struct-значение иначе досрочно открыло бы mass-delete.
			var freshStreak int
			if e := s.db.Model(&models.GeliosConnection{}).
				Select("empty_sync_streak").Where("id = ?", conn.ID).
				Scan(&freshStreak).Error; e != nil {
				// DB-сбой на чтении streak → transient, не решаем по
				// неизвестному состоянию (единообразно с live-count).
				s.recordError(conn, fmt.Sprintf("fresh-streak read: %v", e))
				log.Printf("⚠️ GELIOS conn=%d: fresh-streak read fail: %v", conn.ID, e)
				return saved, fmt.Errorf("gelios conn=%d: fresh-streak read: %w", conn.ID, e)
			}
			streak := freshStreak + 1
			if streak < geliosEmptyConfirmTicks {
				if e := s.db.Model(conn).Update("empty_sync_streak", streak).Error; e != nil {
					// Не персистнули streak → следующий тик начнёт заново
					// (лишняя задержка, но НЕ ложный delete/success).
					log.Printf("⚠️ GELIOS conn=%d: persist streak fail: %v", conn.ID, e)
				}
				msg := fmt.Sprintf("totalCount=0 при %d живых — подтверждение %d/%d, mass-delete отложен (API-flap защита)",
					liveCount, streak, geliosEmptyConfirmTicks)
				s.recordError(conn, msg)
				log.Printf("⚠️ GELIOS conn=%d: %s", conn.ID, msg)
				return saved, fmt.Errorf("gelios conn=%d: %s", conn.ID, msg)
			}
			// Подтверждено N тиков подряд → реально пустой аккаунт.
			res := s.db.Model(&models.GeliosUnit{}).
				Where("connection_id = ? AND gelios_deleted_at IS NULL", conn.ID).
				Update("gelios_deleted_at", now)
			if res.Error != nil {
				// Delete не выполнился → подтверждение НЕ теряем (streak
				// не сбрасываем, success не объявляем) — иначе легит-пустой
				// никогда не очистится.
				s.recordError(conn, fmt.Sprintf("confirmed-empty delete: %v", res.Error))
				log.Printf("⚠️ GELIOS conn=%d: confirmed-empty delete fail: %v", conn.ID, res.Error)
				return saved, fmt.Errorf("gelios conn=%d: confirmed-empty delete: %w", conn.ID, res.Error)
			}
			if res.RowsAffected > 0 {
				log.Printf("🗑 GELIOS conn=%d: пусто подтверждено %dx, помечено удалёнными %d",
					conn.ID, streak, res.RowsAffected)
			}
		}
		// liveCount==0 → нечего удалять (success-блок ниже сбросит streak).
	} else {
		seen := make([]string, 0, len(seenSet))
		for uid := range seenSet {
			seen = append(seen, uid)
		}
		res := s.db.Model(&models.GeliosUnit{}).
			Where("connection_id = ? AND gelios_deleted_at IS NULL AND gelios_unit_id NOT IN ?", conn.ID, seen).
			Update("gelios_deleted_at", now)
		if res.Error != nil {
			log.Printf("⚠️ GELIOS mark deleted: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("🗑 GELIOS conn=%d: помечено удалёнными %d юнитов", conn.ID, res.RowsAffected)
		}
	}

	s.db.Model(conn).Updates(map[string]interface{}{
		"units_count":       len(seenSet), // уникальные (без дублей пагинации)
		"last_sync_at":      &now,
		"error_message":     "",
		"error_count":       0,
		"last_error_at":     nil,
		"empty_sync_streak": 0, // здоровый/подтверждённый синк → сброс
	})
	log.Printf("✅ GELIOS sync units: conn=%d unique=%d total=%d", conn.ID, len(seenSet), expectedTotal)
	return saved, nil
}

func (s *GeliosService) recordError(conn *models.GeliosConnection, msg string) {
	now := time.Now()
	s.db.Model(conn).Updates(map[string]interface{}{
		"last_error_at": &now,
		"error_message": msg,
		"error_count":   gorm.Expr("error_count + 1"),
	})
}
