package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"backend_axenta/models"
	"backend_axenta/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// OperatorSMTPAPI — control-plane CRUD для глобального SMTP-конфига.
// Используется для системных email'ов (forgot-password / invite /
// notifications оператору), независимо от tenant-Email-integration.
type OperatorSMTPAPI struct {
	db *gorm.DB
}

// NewOperatorSMTPAPI — конструктор.
func NewOperatorSMTPAPI(db *gorm.DB) *OperatorSMTPAPI {
	return &OperatorSMTPAPI{db: db}
}

// RegisterRoutes монтируется в /api/control/smtp.
func (api *OperatorSMTPAPI) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/smtp", api.Get)
	g.PUT("/smtp", api.Update)
	g.POST("/smtp/test", api.Test)
}

// loadConfig читает singleton-строку (создаёт если нет).
func (api *OperatorSMTPAPI) loadConfig() (*models.OperatorSMTPConfig, error) {
	var cfg models.OperatorSMTPConfig
	if err := api.db.Where("singleton = ?", true).First(&cfg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			cfg = models.OperatorSMTPConfig{Singleton: true, Port: 587, UseTLS: true}
			if cerr := api.db.Create(&cfg).Error; cerr != nil {
				return nil, cerr
			}
			return &cfg, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// Get — GET /api/control/smtp. Пароль наружу НЕ отдаётся; флаг has_password
// показывает наличие установленного пароля.
func (api *OperatorSMTPAPI) Get(c *gin.Context) {
	cfg, err := api.loadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка загрузки конфига"})
		return
	}
	cfg.HasPassword = cfg.PasswordEnc != ""
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": cfg})
}

// UpdateOperatorSMTPRequest — тело PUT /api/control/smtp.
// Все поля опциональны. Пустой password = НЕ менять (сохраняем существующий
// зашифрованный); явный "" в JSON-payload (через *string + проверка) = очистить.
type UpdateOperatorSMTPRequest struct {
	Host      *string `json:"host"`
	Port      *int    `json:"port"`
	Username  *string `json:"username"`
	Password  *string `json:"password"` // nil = не менять; "" = очистить; else = установить
	UseTLS    *bool   `json:"use_tls"`
	FromEmail *string `json:"from_email"`
	FromName  *string `json:"from_name"`
}

// Update — PUT /api/control/smtp.
func (api *OperatorSMTPAPI) Update(c *gin.Context) {
	opIDRaw, _ := c.Get("operator_id")
	opID, _ := opIDRaw.(uint)

	var req UpdateOperatorSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Неверный формат"})
		return
	}

	cfg, err := api.loadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка загрузки конфига"})
		return
	}

	if req.Host != nil {
		cfg.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		if *req.Port <= 0 || *req.Port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Невалидный порт"})
			return
		}
		cfg.Port = *req.Port
	}
	if req.Username != nil {
		cfg.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil {
		if *req.Password == "" {
			cfg.PasswordEnc = ""
		} else {
			enc, encErr := utils.EncryptString(*req.Password)
			if encErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка шифрования пароля"})
				return
			}
			cfg.PasswordEnc = enc
		}
	}
	if req.UseTLS != nil {
		cfg.UseTLS = *req.UseTLS
	}
	if req.FromEmail != nil {
		em := strings.TrimSpace(*req.FromEmail)
		if em != "" && !strings.Contains(em, "@") {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Невалидный from_email"})
			return
		}
		cfg.FromEmail = em
	}
	if req.FromName != nil {
		cfg.FromName = strings.TrimSpace(*req.FromName)
	}
	cfg.UpdatedByOpID = opID

	if err := api.db.Save(cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка сохранения"})
		return
	}

	// Не кэшируем секреты на промежуточных прокси.
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("Pragma", "no-cache")
	cfg.HasPassword = cfg.PasswordEnc != ""
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": cfg})
}

// TestOperatorSMTPRequest — тело POST /api/control/smtp/test.
type TestOperatorSMTPRequest struct {
	To string `json:"to" binding:"required,email"`
}

// Test — отправляет тестовый email на указанный адрес, используя текущий
// сохранённый config (с дешифрованием пароля).
func (api *OperatorSMTPAPI) Test(c *gin.Context) {
	var req TestOperatorSMTPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Укажите валидный email в поле to"})
		return
	}

	cfg, err := api.loadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка загрузки конфига"})
		return
	}
	if cfg.Host == "" || cfg.FromEmail == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "Заполните host и from_email"})
		return
	}

	password := ""
	if cfg.PasswordEnc != "" {
		p, derr := utils.DecryptString(cfg.PasswordEnc)
		if derr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": "Ошибка дешифрования пароля"})
			return
		}
		password = p
	}

	subject := "ACRM SMTP test"
	body := fmt.Sprintf(`<html><body><h2>SMTP test — успех</h2>
<p>Время: %s</p>
<p>Письмо отправлено из ACRM control-plane SMTP-конфига.</p>
<hr><small>Если получили — настройка работает.</small></body></html>`,
		time.Now().Format(time.RFC3339))

	if err := sendOperatorSMTP(cfg, password, req.To, subject, body); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "SMTP test failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"sent_to": req.To}})
}

// LoadOperatorSMTP — публичный helper: читает singleton + дешифрует
// пароль. Используется системными email-flows (welcome / forgot-password
// reset / invites).
//
// Если конфиг ещё не задан (host пуст) — возвращает ошибку "SMTP не настроен".
func LoadOperatorSMTP(db *gorm.DB) (*models.OperatorSMTPConfig, string, error) {
	var cfg models.OperatorSMTPConfig
	if err := db.Where("singleton = ?", true).First(&cfg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "", fmt.Errorf("SMTP не настроен — задайте конфиг в /control-plane/SMTP")
		}
		return nil, "", err
	}
	if cfg.Host == "" || cfg.FromEmail == "" {
		return nil, "", fmt.Errorf("SMTP не настроен (пустой host или from_email)")
	}
	password := ""
	if cfg.PasswordEnc != "" {
		p, derr := utils.DecryptString(cfg.PasswordEnc)
		if derr != nil {
			return nil, "", fmt.Errorf("дешифрование SMTP-пароля: %w", derr)
		}
		password = p
	}
	return &cfg, password, nil
}

// SendSystemEmail — public-helper для системных email'ов (welcome,
// reset-password, invite). Дёргает SMTP-config из БД и шлёт. Кейсы пустого
// конфига логируются и возвращают ошибку — caller решает что делать
// (например, register может игнорировать ошибку отправки и не блокировать
// создание юзера).
func SendSystemEmail(db *gorm.DB, to, subject, htmlBody string) error {
	cfg, password, err := LoadOperatorSMTP(db)
	if err != nil {
		return err
	}
	return sendOperatorSMTP(cfg, password, to, subject, htmlBody)
}

// sendOperatorSMTP — низкоуровневая отправка через текущий
// OperatorSMTPConfig. Зеркало services/notification_email.go но
// принимает operator-конфиг и расшифрованный пароль.
func sendOperatorSMTP(cfg *models.OperatorSMTPConfig, password, to, subject, htmlBody string) error {
	from := cfg.FromEmail
	if from == "" {
		from = cfg.Username
	}
	fromHeader := from
	if cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", cfg.FromName, from)
	}

	headers := map[string]string{
		"From":         fromHeader,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}
	var msg strings.Builder
	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, password, cfg.Host)

	if cfg.UseTLS {
		return sendOperatorSMTPViaTLS(addr, auth, cfg.Host, from, to, msg.String())
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
}

func sendOperatorSMTPViaTLS(addr string, auth smtp.Auth, host, from, to, msg string) error {
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}

	// Implicit TLS на 465 (порт зашит в addr — выводим из конца).
	if strings.HasSuffix(addr, ":465") {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()
		return finishOperatorSMTP(client, auth, from, to, msg)
	}

	// STARTTLS (587 и т.п.).
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer client.Close()
	if err := client.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	return finishOperatorSMTP(client, auth, from, to, msg)
}

func finishOperatorSMTP(client *smtp.Client, auth smtp.Auth, from, to, msg string) error {
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return client.Quit()
}
