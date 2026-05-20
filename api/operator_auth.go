package api

import (
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Операторский (control-plane) auth-контур. Полностью отдельный от
// tenant: своя cookie (acrm_op_*), свой путь /api/control, свой
// JWT-сервис/секрет. Ноль пересечений с Ф1.
const (
	opRefreshCookie = "acrm_op_refresh"
	opCSRFCookie    = "acrm_op_csrf"
	opCookiePath    = "/api/control"
)

const opLockThreshold = 5
const opLockDuration = 15 * time.Minute

// opSetupAdvisoryLockKey — отдельный ключ от tenant-bootstrap.
const opSetupAdvisoryLockKey int64 = 918273646

type OperatorAuthAPI struct {
	db  *gorm.DB
	jwt *services.OperatorJWTService
}

func NewOperatorAuthAPI(db *gorm.DB, jwt *services.OperatorJWTService) *OperatorAuthAPI {
	return &OperatorAuthAPI{db: db, jwt: jwt}
}

func (a *OperatorAuthAPI) publicDB() *gorm.DB {
	db := a.db.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ OperatorAuthAPI: search_path public: %v", err)
	}
	return db
}

func setOpRefreshCookie(c *gin.Context, token string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: opRefreshCookie, Value: token, Path: opCookiePath,
		MaxAge: maxAge, HttpOnly: true, Secure: authIsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
}

func clearOpRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: opRefreshCookie, Value: "", Path: opCookiePath,
		MaxAge: -1, HttpOnly: true, Secure: authIsProduction(),
		SameSite: http.SameSiteLaxMode,
	})
}

func setOpCSRFCookie(c *gin.Context) string {
	v := newCSRFToken()
	http.SetCookie(c.Writer, &http.Cookie{
		Name: opCSRFCookie, Value: v, Path: "/",
		MaxAge: int((7 * 24 * time.Hour).Seconds()), HttpOnly: false,
		Secure: authIsProduction(), SameSite: http.SameSiteLaxMode,
	})
	return v
}

func opSetupToken() (token string, mandatory bool) {
	return os.Getenv("OPERATOR_SETUP_TOKEN"), authIsProduction()
}

// --- Bootstrap первого оператора ---

type OperatorSetupRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=64"`
	Password   string `json:"password" binding:"required,min=10,max=128"`
	Email      string `json:"email" binding:"required,email"`
	Name       string `json:"name" binding:"required,min=1,max=255"`
	SetupToken string `json:"setup_token"`
}

func (a *OperatorAuthAPI) SetupStatus(c *gin.Context) {
	var cnt int64
	if err := a.publicDB().Model(&models.OperatorBootstrapState{}).Count(&cnt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "db error"})
		return
	}
	tok, mandatory := opSetupToken()
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"initialized":          cnt > 0,
		"setup_token_required": mandatory && tok != "",
		"setup_disabled":       cnt == 0 && mandatory && tok == "",
	}})
}

// SetupBootstrap — атомарно создаёт первого оператора (advisory-lock +
// singleton operator_bootstrap_state). Повтор → 410.
func (a *OperatorAuthAPI) SetupBootstrap(c *gin.Context) {
	var req OperatorSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат: " + err.Error()})
		return
	}
	tok, mandatory := opSetupToken()
	if mandatory && tok == "" {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "not found"})
		return
	}
	if tok != "" && subtle.ConstantTimeCompare([]byte(req.SetupToken), []byte(tok)) != 1 {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Неверный setup token"})
		return
	}

	var createdID uint
	txErr := a.publicDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", opSetupAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		var cnt int64
		if err := tx.Model(&models.OperatorBootstrapState{}).Count(&cnt).Error; err != nil {
			return fmt.Errorf("bootstrap count: %w", err)
		}
		if cnt > 0 {
			return errOpAlreadyInit
		}
		var uCnt int64
		if err := tx.Model(&models.Operator{}).Where("username = ?", req.Username).Count(&uCnt).Error; err != nil {
			return fmt.Errorf("username check: %w", err)
		}
		if uCnt > 0 {
			return errOpUsernameTaken
		}
		op := models.Operator{
			Username: req.Username, Email: req.Email, Name: req.Name,
			IsActive: true, TokenVersion: 1,
		}
		if err := op.SetPassword(req.Password); err != nil {
			return fmt.Errorf("hash: %w", err)
		}
		if err := tx.Create(&op).Error; err != nil {
			return fmt.Errorf("create operator: %w", err)
		}
		if err := tx.Create(&models.OperatorBootstrapState{
			Singleton: true, OperatorID: op.ID, InitializedAt: time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("bootstrap_state: %w", err)
		}
		createdID = op.ID
		return nil
	})

	switch txErr {
	case nil:
		log.Printf("✅ Operator bootstrap: #%d (%s)", createdID, req.Username)
		c.JSON(http.StatusCreated, gin.H{"status": "success", "data": gin.H{
			"operator_id": createdID,
			"message":     "Оператор создан. Войдите в control-plane.",
		}})
	case errOpAlreadyInit:
		c.JSON(http.StatusGone, gin.H{"status": "error", "error": "Оператор уже инициализирован"})
	case errOpUsernameTaken:
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "Имя занято"})
	default:
		log.Printf("❌ Operator bootstrap failed: %v", txErr)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка инициализации"})
	}
}

var (
	errOpAlreadyInit   = fmt.Errorf("operator already initialized")
	errOpUsernameTaken = fmt.Errorf("operator username taken")
)

// --- Login / Refresh / Logout / Current ---

type OperatorLoginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=3,max=128"`
}

func (a *OperatorAuthAPI) Login(c *gin.Context) {
	var req OperatorLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат"})
		return
	}
	db := a.publicDB()
	var op models.Operator
	findErr := db.Where("username = ?", req.Username).First(&op).Error
	deny := func() {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Неверные учётные данные"})
	}
	if findErr != nil {
		deny()
		return
	}
	if op.IsLocked() {
		c.JSON(http.StatusTooManyRequests, gin.H{"status": "error", "error": "Аккаунт временно заблокирован"})
		return
	}
	if !op.IsActive {
		deny()
		return
	}
	if !op.CheckPassword(req.Password) {
		upd := map[string]interface{}{"failed_attempts": op.FailedAttempts + 1}
		if op.FailedAttempts+1 >= opLockThreshold {
			until := time.Now().Add(opLockDuration)
			upd["locked_until"] = until
			upd["failed_attempts"] = 0
		}
		db.Model(&models.Operator{}).Where("id = ?", op.ID).Updates(upd)
		deny()
		return
	}
	db.Model(&models.Operator{}).Where("id = ?", op.ID).
		Updates(map[string]interface{}{"failed_attempts": 0, "locked_until": nil})

	access, refresh, err := a.jwt.GenerateTokenPair(&op)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка токенов"})
		return
	}
	now := time.Now()
	db.Model(&models.Operator{}).Where("id = ?", op.ID).Updates(map[string]interface{}{
		"last_login": now, "login_count": op.LoginCount + 1,
	})

	setOpRefreshCookie(c, refresh, int(7*24*time.Hour.Seconds()))
	csrf := setOpCSRFCookie(c)
	log.Printf("✅ Operator login: %s (#%d)", op.Username, op.ID)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"access_token": access, "token_type": "Bearer",
		"expires_in": a.jwt.AccessTTLSeconds(), "csrf_token": csrf,
		"operator": op.ToPublic(),
	}})
}

func (a *OperatorAuthAPI) Refresh(c *gin.Context) {
	raw, err := c.Cookie(opRefreshCookie)
	if err != nil || raw == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Нет refresh-токена"})
		return
	}
	access, refresh, rotErr := a.jwt.RotateRefreshToken(raw)
	if rotErr != nil {
		if errors.Is(rotErr, services.ErrRefreshRace) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "Параллельное обновление, повторите"})
			return
		}
		clearOpRefreshCookie(c)
		if errors.Is(rotErr, services.ErrRefreshReuse) {
			log.Printf("⚠️ Operator refresh reuse detected, family revoked, ip=%s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Сессия скомпрометирована"})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Неверный/истёкший refresh"})
		return
	}
	setOpRefreshCookie(c, refresh, int(7*24*time.Hour.Seconds()))
	csrf := setOpCSRFCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{
		"access_token": access, "token_type": "Bearer",
		"expires_in": a.jwt.AccessTTLSeconds(), "csrf_token": csrf,
	}})
}

func (a *OperatorAuthAPI) Logout(c *gin.Context) {
	if raw, err := c.Cookie(opRefreshCookie); err == nil && raw != "" {
		if err := a.jwt.RevokeRefreshToken(raw); err != nil {
			log.Printf("⚠️ operator logout revoke: %v", err)
		}
	}
	clearOpRefreshCookie(c)
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Logged out"})
}

func (a *OperatorAuthAPI) CurrentOperator(c *gin.Context) {
	id, ok := middleware.GetOperatorID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "error": "Не авторизован"})
		return
	}
	var op models.Operator
	if err := a.publicDB().First(&op, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Оператор не найден"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": op.ToPublic()})
}
