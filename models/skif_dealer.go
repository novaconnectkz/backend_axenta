package models

import "time"

// SkifDealer — локальный кеш дилеров/субинтеграторов SKIF.PRO.
//
// Заполняется SkifService.SyncSubdealers через POST admin/api_v1/dealers_admin_query.
// Хранятся только subdealers (с parent_id), root-dealer текущего интегратора в эту
// таблицу не попадает (он определяется через SkifConnection.login).
type SkifDealer struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;uniqueIndex:uq_skif_dealer_conn_uuid;index"`
	SkifDealerID string `json:"skif_dealer_id" gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_dealer_conn_uuid"` // UUID dealer'а в SKIF

	Name          string `json:"name" gorm:"type:varchar(255);index"`
	Type          string `json:"type" gorm:"type:varchar(32)"` // legal_entity / individual_entrepreneur
	INN           string `json:"inn" gorm:"type:varchar(32)"`
	Phone         string `json:"phone" gorm:"type:varchar(64)"`
	Email         string `json:"email" gorm:"type:varchar(255);index"`
	ContactPerson string `json:"contact_person" gorm:"type:varchar(255)"`
	Address       string `json:"address" gorm:"type:text"`

	// ParentID — UUID parent-dealer (root-dealer текущего интегратора).
	// Subdealers всегда имеют parent_id, root — нет.
	ParentID   string `json:"parent_id" gorm:"type:varchar(64);index"`
	ParentName string `json:"parent_name" gorm:"type:varchar(255)"`

	UnitsCount int  `json:"units_count" gorm:"default:0"`
	Blocked    bool `json:"blocked" gorm:"default:false"`
	IsDefault  bool `json:"is_default" gorm:"default:false"`

	SkifSupportTypeKey string `json:"skif_support_type_key" gorm:"type:varchar(64)"` // telegram / helpdeskeddy

	LastSyncedAt time.Time `json:"last_synced_at" gorm:"index"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifDealer) TableName() string { return "skif_dealers" }
