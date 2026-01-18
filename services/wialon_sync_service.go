package services

import (
	"backend_axenta/models"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// WialonSyncService сервис для синхронизации данных из Wialon
type WialonSyncService struct {
	db            *gorm.DB
	wialonService *WialonService
}

// NewWialonSyncService создает новый экземпляр сервиса синхронизации
func NewWialonSyncService(db *gorm.DB) *WialonSyncService {
	return &WialonSyncService{
		db:            db,
		wialonService: NewWialonService(),
	}
}

// SourceStats статистика по одному источнику
type SourceStats struct {
	ConnectionID   uint   `json:"connection_id"`
	ConnectionName string `json:"connection_name"`
	ConnectionType string `json:"connection_type"` // "hosting" или "local"
	UserName       string `json:"user_name"`       // Имя учетной записи (ACRM, Профмонитор)
	ShortLabel     string `json:"short_label"`     // WH(ACRM) или WL(Профмонитор)
	UnitsTotal     int    `json:"units_total"`
	UnitsActive    int    `json:"units_active"`
	UsersCount     int    `json:"users_count"`
	AccountsCount  int    `json:"accounts_count"`
	IsActive       bool   `json:"is_active"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	Error          string `json:"error,omitempty"`
}

// SyncStats полная статистика синхронизации
type SyncStats struct {
	Sources     []SourceStats `json:"sources"`
	TotalUnits  int           `json:"total_units"`
	TotalUsers  int           `json:"total_users"`
	TotalActive int           `json:"total_active"`
}

// GetSyncStats возвращает статистику по всем источникам Wialon
func (s *WialonSyncService) GetSyncStats(companyID uint) (*SyncStats, error) {
	// Получаем все активные подключения компании
	var connections []models.WialonConnection
	if err := s.db.Where("company_id = ? AND is_active = ?", companyID, true).Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("ошибка получения подключений: %w", err)
	}

	stats := &SyncStats{
		Sources: make([]SourceStats, 0, len(connections)),
	}

	// Параллельно получаем статистику из каждого подключения
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]SourceStats, len(connections))

	for i, conn := range connections {
		wg.Add(1)
		go func(idx int, connection models.WialonConnection) {
			defer wg.Done()

			source := SourceStats{
				ConnectionID:   connection.ID,
				ConnectionName: connection.Name,
				ConnectionType: string(connection.ConnectionType),
				UserName:       connection.UserName,
				IsActive:       connection.IsActive,
			}

			// Формируем короткую метку
			if connection.ConnectionType == models.WialonConnectionTypeHosting {
				source.ShortLabel = fmt.Sprintf("WH(%s)", connection.UserName)
			} else {
				source.ShortLabel = fmt.Sprintf("WL(%s)", connection.UserName)
			}

			// Используем сохранённое количество объектов
			source.UnitsTotal = connection.UnitsCount

			if connection.LastSyncAt != nil {
				source.LastSyncAt = connection.LastSyncAt.Format("2006-01-02T15:04:05Z")
			}

			if connection.ErrorMessage != "" {
				source.Error = connection.ErrorMessage
			}

			mu.Lock()
			results[idx] = source
			mu.Unlock()
		}(i, conn)
	}

	wg.Wait()

	// Собираем результаты
	for _, source := range results {
		if source.ConnectionID > 0 {
			stats.Sources = append(stats.Sources, source)
			stats.TotalUnits += source.UnitsTotal
			stats.TotalActive += source.UnitsActive
			stats.TotalUsers += source.UsersCount
		}
	}

	return stats, nil
}

// GetStatsFromConnection получает актуальную статистику из одного подключения
func (s *WialonSyncService) GetStatsFromConnection(connection *models.WialonConnection) (*SourceStats, error) {
	source := &SourceStats{
		ConnectionID:   connection.ID,
		ConnectionName: connection.Name,
		ConnectionType: string(connection.ConnectionType),
		UserName:       connection.UserName,
		IsActive:       connection.IsActive,
	}

	// Формируем короткую метку
	if connection.ConnectionType == models.WialonConnectionTypeHosting {
		source.ShortLabel = fmt.Sprintf("WH(%s)", connection.UserName)
	} else {
		source.ShortLabel = fmt.Sprintf("WL(%s)", connection.UserName)
	}

	// Получаем объекты из Wialon
	units, err := s.wialonService.SearchUnitsWithHost(connection.Host, connection.Token)
	if err != nil {
		source.Error = err.Error()
		return source, nil
	}

	source.UnitsTotal = len(units)

	// Подсчитываем активные объекты (с сообщениями за последние 24 часа)
	// now := time.Now().Unix()
	// oneDayAgo := now - 86400
	for range units {
		// Пока считаем все как активные
		source.UnitsActive++
	}

	return source, nil
}

// RefreshConnectionStats обновляет статистику для одного подключения и сохраняет в БД
func (s *WialonSyncService) RefreshConnectionStats(connectionID uint) (*SourceStats, error) {
	var connection models.WialonConnection
	if err := s.db.First(&connection, connectionID).Error; err != nil {
		return nil, fmt.Errorf("подключение не найдено: %w", err)
	}

	stats, err := s.GetStatsFromConnection(&connection)
	if err != nil {
		return nil, err
	}

	// Обновляем данные в БД
	updates := map[string]interface{}{
		"units_count": stats.UnitsTotal,
	}

	if stats.Error != "" {
		updates["error_message"] = stats.Error
	} else {
		updates["error_message"] = ""
	}

	s.db.Model(&connection).Updates(updates)

	return stats, nil
}
