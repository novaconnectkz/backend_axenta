package services

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"backend_axenta/models"
)

// WarehouseService предоставляет бизнес-логику для складских операций
type WarehouseService struct {
	DB                  *gorm.DB
	NotificationService *NotificationService
}

// NewWarehouseService создает новый экземпляр WarehouseService
func NewWarehouseService(db *gorm.DB, notificationService *NotificationService) *WarehouseService {
	return &WarehouseService{
		DB:                  db,
		NotificationService: notificationService,
	}
}

// CheckLowStockLevels проверяет низкие остатки и создает уведомления
func (ws *WarehouseService) CheckLowStockLevels() error {
	var categories []models.EquipmentCategory
	if err := ws.DB.Where("is_active = ?", true).Find(&categories).Error; err != nil {
		return fmt.Errorf("ошибка при получении категорий: %w", err)
	}

	for _, category := range categories {
		// Подсчитываем количество доступного оборудования в категории
		var inStockCount int64
		if err := ws.DB.Model(&models.Equipment{}).
			Where("category_id = ? AND status = 'in_stock'", category.ID).
			Count(&inStockCount); err != nil {
			log.Printf("Ошибка при подсчете оборудования для категории %s: %v", category.Name, err)
			continue
		}

		// Если остаток ниже минимального уровня, создаем уведомление
		if inStockCount < int64(category.MinStockLevel) {
			if err := ws.createLowStockAlert(category, int(inStockCount)); err != nil {
				log.Printf("Ошибка при создании уведомления о низком остатке для категории %s: %v", category.Name, err)
			}
		}
	}

	return nil
}

// createLowStockAlert создает уведомление о низком остатке
func (ws *WarehouseService) createLowStockAlert(category models.EquipmentCategory, currentStock int) error {
	// Проверяем, есть ли уже активное уведомление для этой категории
	var existingAlert models.StockAlert
	err := ws.DB.Where("equipment_category_id = ? AND type = 'low_stock' AND status = 'active'", category.ID).
		First(&existingAlert).Error

	if err == nil {
		// Уведомление уже существует, обновляем его
		existingAlert.Description = fmt.Sprintf("Низкий остаток в категории %s: %d шт. (минимум: %d шт.)",
			category.Name, currentStock, category.MinStockLevel)
		existingAlert.UpdatedAt = time.Now()
		return ws.DB.Save(&existingAlert).Error
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("ошибка при проверке существующих уведомлений: %w", err)
	}

	// Создаем новое уведомление
	alert := models.StockAlert{
		Type:  "low_stock",
		Title: fmt.Sprintf("Низкий остаток: %s", category.Name),
		Description: fmt.Sprintf("Низкий остаток в категории %s: %d шт. (минимум: %d шт.)",
			category.Name, currentStock, category.MinStockLevel),
		Severity:            ws.determineSeverity(currentStock, category.MinStockLevel),
		EquipmentCategoryID: &category.ID,
		Status:              "active",
	}

	if err := ws.DB.Create(&alert).Error; err != nil {
		return fmt.Errorf("ошибка при создании уведомления: %w", err)
	}

	// Отправляем уведомление
	if ws.NotificationService != nil {
		go ws.NotificationService.SendStockAlert(alert)
	}

	return nil
}

// determineSeverity определяет уровень важности уведомления
func (ws *WarehouseService) determineSeverity(currentStock, minStock int) string {
	if currentStock == 0 {
		return "critical"
	}
	if currentStock < minStock/2 {
		return "high"
	}
	if currentStock < minStock {
		return "medium"
	}
	return "low"
}

// CheckExpiredWarranties проверяет истекшие гарантии
func (ws *WarehouseService) CheckExpiredWarranties() error {
	var equipment []models.Equipment
	if err := ws.DB.Where("warranty_until < ? AND warranty_until IS NOT NULL", time.Now()).
		Find(&equipment).Error; err != nil {
		return fmt.Errorf("ошибка при поиске оборудования с истекшей гарантией: %w", err)
	}

	for _, eq := range equipment {
		if err := ws.createWarrantyExpiredAlert(eq); err != nil {
			log.Printf("Ошибка при создании уведомления об истекшей гарантии для оборудования %d: %v", eq.ID, err)
		}
	}

	return nil
}

// createWarrantyExpiredAlert создает уведомление об истекшей гарантии
func (ws *WarehouseService) createWarrantyExpiredAlert(equipment models.Equipment) error {
	// Проверяем, есть ли уже активное уведомление для этого оборудования
	var existingAlert models.StockAlert
	err := ws.DB.Where("equipment_id = ? AND type = 'expired_warranty' AND status = 'active'", equipment.ID).
		First(&existingAlert).Error

	if err == nil {
		// Уведомление уже существует
		return nil
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("ошибка при проверке существующих уведомлений: %w", err)
	}

	// Создаем новое уведомление
	alert := models.StockAlert{
		Type:  "expired_warranty",
		Title: fmt.Sprintf("Истекла гарантия: %s %s", equipment.Brand, equipment.Model),
		Description: fmt.Sprintf("У оборудования %s %s (S/N: %s) истекла гарантия %s",
			equipment.Brand, equipment.Model, equipment.SerialNumber,
			equipment.WarrantyUntil.Format("02.01.2006")),
		Severity:    "medium",
		EquipmentID: &equipment.ID,
		Status:      "active",
	}

	if err := ws.DB.Create(&alert).Error; err != nil {
		return fmt.Errorf("ошибка при создании уведомления: %w", err)
	}

	// Отправляем уведомление
	if ws.NotificationService != nil {
		go ws.NotificationService.SendWarrantyAlert(alert)
	}

	return nil
}

// CheckMaintenanceDue проверяет оборудование, требующее обслуживания
func (ws *WarehouseService) CheckMaintenanceDue() error {
	var equipment []models.Equipment
	// Проверяем оборудование, которое давно не обслуживалось (более 6 месяцев)
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	if err := ws.DB.Where("(last_maintenance_at < ? OR last_maintenance_at IS NULL) AND status != 'maintenance'",
		sixMonthsAgo).Find(&equipment).Error; err != nil {
		return fmt.Errorf("ошибка при поиске оборудования для обслуживания: %w", err)
	}

	for _, eq := range equipment {
		if err := ws.createMaintenanceDueAlert(eq); err != nil {
			log.Printf("Ошибка при создании уведомления о необходимости обслуживания для оборудования %d: %v", eq.ID, err)
		}
	}

	return nil
}

// createMaintenanceDueAlert создает уведомление о необходимости обслуживания
func (ws *WarehouseService) createMaintenanceDueAlert(equipment models.Equipment) error {
	// Проверяем, есть ли уже активное уведомление для этого оборудования
	var existingAlert models.StockAlert
	err := ws.DB.Where("equipment_id = ? AND type = 'maintenance_due' AND status = 'active'", equipment.ID).
		First(&existingAlert).Error

	if err == nil {
		// Уведомление уже существует
		return nil
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("ошибка при проверке существующих уведомлений: %w", err)
	}

	// Создаем новое уведомление
	alert := models.StockAlert{
		Type:  "maintenance_due",
		Title: fmt.Sprintf("Требуется обслуживание: %s %s", equipment.Brand, equipment.Model),
		Description: fmt.Sprintf("Оборудование %s %s (S/N: %s) требует планового обслуживания",
			equipment.Brand, equipment.Model, equipment.SerialNumber),
		Severity:    "medium",
		EquipmentID: &equipment.ID,
		Status:      "active",
	}

	if err := ws.DB.Create(&alert).Error; err != nil {
		return fmt.Errorf("ошибка при создании уведомления: %w", err)
	}

	// Отправляем уведомление
	if ws.NotificationService != nil {
		go ws.NotificationService.SendMaintenanceAlert(alert)
	}

	return nil
}

// ProcessEquipmentInstallation обрабатывает установку оборудования на объект
func (ws *WarehouseService) ProcessEquipmentInstallation(equipmentID uint, objectID uint, installerID uint) error {
	// Проверяем доступность оборудования
	var equipment models.Equipment
	if err := ws.DB.First(&equipment, equipmentID).Error; err != nil {
		return fmt.Errorf("оборудование не найдено: %w", err)
	}

	if !equipment.IsAvailable() {
		return fmt.Errorf("оборудование недоступно для установки")
	}

	// Проверяем существование объекта
	var object models.Object
	if err := ws.DB.First(&object, objectID).Error; err != nil {
		return fmt.Errorf("объект не найден: %w", err)
	}

	// Начинаем транзакцию
	tx := ws.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Обновляем статус оборудования
	equipment.Status = "installed"
	equipment.ObjectID = &objectID
	if err := tx.Save(&equipment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при обновлении оборудования: %w", err)
	}

	// Создаем операцию выдачи
	operation := models.WarehouseOperation{
		Type:         "issue",
		Description:  fmt.Sprintf("Выдача оборудования для установки на объект %s", object.Name),
		EquipmentID:  equipmentID,
		UserID:       installerID,
		Status:       "completed",
		FromLocation: equipment.WarehouseLocation,
		ToLocation:   "Установлено на объекте",
	}

	if err := tx.Create(&operation).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при создании операции: %w", err)
	}

	// Фиксируем транзакцию
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("ошибка при фиксации транзакции: %w", err)
	}

	return nil
}

// ProcessEquipmentReturn обрабатывает возврат оборудования на склад
func (ws *WarehouseService) ProcessEquipmentReturn(equipmentID uint, userID uint, warehouseLocation string) error {
	// Проверяем оборудование
	var equipment models.Equipment
	if err := ws.DB.First(&equipment, equipmentID).Error; err != nil {
		return fmt.Errorf("оборудование не найдено: %w", err)
	}

	if equipment.Status != "installed" {
		return fmt.Errorf("оборудование не установлено")
	}

	// Начинаем транзакцию
	tx := ws.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Обновляем статус оборудования
	equipment.Status = "in_stock"
	equipment.ObjectID = nil
	equipment.WarehouseLocation = warehouseLocation
	if err := tx.Save(&equipment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при обновлении оборудования: %w", err)
	}

	// Создаем операцию поступления
	operation := models.WarehouseOperation{
		Type:         "receive",
		Description:  "Возврат оборудования с объекта на склад",
		EquipmentID:  equipmentID,
		UserID:       userID,
		Status:       "completed",
		FromLocation: "Объект",
		ToLocation:   warehouseLocation,
	}

	if err := tx.Create(&operation).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при создании операции: %w", err)
	}

	// Фиксируем транзакцию
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("ошибка при фиксации транзакции: %w", err)
	}

	return nil
}

// RunPeriodicChecks запускает периодические проверки склада
func (ws *WarehouseService) RunPeriodicChecks() {
	log.Println("Запуск периодических проверок склада...")

	if err := ws.CheckLowStockLevels(); err != nil {
		log.Printf("Ошибка при проверке низких остатков: %v", err)
	}

	if err := ws.CheckExpiredWarranties(); err != nil {
		log.Printf("Ошибка при проверке истекших гарантий: %v", err)
	}

	if err := ws.CheckMaintenanceDue(); err != nil {
		log.Printf("Ошибка при проверке необходимости обслуживания: %v", err)
	}

	log.Println("Периодические проверки склада завершены")
}

// GenerateQRCode генерирует QR код для оборудования
func (ws *WarehouseService) GenerateQRCode(equipmentID uint) (string, error) {
	var equipment models.Equipment
	if err := ws.DB.First(&equipment, equipmentID).Error; err != nil {
		return "", fmt.Errorf("оборудование не найдено: %w", err)
	}

	// Если QR код уже существует, возвращаем его
	if equipment.QRCode != "" {
		return equipment.QRCode, nil
	}

	// Генерируем уникальный QR код на основе ID и серийного номера
	qrCode := fmt.Sprintf("EQ-%d-%s", equipment.ID, equipment.SerialNumber)

	// Проверяем уникальность
	var existingEquipment models.Equipment
	if err := ws.DB.Where("qr_code = ?", qrCode).First(&existingEquipment).Error; err == nil {
		// Если код уже существует, добавляем timestamp
		qrCode = fmt.Sprintf("EQ-%d-%s-%d", equipment.ID, equipment.SerialNumber, time.Now().Unix())
	}

	// Сохраняем QR код
	equipment.QRCode = qrCode
	if err := ws.DB.Save(&equipment).Error; err != nil {
		return "", fmt.Errorf("ошибка при сохранении QR кода: %w", err)
	}

	return qrCode, nil
}

// GetEquipmentHistory возвращает историю операций с оборудованием
func (ws *WarehouseService) GetEquipmentHistory(equipmentID uint) ([]models.WarehouseOperation, error) {
	var operations []models.WarehouseOperation
	if err := ws.DB.Preload("User").Preload("Installation").
		Where("equipment_id = ?", equipmentID).
		Order("created_at DESC").
		Find(&operations).Error; err != nil {
		return nil, fmt.Errorf("ошибка при получении истории оборудования: %w", err)
	}

	return operations, nil
}

// ReserveEquipment резервирует оборудование для установки
func (ws *WarehouseService) ReserveEquipment(equipmentID uint, userID uint, notes string) error {
	var equipment models.Equipment
	if err := ws.DB.First(&equipment, equipmentID).Error; err != nil {
		return fmt.Errorf("оборудование не найдено: %w", err)
	}

	if !equipment.IsAvailable() {
		return fmt.Errorf("оборудование недоступно для резервирования")
	}

	// Начинаем транзакцию
	tx := ws.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Обновляем статус оборудования
	equipment.Status = "reserved"
	if err := tx.Save(&equipment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при резервировании оборудования: %w", err)
	}

	// Создаем операцию резервирования
	operation := models.WarehouseOperation{
		Type:        "reserve",
		Description: "Резервирование оборудования",
		EquipmentID: equipmentID,
		UserID:      userID,
		Status:      "completed",
		Notes:       notes,
	}

	if err := tx.Create(&operation).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при создании операции: %w", err)
	}

	// Фиксируем транзакцию
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("ошибка при фиксации транзакции: %w", err)
	}

	return nil
}

// UnreserveEquipment снимает резервирование с оборудования
func (ws *WarehouseService) UnreserveEquipment(equipmentID uint, userID uint) error {
	var equipment models.Equipment
	if err := ws.DB.First(&equipment, equipmentID).Error; err != nil {
		return fmt.Errorf("оборудование не найдено: %w", err)
	}

	if equipment.Status != "reserved" {
		return fmt.Errorf("оборудование не зарезервировано")
	}

	// Начинаем транзакцию
	tx := ws.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Обновляем статус оборудования
	equipment.Status = "in_stock"
	if err := tx.Save(&equipment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при снятии резервирования: %w", err)
	}

	// Создаем операцию снятия резервирования
	operation := models.WarehouseOperation{
		Type:        "unreserve",
		Description: "Снятие резервирования с оборудования",
		EquipmentID: equipmentID,
		UserID:      userID,
		Status:      "completed",
	}

	if err := tx.Create(&operation).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при создании операции: %w", err)
	}

	// Фиксируем транзакцию
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("ошибка при фиксации транзакции: %w", err)
	}

	return nil
}
