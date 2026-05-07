package models

import "time"

// SkifUnit — реестр объектов мониторинга SKIF.PRO (одна запись на (connection, skif_unit_id)).
// UNIQUE(connection_id, skif_unit_id) гарантирует дедупликацию.
//
// Источник: GET /api_v1/units (см. wiki/sources/skif-api/obekty.md).
// Заполняется SkifSyncScheduler по расписанию connection.SyncInterval.
type SkifUnit struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint   `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_skif_units_conn_unit"`
	SkifUnitID   string `json:"skif_unit_id"  gorm:"not null;type:varchar(100);uniqueIndex:uq_skif_units_conn_unit"` // UUID из SKIF

	Name          string `json:"name"           gorm:"type:varchar(255)"`
	IMEI          string `json:"imei"           gorm:"type:varchar(50);index"`
	Phone         string `json:"phone"          gorm:"type:varchar(50)"`
	Model         string `json:"model"          gorm:"type:varchar(100)"`
	IsActive      bool   `json:"is_active"      gorm:"index;default:true"`
	CompanyID     uint   `json:"company_id"     gorm:"index"` // ACRM tenant company_id (для фильтра)
	SkifCompanyID string `json:"skif_company_id" gorm:"type:varchar(64);index"` // UUID компании-дилера в SKIF
	SkifCompany   string `json:"skif_company"   gorm:"type:varchar(255)"` // Название компании в SKIF

	LastCollectedAt time.Time  `json:"last_collected_at" gorm:"not null;index"`
	SkifDeletedAt   *time.Time `json:"skif_deleted_at" gorm:"index"` // Когда юнит исчез из выгрузки SKIF (бизнес-метка)
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifUnit) TableName() string { return "skif_units" }
