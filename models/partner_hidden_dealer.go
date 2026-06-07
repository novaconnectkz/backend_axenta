package models

import "time"

// PartnerHiddenDealer — дилеры-ОРИЕНТИРЫ (contract_id=0), скрытые из справочника снимков.
// Скрывает дилера ЦЕЛИКОМ (все даты, включая будущие снимки): ключ — (partner_source,
// connection_id, partner_external_id), а не конкретная строка-снимок, поэтому новые
// ночные снимки этого дилера тоже не покажутся. Реверсивно: unhide удаляет запись.
// Только ориентиры — договорные снимки (contract_id>0) скрывать запрещено серверно
// (защита от потери денежной строки из вида). Живёт в tenant-схеме (как
// partner_daily_snapshots), читается/пишется через tenant_db из контекста.
type PartnerHiddenDealer struct {
	ID                uint      `json:"id" gorm:"primarykey"`
	CreatedAt         time.Time `json:"created_at"`
	PartnerSource     string    `json:"partner_source" gorm:"type:varchar(20);not null;uniqueIndex:idx_partner_hidden_dealer_unique,priority:1"`
	ConnectionID      uint      `json:"connection_id" gorm:"not null;default:0;uniqueIndex:idx_partner_hidden_dealer_unique,priority:2"`
	PartnerExternalID string    `json:"partner_external_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_partner_hidden_dealer_unique,priority:3"`
	// HiddenBy — ID пользователя, скрывшего дилера (для аудита).
	HiddenBy uint `json:"hidden_by" gorm:"default:0"`
}

// TableName задаёт имя таблицы для PartnerHiddenDealer.
func (PartnerHiddenDealer) TableName() string {
	return "partner_hidden_dealers"
}
