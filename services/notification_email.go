package services

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"backend_axenta/models"
)

// sendEmail отправляет письмо через SMTP по настройкам компании.
// Возвращает ошибку или nil. Фактический результат пишет caller через writeNotificationLog.
func sendEmail(settings *models.NotificationSettings, recipient, subject, body string) error {
	if !settings.EmailEnabled {
		return fmt.Errorf("email канал отключён для компании %d", settings.CompanyID)
	}
	if settings.SMTPHost == "" {
		return fmt.Errorf("SMTP host не настроен")
	}
	if recipient == "" {
		return fmt.Errorf("пустой recipient")
	}

	from := settings.SMTPFromEmail
	if from == "" {
		from = settings.SMTPUsername
	}

	fromHeader := from
	if settings.SMTPFromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", settings.SMTPFromName, from)
	}

	headers := map[string]string{
		"From":         fromHeader,
		"To":           recipient,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	var msg strings.Builder
	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)
	auth := smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)

	if settings.SMTPUseTLS {
		return sendEmailTLS(addr, auth, settings.SMTPHost, from, recipient, msg.String())
	}

	return smtp.SendMail(addr, auth, from, []string{recipient}, []byte(msg.String()))
}

// sendEmailTLS отправляет через STARTTLS либо implicit TLS (для портов 465).
func sendEmailTLS(addr string, auth smtp.Auth, host, from, to, msg string) error {
	tlsCfg := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	// Implicit TLS на 465 (порт зашит в addr — выводим из конца)
	if strings.HasSuffix(addr, ":465") {
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()

		c, err := smtp.NewClient(conn, host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer c.Close()

		return finishSMTP(c, auth, from, to, msg)
	}

	// STARTTLS (587 итд)
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer c.Close()

	if err := c.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}

	return finishSMTP(c, auth, from, to, msg)
}

func finishSMTP(c *smtp.Client, auth smtp.Auth, from, to, msg string) error {
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	return c.Quit()
}
