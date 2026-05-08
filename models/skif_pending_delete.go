package models

import "time"

// SkifPendingDelete — локальная запись о запланированном удалении SKIF-компании.
//
// SKIF использует schedule_delete с 14-дневной отсрочкой. Сам SKIF API не отдаёт
// дату фактического удаления в company list — мы храним её локально для UI countdown
// и возможности отмены через cancel_delete до истечения срока.
//
// Заполняется в SkifService.DeleteCompany, удаляется в SkifService.CancelDeleteCompany.
type SkifPendingDelete struct {
	ID uint `json:"id" gorm:"primarykey"`

	ConnectionID  uint   `json:"connection_id" gorm:"not null;uniqueIndex:uq_skif_pending_conn_comp;index"`
	SkifCompanyID string `json:"skif_company_id" gorm:"not null;type:varchar(64);uniqueIndex:uq_skif_pending_conn_comp"`
	CompanyName   string `json:"company_name" gorm:"type:varchar(255)"`

	ScheduledAt  time.Time `json:"scheduled_at"`             // когда юзер инициировал удаление
	ScheduledFor time.Time `json:"scheduled_for" gorm:"index"` // когда SKIF фактически удалит (now+14d)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Connection *SkifConnection `json:"-" gorm:"foreignKey:ConnectionID;constraint:OnDelete:CASCADE"`
}

func (SkifPendingDelete) TableName() string { return "skif_pending_deletes" }
