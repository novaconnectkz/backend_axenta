package models

import "testing"

// Тесты слоя отображения C3 (subject-first): приоритет источников
// partner_* / Counterparty / fallback client_*.

func TestContractDisplay_PartnerSourcesFromPartnerFields(t *testing.T) {
	c := &Contract{
		ContractType:      "partner",
		PartnerName:       "ООО Партнёр",
		PartnerINN:        "7700000001",
		PartnerRequisites: `{"short_name":"Партнёр ОПФ","email":"p@ex.ru","phone":"+700","address":"Москва"}`,
		// денорм client_* специально другие — не должны просочиться
		ClientName:  "СТАРОЕ КЛИЕНТ-ИМЯ",
		ClientEmail: "old@ex.ru",
	}
	if got := c.DisplayName(); got != "ООО Партнёр" {
		t.Errorf("DisplayName = %q, want partner_name", got)
	}
	if got := c.DisplayShortName(); got != "Партнёр ОПФ" {
		t.Errorf("DisplayShortName = %q, want requisites.short_name", got)
	}
	if got := c.DisplayShortOrName(); got != "Партнёр ОПФ" {
		t.Errorf("DisplayShortOrName = %q", got)
	}
	if got := c.DisplayINN(); got != "7700000001" {
		t.Errorf("DisplayINN = %q, want partner_inn", got)
	}
	if got := c.DisplayEmail(); got != "p@ex.ru" {
		t.Errorf("DisplayEmail = %q, want requisites.email", got)
	}
	if got := c.DisplayPhone(); got != "+700" {
		t.Errorf("DisplayPhone = %q", got)
	}
	if got := c.DisplayAddress(); got != "Москва" {
		t.Errorf("DisplayAddress = %q", got)
	}
}

func TestContractDisplay_PartnerEmptyRequisites(t *testing.T) {
	for _, req := range []string{"", "{}"} {
		c := &Contract{ContractType: "partner", PartnerName: "P", PartnerRequisites: req}
		if got := c.DisplayShortName(); got != "" {
			t.Errorf("req=%q DisplayShortName = %q, want empty", req, got)
		}
		// DisplayShortOrName падает на полное имя
		if got := c.DisplayShortOrName(); got != "P" {
			t.Errorf("req=%q DisplayShortOrName = %q, want P", req, got)
		}
	}
}

func TestContractDisplay_PartnerFallbackToClientUntilC4(t *testing.T) {
	// Партнёр с пустым partner_name/requisites → fallback на денорм client_*
	// (инвариант C3: client_* живёт до C4). Защита от строк, не прошедших backfill.
	c := &Contract{
		ContractType:    "partner",
		PartnerName:     "",
		ClientName:      "Старый Партнёр client_name",
		ClientShortName: "Партнёр ОПФ",
		ClientINN:       "555",
		ClientEmail:     "legacy@ex.ru",
		ClientPhone:     "+701",
		ClientAddress:   "Казань",
	}
	if got := c.DisplayName(); got != "Старый Партнёр client_name" {
		t.Errorf("DisplayName = %q, want client_name fallback", got)
	}
	if got := c.DisplayShortName(); got != "Партнёр ОПФ" {
		t.Errorf("DisplayShortName = %q, want client_short fallback", got)
	}
	if got := c.DisplayINN(); got != "555" {
		t.Errorf("DisplayINN = %q, want client_inn fallback", got)
	}
	if got := c.DisplayEmail(); got != "legacy@ex.ru" {
		t.Errorf("DisplayEmail = %q, want client_email fallback", got)
	}
	if got := c.DisplayPhone(); got != "+701" {
		t.Errorf("DisplayPhone = %q", got)
	}
	if got := c.DisplayAddress(); got != "Казань" {
		t.Errorf("DisplayAddress = %q", got)
	}
}

func TestContractDisplay_ClientPrefersCounterparty(t *testing.T) {
	c := &Contract{
		ContractType:    "client",
		ClientName:      "client_name снимок",
		ClientShortName: "client_short снимок",
		ClientINN:       "111",
		ClientEmail:     "c@ex.ru",
		Counterparty: &Counterparty{
			Name:      "Контрагент Имя",
			ShortName: "Контрагент ОПФ",
			TaxID:     "999",
			Email:     "cp@ex.ru",
		},
	}
	if got := c.DisplayName(); got != "Контрагент Имя" {
		t.Errorf("DisplayName = %q, want cp.Name", got)
	}
	if got := c.DisplayShortName(); got != "Контрагент ОПФ" {
		t.Errorf("DisplayShortName = %q, want cp.ShortName", got)
	}
	if got := c.DisplayINN(); got != "999" {
		t.Errorf("DisplayINN = %q, want cp.TaxID", got)
	}
	if got := c.DisplayEmail(); got != "cp@ex.ru" {
		t.Errorf("DisplayEmail = %q, want cp.Email", got)
	}
}

func TestContractDisplay_ClientFallbackToDenorm(t *testing.T) {
	// cp не Preload'нут (nil) ИЛИ пустые поля → fallback на client_* (mirror).
	c := &Contract{
		ContractType:    "client",
		ClientName:      "Клиент снимок",
		ClientShortName: "Клиент ОПФ",
		ClientINN:       "222",
		ClientEmail:     "c@ex.ru",
	}
	if got := c.DisplayName(); got != "Клиент снимок" {
		t.Errorf("DisplayName = %q, want client_name fallback", got)
	}
	if got := c.DisplayShortOrName(); got != "Клиент ОПФ" {
		t.Errorf("DisplayShortOrName = %q, want client_short_name", got)
	}
	if got := c.DisplayINN(); got != "222" {
		t.Errorf("DisplayINN = %q", got)
	}

	// cp есть, но имя пустое → всё равно fallback на client_*
	c.Counterparty = &Counterparty{Name: "  "}
	if got := c.DisplayName(); got != "Клиент снимок" {
		t.Errorf("empty cp.Name: DisplayName = %q, want fallback", got)
	}
}
