package models

import "time"

// Контур МОНЕТИЗАЦИИ ACRM (Фаза 2) — оплата ДОСТУПА к самой ACRM и
// продажа отдельных фич (feature-entitlements).
//
// ВАЖНО: это ОТДЕЛЬНЫЙ неймспейс (префикс platform_). НЕ путать с
// операционным биллингом (TariffPlan/BillingPlan/Subscription/Invoice/
// Contract) — тот про бизнес тенанта (дилеры ↔ конечники, GPS-объекты)
// и здесь НЕ переиспользуется и НЕ затрагивается. Ноль FK/связей между
// контурами. Все таблицы — глобальные (схема public).

// PlatformPlan — коммерческий пакет доступа к ACRM (Basic/Pro/...).
type PlatformPlan struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;not null;size:64"`
	Name        string    `json:"name" gorm:"not null;size:128"`
	Description string    `json:"description" gorm:"size:512"`
	PriceMinor  int64     `json:"price_minor" gorm:"not null;default:0"` // в копейках
	Currency    string    `json:"currency" gorm:"not null;size:3;default:'RUB'"`
	Period      string    `json:"period" gorm:"not null;size:16;default:'month'"` // month|year
	IsActive    bool      `json:"is_active" gorm:"not null;default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PlatformPlan) TableName() string { return "platform_plans" }

// PlatformFeature — каталог продаваемых фич ACRM.
type PlatformFeature struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;not null;size:64"`
	Name        string    `json:"name" gorm:"not null;size:128"`
	Description string    `json:"description" gorm:"size:512"`
	IsActive    bool      `json:"is_active" gorm:"not null;default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PlatformFeature) TableName() string { return "platform_features" }

// PlatformPlanFeature — какие фичи входят в пакет (+ лимиты).
type PlatformPlanFeature struct {
	ID          uint   `json:"id" gorm:"primaryKey"`
	PlanID      uint   `json:"plan_id" gorm:"not null;index;uniqueIndex:uq_plan_feature"`
	FeatureCode string `json:"feature_code" gorm:"not null;size:64;index;uniqueIndex:uq_plan_feature"`
	// LimitsJSON — произвольные лимиты фичи в пакете (JSON-строка),
	// напр. {"max_objects":1000}. Пусто = без лимита.
	LimitsJSON string    `json:"limits_json" gorm:"type:text"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (PlatformPlanFeature) TableName() string { return "platform_plan_features" }

// PlatformSubscription — подписка компании на пакет доступа к ACRM.
// CompanyID ссылается на public.companies.id, но БЕЗ жёсткого FK
// (constraint:-) — как и прочие кросс-схемные связи в проекте.
type PlatformSubscription struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	CompanyID   uint       `json:"company_id" gorm:"not null;index"`
	PlanID      uint       `json:"plan_id" gorm:"not null;index"`
	Status      string     `json:"status" gorm:"not null;size:16;default:'active'"` // active|trial|past_due|canceled
	StartsAt    time.Time  `json:"starts_at" gorm:"not null"`
	EndsAt      *time.Time `json:"ends_at"`
	TrialEndsAt *time.Time `json:"trial_ends_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Plan *PlatformPlan `json:"plan,omitempty" gorm:"foreignKey:PlanID;constraint:-"`
}

func (PlatformSubscription) TableName() string { return "platform_subscriptions" }

// IsCurrent — подписка действует прямо сейчас.
func (s *PlatformSubscription) IsCurrent(now time.Time) bool {
	if s.Status != "active" && s.Status != "trial" {
		return false
	}
	if now.Before(s.StartsAt) {
		return false
	}
	if s.EndsAt != nil && now.After(*s.EndsAt) {
		return false
	}
	return true
}

// CompanyEntitlement — ручной override фичи для компании поверх плана
// (включить/выключить точечно, с лимитами и причиной).
type CompanyEntitlement struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	CompanyID      uint       `json:"company_id" gorm:"not null;index;uniqueIndex:uq_company_feature"`
	FeatureCode    string     `json:"feature_code" gorm:"not null;size:64;index;uniqueIndex:uq_company_feature"`
	// БЕЗ default — иначе GORM пропускает zero-value(false) в INSERT
	// и БД ставит default → override не может ВЫКЛЮЧИТЬ фичу плана.
	// Значение всегда задаётся явно control-plane'ом.
	Enabled        bool       `json:"enabled" gorm:"not null"`
	LimitsJSON     string     `json:"limits_json" gorm:"type:text"`
	OverrideReason string     `json:"override_reason" gorm:"size:256"`
	StartsAt       *time.Time `json:"starts_at"`
	EndsAt         *time.Time `json:"ends_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (CompanyEntitlement) TableName() string { return "company_entitlements" }

// IsCurrent — override действует прямо сейчас.
func (e *CompanyEntitlement) IsCurrent(now time.Time) bool {
	if e.StartsAt != nil && now.Before(*e.StartsAt) {
		return false
	}
	if e.EndsAt != nil && now.After(*e.EndsAt) {
		return false
	}
	return true
}
