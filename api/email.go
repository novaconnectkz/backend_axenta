package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
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
	
	// КРИТИЧНО: Получаем company_id из контекста через middleware
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию. Обратитесь к администратору.",
		})
		return
	}

	var req struct {
		SMTPHost      string `json:"smtp_host" binding:"required"`
		SMTPPort      int    `json:"smtp_port" binding:"required"`
		SMTPUsername  string `json:"smtp_username" binding:"required"`
		SMTPPassword  string `json:"smtp_password"` // Пароль опционален (может быть замаскирован)
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
	// КРИТИЧНО: Используем прямой SQL запрос с явным указанием схемы public и фильтрацией по company_id
	if err := database.DB.Table("public.notification_settings").Where("company_id = ?", companyID).First(&settings).Error; err != nil {
		// Если настроек нет, создаем новые
		settings = models.NotificationSettings{
			CompanyID: companyID,
		}
	}

	// Обновляем настройки Email
	settings.SMTPHost = req.SMTPHost
	settings.SMTPPort = req.SMTPPort
	settings.SMTPUsername = req.SMTPUsername
	
	// Обновляем пароль только если он не замаскирован (не состоит только из звездочек)
	if req.SMTPPassword != "" && req.SMTPPassword != "*********************" && req.SMTPPassword != "******" {
		settings.SMTPPassword = req.SMTPPassword
	}
	// Если пароль замаскирован, оставляем существующий
	
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
	
	// КРИТИЧНО: Получаем company_id из контекста через middleware
	companyID := middleware.GetCompanyID(c)
	if companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию. Обратитесь к администратору.",
		})
		return
	}

	var settings models.NotificationSettings
	// КРИТИЧНО: Используем прямой SQL запрос с явным указанием схемы public и фильтрацией по company_id
	if err := database.DB.Table("public.notification_settings").Where("company_id = ?", companyID).First(&settings).Error; err != nil {
		// Если настройки не найдены, возвращаем пустой объект вместо ошибки
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"config": nil,
		})
		return
	}

	// Проверяем параметр show_password для отображения реального пароля
	showPassword := c.Query("show_password") == "true"
	
	password := settings.SMTPPassword
	if !showPassword && password != "" {
		// Скрываем пароль в ответе по умолчанию
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
	log.Printf("🔔 TestEmailConnection вызвана! Path: %s, Method: %s", c.Request.URL.Path, c.Request.Method)
	
	// Получаем данные пользователя из контекста
	log.Printf("📥 Получаем user из контекста...")
	userInterface, exists := c.Get("user")
	if !exists {
		log.Printf("❌ TestEmailConnection: пользователь не найден в контексте")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Требуется авторизация",
		})
		return
	}
	log.Printf("✅ User найден в контексте")
	
	userData, ok := userInterface.(map[string]interface{})
	if !ok {
		log.Printf("❌ Не удалось преобразовать user к map[string]interface{}")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения данных пользователя",
		})
		return
	}
	log.Printf("✅ User data преобразован успешно")
	
	// КРИТИЧНО: Получаем company_id из контекста через middleware
	companyID := middleware.GetCompanyID(c)
	log.Printf("✅ Company ID получен: %d", companyID)
	if companyID == 0 {
		log.Printf("❌ Company ID = 0!")
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не удалось определить компанию. Обратитесь к администратору.",
		})
		return
	}
	
	// Получаем настройки Email
	log.Printf("📥 Запрашиваем настройки Email для company_id=%d", companyID)
	var settings models.NotificationSettings
	// КРИТИЧНО: Используем прямой SQL запрос с явным указанием схемы public и фильтрацией по company_id
	if err := database.DB.Table("public.notification_settings").Where("company_id = ?", companyID).First(&settings).Error; err != nil {
		log.Printf("❌ Настройки не найдены: %v", err)
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Настройки Email не найдены. Сначала настройте интеграцию.",
		})
		return
	}
	log.Printf("✅ Настройки Email найдены: host=%s, port=%d", settings.SMTPHost, settings.SMTPPort)

	if settings.SMTPHost == "" || settings.SMTPUsername == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Настройки Email не заполнены. Укажите SMTP сервер и логин.",
		})
		return
	}

	// Отправляем тестовое письмо на адрес из настроек (smtp_username)
	testEmail := settings.SMTPUsername
	if testEmail == "" {
		testEmail = settings.SMTPFromEmail
	}
	
	// Получаем имя пользователя для приветствия (необязательно)
	userName, _ := userData["username"].(string)
	if userName == "" {
		userName = "Администратор" // fallback
	}

	// Тестируем подключение к SMTP
	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)
	log.Printf("📧 Начинаем тест подключения к SMTP: %s", addr)
	auth := smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)

	// Формируем тестовое письмо
	from := settings.SMTPFromEmail
	if from == "" {
		from = settings.SMTPUsername
	}
	to := []string{testEmail}
	subject := "Тест Email SMTP - Axenta CRM"
	
	// Формируем HTML письмо
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: #f9f9f9; padding: 30px; border-radius: 0 0 10px 10px; }
        .success { color: #27ae60; font-size: 24px; margin: 20px 0; }
        .info { background: white; padding: 15px; border-left: 4px solid #667eea; margin: 20px 0; }
        .footer { text-align: center; color: #666; margin-top: 30px; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>✅ Тестовое письмо</h1>
            <p>Axenta CRM</p>
        </div>
        <div class="content">
            <p>Здравствуйте, %s!</p>
            <div class="success">✓ Настройки SMTP работают корректно</div>
            <div class="info">
                <p><strong>Детали подключения:</strong></p>
                <p>📧 Отправитель: %s</p>
                <p>🖥️ SMTP сервер: %s:%d</p>
                <p>🔐 TLS: %t</p>
            </div>
            <p>Если вы получили это письмо, значит Email интеграция настроена правильно и готова к использованию для отправки счетов и уведомлений.</p>
        </div>
        <div class="footer">
            <p>Письмо отправлено из Axenta CRM</p>
        </div>
    </div>
</body>
</html>`, userName, settings.SMTPFromName, settings.SMTPHost, settings.SMTPPort, settings.SMTPUseTLS)

	msg := []byte(fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-version: 1.0;\r\n"+
		"Content-Type: text/html; charset=\"UTF-8\";\r\n"+
		"\r\n"+
		"%s\r\n", settings.SMTPFromName, from, testEmail, subject, htmlBody))

	// Отправляем тестовое письмо
	log.Printf("📤 Отправляем тестовое письмо на %s через %s (TLS: %t)", testEmail, addr, settings.SMTPUseTLS)
	
	var err error
	if settings.SMTPPort == 465 && settings.SMTPUseTLS {
		// Для порта 465 используем прямое SSL/TLS соединение
		err = sendMailWithTLS(addr, auth, from, to, msg)
	} else {
		// Для других портов используем стандартный STARTTLS
		err = smtp.SendMail(addr, auth, from, to, msg)
	}
	
	if err != nil {
		log.Printf("❌ Ошибка отправки: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"error":   "Ошибка отправки тестового письма",
			"details": err.Error(),
		})
		return
	}
	log.Printf("✅ Письмо успешно отправлено!")

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("✅ Тестовое письмо успешно отправлено на %s", testEmail),
	})
}

// sendMailWithTLS отправляет письмо через прямое SSL/TLS соединение (для порта 465)
func sendMailWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	// Извлекаем хост из адреса
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	// Создаем TLS конфигурацию
	tlsConfig := &tls.Config{
		ServerName: host,
	}

	// Устанавливаем TLS соединение
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("❌ Ошибка TLS Dial: %v", err)
		return err
	}
	defer conn.Close()

	// Создаем SMTP клиент поверх TLS соединения
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Printf("❌ Ошибка создания SMTP клиента: %v", err)
		return err
	}
	defer client.Close()

	// Аутентификация
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			log.Printf("❌ Ошибка аутентификации: %v", err)
			return err
		}
	}

	// Устанавливаем отправителя
	if err = client.Mail(from); err != nil {
		log.Printf("❌ Ошибка установки отправителя: %v", err)
		return err
	}

	// Устанавливаем получателей
	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			log.Printf("❌ Ошибка установки получателя: %v", err)
			return err
		}
	}

	// Отправляем данные письма
	w, err := client.Data()
	if err != nil {
		log.Printf("❌ Ошибка Data(): %v", err)
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		log.Printf("❌ Ошибка записи сообщения: %v", err)
		return err
	}

	err = w.Close()
	if err != nil {
		log.Printf("❌ Ошибка закрытия writer: %v", err)
		return err
	}

	// Отправляем QUIT
	return client.Quit()
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

