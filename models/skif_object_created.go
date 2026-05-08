package models

import "time"

// SkifObjectCreated — кэш дат создания сущностей SKIF (units / company / users / etc).
//
// SKIF не отдаёт `created` в response объектов через `auth_admin_query` (whitelist
// полей). Реальная дата создания доступна только через `/company/updates/query`
// (operation=POST event с `created` timestamp).
//
// Окно /updates/query — макс 1 год, поэтому для старых сущностей нужен backfill
// по годам с сохранением в эту таблицу. После backfill дата сохраняется навсегда.
type SkifObjectCreated struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;uniqueIndex:uq_skif_obj_created;index"`
	ObjectType   string `json:"object_type"   gorm:"not null;type:varchar(32);uniqueIndex:uq_skif_obj_created"` // units / company / users / geozones / etc
	ObjectID     string `json:"object_id"     gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_obj_created"`

	CompanyContextID string `json:"company_context_id" gorm:"type:varchar(64);index"` // SKIF company_id в контексте которой произошло событие

	CreatedSkifAt time.Time `json:"created_skif_at" gorm:"index"` // дата создания сущности в SKIF (created из event)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifObjectCreated) TableName() string { return "skif_object_created" }
