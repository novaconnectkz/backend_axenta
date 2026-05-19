package services

import (
	"backend_axenta/models"
	"backend_axenta/utils"
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

// AxentaServerToken (Фаза 3) — единый чокпоинт получения Axenta-токена
// для server-side вызовов axenta.cloud.
//
// Зачем: после Ф1 логин = локальный JWT (НЕ Axenta-токен). Эндпоинты,
// которые раньше проксировали в axenta.cloud по токену запроса,
// сломаны. Здесь токен берётся из ХРАНИМЫХ per-company кред
// (Company.AxetnaLogin/AxetnaPassword, пароль enc:v1:) — тот же
// принцип, что у sync-шедулера, не из токена пользователя.
//
// - per-company кэш TenantCredentials (Token+ExpiresAt от Axenta);
//   повторный логин только по истечении/Invalidate → rate-limit
//   axenta.cloud (500/мин/токен) не задеваем (≈1 auth на TTL).
// - singleflight: параллельные запросы одной компании не устраивают
//   auth-stampede.
// - Company читается из main-пула БЕЗ мутации search_path (главный
//   пул всегда public, см. долг #15) — schema-clean by design.

// axentaAuthenticator — узкий интерфейс для тестируемости (реальный —
// *AxetnaClient).
type axentaAuthenticator interface {
	Authenticate(ctx context.Context, login, password string) (*TenantCredentials, error)
}

// expirySkew — обновляем токен заранее, не на грани истечения.
const axentaTokenExpirySkew = 60 * time.Second

// ErrNoAxentaCreds — у компании не настроены Axenta-креды (после Ф1
// bootstrap/provision AxetnaPassword пустой). Вызывающий должен
// деградировать на snapshot/отдать понятную ошибку, не 500.
var ErrNoAxentaCreds = fmt.Errorf("axenta credentials not configured for company")

type AxentaServerToken struct {
	db     *gorm.DB
	client axentaAuthenticator
	sf     singleflight.Group
	mu     sync.RWMutex
	cache  map[uint]*TenantCredentials
	// epoch — счётчик ротаций per-company (Ф3-D #1/Codex Q3). Invalidate
	// инкрементит; acquire фиксирует epoch ДО чтения кред и пишет кэш
	// только если epoch не изменился — иначе in-flight acquire со
	// старыми кредами «воскресил» бы кэш после Invalidate (ротация не
	// применилась бы до TTL).
	epoch map[uint]uint64
}

// NewAxentaServerToken: baseURL обычно "https://axenta.cloud/api"
// (Authenticate бьёт baseURL+"/auth/login").
func NewAxentaServerToken(db *gorm.DB, client axentaAuthenticator) *AxentaServerToken {
	return &AxentaServerToken{
		db:     db,
		client: client,
		cache:  make(map[uint]*TenantCredentials),
		epoch:  make(map[uint]uint64),
	}
}

// Token возвращает валидный Axenta-токен для компании.
func (s *AxentaServerToken) Token(ctx context.Context, companyID uint) (string, error) {
	s.mu.RLock()
	if c, ok := s.cache[companyID]; ok && time.Now().Before(c.ExpiresAt.Add(-axentaTokenExpirySkew)) {
		tok := c.Token
		s.mu.RUnlock()
		return tok, nil
	}
	s.mu.RUnlock()

	// singleflight по компании — один логин на стадо запросов.
	key := strconv.FormatUint(uint64(companyID), 10)
	v, err, _ := s.sf.Do(key, func() (any, error) {
		// Перепроверка кэша внутри singleflight (мог обновиться).
		s.mu.RLock()
		if c, ok := s.cache[companyID]; ok && time.Now().Before(c.ExpiresAt.Add(-axentaTokenExpirySkew)) {
			s.mu.RUnlock()
			return c, nil
		}
		// Ф3-D #1/Codex Q3: фиксируем epoch ДО чтения кред в acquire.
		startEpoch := s.epoch[companyID]
		s.mu.RUnlock()

		creds, aerr := s.acquire(ctx, companyID)
		if aerr != nil {
			return nil, aerr
		}
		s.mu.Lock()
		// Если в окне acquire прошла ротация (Invalidate инкрементил
		// epoch) — НЕ пишем устаревший entry обратно. creds возвращаем
		// вызывающему (для ЭТОГО запроса валидны), но кэш не «воскрешаем»:
		// следующий вызов сделает свежий acquire новыми кредами.
		if s.epoch[companyID] == startEpoch {
			s.cache[companyID] = creds
		}
		s.mu.Unlock()
		return creds, nil
	})
	if err != nil {
		return "", err
	}
	return v.(*TenantCredentials).Token, nil
}

// Invalidate сбрасывает кэш компании (вызывать при downstream 401 от
// axenta.cloud — токен отозван раньше срока).
func (s *AxentaServerToken) Invalidate(companyID uint) {
	s.mu.Lock()
	delete(s.cache, companyID)
	// Ф3-D #1/Codex Q3: бамп epoch — любой in-flight acquire, начавшийся
	// до этого момента (со старыми кредами), при попытке записать кэш
	// увидит изменившийся epoch и НЕ перезапишет (ротация применится
	// сразу, а не после TTL).
	s.epoch[companyID]++
	s.mu.Unlock()
	// Ф3-D #1/Codex Q1 (вариант B): сбрасываем singleflight-флайт, чтобы
	// вызов, пришедший ПОСЛЕ Invalidate, не присоединился к ещё бегущему
	// acquire со старыми кредами и не получил old token. Следующий
	// Token() запустит свежий acquire новыми кредами из БД.
	s.sf.Forget(strconv.FormatUint(uint64(companyID), 10))
}

// Probe проверяет пару login/password против axenta.cloud БЕЗ кэша и БЕЗ
// чтения/записи Company — для "Test connection" в cred-UX (Ф3-D #1).
// nil = креды валидны. Тот же client/endpoint, что acquire → тест
// отражает реальный server-логин (а не угадывает).
func (s *AxentaServerToken) Probe(ctx context.Context, login, password string) error {
	if login == "" || password == "" {
		return ErrNoAxentaCreds
	}
	if _, err := s.client.Authenticate(ctx, login, password); err != nil {
		return err
	}
	return nil
}

// acquire логинится в Axenta по хранимым кредам компании.
func (s *AxentaServerToken) acquire(ctx context.Context, companyID uint) (*TenantCredentials, error) {
	var company models.Company
	// main-пул всегда public (долг #15) — без SET search_path.
	if err := s.db.Session(&gorm.Session{}).First(&company, companyID).Error; err != nil {
		return nil, fmt.Errorf("company %d not found: %w", companyID, err)
	}
	if company.AxetnaLogin == "" || company.AxetnaPassword == "" {
		return nil, ErrNoAxentaCreds
	}
	password, err := utils.DecryptString(company.AxetnaPassword)
	if err != nil {
		return nil, fmt.Errorf("decrypt axenta password (company %d): %w", companyID, err)
	}

	creds, err := s.client.Authenticate(ctx, company.AxetnaLogin, password)
	if err != nil {
		return nil, fmt.Errorf("axenta auth (company %d): %w", companyID, err)
	}
	log.Printf("🔑 AxentaServerToken: получен токен для company %d (exp %s)",
		companyID, creds.ExpiresAt.Format(time.RFC3339))
	return creds, nil
}
