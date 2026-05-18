package models

import "time"

// GeliosUnit — реестр объектов мониторинга GELIOS (одна запись на (connection, gelios_unit_id)).
// UNIQUE(connection_id, gelios_unit_id) гарантирует дедупликацию.
//
// Источник: GET /api/v1/units?limit&offset (paginationMetadata.totalCount).
// Item: id, name, imei, isBlock, isFree, removed, removedAt, activatedAt,
// createdAt, creator{id,login}, lastMsg{time}.
// active = !removed && !isBlock. Заполняется GeliosSyncScheduler по
// расписанию connection.SyncInterval (forward-сбор, прошлое невосстановимо).
//
// База знаний: ACRM-Brain/wiki/sources/gelios-api/billing.md
type GeliosUnit struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_gelios_units_conn_unit"`
	GeliosUnitID string `json:"gelios_unit_id" gorm:"not null;type:varchar(100);uniqueIndex:uq_gelios_units_conn_unit"` // unit.id из GELIOS

	Name      string `json:"name"       gorm:"type:varchar(255)"`
	IMEI      string `json:"imei"       gorm:"type:varchar(50);index"`
	IsActive  bool   `json:"is_active"  gorm:"index;default:true"` // !removed && !isBlock
	CompanyID uint   `json:"company_id" gorm:"index"`              // ACRM tenant company_id (для фильтра)

	// Владелец-аккаунт в GELIOS (creator). На тест-токене маскирован (id=0, login="*****").
	GeliosCreatorID    int64  `json:"gelios_creator_id"    gorm:"index"`
	GeliosCreatorLogin string `json:"gelios_creator_login" gorm:"type:varchar(255);index"`

	LastMsgAt       *time.Time `json:"last_msg_at"`                    // lastMsg.time (had_comm индикатор)
	GeliosCreatedAt *time.Time `json:"gelios_created_at" gorm:"index"` // createdAt в GELIOS
	GeliosRemovedAt *time.Time `json:"gelios_removed_at" gorm:"index"` // removedAt в GELIOS
	LastCollectedAt time.Time  `json:"last_collected_at" gorm:"not null;index"`
	GeliosDeletedAt *time.Time `json:"gelios_deleted_at" gorm:"index"` // Когда юнит исчез из выгрузки (бизнес-метка)
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	Connection *GeliosConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (GeliosUnit) TableName() string { return "gelios_units" }
