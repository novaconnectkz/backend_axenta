package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"backend_axenta/config"

	"github.com/gin-gonic/gin"
)

const (
	daDataBaseURL  = "https://suggestions.dadata.ru/suggestions/api/4_1"
	requestTimeout = 10 * time.Second
)

// DaDataOrganizationRequest запрос для поиска организации
type DaDataOrganizationRequest struct {
	Query      string `json:"query" binding:"required"`
	Count      int    `json:"count,omitempty"`
	BranchType string `json:"branch_type,omitempty"`
	Type       string `json:"type,omitempty"`
}

// DaDataOrganizationResponse ответ от DaData API
type DaDataOrganizationResponse struct {
	Suggestions []DaDataSuggestion `json:"suggestions"`
}

// DaDataSuggestion предложение от DaData
type DaDataSuggestion struct {
	Value string             `json:"value"`
	Data  DaDataOrganization `json:"data"`
}

// DaDataOrganization данные организации
type DaDataOrganization struct {
	INN         string            `json:"inn"`
	KPP         string            `json:"kpp"`
	OGRN        string            `json:"ogrn"`
	Name        *DaDataName       `json:"name,omitempty"` // Объект с полями full_with_opf, short_with_opf, full, short
	Management  *DaDataManagement `json:"management,omitempty"`
	Address     *DaDataAddress    `json:"address,omitempty"`
	Phones      []*DaDataPhone    `json:"phones,omitempty"` // Массив телефонов (может быть null)
	Emails      []*DaDataEmail    `json:"emails,omitempty"` // Массив email (может быть null)
	Type        string            `json:"type,omitempty"`
	BranchType  string            `json:"branch_type,omitempty"`
	State       *DaDataState      `json:"state,omitempty"`
	// Игнорируем остальные поля
}

// DaDataPhone телефон организации
type DaDataPhone struct {
	Value string `json:"value,omitempty"`
}

// DaDataEmail email организации
type DaDataEmail struct {
	Value string `json:"value,omitempty"`
}

// DaDataName наименование организации
type DaDataName struct {
	FullWithOPF  string `json:"full_with_opf"`
	ShortWithOPF string `json:"short_with_opf"`
	Full         string `json:"full"`
	Short        string `json:"short"`
	Latin        string `json:"latin,omitempty"`
}

// DaDataManagement данные руководителя
type DaDataManagement struct {
	Name string `json:"name"`
	Post string `json:"post"`
}

// DaDataAddress адрес организации
type DaDataAddress struct {
	Value             string `json:"value"`
	UnrestrictedValue string `json:"unrestricted_value"`
}

// DaDataState статус организации
type DaDataState struct {
	Status           string  `json:"status"`
	ActualityDate    *int64  `json:"actuality_date,omitempty"`    // Timestamp в миллисекундах
	RegistrationDate *int64  `json:"registration_date,omitempty"` // Timestamp в миллисекундах
	LiquidationDate  *int64  `json:"liquidation_date,omitempty"` // Timestamp в миллисекундах
}

// FindOrganizationByINN находит организацию по ИНН или ОГРН
// POST /api/dadata/organization
func FindOrganizationByINN(c *gin.Context) {
	// Проверяем наличие API ключа
	cfg := config.GetConfig()
	if cfg.External.DaDataAPIKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "DaData API ключ не настроен на сервере",
		})
		return
	}

	// Парсим запрос
	var req DaDataOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Неверный формат запроса: %v", err),
		})
		return
	}

	// Валидация запроса
	cleanQuery := strings.TrimSpace(strings.ReplaceAll(req.Query, " ", ""))
	if cleanQuery == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "ИНН или ОГРН не может быть пустым",
		})
		return
	}

	// Проверяем формат: ИНН должен быть 10 или 12 цифр, ОГРН - 13 цифр
	if !regexp.MustCompile(`^\d{10}$|^\d{12}$|^\d{13}$`).MatchString(cleanQuery) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "ИНН должен содержать 10 или 12 цифр, ОГРН - 13 цифр",
		})
		return
	}

	// Подготавливаем запрос к DaData
	requestBody := map[string]interface{}{
		"query": cleanQuery,
	}

	// Добавляем branch_type: "MAIN" для получения головной организации
	if req.BranchType != "" {
		requestBody["branch_type"] = req.BranchType
	} else {
		requestBody["branch_type"] = "MAIN"
	}

	if req.Type != "" {
		requestBody["type"] = req.Type
	}

	if req.Count > 0 {
		requestBody["count"] = req.Count
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("Error marshaling request body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при подготовке запроса",
		})
		return
	}

	// Создаем HTTP клиент с таймаутом
	client := &http.Client{
		Timeout: requestTimeout,
	}

	// Формируем URL
	url := fmt.Sprintf("%s/rs/findById/party", daDataBaseURL)

	// Создаем запрос
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при создании запроса",
		})
		return
	}

	// Устанавливаем заголовки
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Token %s", cfg.External.DaDataAPIKey))

	// Выполняем запрос
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Error executing DaData request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при запросе к DaData API",
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при чтении ответа от DaData",
		})
		return
	}

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		log.Printf("DaData API returned status %d: %s", resp.StatusCode, string(body))

		// Обрабатываем специфичные ошибки
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "Неверный API ключ DaData. Пожалуйста, проверьте настройки сервера",
			})
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":  "error",
				"message": "Превышен лимит запросов к DaData API. Попробуйте позже",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Ошибка DaData API: статус %d", resp.StatusCode),
		})
		return
	}

	// Парсим ответ
	var daDataResp DaDataOrganizationResponse
	if err := json.Unmarshal(body, &daDataResp); err != nil {
		bodyStr := string(body)
		bodyPreview := bodyStr
		if len(bodyStr) > 1000 {
			bodyPreview = bodyStr[:1000] + "..."
		}
		log.Printf("Error unmarshaling response: %v", err)
		log.Printf("Response body (first 1000 chars): %s", bodyPreview)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Ошибка при парсинге ответа от DaData: %v", err),
		})
		return
	}

	// Проверяем наличие результатов
	if len(daDataResp.Suggestions) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"data":    nil,
			"message": "Организация не найдена",
		})
		return
	}

	// Возвращаем первый результат (уже будет головная организация, т.к. указали branch_type: "MAIN")
	organization := daDataResp.Suggestions[0]

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   organization,
	})
}

// DaDataBankRequest запрос для поиска банка
type DaDataBankRequest struct {
	Query string `json:"query" binding:"required"`
}

// DaDataBankResponse ответ от DaData API для банка
type DaDataBankResponse struct {
	Suggestions []DaDataBankSuggestion `json:"suggestions"`
}

// DaDataBankSuggestion предложение от DaData для банка
type DaDataBankSuggestion struct {
	Value string         `json:"value"`
	Data  DaDataBankData `json:"data"`
}

// DaDataBankName структура для имени банка
type DaDataBankName struct {
	Payment string `json:"payment"`
	Full    string `json:"full,omitempty"`
	Short   string `json:"short,omitempty"`
}

// DaDataBankData данные банка
type DaDataBankData struct {
	Bik                  string          `json:"bic"`
	Name                 DaDataBankName  `json:"name"`
	CorrespondentAccount string          `json:"correspondent_account,omitempty"`
	Okpo                 string          `json:"okpo,omitempty"`
	RegNumber            string          `json:"registration_number,omitempty"`
	Swift                string          `json:"swift,omitempty"`
	Inn                  string          `json:"inn,omitempty"`
	Kpp                  string          `json:"kpp,omitempty"`
}

// FindBankByBIK находит банк по БИК
// POST /api/auth/dadata/bank
func FindBankByBIK(c *gin.Context) {
	// Проверяем наличие API ключа
	cfg := config.GetConfig()
	if cfg.External.DaDataAPIKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "DaData API ключ не настроен на сервере",
		})
		return
	}

	// Парсим запрос
	var req DaDataBankRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Неверный формат запроса: %v", err),
		})
		return
	}

	// Валидация запроса
	cleanQuery := strings.TrimSpace(strings.ReplaceAll(req.Query, " ", ""))
	if cleanQuery == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "БИК не может быть пустым",
		})
		return
	}

	// Проверяем формат: БИК должен быть 9 цифр
	if !regexp.MustCompile(`^\d{9}$`).MatchString(cleanQuery) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "БИК должен содержать 9 цифр",
		})
		return
	}

	// Подготавливаем запрос к DaData
	requestBody := map[string]interface{}{
		"query": cleanQuery,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Printf("Error marshaling request body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при подготовке запроса",
		})
		return
	}

	// Создаем HTTP клиент с таймаутом
	client := &http.Client{
		Timeout: requestTimeout,
	}

	// Формируем URL для поиска банка по БИК
	url := fmt.Sprintf("%s/rs/findById/bank", daDataBaseURL)

	// Создаем запрос
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Printf("Error creating request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при создании запроса",
		})
		return
	}

	// Устанавливаем заголовки
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Token %s", cfg.External.DaDataAPIKey))

	// Выполняем запрос
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Error executing DaData request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при запросе к DaData API",
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Ошибка при чтении ответа от DaData",
		})
		return
	}

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		log.Printf("DaData API returned status %d: %s", resp.StatusCode, string(body))

		// Обрабатываем специфичные ошибки
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "Неверный API ключ DaData. Пожалуйста, проверьте настройки сервера",
			})
			return
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"status":  "error",
				"message": "Превышен лимит запросов к DaData API. Попробуйте позже",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Ошибка DaData API: статус %d", resp.StatusCode),
		})
		return
	}

	// Парсим ответ
	var daDataResp DaDataBankResponse
	if err := json.Unmarshal(body, &daDataResp); err != nil {
		bodyStr := string(body)
		bodyPreview := bodyStr
		if len(bodyStr) > 1000 {
			bodyPreview = bodyStr[:1000] + "..."
		}
		log.Printf("Error unmarshaling response: %v", err)
		log.Printf("Response body (first 1000 chars): %s", bodyPreview)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Ошибка при парсинге ответа от DaData: %v", err),
		})
		return
	}

	// Проверяем наличие результатов
	if len(daDataResp.Suggestions) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"data":    nil,
			"message": "Банк не найден",
		})
		return
	}

	// Возвращаем первый результат
	bank := daDataResp.Suggestions[0]

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   bank,
	})
}
