package api

import (
	"backend_axenta/services"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// currencyRateScheduler — глобальный экземпляр для ручного триггера.
var currencyRateScheduler *services.CurrencyRateScheduler

// SetCurrencyRateScheduler регистрирует scheduler (вызывается из main).
func SetCurrencyRateScheduler(s *services.CurrencyRateScheduler) {
	currencyRateScheduler = s
}

// PostCurrencyRatesFetch — POST /api/auth/currency/rates/fetch
// Ручная загрузка курсов ЦБ РФ. Body опц.: {"date":"YYYY-MM-DD"} (default сегодня).
// Только admin/superadmin (внешний fetch + запись в public).
func PostCurrencyRatesFetch(c *gin.Context) {
	if !requireContractAssignAccess(c) {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "доступно только администратору"})
		return
	}
	if currencyRateScheduler == nil {
		currencyRateScheduler = services.NewCurrencyRateScheduler()
	}
	var req struct {
		Date string `json:"date"`
	}
	// Невалидный JSON в непустом теле → 400 (Codex #7), пустое тело — ок (default сегодня).
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "некорректное тело запроса"})
			return
		}
	}
	target := time.Now().UTC()
	if req.Date != "" {
		t, e := time.Parse("2006-01-02", req.Date)
		if e != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "date должна быть в формате YYYY-MM-DD"})
			return
		}
		target = t.UTC()
	}
	n, err := currencyRateScheduler.RunForDate(target)
	if err != nil {
		// Внешний fetch/parse/db упал → 502, не молчаливый success (Codex #4).
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "ошибка загрузки курсов: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"loaded": n,
		"date":   target.Format("2006-01-02"),
		"status": currencyRateScheduler.GetStatus(),
	}})
}

// GetCurrencyRate — GET /api/auth/currency/rate?base=EUR&quote=RUB&source=cbr_rf&date=YYYY-MM-DD
// Курс пары на дату (с fallback на последний ≤ date). Любой авторизованный.
func GetCurrencyRate(c *gin.Context) {
	base := c.Query("base")
	quote := c.DefaultQuery("quote", "RUB")
	source := c.DefaultQuery("source", "cbr_rf")
	if base == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "параметр base обязателен"})
		return
	}
	date := time.Now().UTC()
	if d := c.Query("date"); d != "" {
		t, e := time.Parse("2006-01-02", d)
		if e != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "date должна быть в формате YYYY-MM-DD"})
			return
		}
		date = t.UTC()
	}
	svc := services.NewCurrencyRateService()
	rate, rateDate, stale, err := svc.GetRate(date, base, quote, source)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"base":      base,
		"quote":     quote,
		"source":    source,
		"rate":      rate.String(),
		"rate_date": rateDate.Format("2006-01-02"),
		"stale":     stale,
	}})
}
