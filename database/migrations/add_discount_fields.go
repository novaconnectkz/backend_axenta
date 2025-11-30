package migrations

import (
	"gorm.io/gorm"
)

// AddDiscountFields добавляет поля для скидок в таблицы contracts и partner_daily_snapshots
func AddDiscountFields(db *gorm.DB) error {
	// Добавляем поля в таблицу contracts
	if err := db.Exec(`
		ALTER TABLE contracts 
		ADD COLUMN IF NOT EXISTS discount_type VARCHAR(20) DEFAULT 'none',
		ADD COLUMN IF NOT EXISTS manual_discount_percent DECIMAL(5,2) DEFAULT 0,
		ADD COLUMN IF NOT EXISTS use_auto_discount BOOLEAN DEFAULT false
	`).Error; err != nil {
		return err
	}

	// Добавляем поля в таблицу partner_daily_snapshots
	if err := db.Exec(`
		ALTER TABLE partner_daily_snapshots 
		ADD COLUMN IF NOT EXISTS discount_type VARCHAR(20) DEFAULT 'none',
		ADD COLUMN IF NOT EXISTS discount_percent DECIMAL(5,2) DEFAULT 0,
		ADD COLUMN IF NOT EXISTS cost_before_discount DECIMAL(12,4) DEFAULT 0,
		ADD COLUMN IF NOT EXISTS discount_amount DECIMAL(12,4) DEFAULT 0
	`).Error; err != nil {
		return err
	}

	return nil
}
