package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"backend_axenta/middleware"
	"backend_axenta/services"
)

// axentaServerToken — singleton Ф3-A (server-side Axenta-токен из хранимых
// Company-кред). Инициализируется в main.go через SetAxentaServerToken.
//
// Ф3-C: live-мутации (создание юзера/аккаунта, activate, toggle, move)
// ходят в axenta.cloud ЭТИМ токеном, НЕ токеном запроса — после Ф1 логин =
// локальный JWT, request-токен невалиден для Axenta (см. concepts/local-auth-ph1.md).
var axentaServerToken *services.AxentaServerToken

// SetAxentaServerToken устанавливает singleton (вызывается из main.go).
func SetAxentaServerToken(s *services.AxentaServerToken) {
	axentaServerToken = s
}

// axentaServerTokenFor возвращает server-side Axenta-токен для компании
// текущего запроса (companyID из доверенного JWT-claim). Если вернул
// ok=false — ответ клиенту уже записан (200 + degraded), вызывающий
// мутационный handler ДОЛЖЕН немедленно `return` и НЕ выполнять мутацию.
// Никакого fallback на request-токен (после Ф1 невалиден).
//
// Семантика: токен = креды компании-аккаунта (Company.AxetnaLogin), поэтому
// мутация в Axenta идёт от имени АККАУНТА КОМПАНИИ, а не конкретного
// залогиненного пользователя (creator/audit в Axenta = компания). Иначе
// после Ф1 никак — персонального request-токена больше нет.
func axentaServerTokenFor(c *gin.Context) (string, bool) {
	if axentaServerToken == nil {
		c.JSON(http.StatusOK, gin.H{
			"status":   "error",
			"degraded": true,
			"error":    "Axenta server-token не инициализирован",
		})
		return "", false
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":   "error",
			"degraded": true,
			"error":    "Компания не определена в запросе",
		})
		return "", false
	}
	tok, err := axentaServerToken.Token(c.Request.Context(), companyID)
	if err != nil {
		// ErrNoAxentaCreds (у компании пустой AxetnaLogin/Password после
		// Ф1-bootstrap) или ошибка авторизации в Axenta — мутацию НЕ делаем,
		// но и не 500: понятный degraded-ответ, фронт покажет «Axenta не
		// подключена».
		msg := "Axenta-аккаунт компании не настроен"
		if !errors.Is(err, services.ErrNoAxentaCreds) {
			msg = "Не удалось получить доступ к Axenta: " + err.Error()
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   "error",
			"degraded": true,
			"error":    msg,
		})
		return "", false
	}
	return tok, true
}

// axentaMutationUnavailable — единый degraded-ответ когда мутацию нельзя
// довести до Axenta (сеть/timeout/недоступность axenta.cloud). Контракт
// Ф3-C: НЕ 500 — мутация не выполнена, фронт показывает «Axenta недоступна».
func axentaMutationUnavailable(c *gin.Context, err error) {
	c.JSON(http.StatusOK, gin.H{
		"status":   "error",
		"degraded": true,
		"error":    "Axenta недоступна, операция не выполнена: " + err.Error(),
	})
}

// invalidateAxentaServerToken сбрасывает кэш server-токена компании запроса.
// Вызывать при downstream 401 от axenta.cloud (токен отозван раньше срока)
// перед единственным retry.
func invalidateAxentaServerToken(c *gin.Context) {
	if axentaServerToken == nil {
		return
	}
	if companyID := middleware.GetCompanyID(c); companyID != 0 {
		axentaServerToken.Invalidate(companyID)
	}
}
