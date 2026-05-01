package services

import (
	"errors"
	"fmt"
	"strings"

	"backend_axenta/models"
)

// alertRecipient — резолвленный получатель алерта (один пользователь, одна компания).
type alertRecipient struct {
	UserID     uint
	Email      string
	TelegramID string
	MaxID      string
	Prefs      *models.UserNotificationPreferences // nil если запись не найдена → дефолты true
}

// resolveRecipientsByRoles ищет активных пользователей компании, чья роль
// (по имени Role.Name) входит в roleNames.
//
// Возвращаемый список — все подходящие пользователи; фильтрация по каналам
// и per-user prefs выполняется в sendAlertToRecipients.
func (s *NotificationService) resolveRecipientsByRoles(companyID uint, roleNames []string) ([]alertRecipient, error) {
	if s.DB == nil {
		return nil, errors.New("БД не подключена")
	}
	if len(roleNames) == 0 {
		return nil, errors.New("пустой список ролей")
	}

	var users []models.User
	q := s.DB.
		Joins("JOIN roles ON roles.id = users.role_id").
		Where("users.company_id = ?", companyID).
		Where("users.is_active = ?", true).
		Where("roles.name IN ?", roleNames).
		Find(&users)
	if q.Error != nil {
		return nil, fmt.Errorf("поиск получателей: %w", q.Error)
	}

	if len(users) == 0 {
		return nil, nil
	}

	userIDs := make([]uint, 0, len(users))
	for _, u := range users {
		userIDs = append(userIDs, u.ID)
	}

	var prefs []models.UserNotificationPreferences
	if err := s.DB.Where("user_id IN ?", userIDs).Find(&prefs).Error; err != nil {
		return nil, fmt.Errorf("предпочтения уведомлений: %w", err)
	}
	prefsByUser := make(map[uint]*models.UserNotificationPreferences, len(prefs))
	for i := range prefs {
		prefsByUser[prefs[i].UserID] = &prefs[i]
	}

	out := make([]alertRecipient, 0, len(users))
	for _, u := range users {
		out = append(out, alertRecipient{
			UserID:     u.ID,
			Email:      u.Email,
			TelegramID: u.TelegramID,
			MaxID:      u.MaxID,
			Prefs:      prefsByUser[u.ID],
		})
	}
	return out, nil
}

// alertCategory определяет какой флаг UserNotificationPreferences проверять.
type alertCategory string

const (
	alertCategoryBilling   alertCategory = "billing"
	alertCategoryWarehouse alertCategory = "warehouse"
	alertCategorySystem    alertCategory = "system"
)

// allowsCategory возвращает true если получатель не отписан от категории
// (отсутствие записи prefs трактуется как «согласен», т.к. дефолты модели true).
func (r alertRecipient) allowsCategory(cat alertCategory) bool {
	if r.Prefs == nil {
		return true
	}
	switch cat {
	case alertCategoryBilling:
		return r.Prefs.BillingAlerts
	case alertCategoryWarehouse:
		return r.Prefs.WarehouseAlerts
	case alertCategorySystem:
		return r.Prefs.SystemNotifications
	}
	return true
}

// allowsChannel — per-user opt-in/out для конкретного канала (дефолты true).
func (r alertRecipient) allowsChannel(channel string) bool {
	if r.Prefs == nil {
		return true
	}
	switch channel {
	case "email":
		return r.Prefs.EmailEnabled
	case "telegram":
		return r.Prefs.TelegramEnabled
	case "max":
		return r.Prefs.MaxEnabled
	}
	return true
}

// sendAlertToRecipients рассылает уведомление списку получателей по всем
// активным каналам компании, для которых у пользователя заполнен контакт
// и не отключён opt-in.
//
// Возвращает агрегированную ошибку (если все попытки упали) или nil
// (хотя бы одна успешная отправка / список получателей пуст).
func (s *NotificationService) sendAlertToRecipients(
	companyID uint,
	recipients []alertRecipient,
	cat alertCategory,
	notifType string,
	data map[string]interface{},
	relatedID uint,
	relatedType string,
) error {
	if len(recipients) == 0 {
		s.Logger.Printf("⚠️ Alert %s: получатели не найдены (company=%d)", notifType, companyID)
		return nil
	}

	settings, err := s.GetNotificationSettings(companyID)
	if err != nil {
		return fmt.Errorf("настройки уведомлений: %w", err)
	}

	var errs []string
	sentCount := 0

	for _, r := range recipients {
		if !r.allowsCategory(cat) {
			continue
		}

		if settings.EmailEnabled && r.allowsChannel("email") && r.Email != "" {
			if err := s.SendNotification(notifType, "email", r.Email, data, companyID, relatedID, relatedType); err != nil {
				errs = append(errs, fmt.Sprintf("user=%d email: %v", r.UserID, err))
			} else {
				sentCount++
			}
		}

		if settings.TelegramEnabled && r.allowsChannel("telegram") && r.TelegramID != "" {
			if err := s.SendNotification(notifType, "telegram", r.TelegramID, data, companyID, relatedID, relatedType); err != nil {
				errs = append(errs, fmt.Sprintf("user=%d telegram: %v", r.UserID, err))
			} else {
				sentCount++
			}
		}

		if settings.MaxEnabled && r.allowsChannel("max") && r.MaxID != "" {
			if err := s.SendNotification(notifType, "max", r.MaxID, data, companyID, relatedID, relatedType); err != nil {
				errs = append(errs, fmt.Sprintf("user=%d max: %v", r.UserID, err))
			} else {
				sentCount++
			}
		}
	}

	if sentCount == 0 && len(errs) > 0 {
		return fmt.Errorf("ни одна отправка не удалась: %s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		s.Logger.Printf("⚠️ Alert %s частично доставлен (sent=%d, errors=%d): %s",
			notifType, sentCount, len(errs), strings.Join(errs, "; "))
	}
	return nil
}
