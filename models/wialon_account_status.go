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
// Модель «поддерево» (2026-06-06): Wialon-партнёр = ПРЯМОЙ дилер под интеграционной
// у/з (is_direct_dealer), биллится за ВСЁ поддерево под собой — как Wialon CMS и
// bill_wialon. База биллинга — account/get_account_data (activated_units.usage), а НЕ
// прямой units_count: для дилера с субаккаунтами units_count недосчитывал (напр.
// Шевердяев: прямые 31, поддерево 110). Двойного счёта нет — биллятся только прямые
// дилеры под интеграцией, их дети (под ними, не под интеграцией) отдельно не биллятся.
// См. ObjectsTotal/ObjectsActive ниже.
//
// units_count — прямой счёт юнитов аккаунта (wialon_units.billing_id →
// wialon_object_stats.resource_id → user_id). Оставлен для справки/UI, но для
// дилерского биллинга НЕ годится (только прямые, без поддерева).
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

	// ParentAccountID — родительская у/з (account/get_account_data.parentAccountId,
	// resource id). НЕ crt и НЕ bact-owner — реальный billing-родитель Wialon.
	ParentAccountID int64 `json:"parent_account_id" gorm:"index;default:0"`

	// IsDirectDealer — дилер ПРЯМО под интеграционной у/з (parentAccountId =
	// bact токен-овнера). Для дропдауна партнёрских договоров: дилеры-дилеров
	// (глубже по иерархии) исключаются. Считается в синке через get_account_data.
	IsDirectDealer bool `json:"is_direct_dealer" gorm:"index;default:false"`

	IsActive bool `json:"is_active" gorm:"index;default:true"`

	// UnitsCount — прямой счёт юнитов аккаунта (не recursive).
	// ВНИМАНИЕ: для дилеров недосчитывает поддерево (только прямые юниты). Для
	// partner billing используется ObjectsTotal/ObjectsActive (account_data, поддерево).
	UnitsCount int `json:"units_count" gorm:"default:0"`

	// ObjectsTotal/ObjectsActive — поддеревная метрика из account/get_account_data
	// (activated_units.usage + seasonal_units.usage), агрегированная MAX per user_id
	// из wialon_object_stats (строки дублируются по ресурсам аккаунта одним значением →
	// MAX, не SUM). Это база partner billing Wialon (модель «поддерево», = CMS/bill_wialon):
	// дилер биллится за ВСЁ под собой. ObjectsActive = оплачиваемые (activated_units),
	// ObjectsTotal = все (activated+seasonal). Заполняются в collectAccountsForConnection.
	ObjectsTotal  int `json:"objects_total" gorm:"not null;default:0"`
	ObjectsActive int `json:"objects_active" gorm:"not null;default:0"`

	LastCollectedAt time.Time `json:"last_collected_at" gorm:"not null;index"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Connection *WialonConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (WialonAccountStatus) TableName() string { return "wialon_account_statuses" }
