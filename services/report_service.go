package services

import (
	"backend_axenta/models"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ReportService предоставляет функциональность для работы с отчетами
type ReportService struct {
	db *gorm.DB
}

// NewReportService создает новый экземпляр ReportService
func NewReportService(db *gorm.DB) *ReportService {
	return &ReportService{db: db}
}

// ReportData представляет данные для отчета
type ReportData struct {
	Headers []string                 `json:"headers"`
	Rows    []map[string]interface{} `json:"rows"`
	Summary map[string]interface{}   `json:"summary,omitempty"`
}

// ReportParams представляет параметры для генерации отчета
type ReportParams struct {
	Type       models.ReportType      `json:"type"`
	DateFrom   *time.Time             `json:"date_from,omitempty"`
	DateTo     *time.Time             `json:"date_to,omitempty"`
	UserID     *uint                  `json:"user_id,omitempty"`
	LocationID *uint                  `json:"location_id,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Format     models.ReportFormat    `json:"format"`
	CompanyID  uint                   `json:"company_id"`
	Filters    map[string]interface{} `json:"filters,omitempty"`
}

// GenerateReport генерирует отчет по заданным параметрам
func (rs *ReportService) GenerateReport(params ReportParams, report *models.Report) error {
	// Обновляем статус на "обрабатывается"
	now := time.Now()
	report.Status = models.ReportStatusProcessing
	report.StartedAt = &now
	if err := rs.db.Save(report).Error; err != nil {
		return fmt.Errorf("failed to update report status: %w", err)
	}

	// Получаем данные для отчета
	data, err := rs.getReportData(params)
	if err != nil {
		rs.updateReportError(report, fmt.Sprintf("failed to get report data: %v", err))
		return err
	}

	// Генерируем файл отчета
	filePath, err := rs.generateReportFile(data, params, report)
	if err != nil {
		rs.updateReportError(report, fmt.Sprintf("failed to generate report file: %v", err))
		return err
	}

	// Получаем информацию о файле
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		rs.updateReportError(report, fmt.Sprintf("failed to get file info: %v", err))
		return err
	}

	// Обновляем отчет с результатами
	completedAt := time.Now()
	duration := int(completedAt.Sub(*report.StartedAt).Seconds())

	report.Status = models.ReportStatusCompleted
	report.CompletedAt = &completedAt
	report.Duration = duration
	report.FilePath = filePath
	report.FileSize = fileInfo.Size()
	report.RecordCount = len(data.Rows)
	report.ErrorMsg = ""

	return rs.db.Save(report).Error
}

// getReportData получает данные для отчета в зависимости от типа
func (rs *ReportService) getReportData(params ReportParams) (*ReportData, error) {
	switch params.Type {
	case models.ReportTypeObjects:
		return rs.getObjectsReportData(params)
	case models.ReportTypeUsers:
		return rs.getUsersReportData(params)
	case models.ReportTypeBilling:
		return rs.getBillingReportData(params)
	case models.ReportTypeInstallations:
		return rs.getInstallationsReportData(params)
	case models.ReportTypeWarehouse:
		return rs.getWarehouseReportData(params)
	case models.ReportTypeContracts:
		return rs.getContractsReportData(params)
	case models.ReportTypeGeneral:
		return rs.getGeneralReportData(params)
	default:
		return nil, fmt.Errorf("unsupported report type: %s", params.Type)
	}
}

// getObjectsReportData получает данные отчета по объектам
func (rs *ReportService) getObjectsReportData(params ReportParams) (*ReportData, error) {
	var objects []models.Object
	query := rs.db.Where("company_id = ?", params.CompanyID)

	// Применяем фильтры по датам
	if params.DateFrom != nil {
		query = query.Where("created_at >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("created_at <= ?", params.DateTo)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}
	if params.LocationID != nil {
		query = query.Where("location_id = ?", *params.LocationID)
	}

	// Загружаем связанные данные
	if err := query.Preload("Contract").Preload("Location").Find(&objects).Error; err != nil {
		return nil, err
	}

	// Формируем данные отчета
	headers := []string{"ID", "Название", "Тип", "IMEI", "Телефон", "Адрес", "Статус", "Активен", "Договор", "Локация", "Дата создания"}
	rows := make([]map[string]interface{}, len(objects))

	for i, obj := range objects {
		contractName := ""
		if obj.Contract != nil {
			contractName = obj.Contract.Number
		}
		locationName := ""
		if obj.Location != nil {
			locationName = obj.Location.City
		}

		rows[i] = map[string]interface{}{
			"ID":            obj.ID,
			"Название":      obj.Name,
			"Тип":           obj.Type,
			"IMEI":          obj.IMEI,
			"Телефон":       obj.PhoneNumber,
			"Адрес":         obj.Address,
			"Статус":        obj.Status,
			"Активен":       obj.IsActive,
			"Договор":       contractName,
			"Локация":       locationName,
			"Дата создания": obj.CreatedAt.Format("02.01.2006 15:04"),
		}
	}

	// Формируем сводку
	summary := map[string]interface{}{
		"total_objects":    len(objects),
		"active_objects":   rs.countObjectsByField(objects, func(o models.Object) bool { return o.IsActive }),
		"inactive_objects": rs.countObjectsByField(objects, func(o models.Object) bool { return !o.IsActive }),
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getUsersReportData получает данные отчета по пользователям
func (rs *ReportService) getUsersReportData(params ReportParams) (*ReportData, error) {
	var users []models.User
	query := rs.db.Where("company_id = ?", params.CompanyID)

	if params.DateFrom != nil {
		query = query.Where("created_at >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("created_at <= ?", params.DateTo)
	}

	if err := query.Preload("Role").Find(&users).Error; err != nil {
		return nil, err
	}

	headers := []string{"ID", "Имя пользователя", "Email", "Имя", "Фамилия", "Телефон", "Роль", "Активен", "Последний вход", "Дата создания"}
	rows := make([]map[string]interface{}, len(users))

	for i, user := range users {
		roleName := ""
		if user.Role != nil {
			roleName = user.Role.Name
		}
		lastLogin := ""
		if user.LastLogin != nil {
			lastLogin = user.LastLogin.Format("02.01.2006 15:04")
		}

		rows[i] = map[string]interface{}{
			"ID":               user.ID,
			"Имя пользователя": user.Username,
			"Email":            user.Email,
			"Имя":              user.FirstName,
			"Фамилия":          user.LastName,
			"Телефон":          user.Phone,
			"Роль":             roleName,
			"Активен":          user.IsActive,
			"Последний вход":   lastLogin,
			"Дата создания":    user.CreatedAt.Format("02.01.2006 15:04"),
		}
	}

	summary := map[string]interface{}{
		"total_users":    len(users),
		"active_users":   rs.countByField(users, func(u models.User) bool { return u.IsActive }),
		"inactive_users": rs.countByField(users, func(u models.User) bool { return !u.IsActive }),
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getBillingReportData получает данные отчета по биллингу
func (rs *ReportService) getBillingReportData(params ReportParams) (*ReportData, error) {
	var invoices []models.Invoice
	query := rs.db.Where("company_id = ?", params.CompanyID)

	if params.DateFrom != nil {
		query = query.Where("created_at >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("created_at <= ?", params.DateTo)
	}

	if err := query.Preload("Items").Find(&invoices).Error; err != nil {
		return nil, err
	}

	headers := []string{"ID", "Номер", "Дата", "Сумма", "НДС", "Итого", "Статус", "Дата оплаты", "Позиций"}
	rows := make([]map[string]interface{}, len(invoices))

	var totalAmount, totalTax, totalFinal float64

	for i, invoice := range invoices {
		paidDate := ""
		if invoice.PaidAt != nil {
			paidDate = invoice.PaidAt.Format("02.01.2006")
		}

		subtotal, _ := invoice.SubtotalAmount.Float64()
		tax, _ := invoice.TaxAmount.Float64()
		total, _ := invoice.TotalAmount.Float64()

		totalAmount += subtotal
		totalTax += tax
		totalFinal += total

		rows[i] = map[string]interface{}{
			"ID":          invoice.ID,
			"Номер":       invoice.Number,
			"Дата":        invoice.CreatedAt.Format("02.01.2006"),
			"Сумма":       subtotal,
			"НДС":         tax,
			"Итого":       total,
			"Статус":      invoice.Status,
			"Дата оплаты": paidDate,
			"Позиций":     len(invoice.Items),
		}
	}

	summary := map[string]interface{}{
		"total_invoices": len(invoices),
		"total_amount":   totalAmount,
		"total_tax":      totalTax,
		"total_final":    totalFinal,
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getInstallationsReportData получает данные отчета по монтажам
func (rs *ReportService) getInstallationsReportData(params ReportParams) (*ReportData, error) {
	var installations []models.Installation
	query := rs.db.Where("company_id = ?", params.CompanyID)

	if params.DateFrom != nil {
		query = query.Where("scheduled_date >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("scheduled_date <= ?", params.DateTo)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}

	if err := query.Preload("Object").Preload("Installer").Preload("Location").Find(&installations).Error; err != nil {
		return nil, err
	}

	headers := []string{"ID", "Объект", "Монтажник", "Локация", "Дата", "Статус", "Тип", "Продолжительность", "Стоимость", "Дата создания"}
	rows := make([]map[string]interface{}, len(installations))

	var totalCost float64

	for i, installation := range installations {
		objectName := ""
		if installation.Object != nil {
			objectName = installation.Object.Name
		}
		installerName := ""
		if installation.Installer != nil {
			installerName = installation.Installer.FirstName + " " + installation.Installer.LastName
		}
		locationName := ""
		if installation.Location != nil {
			locationName = installation.Location.City
		}

		cost, _ := installation.Cost.Float64()
		totalCost += cost

		rows[i] = map[string]interface{}{
			"ID":                installation.ID,
			"Объект":            objectName,
			"Монтажник":         installerName,
			"Локация":           locationName,
			"Дата":              installation.ScheduledAt.Format("02.01.2006 15:04"),
			"Статус":            installation.Status,
			"Тип":               installation.Type,
			"Продолжительность": installation.EstimatedDuration,
			"Стоимость":         cost,
			"Дата создания":     installation.CreatedAt.Format("02.01.2006 15:04"),
		}
	}

	summary := map[string]interface{}{
		"total_installations": len(installations),
		"total_cost":          totalCost,
		"avg_cost":            totalCost / float64(len(installations)),
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getWarehouseReportData получает данные отчета по складу
func (rs *ReportService) getWarehouseReportData(params ReportParams) (*ReportData, error) {
	var equipment []models.Equipment
	query := rs.db.Where("company_id = ?", params.CompanyID)

	if params.DateFrom != nil {
		query = query.Where("created_at >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("created_at <= ?", params.DateTo)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}

	if err := query.Preload("Category").Find(&equipment).Error; err != nil {
		return nil, err
	}

	headers := []string{"ID", "Название", "Модель", "Серийный номер", "IMEI", "Категория", "Статус", "Локация склада", "Стоимость", "Дата создания"}
	rows := make([]map[string]interface{}, len(equipment))

	var totalCost float64

	for i, eq := range equipment {
		categoryName := ""
		if eq.Category != nil {
			categoryName = eq.Category.Name
		}

		cost, _ := eq.PurchasePrice.Float64()
		totalCost += cost

		rows[i] = map[string]interface{}{
			"ID":             eq.ID,
			"Название":       eq.Type,
			"Модель":         eq.Model,
			"Серийный номер": eq.SerialNumber,
			"IMEI":           eq.IMEI,
			"Категория":      categoryName,
			"Статус":         eq.Status,
			"Локация склада": eq.WarehouseLocation,
			"Стоимость":      cost,
			"Дата создания":  eq.CreatedAt.Format("02.01.2006 15:04"),
		}
	}

	summary := map[string]interface{}{
		"total_equipment": len(equipment),
		"total_cost":      totalCost,
		"avg_cost":        totalCost / float64(len(equipment)),
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getContractsReportData получает данные отчета по договорам
func (rs *ReportService) getContractsReportData(params ReportParams) (*ReportData, error) {
	var contracts []models.Contract
	query := rs.db.Where("company_id = ?", params.CompanyID)

	if params.DateFrom != nil {
		query = query.Where("created_at >= ?", params.DateFrom)
	}
	if params.DateTo != nil {
		query = query.Where("created_at <= ?", params.DateTo)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}

	if err := query.Find(&contracts).Error; err != nil {
		return nil, err
	}

	headers := []string{"ID", "Номер", "Клиент", "Дата начала", "Дата окончания", "Статус", "Стоимость", "Объектов", "Дата создания"}
	rows := make([]map[string]interface{}, len(contracts))

	var totalCost float64
	var totalObjects int

	for i, contract := range contracts {
		// Подсчитываем количество объектов по договору
		var objectCount int64
		rs.db.Model(&models.Object{}).Where("contract_id = ?", contract.ID).Count(&objectCount)
		totalObjects += int(objectCount)

		contractCost, _ := contract.TotalAmount.Float64()
		totalCost += contractCost

		rows[i] = map[string]interface{}{
			"ID":             contract.ID,
			"Номер":          contract.Number,
			"Клиент":         contract.ClientName,
			"Дата начала":    contract.StartDate.Format("02.01.2006"),
			"Дата окончания": contract.EndDate.Format("02.01.2006"),
			"Статус":         contract.Status,
			"Стоимость":      contractCost,
			"Объектов":       objectCount,
			"Дата создания":  contract.CreatedAt.Format("02.01.2006 15:04"),
		}
	}

	summary := map[string]interface{}{
		"total_contracts": len(contracts),
		"total_cost":      totalCost,
		"total_objects":   totalObjects,
		"avg_cost":        totalCost / float64(len(contracts)),
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: summary,
	}, nil
}

// getGeneralReportData получает общие данные системы
func (rs *ReportService) getGeneralReportData(params ReportParams) (*ReportData, error) {
	var stats struct {
		TotalUsers         int64   `json:"total_users"`
		ActiveUsers        int64   `json:"active_users"`
		TotalObjects       int64   `json:"total_objects"`
		ActiveObjects      int64   `json:"active_objects"`
		TotalContracts     int64   `json:"total_contracts"`
		ActiveContracts    int64   `json:"active_contracts"`
		TotalInstallations int64   `json:"total_installations"`
		TotalEquipment     int64   `json:"total_equipment"`
		TotalRevenue       float64 `json:"total_revenue"`
	}

	// Получаем статистику по каждой таблице
	rs.db.Model(&models.User{}).Where("company_id = ?", params.CompanyID).Count(&stats.TotalUsers)
	rs.db.Model(&models.User{}).Where("company_id = ? AND is_active = ?", params.CompanyID, true).Count(&stats.ActiveUsers)

	rs.db.Model(&models.Object{}).Where("company_id = ?", params.CompanyID).Count(&stats.TotalObjects)
	rs.db.Model(&models.Object{}).Where("company_id = ? AND is_active = ?", params.CompanyID, true).Count(&stats.ActiveObjects)

	rs.db.Model(&models.Contract{}).Where("company_id = ?", params.CompanyID).Count(&stats.TotalContracts)
	rs.db.Model(&models.Contract{}).Where("company_id = ? AND status = ?", params.CompanyID, "active").Count(&stats.ActiveContracts)

	rs.db.Model(&models.Installation{}).Where("company_id = ?", params.CompanyID).Count(&stats.TotalInstallations)
	rs.db.Model(&models.Equipment{}).Where("company_id = ?", params.CompanyID).Count(&stats.TotalEquipment)

	// Считаем общую выручку
	rs.db.Model(&models.Invoice{}).Where("company_id = ? AND status = ?", params.CompanyID, "paid").Select("COALESCE(SUM(total_amount), 0)").Scan(&stats.TotalRevenue)

	headers := []string{"Показатель", "Значение"}
	rows := []map[string]interface{}{
		{"Показатель": "Всего пользователей", "Значение": stats.TotalUsers},
		{"Показатель": "Активных пользователей", "Значение": stats.ActiveUsers},
		{"Показатель": "Всего объектов", "Значение": stats.TotalObjects},
		{"Показатель": "Активных объектов", "Значение": stats.ActiveObjects},
		{"Показатель": "Всего договоров", "Значение": stats.TotalContracts},
		{"Показатель": "Активных договоров", "Значение": stats.ActiveContracts},
		{"Показатель": "Всего монтажей", "Значение": stats.TotalInstallations},
		{"Показатель": "Единиц оборудования", "Значение": stats.TotalEquipment},
		{"Показатель": "Общая выручка", "Значение": fmt.Sprintf("%.2f руб.", stats.TotalRevenue)},
	}

	return &ReportData{
		Headers: headers,
		Rows:    rows,
		Summary: map[string]interface{}{
			"total_users":         stats.TotalUsers,
			"active_users":        stats.ActiveUsers,
			"total_objects":       stats.TotalObjects,
			"active_objects":      stats.ActiveObjects,
			"total_contracts":     stats.TotalContracts,
			"active_contracts":    stats.ActiveContracts,
			"total_installations": stats.TotalInstallations,
			"total_equipment":     stats.TotalEquipment,
			"total_revenue":       stats.TotalRevenue,
		},
	}, nil
}

// generateReportFile генерирует файл отчета в нужном формате
func (rs *ReportService) generateReportFile(data *ReportData, params ReportParams, report *models.Report) (string, error) {
	// Создаем директорию для отчетов если её нет
	reportsDir := "reports"
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return "", err
	}

	// Формируем имя файла
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("report_%d_%s_%s", report.ID, params.Type, timestamp)

	switch params.Format {
	case models.ReportFormatCSV:
		return rs.generateCSVReport(data, filepath.Join(reportsDir, fileName+".csv"))
	case models.ReportFormatExcel:
		return rs.generateExcelReport(data, filepath.Join(reportsDir, fileName+".xlsx"))
	case models.ReportFormatPDF:
		return rs.generatePDFReport(data, filepath.Join(reportsDir, fileName+".pdf"))
	case models.ReportFormatJSON:
		return rs.generateJSONReport(data, filepath.Join(reportsDir, fileName+".json"))
	default:
		return "", fmt.Errorf("unsupported format: %s", params.Format)
	}
}

// generateCSVReport генерирует CSV файл отчета
func (rs *ReportService) generateCSVReport(data *ReportData, filePath string) (string, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Записываем заголовки
	if err := writer.Write(data.Headers); err != nil {
		return "", err
	}

	// Записываем данные
	for _, row := range data.Rows {
		record := make([]string, len(data.Headers))
		for i, header := range data.Headers {
			if value, ok := row[header]; ok {
				record[i] = fmt.Sprintf("%v", value)
			}
		}
		if err := writer.Write(record); err != nil {
			return "", err
		}
	}

	return filePath, nil
}

// generateExcelReport генерирует Excel файл отчета
func (rs *ReportService) generateExcelReport(data *ReportData, filePath string) (string, error) {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Failed to close Excel file: %v", err)
		}
	}()

	sheetName := "Отчет"
	f.SetSheetName("Sheet1", sheetName)

	// Записываем заголовки
	for i, header := range data.Headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// Записываем данные
	for rowIdx, row := range data.Rows {
		for colIdx, header := range data.Headers {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			if value, ok := row[header]; ok {
				f.SetCellValue(sheetName, cell, value)
			}
		}
	}

	// Добавляем автофильтр
	endCell := fmt.Sprintf("%s%d", string(rune('A'+len(data.Headers)-1)), len(data.Rows)+1)
	f.AutoFilter(sheetName, "A1:"+endCell, []excelize.AutoFilterOptions{})

	// Сохраняем файл
	if err := f.SaveAs(filePath); err != nil {
		return "", err
	}

	return filePath, nil
}

// generatePDFReport генерирует PDF файл отчета
func (rs *ReportService) generatePDFReport(data *ReportData, filePath string) (string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	// Заголовок отчета
	pdf.Cell(40, 10, "Отчет")
	pdf.Ln(20)

	// Таблица с данными (упрощенная версия)
	pdf.SetFont("Arial", "", 8)

	// Заголовки
	for _, header := range data.Headers {
		pdf.Cell(20, 10, header)
	}
	pdf.Ln(10)

	// Данные (ограничиваем количество строк для PDF)
	maxRows := 50
	for i, row := range data.Rows {
		if i >= maxRows {
			pdf.Cell(20, 10, "... и еще записей")
			break
		}

		for _, header := range data.Headers {
			value := ""
			if val, ok := row[header]; ok {
				value = fmt.Sprintf("%.10s", fmt.Sprintf("%v", val))
			}
			pdf.Cell(20, 10, value)
		}
		pdf.Ln(5)
	}

	return filePath, pdf.OutputFileAndClose(filePath)
}

// generateJSONReport генерирует JSON файл отчета
func (rs *ReportService) generateJSONReport(data *ReportData, filePath string) (string, error) {
	file, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	reportData := map[string]interface{}{
		"headers":      data.Headers,
		"data":         data.Rows,
		"summary":      data.Summary,
		"generated_at": time.Now(),
	}

	return filePath, encoder.Encode(reportData)
}

// updateReportError обновляет отчет с информацией об ошибке
func (rs *ReportService) updateReportError(report *models.Report, errorMsg string) {
	now := time.Now()
	report.Status = models.ReportStatusFailed
	report.ErrorMsg = errorMsg
	report.CompletedAt = &now
	if report.StartedAt != nil {
		report.Duration = int(now.Sub(*report.StartedAt).Seconds())
	}
	rs.db.Save(report)
}

// countByField вспомогательная функция для подсчета пользователей по условию
func (rs *ReportService) countByField(items []models.User, predicate func(models.User) bool) int {
	count := 0
	for _, item := range items {
		if predicate(item) {
			count++
		}
	}
	return count
}

// countObjectsByField вспомогательная функция для подсчета объектов по условию
func (rs *ReportService) countObjectsByField(items []models.Object, predicate func(models.Object) bool) int {
	count := 0
	for _, item := range items {
		if predicate(item) {
			count++
		}
	}
	return count
}
