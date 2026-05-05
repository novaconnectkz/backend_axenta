package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AxentaUserFromAPI представляет пользователя из Axenta API
type AxentaUserFromAPI struct {
	ID                      int     `json:"id"`
	Username                string  `json:"username"`
	Name                    string  `json:"name"`
	Email                   string  `json:"email"`
	AccountName             string  `json:"accountName"`
	AccountType             string  `json:"accountType"`
	CreatorName             string  `json:"creatorName"`
	LastLogin               string  `json:"lastLogin"`
	AccountBlockingDatetime *string `json:"accountBlockingDatetime"`
	AccountID               int     `json:"accountId"`
	IsAdmin                 bool    `json:"isAdmin"`
	IsActive                bool    `json:"isActive"`
	Language                string  `json:"language"`
	Timezone                int     `json:"timezone"`
}

// AxentaUsersAPIResponse представляет ответ от Axenta API со списком пользователей
type AxentaUsersAPIResponse struct {
	Count    int                 `json:"count"`
	Next     *string             `json:"next"`
	Previous *string             `json:"previous"`
	Results  []AxentaUserFromAPI `json:"results"`
}

// SyncAllAxentaUsers синхронизирует всех пользователей из Axenta с локальной базой данных
func SyncAllAxentaUsers(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Получаем токен пользователя
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Токен авторизации не предоставлен",
		})
		return
	}

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

	// Создаем сервис для работы с пользователями Axenta
	axentaUserService := services.NewAxentaUserService(db)

	// Убеждаемся, что роли по умолчанию существуют
	if err := axentaUserService.EnsureDefaultRoles(); err != nil {
		log.Printf("Failed to ensure default roles: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to ensure default roles",
		})
		return
	}

	// Загружаем всех пользователей из Axenta
	axentaUsers, err := getAllAxentaUsers(userToken)
	if err != nil {
		log.Printf("Failed to get users from Axenta: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get users from Axenta: " + err.Error(),
		})
		return
	}

	log.Printf("Loaded %d users from Axenta API", len(axentaUsers))

	// Синхронизируем каждого пользователя
	var syncedCount, errorCount int
	var syncErrors []string

	for _, axentaUser := range axentaUsers {
		err := syncSingleAxentaUser(axentaUserService, &axentaUser)
		if err != nil {
			errorCount++
			syncErrors = append(syncErrors, fmt.Sprintf("User %s: %v", axentaUser.Username, err))
			log.Printf("Failed to sync user %s: %v", axentaUser.Username, err)
		} else {
			syncedCount++
			log.Printf("Successfully synced user: %s (type: %s)", axentaUser.Username, axentaUser.AccountType)
		}
	}

	// Возвращаем результат синхронизации
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Синхронизация завершена: %d успешно, %d ошибок", syncedCount, errorCount),
		"data": gin.H{
			"total_users":  len(axentaUsers),
			"synced_count": syncedCount,
			"error_count":  errorCount,
			"errors":       syncErrors,
		},
	})
}

// getAllAxentaUsers загружает всех пользователей из Axenta API (с пагинацией)
func getAllAxentaUsers(token string) ([]AxentaUserFromAPI, error) {
	var allUsers []AxentaUserFromAPI
	page := 1
	perPage := 100

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for {
		url := fmt.Sprintf("https://axenta.cloud/api/cms/users/?page=%d&per_page=%d", page, perPage)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Token "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("axenta API returned status %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var response AxentaUsersAPIResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		allUsers = append(allUsers, response.Results...)

		// Если есть следующая страница, продолжаем
		if response.Next == nil {
			break
		}
		page++
	}

	return allUsers, nil
}

// syncSingleAxentaUser синхронизирует одного пользователя из Axenta с локальной базой
func syncSingleAxentaUser(service *services.AxentaUserService, axentaUser *AxentaUserFromAPI) error {
	// Преобразуем данные из API в формат AxentaUserResponse
	axentaUserData := &services.AxentaUserResponse{
		ID:                      axentaUser.ID,
		Username:                axentaUser.Username,
		Name:                    axentaUser.Name,
		Email:                   axentaUser.Email,
		AccountName:             axentaUser.AccountName,
		AccountType:             axentaUser.AccountType,
		CreatorName:             axentaUser.CreatorName,
		LastLogin:               axentaUser.LastLogin,
		AccountBlockingDatetime: axentaUser.AccountBlockingDatetime,
		AccountID:               axentaUser.AccountID,
		IsAdmin:                 axentaUser.IsAdmin,
		IsActive:                axentaUser.IsActive,
		Language:                axentaUser.Language,
		Timezone:                axentaUser.Timezone,
	}

	// Синхронизируем пользователя
	_, err := service.SyncUserFromAxentaData(axentaUserData)
	return err
}

// GetSyncedUsersFromLocal возвращает пользователей из локальной базы данных (уже синхронизированных)
func GetSyncedUsersFromLocal(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Database connection not available",
		})
		return
	}

	// Параметры пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Параметры фильтрации
	axentaType := c.Query("axenta_type")
	isAxentaUser := c.Query("is_axenta_user")
	search := c.Query("search")

	// Построение запроса - загружаем только пользователей с назначенными ролями
	query := db.Model(&models.User{}).Preload("Role").Preload("Template").Where("role_id > 0")

	// Фильтр по типу Axenta
	if axentaType != "" {
		if axentaType == "local" {
			query = query.Where("is_axenta_user = ? OR axenta_user_type = ?", false, "local")
		} else {
			query = query.Where("axenta_user_type = ?", axentaType)
		}
	}

	// Фильтр по источнику (Axenta или локальный)
	if isAxentaUser != "" {
		isAxenta := isAxentaUser == "true"
		query = query.Where("is_axenta_user = ?", isAxenta)
	}

	// Поиск
	if search != "" {
		searchPattern := "%" + strings.ToLower(search) + "%"
		query = query.Where(
			"LOWER(username) LIKE ? OR LOWER(email) LIKE ? OR LOWER(first_name) LIKE ? OR LOWER(last_name) LIKE ?",
			searchPattern, searchPattern, searchPattern, searchPattern,
		)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to count users: " + err.Error(),
		})
		return
	}

	// Получение пользователей с пагинацией
	var users []models.User
	if err := query.Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to get users: " + err.Error(),
		})
		return
	}

	// Формируем ответ
	pages := int((total + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": users,
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": pages,
		},
	})
}
