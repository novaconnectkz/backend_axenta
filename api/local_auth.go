package api

import (
	"backend_axenta/audit"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// RegisterRequest структура запроса для регистрации (admin endpoint).
// Password больше НЕ принимается — BE генерит автоматически и шлёт юзеру
// email с креденшалами (см. RegisterLocalUser). Юзер сам меняет пароль
// через change-password endpoint после первого входа.
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required,min=1,max=255"`
	// CompanyID из тела ИГНОРИРУЕТСЯ (риск BLK3: иначе admin создал бы
	// юзера в чужом tenant). Берётся из JWT создателя.
	CompanyID string `json:"company_id"`
	Role      string `json:"role" binding:"required"`
}

// localUserAudit — пишет событие в audit_logs. action: 'local_user.<event>'.
// target_user_id + target_username в details чтобы UI мог фильтровать
// по конкретному юзеру (GET /local/users/:id/history).
func localUserAudit(c *gin.Context, action string, targetID uint, targetUsername string, success bool, details map[string]interface{}) {
	if details == nil {
		details = map[string]interface{}{}
	}
	details["target_user_id"] = targetID
	details["target_username"] = targetUsername
	full := "local_user." + action
	if c != nil {
		if success {
			audit.LogSuccess(c, full, details)
		} else {
			audit.LogFromContext(c, full, details)
		}
	} else {
		audit.Log("", full, details)
	}
}

// generateTempPassword — крипто-стойкий временный пароль (12 символов,
// a-zA-Z0-9). Длины достаточно для anti-bruteforce + ограничения 10+
// модели LocalUser.
func generateTempPassword() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out), nil
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

	// Генерируем временный пароль (юзер сменит через change-password).
	tempPassword, err := generateTempPassword()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка генерации пароля"})
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
	if err := user.SetPassword(tempPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка хеширования пароля"})
		return
	}
	if err := publicDB.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка создания пользователя"})
		return
	}

	localUserAudit(c, "created", user.ID, user.Username, true, map[string]interface{}{
		"role":  req.Role,
		"email": req.Email,
	})

	// Шлём welcome email с креденшалами. Если SMTP не настроен или письмо
	// не ушло — пользователь создан, но в response пишем emailed=false,
	// чтобы админ увидел проблему и мог переотправить через UI.
	emailed := true
	emailErr := ""
	if smtpErr := sendWelcomeEmail(publicDB, req.Email, req.Name, req.Username, tempPassword); smtpErr != nil {
		emailed = false
		emailErr = smtpErr.Error()
		log.Printf("⚠️ welcome email failed for %s: %v", req.Username, smtpErr)
		localUserAudit(c, "welcome_email_failed", user.ID, user.Username, false, map[string]interface{}{
			"email": req.Email,
			"error": emailErr,
		})
	} else {
		localUserAudit(c, "welcome_email_sent", user.ID, user.Username, true, map[string]interface{}{
			"email": req.Email,
		})
	}

	logLocalAuthOperation("user_registered", req.Username, itoa(user.ID), req.CompanyID, map[string]interface{}{
		"status": "success", "role": req.Role, "emailed": emailed,
	})
	resp := gin.H{
		"status":  "success",
		"data":    user.ToPublicUser(),
		"emailed": emailed,
	}
	if !emailed {
		resp["email_error"] = emailErr
	}
	c.JSON(http.StatusCreated, resp)
}

// sendWelcomeEmail — отправляет приветственное письмо с креденшалами
// новому local-user'у через operator-SMTP. caller — RegisterLocalUser.
func sendWelcomeEmail(db *gorm.DB, to, name, username, tempPassword string) error {
	subject := "Добро пожаловать в ACRM — данные для входа"
	body := fmt.Sprintf(`<html><body style="font-family:-apple-system,Segoe UI,sans-serif;color:#222">
<h2>Добро пожаловать в ACRM</h2>
<p>Здравствуйте, %s!</p>
<p>Для вас создана учётная запись в системе. Данные для входа:</p>
<table style="border-collapse:collapse;margin:16px 0">
  <tr><td style="padding:6px 14px 6px 0"><b>Логин:</b></td><td><code style="background:#f5f5f7;padding:4px 8px;border-radius:4px">%s</code></td></tr>
  <tr><td style="padding:6px 14px 6px 0"><b>Временный пароль:</b></td><td><code style="background:#f5f5f7;padding:4px 8px;border-radius:4px">%s</code></td></tr>
</table>
<p><a href="https://acrm.su/login" style="display:inline-block;background:#1f6feb;color:#fff;padding:10px 18px;border-radius:6px;text-decoration:none">Войти в ACRM</a></p>
<p style="color:#555;font-size:14px">После входа смените пароль в разделе «Профиль» — это нужно для безопасности.</p>
<hr style="border:none;border-top:1px solid #eee;margin:20px 0">
<p style="color:#888;font-size:12px">Если вы не запрашивали учётную запись — проигнорируйте письмо.</p>
</body></html>`, escapeHTML(name), escapeHTML(username), escapeHTML(tempPassword))

	return SendSystemEmail(db, to, subject, body)
}

// escapeHTML — минимальный escape для HTML-вставки (логин/имя/пароль
// в письме). Полный html/template overkill для одного шаблона.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

// ListLocalUsers — список локальных пользователей текущей компании
// (admin+superadmin). Без password_hash в ответе.
func (api *LocalAuthAPI) ListLocalUsers(c *gin.Context) {
	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания создателя не определена"})
		return
	}

	publicDB := api.getPublicDB()
	var users []models.LocalUser
	// Cross-tenant запрет: видим ТОЛЬКО юзеров своей компании. Superadmin
	// в Ф1 не исключение (cross-tenant — Ф2 invites/RBAC).
	if err := publicDB.Where("company_id = ?", callerCompany).Order("id ASC").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка получения пользователей"})
		return
	}

	out := make([]gin.H, 0, len(users))
	for _, u := range users {
		out = append(out, u.ToPublicUser())
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": out})
}

// DeleteLocalUser — удаление локального пользователя (superadmin only).
// superadmin не может удалить сам себя (риск самоблокировки админа).
func (api *LocalAuthAPI) DeleteLocalUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный ID"})
		return
	}

	callerID, _ := middleware.GetCurrentUserID(c)
	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания создателя не определена"})
		return
	}
	if uint64(callerID) == id {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Нельзя удалить себя"})
		return
	}

	publicDB := api.getPublicDB()
	var target models.LocalUser
	if err := publicDB.Where("id = ? AND company_id = ?", id, callerCompany).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}

	// Суперадмин неудалим — система гарантирует ровно одного (bootstrap
	// singleton). Удаление = блокировка доступа без возможности
	// восстановить. Edit/reset-password — допустимо.
	if target.Role == models.RoleSuperadmin || target.IsSuperadmin {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Суперадмина нельзя удалить (см. правило: 1 суперадмин, только редактирование)"})
		return
	}

	// Cleanup refresh_tokens (cascade-aware). DELETE local_users — затем
	// никаких orphan refresh-токенов не остаётся.
	if err := publicDB.Exec("DELETE FROM public.refresh_tokens WHERE user_id = ?", target.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка очистки refresh_tokens"})
		return
	}
	if err := publicDB.Unscoped().Delete(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка удаления пользователя"})
		return
	}

	localUserAudit(c, "deleted", target.ID, target.Username, true, map[string]interface{}{
		"role": target.Role,
	})
	logLocalAuthOperation("user_deleted", target.Username, itoa(target.ID), callerCompany, map[string]interface{}{
		"status": "success", "deleted_by": itoa(callerID),
	})
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"deleted_id": target.ID}})
}

// UpdateLocalUserRequest — частичный апдейт локального пользователя.
// Меняем ТОЛЬКО безопасные поля: email/name/is_active. username/role/
// company_id/password — отдельными endpoint-ами (reset-password) либо
// неизменяемы (username/role/company — chain-of-trust из bootstrap).
type UpdateLocalUserRequest struct {
	Email    *string `json:"email"`
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// UpdateLocalUser — partial update локального пользователя
// (admin+superadmin). superadmin может редактировать самого себя.
// Деактивация superadmin запрещена (риск самоблокировки системы).
func (api *LocalAuthAPI) UpdateLocalUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный ID"})
		return
	}

	var req UpdateLocalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат запроса"})
		return
	}

	callerID, _ := middleware.GetCurrentUserID(c)
	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания не определена"})
		return
	}

	publicDB := api.getPublicDB()
	var target models.LocalUser
	if err := publicDB.Where("id = ? AND company_id = ?", id, callerCompany).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}

	updates := map[string]interface{}{}
	if req.Email != nil {
		// Минимальная валидация: пустую строку или невалидный email отвергаем.
		em := strings.TrimSpace(*req.Email)
		if em == "" || !strings.Contains(em, "@") {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Невалидный email"})
			return
		}
		updates["email"] = em
	}
	if req.Name != nil {
		nm := strings.TrimSpace(*req.Name)
		if nm == "" {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Имя не может быть пустым"})
			return
		}
		updates["name"] = nm
	}
	if req.IsActive != nil {
		// superadmin'а нельзя деактивировать (риск самоблокировки системы).
		if !*req.IsActive && (target.Role == models.RoleSuperadmin || target.IsSuperadmin) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Суперадмина нельзя деактивировать"})
			return
		}
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Нет полей для обновления"})
		return
	}

	if err := publicDB.Model(&target).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка обновления"})
		return
	}

	// Перечитываем для актуального ToPublicUser
	if err := publicDB.Where("id = ?", target.ID).First(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка чтения после обновления"})
		return
	}

	updatedFields := keys(updates)
	localUserAudit(c, "updated", target.ID, target.Username, true, map[string]interface{}{
		"fields": updatedFields,
	})
	logLocalAuthOperation("user_updated", target.Username, itoa(target.ID), callerCompany, map[string]interface{}{
		"status": "success", "updated_by": itoa(callerID), "fields": fmt.Sprintf("%v", updatedFields),
	})
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": target.ToPublicUser()})
}

// keys возвращает ключи map (для логирования).
func keys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// GetLocalUserHistory — таймлайн событий по конкретному local-юзеру.
// Берётся из общей таблицы audit_logs где action LIKE 'local_user.%'
// AND details->>'target_user_id' = :id. admin+superadmin only.
func (api *LocalAuthAPI) GetLocalUserHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный ID"})
		return
	}

	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания не определена"})
		return
	}

	publicDB := api.getPublicDB()
	// Проверка существования юзера + cross-tenant защита.
	var target models.LocalUser
	if err := publicDB.Where("id = ? AND company_id = ?", id, callerCompany).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}

	var rows []audit.AuditLog
	// PostgreSQL JSONB: details::jsonb->>'target_user_id' дает строку.
	if err := publicDB.Where("action LIKE ? AND (details::jsonb->>'target_user_id')::int = ?", "local_user.%", id).
		Order("timestamp DESC").Limit(200).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка чтения истории"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": rows})
}

// ResetLocalUserPasswordRequest — тело POST /local/users/:id/reset-password.
type ResetLocalUserPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=10,max=128"`
}

// ResetLocalUserPassword — сброс пароля локального пользователя
// (superadmin only). Инвалидирует все JWT через инкремент token_version.
func (api *LocalAuthAPI) ResetLocalUserPassword(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный ID"})
		return
	}

	var req ResetLocalUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Пароль минимум 10 символов"})
		return
	}

	callerID, _ := middleware.GetCurrentUserID(c)
	callerCompany, ok := middleware.GetCurrentCompanyID(c)
	if !ok || callerCompany == "" {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Компания создателя не определена"})
		return
	}

	publicDB := api.getPublicDB()
	var target models.LocalUser
	if err := publicDB.Where("id = ? AND company_id = ?", id, callerCompany).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}

	if err := target.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка хеширования пароля"})
		return
	}
	// token_version+1 — все existing JWT с прежним claim становятся невалидными.
	target.TokenVersion = target.TokenVersion + 1
	if err := publicDB.Save(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка сохранения пароля"})
		return
	}
	// Все refresh-токены тоже выкидываем.
	publicDB.Exec("DELETE FROM public.refresh_tokens WHERE user_id = ?", target.ID)

	localUserAudit(c, "password_reset_by_admin", target.ID, target.Username, true, map[string]interface{}{
		"reset_by_id": callerID,
	})
	logLocalAuthOperation("password_reset", target.Username, itoa(target.ID), callerCompany, map[string]interface{}{
		"status": "success", "reset_by": itoa(callerID),
	})
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"user_id": target.ID, "token_version": target.TokenVersion}})
}

// ChangePasswordRequest — тело POST /api/auth/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=10,max=128"`
}

// ChangePassword — смена пароля авторизованным юзером. Требует current
// для подтверждения, чтобы XSS-токен не мог перезаписать без знания
// текущего пароля. Все JWT инвалидируются (token_version+1) — все сессии
// кроме текущей (тоже мёртвая по факту, юзеру redirect на login).
func (api *LocalAuthAPI) ChangePassword(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Не авторизован"})
		return
	}

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Новый пароль минимум 10 символов"})
		return
	}

	publicDB := api.getPublicDB()
	var user models.LocalUser
	if err := publicDB.Where("id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}

	if !user.CheckPassword(req.CurrentPassword) {
		logLocalAuthOperation("change_password_bad_current", user.Username, itoa(user.ID), user.CompanyID, map[string]interface{}{
			"status": "failed",
		})
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Неверный текущий пароль"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка хеширования"})
		return
	}
	user.TokenVersion = user.TokenVersion + 1
	if err := publicDB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка сохранения"})
		return
	}
	// Очищаем refresh-токены — все сессии этого юзера умирают.
	publicDB.Exec("DELETE FROM public.refresh_tokens WHERE user_id = ?", user.ID)

	localUserAudit(c, "password_changed", user.ID, user.Username, true, nil)
	logLocalAuthOperation("password_changed", user.Username, itoa(user.ID), user.CompanyID, map[string]interface{}{
		"status": "success",
	})
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"user_id": user.ID}})
}

// ForgotPasswordRequest — тело POST /api/auth/forgot-password.
type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ForgotPassword — anti-enumeration: всегда 200, чтобы атакующий не
// мог проверить какие email зарегистрированы. Если email найден —
// создаём single-use токен (TTL 1 час) и шлём reset-ссылку. Иначе тихий
// no-op.
func (api *LocalAuthAPI) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 200 даже на bad input — anti-enum
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если email зарегистрирован — письмо отправлено"})
		return
	}

	publicDB := api.getPublicDB()
	var user models.LocalUser
	err := publicDB.Where("LOWER(email) = LOWER(?)", strings.TrimSpace(req.Email)).First(&user).Error
	if err == nil && user.IsActive {
		// Генерим plaintext-токен (32 байт = 64 hex). В БД храним sha256-hex.
		rawBytes := make([]byte, 32)
		if _, rerr := rand.Read(rawBytes); rerr != nil {
			// Без CSPRNG не делаем reset
			log.Printf("⚠️ forgot-password: rand.Read failed: %v", rerr)
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если email зарегистрирован — письмо отправлено"})
			return
		}
		token := hex.EncodeToString(rawBytes)
		sum := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(sum[:])

		rec := models.LocalPasswordResetToken{
			UserID:    user.ID,
			TokenHash: tokenHash,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		if cerr := publicDB.Create(&rec).Error; cerr != nil {
			log.Printf("⚠️ forgot-password: db create token failed: %v", cerr)
			c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если email зарегистрирован — письмо отправлено"})
			return
		}

		// Шлём email со ссылкой. Best-effort: ошибка не блокирует ответ.
		resetURL := fmt.Sprintf("https://acrm.su/reset-password/%s", token)
		subject := "Сброс пароля ACRM"
		body := fmt.Sprintf(`<html><body style="font-family:-apple-system,Segoe UI,sans-serif;color:#222">
<h2>Сброс пароля</h2>
<p>Здравствуйте, %s!</p>
<p>Для сброса пароля учётной записи <code style="background:#f5f5f7;padding:2px 6px;border-radius:4px">%s</code> перейдите по ссылке (действует 1 час):</p>
<p><a href="%s" style="display:inline-block;background:#1f6feb;color:#fff;padding:10px 18px;border-radius:6px;text-decoration:none">Установить новый пароль</a></p>
<p style="color:#555;font-size:14px">Если ссылка не открывается — скопируйте в браузер:<br><code style="font-size:11px;color:#888">%s</code></p>
<hr style="border:none;border-top:1px solid #eee;margin:20px 0">
<p style="color:#888;font-size:12px">Если вы не запрашивали сброс — проигнорируйте письмо.</p>
</body></html>`, escapeHTML(user.Name), escapeHTML(user.Username), resetURL, resetURL)

		emailedOk := true
		emailErr := ""
		if smtpErr := SendSystemEmail(publicDB, user.Email, subject, body); smtpErr != nil {
			emailedOk = false
			emailErr = smtpErr.Error()
			log.Printf("⚠️ forgot-password: email send failed for %s: %v", user.Email, smtpErr)
		}

		auditDetails := map[string]interface{}{"email": user.Email}
		if !emailedOk {
			auditDetails["error"] = emailErr
			localUserAudit(c, "reset_email_failed", user.ID, user.Username, false, auditDetails)
		} else {
			localUserAudit(c, "reset_requested", user.ID, user.Username, true, auditDetails)
		}

		logLocalAuthOperation("password_reset_requested", user.Username, itoa(user.ID), user.CompanyID, map[string]interface{}{
			"status": "success",
		})
	}

	// Всегда 200 — anti-enumeration.
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Если email зарегистрирован — письмо отправлено"})
}

// ResetByTokenRequest — тело POST /api/auth/reset-password-by-token.
type ResetByTokenRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=10,max=128"`
}

// ResetPasswordByToken — финализация forgot-flow. Single-use токен;
// после использования помечается used_at. Инвалидирует все JWT юзера
// (token_version+1) + удаляет refresh_tokens.
func (api *LocalAuthAPI) ResetPasswordByToken(c *gin.Context) {
	var req ResetByTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Новый пароль минимум 10 символов"})
		return
	}

	sum := sha256.Sum256([]byte(req.Token))
	tokenHash := hex.EncodeToString(sum[:])

	publicDB := api.getPublicDB()
	var rec models.LocalPasswordResetToken
	if err := publicDB.Where("token_hash = ?", tokenHash).First(&rec).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Ссылка недействительна"})
		return
	}
	if rec.UsedAt != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Ссылка уже использована"})
		return
	}
	if time.Now().After(rec.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Срок действия ссылки истёк"})
		return
	}

	var user models.LocalUser
	if err := publicDB.Where("id = ?", rec.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Пользователь не найден"})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Учётная запись отключена"})
		return
	}

	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка хеширования"})
		return
	}
	user.TokenVersion = user.TokenVersion + 1
	if err := publicDB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка сохранения"})
		return
	}
	publicDB.Exec("DELETE FROM public.refresh_tokens WHERE user_id = ?", user.ID)

	now := time.Now()
	rec.UsedAt = &now
	publicDB.Save(&rec)

	localUserAudit(c, "reset_completed", user.ID, user.Username, true, nil)
	logLocalAuthOperation("password_reset_completed", user.Username, itoa(user.ID), user.CompanyID, map[string]interface{}{
		"status": "success",
	})
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"user_id": user.ID}})
}

// RegisterRoutes регистрирует /local/* (публичные login/refresh/logout +
// защищённые). Главные алиасы /api/auth/{login,refresh,logout} навешивает
// main.go в cutover (Task #6).
func (api *LocalAuthAPI) RegisterRoutes(router *gin.RouterGroup) {
	router.POST("/local/login", api.LocalLogin)
	router.POST("/local/refresh", api.RefreshToken)
	router.POST("/local/logout", api.LocalLogout)

	// Публичные endpoints для забытого пароля. anti-enumeration в
	// ForgotPassword (всегда 200). Rate-limit добавляется middleware на
	// main.go-уровне (см. там).
	router.POST("/local/forgot-password", api.ForgotPassword)
	router.POST("/local/reset-password-by-token", api.ResetPasswordByToken)

	authMiddleware := middleware.NewLocalAuthMiddleware(api.jwtService)
	protected := router.Group("")
	protected.Use(authMiddleware.RequireAuth())
	{
		protected.GET("/local/current_user", api.LocalCurrentUser)
		// Юзер сам меняет свой пароль.
		protected.POST("/local/change-password", api.ChangePassword)

		adminOnly := protected.Group("")
		adminOnly.Use(authMiddleware.RequireRole(models.RoleAdmin, models.RoleSuperadmin))
		{
			adminOnly.POST("/local/register", api.RegisterLocalUser)
			adminOnly.GET("/local/users", api.ListLocalUsers)
			adminOnly.GET("/local/users/:id/history", api.GetLocalUserHistory)
			// Edit разрешён admin+superadmin (но изменить можно только
			// email/name/is_active; роль/username/company_id — иммутабельны).
			adminOnly.PATCH("/local/users/:id", api.UpdateLocalUser)
		}

		// Delete + reset-password — superadmin only (риск массовой
		// инвалидации сессий + удаления коллег). superadmin-target
		// заблокирован на handler-level (1 superadmin, неудалим).
		superadminOnly := protected.Group("")
		superadminOnly.Use(authMiddleware.RequireRole(models.RoleSuperadmin))
		{
			superadminOnly.DELETE("/local/users/:id", api.DeleteLocalUser)
			superadminOnly.POST("/local/users/:id/reset-password", api.ResetLocalUserPassword)
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
