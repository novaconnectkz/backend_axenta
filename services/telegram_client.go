package services

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"gorm.io/gorm"

	"backend_axenta/models"
)

// TelegramClient представляет клиент для работы с Telegram Bot API
type TelegramClient struct {
	bot       *tgbotapi.BotAPI
	db        *gorm.DB
	companyID uint
	settings  *models.NotificationSettings
}

// NewTelegramClient создает новый экземпляр Telegram клиента
func NewTelegramClient(db *gorm.DB, companyID uint) (*TelegramClient, error) {
	// Получаем настройки уведомлений для компании
	var settings models.NotificationSettings
	err := db.Where("company_id = ?", companyID).First(&settings).Error
	if err != nil {
		return nil, fmt.Errorf("настройки уведомлений не найдены для компании %d: %w", companyID, err)
	}

	if !settings.TelegramEnabled || settings.TelegramBotToken == "" {
		return nil, fmt.Errorf("Telegram не настроен для компании %d", companyID)
	}

	// Создаем Bot API клиент
	bot, err := tgbotapi.NewBotAPI(settings.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания Telegram бота: %w", err)
	}

	// В продакшене отключаем debug
	bot.Debug = false

	log.Printf("✅ Telegram бот авторизован: %s", bot.Self.UserName)

	return &TelegramClient{
		bot:       bot,
		db:        db,
		companyID: companyID,
		settings:  &settings,
	}, nil
}

// SendMessage отправляет сообщение пользователю
func (tc *TelegramClient) SendMessage(chatID string, message string) (*tgbotapi.Message, error) {
	// Парсим chat ID
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("неверный chat ID: %s", chatID)
	}

	// Создаем сообщение
	msg := tgbotapi.NewMessage(chatIDInt, message)
	msg.ParseMode = tgbotapi.ModeHTML

	// Отправляем сообщение
	sentMsg, err := tc.bot.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки сообщения: %w", err)
	}

	return &sentMsg, nil
}

// SendMessageWithKeyboard отправляет сообщение с inline клавиатурой
func (tc *TelegramClient) SendMessageWithKeyboard(chatID string, message string, keyboard [][]InlineKeyboardButton) (*tgbotapi.Message, error) {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("неверный chat ID: %s", chatID)
	}

	msg := tgbotapi.NewMessage(chatIDInt, message)
	msg.ParseMode = tgbotapi.ModeHTML

	// Создаем inline клавиатуру
	if len(keyboard) > 0 {
		var inlineKeyboard [][]tgbotapi.InlineKeyboardButton
		for _, row := range keyboard {
			var keyboardRow []tgbotapi.InlineKeyboardButton
			for _, button := range row {
				btn := tgbotapi.NewInlineKeyboardButtonData(button.Text, button.CallbackData)
				if button.URL != "" {
					btn = tgbotapi.NewInlineKeyboardButtonURL(button.Text, button.URL)
				}
				keyboardRow = append(keyboardRow, btn)
			}
			inlineKeyboard = append(inlineKeyboard, keyboardRow)
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(inlineKeyboard...)
	}

	sentMsg, err := tc.bot.Send(msg)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки сообщения с клавиатурой: %w", err)
	}

	return &sentMsg, nil
}

// InlineKeyboardButton представляет кнопку inline клавиатуры
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// EditMessage редактирует существующее сообщение
func (tc *TelegramClient) EditMessage(chatID string, messageID int, newText string) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("неверный chat ID: %s", chatID)
	}

	edit := tgbotapi.NewEditMessageText(chatIDInt, messageID, newText)
	edit.ParseMode = tgbotapi.ModeHTML

	_, err = tc.bot.Send(edit)
	if err != nil {
		return fmt.Errorf("ошибка редактирования сообщения: %w", err)
	}

	return nil
}

// DeleteMessage удаляет сообщение
func (tc *TelegramClient) DeleteMessage(chatID string, messageID int) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return fmt.Errorf("неверный chat ID: %s", chatID)
	}

	delete := tgbotapi.NewDeleteMessage(chatIDInt, messageID)
	_, err = tc.bot.Send(delete)
	if err != nil {
		return fmt.Errorf("ошибка удаления сообщения: %w", err)
	}

	return nil
}

// SetWebhook устанавливает webhook для получения обновлений
func (tc *TelegramClient) SetWebhook(webhookURL string) error {
	webhook, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("ошибка создания webhook: %w", err)
	}

	_, err = tc.bot.Request(webhook)
	if err != nil {
		return fmt.Errorf("ошибка установки webhook: %w", err)
	}

	log.Printf("✅ Webhook установлен: %s", webhookURL)
	return nil
}

// RemoveWebhook удаляет webhook
func (tc *TelegramClient) RemoveWebhook() error {
	_, err := tc.bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		return fmt.Errorf("ошибка удаления webhook: %w", err)
	}

	log.Println("✅ Webhook удален")
	return nil
}

// GetUpdates получает обновления через polling (альтернатива webhook)
func (tc *TelegramClient) GetUpdates() (tgbotapi.UpdatesChannel, error) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := tc.bot.GetUpdatesChan(u)
	return updates, nil
}

// ProcessUpdate обрабатывает входящее обновление от Telegram
func (tc *TelegramClient) ProcessUpdate(update tgbotapi.Update) error {
	// Обрабатываем текстовые сообщения
	if update.Message != nil {
		return tc.processMessage(update.Message)
	}

	// Обрабатываем callback queries (нажатия на inline кнопки)
	if update.CallbackQuery != nil {
		return tc.processCallbackQuery(update.CallbackQuery)
	}

	return nil
}

// processMessage обрабатывает входящие текстовые сообщения
func (tc *TelegramClient) processMessage(message *tgbotapi.Message) error {
	chatID := strconv.FormatInt(message.Chat.ID, 10)
	text := message.Text

	log.Printf("Получено сообщение от %s (ID: %s): %s", message.From.UserName, chatID, text)

	// Обрабатываем команды
	if strings.HasPrefix(text, "/") {
		return tc.processCommand(chatID, text, message)
	}

	// Обрабатываем обычные сообщения как ответы
	return tc.processUserResponse(chatID, text, message)
}

// processCommand обрабатывает команды бота
func (tc *TelegramClient) processCommand(chatID, command string, message *tgbotapi.Message) error {
	switch command {
	case "/start":
		return tc.handleStartCommand(chatID, message)
	case "/help":
		return tc.handleHelpCommand(chatID)
	case "/status":
		return tc.handleStatusCommand(chatID, message)
	case "/settings":
		return tc.handleSettingsCommand(chatID, message)
	default:
		_, err := tc.SendMessage(chatID, "❌ Неизвестная команда. Используйте /help для просмотра доступных команд.")
		return err
	}
}

// handleStartCommand обрабатывает команду /start
func (tc *TelegramClient) handleStartCommand(chatID string, message *tgbotapi.Message) error {
	welcomeText := fmt.Sprintf(`👋 <b>Добро пожаловать в Axenta CRM!</b>

Я бот для уведомлений системы управления объектами.

Ваш Telegram ID: <code>%s</code>

Чтобы получать уведомления, обратитесь к администратору системы для привязки вашего аккаунта.

Доступные команды:
/help - Справка по командам
/status - Проверить статус подключения
/settings - Настройки уведомлений`, chatID)

	_, err := tc.SendMessage(chatID, welcomeText)
	return err
}

// handleHelpCommand обрабатывает команду /help
func (tc *TelegramClient) handleHelpCommand(chatID string) error {
	helpText := `📋 <b>Доступные команды:</b>

/start - Начать работу с ботом
/help - Показать эту справку
/status - Проверить статус подключения
/settings - Настройки уведомлений

<b>Типы уведомлений:</b>
🔧 Монтажи и диагностика
💰 Биллинг и платежи
📦 Складские операции
⚠️ Системные уведомления

Для настройки уведомлений обратитесь к администратору системы.`

	_, err := tc.SendMessage(chatID, helpText)
	return err
}

// handleStatusCommand обрабатывает команду /status
func (tc *TelegramClient) handleStatusCommand(chatID string, message *tgbotapi.Message) error {
	// Проверяем, привязан ли пользователь к системе
	var user models.User
	err := tc.db.Where("telegram_id = ? AND company_id = ?", chatID, tc.companyID).First(&user).Error

	var statusText string
	if err != nil {
		statusText = `❌ <b>Аккаунт не привязан</b>

Ваш Telegram аккаунт не привязан к системе.
Обратитесь к администратору для настройки.

Ваш Telegram ID: <code>` + chatID + `</code>`
	} else {
		statusText = fmt.Sprintf(`✅ <b>Аккаунт привязан</b>

Пользователь: %s %s
Email: %s
Статус: %s

Уведомления настроены и работают.`,
			user.FirstName, user.LastName, user.Email,
			map[bool]string{true: "Активен", false: "Неактивен"}[user.IsActive])
	}

	_, err = tc.SendMessage(chatID, statusText)
	return err
}

// handleSettingsCommand обрабатывает команду /settings
func (tc *TelegramClient) handleSettingsCommand(chatID string, message *tgbotapi.Message) error {
	// Получаем пользователя и его настройки
	var user models.User
	err := tc.db.Where("telegram_id = ? AND company_id = ?", chatID, tc.companyID).First(&user).Error
	if err != nil {
		_, sendErr := tc.SendMessage(chatID, "❌ Аккаунт не привязан. Обратитесь к администратору.")
		return sendErr
	}

	// Получаем настройки уведомлений пользователя
	var prefs models.UserNotificationPreferences
	err = tc.db.Where("user_id = ?", user.ID).First(&prefs).Error
	if err != nil {
		// Создаем настройки по умолчанию
		prefs = models.UserNotificationPreferences{
			UserID:                user.ID,
			TelegramEnabled:       true,
			EmailEnabled:          true,
			SMSEnabled:            false,
			InstallationReminders: true,
			InstallationUpdates:   true,
			BillingAlerts:         true,
			WarehouseAlerts:       true,
			SystemNotifications:   true,
			CompanyID:             tc.companyID,
		}
		tc.db.Create(&prefs)
	}

	// Создаем клавиатуру с настройками
	keyboard := [][]InlineKeyboardButton{
		{{Text: fmt.Sprintf("Telegram: %s", map[bool]string{true: "✅", false: "❌"}[prefs.TelegramEnabled]), CallbackData: "toggle_telegram"}},
		{{Text: fmt.Sprintf("Email: %s", map[bool]string{true: "✅", false: "❌"}[prefs.EmailEnabled]), CallbackData: "toggle_email"}},
		{{Text: fmt.Sprintf("Монтажи: %s", map[bool]string{true: "✅", false: "❌"}[prefs.InstallationReminders]), CallbackData: "toggle_installations"}},
		{{Text: fmt.Sprintf("Биллинг: %s", map[bool]string{true: "✅", false: "❌"}[prefs.BillingAlerts]), CallbackData: "toggle_billing"}},
		{{Text: fmt.Sprintf("Склад: %s", map[bool]string{true: "✅", false: "❌"}[prefs.WarehouseAlerts]), CallbackData: "toggle_warehouse"}},
		{{Text: "🔄 Обновить", CallbackData: "refresh_settings"}},
	}

	settingsText := `⚙️ <b>Настройки уведомлений</b>

Выберите типы уведомлений, которые хотите получать:

Тихие часы: ` + prefs.QuietHoursStart + ` - ` + prefs.QuietHoursEnd + `
Часовой пояс: ` + prefs.Timezone

	_, err = tc.SendMessageWithKeyboard(chatID, settingsText, keyboard)
	return err
}

// processCallbackQuery обрабатывает нажатия на inline кнопки
func (tc *TelegramClient) processCallbackQuery(query *tgbotapi.CallbackQuery) error {
	chatID := strconv.FormatInt(query.Message.Chat.ID, 10)
	data := query.Data

	// Отвечаем на callback query
	callback := tgbotapi.NewCallback(query.ID, "")
	tc.bot.Request(callback)

	// Получаем пользователя
	var user models.User
	err := tc.db.Where("telegram_id = ? AND company_id = ?", chatID, tc.companyID).First(&user).Error
	if err != nil {
		return err
	}

	// Получаем настройки пользователя
	var prefs models.UserNotificationPreferences
	err = tc.db.Where("user_id = ?", user.ID).First(&prefs).Error
	if err != nil {
		return err
	}

	// Обрабатываем действия
	switch data {
	case "toggle_telegram":
		prefs.TelegramEnabled = !prefs.TelegramEnabled
	case "toggle_email":
		prefs.EmailEnabled = !prefs.EmailEnabled
	case "toggle_installations":
		prefs.InstallationReminders = !prefs.InstallationReminders
	case "toggle_billing":
		prefs.BillingAlerts = !prefs.BillingAlerts
	case "toggle_warehouse":
		prefs.WarehouseAlerts = !prefs.WarehouseAlerts
	case "refresh_settings":
		// Просто обновляем сообщение
	}

	// Сохраняем изменения
	tc.db.Save(&prefs)

	// Обновляем сообщение
	return tc.handleSettingsCommand(chatID, query.Message)
}

// processUserResponse обрабатывает ответы пользователей на уведомления
func (tc *TelegramClient) processUserResponse(chatID, text string, message *tgbotapi.Message) error {
	// Логируем ответ пользователя
	log.Printf("Ответ пользователя %s: %s", chatID, text)

	// Здесь можно добавить логику обработки ответов
	// Например, подтверждение получения монтажа, изменение времени и т.д.

	// Пока что просто отвечаем, что получили сообщение
	_, err := tc.SendMessage(chatID, "✅ Сообщение получено. Спасибо за обратную связь!")
	return err
}

// GetBotInfo возвращает информацию о боте
func (tc *TelegramClient) GetBotInfo() (*tgbotapi.User, error) {
	return &tc.bot.Self, nil
}

// IsHealthy проверяет, работает ли бот
func (tc *TelegramClient) IsHealthy() bool {
	// Пробуем получить информацию о боте
	_, err := tc.bot.GetMe()
	return err == nil
}
