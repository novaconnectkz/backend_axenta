package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"backend_axenta/utils"
)

// Ф3-D #1 cred-UX (минимальный): UI/endpoint управления Axenta-кредами
// компании (Company.AxetnaLogin/AxetnaPassword). После Ф1 логин = локальный
// JWT, request-токен невалиден для axenta.cloud → весь Ф3-B/C server-токен
// (services.AxentaServerToken) держится на этих хранимых кредах. Bootstrap/
// provision оставляют их пустыми → до ввода всё degraded. Здесь оператор
// (admin/superadmin своей компании) их задаёт, тестирует и ротирует.
//
// Без новой модели/миграции: пишем в существующие колонки Company
// (пароль — utils enc:v1:). companyID СТРОГО из доверенного claim
// (middleware.GetCompanyID), НЕ из client-заголовка — кросс-тенант
// исключён; маршруты под apiGroup(/api/auth) RequireAuth+SetTenant +
// RequireRole(admin,superadmin).

// Ф3-D #1/Codex Q4: каждый Probe = прямой axenta.cloud /auth/login
// (AxetnaClient ретраит до 4× на 429/5xx). Скомпрометированная/кривая
// admin-сессия могла бы исчерпать квоту Axenta 500 req/min на тенант.
// Серверный per-company cooldown: не чаще 1 probe / axentaProbeCooldown.
const axentaProbeCooldown = 5 * time.Second

var (
	axentaProbeMu   sync.Mutex
	axentaProbeLast = map[uint]time.Time{}
)

// axentaProbeAllowed — true если для компании можно сделать probe сейчас;
// иначе false + сколько секунд ждать. companyID==0 → один общий бакет
// (claim не дал компанию, всё равно троттлим).
func axentaProbeAllowed(companyID uint) (bool, int) {
	axentaProbeMu.Lock()
	defer axentaProbeMu.Unlock()
	now := time.Now()
	if last, ok := axentaProbeLast[companyID]; ok {
		if elapsed := now.Sub(last); elapsed < axentaProbeCooldown {
			return false, int((axentaProbeCooldown - elapsed).Seconds()) + 1
		}
	}
	axentaProbeLast[companyID] = now
	return true, 0
}

type axentaCredsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// loadCompanyFromMainPool читает Company из main-пула БЕЗ мутации
// search_path (главный пул всегда public, долг #15) — Company глобальна.
func loadCompanyFromMainPool(companyID uint) (*models.Company, error) {
	if database.DB == nil {
		return nil, errors.New("main DB pool nil")
	}
	var company models.Company
	if err := database.DB.Session(&gorm.Session{}).First(&company, companyID).Error; err != nil {
		return nil, err
	}
	return &company, nil
}

// axentaCredsRequireAdmin — задавать/смотреть креды интеграции может только
// admin/superadmin компании (роль из доверенного JWT-claim). Установка
// Axenta-кред = высокопривилегированное действие (перепривязка интеграции
// всей компании) — не давать любому аутентифицированному tenant-юзеру.
func axentaCredsRequireAdmin(c *gin.Context) bool {
	role, _ := middleware.GetCurrentUserRole(c)
	if role == models.RoleAdmin || role == models.RoleSuperadmin {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Требуется роль администратора"})
	return false
}

// GetAxentaCredentials — статус Axenta-подключения компании.
// GET /api/auth/axenta-credentials
// Возвращает login (не секрет) и configured (заданы ли логин+пароль).
// Пароль НИКОГДА не возвращается.
func GetAxentaCredentials(c *gin.Context) {
	if !axentaCredsRequireAdmin(c) {
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":   "error",
			"degraded": true,
			"error":    "Компания не определена в запросе",
		})
		return
	}
	company, err := loadCompanyFromMainPool(companyID)
	if err != nil {
		// Не 5xx: фронт покажет «не настроено / недоступно».
		c.JSON(http.StatusOK, gin.H{
			"status":   "error",
			"degraded": true,
			"error":    "Не удалось загрузить компанию",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"login":      company.AxetnaLogin,
			"configured": company.AxetnaLogin != "" && company.AxetnaPassword != "",
		},
	})
}

// TestAxentaCredentials — проверка пары login/password против axenta.cloud
// БЕЗ сохранения. POST /api/auth/axenta-credentials/test
func TestAxentaCredentials(c *gin.Context) {
	if !axentaCredsRequireAdmin(c) {
		return
	}
	var req axentaCredsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Некорректное тело запроса"})
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "login и password обязательны"})
		return
	}
	if axentaServerToken == nil {
		c.JSON(http.StatusOK, gin.H{"status": "error", "degraded": true, "error": "Axenta server-token не инициализирован"})
		return
	}
	// Q4: per-company cooldown — этот endpoint = чистый probe-вектор.
	if ok, retryAfter := axentaProbeAllowed(middleware.GetCompanyID(c)); !ok {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		c.JSON(http.StatusTooManyRequests, gin.H{
			"status": "error",
			"error":  "Слишком часто. Повторите проверку позже.",
		})
		return
	}
	if err := axentaServerToken.Probe(c.Request.Context(), req.Login, req.Password); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   gin.H{"ok": false, "error": axentaCredsErrMsg(err)},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   gin.H{"ok": true},
	})
}

// UpdateAxentaCredentials — сохранить/ротировать Axenta-креды компании.
// PUT /api/auth/axenta-credentials
// Пароль шифруется (enc:v1:), кэш server-токена сбрасывается, затем
// выполняется live-probe новыми кредами; результат возвращается.
func UpdateAxentaCredentials(c *gin.Context) {
	if !axentaCredsRequireAdmin(c) {
		return
	}
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "error", "degraded": true, "error": "Компания не определена в запросе"})
		return
	}
	var req axentaCredsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Некорректное тело запроса"})
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "login и password обязательны"})
		return
	}
	if len(req.Login) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "login слишком длинный (max 100)"})
		return
	}
	// Q2: utils.EncryptString идемпотентна по префиксу enc:v1: — если
	// оператор случайно вставит ранее скопированный encrypted-blob, он
	// сохранился бы В ОТКРЫТОМ виде (EncryptString вернёт его как есть),
	// и последующий DecryptString сломал бы всю Axenta-синхронизацию.
	// Реальные Axenta-пароли с enc:v1: не начинаются → честно reject.
	if utils.IsEncrypted(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Пароль не может начинаться с enc:v1: (похоже на вставленный зашифрованный блок)",
		})
		return
	}

	// Шифруем plaintext-пароль пользователя.
	// Q6: 5xx ниже (шифрование/DB) — НАМЕРЕННО вне Ф3-B no-5xx контракта.
	// Ф3-B про degradation read-path после Ф1 (не отдавать 401/500 из-за
	// мёртвого proxy). Здесь — config-write; реальный сбой шифрования/БД =
	// честный server error, маскировать его 200-«ок» хуже (оператор решит
	// что сохранилось, а креды не записаны).
	encPassword, err := utils.EncryptString(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка шифрования пароля"})
		return
	}

	if database.DB == nil {
		c.JSON(http.StatusOK, gin.H{"status": "error", "degraded": true, "error": "main DB pool недоступен"})
		return
	}
	// Company глобальна → main-пул, без мутации search_path (долг #15).
	res := database.DB.Session(&gorm.Session{}).
		Model(&models.Company{}).
		Where("id = ?", companyID).
		Updates(map[string]interface{}{
			"axetna_login":    req.Login,
			"axetna_password": encPassword,
		})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка сохранения кред"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "Компания не найдена"})
		return
	}

	// Сбрасываем кэш server-токена → следующий вызов возьмёт новые креды.
	if axentaServerToken != nil {
		axentaServerToken.Invalidate(companyID)
	}

	// Live-probe новыми кредами: пользователю сразу видно, рабочие ли они.
	// Save+Invalidate уже выполнены безусловно — троттлим ТОЛЬКО probe
	// (Q4): при cooldown не ходим в axenta.cloud, но креды сохранены.
	probeOK := false
	var probeErr string
	if axentaServerToken != nil {
		if ok, retryAfter := axentaProbeAllowed(companyID); !ok {
			probeErr = "проверка соединения пропущена (слишком часто, ~" + strconv.Itoa(retryAfter) + "с) — креды сохранены"
		} else if e := axentaServerToken.Probe(c.Request.Context(), req.Login, req.Password); e != nil {
			probeErr = axentaCredsErrMsg(e)
		} else {
			probeOK = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"configured":   true,
			"connectionOk": probeOK,
			"error":        probeErr,
		},
	})
}

// axentaCredsErrMsg — человекочитаемая причина без утечки внутренних деталей.
func axentaCredsErrMsg(err error) string {
	if errors.Is(err, services.ErrNoAxentaCreds) {
		return "Логин или пароль пустой"
	}
	return "Axenta отклонил креды или недоступен"
}
