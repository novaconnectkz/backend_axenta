package models

import "time"

// WialonUser — снимок avl_user из Wialon connection.
// Заполняется WialonStatsService.collectUsersForConnection (core/search_items
// itemsType=avl_user, flags=0x00000002 для custom prop). Используется в
// /api/auth/unified/objects чтобы подменять creator/accountName реальными
// именами из Wialon (включая prp.label/prp.short_name).
//
// Глобальная таблица в public-схеме (как wialon_units), т.к. ссылается на
// wialon_connections (которая глобальная).
type WialonUser struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint  `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_wialon_users_conn_user"`
	UserID       int64 `json:"user_id"       gorm:"not null;uniqueIndex:uq_wialon_users_conn_user"` // avl_user.id

	Name      string `json:"name"       gorm:"type:varchar(255);default:''"`
	ShortName string `json:"short_name" gorm:"type:varchar(255);default:''"` // prp.label / prp.short_name (CMS-style)
	IsActive  bool   `json:"is_active"  gorm:"default:true"`

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"not null;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonUser) TableName() string {
	return "wialon_users"
}
