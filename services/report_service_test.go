package services

import (
	"backend_axenta/models"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB создает тестовую базу данных в памяти
func setupReportTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Мигрируем все необходимые модели
	err = db.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Role{},
		&models.Object{},
		&models.Contract{},
		&models.Location{},
		&models.Installation{},
		&models.Installer{},
		&models.Equipment{},
		&models.EquipmentCategory{},
		&models.Invoice{},
		&models.Report{},
		&models.ReportTemplate{},
		&models.ReportSchedule{},
		&models.ReportExecution{},
	)
	require.NoError(t, err)

	return db
}

// createTestData создает тестовые данные
func createReportTestData(t *testing.T, db *gorm.DB) {
	// Создаем тарифный план
	tariffPlan := models.TariffPlan{
		BillingPlan: models.BillingPlan{
			Name:        "Test Tariff",
			Description: "Test tariff plan",
			Price:       decimal.NewFromFloat(1000.0),
			Currency:    "RUB",
		},
		PricePerObject: decimal.NewFromFloat(100.0),
	}
	require.NoError(t, db.Create(&tariffPlan).Error)

	// Создаем роль
	role := models.Role{
		Name:        "Admin",
		Description: "Administrator role",
		Permissions: []models.Permission{},
	}
	require.NoError(t, db.Create(&role).Error)

	// Создаем пользователя
	user := models.User{
		Username:  "testuser",
		Email:     "test@example.com",
		Password:  "hashedpassword",
		FirstName: "Test",
		LastName:  "User",
		IsActive:  true,
		RoleID:    role.ID,
		CompanyID: uuid.New(),
	}
	require.NoError(t, db.Create(&user).Error)

	// Создаем локацию
	location := models.Location{
		City:     "Test City",
		Region:   "Test Region",
		Country:  "Russia",
		IsActive: true,
	}
	require.NoError(t, db.Create(&location).Error)

	// Создаем договор
	contract := models.Contract{
		Number:      "TEST-001",
		ClientName:  "Test Client",
		StartDate:   time.Now().AddDate(0, -1, 0),
		EndDate:     time.Now().AddDate(1, 0, 0),
		Status:      "active",
		TotalAmount: decimal.NewFromFloat(10000.0),
		CompanyID:   uuid.New(),
	}
	require.NoError(t, db.Create(&contract).Error)

	// Создаем объекты
	objects := []models.Object{
		{
			Name:        "Object 1",
			Type:        "vehicle",
			IMEI:        "123456789012345",
			PhoneNumber: "+7123456789",
			Status:      "active",
			IsActive:    true,
			ContractID:  contract.ID,
			LocationID:  location.ID,
		},
		{
			Name:        "Object 2",
			Type:        "equipment",
			IMEI:        "123456789012346",
			PhoneNumber: "+7123456790",
			Status:      "inactive",
			IsActive:    false,
			ContractID:  contract.ID,
			LocationID:  location.ID,
		},
	}

	for _, obj := range objects {
		require.NoError(t, db.Create(&obj).Error)
	}

	// Создаем монтажника
	installer := models.Installer{
		FirstName:      "Test",
		LastName:       "Installer",
		Phone:          "+7987654321",
		Email:          "installer@test.com",
		Type:           "staff",
		Specialization: []string{"electronics"},
		IsActive:       true,
	}
	require.NoError(t, db.Create(&installer).Error)

	// Создаем монтажи
	locationID := location.ID
	installations := []models.Installation{
		{
			ObjectID:          objects[0].ID,
			InstallerID:       installer.ID,
			LocationID:        &locationID,
			Type:              "installation",
			Status:            "completed",
			ScheduledAt:       time.Now().AddDate(0, 0, -5),
			EstimatedDuration: 120,
			Cost:              decimal.NewFromFloat(5000.0),
			CompanyID:         1,
		},
		{
			ObjectID:          objects[1].ID,
			InstallerID:       installer.ID,
			LocationID:        &locationID,
			Type:              "maintenance",
			Status:            "scheduled",
			ScheduledAt:       time.Now().AddDate(0, 0, 5),
			EstimatedDuration: 60,
			Cost:              decimal.NewFromFloat(2000.0),
			CompanyID:         1,
		},
	}

	for _, installation := range installations {
		require.NoError(t, db.Create(&installation).Error)
	}

	// Создаем категорию оборудования
	category := models.EquipmentCategory{
		Name:          "Test Category",
		Description:   "Test equipment category",
		Code:          "TC001",
		MinStockLevel: 10,
		IsActive:      true,
	}
	require.NoError(t, db.Create(&category).Error)

	categoryID := category.ID
	// Создаем оборудование
	equipment := []models.Equipment{
		{
			Type:              "GPS-tracker",
			Model:             "GT-100",
			Brand:             "TestBrand",
			SerialNumber:      "SN001",
			IMEI:              "111111111111111",
			Status:            "in_stock",
			CategoryID:        &categoryID,
			WarehouseLocation: "A1-01",
			PurchasePrice:     decimal.NewFromFloat(1500.0),
		},
		{
			Type:              "GPS-tracker",
			Model:             "GT-200",
			Brand:             "TestBrand",
			SerialNumber:      "SN002",
			IMEI:              "111111111111112",
			Status:            "installed",
			CategoryID:        &categoryID,
			WarehouseLocation: "A1-02",
			PurchasePrice:     decimal.NewFromFloat(2000.0),
			ObjectID:          &objects[0].ID,
		},
	}

	for _, eq := range equipment {
		require.NoError(t, db.Create(&eq).Error)
	}

	// Создаем счета
	now := time.Now()
	invoices := []models.Invoice{
		{
			Number:             "INV-001",
			Title:              "Test Invoice 1",
			InvoiceDate:        now,
			DueDate:            now.AddDate(0, 0, 30),
			CompanyID:          uuid.New(),
			TariffPlanID:       tariffPlan.ID,
			BillingPeriodStart: now.AddDate(0, -1, 0),
			BillingPeriodEnd:   now,
			SubtotalAmount:     decimal.NewFromFloat(8000.0),
			TaxAmount:          decimal.NewFromFloat(1600.0),
			TotalAmount:        decimal.NewFromFloat(9600.0),
			Status:             "paid",
			PaidAt:             &[]time.Time{time.Now().AddDate(0, 0, -10)}[0],
		},
		{
			Number:             "INV-002",
			Title:              "Test Invoice 2",
			InvoiceDate:        now,
			DueDate:            now.AddDate(0, 0, 30),
			CompanyID:          uuid.New(),
			TariffPlanID:       tariffPlan.ID,
			BillingPeriodStart: now.AddDate(0, -1, 0),
			BillingPeriodEnd:   now,
			SubtotalAmount:     decimal.NewFromFloat(5000.0),
			TaxAmount:          decimal.NewFromFloat(1000.0),
			TotalAmount:        decimal.NewFromFloat(6000.0),
			Status:             "pending",
		},
	}

	for _, invoice := range invoices {
		require.NoError(t, db.Create(&invoice).Error)
	}
}

func TestReportService_GenerateReport(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	// Создаем временную директорию для отчетов
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	tests := []struct {
		name       string
		reportType models.ReportType
		format     models.ReportFormat
		expectErr  bool
	}{
		{
			name:       "Objects CSV Report",
			reportType: models.ReportTypeObjects,
			format:     models.ReportFormatCSV,
			expectErr:  false,
		},
		{
			name:       "Users Excel Report",
			reportType: models.ReportTypeUsers,
			format:     models.ReportFormatExcel,
			expectErr:  false,
		},
		{
			name:       "Billing JSON Report",
			reportType: models.ReportTypeBilling,
			format:     models.ReportFormatJSON,
			expectErr:  false,
		},
		{
			name:       "Installations PDF Report",
			reportType: models.ReportTypeInstallations,
			format:     models.ReportFormatPDF,
			expectErr:  false,
		},
		{
			name:       "Warehouse CSV Report",
			reportType: models.ReportTypeWarehouse,
			format:     models.ReportFormatCSV,
			expectErr:  false,
		},
		{
			name:       "Contracts Excel Report",
			reportType: models.ReportTypeContracts,
			format:     models.ReportFormatExcel,
			expectErr:  false,
		},
		{
			name:       "General JSON Report",
			reportType: models.ReportTypeGeneral,
			format:     models.ReportFormatJSON,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Создаем отчет
			report := models.Report{
				Name:        tt.name,
				Type:        tt.reportType,
				Format:      tt.format,
				Status:      models.ReportStatusPending,
				CreatedByID: 1,
				CompanyID:   1,
			}
			require.NoError(t, db.Create(&report).Error)

			// Параметры отчета
			params := ReportParams{
				Type:      tt.reportType,
				Format:    tt.format,
				CompanyID: 1,
			}

			// Генерируем отчет
			err := service.GenerateReport(params, &report)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Equal(t, models.ReportStatusFailed, report.Status)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, models.ReportStatusCompleted, report.Status)
				assert.NotEmpty(t, report.FilePath)
				assert.Greater(t, report.FileSize, int64(0))
				assert.Greater(t, report.RecordCount, 0)
				assert.NotNil(t, report.CompletedAt)
				assert.Greater(t, report.Duration, 0)

				// Проверяем, что файл существует
				_, err := os.Stat(report.FilePath)
				assert.NoError(t, err)
			}
		})
	}
}

func TestReportService_GetObjectsReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeObjects,
		CompanyID: 1,
	}

	data, err := service.getObjectsReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем заголовки
	expectedHeaders := []string{"ID", "Название", "Тип", "IMEI", "Телефон", "Адрес", "Статус", "Активен", "Договор", "Локация", "Дата создания"}
	assert.Equal(t, expectedHeaders, data.Headers)

	// Проверяем количество записей
	assert.Equal(t, 2, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 2, data.Summary["total_objects"])
	assert.Equal(t, 1, data.Summary["active_objects"])
	assert.Equal(t, 1, data.Summary["inactive_objects"])

	// Проверяем данные первого объекта
	row1 := data.Rows[0]
	assert.Equal(t, "Object 1", row1["Название"])
	assert.Equal(t, "vehicle", row1["Тип"])
	assert.Equal(t, "123456789012345", row1["IMEI"])
	assert.Equal(t, true, row1["Активен"])
}

func TestReportService_GetUsersReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeUsers,
		CompanyID: 1,
	}

	data, err := service.getUsersReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей
	assert.Equal(t, 1, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 1, data.Summary["total_users"])
	assert.Equal(t, 1, data.Summary["active_users"])
	assert.Equal(t, 0, data.Summary["inactive_users"])

	// Проверяем данные пользователя
	row := data.Rows[0]
	assert.Equal(t, "testuser", row["Имя пользователя"])
	assert.Equal(t, "test@example.com", row["Email"])
	assert.Equal(t, "Test", row["Имя"])
	assert.Equal(t, "User", row["Фамилия"])
}

func TestReportService_GetBillingReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeBilling,
		CompanyID: 1,
	}

	data, err := service.getBillingReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей
	assert.Equal(t, 2, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 2, data.Summary["total_invoices"])
	assert.Equal(t, 13000.0, data.Summary["total_amount"])
	assert.Equal(t, 2600.0, data.Summary["total_tax"])
	assert.Equal(t, 15600.0, data.Summary["total_final"])
}

func TestReportService_GetInstallationsReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeInstallations,
		CompanyID: 1,
	}

	data, err := service.getInstallationsReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей
	assert.Equal(t, 2, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 2, data.Summary["total_installations"])
	assert.Equal(t, 7000.0, data.Summary["total_cost"])
	assert.Equal(t, 3500.0, data.Summary["avg_cost"])
}

func TestReportService_GetWarehouseReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeWarehouse,
		CompanyID: 1,
	}

	data, err := service.getWarehouseReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей
	assert.Equal(t, 2, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 2, data.Summary["total_equipment"])
	assert.Equal(t, 3500.0, data.Summary["total_cost"])
	assert.Equal(t, 1750.0, data.Summary["avg_cost"])
}

func TestReportService_GetContractsReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeContracts,
		CompanyID: 1,
	}

	data, err := service.getContractsReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей
	assert.Equal(t, 1, len(data.Rows))

	// Проверяем сводку
	assert.Equal(t, 1, data.Summary["total_contracts"])
	assert.Equal(t, 10000.0, data.Summary["total_cost"])
	assert.Equal(t, 2, data.Summary["total_objects"])
}

func TestReportService_GetGeneralReportData(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeGeneral,
		CompanyID: 1,
	}

	data, err := service.getGeneralReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Проверяем количество записей (показателей)
	assert.Equal(t, 9, len(data.Rows))

	// Проверяем некоторые показатели
	assert.Equal(t, int64(1), data.Summary["total_users"])
	assert.Equal(t, int64(2), data.Summary["total_objects"])
	assert.Equal(t, int64(1), data.Summary["total_contracts"])
	assert.Equal(t, int64(2), data.Summary["total_installations"])
	assert.Equal(t, int64(2), data.Summary["total_equipment"])
}

func TestReportService_GenerateCSVReport(t *testing.T) {
	db := setupReportTestDB(t)
	service := NewReportService(db)

	// Создаем временную директорию
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_report.csv")

	data := &ReportData{
		Headers: []string{"ID", "Name", "Status"},
		Rows: []map[string]interface{}{
			{"ID": 1, "Name": "Object 1", "Status": "active"},
			{"ID": 2, "Name": "Object 2", "Status": "inactive"},
		},
	}

	resultPath, err := service.generateCSVReport(data, filePath)
	require.NoError(t, err)
	assert.Equal(t, filePath, resultPath)

	// Проверяем, что файл создан
	_, err = os.Stat(filePath)
	assert.NoError(t, err)

	// Проверяем содержимое файла
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	expectedContent := "ID,Name,Status\n1,Object 1,active\n2,Object 2,inactive\n"
	assert.Equal(t, expectedContent, string(content))
}

func TestReportService_GenerateJSONReport(t *testing.T) {
	db := setupReportTestDB(t)
	service := NewReportService(db)

	// Создаем временную директорию
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_report.json")

	data := &ReportData{
		Headers: []string{"ID", "Name"},
		Rows: []map[string]interface{}{
			{"ID": 1, "Name": "Object 1"},
		},
		Summary: map[string]interface{}{
			"total": 1,
		},
	}

	resultPath, err := service.generateJSONReport(data, filePath)
	require.NoError(t, err)
	assert.Equal(t, filePath, resultPath)

	// Проверяем, что файл создан
	_, err = os.Stat(filePath)
	assert.NoError(t, err)
}

func TestReportService_GenerateExcelReport(t *testing.T) {
	db := setupReportTestDB(t)
	service := NewReportService(db)

	// Создаем временную директорию
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_report.xlsx")

	data := &ReportData{
		Headers: []string{"ID", "Name", "Status"},
		Rows: []map[string]interface{}{
			{"ID": 1, "Name": "Object 1", "Status": "active"},
			{"ID": 2, "Name": "Object 2", "Status": "inactive"},
		},
	}

	resultPath, err := service.generateExcelReport(data, filePath)
	require.NoError(t, err)
	assert.Equal(t, filePath, resultPath)

	// Проверяем, что файл создан
	_, err = os.Stat(filePath)
	assert.NoError(t, err)
}

func TestReportService_GeneratePDFReport(t *testing.T) {
	db := setupReportTestDB(t)
	service := NewReportService(db)

	// Создаем временную директорию
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_report.pdf")

	data := &ReportData{
		Headers: []string{"ID", "Name"},
		Rows: []map[string]interface{}{
			{"ID": 1, "Name": "Object 1"},
			{"ID": 2, "Name": "Object 2"},
		},
	}

	resultPath, err := service.generatePDFReport(data, filePath)
	require.NoError(t, err)
	assert.Equal(t, filePath, resultPath)

	// Проверяем, что файл создан
	_, err = os.Stat(filePath)
	assert.NoError(t, err)
}

func TestReportService_DateFilters(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	// Тестируем фильтрацию по датам
	dateFrom := time.Now().AddDate(0, 0, -1)
	dateTo := time.Now().AddDate(0, 0, 1)

	params := ReportParams{
		Type:      models.ReportTypeObjects,
		CompanyID: 1,
		DateFrom:  &dateFrom,
		DateTo:    &dateTo,
	}

	data, err := service.getObjectsReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Все объекты должны попасть в диапазон
	assert.Equal(t, 2, len(data.Rows))
}

func TestReportService_StatusFilter(t *testing.T) {
	db := setupReportTestDB(t)
	createReportTestData(t, db)

	service := NewReportService(db)

	// Тестируем фильтрацию по статусу
	params := ReportParams{
		Type:      models.ReportTypeObjects,
		CompanyID: 1,
		Status:    "active",
	}

	data, err := service.getObjectsReportData(params)
	require.NoError(t, err)
	require.NotNil(t, data)

	// Должен быть только один активный объект
	assert.Equal(t, 1, len(data.Rows))
	assert.Equal(t, "active", data.Rows[0]["Статус"])
}

func TestReportService_UnsupportedReportType(t *testing.T) {
	db := setupReportTestDB(t)
	service := NewReportService(db)

	params := ReportParams{
		Type:      "unsupported",
		CompanyID: 1,
	}

	_, err := service.getReportData(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported report type")
}

// Бенчмарк тесты

func BenchmarkReportService_GenerateObjectsReport(b *testing.B) {
	db := setupReportTestDB(&testing.T{})
	createReportTestData(&testing.T{}, db)

	service := NewReportService(db)

	params := ReportParams{
		Type:      models.ReportTypeObjects,
		CompanyID: 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.getObjectsReportData(params)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReportService_GenerateCSVReport(b *testing.B) {
	db := setupReportTestDB(&testing.T{})
	service := NewReportService(db)

	tempDir := b.TempDir()

	data := &ReportData{
		Headers: []string{"ID", "Name", "Status"},
		Rows:    make([]map[string]interface{}, 1000),
	}

	// Создаем тестовые данные
	for i := 0; i < 1000; i++ {
		data.Rows[i] = map[string]interface{}{
			"ID":     i + 1,
			"Name":   "Object " + string(rune(i+1)),
			"Status": "active",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filePath := filepath.Join(tempDir, "benchmark_report.csv")
		_, err := service.generateCSVReport(data, filePath)
		if err != nil {
			b.Fatal(err)
		}
		os.Remove(filePath)
	}
}
