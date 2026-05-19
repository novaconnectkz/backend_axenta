package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Anti-bruteforce (риск B9): после lockThreshold неудачных входов
// подряд аккаунт блокируется на lockDuration. Сбрасывается при успехе.
// Per-IP троттлинг — отдельно, через middleware.StrictRateLimit на роутах.
const (
	lockThreshold = 5
	lockDuration  = 15 * time.Minute
)

// Имена/область cookie. Refresh — httpOnly, host-only (без Domain →
// только api.acrm.su, риск B3), Path /api/auth (не шлётся на остальное
// API). CSRF — JS-читаемый, double-submit (риск B4).
const (
	refreshCookieName = "acrm_refresh"
	csrfCookieName    = "acrm_csrf"
	authCookiePath    = "/api/auth"
)

// LocalAuthAPI API для локальной авторизации
type LocalAuthAPI struct {
	db         *gorm.DB
	jwtService *services.JWTService
}

func (api *LocalAuthAPI) getPublicDB() *gorm.DB {
	publicDB := api.db.Session(&gorm.Session{})
	if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ Не удалось переключиться на схему public: %v", err)
	}
	return publicDB
}

// NewLocalAuthAPI создает новый API для локальной авторизации
func NewLocalAuthAPI(db *gorm.DB, jwtService *services.JWTService) *LocalAuthAPI {
	return &LocalAuthAPI{db: db, jwtService: jwtService}
}

func authIsProduction() bool {
	env := os.Getenv("APP_ENV")
	return env == "production" || env == "prod"
}

// setRefreshCookie ставит httpOnly refresh-cookie.
// prod: Secure + SameSite=Lax, host-only (Domain не задаём — риск B3).
// dev: без Secure (http://localhost).
func setRefreshCookie(c *gin.Context, token string, maxAge int) {
	secure := authIsProduction()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     authCookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     authCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   authIsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
}

// setCSRFCookie ставит НЕ-httpOnly csrf-cookie (double-submit, риск B4):
// фронт читает значение и дублирует в заголовок X-CSRF-Token.
func setCSRFCookie(c *gin.Context) string {
	val := newCSRFToken()
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     csrfCookieName,
		Value:    val,
		Path:     "/",
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
		HttpOnly: false,
		Secure:   authIsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
	return val
}

// LocalLoginRequest структура запроса для локального входа
type LocalLoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=3,max=128"`
}

// RegisterRequest структура запроса для регистрации (legacy admin endpoint)
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=10,max=128"`
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required,min=1,max=255"`
	// CompanyID из тела ИГНОРИРУЕТСЯ (риск BLK3: иначе admin создал бы
	// юзера в чужом tenant). Берётся из JWT создателя.
	CompanyID string `json:"company_id"`
	Role      string `json:"role" binding:"required"`
}

func logLocalAuthOperation(operation, username, userID, companyID string, details map[string]interface{}) {
	logData := map[string]interface{}{
		"timestamp":  time.Now().Format(time.RFC3339),
		"operation":  operation,
		"username":   username,
		"user_id":    userID,
		"company_id": companyID,
		"auth_type":  "local",
	}
	for k, v := range details {
		logData[k] = v
	}
	j, _ := json.Marshal(logData)
	log.Printf("LOCAL_AUTH_LOG: %s", string(j))
}

// LocalLogin — чистая локальная авторизация (Axenta отвязана полностью).
//
// Generic-ошибка «Неверные учётные данные» без различения
// «нет юзера / неверный пароль» (анти-энумерация). Lockout по
// FailedAttempts/LockedUntil (риск B9). Refresh — в httpOnly cookie,
// access — в теле.
func (api *LocalAuthAPI) LocalLogin(c *gin.Context) {
	var req LocalLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат запроса"})
		return
	}

	ip := c.ClientIP()
	logLocalAuthOperation("login_attempt", req.Username, "", "", map[string]interface{}{
		"ip_address": ip, "user_agent": c.GetHeader("User-Agent"),
	})

	publicDB := api.getPublicDB()
	var user models.LocalUser
	findErr := publicDB.Where("username = ?", req.Username).First(&user).Error

	// Generic-ответ при любом провале (анти-энумерация).
	deny := func(reason string) {
		logLocalAuthOperation("login_failed", req.Username, "", "", map[string]interface{}{
			"status": "failed", "reason": reason, "ip_address": ip,
		})
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Неверные учётные данные"})
	}

	if findErr != nil {
		deny("user_not_found")
		return
	}

	// Блокировка.
	if user.IsLocked() {
		logLocalAuthOperation("login_locked", req.Username, "", user.CompanyID, map[string]interface{}{
			"status": "locked", "locked_until": user.LockedUntil, "ip_address": ip,
		})
		c.JSON(http.StatusTooManyRequests, gin.H{
			"status": "error",
			"error":  "Аккаунт временно заблокирован из-за неудачных попыток входа",
		})
		return
	}

	if !user.IsActive {
		deny("inactive")
		return
	}

	if !user.CheckPassword(req.Password) {
		// Инкремент счётчика, при достижении порога — блок.
		updates := map[string]interface{}{"failed_attempts": user.FailedAttempts + 1}
		if user.FailedAttempts+1 >= lockThreshold {
			until := time.Now().Add(lockDuration)
			updates["locked_until"] = until
			updates["failed_attempts"] = 0
		}
		publicDB.Model(&models.LocalUser{}).Where("id = ?", user.ID).Updates(updates)
		deny("bad_password")
		return
	}

	// Успех — сброс счётчика/блокировки.
	publicDB.Model(&models.LocalUser{}).Where("id = ?", user.ID).Updates(map[string]interface{}{
		"failed_attempts": 0, "locked_until": nil,
	})

	accessToken, refreshToken, err := api.jwtService.GenerateTokenPair(&user)
	if err != nil {
		logLocalAuthOperation("login_token_error", req.Username, "", user.CompanyID, map[string]interface{}{
			"status": "failed", "error": err.Error(),
		})
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка генерации токенов"})
		return
	}

	if err := user.UpdateLastLogin(publicDB); err != nil {
		log.Printf("⚠️ last_login update failed for %d: %v", user.ID, err)
	}

	setRefreshCookie(c, refreshToken, int(7*24*time.Hour.Seconds()))
	csrf := setCSRFCookie(c)

	logLocalAuthOperation("login_success", req.Username, itoa(user.ID), user.CompanyID, map[string]interface{}{
		"status": "success", "role": user.Role,
	})

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   api.jwtService.AccessTTLSeconds(),
			"csrf_token":   csrf,
			"user":         user.ToPublicUser(),
		},
	})
}

// RefreshToken — ротация по refresh-cookie (риск B1).
// При reuse-detection: 401 + чистка cookie (вся семья отозвана сервисом).
func (api *LocalAuthAPI) RefreshToken(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil || raw == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Нет refresh-токена"})
		return
	}

	newAccess, newRefresh, rotErr := api.jwtService.RotateRefreshToken(raw)
	if rotErr != nil {
		// Гонка легитимного клиента (BLK1): семья жива, cookie НЕ
		// чистим — победившая ротация уже положила валидный преемник
		// в общий cookie. 409 → фронт не разлогинивает, ретраит.
		if errors.Is(rotErr, services.ErrRefreshRace) {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Параллельное обновление сессии, повторите запрос",
			})
			return
		}
		clearRefreshCookie(c)
		if errors.Is(rotErr, services.ErrRefreshReuse) {
			logLocalAuthOperation("refresh_reuse_detected", "", "", "", map[string]interface{}{
				"status": "family_revoked", "ip_address": c.ClientIP(),
			})
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": "error",
				"error":  "Сессия скомпрометирована, требуется повторный вход",
			})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Неверный или истёкший refresh-токен"})
		return
	}

	setRefreshCookie(c, newRefresh, int(7*24*time.Hour.Seconds()))
	csrf := setCSRFCookie(c)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"access_token": newAccess,
			"token_type":   "Bearer",
			"expires_in":   api.jwtService.AccessTTLSeconds(),
			"csrf_token":   csrf,
		},
	})
}

// LocalLogout отзывает всю семью refresh-токена и чистит cookie.
func (api *LocalAuthAPI) LocalLogout(c *gin.Context) {
	if raw, err := c.Cookie(refreshCookieName); err == nil && raw != "" {
		if err := api.jwtService.RevokeRefreshToken(raw); err != nil {
			log.Printf("⚠️ logout revoke failed: %v", err)
		}
	}
	clearRefreshCookie(c)
	if uid, ok := middleware.GetCurrentUserID(c); ok {
		logLocalAuthOperation("logout", "", itoa(uid), "", map[string]interface{}{"status": "success"})
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Logged out"})
}

// LocalCurrentUser возвращает данные текущего пользователя.
func (api *LocalAuthAPI) LocalCurrentUser(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Не авторизован"})
		return
	}
	var user models.LocalUser
	if err := api.getPublicDB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": user.ToPublicUser()})
}

// RegisterLocalUser — создание локального пользователя (legacy admin
// endpoint; полноценные invite/RBAC — Ф2).
func (api *LocalAuthAPI) RegisterLocalUser(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат запроса"})
		return
	}
	// superadmin создаётся ТОЛЬКО bootstrap'ом /api/setup (риск BLK3:
	// привилегий-эскалация через этот endpoint).
	if !models.IsValidRole(req.Role) || req.Role == models.RoleSuperadmin {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Недопустимая роль"})
		return
	}

	// CompanyID — ВСЕГДА компания создателя из подписанного JWT, не из
	// тела (риск BLK3: cross-tenant user creation). Суперадмин не
	// исключение в Ф1 — кросс-tenant провижн будет в Ф2 (invites/RBAC).
	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания создателя не определена"})
		return
	}
	if req.CompanyID != "" && req.CompanyID != callerCompany {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Создание пользователя в чужой компании запрещено",
		})
		return
	}

	publicDB := api.getPublicDB()
	var cnt int64
	publicDB.Model(&models.LocalUser{}).Where("username = ?", req.Username).Count(&cnt)
	if cnt > 0 {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "Имя пользователя уже существует"})
		return
	}

	user := models.LocalUser{
		Username:     req.Username,
		Email:        req.Email,
		Name:         req.Name,
		CompanyID:    callerCompany,
		Role:         req.Role,
		IsActive:     true,
		TokenVersion: 1,
	}
	if err := user.SetPassword(req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка хеширования пароля"})
		return
	}
	if err := publicDB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка создания пользователя"})
		return
	}

	logLocalAuthOperation("user_registered", req.Username, itoa(user.ID), req.CompanyID, map[string]interface{}{
		"status": "success", "role": req.Role,
	})
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": user.ToPublicUser()})
}

// RegisterRoutes регистрирует /local/* (публичные login/refresh/logout +
// защищённые). Главные алиасы /api/auth/{login,refresh,logout} навешивает
// main.go в cutover (Task #6).
func (api *LocalAuthAPI) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/local/login", api.LocalLogin)
	router.POST("/local/refresh", api.RefreshToken)
	router.POST("/local/logout", api.LocalLogout)

	authMiddleware := middleware.NewLocalAuthMiddleware(api.jwtService)
	protected := router.Group("")
	protected.Use(authMiddleware.RequireAuth())
	{
		protected.GET("/local/current_user", api.LocalCurrentUser)

		adminOnly := protected.Group("")
		adminOnly.Use(authMiddleware.RequireRole(models.RoleAdmin, models.RoleSuperadmin))
		{
			adminOnly.POST("/local/register", api.RegisterLocalUser)
		}
	}
}

func itoa(u uint) string {
	return strconv.FormatUint(uint64(u), 10)
}

// newCSRFToken — 256-bit криптослучайный CSRF-токен (double-submit, риск B4).
func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// CSPRNG-сбой крайне маловероятен; не отдаём предсказуемый токен.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
