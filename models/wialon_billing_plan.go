package models

import "time"

// WialonBillingPlan — кэш тарифного плана Wialon в БД.
// Заполняется WialonBillingPlansScheduler (раз в час) + lazy-init при первом обращении.
// Endpoint /api/wialon/connections/:id/billing-plans читает отсюда мгновенно.
type WialonBillingPlan struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_wialon_billing_plans_conn_name"`
	PlanName     string `json:"plan_name"     gorm:"not null;type:varchar(255);uniqueIndex:uq_wialon_billing_plans_conn_name"`
	RawPayload   string `json:"raw_payload"   gorm:"type:jsonb"`

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"not null;index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonBillingPlan) TableName() string {
	return "wialon_billing_plans"
}
