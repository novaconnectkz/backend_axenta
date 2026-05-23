package models

import "time"

// WialonAccountStatus — снимок Wialon-аккаунта (avl_user) для partner billing (Ф2).
//
// Зачем отдельная таблица: список аккаунтов Wialon (с ParentId-иерархией и
// DealerRights) живёт только в Redis-кэше all-accounts (эфемерно, заполняется
// LIVE-fetch'ем). Для классификации партнёров в /unified и для агрегации units
// на дилера нужно стабильное БД-представление — иначе на staging (Redis холодный,
// schedulers off) данных нет. Заполняется WialonStatsService при синке на проде;
// на staging приезжает через restore прод-БД (как skif_company_statuses → Ф1).
//
// Модель A (per-account, разведка 2026-05-23): дилер-дерево в Wialon НЕ строится
// — поля bpact нет, parent_user_id(=crt) схлопывается под root-интеграторов, юниты
// биллятся на общий мастер-ресурс. Поэтому Wialon-партнёр = аккаунт с DealerRights,
// биллится за СВОИ прямые units_count (НЕ за поддерево). parent_user_id хранится,
// но для биллинга НЕ используется (оставлен на случай будущей иерархии).
//
// units_count — прямой счёт юнитов аккаунта: wialon_units.billing_id →
// wialon_object_stats.resource_id → user_id (покрытие 100%, без дублей
// wialon_object_stats.active).
//
// Глобальная таблица в public-схеме (как wialon_units), т.к. ссылается на
// wialon_connections (которая глобальная).
type WialonAccountStatus struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID uint  `json:"connection_id" gorm:"not null;index;uniqueIndex:uq_wialon_acc_conn_user"`
	WialonUserID int64 `json:"wialon_user_id" gorm:"not null;uniqueIndex:uq_wialon_acc_conn_user"` // avl_user.id

	Name string `json:"name" gorm:"type:varchar(255);default:''"`

	// ParentUserID — id родительского аккаунта (avl_user, из ParentId/bpact).
	// 0 = корневой аккаунт коннекта. Иерархия дилер→дети строится по этому полю.
	ParentUserID int64 `json:"parent_user_id" gorm:"index;default:0"`

	// DealerRights — sys_account_enable_parent=1 (флаг дилера в Wialon).
	DealerRights bool `json:"dealer_rights" gorm:"index;default:false"`

	IsActive bool `json:"is_active" gorm:"index;default:true"`

	// UnitsCount — прямой счёт юнитов аккаунта (не recursive).
	UnitsCount int `json:"units_count" gorm:"default:0"`

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"not null;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonAccountStatus) TableName() string { return "wialon_account_statuses" }
