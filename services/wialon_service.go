package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WialonService сервис для работы с Wialon Remote API
type WialonService struct {
	httpClient *http.Client
}

// NewWialonService создает новый экземпляр сервиса Wialon
func NewWialonService() *WialonService {
	return &WialonService{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WialonDataCenter дата-центры Wialon
type WialonDataCenter string

const (
	WialonDataCenterCom WialonDataCenter = "com"
	WialonDataCenterUS  WialonDataCenter = "us"
	WialonDataCenterEU  WialonDataCenter = "eu"
	WialonDataCenterOrg WialonDataCenter = "org"
)

// GetAPIHost возвращает хост API для указанного дата-центра
func GetAPIHost(dataCenter string) string {
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

// WialonLoginResponse ответ на запрос авторизации
type WialonLoginResponse struct {
	Eid   string       `json:"eid"`   // Session ID
	Host  string       `json:"host"`  // Хост для дальнейших запросов
	User  *WialonUser  `json:"user"`  // Информация о пользователе
	Error *WialonError `json:"error"` // Ошибка (если есть)
}

// WialonUser информация о пользователе Wialon
type WialonUser struct {
	ID   int64  `json:"id"`
	Name string `json:"nm"`
}

// WialonError ошибка Wialon API
type WialonError struct {
	Code    int    `json:"error"`
	Message string `json:"reason"`
}

// WialonUnit объект мониторинга
type WialonUnit struct {
	ID               int64   `json:"id"`
	Name             string  `json:"nm"`
	UniqueID         string  `json:"uid"`          // IMEI или уникальный ID
	PhoneNumber      string  `json:"ph"`           // Телефон
	PhoneNumber2     string  `json:"ph2"`          // Телефон 2
	Mileage          float64 `json:"cnm"`          // Пробег
	EngineHours      float64 `json:"cneh"`         // Моточасы
	HardwareType     int64   `json:"hw"`           // ID типа устройства
	HardwareTypeName string  `json:"hw_name"`      // Название типа устройства
	LastMessage      int64   `json:"last_message"` // Время последнего сообщения (UTC)
	CreatedAt        int64   `json:"ct"`           // Время создания объекта (UTC)
}

// WialonHardwareType тип оборудования
type WialonHardwareType struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HWCategory string `json:"hw_category"`
}

// WialonSensor датчик объекта
type WialonSensor struct {
	ID    int64  `json:"id"`
	Name  string `json:"n"`
	Type  string `json:"t"`
	Param string `json:"p"`
}

// WialonDriver водитель
type WialonDriver struct {
	ID    int64  `json:"id"`
	Name  string `json:"n"`
	Code  string `json:"c"`
	Phone string `json:"p"`
}

// WialonGeofence геозона
type WialonGeofence struct {
	ID          int64  `json:"id"`
	Name        string `json:"n"`
	Description string `json:"d"`
	Type        int    `json:"t"` // 1=polygon, 2=line, 3=circle
}

// WialonSearchResponse ответ на поиск объектов
type WialonSearchResponse struct {
	SearchSpec struct {
		ItemsType     string `json:"itemsType"`
		PropName      string `json:"propName"`
		PropValueMask string `json:"propValueMask"`
	} `json:"searchSpec"`
	TotalItemsCount int                      `json:"totalItemsCount"`
	Items           []map[string]interface{} `json:"items"`
}

// TestConnectionResult результат теста подключения
type TestConnectionResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	UserName     string `json:"user_name,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
	ResponseTime int64  `json:"response_time_ms,omitempty"`
}

// Login авторизация через токен
func (s *WialonService) Login(token string, dataCenter string) (*WialonLoginResponse, error) {
	host := GetAPIHost(dataCenter)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	params := url.Values{}
	params.Set("svc", "token/login")
	params.Set("params", fmt.Sprintf(`{"token":"%s"}`, token))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к Wialon API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	// Проверяем на ошибку Wialon
	var errorResp struct {
		Error  int    `json:"error"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != 0 {
		return nil, fmt.Errorf("ошибка Wialon API (код %d): %s", errorResp.Error, s.getErrorMessage(errorResp.Error))
	}

	var loginResp WialonLoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if loginResp.Eid == "" {
		return nil, fmt.Errorf("не удалось получить идентификатор сессии")
	}

	return &loginResp, nil
}

// TestConnection тестирует подключение к Wialon API
func (s *WialonService) TestConnection(token string, dataCenter string) (*TestConnectionResult, error) {
	start := time.Now()

	loginResp, err := s.Login(token, dataCenter)
	if err != nil {
		return &TestConnectionResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	responseTime := time.Since(start).Milliseconds()

	userName := ""
	if loginResp.User != nil {
		userName = loginResp.User.Name
	}

	// Закрываем сессию
	go s.Logout(loginResp.Eid, dataCenter)

	return &TestConnectionResult{
		Success:      true,
		Message:      "Подключение к Wialon успешно установлено",
		UserName:     userName,
		SessionID:    loginResp.Eid,
		ResponseTime: responseTime,
	}, nil
}

// Logout завершает сессию Wialon
func (s *WialonService) Logout(sessionID string, dataCenter string) error {
	host := GetAPIHost(dataCenter)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	params := url.Values{}
	params.Set("svc", "core/logout")
	params.Set("sid", sessionID)
	params.Set("params", "{}")

	_, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	return err
}

// SearchUnits поиск объектов мониторинга
func (s *WialonService) SearchUnits(sessionID string, dataCenter string) ([]WialonUnit, error) {
	host := GetAPIHost(dataCenter)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Флаги для запроса данных объектов (в DEC формате)
	// 0x00000001 (1) - базовые свойства (nm, id и т.д.)
	// 0x00000002 (2) - пользовательские свойства (prp, ct - время создания)
	// 0x00000100 (256) - расширенные свойства (uid, hw, ph, ph2)
	// 0x00000400 (1024) - последнее сообщение и местоположение (pos, lmsg)
	// 0x00002000 (8192) - счётчики (cnm, cneh)
	flags := 0x00000001 | 0x00000002 | 0x00000100 | 0x00000400 | 0x00002000

	searchParams := fmt.Sprintf(`{
		"spec": {
			"itemsType": "avl_unit",
			"propName": "sys_name",
			"propValueMask": "*",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": %d,
		"from": 0,
		"to": 0
	}`, flags)

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(searchParams, "\n", ""))
	params.Set("params", strings.ReplaceAll(params.Get("params"), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса объектов: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResp WialonSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	units := make([]WialonUnit, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		unit := WialonUnit{}

		if id, ok := item["id"].(float64); ok {
			unit.ID = int64(id)
		}
		if nm, ok := item["nm"].(string); ok {
			unit.Name = nm
		}
		if uid, ok := item["uid"].(string); ok {
			unit.UniqueID = uid
		}
		if ph, ok := item["ph"].(string); ok {
			unit.PhoneNumber = ph
		}
		if ph2, ok := item["ph2"].(string); ok {
			unit.PhoneNumber2 = ph2
		}
		if cnm, ok := item["cnm"].(float64); ok {
			unit.Mileage = cnm
		}
		if cneh, ok := item["cneh"].(float64); ok {
			unit.EngineHours = cneh
		}
		if hw, ok := item["hw"].(float64); ok {
			unit.HardwareType = int64(hw)
		}
		// Получаем время последнего сообщения из pos.t
		if pos, ok := item["pos"].(map[string]interface{}); ok {
			if t, ok := pos["t"].(float64); ok {
				unit.LastMessage = int64(t)
			}
		}
		// Получаем время создания объекта (ct)
		if ct, ok := item["ct"].(float64); ok {
			unit.CreatedAt = int64(ct)
		}

		units = append(units, unit)
	}

	return units, nil
}

// getErrorMessage возвращает сообщение об ошибке по коду
func (s *WialonService) getErrorMessage(code int) string {
	errorMessages := map[int]string{
		0:    "Успешный запрос",
		1:    "Неизвестная ошибка сервиса",
		2:    "Недействительная сессия",
		3:    "Недействительный сервис",
		4:    "Недействительный результат",
		5:    "Ошибка выполнения удаленного вызова",
		6:    "Запрос недоступен из-за ограничений",
		7:    "Доступ запрещен",
		8:    "Токен недействителен или истек",
		9:    "Лимит вызовов API исчерпан",
		10:   "DNS ошибка",
		11:   "Ошибка соединения с хостом",
		14:   "Такой элемент уже существует",
		1001: "Неверные параметры",
		1002: "Внутренняя ошибка",
		1003: "Длительный API запрос",
		1004: "Неверный формат ключа",
		1005: "Неверные данные сессии",
	}

	if msg, ok := errorMessages[code]; ok {
		return msg
	}
	return "Неизвестная ошибка"
}

// WialonAccount аккаунт (пользователь) Wialon
type WialonAccount struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"` // "user" или "resource"
	IsActive      bool   `json:"is_active"`
	ObjectsTotal  int    `json:"objects_total"`
	ObjectsActive int    `json:"objects_active"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// SearchUsers поиск пользователей (аккаунтов) Wialon
func (s *WialonService) SearchUsers(sessionID string, dataCenter string) ([]WialonAccount, error) {
	host := GetAPIHost(dataCenter)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Флаги для пользователей
	// 0x00000001 (1) — базовая информация
	// 0x00000002 (2) — пользовательские поля
	// 0x00000100 (256) — биллинг информация
	flags := 0x00000001 | 0x00000002 | 0x00000100

	searchParams := fmt.Sprintf(`{
		"spec": {
			"itemsType": "user",
			"propName": "sys_name",
			"propValueMask": "*",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": %d,
		"from": 0,
		"to": 0
	}`, flags)

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса пользователей: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResp WialonSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	accounts := make([]WialonAccount, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		account := WialonAccount{
			Type:     "user",
			IsActive: true, // По умолчанию активен
		}

		if id, ok := item["id"].(float64); ok {
			account.ID = int64(id)
		}
		if nm, ok := item["nm"].(string); ok {
			account.Name = nm
		}
		// Проверяем флаги пользователя
		if fl, ok := item["fl"].(float64); ok {
			// Флаг 0x00000001 - пользователь отключен
			if int(fl)&0x00000001 != 0 {
				account.IsActive = false
			}
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// GetAccountsWithObjects возвращает аккаунты с количеством объектов
func (s *WialonService) GetAccountsWithObjects(token string, dataCenter string) ([]WialonAccount, error) {
	// Авторизуемся
	loginResp, err := s.Login(token, dataCenter)
	if err != nil {
		return nil, err
	}
	defer s.Logout(loginResp.Eid, dataCenter)

	// Получаем пользователей
	accounts, err := s.SearchUsers(loginResp.Eid, dataCenter)
	if err != nil {
		return nil, err
	}

	// Получаем объекты для подсчёта
	units, err := s.SearchUnits(loginResp.Eid, dataCenter)
	if err != nil {
		// Если не удалось получить объекты, возвращаем аккаунты без статистики
		return accounts, nil
	}

	// Подсчитываем общее количество объектов (для текущего пользователя)
	totalUnits := len(units)

	// Для главного аккаунта добавляем статистику объектов
	if len(accounts) > 0 {
		accounts[0].ObjectsTotal = totalUnits
		accounts[0].ObjectsActive = totalUnits // Все объекты считаем активными
	}

	return accounts, nil
}

// GetHardwareTypes получает список типов оборудования
func (s *WialonService) GetHardwareTypes(sessionID string, dataCenter string) (map[int64]string, error) {
	host := GetAPIHost(dataCenter)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	params := url.Values{}
	params.Set("svc", "core/get_hw_types")
	params.Set("sid", sessionID)
	params.Set("params", "{}")

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса типов оборудования: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var hwTypes []map[string]interface{}
	if err := json.Unmarshal(body, &hwTypes); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Создаём карту ID -> Name
	hwMap := make(map[int64]string)
	for _, hw := range hwTypes {
		if id, ok := hw["id"].(float64); ok {
			if name, ok := hw["name"].(string); ok {
				hwMap[int64(id)] = name
			}
		}
	}

	return hwMap, nil
}
