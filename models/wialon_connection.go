package models

import (
	"time"

	"gorm.io/gorm"

	"backend_axenta/utils"
)

// WialonConnectionType тип подключения Wialon
type WialonConnectionType string

const (
	WialonConnectionTypeHosting WialonConnectionType = "hosting" // Облачный Wialon Hosting
	WialonConnectionTypeLocal   WialonConnectionType = "local"   // Локальный Wialon Local
)

// WialonConnection представляет подключение к серверу Wialon
type WialonConnection struct {
	ID        uint           `json:"id" gorm:"primarykey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Основные поля
	CompanyID      uint                 `json:"company_id" gorm:"not null;index"`
	Name           string               `json:"name" gorm:"not null;type:varchar(255)"`           // "Офис Алматы", "Филиал Караганда"
	ConnectionType WialonConnectionType `json:"connection_type" gorm:"not null;type:varchar(50)"` // "hosting" или "local"
	Host           string               `json:"host" gorm:"not null;type:varchar(255)"`           // URL хоста API (https://hst-api.wialon.com или пользовательский)
	DataCenter     string               `json:"data_center" gorm:"type:varchar(50)"`              // Для hosting: com, us, eu, org, alt
	Token          string               `json:"-" gorm:"type:text"`                               // API токен (не возвращается в JSON)
	TokenMasked    string               `json:"token_masked" gorm:"-"`                            // Маскированный токен для отображения

	// Информация о пользователе и сервере Wialon
	UserName     string `json:"user_name" gorm:"type:varchar(255)"`     // Имя пользователя Wialon
	LocalVersion string `json:"local_version" gorm:"type:varchar(100)"` // Версия Wialon Local (например: local_2504)
	CmsURL       string `json:"cms_url" gorm:"type:varchar(255)"`       // URL CMS-интерфейса (опционально, например: https://cms-app.gpsnetwork.ru)

	// Статус
	IsActive   bool       `json:"is_active" gorm:"default:true"`
	LastSyncAt *time.Time `json:"last_sync_at"`
	UnitsCount int        `json:"units_count" gorm:"default:0"` // Количество объектов

	// Настройки синхронизации
	SyncInterval    int  `json:"sync_interval" gorm:"default:5"`        // Интервал синхронизации в минутах
	AutoSyncEnabled bool `json:"auto_sync_enabled" gorm:"default:true"` // Автоматическая синхронизация

	// Данные для синхронизации
	SyncVehicles    bool `json:"sync_vehicles" gorm:"default:true"`     // Синхронизация объектов
	SyncSensors     bool `json:"sync_sensors" gorm:"default:false"`     // Синхронизация датчиков
	SyncMaintenance bool `json:"sync_maintenance" gorm:"default:false"` // Синхронизация ТО
	SyncDrivers     bool `json:"sync_drivers" gorm:"default:false"`     // Синхронизация водителей
	SyncGeozones    bool `json:"sync_geozones" gorm:"default:false"`    // Синхронизация геозон

	// Статистика ошибок
	LastErrorAt  *time.Time `json:"last_error_at"`
	ErrorMessage string     `json:"error_message" gorm:"type:text"`
	ErrorCount   int        `json:"error_count" gorm:"default:0"`

	// Связи
	Company *Company `json:"company,omitempty" gorm:"foreignKey:CompanyID;constraint:-"`
}

// TableName задает имя таблицы
func (WialonConnection) TableName() string {
	return "wialon_connections"
}

// BeforeCreate хук перед созданием
func (w *WialonConnection) BeforeCreate(tx *gorm.DB) error {
	// Устанавливаем хост на основе типа подключения и дата-центра
	if w.ConnectionType == WialonConnectionTypeHosting && w.Host == "" {
		w.Host = GetWialonHostingURL(w.DataCenter)
	}
	return w.encryptToken()
}

// BeforeUpdate хук — encrypt token при сохранении (struct save).
// На raw map updates через .Updates(map[string]interface{}{...}) не сработает —
// для таких случаев encrypt вручную через utils.EncryptString.
func (w *WialonConnection) BeforeUpdate(tx *gorm.DB) error {
	return w.encryptToken()
}

// AfterFind хук — decrypt token после загрузки из БД. Идемпотентно через
// префикс "enc:v1:" — если токен plaintext (legacy до миграции), возвращается as-is.
func (w *WialonConnection) AfterFind(tx *gorm.DB) error {
	if w.Token == "" {
		return nil
	}
	plain, err := utils.DecryptString(w.Token)
	if err != nil {
		return err
	}
	w.Token = plain
	return nil
}

func (w *WialonConnection) encryptToken() error {
	if w.Token == "" {
		return nil
	}
	enc, err := utils.EncryptString(w.Token)
	if err != nil {
		return err
	}
	w.Token = enc
	return nil
}

// GetWialonHostingURL возвращает URL хоста для дата-центра Wialon Hosting
func GetWialonHostingURL(dataCenter string) string {
	switch dataCenter {
	case "us":
		return "https://hst-api.wialon.us"
	case "eu":
		return "https://hst-api.wialon.eu"
	case "org":
		return "https://hst-api.wialon.org"
	case "alt":
		return "https://hst-api.regwialon.com"
	default:
		return "https://hst-api.wialon.com"
	}
}

// GetOAuthURL возвращает URL для OAuth авторизации
func (w *WialonConnection) GetOAuthURL() string {
	if w.ConnectionType == WialonConnectionTypeLocal {
		// Для Local используем тот же хост, но заменяем hst-api на hosting
		// Например: https://wialon.mycompany.kz -> https://wialon.mycompany.kz (OAuth endpoint)
		return w.Host + "/login.html"
	}

	// Для Hosting используем стандартные URL
	switch w.DataCenter {
	case "us":
		return "https://hosting.wialon.us"
	case "eu":
		return "https://hosting.wialon.eu"
	case "org":
		return "https://hosting.wialon.org"
	case "alt":
		return "https://hosting.regwialon.com"
	default:
		return "https://hosting.wialon.com"
	}
}

// GetCmsURL возвращает URL для CMS-интерфейса
func (w *WialonConnection) GetCmsURL() string {
	// Если CmsURL указан явно, используем его
	if w.CmsURL != "" {
		return w.CmsURL
	}

	// Для Local fallback на /cms_manager/ путь
	if w.ConnectionType == WialonConnectionTypeLocal {
		return w.Host + "/cms_manager/"
	}

	// Для Hosting используем стандартные URL (cms.wialon.*)
	switch w.DataCenter {
	case "us":
		return "https://cms.wialon.us"
	case "eu":
		return "https://cms.wialon.eu"
	case "org":
		return "https://cms.wialon.org"
	case "alt":
		return "https://cms.regwialon.com"
	default:
		return "https://cms.wialon.com"
	}
}

// MaskToken маскирует токен для отображения
func (w *WialonConnection) MaskToken() {
	if len(w.Token) > 8 {
		w.TokenMasked = w.Token[:4] + "..." + w.Token[len(w.Token)-4:]
	} else if w.Token != "" {
		w.TokenMasked = "***"
	}
}

// IsHealthy проверяет, работает ли подключение нормально
func (w *WialonConnection) IsHealthy() bool {
	if !w.IsActive {
		return false
	}

	// Если последняя ошибка была недавно (в течение часа), считаем подключение нездоровым
	if w.LastErrorAt != nil && time.Since(*w.LastErrorAt) < time.Hour {
		return false
	}

	return true
}

// UpdateError обновляет информацию об ошибке
func (w *WialonConnection) UpdateError(errorMessage string) {
	now := time.Now()
	w.LastErrorAt = &now
	w.ErrorMessage = errorMessage
	w.ErrorCount++
	w.UpdatedAt = now
}

// ClearError очищает информацию об ошибке
func (w *WialonConnection) ClearError() {
	w.LastErrorAt = nil
	w.ErrorMessage = ""
}

// UpdateSyncStats обновляет статистику синхронизации
func (w *WialonConnection) UpdateSyncStats(unitsCount int) {
	now := time.Now()
	w.LastSyncAt = &now
	w.UnitsCount = unitsCount
	w.UpdatedAt = now
	w.ClearError()
}
