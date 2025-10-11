package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// AccountsHandler обрабатывает запросы к API учетных записей Axenta
type AccountsHandler struct{}

// NewAccountsHandler создает новый обработчик учетных записей
func NewAccountsHandler() *AccountsHandler {
	return &AccountsHandler{}
}

// Account представляет структуру учетной записи из Axenta API
type Account struct {
	ID                 int     `json:"id"`
	Name               string  `json:"name"`
	Type               string  `json:"type"`
	AdminFullname      string  `json:"adminFullname"`
	AdminID            int     `json:"adminId"`
	AdminIsActive      bool    `json:"adminIsActive"`
	ParentAccountName  string  `json:"parentAccountName"`
	ObjectsActive      int     `json:"objectsActive"`
	ObjectsTotal       int     `json:"objectsTotal"`
	ObjectsDeleted     int     `json:"objectsDeleted"`
	Comment            *string `json:"comment"`
	IsActive           bool    `json:"isActive"`
	BlockingDatetime   *string `json:"blockingDatetime"`
	Hierarchy          string  `json:"hierarchy"`
	DaysBeforeBlocking *int    `json:"daysBeforeBlocking"`
	CreationDatetime   string  `json:"creationDatetime"`
}

// AccountsResponse представляет ответ API со списком учетных записей
type AccountsResponse struct {
	Count    int       `json:"count"`
	Next     *string   `json:"next"`
	Previous *string   `json:"previous"`
	Results  []Account `json:"results"`
}

// GetAccounts получает список учетных записей через прокси к Axenta API
func (h *AccountsHandler) GetAccounts(c *gin.Context) {
	// Получаем токен из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = c.GetHeader("authorization")
	}

	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	ordering := c.DefaultQuery("ordering", "name")
	search := c.Query("search")
	accountType := c.Query("type")
	isActive := c.Query("is_active")

	// Строим URL для Axenta API
	axentaURL := "https://axenta.cloud/api/cms/accounts/"

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request: " + err.Error(),
		})
		return
	}

	// Добавляем параметры запроса
	q := req.URL.Query()
	q.Add("page", page)
	q.Add("per_page", perPage)
	q.Add("ordering", ordering)

	if search != "" {
		q.Add("search", search)
	}
	if accountType != "" {
		q.Add("type", accountType)
	}
	if isActive != "" {
		q.Add("is_active", isActive)
	}

	req.URL.RawQuery = q.Encode()

	// Добавляем заголовки авторизации
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Добавляем X-Tenant-ID если есть
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	// Логируем запрос
	fmt.Printf("🔄 Proxy request to Axenta API: %s\n", req.URL.String())
	fmt.Printf("📋 Headers: Authorization=%s, X-Tenant-ID=%s\n",
		authHeader[:min(len(authHeader), 20)]+"...", tenantID)

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Axenta API request failed: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to connect to Axenta API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read Axenta API response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read Axenta API response: " + err.Error(),
		})
		return
	}

	// Логируем ответ
	fmt.Printf("✅ Axenta API response: status=%d, size=%d bytes\n", resp.StatusCode, len(body))

	// Если статус не 200, возвращаем ошибку
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Axenta API error: status=%d, body=%s\n", resp.StatusCode, string(body))

		// Пытаемся распарсить ошибку
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			c.JSON(resp.StatusCode, gin.H{
				"status":  "error",
				"error":   "Axenta API error",
				"details": errorResponse,
			})
		} else {
			c.JSON(resp.StatusCode, gin.H{
				"status": "error",
				"error":  "Axenta API error: " + string(body),
			})
		}
		return
	}

	// Парсим успешный ответ
	var accountsResponse AccountsResponse
	if err := json.Unmarshal(body, &accountsResponse); err != nil {
		fmt.Printf("❌ Failed to parse Axenta API response: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta API response: " + err.Error(),
		})
		return
	}

	fmt.Printf("✅ Successfully proxied accounts: count=%d, results=%d\n",
		accountsResponse.Count, len(accountsResponse.Results))

	// Возвращаем данные
	c.JSON(http.StatusOK, accountsResponse)
}

// GetAccount получает конкретную учетную запись по ID
func (h *AccountsHandler) GetAccount(c *gin.Context) {
	// Получаем ID из параметров
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid account ID",
		})
		return
	}

	// Получаем токен из заголовка
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		authHeader = c.GetHeader("authorization")
	}

	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Authorization header is required",
		})
		return
	}

	// Строим URL для Axenta API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/accounts/%d/", id)

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to create request: " + err.Error(),
		})
		return
	}

	// Добавляем заголовки авторизации
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Добавляем X-Tenant-ID если есть
	tenantID := c.GetHeader("X-Tenant-ID")
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to connect to Axenta API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read Axenta API response: " + err.Error(),
		})
		return
	}

	// Если статус не 200, возвращаем ошибку
	if resp.StatusCode != http.StatusOK {
		var errorResponse map[string]interface{}
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			c.JSON(resp.StatusCode, gin.H{
				"status":  "error",
				"error":   "Axenta API error",
				"details": errorResponse,
			})
		} else {
			c.JSON(resp.StatusCode, gin.H{
				"status": "error",
				"error":  "Axenta API error: " + string(body),
			})
		}
		return
	}

	// Парсим успешный ответ
	var account Account
	if err := json.Unmarshal(body, &account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to parse Axenta API response: " + err.Error(),
		})
		return
	}

	// Возвращаем данные
	c.JSON(http.StatusOK, account)
}

// min возвращает минимальное из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
