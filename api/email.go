package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"

	"github.com/gin-gonic/gin"
)

// SetupEmailIntegration настраивает интеграцию Email SMTP (POST /api/email/setup)
func SetupEmailIntegration(c *gin.Context) {
	// Получаем данные пользователя из контекста
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Требуется авторизация",
		})
		return
	}

	var req struct {
		SMTPHost      string `json:"smtp_host" binding:"required"`
		SMTPPort      int    `json:"smtp_port" binding:"required"`
		SMTPUsername  string `json:"smtp_username" binding:"required"`
		SMTPPassword  string `json:"smtp_password" binding:"required"`
		SMTPFromEmail string `json:"smtp_from_email" binding:"required"`
		SMTPFromName  string `json:"smtp_from_name"`
		SMTPUseTLS    bool   `json:"smtp_use_tls"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат данных: " + err.Error(),
		})
		return
	}

	// Получаем или создаем настройки уведомлений (из public схемы)
	var settings models.NotificationSettings
	// Используем прямой SQL запрос с явным указанием схемы public
	if err := database.DB.Table("public.notification_settings").First(&settings).Error; err != nil {
		// Если настроек нет, создаем новые
		settings = models.NotificationSettings{}
	}

	// Обновляем настройки Email
	settings.SMTPHost = req.SMTPHost
	settings.SMTPPort = req.SMTPPort
	settings.SMTPUsername = req.SMTPUsername
	settings.SMTPPassword = req.SMTPPassword
	settings.SMTPFromEmail = req.SMTPFromEmail
	settings.SMTPFromName = req.SMTPFromName
	if req.SMTPFromName == "" {
		settings.SMTPFromName = "Axenta CRM"
	}
	settings.SMTPUseTLS = req.SMTPUseTLS
	settings.EmailEnabled = true

	if err := database.DB.Table("public.notification_settings").Save(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка сохранения настроек Email: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Настройки Email SMTP успешно сохранены",
		"config": gin.H{
			"smtp_host":       settings.SMTPHost,
			"smtp_port":       settings.SMTPPort,
			"smtp_username":   settings.SMTPUsername,
			"smtp_from_email": settings.SMTPFromEmail,
			"smtp_from_name":  settings.SMTPFromName,
			"smtp_use_tls":    settings.SMTPUseTLS,
			"enabled":         settings.EmailEnabled,
			"is_active":       settings.EmailEnabled,
		},
	})
}

// UpdateEmailIntegration обновляет интеграцию Email SMTP (PUT /api/email/setup)
func UpdateEmailIntegration(c *gin.Context) {
	// Используем ту же логику, что и для настройки
	SetupEmailIntegration(c)
}

// GetEmailConfig возвращает текущую конфигурацию Email SMTP (GET /api/email/config)
func GetEmailConfig(c *gin.Context) {
	// Проверяем наличие пользователя в контексте
	_, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Требуется авторизация",
		})
		return
	}

	var settings models.NotificationSettings
	// Используем прямой SQL запрос с явным указанием схемы public
	if err := database.DB.Table("public.notification_settings").First(&settings).Error; err != nil {
		// Если настройки не найдены, возвращаем пустой объект вместо ошибки
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"config": nil,
		})
		return
	}

	// Скрываем пароль в ответе
	password := settings.SMTPPassword
	if password != "" {
		password = "*********************"
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"config": gin.H{
			"smtp_host":       settings.SMTPHost,
			"smtp_port":       settings.SMTPPort,
			"smtp_username":   settings.SMTPUsername,
			"smtp_password":   password,
			"smtp_from_email": settings.SMTPFromEmail,
			"smtp_from_name":  settings.SMTPFromName,
			"smtp_use_tls":    settings.SMTPUseTLS,
			"enabled":         settings.EmailEnabled,
			"is_active":       settings.EmailEnabled,
		},
	})
}

// TestEmailConnection тестирует подключение к SMTP серверу (POST /api/email/test-connection)
func TestEmailConnection(c *gin.Context) {
	// Получаем данные пользователя из контекста
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Требуется авторизация",
		})
		return
	}
	
	userData, ok := userInterface.(map[string]interface{})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения данных пользователя",
		})
		return
	}
	
	// Получаем email пользователя для тестовой отправки
	userEmail, _ := userData["email"].(string)
	if userEmail == "" {
		userEmail = "test@example.com" // fallback
	}
	
	userName, _ := userData["username"].(string)
	if userName == "" {
		userName = "User" // fallback
	}

	// Получаем настройки Email
	var settings models.NotificationSettings
	// Используем прямой SQL запрос с явным указанием схемы public
	if err := database.DB.Table("public.notification_settings").First(&settings).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Настройки Email не найдены. Сначала настройте интеграцию.",
		})
		return
	}

	if settings.SMTPHost == "" || settings.SMTPUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Настройки Email не заполнены. Укажите SMTP сервер и логин.",
		})
		return
	}

	// Тестируем подключение к SMTP
	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)
	auth := smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)

	// Формируем тестовое письмо
	from := settings.SMTPFromEmail
	to := []string{userEmail}
	subject := "Тестовое письмо от Axenta CRM"
	body := fmt.Sprintf(`
Привет, %s!

Это тестовое письмо для проверки настроек Email SMTP в Axenta CRM.

Если вы получили это письмо, значит настройки SMTP работают корректно.

---
С уважением,
Команда Axenta CRM
`, userName)

	msg := []byte(fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-version: 1.0;\r\n"+
		"Content-Type: text/plain; charset=\"UTF-8\";\r\n"+
		"\r\n"+
		"%s\r\n", settings.SMTPFromName, from, userEmail, subject, body))

	// Отправляем тестовое письмо
	err := smtp.SendMail(addr, auth, from, to, msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"error":   "Ошибка отправки тестового письма",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Тестовое письмо успешно отправлено на %s", userEmail),
	})
}

// GetEmailIntegrationDocs возвращает документацию по Email интеграции (GET /docs/EMAIL_INTEGRATION.md)
func GetEmailIntegrationDocs(c *gin.Context) {
	file, err := os.Open("docs/EMAIL_INTEGRATION.md")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Документация не найдена",
		})
		return
	}
	defer file.Close()

	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", "inline; filename=EMAIL_INTEGRATION.md")
	io.Copy(c.Writer, file)
}

