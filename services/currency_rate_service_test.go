package services

import (
	"testing"
	"time"

	"backend_axenta/models"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Реальный формат CBR XML_daily: Value через запятую (RU-decimal), Nominal — за
// сколько единиц указан Value. parseCBRXML нормализует на за-1-единицу.
const cbrSampleXML = `<?xml version="1.0" encoding="windows-1251"?>
<ValCurs Date="29.05.2026" name="Foreign Currency Market">
	<Valute ID="R01239">
		<NumCode>978</NumCode><CharCode>EUR</CharCode>
		<Nominal>1</Nominal><Name>Evro</Name>
		<Value>98,5012</Value><VunitRate>98,5012</VunitRate>
	</Valute>
	<Valute ID="R01235">
		<NumCode>840</NumCode><CharCode>USD</CharCode>
		<Nominal>1</Nominal><Name>Dollar US</Name>
		<Value>90,1234</Value><VunitRate>90,1234</VunitRate>
	</Valute>
	<Valute ID="R01335">
		<NumCode>398</NumCode><CharCode>KZT</CharCode>
		<Nominal>100</Nominal><Name>Tenge</Name>
		<Value>18,5000</Value><VunitRate>0,185</VunitRate>
	</Valute>
</ValCurs>`

func TestParseCBRXML(t *testing.T) {
	effDate, rates, err := parseCBRXML([]byte(cbrSampleXML))
	require.NoError(t, err)
	require.Len(t, rates, 3)

	// Фактическая дата курса из атрибута ValCurs Date (Codex #2).
	assert.Equal(t, day(2026, 5, 29), effDate)

	// EUR/USD: Nominal=1 → как есть.
	assert.True(t, rates["EUR"].Equal(d("98.5012")), "EUR got %s", rates["EUR"])
	assert.True(t, rates["USD"].Equal(d("90.1234")), "USD got %s", rates["USD"])
	// KZT: Nominal=100, Value=18.5 → за 1 единицу = 0.185.
	assert.True(t, rates["KZT"].Equal(d("0.185")), "KZT got %s (ожидался 0.185 = 18.5/100)", rates["KZT"])
}

// Codex #1: реальный CBR XML в windows-1251 с кириллицей в <Name> — декодер
// charmap должен корректно обработать, не упасть. cp1251-байты «Евро» = ЕED 02 EE.
func TestParseCBRXML_Windows1251(t *testing.T) {
	// <Name>Евро</Name> в cp1251: Е=0xC5 в=0xE2 р=0xF0 о=0xEE.
	cp1251 := []byte{
		0xC5, 0xE2, 0xF0, 0xEE, // "Евро" в windows-1251
	}
	xmlStr := `<?xml version="1.0" encoding="windows-1251"?><ValCurs Date="29.05.2026"><Valute><CharCode>EUR</CharCode><Nominal>1</Nominal><Name>` +
		string(cp1251) + `</Name><Value>98,50</Value></Valute></ValCurs>`
	effDate, rates, err := parseCBRXML([]byte(xmlStr))
	require.NoError(t, err, "cp1251 кириллица в Name не должна ронять парсер")
	assert.Equal(t, day(2026, 5, 29), effDate)
	assert.True(t, rates["EUR"].Equal(d("98.50")))
}

func TestParseCBRXML_Errors(t *testing.T) {
	// Пустой/мусорный XML → ошибка, не паника.
	_, _, err := parseCBRXML([]byte(`<ValCurs></ValCurs>`))
	assert.Error(t, err, "пустой набор курсов должен дать ошибку")

	_, _, err = parseCBRXML([]byte(`не xml вовсе`))
	assert.Error(t, err)

	// Битый Value + битый/нулевой Nominal пропускаются, валидный остаётся (Codex #5).
	partial := `<ValCurs Date="29.05.2026">
		<Valute><CharCode>EUR</CharCode><Nominal>1</Nominal><Value>abc</Value></Valute>
		<Valute><CharCode>JPY</CharCode><Nominal>0</Nominal><Value>50,00</Value></Valute>
		<Valute><CharCode>USD</CharCode><Nominal>1</Nominal><Value>90,00</Value></Valute>
	</ValCurs>`
	_, rates, err := parseCBRXML([]byte(partial))
	require.NoError(t, err)
	assert.Len(t, rates, 1)
	assert.True(t, rates["USD"].Equal(d("90")))
	_, hasEUR := rates["EUR"]
	assert.False(t, hasEUR, "битый EUR Value должен быть пропущен")
	_, hasJPY := rates["JPY"]
	assert.False(t, hasJPY, "JPY с Nominal=0 должен быть пропущен (не делён на 1)")
}

// quoteCcyForSource: cbr_rf котирует в RUB, nbk_kz — в KZT.
func TestQuoteCcyForSource(t *testing.T) {
	assert.Equal(t, "RUB", quoteCcyForSource("cbr_rf"))
	assert.Equal(t, "RUB", quoteCcyForSource(""))
	assert.Equal(t, "KZT", quoteCcyForSource("nbk_kz"))
}

func setupRateTestSvc(t *testing.T) *CurrencyRateService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CurrencyRate{}))
	return &CurrencyRateService{db: db}
}

func day(y, m, dd int) time.Time { return time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC) }

func TestGetRate(t *testing.T) {
	s := setupRateTestSvc(t)
	mk := func(dt time.Time, base string, rate string) {
		require.NoError(t, s.db.Create(&models.CurrencyRate{
			RateDate: dt, BaseCcy: base, QuoteCcy: "RUB", Source: "cbr_rf", Rate: d(rate),
		}).Error)
	}
	mk(day(2026, 5, 29), "EUR", "98.50")
	mk(day(2026, 5, 29), "USD", "90.00")

	// base==quote → 1, без обращения к БД.
	r, _, stale, err := s.GetRate(day(2026, 6, 1), "RUB", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("1")))
	assert.False(t, stale)

	// Точная дата.
	r, rd, stale, err := s.GetRate(day(2026, 5, 29), "EUR", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("98.50")))
	assert.Equal(t, day(2026, 5, 29), rd)
	assert.False(t, stale)

	// Fallback на последний ≤ date: 30-го курса нет → берём 29-й, staleness=1 (≤3) → не stale.
	r, rd, stale, err = s.GetRate(day(2026, 5, 30), "EUR", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("98.50")))
	assert.Equal(t, day(2026, 5, 29), rd)
	assert.False(t, stale)

	// Fallback дальше порога: запрос на 02.06 (4 дня от 29.05) → stale=true.
	_, _, stale, err = s.GetRate(day(2026, 6, 2), "EUR", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, stale, "4 дня от последнего курса > maxRateStalenessDays=3 → stale")

	// Нет курса вообще (валюта без записей) → ошибка.
	_, _, _, err = s.GetRate(day(2026, 5, 29), "GBP", "RUB", "cbr_rf")
	assert.Error(t, err)

	// Дата раньше всех курсов → нет ≤ date → ошибка.
	_, _, _, err = s.GetRate(day(2026, 5, 1), "EUR", "RUB", "cbr_rf")
	assert.Error(t, err)
}

func TestUpsertRates_Idempotent(t *testing.T) {
	s := setupRateTestSvc(t)
	rates := map[string]decimal.Decimal{
		"EUR": d("98.50"),
		"USD": d("90.00"),
		"RUB": d("1"), // RUB→RUB должен отфильтроваться (base==quote)
	}
	// Первый upsert: EUR+USD (RUB пропущен).
	n, err := s.upsertRates(day(2026, 5, 29), "cbr_rf", "RUB", rates)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// Повторный upsert с обновлённым EUR — не дубль, а update (идемпотентно по unique).
	rates["EUR"] = d("99.00")
	n, err = s.upsertRates(day(2026, 5, 29), "cbr_rf", "RUB", rates)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	var cnt int64
	s.db.Model(&models.CurrencyRate{}).Count(&cnt)
	assert.Equal(t, int64(2), cnt, "повторный upsert не должен плодить дубли")

	r, _, _, err := s.GetRate(day(2026, 5, 29), "EUR", "RUB", "cbr_rf")
	require.NoError(t, err)
	assert.True(t, r.Equal(d("99.00")), "EUR должен обновиться до 99.00, got %s", r)
}
