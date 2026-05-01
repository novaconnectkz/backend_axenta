package services

import (
	"errors"

	"backend_axenta/models"
)

// defaultTemplate описывает встроенный шаблон-fallback.
// Используется когда в БД нет company-specific и global записи.
type defaultTemplate struct {
	NotifType string
	Channel   string
	Subject   string
	Body      string
}

// builtinTemplates — minимальный набор шаблонов для базовых событий.
// Доступен сразу без сидинга. Может быть переопределён через
// NotificationTemplate в БД (с company_id или company_id=0 для global).
var builtinTemplates = []defaultTemplate{
	// === Installation events ===
	{
		NotifType: "installation_created",
		Channel:   "email",
		Subject:   "Новый монтаж #{{.installation_id}} — {{.scheduled_at}}",
		Body: `<p>Здравствуйте, {{.installer_full_name}}!</p>
<p>Вам назначен новый монтаж:</p>
<ul>
<li>Тип: <strong>{{.installation_type}}</strong></li>
<li>Дата: <strong>{{.scheduled_at}}</strong></li>
<li>Адрес: {{.address}}</li>
<li>Контакт клиента: {{.client_contact}}</li>
<li>Длительность (план): {{.estimated_duration}} мин</li>
</ul>
<p>{{.description}}</p>`,
	},
	{
		NotifType: "installation_created",
		Channel:   "telegram",
		Body: `🔧 *Новый монтаж #{{.installation_id}}*
Тип: {{.installation_type}}
Дата: {{.scheduled_at}}
Адрес: {{.address}}
Клиент: {{.client_contact}}
Длительность: {{.estimated_duration}} мин`,
	},
	{
		NotifType: "installation_updated",
		Channel:   "email",
		Subject:   "Монтаж #{{.installation_id}} обновлён",
		Body: `<p>Изменены данные монтажа #{{.installation_id}}:</p>
<ul>
<li>Тип: {{.installation_type}}</li>
<li>Дата: <strong>{{.scheduled_at}}</strong></li>
<li>Статус: {{.status}}</li>
<li>Адрес: {{.address}}</li>
</ul>`,
	},
	{
		NotifType: "installation_updated",
		Channel:   "telegram",
		Body: `📝 Монтаж #{{.installation_id}} обновлён
Дата: {{.scheduled_at}}
Статус: {{.status}}`,
	},
	{
		NotifType: "installation_completed",
		Channel:   "email",
		Subject:   "Монтаж #{{.installation_id}} завершён",
		Body: `<p>Монтаж #{{.installation_id}} ({{.installation_type}}) завершён.</p>
<p>Адрес: {{.address}}</p>
<p>Спасибо за работу!</p>`,
	},
	{
		NotifType: "installation_completed",
		Channel:   "telegram",
		Body:      `✅ Монтаж #{{.installation_id}} завершён. Спасибо!`,
	},
	{
		NotifType: "installation_cancelled",
		Channel:   "email",
		Subject:   "Монтаж #{{.installation_id}} отменён",
		Body: `<p>Монтаж #{{.installation_id}} отменён.</p>
<p>Дата: {{.scheduled_at}}, адрес: {{.address}}.</p>`,
	},
	{
		NotifType: "installation_cancelled",
		Channel:   "telegram",
		Body:      `❌ Монтаж #{{.installation_id}} отменён ({{.scheduled_at}}, {{.address}})`,
	},
	{
		NotifType: "installation_rescheduled",
		Channel:   "email",
		Subject:   "Монтаж #{{.installation_id}} перенесён",
		Body: `<p>Монтаж #{{.installation_id}} перенесён.</p>
<p>Старая дата: {{.old_scheduled_at}}<br>Новая дата: <strong>{{.scheduled_at}}</strong></p>
<p>Адрес: {{.address}}</p>`,
	},
	{
		NotifType: "installation_rescheduled",
		Channel:   "telegram",
		Body: `🔄 Монтаж #{{.installation_id}} перенесён
{{.old_scheduled_at}} → *{{.scheduled_at}}*
Адрес: {{.address}}`,
	},
	{
		NotifType: "installation_reminder",
		Channel:   "email",
		Subject:   "Напоминание о монтаже #{{.installation_id}} — {{.scheduled_at}}",
		Body: `<p>Напоминаем о предстоящем монтаже:</p>
<ul>
<li>#{{.installation_id}} ({{.installation_type}})</li>
<li>Дата: <strong>{{.scheduled_at}}</strong></li>
<li>Адрес: {{.address}}</li>
<li>Контакт клиента: {{.client_contact}}</li>
</ul>`,
	},
	{
		NotifType: "installation_reminder",
		Channel:   "telegram",
		Body: `⏰ Напоминание: монтаж #{{.installation_id}}
{{.scheduled_at}} — {{.address}}
Клиент: {{.client_contact}}`,
	},
}

// findBuiltinTemplate возвращает builtin-шаблон по type+channel или nil.
func findBuiltinTemplate(notifType, channel string) *models.NotificationTemplate {
	for _, t := range builtinTemplates {
		if t.NotifType == notifType && t.Channel == channel {
			return &models.NotificationTemplate{
				Name:     "builtin_" + notifType + "_" + channel,
				Type:     notifType,
				Channel:  channel,
				Subject:  t.Subject,
				Template: t.Body,
				IsActive: true,
				Language: "ru",
				Priority: "normal",
			}
		}
	}
	return nil
}

// SeedBuiltinTemplates записывает builtin-шаблоны в БД с company_id = companyID.
// Если шаблон с таким Name уже есть — пропускает. Используется CreateDefaultTemplates.
func (s *NotificationService) seedBuiltinTemplates(companyID uint) error {
	if s.DB == nil {
		return errors.New("БД не подключена")
	}

	for _, t := range builtinTemplates {
		name := "builtin_" + t.NotifType + "_" + t.Channel
		var existing models.NotificationTemplate
		err := s.DB.Where("name = ? AND company_id = ?", name, companyID).First(&existing).Error
		if err == nil {
			continue // уже есть
		}

		tmpl := models.NotificationTemplate{
			Name:      name,
			Type:      t.NotifType,
			Channel:   t.Channel,
			Subject:   t.Subject,
			Template:  t.Body,
			IsActive:  true,
			Language:  "ru",
			Priority:  "normal",
			CompanyID: companyID,
		}
		if err := s.DB.Create(&tmpl).Error; err != nil {
			return err
		}
	}
	return nil
}
