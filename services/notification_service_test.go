package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"backend_axenta/models"
)

// setupNotifTestDB поднимает in-memory SQLite со всеми моделями,
// нужными для тестов NotificationService.
func setupNotifTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&models.Company{},
		&models.Role{},
		&models.User{},
		&models.NotificationTemplate{},
		&models.NotificationLog{},
		&models.NotificationSettings{},
		&models.UserNotificationPreferences{},
		&models.Equipment{},
		&models.WarehouseOperation{},
		&models.StockAlert{},
		&models.EquipmentCategory{},
	))
	return db
}

// newNotifService — сервис с nil telegram/max клиентами (для тестов отправки → fail).
func newNotifService(db *gorm.DB) *NotificationService {
	return NewNotificationService(db, nil, nil, nil)
}

// notifTestCompany создаёт компанию с уникальными schema/domain (можно вызывать несколько раз).
var notifTestCompanyCounter uint = 0

func notifTestCompany(t *testing.T, db *gorm.DB) models.Company {
	t.Helper()
	notifTestCompanyCounter++
	suffix := fmt.Sprintf("%d", notifTestCompanyCounter)
	c := models.Company{
		Name:           "TestCo" + suffix,
		DatabaseSchema: "test_schema_" + suffix,
		Domain:         "co" + suffix + ".test",
		AxetnaLogin:    "login" + suffix,
		AxetnaPassword: "pwd",
	}
	require.NoError(t, db.Create(&c).Error)
	return c
}

// notifEnableAllSettings создаёт NotificationSettings с включёнными email/telegram/max.
func notifEnableAllSettings(t *testing.T, db *gorm.DB, companyID uint) {
	t.Helper()
	require.NoError(t, db.Create(&models.NotificationSettings{
		CompanyID:        companyID,
		EmailEnabled:     true,
		TelegramEnabled:  true,
		MaxEnabled:       true,
		SMTPHost:         "smtp.invalid",
		SMTPPort:         587,
		SMTPFromEmail:    "noreply@invalid",
		MaxRetryAttempts: 3,
	}).Error)
}

// =====================================================================
// SendNotification: валидация и побочные эффекты (логи)
// =====================================================================

func TestSendNotification_EmptyRecipient(t *testing.T) {
	db := setupNotifTestDB(t)
	svc := newNotifService(db)

	err := svc.SendNotification("billing_alert", "email", "", nil, 1, 0, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "пустой recipient")
}

func TestSendNotification_UnknownChannel(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	// Шаблон для несуществующего канала — иначе SendNotification упадёт раньше
	// на findTemplate, не дойдя до switch с проверкой канала.
	require.NoError(t, db.Create(&models.NotificationTemplate{
		Name: "smoke", Type: "ping", Channel: "smoke-signal",
		Template: "x", IsActive: true, CompanyID: c.ID,
	}).Error)

	err := svc.SendNotification("ping", "smoke-signal", "x", nil, c.ID, 0, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неизвестный канал")
}

func TestSendNotification_TelegramDisabledOnCompanyLevel(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	// settings со всеми отключёнными каналами (default)
	require.NoError(t, db.Create(&models.NotificationSettings{CompanyID: c.ID}).Error)
	svc := newNotifService(db)

	err := svc.SendNotification("billing_alert", "telegram", "12345", nil, c.ID, 0, "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "telegram канал отключён")
}

func TestSendNotification_FailedSendIsLogged(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	// telegram включён, но клиент = nil → отправка провалится
	_ = svc.SendNotification("billing_alert", "telegram", "12345",
		map[string]interface{}{"alert_type": "overdue", "message": "оплата просрочена", "company_id": c.ID},
		c.ID, 42, "billing")

	var log models.NotificationLog
	require.NoError(t, db.Where("company_id = ?", c.ID).First(&log).Error)
	assert.Equal(t, "telegram", log.Channel)
	assert.Equal(t, "12345", log.Recipient)
	assert.Equal(t, "failed", log.Status)
	assert.NotEmpty(t, log.ErrorMessage)
	assert.Equal(t, "billing", log.RelatedType)
	require.NotNil(t, log.RelatedID)
	assert.Equal(t, uint(42), *log.RelatedID)
	// Builtin шаблон отрендерил подстановку
	assert.Contains(t, log.Message, "оплата просрочена")
}

// =====================================================================
// findTemplate: company-specific → global → builtin fallback
// =====================================================================

func TestFindTemplate_PrefersCompanySpecific(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	svc := newNotifService(db)

	// Глобальный (company_id = 0)
	require.NoError(t, db.Create(&models.NotificationTemplate{
		Name: "global_t", Type: "billing_alert", Channel: "email",
		Subject: "GLOBAL", Template: "global body", IsActive: true, CompanyID: 0,
	}).Error)
	// Company-specific
	require.NoError(t, db.Create(&models.NotificationTemplate{
		Name: "company_t", Type: "billing_alert", Channel: "email",
		Subject: "CO", Template: "company body", IsActive: true, CompanyID: c.ID,
	}).Error)

	tmpl, err := svc.findTemplate("billing_alert", "email", c.ID)
	require.NoError(t, err)
	assert.Equal(t, "CO", tmpl.Subject)
}

func TestFindTemplate_FallsBackToBuiltin(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	svc := newNotifService(db)

	tmpl, err := svc.findTemplate("billing_alert", "email", c.ID)
	require.NoError(t, err)
	assert.Equal(t, "billing_alert", tmpl.Type)
	assert.Equal(t, "builtin_billing_alert_email", tmpl.Name)
}

// =====================================================================
// GetNotificationLogs: пагинация, фильтры, изоляция по company
// =====================================================================

func TestGetNotificationLogs_FiltersAndIsolation(t *testing.T) {
	db := setupNotifTestDB(t)
	c1 := notifTestCompany(t, db)
	c2 := notifTestCompany(t, db)
	svc := newNotifService(db)

	mk := func(companyID uint, notifType, channel, status string) {
		require.NoError(t, db.Create(&models.NotificationLog{
			Type: notifType, Channel: channel, Recipient: "r", Status: status,
			CompanyID: companyID, RelatedType: "test", AttemptCount: 1,
		}).Error)
	}
	mk(c1.ID, "billing_alert", "email", "sent")
	mk(c1.ID, "billing_alert", "telegram", "failed")
	mk(c1.ID, "stock_alert", "email", "sent")
	mk(c2.ID, "billing_alert", "email", "sent") // другая компания, не должно попадать

	t.Run("isolates by company", func(t *testing.T) {
		_, total, err := svc.GetNotificationLogs(50, 0, nil, c1.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
	})

	t.Run("filter by type", func(t *testing.T) {
		logs, total, err := svc.GetNotificationLogs(50, 0, map[string]interface{}{"type": "stock_alert"}, c1.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, logs, 1)
		assert.Equal(t, "stock_alert", logs[0].Type)
	})

	t.Run("filter by channel + status", func(t *testing.T) {
		logs, total, err := svc.GetNotificationLogs(50, 0, map[string]interface{}{
			"channel": "telegram", "status": "failed",
		}, c1.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, logs, 1)
		assert.Equal(t, "telegram", logs[0].Channel)
	})

	t.Run("ignores non-whitelisted filters", func(t *testing.T) {
		// recipient не в whitelist — фильтр должен игнорироваться
		_, total, err := svc.GetNotificationLogs(50, 0, map[string]interface{}{"recipient": "nope"}, c1.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
	})

	t.Run("pagination", func(t *testing.T) {
		page1, _, err := svc.GetNotificationLogs(2, 0, nil, c1.ID)
		require.NoError(t, err)
		assert.Len(t, page1, 2)
		page2, _, err := svc.GetNotificationLogs(2, 2, nil, c1.ID)
		require.NoError(t, err)
		assert.Len(t, page2, 1)
	})
}

// =====================================================================
// GetNotificationStatistics: группировка по channel × status
// =====================================================================

func TestGetNotificationStatistics_Aggregates(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	svc := newNotifService(db)

	for _, st := range []string{"sent", "sent", "failed"} {
		require.NoError(t, db.Create(&models.NotificationLog{
			Type: "billing_alert", Channel: "email", Recipient: "r", Status: st, CompanyID: c.ID, AttemptCount: 1,
		}).Error)
	}
	require.NoError(t, db.Create(&models.NotificationLog{
		Type: "billing_alert", Channel: "telegram", Recipient: "r", Status: "sent", CompanyID: c.ID, AttemptCount: 1,
	}).Error)

	stats, err := svc.GetNotificationStatistics(c.ID)
	require.NoError(t, err)

	// by_channel_status — anonymous struct slice. Прогоняем через JSON,
	// чтобы не зависеть от приватного типа.
	raw, err := json.Marshal(stats["by_channel_status"])
	require.NoError(t, err)

	var rows []struct {
		Channel string `json:"Channel"`
		Status  string `json:"Status"`
		Count   int64  `json:"Count"`
	}
	require.NoError(t, json.Unmarshal(raw, &rows))

	totals := map[string]int64{}
	for _, r := range rows {
		totals[r.Channel+"_"+r.Status] = r.Count
	}
	assert.Equal(t, int64(2), totals["email_sent"])
	assert.Equal(t, int64(1), totals["email_failed"])
	assert.Equal(t, int64(1), totals["telegram_sent"])
}

// =====================================================================
// resolveRecipientsByRoles: JOIN ролей, фильтры активности и компании
// =====================================================================

type recipientFixture struct {
	companyID uint
	adminID   uint
	techID    uint
	otherID   uint
}

func seedRecipientFixture(t *testing.T, db *gorm.DB) recipientFixture {
	t.Helper()
	c := notifTestCompany(t, db)
	other := notifTestCompany(t, db)

	rolesByName := map[string]uint{}
	for _, name := range []string{"admin", "tech", "manager"} {
		r := models.Role{Name: name, DisplayName: name}
		require.NoError(t, db.Create(&r).Error)
		rolesByName[name] = r.ID
	}

	mkUser := func(name string, roleName string, companyID uint, active bool) uint {
		rid := rolesByName[roleName]
		u := models.User{
			Username: name, Email: name + "@x.com", Password: "p",
			TelegramID: "tg_" + name, MaxID: "max_" + name,
			IsActive: active, CompanyID: companyID, RoleID: &rid,
		}
		require.NoError(t, db.Create(&u).Error)
		// GORM не пишет zero-value (false) при наличии default:true — обновим явно.
		require.NoError(t, db.Model(&u).Update("is_active", active).Error)
		return u.ID
	}

	return recipientFixture{
		companyID: c.ID,
		adminID:   mkUser("alice", "admin", c.ID, true),
		techID:    mkUser("bob", "tech", c.ID, true),
		otherID: func() uint {
			// Менеджер той же компании — не должен попадать в admin/tech
			mkUser("mary", "manager", c.ID, true)
			// Inactive admin — не должен попадать
			mkUser("ghost", "admin", c.ID, false)
			// Admin другой компании — не должен попадать
			return mkUser("foreign", "admin", other.ID, true)
		}(),
	}
}

func TestResolveRecipientsByRoles_FiltersByRoleCompanyAndActive(t *testing.T) {
	db := setupNotifTestDB(t)
	svc := newNotifService(db)
	f := seedRecipientFixture(t, db)

	recs, err := svc.resolveRecipientsByRoles(f.companyID, []string{models.RoleAdmin, models.RoleTech})
	require.NoError(t, err)

	ids := map[uint]bool{}
	for _, r := range recs {
		ids[r.UserID] = true
	}
	assert.True(t, ids[f.adminID], "admin должен быть в выборке")
	assert.True(t, ids[f.techID], "tech должен быть в выборке")
	assert.False(t, ids[f.otherID], "admin другой компании не должен попадать")
	assert.Len(t, recs, 2, "только активные admin+tech своей компании")
}

func TestResolveRecipientsByRoles_EmptyRolesError(t *testing.T) {
	db := setupNotifTestDB(t)
	svc := newNotifService(db)
	_, err := svc.resolveRecipientsByRoles(1, nil)
	assert.Error(t, err)
}

// =====================================================================
// allowsCategory / allowsChannel: per-user opt-in
// =====================================================================

func TestAlertRecipient_AllowsCategoryDefaults(t *testing.T) {
	r := alertRecipient{} // prefs nil
	assert.True(t, r.allowsCategory(alertCategoryBilling))
	assert.True(t, r.allowsCategory(alertCategoryWarehouse))
	assert.True(t, r.allowsChannel("email"))
	assert.True(t, r.allowsChannel("telegram"))
	assert.True(t, r.allowsChannel("max"))
}

func TestAlertRecipient_AllowsCategoryRespectsPrefs(t *testing.T) {
	r := alertRecipient{
		Prefs: &models.UserNotificationPreferences{
			BillingAlerts:   false,
			WarehouseAlerts: true,
			EmailEnabled:    false,
			TelegramEnabled: true,
		},
	}
	assert.False(t, r.allowsCategory(alertCategoryBilling))
	assert.True(t, r.allowsCategory(alertCategoryWarehouse))
	assert.False(t, r.allowsChannel("email"))
	assert.True(t, r.allowsChannel("telegram"))
}

// =====================================================================
// sendAlertToRecipients: end-to-end, проверка фильтров и логов
// =====================================================================

func TestSendAlertToRecipients_NoRecipientsReturnsNil(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	err := svc.sendAlertToRecipients(c.ID, nil, alertCategoryBilling,
		"billing_alert", map[string]interface{}{"alert_type": "x", "message": "y"}, 0, "billing")
	assert.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.NotificationLog{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSendAlertToRecipients_SkipsCategoryOptOut(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	recs := []alertRecipient{
		{
			UserID: 1, Email: "a@x.com", TelegramID: "tg_a",
			Prefs: &models.UserNotificationPreferences{BillingAlerts: false},
		},
	}
	err := svc.sendAlertToRecipients(c.ID, recs, alertCategoryBilling,
		"billing_alert", map[string]interface{}{"alert_type": "x", "message": "y"}, 0, "billing")
	assert.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.NotificationLog{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "получатель отписан от категории — лог писаться не должен")
}

func TestSendAlertToRecipients_SkipsChannelOptOut(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	// EmailEnabled=false → email скипается, telegram попытается отправить (упадёт без клиента → лог fail)
	recs := []alertRecipient{
		{
			UserID: 1, Email: "a@x.com", TelegramID: "tg_a",
			Prefs: &models.UserNotificationPreferences{
				BillingAlerts:   true,
				EmailEnabled:    false,
				TelegramEnabled: true,
			},
		},
	}
	err := svc.sendAlertToRecipients(c.ID, recs, alertCategoryBilling,
		"billing_alert", map[string]interface{}{"alert_type": "x", "message": "y"}, 0, "billing")
	// sentCount=0 + errs (telegram fail) → агрегированная ошибка
	assert.Error(t, err)

	var logs []models.NotificationLog
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1, "должна быть только telegram-запись (email отключён)")
	assert.Equal(t, "telegram", logs[0].Channel)
}

// =====================================================================
// SendBillingAlert / SendStockAlert: end-to-end resolve + send
// =====================================================================

func TestSendBillingAlert_NoMatchingRolesReturnsNil(t *testing.T) {
	db := setupNotifTestDB(t)
	c := notifTestCompany(t, db)
	notifEnableAllSettings(t, db, c.ID)
	svc := newNotifService(db)

	err := svc.SendBillingAlert(c.ID, "overdue", "оплата просрочена")
	assert.NoError(t, err, "нет admin/accountant — не ошибка, просто пусто")

	var count int64
	require.NoError(t, db.Model(&models.NotificationLog{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestSendBillingAlert_E2E_LogsForAdmin(t *testing.T) {
	db := setupNotifTestDB(t)
	svc := newNotifService(db)
	f := seedRecipientFixture(t, db)
	notifEnableAllSettings(t, db, f.companyID)

	err := svc.SendBillingAlert(f.companyID, "overdue", "оплата просрочена")
	// Все каналы упадут (нет SMTP/telegram/max клиентов) → агрегированная ошибка
	assert.Error(t, err)

	var logs []models.NotificationLog
	require.NoError(t, db.Where("company_id = ?", f.companyID).Find(&logs).Error)
	// admin (alice) + tech (bob) — нет, только admin/accountant попадают для billing
	// alice — admin, bob — tech (не accountant), значит только alice
	// 3 канала × 1 пользователь = 3 лога
	assert.Equal(t, 3, len(logs), "ожидалось 3 лога: email + telegram + max для admin alice")
	for _, l := range logs {
		assert.Equal(t, "failed", l.Status)
		assert.Contains(t, l.Message, "оплата просрочена")
	}
}

func TestSendStockAlert_E2E_LogsForAdminAndTech(t *testing.T) {
	db := setupNotifTestDB(t)
	svc := newNotifService(db)
	f := seedRecipientFixture(t, db)
	// Включаем только email (упрощение)
	require.NoError(t, db.Create(&models.NotificationSettings{
		CompanyID:    f.companyID,
		EmailEnabled: true,
		SMTPHost:     "smtp.invalid",
	}).Error)

	alert := models.StockAlert{
		Type: "low_stock", Title: "Мало GPS-трекеров", Description: "осталось 2",
		Severity: "high", Status: "active", CompanyID: f.companyID,
	}
	require.NoError(t, db.Create(&alert).Error)

	err := svc.SendStockAlert(alert)
	assert.Error(t, err) // SMTP fail

	var logs []models.NotificationLog
	require.NoError(t, db.Where("company_id = ? AND type = ?", f.companyID, "stock_alert").Find(&logs).Error)
	assert.Equal(t, 2, len(logs), "admin + tech, 1 канал (email) = 2 лога")
	for _, l := range logs {
		assert.Equal(t, "email", l.Channel)
		assert.Equal(t, "failed", l.Status)
		assert.Contains(t, l.Subject, "Мало GPS-трекеров")
	}
}
