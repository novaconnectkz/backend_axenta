package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIsRuInnFormat(t *testing.T) {
	assert.True(t, isRuInnFormat("645300905805"))  // 12 цифр (ИП)
	assert.True(t, isRuInnFormat("7707083893"))    // 10 цифр (орг)
	assert.False(t, isRuInnFormat("12345"))        // короткий
	assert.False(t, isRuInnFormat("64530090580X")) // буква
	assert.False(t, isRuInnFormat(""))             // пусто
}

func TestCounterparty_BeforeSave_InnClassify(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Counterparty{}))

	// РФ + 12-значный ИНН + id_type='other' → апгрейд до 'inn'.
	cp := Counterparty{AdminAccountID: 1, CompanyID: 186, Country: "ru", Kind: "partner", IDType: "other", TaxID: "645300905805", Name: "Шигаев"}
	require.NoError(t, db.Create(&cp).Error)
	assert.Equal(t, "inn", cp.IDType)

	// KZ БИН (12 цифр, другой country) НЕ трогаем.
	kz := Counterparty{AdminAccountID: 1, CompanyID: 186, Country: "kz", Kind: "partner", IDType: "other", TaxID: "123456789012", Name: "KZ"}
	require.NoError(t, db.Create(&kz).Error)
	assert.Equal(t, "other", kz.IDType)

	// Явный паспорт НЕ трогаем (не 'other').
	pp := Counterparty{AdminAccountID: 1, CompanyID: 186, Country: "ru", Kind: "client", IDType: "passport", TaxID: "1234567890", Name: "Физик"}
	require.NoError(t, db.Create(&pp).Error)
	assert.Equal(t, "passport", pp.IDType)
}
