package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// AxentaCloudObject представляет объект из Axenta Cloud API
type AxentaCloudObject struct {
	ID                  int      `json:"id"`
	Name                string   `json:"name"`
	UniqueID            string   `json:"uniqueId"`
	CreatorName         string   `json:"creatorName"`
	CreatorID           int      `json:"creatorId"`
	CreatorIsActive     bool     `json:"creatorIsActive"`
	AccountID           int      `json:"accountId"`
	AccountName         string   `json:"accountName"`
	AccountType         string   `json:"accountType"`
	AccountIsActive     bool     `json:"accountIsActive"`
	PhoneNumbers        []string `json:"phoneNumbers"`
	DeviceTypeName      string   `json:"deviceTypeName"`
	LastMessageDatetime string   `json:"lastMessageDatetime"`
	CreatedAt           string   `json:"createdAt"`
	IsActive            bool     `json:"isActive"`
	CurrentUserAccess   []string `json:"currentUserAccess"`
}

// AxentaCloudResponse представляет ответ от Axenta Cloud API
type AxentaCloudResponse struct {
	Count    int                 `json:"count"`
	Next     *string             `json:"next"`
	Previous *string             `json:"previous"`
	Results  []AxentaCloudObject `json:"results"`
}

// GetObjectsFromAxentaCloud проксирует запрос к Axenta Cloud API
func GetObjectsFromAxentaCloud(c *gin.Context) {
	// Получаем параметры запроса
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")
	ordering := c.DefaultQuery("ordering", "name")

	// Формируем URL для Axenta Cloud API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/objects/?page=%s&per_page=%s&ordering=%s", page, perPage, ordering)

	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Создаем запрос к Axenta Cloud
	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Добавляем заголовки авторизации с токеном пользователя
	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Парсим ответ от Axenta Cloud
	var axentaResponse AxentaCloudResponse
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Конвертируем в стандартную структуру ответа
	pageNum, _ := strconv.Atoi(page)
	perPageNum, _ := strconv.Atoi(perPage)
	totalPages := (axentaResponse.Count + perPageNum - 1) / perPageNum

	// Конвертируем объекты Axenta Cloud в локальную структуру
	var localObjects []gin.H
	for _, obj := range axentaResponse.Results {
		localObject := gin.H{
			"id":                  obj.ID,
			"name":                obj.Name,
			"type":                "vehicle", // По умолчанию
			"description":         fmt.Sprintf("%s - %s", obj.DeviceTypeName, obj.AccountName),
			"created_at":          obj.CreatedAt,
			"updated_at":          obj.CreatedAt,
			"accountName":         obj.AccountName,
			"creatorName":         obj.CreatorName,
			"deviceTypeName":      obj.DeviceTypeName,
			"phoneNumbers":        obj.PhoneNumbers,
			"createdAt":           obj.CreatedAt,
			"lastMessageDatetime": obj.LastMessageDatetime,
			"uniqueId":            obj.UniqueID,
			"status":              map[bool]string{true: "active", false: "inactive"}[obj.IsActive],
			"is_active":           obj.IsActive,
			"address":             obj.AccountName,
			"imei":                obj.UniqueID,
			"phone_number":        "",
			"serial_number":       obj.UniqueID,
			"company_id":          obj.AccountID,
			"contract_id":         obj.AccountID,
			"location_id":         obj.AccountID,
			"settings":            "{}",
			"tags":                []string{obj.DeviceTypeName, obj.AccountType},
			"notes":               fmt.Sprintf("Создатель: %s", obj.CreatorName),
			"external_id":         obj.UniqueID,
		}

		if len(obj.PhoneNumbers) > 0 {
			localObject["phone_number"] = obj.PhoneNumbers[0]
		}

		localObjects = append(localObjects, localObject)
	}

	// Возвращаем в стандартном формате
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items":       localObjects,
			"total":       axentaResponse.Count,
			"page":        pageNum,
			"per_page":    perPageNum,
			"total_pages": totalPages,
		},
	})
}

// GetObjectsStatsFromAxentaCloud получает статистику объектов
func GetObjectsStatsFromAxentaCloud(c *gin.Context) {
	// Получаем токен пользователя из заголовка Authorization
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

	// Извлекаем токен из заголовка "Token <token>" или "Bearer <token>"
	var userToken string
	if strings.HasPrefix(authHeader, "Token ") {
		userToken = strings.TrimPrefix(authHeader, "Token ")
	} else if strings.HasPrefix(authHeader, "Bearer ") {
		userToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Неверный формат токена авторизации",
		})
		return
	}

	// Для статистики можно сделать запрос к объектам и посчитать
	axentaURL := "https://axenta.cloud/api/cms/objects/?page=1&per_page=1"

	req, err := http.NewRequest("GET", axentaURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}

	req.Header.Set("Authorization", "Token "+userToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка запроса к Axenta Cloud: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка чтения ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	var axentaResponse AxentaCloudResponse
	if err := json.Unmarshal(body, &axentaResponse); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка парсинга ответа от Axenta Cloud: " + err.Error(),
		})
		return
	}

	// Возвращаем статистику
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"total":                axentaResponse.Count,
			"active":               axentaResponse.Count, // Предполагаем, что большинство активны
			"inactive":             0,
			"scheduled_for_delete": 0,
			"by_type": gin.H{
				"vehicle": axentaResponse.Count,
			},
			"by_status": gin.H{
				"active": axentaResponse.Count,
			},
		},
	})
}
