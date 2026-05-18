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
	Phone   string `json:"phone"`
	Phone2  string `json:"phone2"`
	IsBlock bool   `json:"isBlock"`
	Removed bool   `json:"removed"`
	// Unix epoch seconds; null → nil.
	RemovedAt *int64 `json:"removedAt"`
	CreatedAt *int64 `json:"createdAt"`
	Creator   struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"creator"`
	HwType *struct {
		Name string `json:"name"`
	} `json:"hwType"` // тип оборудования ("Wialon Combine")
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
			hwType := ""
			if it.HwType != nil {
				hwType = it.HwType.Name
			}
			row := models.GeliosUnit{
				ConnectionID:       conn.ID,
				GeliosUnitID:       uid,
				Name:               it.Name,
				IMEI:               it.IMEI,
				Phone:              it.Phone,
				Phone2:             it.Phone2,
				HwTypeName:         hwType,
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
				// phone/phone2/hw_type_name — изменяемые current-state атрибуты
				// (как name/imei): GELIOS units API отдаёт полный объект (поле=""
				// если очищено, не отсутствует), completeness-gate отсекает
				// частичные дампы → unconditional overwrite корректен. НЕ
				// COALESCE-preserve (в отличие от иммутабельного gelios_created_at):
				// иначе легитимная очистка телефона не отразилась бы (Codex #4).
				DoUpdates: clause.Assignments(map[string]interface{}{
					"name":                 row.Name,
					"imei":                 row.IMEI,
					"phone":                row.Phone,
					"phone2":               row.Phone2,
					"hw_type_name":         row.HwTypeName,
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

// geliosUserItem — элемент GET /api/v1/users?inclbillinf=true.
// billingInfo/unitsCount = null без inclbillinf / когда нет юнитов.
type geliosUserItem struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	LegalName string `json:"legalName"`
	IsAdmin   bool   `json:"isAdmin"`
	IsBlock   bool   `json:"isBlock"`
	Creator   struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"creator"`
	BillingInfo *struct {
		Balance    float64 `json:"balance"`
		MinBalance float64 `json:"minBalance"`
		Currency   *struct {
			AlphabeticCode string `json:"alphabeticCode"`
			Abbreviation   string `json:"abbreviation"`
		} `json:"currency"`
		UnitsLimit     *int `json:"unitsLimit"`
		FreeUnitsLimit *int `json:"freeUnitsLimit"`
		PaymentType    *struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"paymentType"`
		PaymentPeriod *struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"paymentPeriod"`
		PaymentPeriodLength int `json:"paymentPeriodLength"`
	} `json:"billingInfo"`
	UnitsCount *struct {
		All    int `json:"all"`
		Active int `json:"active"`
	} `json:"unitsCount"`
	Timestamp *struct {
		Create    *int64 `json:"create"`
		LastLogin *int64 `json:"lastLogin"`
	} `json:"timestamp"`
}

// SyncGeliosUsers — зеркало SyncUnits для дерева пользователей GELIOS.
// GET /api/v1/users?inclbillinf=true (billing в одном вызове, без N+1).
// Тот же completeness-gate + API-flap streak-guard (отдельный
// UsersEmptySyncStreak), soft-delete gelios_deleted_at (GELIOS DELETE = hard,
// у нас только метка). Общий geliosConnLock с SyncUnits — сериализует
// users/units одного conn (защита от race token-refresh).
func (s *GeliosService) SyncGeliosUsers(conn *models.GeliosConnection) (int, error) {
	mu := geliosConnLock(conn.ID)
	if !mu.TryLock() {
		return 0, fmt.Errorf("gelios users sync conn=%d уже выполняется", conn.ID)
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
	seenSet := make(map[string]struct{}, 256)
	offset := 0
	totalCount := 0
	expectedTotal := -1
	for {
		req, e := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/api/v1/users?inclbillinf=true&limit=%d&offset=%d", s.baseURL(conn), geliosPageSize, offset), nil)
		if e != nil {
			s.recordError(conn, e.Error())
			return 0, e
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		resp, e := s.cli.Do(req)
		if e != nil {
			s.recordError(conn, fmt.Sprintf("users: %v", e))
			return 0, fmt.Errorf("gelios users: %w", e)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			s.recordError(conn, fmt.Sprintf("users HTTP %d", resp.StatusCode))
			return 0, fmt.Errorf("gelios users: HTTP %d", resp.StatusCode)
		}
		var page struct {
			Items              []geliosUserItem `json:"items"`
			PaginationMetadata struct {
				TotalCount int `json:"totalCount"`
			} `json:"paginationMetadata"`
		}
		if e := json.Unmarshal(body, &page); e != nil {
			s.recordError(conn, "users parse")
			return 0, fmt.Errorf("gelios users: parse: %w", e)
		}
		totalCount = page.PaginationMetadata.TotalCount
		if offset == 0 {
			expectedTotal = totalCount
		}
		if len(page.Items) == 0 {
			if offset == 0 && totalCount > 0 {
				s.recordError(conn, "users empty page при totalCount>0 (transient)")
				return 0, fmt.Errorf("gelios users: пустая страница при totalCount=%d", totalCount)
			}
			break
		}
		for _, it := range page.Items {
			uid := fmt.Sprintf("%d", it.ID)
			row := models.GeliosUser{
				ConnectionID:    conn.ID,
				GeliosUserID:    uid,
				Login:           it.Login,
				Email:           it.Email,
				Phone:           it.Phone,
				LegalName:       it.LegalName,
				IsAdmin:         it.IsAdmin,
				IsBlock:         it.IsBlock,
				CreatorID:       it.Creator.ID,
				CreatorLogin:    it.Creator.Login,
				CompanyID:       conn.CompanyID,
				LastCollectedAt: now,
			}
			if b := it.BillingInfo; b != nil {
				row.Balance = b.Balance
				row.MinBalance = b.MinBalance
				row.UnitsLimit = b.UnitsLimit
				row.FreeUnitsLimit = b.FreeUnitsLimit
				row.PaymentPeriodLen = b.PaymentPeriodLength
				if b.Currency != nil {
					row.CurrencyCode = b.Currency.AlphabeticCode
					row.CurrencyAbbr = b.Currency.Abbreviation
				}
				if b.PaymentType != nil {
					row.PaymentTypeID = b.PaymentType.ID
					row.PaymentTypeName = b.PaymentType.Name
				}
				if b.PaymentPeriod != nil {
					row.PaymentPeriodID = b.PaymentPeriod.ID
					row.PaymentPeriodName = b.PaymentPeriod.Name
				}
			}
			if uc := it.UnitsCount; uc != nil {
				row.UnitsCount = uc.All
			}
			if ts := it.Timestamp; ts != nil {
				row.GeliosCreatedAt = geliosUnixPtr(ts.Create)
				row.LastLoginAt = geliosUnixPtr(ts.LastLogin)
			}
			if e := s.db.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "connection_id"}, {Name: "gelios_user_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"login":               row.Login,
					"email":               row.Email,
					"phone":               row.Phone,
					"legal_name":          row.LegalName,
					"is_admin":            row.IsAdmin,
					"is_block":            row.IsBlock,
					"creator_id":          row.CreatorID,
					"creator_login":       row.CreatorLogin,
					"company_id":          row.CompanyID,
					"balance":             row.Balance,
					"min_balance":         row.MinBalance,
					"currency_code":       row.CurrencyCode,
					"currency_abbr":       row.CurrencyAbbr,
					"units_limit":         row.UnitsLimit,
					"free_units_limit":    row.FreeUnitsLimit,
					"payment_type_id":     row.PaymentTypeID,
					"payment_type_name":   row.PaymentTypeName,
					"payment_period_id":   row.PaymentPeriodID,
					"payment_period_name": row.PaymentPeriodName,
					"payment_period_len":  row.PaymentPeriodLen,
					"units_count":         row.UnitsCount,
					// Дата создания иммутабельна, last_login монотонен: если
					// GELIOS не отдал timestamp в этот sync (nil) — НЕ затираем
					// уже сохранённое (Codex High). COALESCE(новое, старое).
					"gelios_created_at": gorm.Expr(
						"COALESCE(EXCLUDED.gelios_created_at, gelios_users.gelios_created_at)"),
					"last_login_at": gorm.Expr(
						"COALESCE(EXCLUDED.last_login_at, gelios_users.last_login_at)"),
					"last_collected_at": row.LastCollectedAt,
					"gelios_deleted_at": nil, // юзер вернулся → сброс mark
					"updated_at":        time.Now(),
				}),
			}).Create(&row).Error; e != nil {
				log.Printf("⚠️ GELIOS upsert user %s: %v", uid, e)
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

	complete := upsertErrs == 0 && expectedTotal >= 0 && len(seenSet) == expectedTotal
	if !complete {
		msg := fmt.Sprintf("неполная выгрузка users: seen=%d expected=%d upsertErrs=%d", len(seenSet), expectedTotal, upsertErrs)
		s.recordError(conn, msg)
		log.Printf("⚠️ GELIOS conn=%d users: %s → mark-deleted+success пропущены", conn.ID, msg)
		return saved, fmt.Errorf("gelios users conn=%d: %s", conn.ID, msg)
	}

	if expectedTotal == 0 {
		var liveCount int64
		if e := s.db.Model(&models.GeliosUser{}).
			Where("connection_id = ? AND gelios_deleted_at IS NULL", conn.ID).
			Count(&liveCount).Error; e != nil {
			s.recordError(conn, fmt.Sprintf("users live-count: %v", e))
			return saved, fmt.Errorf("gelios users conn=%d: live-count: %w", conn.ID, e)
		}
		if liveCount > 0 {
			var freshStreak int
			if e := s.db.Model(&models.GeliosConnection{}).
				Select("users_empty_sync_streak").Where("id = ?", conn.ID).
				Scan(&freshStreak).Error; e != nil {
				s.recordError(conn, fmt.Sprintf("users fresh-streak read: %v", e))
				return saved, fmt.Errorf("gelios users conn=%d: fresh-streak read: %w", conn.ID, e)
			}
			streak := freshStreak + 1
			if streak < geliosEmptyConfirmTicks {
				if e := s.db.Model(conn).Update("users_empty_sync_streak", streak).Error; e != nil {
					log.Printf("⚠️ GELIOS conn=%d: persist users streak fail: %v", conn.ID, e)
				}
				msg := fmt.Sprintf("users totalCount=0 при %d живых — подтверждение %d/%d, mass-delete отложен",
					liveCount, streak, geliosEmptyConfirmTicks)
				s.recordError(conn, msg)
				return saved, fmt.Errorf("gelios users conn=%d: %s", conn.ID, msg)
			}
			res := s.db.Model(&models.GeliosUser{}).
				Where("connection_id = ? AND gelios_deleted_at IS NULL", conn.ID).
				Update("gelios_deleted_at", now)
			if res.Error != nil {
				s.recordError(conn, fmt.Sprintf("users confirmed-empty delete: %v", res.Error))
				return saved, fmt.Errorf("gelios users conn=%d: confirmed-empty delete: %w", conn.ID, res.Error)
			}
			if res.RowsAffected > 0 {
				log.Printf("🗑 GELIOS conn=%d: users пусто подтверждено %dx, помечено %d", conn.ID, streak, res.RowsAffected)
			}
		}
	} else {
		seen := make([]string, 0, len(seenSet))
		for uid := range seenSet {
			seen = append(seen, uid)
		}
		res := s.db.Model(&models.GeliosUser{}).
			Where("connection_id = ? AND gelios_deleted_at IS NULL AND gelios_user_id NOT IN ?", conn.ID, seen).
			Update("gelios_deleted_at", now)
		if res.Error != nil {
			log.Printf("⚠️ GELIOS users mark deleted: %v", res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("🗑 GELIOS conn=%d: помечено удалёнными %d users", conn.ID, res.RowsAffected)
		}
	}

	s.db.Model(conn).Updates(map[string]interface{}{
		"users_count":             len(seenSet),
		"users_empty_sync_streak": 0,
	})
	log.Printf("✅ GELIOS sync users: conn=%d unique=%d total=%d", conn.ID, len(seenSet), expectedTotal)
	return saved, nil
}

// GeliosUserCreateReq — вход создания GELIOS-юзера (только разведанные поля).
type GeliosUserCreateReq struct {
	Login     string `json:"login"`
	Password  string `json:"password"`
	CreatorID int64  `json:"creator_id"` // GELIOS id родителя (узел дерева)
	IsAdmin   bool   `json:"is_admin"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	LegalName string `json:"legal_name"`
}

// CreateGeliosUser — POST /api/v1/users. Защита от дурака: длины валидируются
// локально (быстрый внятный fail до сетевого вызова); creator/login GELIOS
// добивает 403 (structurally impossible). Под per-conn lock (как Sync — общий
// token-refresh). Возвращает GELIOS id созданного юзера.
func (s *GeliosService) CreateGeliosUser(conn *models.GeliosConnection, req GeliosUserCreateReq) (int64, error) {
	if len([]rune(req.Login)) < 3 {
		return 0, fmt.Errorf("login: минимум 3 символа")
	}
	if len(req.Password) < 5 {
		return 0, fmt.Errorf("password: минимум 5 символов")
	}
	if req.CreatorID <= 0 {
		return 0, fmt.Errorf("creator_id обязателен")
	}

	mu := geliosConnLock(conn.ID)
	if !mu.TryLock() {
		return 0, fmt.Errorf("gelios conn=%d занят (sync/op уже выполняется)", conn.ID)
	}
	defer mu.Unlock()

	tok, err := s.token(conn)
	if err != nil {
		return 0, err
	}
	body, _ := json.Marshal(map[string]interface{}{
		"login": req.Login, "password": req.Password,
		"creatorId": req.CreatorID, "isAdmin": req.IsAdmin,
		"email": req.Email, "phone": req.Phone, "legalName": req.LegalName,
	})
	httpReq, e := http.NewRequest(http.MethodPost, s.baseURL(conn)+"/api/v1/users", strings.NewReader(string(body)))
	if e != nil {
		return 0, e
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	resp, e := s.cli.Do(httpReq)
	if e != nil {
		return 0, fmt.Errorf("gelios create user: %w", e)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// НЕ прокидываем raw body: запрос содержит password, если GELIOS
		// когда-либо эхо-нёт его в ошибке — утечёт клиенту (Codex Sec).
		// Извлекаем только известные безопасные поля.
		var er struct {
			Error  string `json:"error"`
			Detail any    `json:"detail"`
		}
		_ = json.Unmarshal(respBody, &er)
		msg := er.Error
		if msg == "" && er.Detail != nil {
			if d, e := json.Marshal(er.Detail); e == nil {
				msg = string(d) // 422 detail[] = {loc,msg,type} — без пароля
			}
		}
		if msg == "" {
			msg = "запрос отклонён GELIOS"
		}
		return 0, fmt.Errorf("gelios create user: HTTP %d: %s", resp.StatusCode, msg)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if e := json.Unmarshal(respBody, &created); e != nil || created.ID == 0 {
		return 0, fmt.Errorf("gelios create user: не распарсен id ответа")
	}
	return created.ID, nil
}

// DeleteGeliosUser — DELETE /api/v1/users/{geliosUserID}. HARD-delete в GELIOS.
// Idempotent: 403 "Access denied" (уже нет / вне scope) трактуем как success
// (sync всё равно бы пометил). Под per-conn lock. Защита root/scope — в
// handler'е (только known gelios_users этого conn, см. api).
func (s *GeliosService) DeleteGeliosUser(conn *models.GeliosConnection, geliosUserID string) error {
	mu := geliosConnLock(conn.ID)
	if !mu.TryLock() {
		return fmt.Errorf("gelios conn=%d занят (sync/op уже выполняется)", conn.ID)
	}
	defer mu.Unlock()

	tok, err := s.token(conn)
	if err != nil {
		return err
	}
	httpReq, e := http.NewRequest(http.MethodDelete, s.baseURL(conn)+"/api/v1/users/"+url.PathEscape(geliosUserID), nil)
	if e != nil {
		return e
	}
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("Accept", "application/json")
	resp, e := s.cli.Do(httpReq)
	if e != nil {
		return fmt.Errorf("gelios delete user: %w", e)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	switch {
	case resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK:
		return nil
	case resp.StatusCode == http.StatusForbidden &&
		strings.Contains(string(respBody), "Access denied"):
		// «уже нет» / вне scope — idempotent success (sync синхронизировал бы).
		log.Printf("ℹ️ GELIOS delete user %s: 403 Access denied → idempotent (уже отсутствует)", geliosUserID)
		return nil
	default:
		return fmt.Errorf("gelios delete user: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func (s *GeliosService) recordError(conn *models.GeliosConnection, msg string) {
	now := time.Now()
	s.db.Model(conn).Updates(map[string]interface{}{
		"last_error_at": &now,
		"error_message": msg,
		"error_count":   gorm.Expr("error_count + 1"),
	})
}
