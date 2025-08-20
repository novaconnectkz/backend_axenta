package models

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestDatabaseMigrations тестирует миграции базы данных
func TestDatabaseMigrations(t *testing.T) {
	t.Run("Миграция всех моделей", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Список всех моделей для миграции
		models := []interface{}{
			&Company{},
			&BillingPlan{},
			&Subscription{},
			&Permission{},
			&Role{},
			&User{},
			&UserTemplate{},
			&ObjectTemplate{},
			&MonitoringTemplate{},
			&NotificationTemplate{},
			&Object{},
			&Location{},
			&Installer{},
			&Equipment{},
			&Installation{},
			&Contract{},
			&ContractAppendix{},
			&TariffPlan{},
		}

		// Выполняем миграцию всех моделей
		err = db.AutoMigrate(models...)
		require.NoError(t, err)

		// Проверяем, что все таблицы созданы
		for _, model := range models {
			assert.True(t, db.Migrator().HasTable(model), "Таблица для модели %T должна существовать", model)
		}
	})

	t.Run("Проверка индексов", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(
			&User{},
			&Object{},
			&Contract{},
			&Equipment{},
			&Company{},
			&Role{},
			&Permission{},
		)
		require.NoError(t, err)

		// Проверяем индексы для User
		assert.True(t, db.Migrator().HasIndex(&User{}, "username"))
		assert.True(t, db.Migrator().HasIndex(&User{}, "email"))
		assert.True(t, db.Migrator().HasIndex(&User{}, "role_id"))

		// Проверяем индексы для Object
		assert.True(t, db.Migrator().HasIndex(&Object{}, "imei"))
		assert.True(t, db.Migrator().HasIndex(&Object{}, "contract_id"))
		assert.True(t, db.Migrator().HasIndex(&Object{}, "location_id"))

		// Проверяем индексы для Contract
		assert.True(t, db.Migrator().HasIndex(&Contract{}, "number"))

		// Проверяем индексы для Equipment
		assert.True(t, db.Migrator().HasIndex(&Equipment{}, "serial_number"))
		assert.True(t, db.Migrator().HasIndex(&Equipment{}, "imei"))
	})

	t.Run("Проверка внешних ключей", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(
			&Role{},
			&User{},
			&UserTemplate{},
			&BillingPlan{},
			&Contract{},
			&Object{},
			&Location{},
			&Installer{},
			&Installation{},
			&Equipment{},
		)
		require.NoError(t, err)

		// Проверяем внешние ключи для User
		assert.True(t, db.Migrator().HasConstraint(&User{}, "Role"))
		assert.True(t, db.Migrator().HasConstraint(&User{}, "Template"))

		// Проверяем внешние ключи для Object
		assert.True(t, db.Migrator().HasConstraint(&Object{}, "Contract"))
		assert.True(t, db.Migrator().HasConstraint(&Object{}, "Location"))

		// Проверяем внешние ключи для Installation
		assert.True(t, db.Migrator().HasConstraint(&Installation{}, "Object"))
		assert.True(t, db.Migrator().HasConstraint(&Installation{}, "Installer"))
	})

	t.Run("Проверка типов колонок", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(&User{}, &Object{}, &Contract{})
		require.NoError(t, err)

		// Проверяем колонки User
		assert.True(t, db.Migrator().HasColumn(&User{}, "username"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "email"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "password"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "first_name"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "last_name"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "is_active"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "role_id"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "template_id"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "last_login"))
		assert.True(t, db.Migrator().HasColumn(&User{}, "login_count"))

		// Проверяем колонки Object
		assert.True(t, db.Migrator().HasColumn(&Object{}, "name"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "type"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "description"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "latitude"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "longitude"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "address"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "imei"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "phone_number"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "serial_number"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "status"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "is_active"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "scheduled_delete_at"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "last_activity_at"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "contract_id"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "template_id"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "location_id"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "settings"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "tags"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "notes"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "external_id"))

		// Проверяем колонки Contract
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "number"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "title"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "description"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_name"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_inn"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_kpp"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_email"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_phone"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "client_address"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "start_date"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "end_date"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "signed_at"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "tariff_plan_id"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "total_amount"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "currency"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "status"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "is_active"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "notify_before"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "notes"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "external_id"))
	})

	t.Run("Проверка many-to-many связей", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(
			&Role{},
			&Permission{},
			&Location{},
			&Installer{},
			&Installation{},
			&Equipment{},
		)
		require.NoError(t, err)

		// Проверяем таблицу связи role_permissions
		assert.True(t, db.Migrator().HasTable("role_permissions"))

		// Проверяем таблицу связи installer_locations
		assert.True(t, db.Migrator().HasTable("installer_locations"))

		// Проверяем таблицу связи installation_equipment
		assert.True(t, db.Migrator().HasTable("installation_equipment"))
	})

	t.Run("Тест миграции с существующими данными", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Первая миграция - только базовые модели
		err = db.AutoMigrate(&Company{}, &BillingPlan{}, &Role{}, &Permission{})
		require.NoError(t, err)

		// Добавляем тестовые данные
		company := Company{
			Name:           "Test Company",
			DatabaseSchema: "test_schema",
			AxetnaLogin:    "test",
			AxetnaPassword: "password",
			ContactEmail:   "test@example.com",
			IsActive:       true,
		}
		err = db.Create(&company).Error
		require.NoError(t, err)

		role := Role{
			Name:        "admin",
			DisplayName: "Administrator",
			IsActive:    true,
		}
		err = db.Create(&role).Error
		require.NoError(t, err)

		permission := Permission{
			Name:        "users.read",
			DisplayName: "Read Users",
			Resource:    "users",
			Action:      "read",
			IsActive:    true,
		}
		err = db.Create(&permission).Error
		require.NoError(t, err)

		// Вторая миграция - добавляем остальные модели
		err = db.AutoMigrate(
			&User{},
			&Object{},
			&Contract{},
			&Location{},
			&Equipment{},
		)
		require.NoError(t, err)

		// Проверяем, что старые данные сохранились
		var savedCompany Company
		err = db.First(&savedCompany, company.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "Test Company", savedCompany.Name)

		var savedRole Role
		err = db.First(&savedRole, role.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "admin", savedRole.Name)

		var savedPermission Permission
		err = db.First(&savedPermission, permission.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "users.read", savedPermission.Name)

		// Проверяем, что новые таблицы созданы
		assert.True(t, db.Migrator().HasTable(&User{}))
		assert.True(t, db.Migrator().HasTable(&Object{}))
		assert.True(t, db.Migrator().HasTable(&Contract{}))
	})

	t.Run("Тест soft delete", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(&User{}, &Role{}, &Object{}, &Contract{}, &BillingPlan{})
		require.NoError(t, err)

		// Проверяем наличие колонки deleted_at для soft delete
		assert.True(t, db.Migrator().HasColumn(&User{}, "deleted_at"))
		assert.True(t, db.Migrator().HasColumn(&Role{}, "deleted_at"))
		assert.True(t, db.Migrator().HasColumn(&Object{}, "deleted_at"))
		assert.True(t, db.Migrator().HasColumn(&Contract{}, "deleted_at"))

		// Создаем тестовые данные
		role := Role{
			Name:        "test_role",
			DisplayName: "Test Role",
			IsActive:    true,
		}
		err = db.Create(&role).Error
		require.NoError(t, err)

		user := User{
			Username: "test_user",
			Email:    "test@example.com",
			Password: "password",
			RoleID:   role.ID,
			IsActive: true,
		}
		err = db.Create(&user).Error
		require.NoError(t, err)

		// Тестируем soft delete
		err = db.Delete(&user).Error
		require.NoError(t, err)

		// Проверяем, что запись не найдена обычным запросом
		var foundUser User
		err = db.First(&foundUser, user.ID).Error
		assert.Error(t, err)

		// Проверяем, что запись найдена с Unscoped
		err = db.Unscoped().First(&foundUser, user.ID).Error
		require.NoError(t, err)
		assert.NotNil(t, foundUser.DeletedAt)
	})

	t.Run("Тест JSONB полей", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(
			&Object{},
			&UserTemplate{},
			&ObjectTemplate{},
			&MonitoringTemplate{},
			&NotificationTemplate{},
			&Equipment{},
		)
		require.NoError(t, err)

		// Проверяем наличие JSONB полей
		assert.True(t, db.Migrator().HasColumn(&Object{}, "settings"))
		assert.True(t, db.Migrator().HasColumn(&UserTemplate{}, "settings"))
		assert.True(t, db.Migrator().HasColumn(&ObjectTemplate{}, "config"))
		assert.True(t, db.Migrator().HasColumn(&ObjectTemplate{}, "default_settings"))
		assert.True(t, db.Migrator().HasColumn(&MonitoringTemplate{}, "settings"))
		assert.True(t, db.Migrator().HasColumn(&NotificationTemplate{}, "variables"))
		assert.True(t, db.Migrator().HasColumn(&Equipment{}, "specifications"))
	})

	t.Run("Тест массивных полей", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = db.AutoMigrate(
			&Object{},
			&ObjectTemplate{},
			&Installer{},
			&Installation{},
		)
		require.NoError(t, err)

		// Проверяем наличие массивных полей
		assert.True(t, db.Migrator().HasColumn(&Object{}, "tags"))
		assert.True(t, db.Migrator().HasColumn(&ObjectTemplate{}, "required_equipment"))
		assert.True(t, db.Migrator().HasColumn(&Installer{}, "specialization"))
		assert.True(t, db.Migrator().HasColumn(&Installer{}, "location_ids"))
		assert.True(t, db.Migrator().HasColumn(&Installation{}, "photos"))
	})
}

// TestSchemaIsolation тестирует изоляцию схем для мультитенантности
func TestSchemaIsolation(t *testing.T) {
	t.Run("Симуляция создания схем компаний", func(t *testing.T) {
		// Создаем основную БД
		mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Мигрируем глобальные таблицы
		err = mainDB.AutoMigrate(&Company{}, &BillingPlan{})
		require.NoError(t, err)

		// Создаем компании
		company1 := Company{
			Name:           "Company 1",
			DatabaseSchema: "tenant_1",
			AxetnaLogin:    "login1",
			AxetnaPassword: "pass1",
			ContactEmail:   "company1@test.com",
			IsActive:       true,
		}
		company2 := Company{
			Name:           "Company 2",
			DatabaseSchema: "tenant_2",
			AxetnaLogin:    "login2",
			AxetnaPassword: "pass2",
			ContactEmail:   "company2@test.com",
			IsActive:       true,
		}

		err = mainDB.Create(&company1).Error
		require.NoError(t, err)
		err = mainDB.Create(&company2).Error
		require.NoError(t, err)

		// Симулируем создание отдельных БД для каждой компании
		// (В реальной системе это будут схемы PostgreSQL)
		tenant1DB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		tenant2DB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		// Мигрируем мультитенантные модели для каждой компании
		tenantModels := []interface{}{
			&Permission{},
			&Role{},
			&User{},
			&UserTemplate{},
			&ObjectTemplate{},
			&MonitoringTemplate{},
			&NotificationTemplate{},
			&Object{},
			&Location{},
			&Installer{},
			&Installation{},
			&Equipment{},
			&Contract{},
			&ContractAppendix{},
			&TariffPlan{},
			&Subscription{},
		}

		err = tenant1DB.AutoMigrate(tenantModels...)
		require.NoError(t, err)
		err = tenant2DB.AutoMigrate(tenantModels...)
		require.NoError(t, err)

		// Создаем тестовые данные в каждой схеме
		role1 := Role{Name: "admin1", DisplayName: "Admin 1", IsActive: true}
		role2 := Role{Name: "admin2", DisplayName: "Admin 2", IsActive: true}

		err = tenant1DB.Create(&role1).Error
		require.NoError(t, err)
		err = tenant2DB.Create(&role2).Error
		require.NoError(t, err)

		user1 := User{
			Username: "user1@company1.com",
			Email:    "user1@company1.com",
			Password: "password1",
			RoleID:   role1.ID,
			IsActive: true,
		}
		user2 := User{
			Username: "user2@company2.com",
			Email:    "user2@company2.com",
			Password: "password2",
			RoleID:   role2.ID,
			IsActive: true,
		}

		err = tenant1DB.Create(&user1).Error
		require.NoError(t, err)
		err = tenant2DB.Create(&user2).Error
		require.NoError(t, err)

		// Проверяем изоляцию данных
		var users1, users2 []User
		err = tenant1DB.Find(&users1).Error
		require.NoError(t, err)
		err = tenant2DB.Find(&users2).Error
		require.NoError(t, err)

		assert.Len(t, users1, 1)
		assert.Len(t, users2, 1)
		assert.Equal(t, "user1@company1.com", users1[0].Username)
		assert.Equal(t, "user2@company2.com", users2[0].Username)

		// Проверяем, что пользователи не видят друг друга
		var countInTenant1, countInTenant2 int64
		tenant1DB.Model(&User{}).Where("username = ?", "user2@company2.com").Count(&countInTenant1)
		tenant2DB.Model(&User{}).Where("username = ?", "user1@company1.com").Count(&countInTenant2)

		assert.Equal(t, int64(0), countInTenant1)
		assert.Equal(t, int64(0), countInTenant2)
	})

	t.Run("Тест целостности связей между схемами", func(t *testing.T) {
		// Создаем основную БД с глобальными таблицами
		mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = mainDB.AutoMigrate(&Company{}, &BillingPlan{})
		require.NoError(t, err)

		// Создаем глобальный тарифный план
		globalPlan := BillingPlan{
			Name:          "Global Plan",
			Description:   "Plan available for all companies",
			Price:         decimal.NewFromFloat(1000.0),
			Currency:      "RUB",
			BillingPeriod: "monthly",
			IsActive:      true,
		}
		err = mainDB.Create(&globalPlan).Error
		require.NoError(t, err)

		// Создаем компанию
		company := Company{
			Name:           "Test Company",
			DatabaseSchema: "test_tenant",
			AxetnaLogin:    "test_login",
			AxetnaPassword: "test_pass",
			ContactEmail:   "test@company.com",
			IsActive:       true,
		}
		err = mainDB.Create(&company).Error
		require.NoError(t, err)

		// Создаем тенантную БД
		tenantDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		err = tenantDB.AutoMigrate(&Contract{}, &TariffPlan{})
		require.NoError(t, err)

		// Создаем тарифный план в тенантной БД (наследует от глобального)
		tenantPlan := TariffPlan{
			BillingPlan: BillingPlan{
				Name:          "Tenant Specific Plan",
				Description:   "Plan specific to this tenant",
				Price:         decimal.NewFromFloat(1500.0),
				Currency:      "RUB",
				BillingPeriod: "monthly",
				IsActive:      true,
				CompanyID:     &company.ID, // Привязка к компании
			},
			SetupFee:         decimal.NewFromFloat(500.0),
			MinimumPeriod:    3,
			PricePerObject:   decimal.NewFromFloat(100.0),
			FreeObjectsCount: 5,
		}
		err = tenantDB.Create(&tenantPlan).Error
		require.NoError(t, err)

		// Создаем договор с использованием тарифного плана
		contract := Contract{
			Number:       "TEST-001",
			Title:        "Test Contract",
			ClientName:   "Test Client",
			StartDate:    time.Now(),
			EndDate:      time.Now().AddDate(1, 0, 0),
			TariffPlanID: tenantPlan.ID,
			TotalAmount:  decimal.NewFromFloat(18000.0),
			Status:       "active",
			IsActive:     true,
		}
		err = tenantDB.Create(&contract).Error
		require.NoError(t, err)

		// Проверяем связи
		var contractWithPlan Contract
		err = tenantDB.Preload("TariffPlan").First(&contractWithPlan, contract.ID).Error
		require.NoError(t, err)
		assert.Equal(t, "Tenant Specific Plan", contractWithPlan.TariffPlan.Name)
		assert.Equal(t, company.ID, contractWithPlan.TariffPlan.CompanyID)
	})
}

// TestMigrationPerformance тестирует производительность миграций
func TestMigrationPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	t.Run("Производительность миграции большого количества таблиц", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		allModels := []interface{}{
			&Company{},
			&BillingPlan{},
			&Subscription{},
			&Permission{},
			&Role{},
			&User{},
			&UserTemplate{},
			&ObjectTemplate{},
			&MonitoringTemplate{},
			&NotificationTemplate{},
			&Object{},
			&Location{},
			&Installer{},
			&Installation{},
			&Equipment{},
			&Contract{},
			&ContractAppendix{},
			&TariffPlan{},
		}

		start := time.Now()
		err = db.AutoMigrate(allModels...)
		duration := time.Since(start)

		require.NoError(t, err)
		t.Logf("Миграция %d моделей заняла: %v", len(allModels), duration)

		// Проверяем, что все таблицы созданы
		for i, model := range allModels {
			assert.True(t, db.Migrator().HasTable(model), "Модель %d (%T) должна иметь таблицу", i, model)
		}

		// Миграция должна выполняться достаточно быстро
		assert.Less(t, duration, time.Second*10, "Миграция не должна занимать более 10 секунд")
	})

	t.Run("Повторная миграция не должна изменять схему", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)

		models := []interface{}{&User{}, &Role{}, &Object{}, &Contract{}}

		// Первая миграция
		start1 := time.Now()
		err = db.AutoMigrate(models...)
		duration1 := time.Since(start1)
		require.NoError(t, err)

		// Повторная миграция
		start2 := time.Now()
		err = db.AutoMigrate(models...)
		duration2 := time.Since(start2)
		require.NoError(t, err)

		t.Logf("Первая миграция: %v, повторная миграция: %v", duration1, duration2)

		// Повторная миграция должна быть быстрее
		assert.Less(t, duration2, duration1, "Повторная миграция должна быть быстрее первой")

		// Проверяем, что таблицы все еще существуют
		for _, model := range models {
			assert.True(t, db.Migrator().HasTable(model))
		}
	})
}
