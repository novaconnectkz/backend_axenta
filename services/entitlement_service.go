package services

import (
	"backend_axenta/models"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// EntitlementService — READ-ONLY вычисление «какие фичи ACRM доступны
// компании» (Фаза 2, монетизация). Источник истины:
//
//	effective = фичи плана активной platform_subscription
//	            ⊕ ручные company_entitlements override (могут и
//	              включить, и ВЫКЛЮЧИТЬ точечно)
//
// Никакой записи/бизнес-логики продаж тут нет (это control-plane S3).
// In-memory кэш per-company с TTL + явная инвалидация при изменении
// подписки/override (хук вызывается из control-plane операций).
//
// НЕ трогает операционный billing (TariffPlan/Subscription) — другой
// контур.

const entitlementCacheTTL = 45 * time.Second

// Entitlement — эффективное состояние одной фичи для компании.
type Entitlement struct {
	Enabled    bool
	LimitsJSON string
	Source     string // "plan" | "override"
}

type entitlementCacheEntry struct {
	features  map[string]Entitlement
	expiresAt time.Time
}

// EntitlementService потокобезопасен.
type EntitlementService struct {
	db    *gorm.DB
	mu    sync.RWMutex
	cache map[uint]entitlementCacheEntry
	ttl   time.Duration
}

// NewEntitlementService создаёт сервис.
func NewEntitlementService(db *gorm.DB) *EntitlementService {
	return &EntitlementService{
		db:    db,
		cache: make(map[uint]entitlementCacheEntry),
		ttl:   entitlementCacheTTL,
	}
}

func (s *EntitlementService) publicDB() *gorm.DB {
	db := s.db.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		// sqlite (юнит-тесты) не знает SET search_path — не критично.
		log.Printf("⚠️ EntitlementService: search_path public: %v", err)
	}
	return db
}

// Invalidate сбрасывает кэш компании (вызывать при смене
// platform_subscription / company_entitlements).
func (s *EntitlementService) Invalidate(companyID uint) {
	s.mu.Lock()
	delete(s.cache, companyID)
	s.mu.Unlock()
}

// InvalidateAll сбрасывает весь кэш (напр. при массовом изменении плана).
func (s *EntitlementService) InvalidateAll() {
	s.mu.Lock()
	s.cache = make(map[uint]entitlementCacheEntry)
	s.mu.Unlock()
}

// IsEnabled — доступна ли фича компании прямо сейчас.
func (s *EntitlementService) IsEnabled(companyID uint, featureCode string) bool {
	ent, ok := s.effective(companyID)[featureCode]
	return ok && ent.Enabled
}

// GetLimits возвращает LimitsJSON эффективной фичи ("" если нет/выкл).
func (s *EntitlementService) GetLimits(companyID uint, featureCode string) string {
	ent, ok := s.effective(companyID)[featureCode]
	if !ok || !ent.Enabled {
		return ""
	}
	return ent.LimitsJSON
}

// Effective — копия карты эффективных фич компании (для control-plane/UI).
func (s *EntitlementService) Effective(companyID uint) map[string]Entitlement {
	src := s.effective(companyID)
	out := make(map[string]Entitlement, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// effective — кэшируемое вычисление.
//
// BLK1 fail-CLOSED: при ЛЮБОЙ ошибке чтения из БД возвращаем пустую
// карту (всё выключено) и НЕ кэшируем — иначе потеря disable-override
// из-за сбоя БД выдала бы платную фичу бесплатно (revenue leak).
func (s *EntitlementService) effective(companyID uint) map[string]Entitlement {
	now := time.Now()

	s.mu.RLock()
	if e, ok := s.cache[companyID]; ok && now.Before(e.expiresAt) {
		s.mu.RUnlock()
		return e.features
	}
	s.mu.RUnlock()

	features, err := s.compute(companyID, now)
	if err != nil {
		// Fail-closed: deny + не кэшируем (следующий запрос повторит).
		log.Printf("⚠️ EntitlementService company %d: fail-closed: %v", companyID, err)
		return map[string]Entitlement{}
	}

	s.mu.Lock()
	s.cache[companyID] = entitlementCacheEntry{
		features:  features,
		expiresAt: now.Add(s.ttl),
	}
	s.mu.Unlock()
	return features
}

// compute считает эффективные фичи из БД. Любая ошибка → (nil, err):
// вызывающий обязан трактовать как «всё выключено» (fail-closed).
func (s *EntitlementService) compute(companyID uint, now time.Time) (map[string]Entitlement, error) {
	db := s.publicDB()
	result := make(map[string]Entitlement)

	// 1. Активная подписка компании (последняя по StartsAt).
	var subs []models.PlatformSubscription
	if err := db.Where("company_id = ?", companyID).
		Order("starts_at DESC").Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("read subscriptions: %w", err)
	}
	var active *models.PlatformSubscription
	for i := range subs {
		if subs[i].IsCurrent(now) {
			active = &subs[i]
			break
		}
	}

	// 2. Фичи плана активной подписки.
	if active != nil {
		var planFeatures []models.PlatformPlanFeature
		if err := db.Where("plan_id = ?", active.PlanID).Find(&planFeatures).Error; err != nil {
			return nil, fmt.Errorf("read plan features: %w", err)
		}
		for _, pf := range planFeatures {
			result[pf.FeatureCode] = Entitlement{
				Enabled:    true,
				LimitsJSON: pf.LimitsJSON,
				Source:     "plan",
			}
		}
	}

	// 3. Ручные override (могут включить или ВЫКЛЮЧИТЬ поверх плана).
	// Ошибка тут критична: потеря disable-override = revenue leak.
	var overrides []models.CompanyEntitlement
	if err := db.Where("company_id = ?", companyID).Find(&overrides).Error; err != nil {
		return nil, fmt.Errorf("read overrides: %w", err)
	}
	for _, ov := range overrides {
		if !ov.IsCurrent(now) {
			continue
		}
		result[ov.FeatureCode] = Entitlement{
			Enabled:    ov.Enabled,
			LimitsJSON: ov.LimitsJSON,
			Source:     "override",
		}
	}

	return result, nil
}
