package services

import (
	"backend_axenta/models"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// WialonConnectionService сервис для управления подключениями Wialon
type WialonConnectionService struct {
	db *gorm.DB
	mu sync.RWMutex
}

// NewWialonConnectionService создает новый сервис
func NewWialonConnectionService(db *gorm.DB) *WialonConnectionService {
	return &WialonConnectionService{
		db: db,
	}
}

// Create создает новое подключение Wialon
func (s *WialonConnectionService) Create(connection *models.WialonConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Устанавливаем хост на основе типа подключения
	if connection.ConnectionType == models.WialonConnectionTypeHosting && connection.Host == "" {
		connection.Host = models.GetWialonHostingURL(connection.DataCenter)
	}

	// Валидация
	if connection.Name == "" {
		return fmt.Errorf("название подключения обязательно")
	}
	if connection.Token == "" {
		return fmt.Errorf("API токен обязателен")
	}
	if connection.Host == "" {
		return fmt.Errorf("URL хоста обязателен")
	}

	return s.db.Create(connection).Error
}

// GetByID получает подключение по ID
func (s *WialonConnectionService) GetByID(id uint) (*models.WialonConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var connection models.WialonConnection
	if err := s.db.First(&connection, id).Error; err != nil {
		return nil, err
	}

	connection.MaskToken()
	return &connection, nil
}

// GetByIDWithToken получает подключение по ID с реальным токеном (для внутреннего использования)
func (s *WialonConnectionService) GetByIDWithToken(id uint) (*models.WialonConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var connection models.WialonConnection
	if err := s.db.First(&connection, id).Error; err != nil {
		return nil, err
	}

	return &connection, nil
}

// GetAllByCompany получает все подключения компании
func (s *WialonConnectionService) GetAllByCompany(companyID uint) ([]models.WialonConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var connections []models.WialonConnection
	if err := s.db.Where("company_id = ?", companyID).Order("created_at DESC").Find(&connections).Error; err != nil {
		return nil, err
	}

	// Маскируем токены
	for i := range connections {
		connections[i].MaskToken()
	}

	return connections, nil
}

// GetActiveByCompany получает все активные подключения компании
func (s *WialonConnectionService) GetActiveByCompany(companyID uint) ([]models.WialonConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var connections []models.WialonConnection
	if err := s.db.Where("company_id = ? AND is_active = ?", companyID, true).Order("created_at DESC").Find(&connections).Error; err != nil {
		return nil, err
	}

	return connections, nil
}

// Update обновляет подключение
func (s *WialonConnectionService) Update(connection *models.WialonConnection) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Устанавливаем хост на основе типа подключения
	if connection.ConnectionType == models.WialonConnectionTypeHosting {
		connection.Host = models.GetWialonHostingURL(connection.DataCenter)
	}

	return s.db.Save(connection).Error
}

// UpdatePartial обновляет только определенные поля
func (s *WialonConnectionService) UpdatePartial(id uint, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Model(&models.WialonConnection{}).Where("id = ?", id).Updates(updates).Error
}

// Delete удаляет подключение (soft delete)
func (s *WialonConnectionService) Delete(id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Delete(&models.WialonConnection{}, id).Error
}

// SetActive устанавливает статус активности
func (s *WialonConnectionService) SetActive(id uint, isActive bool) error {
	return s.UpdatePartial(id, map[string]interface{}{
		"is_active":  isActive,
		"updated_at": time.Now(),
	})
}

// UpdateSyncStatus обновляет статус последней синхронизации
func (s *WialonConnectionService) UpdateSyncStatus(id uint, unitsCount int, errorMessage string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_sync_at": now,
		"units_count":  unitsCount,
		"updated_at":   now,
	}

	if errorMessage != "" {
		updates["last_error_at"] = now
		updates["error_message"] = errorMessage
		updates["error_count"] = gorm.Expr("error_count + 1")
	} else {
		updates["last_error_at"] = nil
		updates["error_message"] = ""
	}

	return s.UpdatePartial(id, updates)
}

// TestConnection проверяет подключение к Wialon
func (s *WialonConnectionService) TestConnection(connection *models.WialonConnection) (*TestConnectionResult, error) {
	wialonService := NewWialonService()

	// Тестируем подключение - используем стандартный TestConnection с формированием dataCenter
	dataCenter := connection.DataCenter
	if connection.ConnectionType == models.WialonConnectionTypeLocal {
		dataCenter = "" // Для Local не используется
	}

	// Для Local хоста нужно использовать специальный метод
	var result *TestConnectionResult
	var err error

	if connection.ConnectionType == models.WialonConnectionTypeLocal {
		// Для Local используем прямую авторизацию с хостом
		loginResp, loginErr := wialonService.LoginWithHost(connection.Host, connection.Token)
		if loginErr != nil {
			result = &TestConnectionResult{
				Success: false,
				Message: loginErr.Error(),
			}
		} else {
			userName := ""
			if loginResp.User != nil {
				userName = loginResp.User.Name
			}
			result = &TestConnectionResult{
				Success:      true,
				Message:      "Подключение к Wialon успешно установлено",
				UserName:     userName,
				SessionID:    loginResp.Eid,
				LocalVersion: loginResp.LocalVersion,
			}
			// Закрываем сессию
			go wialonService.LogoutWithHost(connection.Host, loginResp.Eid)
		}
	} else {
		result, err = wialonService.TestConnection(connection.Token, dataCenter)
		if err != nil {
			return nil, err
		}
	}

	// Обновляем статус подключения
	if result.Success {
		// При успешном тесте обновляем статус и очищаем ошибки
		updates := map[string]interface{}{
			"last_sync_at":  time.Now(),
			"error_message": "",
			"last_error_at": nil,
			"updated_at":    time.Now(),
		}
		if result.UserName != "" {
			updates["user_name"] = result.UserName
		}
		if result.LocalVersion != "" {
			updates["local_version"] = result.LocalVersion
		}

		// Пробуем получить количество объектов - используем оптимизированный метод без лимита
		unitsCount, err := wialonService.GetTotalUnitsCount(connection.Host, connection.Token)
		if err == nil {
			updates["units_count"] = unitsCount
		}

		s.UpdatePartial(connection.ID, updates)
	} else {
		// При неуспешном тесте записываем ошибку
		s.UpdatePartial(connection.ID, map[string]interface{}{
			"last_error_at": time.Now(),
			"error_message": result.Message,
			"updated_at":    time.Now(),
		})
	}

	return result, nil
}

// GetUnitsFromConnection получает объекты из конкретного подключения
func (s *WialonConnectionService) GetUnitsFromConnection(connectionID uint) ([]WialonUnitWithConnection, error) {
	connection, err := s.GetByIDWithToken(connectionID)
	if err != nil {
		return nil, fmt.Errorf("подключение не найдено: %w", err)
	}

	if !connection.IsActive {
		return nil, fmt.Errorf("подключение неактивно")
	}

	wialonService := NewWialonService()
	units, err := wialonService.SearchUnitsWithHost(connection.Host, connection.Token)
	if err != nil {
		// Обновляем статус ошибки
		s.UpdateSyncStatus(connectionID, 0, err.Error())
		return nil, err
	}

	// Обновляем статус успешной синхронизации
	s.UpdateSyncStatus(connectionID, len(units), "")

	// Добавляем информацию о подключении к каждому объекту
	result := make([]WialonUnitWithConnection, len(units))
	for i, unit := range units {
		result[i] = WialonUnitWithConnection{
			WialonUnit:     unit,
			ConnectionID:   connection.ID,
			ConnectionName: connection.Name,
			SourceHost:     connection.Host,
		}
	}

	return result, nil
}

// WialonUnitWithConnection объект Wialon с информацией о подключении
type WialonUnitWithConnection struct {
	WialonUnit
	ConnectionID   uint   `json:"connection_id"`
	ConnectionName string `json:"connection_name"`
	SourceHost     string `json:"source_host"`
}

// GetAllUnitsFromActiveConnections получает объекты из всех активных подключений компании.
// Возвращает (units, partial). partial=true если хотя бы одно подключение упало —
// caller должен НЕ кэшировать неполный результат.
func (s *WialonConnectionService) GetAllUnitsFromActiveConnections(companyID uint) ([]WialonUnitWithConnection, error) {
	units, _, err := s.GetAllUnitsFromActiveConnectionsDetailed(companyID)
	return units, err
}

// GetAllUnitsFromActiveConnectionsDetailed — расширенная версия: возвращает (units, partial, err).
// partial=true когда хотя бы один connection отдал ошибку (timeout WH и т.п.).
func (s *WialonConnectionService) GetAllUnitsFromActiveConnectionsDetailed(companyID uint) ([]WialonUnitWithConnection, bool, error) {
	connections, err := s.GetActiveByCompany(companyID)
	if err != nil {
		return nil, false, err
	}

	if len(connections) == 0 {
		return []WialonUnitWithConnection{}, false, nil
	}

	var allUnits []WialonUnitWithConnection
	var wg sync.WaitGroup
	var mu sync.Mutex
	errs := make([]error, 0)

	for _, conn := range connections {
		wg.Add(1)
		go func(connection models.WialonConnection) {
			defer wg.Done()

			units, err := s.GetUnitsFromConnection(connection.ID)
			if err != nil {
				log.Printf("Ошибка получения объектов из подключения %s: %v", connection.Name, err)
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", connection.Name, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			allUnits = append(allUnits, units...)
			mu.Unlock()
		}(conn)
	}

	wg.Wait()

	partial := len(errs) > 0
	if partial {
		log.Printf("⚠️ wialon all-units partial (errors=%d/%d connections)", len(errs), len(connections))
	}

	return allUnits, partial, nil
}

// GetConnectionStats возвращает статистику подключений
func (s *WialonConnectionService) GetConnectionStats(companyID uint) (*ConnectionStats, error) {
	connections, err := s.GetAllByCompany(companyID)
	if err != nil {
		return nil, err
	}

	stats := &ConnectionStats{
		Total:       len(connections),
		Active:      0,
		Inactive:    0,
		WithErrors:  0,
		TotalUnits:  0,
		Connections: make([]ConnectionSummary, len(connections)),
	}

	for i, conn := range connections {
		if conn.IsActive {
			stats.Active++
		} else {
			stats.Inactive++
		}

		if conn.LastErrorAt != nil && time.Since(*conn.LastErrorAt) < 24*time.Hour {
			stats.WithErrors++
		}

		stats.TotalUnits += conn.UnitsCount

		// Формируем короткую метку для источника
		shortLabel := ""
		if conn.ConnectionType == models.WialonConnectionTypeHosting {
			shortLabel = fmt.Sprintf("WH(%s)", conn.UserName)
		} else {
			shortLabel = fmt.Sprintf("WL(%s)", conn.UserName)
		}

		stats.Connections[i] = ConnectionSummary{
			ID:             conn.ID,
			Name:           conn.Name,
			ConnectionType: string(conn.ConnectionType),
			Host:           conn.Host,
			IsActive:       conn.IsActive,
			UnitsCount:     conn.UnitsCount,
			LastSyncAt:     conn.LastSyncAt,
			HasError:       conn.LastErrorAt != nil && time.Since(*conn.LastErrorAt) < 24*time.Hour,
			UserName:       conn.UserName,
			ShortLabel:     shortLabel,
		}
	}

	return stats, nil
}

// ConnectionStats статистика подключений
type ConnectionStats struct {
	Total       int                 `json:"total"`
	Active      int                 `json:"active"`
	Inactive    int                 `json:"inactive"`
	WithErrors  int                 `json:"with_errors"`
	TotalUnits  int                 `json:"total_units"`
	Connections []ConnectionSummary `json:"connections"`
}

// ConnectionSummary краткая информация о подключении
type ConnectionSummary struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	ConnectionType string     `json:"connection_type"`
	Host           string     `json:"host"`
	IsActive       bool       `json:"is_active"`
	UnitsCount     int        `json:"units_count"`
	LastSyncAt     *time.Time `json:"last_sync_at"`
	HasError       bool       `json:"has_error"`
	UserName       string     `json:"user_name"`   // Имя учетной записи в Wialon
	ShortLabel     string     `json:"short_label"` // WH(ACRM) или WL(Профмонитор)
}
