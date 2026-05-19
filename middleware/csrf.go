package middleware

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// Защита от CSRF (риск B4).
//
// Контекст: фронт acrm.su и бэк api.acrm.su — РАЗНЫЕ сабдомены одного
// site (eTLD+1 = acrm.su). SameSite=Lax НЕ блокирует state-changing
// запросы с соседнего сабдомена (он same-site). Любой захваченный
// `*.acrm.su` мог бы слать POST/PUT/DELETE с refresh-cookie.
//
// Два слоя:
//  1. OriginGuard — на все небезопасные методы: Origin/Referer обязан
//     быть в CORS-allowlist (точный список, НЕ wildcard).
//  2. CSRFDoubleSubmit — для cookie-аутентифицированных эндпоинтов
//     (/api/auth/refresh,/logout): cookie acrm_csrf == заголовок
//     X-CSRF-Token. Запросы с Bearer-токеном в заголовке CSRF-неуязвимы
//     (чужой сайт не может выставить Authorization), для них достаточно
//     слоя 1.

const csrfHeaderName = "X-CSRF-Token"

func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// originHost извлекает host из Origin/Referer для сравнения с allowlist.
func originAllowed(reqOrigin string, allowed []string) bool {
	if reqOrigin == "" {
		return false
	}
	for _, a := range allowed {
		if a == reqOrigin {
			return true
		}
	}
	return false
}

// refererOrigin превращает Referer (полный URL) в scheme://host[:port].
func refererOrigin(referer string) string {
	u, err := url.Parse(referer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// OriginGuard проверяет Origin (или Referer) для небезопасных методов
// против точного CORS-allowlist. Если заголовков нет вовсе (server-to-
// server / curl) — пропускаем: такие клиенты не носят браузерных cookie,
// CSRF-вектор к ним неприменим. Если Origin/Referer ЕСТЬ, но не в
// списке — 403 (это и есть браузерный CSRF/cross-site вектор).
func OriginGuard(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			if ref := c.GetHeader("Referer"); ref != "" {
				origin = refererOrigin(ref)
			}
		}

		if origin == "" {
			// Нет ни Origin, ни Referer → не браузерный cross-site.
			c.Next()
			return
		}

		if !originAllowed(origin, allowed) {
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "Cross-origin запрос отклонён",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// CSRFDoubleSubmit — double-submit для cookie-аутентифицированных
// эндпоинтов. Небезопасный метод обязан принести cookie acrm_csrf,
// равный непустому заголовку X-CSRF-Token. Чужой сайт не может прочитать
// cookie (другой сабдомен) и подставить заголовок → CSRF блокируется.
func CSRFDoubleSubmit() gin.HandlerFunc {
	return CSRFDoubleSubmitCookie(csrfCookieNameMW)
}

// CSRFDoubleSubmitCookie — то же, но с произвольным именем cookie
// (операторский контур использует свою csrf-cookie, изолированно).
func CSRFDoubleSubmitCookie(cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		cookieVal, err := c.Cookie(cookieName)
		headerVal := c.GetHeader(csrfHeaderName)

		if err != nil || cookieVal == "" || headerVal == "" || cookieVal != headerVal {
			c.JSON(http.StatusForbidden, gin.H{
				"status": "error",
				"error":  "CSRF-проверка не пройдена",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// csrfCookieNameMW должен совпадать с api.csrfCookieName ("acrm_csrf").
// Дублируем константой, чтобы не плодить import-цикл api↔middleware.
const csrfCookieNameMW = "acrm_csrf"
