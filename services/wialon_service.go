package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
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
	Eid          string       `json:"eid"`           // Session ID
	Host         string       `json:"host"`          // Хост для дальнейших запросов
	User         *WialonUser  `json:"user"`          // Информация о пользователе
	Error        *WialonError `json:"error"`         // Ошибка (если есть)
	LocalVersion string       `json:"local_version"` // Версия Wialon Local (например: local_2504)
	Api          string       `json:"api"`           // Тип API: "local" или "hosting"
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
	CreatorId        int64   `json:"crt"`          // ID пользователя-создателя
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
	LocalVersion string `json:"local_version,omitempty"` // Версия Wialon Local
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
		// Получаем ID создателя (crt)
		if crt, ok := item["crt"].(float64); ok {
			unit.CreatorId = int64(crt)
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
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"` // "user" или "resource"
	IsActive         bool   `json:"is_active"`
	ObjectsTotal     int    `json:"objects_total"`
	ObjectsActive    int    `json:"objects_active"`
	CreatedAt        string `json:"created_at,omitempty"`
	ParentId         int64  `json:"parent_id,omitempty"`          // ID родительской учётной записи (bpact)
	ParentName       string `json:"parent_name,omitempty"`        // Имя родительской учётной записи
	DealerRights     bool   `json:"dealer_rights"`                // Права дилера
	BillingAccountID int64  `json:"billing_account_id,omitempty"` // ID ресурса биллинга (bact)
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
	defer func() {
		if err := s.Logout(loginResp.Eid, dataCenter); err != nil {
			log.Printf("Ошибка при выходе из Wialon (defer): %v", err)
		}
	}()

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

// GetAccountsQuickFromHost быстро загружает аккаунты БЕЗ статистики объектов
// Используется для быстрой первоначальной загрузки страницы
// ObjectsTotal = -1 означает "загружается"
func (s *WialonService) GetAccountsQuickFromHost(host string, token string) ([]WialonAccount, error) {
	// Авторизуемся через хост
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := s.LogoutWithHost(host, loginResp.Eid); err != nil {
			log.Printf("Ошибка при выходе из Wialon (host, defer): %v", err)
		}
	}()

	// Получаем пользователей
	accounts, err := s.SearchUsersWithHost(host, loginResp.Eid)
	if err != nil {
		return nil, err
	}

	// Получаем информацию о дилерах из ресурсов
	dealerMap, _ := s.SearchResourcesWithHost(host, loginResp.Eid)
	if dealerMap != nil {
		for i := range accounts {
			if isDealer, ok := dealerMap[accounts[i].ID]; ok && isDealer {
				accounts[i].DealerRights = true
			}
		}
	}

	// Получаем информацию о заблокированных аккаунтах (sys_account_disabled = 1)
	disabledMap, disabledErr := s.GetAccountDisabledStatusWithHost(host, loginResp.Eid)
	if disabledErr == nil && disabledMap != nil {
		for i := range accounts {
			if isDisabled, ok := disabledMap[accounts[i].ID]; ok && isDisabled {
				accounts[i].IsActive = false
			}
		}
	}

	// Создаём карту ВСЕХ пользователей ДО фильтрации
	// Это нужно для корректного заполнения ParentName - родитель может не быть биллинговым
	allAccountsMap := make(map[int64]string) // ID -> Name
	for _, acc := range accounts {
		allAccountsMap[acc.ID] = acc.Name
	}
	// Добавляем главный аккаунт подключения (loginResp.User) в карту
	// Это позволяет найти создателя, если ParentId указывает на главный аккаунт
	if loginResp.User != nil && loginResp.User.ID > 0 {
		allAccountsMap[loginResp.User.ID] = loginResp.User.Name
	}

	// Фильтруем аккаунты — оставляем только биллинговые
	billingMap, billingErr := s.SearchAllBillingAccountsWithHost(host, loginResp.Eid)
	if billingErr == nil && billingMap != nil {
		filteredAccounts := make([]WialonAccount, 0)
		for _, acc := range accounts {
			if billingMap[acc.ID] {
				filteredAccounts = append(filteredAccounts, acc)
			}
		}
		log.Printf("⚡ Wialon QUICK: отфильтровано %d биллинговых из %d пользователей", len(filteredAccounts), len(accounts))
		accounts = filteredAccounts
	}

	// Заменяем имена пользователей на имена учетных записей (ресурсов)
	// Карта resourceNamesMap содержит: resourceID -> resourceName
	// Пользователь ссылается на ресурс через bact (BillingAccountID)
	resourceNamesMap, resourceErr := s.GetResourceNamesMapWithHost(host, loginResp.Eid)
	if resourceErr == nil && resourceNamesMap != nil {
		replacedCount := 0
		for i := range accounts {
			// Используем BillingAccountID (bact) для поиска имени ресурса
			if accounts[i].BillingAccountID > 0 {
				if resourceName, ok := resourceNamesMap[accounts[i].BillingAccountID]; ok && resourceName != "" {
					accounts[i].Name = resourceName
					replacedCount++
				}
			}
		}
		log.Printf("⚡ Wialon QUICK: заменено %d имён на имена учетных записей", replacedCount)

		// Обновляем карту имён после замены на имена ресурсов
		for _, acc := range accounts {
			allAccountsMap[acc.ID] = acc.Name
		}
	}

	// НЕ загружаем объекты — устанавливаем -1 (индикатор "загружается")
	for i := range accounts {
		accounts[i].ObjectsTotal = -1
		accounts[i].ObjectsActive = -1
	}

	// Заполняем ParentName используя полную карту (до фильтрации)
	// Это позволяет найти имя родителя, даже если он не биллинговый
	for i := range accounts {
		if accounts[i].ParentId > 0 {
			if parentName, ok := allAccountsMap[accounts[i].ParentId]; ok {
				accounts[i].ParentName = parentName
			}
		}
	}

	log.Printf("⚡ Wialon QUICK: загружено %d аккаунтов (без статистики объектов)", len(accounts))
	return accounts, nil
}

// GetAccountsWithObjectsFromHost возвращает аккаунты с количеством объектов используя кастомный хост
func (s *WialonService) GetAccountsWithObjectsFromHost(host string, token string) ([]WialonAccount, error) {
	// Авторизуемся через хост
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := s.LogoutWithHost(host, loginResp.Eid); err != nil {
			log.Printf("Ошибка при выходе из Wialon (host, defer): %v", err)
		}
	}()

	// Получаем пользователей
	accounts, err := s.SearchUsersWithHost(host, loginResp.Eid)
	if err != nil {
		return nil, err
	}

	// Получаем информацию о дилерах из ресурсов
	dealerMap, _ := s.SearchResourcesWithHost(host, loginResp.Eid)
	if dealerMap != nil {
		for i := range accounts {
			if isDealer, ok := dealerMap[accounts[i].ID]; ok && isDealer {
				accounts[i].DealerRights = true
			}
		}
	}

	// Получаем информацию о заблокированных аккаунтах (sys_account_disabled = 1)
	disabledMap, disabledErr := s.GetAccountDisabledStatusWithHost(host, loginResp.Eid)
	if disabledErr == nil && disabledMap != nil {
		for i := range accounts {
			if isDisabled, ok := disabledMap[accounts[i].ID]; ok && isDisabled {
				accounts[i].IsActive = false
				log.Printf("🚫 Wialon: аккаунт %s (ID:%d) помечен как заблокированный", accounts[i].Name, accounts[i].ID)
			}
		}
	}

	// Фильтруем аккаунты — оставляем только биллинговые (у которых есть связанный ресурс)
	// Главный аккаунт (с которого подключаемся) не показывается в списках
	billingMap, billingErr := s.SearchAllBillingAccountsWithHost(host, loginResp.Eid)
	if billingErr == nil && billingMap != nil {
		filteredAccounts := make([]WialonAccount, 0)
		for _, acc := range accounts {
			// Оставляем только аккаунты с биллинговым ресурсом
			if billingMap[acc.ID] {
				filteredAccounts = append(filteredAccounts, acc)
			}
		}
		log.Printf("📊 Wialon: отфильтровано %d биллинговых из %d пользователей", len(filteredAccounts), len(accounts))
		accounts = filteredAccounts
	}

	// Получаем объекты для подсчёта по каждому аккаунту (ОПТИМИЗИРОВАННО)
	// Используем минимальные флаги — только id и creator_id
	directUnitsPerAccount, totalUnits, err := s.GetUnitsCountForStats(host, loginResp.Eid)
	if err != nil {
		// Если не удалось получить объекты, возвращаем аккаунты без статистики
		log.Printf("⚠️ Wialon: не удалось получить статистику объектов: %v", err)
		return accounts, nil
	}

	log.Printf("📊 Wialon: получено %d объектов всего, %d аккаунтов", totalUnits, len(accounts))

	// Создаём карту аккаунтов по ID для быстрого доступа
	accountMap := make(map[int64]*WialonAccount)
	for i := range accounts {
		accountMap[accounts[i].ID] = &accounts[i]
	}

	// Создаём карту детей: parent_id -> []child_ids
	childrenMap := make(map[int64][]int64)
	for _, acc := range accounts {
		if acc.ParentId > 0 {
			childrenMap[acc.ParentId] = append(childrenMap[acc.ParentId], acc.ID)
		}
	}

	// Заполняем ParentName для каждого аккаунта
	for i := range accounts {
		if accounts[i].ParentId > 0 {
			if parent, ok := accountMap[accounts[i].ParentId]; ok {
				accounts[i].ParentName = parent.Name
			}
		}
	}

	// Рекурсивная функция для подсчёта объектов включая подчинённые аккаунты
	var countObjectsRecursive func(accountID int64, visited map[int64]bool) int
	countObjectsRecursive = func(accountID int64, visited map[int64]bool) int {
		// Защита от бесконечной рекурсии
		if visited[accountID] {
			return 0
		}
		visited[accountID] = true

		// Прямые объекты этого аккаунта
		total := directUnitsPerAccount[accountID]

		// Добавляем объекты подчинённых аккаунтов
		for _, childID := range childrenMap[accountID] {
			total += countObjectsRecursive(childID, visited)
		}

		return total
	}

	// Подсчитываем объекты для каждого аккаунта (включая подчинённые)
	for i := range accounts {
		visited := make(map[int64]bool)
		totalObjects := countObjectsRecursive(accounts[i].ID, visited)
		accounts[i].ObjectsTotal = totalObjects
		accounts[i].ObjectsActive = totalObjects // Пока считаем все активными

		if totalObjects > 0 {
			log.Printf("📊 Wialon: аккаунт %s (ID:%d, parent:%d, dealer:%v) = %d объектов",
				accounts[i].Name, accounts[i].ID, accounts[i].ParentId, accounts[i].DealerRights, totalObjects)
		}
	}

	// Подсчитываем нераспределённые объекты (те, чей CreatorId не найден среди аккаунтов)
	distributedCount := 0
	for creatorID, count := range directUnitsPerAccount {
		if _, exists := accountMap[creatorID]; exists {
			distributedCount += count
		}
	}
	undistributed := totalUnits - distributedCount

	log.Printf("📊 Wialon: распределено %d из %d объектов, нераспределённых: %d",
		distributedCount, totalUnits, undistributed)

	// Если остались нераспределённые объекты, добавляем их к главному
	if undistributed > 0 && len(accounts) > 0 {
		accounts[0].ObjectsTotal += undistributed
		accounts[0].ObjectsActive += undistributed
		log.Printf("📊 Wialon: %d нераспределённых объектов добавлено к главному аккаунту %s", undistributed, accounts[0].Name)
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

// ========== Методы для работы с кастомным хостом (для мульти-подключений) ==========

// LoginWithHost авторизация через токен с указанием хоста
func (s *WialonService) LoginWithHost(host string, token string) (*WialonLoginResponse, error) {
	// Убираем trailing slash
	host = strings.TrimSuffix(host, "/")
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

	// Логируем ответ для отладки (только первые 500 символов)
	bodyStr := string(body)
	if len(bodyStr) > 500 {
		log.Printf("📡 Wialon LoginWithHost ответ от %s: %s...(truncated, total %d bytes)", host, bodyStr[:500], len(bodyStr))
	} else {
		log.Printf("📡 Wialon LoginWithHost ответ от %s: %s", host, bodyStr)
	}

	// Проверяем на пустой или null ответ
	bodyStr = strings.TrimSpace(bodyStr)
	if bodyStr == "" || bodyStr == "null" || bodyStr == "NULL" {
		return nil, fmt.Errorf("пустой ответ от Wialon API - проверьте токен")
	}

	// Парсим как map для гибкости
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w (body: %s)", err, bodyStr[:min(100, len(bodyStr))])
	}

	// Проверяем на ошибку Wialon
	if errCode, ok := result["error"].(float64); ok && errCode != 0 {
		return nil, fmt.Errorf("ошибка Wialon API (код %d): %s", int(errCode), s.getErrorMessage(int(errCode)))
	}

	// Извлекаем данные
	loginResp := &WialonLoginResponse{}

	if eid, ok := result["eid"].(string); ok {
		loginResp.Eid = eid
	}
	if host, ok := result["host"].(string); ok {
		loginResp.Host = host
	}

	// Извлекаем пользователя
	if userObj, ok := result["user"].(map[string]interface{}); ok {
		loginResp.User = &WialonUser{}
		if id, ok := userObj["id"].(float64); ok {
			loginResp.User.ID = int64(id)
		}
		if nm, ok := userObj["nm"].(string); ok {
			loginResp.User.Name = nm
		}
	}

	// Извлекаем версию Local и тип API
	if localVersion, ok := result["local_version"].(string); ok {
		loginResp.LocalVersion = localVersion
	}
	if api, ok := result["api"].(string); ok {
		loginResp.Api = api
	}

	return loginResp, nil
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// LogoutWithHost завершает сессию Wialon с указанием хоста
func (s *WialonService) LogoutWithHost(host string, sessionID string) error {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	params := url.Values{}
	params.Set("svc", "core/logout")
	params.Set("sid", sessionID)
	params.Set("params", "{}")

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// TestConnectionWithHost тестирует подключение к Wialon API с указанием хоста
func (s *WialonService) TestConnectionWithHost(host string, token string) (*TestConnectionResult, error) {
	start := time.Now()

	loginResp, err := s.LoginWithHost(host, token)
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
	go s.LogoutWithHost(host, loginResp.Eid)

	return &TestConnectionResult{
		Success:      true,
		Message:      "Подключение к Wialon успешно установлено",
		UserName:     userName,
		ResponseTime: responseTime,
	}, nil
}

// GetUnitsCountForStats получает только количество объектов для статистики (оптимизированная версия)
// Использует минимальные флаги для быстрой загрузки только id и creator_id
func (s *WialonService) GetUnitsCountForStats(host string, eid string) (map[int64]int, int, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Минимальные флаги — только базовые свойства (id) и биллинг (crt - создатель)
	flags := 0x00000001 | // Базовые свойства (nm, id)
		0x00000004 // Биллинг (crt - создатель)

	searchParams := map[string]interface{}{
		"spec": map[string]interface{}{
			"itemsType":     "avl_unit",
			"propName":      "sys_name",
			"propValueMask": "*",
			"sortType":      "sys_name",
		},
		"force": 1,
		"flags": flags,
		"from":  0,
		"to":    3000, // Загружаем порциями по 3000
	}

	countPerCreator := make(map[int64]int)
	totalCount := 0
	pageSize := 3000
	currentFrom := 0

	for {
		searchParams["from"] = currentFrom
		searchParams["to"] = currentFrom + pageSize

		paramsJSON, _ := json.Marshal(searchParams)

		params := url.Values{}
		params.Set("svc", "core/search_items")
		params.Set("sid", eid)
		params.Set("params", string(paramsJSON))

		resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
		if err != nil {
			return nil, 0, fmt.Errorf("ошибка поиска объектов: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, 0, fmt.Errorf("ошибка чтения ответа: %w", err)
		}

		var searchResult struct {
			TotalItemsCount int                      `json:"totalItemsCount"`
			Items           []map[string]interface{} `json:"items"`
		}
		if err := json.Unmarshal(body, &searchResult); err != nil {
			return nil, 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
		}

		// Группируем объекты по creator_id
		for _, item := range searchResult.Items {
			if crt, ok := item["crt"].(float64); ok && crt > 0 {
				countPerCreator[int64(crt)]++
			}
		}

		totalCount = searchResult.TotalItemsCount
		loadedCount := currentFrom + len(searchResult.Items)

		log.Printf("📊 Wialon PAGED: загружено %d/%d объектов (страница %d)",
			loadedCount, totalCount, (currentFrom/pageSize)+1)

		// Проверяем нужно ли загружать ещё
		if len(searchResult.Items) < pageSize || loadedCount >= totalCount {
			break
		}

		currentFrom += pageSize

		// Защита от бесконечного цикла
		if currentFrom > 50000 {
			log.Printf("⚠️ Wialon: достигнут лимит 50000 объектов")
			break
		}
	}

	log.Printf("📊 Wialon OPTIMIZED: получено %d объектов всего, %d уникальных создателей",
		totalCount, len(countPerCreator))

	return countPerCreator, totalCount, nil
}

// UnitsStats статистика объектов учётной записи
type UnitsStats struct {
	Total       int `json:"total"`       // Все объекты (avl_unit)
	Active      int `json:"active"`      // Активные объекты (activated_units)
	Deactivated int `json:"deactivated"` // Деактивированные/сезонные объекты (seasonal_units)
}

// GetUnitsCountWithHierarchy получает количество объектов с учётом иерархии аккаунтов
// Использует Wialon API account/get_account_data для получения usage из услуг:
// - avl_unit: все объекты
// - activated_units: активные объекты
// - seasonal_units: деактивированные/сезонные объекты
func (s *WialonService) GetUnitsCountWithHierarchy(host string, eid string) (map[int64]UnitsStats, int, error) {
	// Сначала получаем все ресурсы (учётные записи)
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Ищем все ресурсы для получения их ID
	searchParams := `{
		"spec": {
			"itemsType": "avl_resource",
			"propName": "sys_name",
			"propValueMask": "*",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": 5,
		"from": 0,
		"to": 0
	}`

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", eid)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка запроса ресурсов: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResp WialonSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Собираем ID всех ресурсов
	resourceIDs := make([]int64, 0, len(searchResp.Items))
	resourceCreatorMap := make(map[int64]int64) // resourceID -> creatorID (user)

	for _, item := range searchResp.Items {
		if id, ok := item["id"].(float64); ok {
			resourceID := int64(id)
			resourceIDs = append(resourceIDs, resourceID)

			// Связь ресурса с пользователем через crt (creator)
			if crt, ok := item["crt"].(float64); ok {
				resourceCreatorMap[resourceID] = int64(crt)
			}
		}
	}

	log.Printf("📊 Wialon: найдено %d ресурсов для получения usage", len(resourceIDs))

	// Используем batch запрос с массивом itemId (по 100 за раз)
	unitsStatsMap := make(map[int64]UnitsStats)
	totalObjects := 0
	batchSize := 100

	for i := 0; i < len(resourceIDs); i += batchSize {
		end := i + batchSize
		if end > len(resourceIDs) {
			end = len(resourceIDs)
		}
		batchIDs := resourceIDs[i:end]

		// Формируем JSON массив ID
		idsJSON, err := json.Marshal(batchIDs)
		if err != nil {
			continue
		}

		// Формируем batch запрос к account/get_account_data
		// type=2: подробная информация с комбинированными настройками
		// Без type=4 — включает подчинённые аккаунты в usage
		accountDataParams := fmt.Sprintf(`{"itemId":%s,"type":2}`, string(idsJSON))

		params := url.Values{}
		params.Set("svc", "account/get_account_data")
		params.Set("sid", eid)
		params.Set("params", accountDataParams)

		resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
		if err != nil {
			log.Printf("⚠️ Wialon: ошибка batch запроса account_data: %v", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		// Парсим ответ — это объект {"resourceID": {...accountData...}}
		var batchResp map[string]map[string]interface{}
		if err := json.Unmarshal(body, &batchResp); err != nil {
			log.Printf("⚠️ Wialon: не удалось распарсить ответ batch: %v, первые 500 байт: %s", err, string(body[:min(len(body), 500)]))
			continue
		}

		// Отладка первого batch
		if i == 0 && len(batchResp) > 0 {
			for key := range batchResp {
				firstJSON, _ := json.Marshal(batchResp[key])
				log.Printf("📊 DEBUG: структура первого ответа account_data: %s", string(firstJSON[:min(len(firstJSON), 1000)]))
				break
			}
		}

		// Обрабатываем каждый ответ — ключи это resourceID в виде строки
		for resourceIDStr, accountData := range batchResp {
			resourceID, err := strconv.ParseInt(resourceIDStr, 10, 64)
			if err != nil {
				continue
			}

			// Извлекаем usage из settings.combined.services
			if settings, ok := accountData["settings"].(map[string]interface{}); ok {
				if combined, ok := settings["combined"].(map[string]interface{}); ok {
					if services, ok := combined["services"].(map[string]interface{}); ok {
						var stats UnitsStats

						// avl_unit — все объекты
						if avlUnit, ok := services["avl_unit"].(map[string]interface{}); ok {
							if usage, ok := avlUnit["usage"].(float64); ok {
								stats.Total = int(usage)
							}
						}

						// activated_units — активные объекты
						if activatedUnits, ok := services["activated_units"].(map[string]interface{}); ok {
							if usage, ok := activatedUnits["usage"].(float64); ok {
								stats.Active = int(usage)
							}
						}

						// seasonal_units — деактивированные/сезонные объекты
						if seasonalUnits, ok := services["seasonal_units"].(map[string]interface{}); ok {
							if usage, ok := seasonalUnits["usage"].(float64); ok {
								stats.Deactivated = int(usage)
							}
						}

						// Если не получили Total из avl_unit, используем Active + Deactivated
						if stats.Total == 0 && (stats.Active > 0 || stats.Deactivated > 0) {
							stats.Total = stats.Active + stats.Deactivated
						}

						// Сохраняем для resourceID
						if stats.Total > 0 || stats.Active > 0 {
							unitsStatsMap[resourceID] = stats
							totalObjects += stats.Total

							// Также сохраняем для creatorID (user ID)
							if creatorID, exists := resourceCreatorMap[resourceID]; exists {
								unitsStatsMap[creatorID] = stats
							}
						}
					}
				}
			}
		}

		log.Printf("📊 Wialon BATCH: обработано %d/%d ресурсов, найдено %d с usage", end, len(resourceIDs), len(unitsStatsMap))
	}

	log.Printf("📊 Wialon ACCOUNT_DATA: получено usage для %d аккаунтов, всего %d объектов",
		len(unitsStatsMap), totalObjects)

	return unitsStatsMap, totalObjects, nil

}

// SearchUnitsWithHost поиск объектов с указанием хоста
func (s *WialonService) SearchUnitsWithHost(host string, token string) ([]WialonUnit, error) {
	// Авторизуемся
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return nil, err
	}
	defer s.LogoutWithHost(host, loginResp.Eid)

	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Флаги для получения нужных данных
	flags := 0x00000001 | // Базовые свойства (nm, id)
		0x00000002 | // Пользовательские свойства (ct - время создания)
		0x00000004 | // Информация о биллинге (crt - создатель, bact - учётная запись)
		0x00000100 | // Расширенные свойства (uid, hw, ph, ph2)
		0x00000400 | // Последнее сообщение и местоположение
		0x00002000 // Счетчики

	searchParams := map[string]interface{}{
		"spec": map[string]interface{}{
			"itemsType":     "avl_unit",
			"propName":      "sys_name",
			"propValueMask": "*",
			"sortType":      "sys_name",
		},
		"force": 1,
		"flags": flags,
		"from":  0,
		"to":    10000, // Лимит для предотвращения таймаутов при больших аккаунтах (13k+ объектов)
	}

	paramsJSON, _ := json.Marshal(searchParams)

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", loginResp.Eid)
	params.Set("params", string(paramsJSON))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска объектов: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResult struct {
		TotalItemsCount int                      `json:"totalItemsCount"`
		Items           []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Получаем типы оборудования для названий
	hwTypes, _ := s.GetHardwareTypesWithHost(host, loginResp.Eid)

	// Преобразуем результаты в структуры
	units := make([]WialonUnit, 0, len(searchResult.Items))
	for _, item := range searchResult.Items {
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
		if hw, ok := item["hw"].(float64); ok {
			unit.HardwareType = int64(hw)
			if hwTypes != nil {
				if name, exists := hwTypes[unit.HardwareType]; exists {
					unit.HardwareTypeName = name
				}
			}
		}
		if ph, ok := item["ph"].(string); ok {
			unit.PhoneNumber = ph
		}
		if ph2, ok := item["ph2"].(string); ok {
			unit.PhoneNumber2 = ph2
		}
		if ct, ok := item["ct"].(float64); ok {
			unit.CreatedAt = int64(ct)
		}
		// Пробуем crt (creator ID)
		if crt, ok := item["crt"].(float64); ok {
			unit.CreatorId = int64(crt)
		}
		// Если нет crt, пробуем bact (billing account ID)
		if unit.CreatorId == 0 {
			if bact, ok := item["bact"].(float64); ok {
				unit.CreatorId = int64(bact)
			}
		}

		// Местоположение
		if pos, ok := item["pos"].(map[string]interface{}); ok {
			if t, ok := pos["t"].(float64); ok {
				unit.LastMessage = int64(t)
			}
		}

		// Счетчики
		if cnm, ok := item["cnm"].(float64); ok {
			unit.Mileage = cnm
		}
		if cneh, ok := item["cneh"].(float64); ok {
			unit.EngineHours = cneh
		}

		units = append(units, unit)
	}

	return units, nil
}

// GetHardwareTypesWithHost получает список типов оборудования с указанием хоста
func (s *WialonService) GetHardwareTypesWithHost(host string, sessionID string) (map[int64]string, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	params := url.Values{}
	params.Set("svc", "core/get_hw_types")
	params.Set("sid", sessionID)
	params.Set("params", "{}")

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения типов оборудования: %w", err)
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

// SearchUsersWithHost поиск пользователей (аккаунтов) Wialon с указанием хоста
func (s *WialonService) SearchUsersWithHost(host string, sessionID string) ([]WialonAccount, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Флаги для пользователей
	// 0x00000001 (1) — базовая информация (id, nm)
	// 0x00000002 (2) — пользовательские свойства (ct - время создания)
	// 0x00000004 (4) — информация о биллинге (bact, bpact - родительский аккаунт)
	// 0x00000100 (256) — расширенные свойства
	flags := 0x00000001 | 0x00000002 | 0x00000004 | 0x00000100

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
		} else {
			// Если nm нет, попробуем взять из других полей
			log.Printf("⚠️ Wialon user без nm: id=%v, item=%+v", item["id"], item)
		}
		// Проверяем флаги пользователя
		if fl, ok := item["fl"].(float64); ok {
			flags := int(fl)
			// Логируем для отладки
			log.Printf("📊 Wialon user %s (ID:%d): fl=%d (binary: %b)", account.Name, account.ID, flags, flags)
			// Флаг 0x00000001 - пользователь отключен
			if flags&0x00000001 != 0 {
				account.IsActive = false
				log.Printf("🚫 Wialon user %s отключён (fl & 0x1 != 0)", account.Name)
			}
			// Флаг 0x0010 (16) - права дилера
			if flags&0x0010 != 0 {
				account.DealerRights = true
			}
		} else {
			log.Printf("⚠️ Wialon user %s (ID:%d): fl отсутствует", account.Name, account.ID)
		}
		// Получаем дату создания (ct - creation time, Unix timestamp)
		if ct, ok := item["ct"].(float64); ok && ct > 0 {
			createdTime := time.Unix(int64(ct), 0)
			account.CreatedAt = createdTime.Format(time.RFC3339)
		}
		// Получаем ID родительской учётной записи (crt - creator, создатель аккаунта)
		// В Wialon иерархия аккаунтов строится через создателя: если A создал B, то B.crt = A.id
		if crt, ok := item["crt"].(float64); ok {
			account.ParentId = int64(crt)
		}
		// Получаем ID ресурса биллинга (bact - billing account)
		// Это ID ресурса (avl_resource), который содержит имя учётной записи для биллинга
		if bact, ok := item["bact"].(float64); ok {
			account.BillingAccountID = int64(bact)
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// SearchResourcesWithHost поиск ресурсов (учётных записей) Wialon с флагом дилера
func (s *WialonService) SearchResourcesWithHost(host string, sessionID string) (map[int64]bool, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Ищем ресурсы с правами дилера
	// sys_account_enable_parent = 1 означает что учётная запись имеет права дилера
	// flags: 0x00000001 (1) + 0x00000004 (4) = 5 для получения bact, crt
	searchParams := `{
		"spec": {
			"itemsType": "avl_resource",
			"propName": "sys_account_enable_parent",
			"propValueMask": "1",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": 5,
		"from": 0,
		"to": 0
	}`

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса ресурсов: %w", err)
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

	// Создаём карту: user_id -> isDealer
	log.Printf("📊 Wialon: получено %d ресурсов", len(searchResp.Items))

	// Логируем первый ресурс для анализа структуры
	if len(searchResp.Items) > 0 {
		firstItem, _ := json.Marshal(searchResp.Items[0])
		log.Printf("📊 Wialon: пример ресурса: %s", string(firstItem))
	}

	// Все найденные ресурсы имеют права дилера (т.к. искали с sys_account_enable_parent=1)
	dealerMap := make(map[int64]bool)
	for _, item := range searchResp.Items {
		var resourceID int64
		var creatorID int64
		var bactID int64
		var resourceName string

		if id, ok := item["id"].(float64); ok {
			resourceID = int64(id)
		}
		if nm, ok := item["nm"].(string); ok {
			resourceName = nm
		}
		if crt, ok := item["crt"].(float64); ok {
			creatorID = int64(crt)
		}
		if bact, ok := item["bact"].(float64); ok {
			bactID = int64(bact)
		}

		log.Printf("🏢 Wialon: ресурс %s (ID:%d, crt:%d, bact:%d) — дилер", resourceName, resourceID, creatorID, bactID)

		// Связываем с ID создателя ресурса
		if creatorID > 0 {
			dealerMap[creatorID] = true
		}
		// Связываем с bact (billing account ID) — это ID ресурса
		if bactID > 0 {
			dealerMap[bactID] = true
		}
		// Связываем с ID ресурса
		dealerMap[resourceID] = true
		// Также добавляем ID-1 (ресурс создаётся вместе с пользователем, их ID отличаются на 1)
		dealerMap[resourceID-1] = true
	}

	log.Printf("📊 Wialon: найдено %d дилеров: %v", len(dealerMap), dealerMap)
	return dealerMap, nil
}

// SearchAllBillingAccountsWithHost возвращает ID всех пользователей с биллинговыми аккаунтами
// (те, у которых есть связанный ресурс avl_resource)
func (s *WialonService) SearchAllBillingAccountsWithHost(host string, sessionID string) (map[int64]bool, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Ищем ВСЕ ресурсы (без фильтра по дилерам)
	// flags: 0x00000004 (4) для получения bact
	searchParams := `{
		"spec": {
			"itemsType": "avl_resource",
			"propName": "sys_name",
			"propValueMask": "*",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": 5,
		"from": 0,
		"to": 0
	}`

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса ресурсов: %w", err)
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

	// Создаём карту: user_id (bact или resourceID-1) -> true
	billingAccounts := make(map[int64]bool)
	for _, item := range searchResp.Items {
		var resourceID int64
		var bactID int64

		if id, ok := item["id"].(float64); ok {
			resourceID = int64(id)
		}
		if bact, ok := item["bact"].(float64); ok {
			bactID = int64(bact)
		}

		// Ресурс связан с пользователем через bact или resourceID-1
		if bactID > 0 {
			billingAccounts[bactID] = true
		}
		// Также добавляем resourceID-1 (ресурс создаётся вместе с пользователем)
		if resourceID > 0 {
			billingAccounts[resourceID-1] = true
		}
	}

	log.Printf("📊 Wialon: найдено %d биллинговых аккаунтов из %d ресурсов", len(billingAccounts), len(searchResp.Items))
	return billingAccounts, nil
}

// GetResourceNamesMapWithHost возвращает карту resource_id -> resource_name (имя учетной записи)
// Используется для отображения имени учетной записи вместо имени создателя
// ВАЖНО: пользователь ссылается на ресурс через поле bact (billing account ID) = resourceID
func (s *WialonService) GetResourceNamesMapWithHost(host string, sessionID string) (map[int64]string, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Ищем ВСЕ ресурсы, получаем id и nm (имя)
	// flags: 0x00000001 (1) - базовая информация (id, nm)
	searchParams := `{
		"spec": {
			"itemsType": "avl_resource",
			"propName": "sys_name",
			"propValueMask": "*",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": 1,
		"from": 0,
		"to": 0
	}`

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса ресурсов: %w", err)
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

	// Создаём карту: resource_id -> resource_name
	// Пользователь ссылается на ресурс через bact (billing account ID) = resourceID
	resourceNamesMap := make(map[int64]string)
	for _, item := range searchResp.Items {
		var resourceID int64
		var resourceName string

		if id, ok := item["id"].(float64); ok {
			resourceID = int64(id)
		}
		if nm, ok := item["nm"].(string); ok {
			resourceName = nm
		}

		// Ресурс связан с пользователем через bact пользователя = resourceID
		if resourceID > 0 && resourceName != "" {
			resourceNamesMap[resourceID] = resourceName
		}
	}

	log.Printf("📊 Wialon: получено %d имён учетных записей из %d ресурсов", len(resourceNamesMap), len(searchResp.Items))
	return resourceNamesMap, nil

}

// GetAccountDisabledStatusWithHost возвращает карту ID -> isDisabled (заблокирован ли аккаунт)
// Использует свойство sys_account_disabled ресурса
func (s *WialonService) GetAccountDisabledStatusWithHost(host string, sessionID string) (map[int64]bool, error) {
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Ищем заблокированные ресурсы (sys_account_disabled = 1)
	searchParams := `{
		"spec": {
			"itemsType": "avl_resource",
			"propName": "sys_account_disabled",
			"propValueMask": "1",
			"sortType": "sys_name"
		},
		"force": 1,
		"flags": 5,
		"from": 0,
		"to": 0
	}`

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sessionID)
	params.Set("params", strings.ReplaceAll(strings.ReplaceAll(searchParams, "\n", ""), "\t", ""))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса заблокированных аккаунтов: %w", err)
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

	// Создаём карту: user_id -> isDisabled
	disabledMap := make(map[int64]bool)
	log.Printf("🚫 Wialon: найдено %d заблокированных ресурсов", len(searchResp.Items))

	for _, item := range searchResp.Items {
		var resourceID int64
		var bactID int64
		var resourceName string

		if id, ok := item["id"].(float64); ok {
			resourceID = int64(id)
		}
		if nm, ok := item["nm"].(string); ok {
			resourceName = nm
		}
		if bact, ok := item["bact"].(float64); ok {
			bactID = int64(bact)
		}

		log.Printf("🚫 Wialon: заблокированный ресурс %s (ID:%d, bact:%d)", resourceName, resourceID, bactID)

		// Связываем с bact (billing account ID)
		if bactID > 0 {
			disabledMap[bactID] = true
		}
		// Также добавляем resourceID-1 (ресурс создаётся вместе с пользователем)
		if resourceID > 0 {
			disabledMap[resourceID-1] = true
			disabledMap[resourceID] = true
		}
	}

	log.Printf("🚫 Wialon: найдено %d заблокированных аккаунтов: %v", len(disabledMap), disabledMap)
	return disabledMap, nil
}

// EnableAccountWithHost включает или отключает учетную запись Wialon
// Использует API: account/enable_account
// itemId — ID ресурса (avl_resource), НЕ пользователя
// enable — true для включения, false для отключения
func (s *WialonService) EnableAccountWithHost(host string, token string, accountResourceID int64, enable bool) error {
	// Авторизуемся
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return fmt.Errorf("ошибка авторизации: %w", err)
	}
	defer s.LogoutWithHost(host, loginResp.Eid)

	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Формируем параметры
	enableValue := 0
	if enable {
		enableValue = 1
	}

	params := map[string]interface{}{
		"itemId": accountResourceID,
		"enable": enableValue,
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("ошибка сериализации параметров: %w", err)
	}

	// Формируем запрос
	queryParams := url.Values{}
	queryParams.Set("svc", "account/enable_account")
	queryParams.Set("sid", loginResp.Eid)
	queryParams.Set("params", string(paramsJSON))

	log.Printf("🔄 Wialon EnableAccount: itemId=%d, enable=%v, host=%s", accountResourceID, enable, host)

	resp, err := s.httpClient.Post(apiURL+"?"+queryParams.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return fmt.Errorf("ошибка запроса: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("📡 Wialon EnableAccount ответ: %s", string(body))

	// Проверяем на ошибку
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		// Пустой ответ {} означает успех
		if string(body) == "{}" || strings.TrimSpace(string(body)) == "{}" {
			log.Printf("✅ Wialon: аккаунт %d успешно %s", accountResourceID, map[bool]string{true: "включён", false: "отключён"}[enable])
			return nil
		}
		return fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Проверяем на код ошибки Wialon
	if errCode, ok := result["error"].(float64); ok && errCode != 0 {
		return fmt.Errorf("ошибка Wialon API (код %d): %s", int(errCode), s.getErrorMessage(int(errCode)))
	}

	log.Printf("✅ Wialon: аккаунт %d успешно %s", accountResourceID, map[bool]string{true: "включён", false: "отключён"}[enable])
	return nil
}

// FindUserByBillingAccountID ищет пользователя по BillingAccountID (bact) или создателю ресурса (crt)
// Если пользователь с bact не найден, ищем ресурс по ID и получаем его создателя
func (s *WialonService) FindUserByBillingAccountID(host string, token string, billingAccountID int64) (string, error) {
	// Убираем trailing slash
	host = strings.TrimSuffix(host, "/")
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Авторизуемся с токеном
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return "", fmt.Errorf("ошибка авторизации: %w", err)
	}
	sid := loginResp.Eid
	defer func() {
		// Выходим из сессии
		logoutParams := url.Values{}
		logoutParams.Set("svc", "core/logout")
		logoutParams.Set("sid", sid)
		logoutParams.Set("params", "{}")
		s.httpClient.Post(apiURL+"?"+logoutParams.Encode(), "application/x-www-form-urlencoded", nil)
	}()

	log.Printf("🔍 Поиск пользователя с bact=%d", billingAccountID)

	// Шаг 1: Ищем пользователя по bact
	userFlags := 0x00000001 | 0x00000100 // базовая + биллинг информация
	userSearchParams := fmt.Sprintf(`{
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
	}`, userFlags)

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", sid)
	params.Set("params", userSearchParams)

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", fmt.Errorf("ошибка запроса пользователей: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResp WialonSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("🔍 Поиск по bact=%d среди %d пользователей", billingAccountID, len(searchResp.Items))

	// Создаём карту пользователей по ID для быстрого поиска
	usersByID := make(map[int64]string)
	for _, item := range searchResp.Items {
		if id, ok := item["id"].(float64); ok {
			if nm, ok := item["nm"].(string); ok {
				usersByID[int64(id)] = nm
			}
		}
		// Проверяем поле bact
		if bact, ok := item["bact"].(float64); ok {
			if int64(bact) == billingAccountID {
				if nm, ok := item["nm"].(string); ok {
					log.Printf("✅ Найден пользователь '%s' с bact=%d", nm, billingAccountID)
					return nm, nil
				}
			}
		}
	}

	// Шаг 2: Если пользователь не найден по bact, ищем ресурс и его создателя
	log.Printf("🔍 Пользователь с bact=%d не найден, ищем ресурс и его создателя", billingAccountID)

	// Используем core/search_item для поиска ресурса по ID
	// Это более надёжный способ получить элемент по ID
	resourceParams := fmt.Sprintf(`{
		"id": %d,
		"flags": 1
	}`, billingAccountID)

	params2 := url.Values{}
	params2.Set("svc", "core/search_item")
	params2.Set("sid", sid)
	params2.Set("params", resourceParams)

	resp2, err := s.httpClient.Post(apiURL+"?"+params2.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", fmt.Errorf("ресурс не найден: %w", err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа ресурса: %w", err)
	}

	log.Printf("🔍 Ответ core/search_item: %s", string(body2)[:min(200, len(body2))])

	// Парсим ответ — это единичный объект, не массив
	var resourceData struct {
		Item map[string]interface{} `json:"item"`
	}
	if err := json.Unmarshal(body2, &resourceData); err != nil {
		// Попробуем парсить как прямой объект
		var directItem map[string]interface{}
		if err2 := json.Unmarshal(body2, &directItem); err2 != nil {
			return "", fmt.Errorf("ошибка парсинга ответа ресурса: %w", err)
		}
		resourceData.Item = directItem
	}

	if resourceData.Item == nil || len(resourceData.Item) == 0 {
		return "", fmt.Errorf("элемент с id=%d не найден", billingAccountID)
	}

	// Проверяем тип найденного элемента (cls: 1=user, 3=resource)
	if cls, ok := resourceData.Item["cls"].(float64); ok {
		log.Printf("🔍 Найден элемент cls=%d (1=user, 3=resource)", int(cls))
		
		// Если это пользователь (cls=1), используем его имя напрямую
		if int(cls) == 1 {
			if nm, ok := resourceData.Item["nm"].(string); ok {
				log.Printf("✅ Найден пользователь '%s' по ID=%d", nm, billingAccountID)
				return nm, nil
			}
		}
	}

	// Если это ресурс — получаем создателя ресурса (поле crt)
	creatorID, ok := resourceData.Item["crt"].(float64)
	if !ok {
		return "", fmt.Errorf("создатель ресурса не указан")
	}

	log.Printf("🔍 Ресурс найден, создатель ID=%d", int64(creatorID))

	// Ищем имя создателя в нашей карте пользователей
	if creatorName, ok := usersByID[int64(creatorID)]; ok {
		log.Printf("✅ Найден создатель ресурса: '%s'", creatorName)
		return creatorName, nil
	}

	return "", fmt.Errorf("создатель ресурса (id=%d) не найден среди пользователей", int64(creatorID))
}

// DuplicateSessionWithHost создает сессию для входа под конкретным пользователем
// Использует token/update для создания временного токена пользователя, затем token/login
func (s *WialonService) DuplicateSessionWithHost(host string, token string, userName string) (string, error) {
	// Убираем trailing slash
	host = strings.TrimSuffix(host, "/")
	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Шаг 1: Авторизуемся с основным токеном
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return "", fmt.Errorf("ошибка авторизации: %w", err)
	}

	currentSid := loginResp.Eid
	currentUser := loginResp.User.Name

	log.Printf("🔄 Wialon: авторизован как '%s'", currentUser)

	// Если пользователь не указан или это основной пользователь, возвращаем auth_hash для него
	if userName == "" || userName == currentUser {
		return s.createAuthHashForSession(apiURL, currentSid)
	}

	// Шаг 2: Переключаемся на пользователя через core/duplicate для получения его userId
	log.Printf("🔄 Wialon: переключаемся на пользователя '%s' через core/duplicate", userName)

	duplicateParams := map[string]interface{}{
		"operateAs":              userName,
		"continueCurrentSession": true,
		"appName":                "Axenta CRM", // Обязательный параметр для core/duplicate
	}
	duplicateParamsJSON, _ := json.Marshal(duplicateParams)

	queryParams := url.Values{}
	queryParams.Set("svc", "core/duplicate")
	queryParams.Set("sid", currentSid)
	queryParams.Set("params", string(duplicateParamsJSON))

	resp, err := s.httpClient.Post(apiURL+"?"+queryParams.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", fmt.Errorf("ошибка core/duplicate: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("📡 Wialon core/duplicate ответ: %s", string(body))

	var duplicateResult map[string]interface{}
	if err := json.Unmarshal(body, &duplicateResult); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if errCode, ok := duplicateResult["error"].(float64); ok && errCode != 0 {
		return "", fmt.Errorf("ошибка Wialon API core/duplicate (код %d): %s", int(errCode), s.getErrorMessage(int(errCode)))
	}

	// Извлекаем информацию о пользователе
	newSid, ok := duplicateResult["eid"].(string)
	if !ok || newSid == "" {
		return "", fmt.Errorf("не удалось получить SID для пользователя %s", userName)
	}

	// Извлекаем userId для создания токена
	var userId int64
	if user, ok := duplicateResult["user"].(map[string]interface{}); ok {
		if id, ok := user["id"].(float64); ok {
			userId = int64(id)
		}
		if nm, ok := user["nm"].(string); ok {
			log.Printf("✅ Wialon: переключились на пользователя '%s' (id=%d)", nm, userId)
		}
	}

	// Шаг 3: Создаем auth_hash для новой сессии
	return s.createAuthHashForSession(apiURL, newSid)
}

// createAuthHashForSession создает auth_hash и затем использует use_auth_hash для получения SID
func (s *WialonService) createAuthHashForSession(apiURL string, sid string) (string, error) {
	// Шаг 1: Создаем auth_hash
	queryParams := url.Values{}
	queryParams.Set("svc", "core/create_auth_hash")
	queryParams.Set("sid", sid)
	queryParams.Set("params", "{}")

	resp, err := s.httpClient.Post(apiURL+"?"+queryParams.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания auth_hash: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("📡 Wialon create_auth_hash ответ: %s", string(body))

	var hashResult map[string]interface{}
	if err := json.Unmarshal(body, &hashResult); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if errCode, ok := hashResult["error"].(float64); ok && errCode != 0 {
		return "", fmt.Errorf("ошибка Wialon API create_auth_hash (код %d): %s", int(errCode), s.getErrorMessage(int(errCode)))
	}

	authHash, ok := hashResult["authHash"].(string)
	if !ok || authHash == "" {
		return "", fmt.Errorf("не удалось получить authHash")
	}

	log.Printf("✅ Wialon: auth_hash создан: %s", authHash)

	// Шаг 2: Используем use_auth_hash для получения нового SID
	useParams := map[string]interface{}{
		"authHash": authHash,
	}
	useParamsJSON, _ := json.Marshal(useParams)

	queryParams = url.Values{}
	queryParams.Set("svc", "core/use_auth_hash")
	queryParams.Set("params", string(useParamsJSON))

	resp2, err := s.httpClient.Post(apiURL+"?"+queryParams.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return "", fmt.Errorf("ошибка use_auth_hash: %w", err)
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	log.Printf("📡 Wialon use_auth_hash ответ: %s", string(body2))

	var useResult map[string]interface{}
	if err := json.Unmarshal(body2, &useResult); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if errCode, ok := useResult["error"].(float64); ok && errCode != 0 {
		return "", fmt.Errorf("ошибка Wialon API use_auth_hash (код %d): %s", int(errCode), s.getErrorMessage(int(errCode)))
	}

	// Извлекаем новый SID
	newSid, ok := useResult["eid"].(string)
	if !ok || newSid == "" {
		return "", fmt.Errorf("не удалось получить SID из use_auth_hash")
	}

	// Проверяем под каким пользователем вошли
	if au, ok := useResult["au"].(string); ok {
		log.Printf("✅ Wialon use_auth_hash: вошли как '%s', sid=%s", au, newSid)
	}

	return newSid, nil
}

// GetMonitoringURL формирует URL для входа в мониторинг Wialon с переданным SID
func (s *WialonService) GetMonitoringURL(host string, sid string) string {
	// Убираем hst-api и используем hosting для UI (только для WH)
	hostingURL := strings.Replace(host, "hst-api.", "hosting.", 1)

	// Формируем URL с SID для авторизации
	return fmt.Sprintf("%s/?sid=%s&lang=ru", hostingURL, sid)
}

// GetTotalUnitsCount получает общее количество объектов без лимита
// Использует totalItemsCount из ответа API вместо загрузки всех объектов
func (s *WialonService) GetTotalUnitsCount(host string, token string) (int, error) {
	// Авторизуемся
	loginResp, err := s.LoginWithHost(host, token)
	if err != nil {
		return 0, err
	}
	defer s.LogoutWithHost(host, loginResp.Eid)

	apiURL := fmt.Sprintf("%s/wialon/ajax.html", host)

	// Минимальные флаги для быстрого запроса
	searchParams := map[string]interface{}{
		"spec": map[string]interface{}{
			"itemsType":     "avl_unit",
			"propName":      "sys_name",
			"propValueMask": "*",
			"sortType":      "sys_name",
		},
		"force": 1,
		"flags": 0x00000001, // Только базовые свойства
		"from":  0,
		"to":    1, // Запрашиваем минимум данных - нам нужен только totalItemsCount
	}

	paramsJSON, _ := json.Marshal(searchParams)

	params := url.Values{}
	params.Set("svc", "core/search_items")
	params.Set("sid", loginResp.Eid)
	params.Set("params", string(paramsJSON))

	resp, err := s.httpClient.Post(apiURL+"?"+params.Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		return 0, fmt.Errorf("ошибка запроса объектов: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var searchResult struct {
		TotalItemsCount int `json:"totalItemsCount"`
	}
	if err := json.Unmarshal(body, &searchResult); err != nil {
		return 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	log.Printf("📊 Wialon GetTotalUnitsCount: totalItemsCount = %d", searchResult.TotalItemsCount)
	return searchResult.TotalItemsCount, nil
}
