package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAdvisoryLockKey — фиксированный ключ pg_advisory_xact_lock для
// сериализации bootstrap'а (риск B2: TOCTOU между проверкой «БД пуста»
// и созданием суперадмина → несколько суперадминов).
const setupAdvisoryLockKey int64 = 918273645

// SetupAPI — первичная инициализация системы (создание суперадмина).
type SetupAPI struct {
	db *gorm.DB
}

// NewSetupAPI создаёт SetupAPI.
func NewSetupAPI(db *gorm.DB) *SetupAPI {
	return &SetupAPI{db: db}
}

func (s *SetupAPI) publicDB() *gorm.DB {
	db := s.db.Session(&gorm.Session{})
	if err := db.Exec("SET search_path TO public").Error; err != nil {
		log.Printf("⚠️ SetupAPI: не удалось переключиться на public: %v", err)
	}
	return db
}

func setupIsProduction() bool {
	env := os.Getenv("APP_ENV")
	return env == "production" || env == "prod"
}

// setupTokenRequired сообщает, требуется ли SETUP_TOKEN.
// В production SETUP_TOKEN обязателен: пустой → endpoint мёртв (риск:
// публичный bootstrap на проде можно перехватить до владельца).
func setupToken() (token string, mandatory bool) {
	return os.Getenv("SETUP_TOKEN"), setupIsProduction()
}

// SetupStatusRequest/Response — GET /api/setup: фронт узнаёт, показывать
// ли экран первичной настройки.
func (s *SetupAPI) Status(c *gin.Context) {
	var cnt int64
	if err := s.publicDB().Model(&models.BootstrapState{}).Count(&cnt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "db error"})
		return
	}
	tok, mandatory := setupToken()
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"initialized":          cnt > 0,
			"setup_token_required": mandatory && tok != "",
			// Прод без SETUP_TOKEN и без инициализации — endpoint мёртв.
			"setup_disabled": cnt == 0 && mandatory && tok == "",
		},
	})
}

// SetupRequest — тело POST /api/setup.
type SetupRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Password    string `json:"password" binding:"required,min=10,max=128"`
	Email       string `json:"email" binding:"required,email"`
	Name        string `json:"name" binding:"required,min=1,max=255"`
	CompanyName string `json:"company_name" binding:"omitempty,max=100"`
	SetupToken  string `json:"setup_token"`
}

// Bootstrap создаёт первого пользователя — суперадмина — и root-tenant.
//
// Атомарность (риск B2): pg_advisory_xact_lock сериализует параллельные
// запросы; singleton-таблица bootstrap_state с UNIQUE по Singleton —
// жёсткая гарантия «ровно один» даже без лока. Повторный вызов → 410.
func (s *SetupAPI) Bootstrap(c *gin.Context) {
	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат запроса: " + err.Error()})
		return
	}

	// SETUP_TOKEN: обязателен в production.
	tok, mandatory := setupToken()
	if mandatory && tok == "" {
		// Прод без токена — endpoint не существует.
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "not found"})
		return
	}
	if tok != "" {
		if subtle.ConstantTimeCompare([]byte(req.SetupToken), []byte(tok)) != 1 {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "error": "Неверный setup token"})
			return
		}
	}

	companyName := req.CompanyName
	if companyName == "" {
		companyName = "ACRM"
	}

	var (
		createdUserID  uint
		createdCompany models.Company
	)

	txErr := s.publicDB().Transaction(func(tx *gorm.DB) error {
		// Сериализуем bootstrap на уровне БД.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", setupAdvisoryLockKey).Error; err != nil {
			return fmt.Errorf("advisory lock failed: %w", err)
		}

		// Уже инициализировано? → 410 (обрабатываем после транзакции).
		var bsCount int64
		if err := tx.Model(&models.BootstrapState{}).Count(&bsCount).Error; err != nil {
			return fmt.Errorf("bootstrap_state count failed: %w", err)
		}
		if bsCount > 0 {
			return errAlreadyInitialized
		}

		// Уникальность username.
		var uCount int64
		if err := tx.Model(&models.LocalUser{}).Where("username = ?", req.Username).Count(&uCount).Error; err != nil {
			return fmt.Errorf("username check failed: %w", err)
		}
		if uCount > 0 {
			return errUsernameTaken
		}

		// Root-компания. DatabaseSchema NOT NULL+unique — ставим
		// временный уникальный плейсхолдер, после INSERT → tenant_<id>.
		rnd := make([]byte, 6)
		_, _ = rand.Read(rnd)
		// Domain тоже под uniqueIndex: пустая строка коллизится с уже
		// существующими companies.domain='' (PG: '' не как NULL).
		// Ставим уникальный плейсхолдер — domain-резолв тенанта при
		// local-auth не используется (tenant из JWT-claim).
		rndHex := hex.EncodeToString(rnd)
		company := models.Company{
			Name:           companyName,
			DatabaseSchema: "tenant_init_" + rndHex,
			Domain:         "bootstrap-" + rndHex,
			AxetnaLogin:    req.Username, // Axenta отвязана; колонка NOT NULL
			AxetnaPassword: "",
			ContactEmail:   req.Email,
			IsActive:       true,
			CompanyType:    "partner",
		}
		if err := tx.Create(&company).Error; err != nil {
			return fmt.Errorf("company create failed: %w", err)
		}
		schemaName := fmt.Sprintf("tenant_%d", company.ID)
		if err := tx.Model(&company).Update("database_schema", schemaName).Error; err != nil {
			return fmt.Errorf("company schema update failed: %w", err)
		}
		company.DatabaseSchema = schemaName

		// Суперадмин.
		su := models.LocalUser{
			Username:     req.Username,
			Email:        req.Email,
			Name:         req.Name,
			CompanyID:    strconv.FormatUint(uint64(company.ID), 10),
			Role:         models.RoleSuperadmin,
			IsSuperadmin: true,
			IsActive:     true,
			TokenVersion: 1,
		}
		if err := su.SetPassword(req.Password); err != nil {
			return fmt.Errorf("password hash failed: %w", err)
		}
		if err := tx.Create(&su).Error; err != nil {
			return fmt.Errorf("superadmin create failed: %w", err)
		}

		// Singleton-маркер. UNIQUE по Singleton=true → второй INSERT
		// упадёт, даже если advisory-лок обойдён.
		bs := models.BootstrapState{
			Singleton:        true,
			SuperadminUserID: su.ID,
			InitializedAt:    time.Now(),
		}
		if err := tx.Create(&bs).Error; err != nil {
			return fmt.Errorf("bootstrap_state create failed: %w", err)
		}

		createdUserID = su.ID
		createdCompany = company
		return nil
	})

	switch txErr {
	case nil:
		// OK
	case errAlreadyInitialized:
		c.JSON(http.StatusGone, gin.H{"status": "error", "error": "Система уже инициализирована"})
		return
	case errUsernameTaken:
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "Имя пользователя занято"})
		return
	default:
		log.Printf("❌ Bootstrap transaction failed: %v", txErr)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка инициализации"})
		return
	}

	// Схема создаётся ПОСЛЕ коммита (DDL + tenant-миграции, тяжело).
	// Риск BLK4: при сбое НЕ оставляем систему «initialized навсегда»
	// с битым root-tenant. Компенсирующий откат: сносим bootstrap_state
	// + суперадмина + компанию → /api/setup снова доступен для повтора.
	if err := database.CreateTenantSchema(createdCompany.ID, createdCompany.DatabaseSchema); err != nil {
		log.Printf("❌ Bootstrap: схема %s не создана: %v — откатываю bootstrap",
			createdCompany.DatabaseSchema, err)
		// Unscoped: hard-delete — soft-delete оставил бы строки под
		// unique-индексами (username/database_schema), и повтор /setup
		// упал бы на конфликте.
		rb := s.publicDB()
		// RB1: дропаем осиротевшую/частично-мигрированную схему, иначе
		// повтор /setup создаст tenant_N+1, а tenant_N останется мусором.
		// Имя схемы — fmt.Sprintf("tenant_%d", ID), только цифры → без
		// инъекции; CASCADE убирает частичные tenant-таблицы.
		if createdCompany.DatabaseSchema != "" {
			if e := rb.Exec(fmt.Sprintf(
				"DROP SCHEMA IF EXISTS %s CASCADE", createdCompany.DatabaseSchema)).Error; e != nil {
				log.Printf("⚠️ rollback DROP SCHEMA %s: %v", createdCompany.DatabaseSchema, e)
			}
		}
		if e := rb.Unscoped().Where("singleton = ?", true).Delete(&models.BootstrapState{}).Error; e != nil {
			log.Printf("⚠️ rollback bootstrap_state: %v", e)
		}
		if e := rb.Unscoped().Delete(&models.LocalUser{}, createdUserID).Error; e != nil {
			log.Printf("⚠️ rollback superadmin #%d: %v", createdUserID, e)
		}
		if e := rb.Unscoped().Delete(&models.Company{}, createdCompany.ID).Error; e != nil {
			log.Printf("⚠️ rollback company #%d: %v", createdCompany.ID, e)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Не удалось создать схему root-tenant, инициализация откатена — повторите /api/setup",
		})
		return
	}

	log.Printf("✅ Bootstrap: суперадмин #%d (%s) + компания #%d (%s) созданы",
		createdUserID, req.Username, createdCompany.ID, createdCompany.DatabaseSchema)

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data": gin.H{
			"superadmin_id": createdUserID,
			"company_id":    createdCompany.ID,
			"message":       "Система инициализирована. Войдите под суперадмином.",
		},
	})
}

var (
	errAlreadyInitialized = fmt.Errorf("already initialized")
	errUsernameTaken      = fmt.Errorf("username taken")
)
