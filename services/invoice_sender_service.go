package services

import (
	"backend_axenta/models"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// InvoiceSenderService предоставляет функциональность отправки счетов клиентам
type InvoiceSenderService struct {
	DB                         *gorm.DB
	telegramIntegrationService *TelegramIntegrationService
	maxIntegrationService      *MaxIntegrationService
}

// NewInvoiceSenderService создает новый экземпляр InvoiceSenderService
func NewInvoiceSenderService(db *gorm.DB) *InvoiceSenderService {
	logger := log.New(log.Writer(), "[InvoiceSender] ", log.LstdFlags|log.Lshortfile)

	return &InvoiceSenderService{
		DB:                         db,
		telegramIntegrationService: NewTelegramIntegrationService(db, logger),
		maxIntegrationService:      NewMaxIntegrationService(db, logger),
	}
}

// SendInvoiceToClient отправляет счет клиенту по выбранным каналам
func (s *InvoiceSenderService) SendInvoiceToClient(invoice *models.Invoice, channels []string, contactInfo map[string]string) error {
	log.Printf("📤 Начинаем отправку счета #%s по каналам: %v", invoice.Number, channels)

	var errors []string
	var sentChannels []string

	for _, channel := range channels {
		var err error

		switch channel {
		case "email":
			if email, ok := contactInfo["email"]; ok && email != "" {
				err = s.sendInvoiceViaEmail(invoice, email)
				if err == nil {
					sentChannels = append(sentChannels, "email")
					invoice.SendToEmail = email
				}
			} else {
				err = fmt.Errorf("email не указан")
			}

		case "telegram":
			if telegramID, ok := contactInfo["telegram"]; ok && telegramID != "" {
				err = s.sendInvoiceViaTelegram(invoice, telegramID)
				if err == nil {
					sentChannels = append(sentChannels, "telegram")
					invoice.SendToTelegram = telegramID
				}
			} else {
				err = fmt.Errorf("telegram ID не указан")
			}

		case "max":
			if maxID, ok := contactInfo["max"]; ok && maxID != "" {
				err = s.sendInvoiceViaMax(invoice, maxID)
				if err == nil {
					sentChannels = append(sentChannels, "max")
					invoice.SendToMax = maxID
				}
			} else {
				err = fmt.Errorf("MAX ID не указан")
			}

		default:
			err = fmt.Errorf("неподдерживаемый канал: %s", channel)
		}

		if err != nil {
			log.Printf("❌ Ошибка отправки счета #%s через %s: %v", invoice.Number, channel, err)
			errors = append(errors, fmt.Sprintf("%s: %v", channel, err))
		} else {
			log.Printf("✅ Счет #%s успешно отправлен через %s", invoice.Number, channel)
		}
	}

	// Обновляем информацию об отправке в счете
	if len(sentChannels) > 0 {
		now := time.Now()
		invoice.LastSentAt = &now
		invoice.LastSentChannels = strings.Join(sentChannels, ",")
		invoice.SendChannels = strings.Join(channels, ",")
		invoice.Status = "sent"

		if err := s.DB.Save(invoice).Error; err != nil {
			log.Printf("⚠️ Не удалось обновить информацию об отправке счета: %v", err)
		}
	}

	// Если хотя бы один канал сработал, считаем отправку успешной
	if len(sentChannels) > 0 {
		if len(errors) > 0 {
			return fmt.Errorf("частичная отправка: %s", strings.Join(errors, "; "))
		}
		return nil
	}

	// Если ни один канал не сработал, возвращаем ошибку
	return fmt.Errorf("не удалось отправить счет ни по одному каналу: %s", strings.Join(errors, "; "))
}

// sendInvoiceViaEmail отправляет счет по email
func (s *InvoiceSenderService) sendInvoiceViaEmail(invoice *models.Invoice, email string) error {
	// Получаем настройки email для компании
	var settings models.NotificationSettings
	err := s.DB.Where("company_id = ?", invoice.CompanyID).First(&settings).Error
	if err != nil {
		if err.Error() == "record not found" || strings.Contains(err.Error(), "record not found") {
			return fmt.Errorf("email интеграция не настроена. Перейдите в Настройки → Интеграции → Email SMTP для настройки")
		}
		return fmt.Errorf("ошибка получения настроек email: %w", err)
	}

	if !settings.EmailEnabled {
		return fmt.Errorf("email уведомления отключены. Включите их в Настройки → Интеграции → Email SMTP")
	}

	if settings.SMTPHost == "" || settings.SMTPUsername == "" {
		return fmt.Errorf("email не настроен полностью. Заполните все поля в Настройки → Интеграции → Email SMTP")
	}

	// Формируем тему и текст письма
	subject := fmt.Sprintf("Счет %s на оплату", invoice.Number)
	message := s.generateInvoiceEmailMessage(invoice)

	// Настраиваем SMTP
	auth := smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)

	// Формируем email
	from := settings.SMTPFromEmail
	if from == "" {
		from = settings.SMTPUsername
	}

	msg := fmt.Sprintf("From: %s <%s>\r\n", settings.SMTPFromName, from)
	msg += fmt.Sprintf("To: %s\r\n", email)
	msg += fmt.Sprintf("Subject: %s\r\n", subject)
	msg += "Content-Type: text/html; charset=UTF-8\r\n"
	msg += "\r\n"
	msg += message

	// Отправляем
	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)

	// Проверяем, использует ли SMTP-сервер TLS (порт 465 обычно SSL/TLS)
	if settings.SMTPUseTLS || settings.SMTPPort == 465 {
		// Используем TLS
		err = s.sendEmailViaTLS(addr, auth, from, []string{email}, []byte(msg), settings.SMTPHost)
		if err != nil {
			return fmt.Errorf("ошибка отправки email через TLS: %w", err)
		}
	} else {
		// Обычный SMTP без TLS
		err = smtp.SendMail(addr, auth, from, []string{email}, []byte(msg))
		if err != nil {
			return fmt.Errorf("ошибка отправки email: %w", err)
		}
	}

	log.Printf("✅ Email с счетом #%s отправлен на %s", invoice.Number, email)
	return nil
}

// sendEmailViaTLS отправляет email через TLS/SSL соединение
func (s *InvoiceSenderService) sendEmailViaTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte, host string) error {
	// Создаем TLS конфигурацию
	tlsConfig := &tls.Config{
		ServerName: host,
	}

	// Подключаемся через TLS
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("ошибка TLS подключения: %w", err)
	}
	defer conn.Close()

	// Создаем SMTP клиент
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("ошибка создания SMTP клиента: %w", err)
	}
	defer client.Quit()

	// Аутентификация
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("ошибка аутентификации: %w", err)
	}

	// Устанавливаем отправителя
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("ошибка установки отправителя: %w", err)
	}

	// Устанавливаем получателей
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return fmt.Errorf("ошибка установки получателя: %w", err)
		}
	}

	// Отправляем данные письма
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("ошибка получения writer: %w", err)
	}

	_, err = w.Write(msg)
	if err != nil {
		return fmt.Errorf("ошибка записи сообщения: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("ошибка закрытия writer: %w", err)
	}

	return nil
}

// sendInvoiceViaTelegram отправляет счет через Telegram
func (s *InvoiceSenderService) sendInvoiceViaTelegram(invoice *models.Invoice, telegramID string) error {
	// Получаем конфигурацию Telegram интеграции
	ctx := context.Background()
	config, err := s.telegramIntegrationService.GetConfig(ctx, invoice.CompanyID)
	if err != nil {
		return fmt.Errorf("telegram не настроен. Перейдите в Настройки → Интеграции → Telegram")
	}

	if config.BotToken == "" {
		return fmt.Errorf("telegram токен не настроен. Перейдите в Настройки → Интеграции → Telegram")
	}

	// Формируем сообщение
	message := s.generateInvoiceTelegramMessage(invoice)

	// Отправляем сообщение через Telegram Integration Service
	options := make(map[string]interface{})
	options["parse_mode"] = config.ParseMode

	err = s.telegramIntegrationService.SendMessage(ctx, invoice.CompanyID, telegramID, message, options)
	if err != nil {
		return fmt.Errorf("ошибка отправки через Telegram: %w", err)
	}

	log.Printf("✅ Telegram сообщение с счетом #%s отправлено пользователю %s", invoice.Number, telegramID)
	return nil
}

// sendInvoiceViaMax отправляет счет через MAX мессенджер
func (s *InvoiceSenderService) sendInvoiceViaMax(invoice *models.Invoice, maxID string) error {
	// Получаем конфигурацию MAX интеграции
	ctx := context.Background()
	config, err := s.maxIntegrationService.GetConfig(ctx, invoice.CompanyID)
	if err != nil {
		return fmt.Errorf("MAX не настроен. Перейдите в Настройки → Интеграции → MAX") // nolint:ST1005
	}

	if config.BotToken == "" {
		return fmt.Errorf("MAX токен не настроен. Перейдите в Настройки → Интеграции → MAX") // nolint:ST1005
	}

	// Формируем сообщение
	message := s.generateInvoiceMaxMessage(invoice)

	// Отправляем сообщение через MAX Integration Service
	options := make(map[string]interface{})
	options["parse_mode"] = config.ParseMode

	err = s.maxIntegrationService.SendMessage(ctx, invoice.CompanyID, maxID, message, options)
	if err != nil {
		return fmt.Errorf("ошибка отправки через MAX: %w", err)
	}

	log.Printf("✅ MAX сообщение с счетом #%s отправлено пользователю %s", invoice.Number, maxID)
	return nil
}

// generateInvoiceEmailMessage генерирует HTML сообщение для email
func (s *InvoiceSenderService) generateInvoiceEmailMessage(invoice *models.Invoice) string {
	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #007AFF; color: white; padding: 20px; text-align: center; border-radius: 5px 5px 0 0; }
        .content { background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; }
        .invoice-info { background-color: white; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .invoice-info p { margin: 5px 0; }
        .amount { font-size: 24px; font-weight: bold; color: #007AFF; }
        .footer { text-align: center; margin-top: 20px; color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📧 Счет на оплату</h1>
        </div>
        <div class="content">
            <div class="invoice-info">
                <p><strong>Номер счета:</strong> %s</p>
                <p><strong>Дата:</strong> %s</p>
                <p><strong>Срок оплаты:</strong> %s</p>
                <p><strong>Период:</strong> %s - %s</p>
                <hr>
                <p class="amount">Сумма к оплате: %s %s</p>
            </div>
            <p>Уважаемый клиент,</p>
            <p>Просим вас оплатить выставленный счет до указанной даты.</p>
            <p>%s</p>
        </div>
        <div class="footer">
            <p>С уважением,<br>Команда Axenta CRM</p>
        </div>
    </div>
</body>
</html>`

	return fmt.Sprintf(html,
		invoice.Number,
		invoice.InvoiceDate.Format("02.01.2006"),
		invoice.DueDate.Format("02.01.2006"),
		invoice.BillingPeriodStart.Format("02.01.2006"),
		invoice.BillingPeriodEnd.Format("02.01.2006"),
		invoice.TotalAmount.String(),
		invoice.Currency,
		invoice.Description,
	)
}

// generateInvoiceTelegramMessage генерирует сообщение для Telegram
func (s *InvoiceSenderService) generateInvoiceTelegramMessage(invoice *models.Invoice) string {
	return fmt.Sprintf(`📧 <b>Новый счет на оплату</b>

📄 Номер: <b>%s</b>
📅 Дата: %s
⏰ Срок оплаты: %s
📆 Период: %s - %s

💰 <b>Сумма: %s %s</b>

%s

Пожалуйста, оплатите счет до указанной даты.`,
		invoice.Number,
		invoice.InvoiceDate.Format("02.01.2006"),
		invoice.DueDate.Format("02.01.2006"),
		invoice.BillingPeriodStart.Format("02.01.2006"),
		invoice.BillingPeriodEnd.Format("02.01.2006"),
		invoice.TotalAmount.String(),
		invoice.Currency,
		invoice.Description,
	)
}

// generateInvoiceMaxMessage генерирует сообщение для MAX
func (s *InvoiceSenderService) generateInvoiceMaxMessage(invoice *models.Invoice) string {
	return fmt.Sprintf(`📧 <b>Новый счет на оплату</b>

📄 Номер: <b>%s</b>
📅 Дата: %s
⏰ Срок оплаты: %s
📆 Период: %s - %s

💰 <b>Сумма: %s %s</b>

%s

Пожалуйста, оплатите счет до указанной даты.`,
		invoice.Number,
		invoice.InvoiceDate.Format("02.01.2006"),
		invoice.DueDate.Format("02.01.2006"),
		invoice.BillingPeriodStart.Format("02.01.2006"),
		invoice.BillingPeriodEnd.Format("02.01.2006"),
		invoice.TotalAmount.String(),
		invoice.Currency,
		invoice.Description,
	)
}
