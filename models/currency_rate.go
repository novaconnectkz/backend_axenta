package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// CurrencyRate — курс валютной пары на дату из конкретного источника (П5 мультивалюта).
// История по датам: храним каждый день отдельной строкой, чтобы пересчитать любой
// прошлый период по курсу, действовавшему ИМЕННО тогда (rebill через reversal+новые
// проводки, не мутация прошлого — Codex [H]).
//
// rate = сколько quote_ccy за 1 единицу base_ccy. Пример: base=EUR, quote=RUB,
// rate=98.5012 → 1 EUR = 98.5012 RUB. Нормализуем на Nominal источника при парсинге
// (CBR отдаёт Value за Nominal единиц — приводим к за-1-единицу).
//
// Глобальная (public) таблица. decimal(18,8) — курсы high-precision, деньги округляем
// только на границе posting (Codex [M] decimal scale).
type CurrencyRate struct {
	ID        uint      `json:"id" gorm:"primarykey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Дата действия курса (день, 00:00 UTC). Не дата загрузки.
	RateDate time.Time `json:"rate_date" gorm:"not null;index:idx_currency_rate_unique,unique,priority:1"`

	BaseCcy  string `json:"base_ccy" gorm:"not null;type:varchar(3);index:idx_currency_rate_unique,unique,priority:2"`  // EUR, USD, KZT
	QuoteCcy string `json:"quote_ccy" gorm:"not null;type:varchar(3);index:idx_currency_rate_unique,unique,priority:3"` // RUB (валюта договора)
	Source   string `json:"source" gorm:"not null;type:varchar(20);index:idx_currency_rate_unique,unique,priority:4"`   // cbr_rf | nbk_kz

	Rate decimal.Decimal `json:"rate" gorm:"not null;type:decimal(18,8)"` // quote за 1 base (нормализован на Nominal)
}

func (CurrencyRate) TableName() string { return "currency_rates" }
