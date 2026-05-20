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
	// epoch/genAll — защита от stale-resurrection (BLK-F3): запись в
	// кэш принимается только если ни per-company epoch, ни глобальная
	// генерация не изменились за время compute (т.е. не было
	// Invalidate/InvalidateAll в гонке с in-flight расчётом).
	epoch  map[uint]uint64
	genAll uint64
}

// NewEntitlementService создаёт сервис.
func NewEntitlementService(db *gorm.DB) *EntitlementService {
	return &EntitlementService{
		db:    db,
		cache: make(map[uint]entitlementCacheEntry),
		epoch: make(map[uint]uint64),
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
	s.epoch[companyID]++
	s.mu.Unlock()
}

// InvalidateAll сбрасывает весь кэш (массовое изменение плана/фичи).
func (s *EntitlementService) InvalidateAll() {
	s.mu.Lock()
	s.cache = make(map[uint]entitlementCacheEntry)
	s.genAll++
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
	e0 := s.epoch[companyID]
	g0 := s.genAll
	s.mu.RUnlock()

	features, err := s.compute(companyID, now)
	if err != nil {
		// Fail-closed: deny + не кэшируем (следующий запрос повторит).
		log.Printf("⚠️ EntitlementService company %d: fail-closed: %v", companyID, err)
		return map[string]Entitlement{}
	}

	s.mu.Lock()
	// BLK-F3: кладём в кэш ТОЛЬКО если за время compute не было
	// Invalidate(company)/InvalidateAll (epoch/genAll не менялись).
	// Иначе результат потенциально stale — вернём свежий вызывающему,
	// но НЕ закэшируем (следующий запрос пересчитает на новом состоянии).
	if s.epoch[companyID] == e0 && s.genAll == g0 {
		s.cache[companyID] = entitlementCacheEntry{
			features:  features,
			expiresAt: now.Add(s.ttl),
		}
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

	// 2. Фичи плана активной подписки — ТОЛЬКО если сам план активен
	// (BLK-F1: деактивация пакета должна отзывать доступ).
	if active != nil {
		var plan models.PlatformPlan
		if err := db.First(&plan, active.PlanID).Error; err != nil {
			return nil, fmt.Errorf("read plan: %w", err)
		}
		if plan.IsActive {
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

	// 4. R3: каталог фич = source of truth (биллинг). Фича грантуется
	// ТОЛЬКО если есть АКТИВНАЯ строка platform_features. Нет строки
	// или is_active=false → код выкидывается и из плана, и из override
	// (продавать снятую с продажи / неизвестную фичу нельзя — fail-
	// closed).
	if len(result) > 0 {
		var feats []models.PlatformFeature
		if err := db.Where("is_active = ?", true).Find(&feats).Error; err != nil {
			return nil, fmt.Errorf("read features: %w", err)
		}
		active := make(map[string]struct{}, len(feats))
		for _, f := range feats {
			active[f.Code] = struct{}{}
		}
		for code := range result {
			if _, ok := active[code]; !ok {
				delete(result, code)
			}
		}
	}

	return result, nil
}
