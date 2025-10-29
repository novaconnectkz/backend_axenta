package api

import (
	"backend_axenta/database"
	"backend_axenta/models"
	"backend_axenta/services"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// CreateUserRequest представляет запрос на создание пользователя
type CreateUserRequest struct {
	Username   string `json:"username" binding:"required,min=3,max=50"`
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=6,max=100"`
	FirstName  string `json:"first_name" binding:"max=50"`
	LastName   string `json:"last_name" binding:"max=50"`
	Name       string `json:"name" binding:"max=200"`       // Полное имя для контрагентов
	Phone      string `json:"phone" binding:"max=50"`       // Телефон
	TelegramID string `json:"telegram_id" binding:"max=50"` // Telegram ID
	UserType   string `json:"user_type" binding:"max=50"`   // Тип пользователя
	RoleID     uint   `json:"role_id" binding:"required,min=1"`
	TemplateID *uint  `json:"template_id"`
	IsActive   *bool  `json:"is_active"`
}

// UpdateUserRequest представляет запрос на обновление пользователя
type UpdateUserRequest struct {
	Username   string `json:"username" binding:"omitempty,min=3,max=50"`
	Email      string `json:"email" binding:"omitempty,email"`
	FirstName  string `json:"first_name" binding:"max=50"`
	LastName   string `json:"last_name" binding:"max=50"`
	Name       string `json:"name" binding:"max=200"`       // Полное имя для контрагентов
	Phone      string `json:"phone" binding:"max=50"`       // Телефон
	TelegramID string `json:"telegram_id" binding:"max=50"` // Telegram ID
	UserType   string `json:"user_type" binding:"max=50"`   // Тип пользователя
	RoleID     *uint  `json:"role_id" binding:"omitempty,min=1"`
	TemplateID *uint  `json:"template_id"`
	IsActive   *bool  `json:"is_active"`
}

// UserResponse представляет ответ с данными пользователя
type UserResponse struct {
	ID               uint                 `json:"id"`
	Username         string               `json:"username"`
	Email            string               `json:"email"`
	FirstName        string               `json:"first_name"`
	LastName         string               `json:"last_name"`
	Name             string               `json:"name"`              // Полное имя
	Phone            string               `json:"phone"`             // Телефон
	TelegramID       string               `json:"telegram_id"`       // Telegram ID
	UserType         string               `json:"user_type"`         // Тип пользователя
	CreatorName      string               `json:"creator_name"`      // Имя создателя
	LastLogin        *string              `json:"lastLogin"`         // Последний вход
	CreationDatetime string               `json:"creation_datetime"` // Дата создания пользователя
	IsActive         bool                 `json:"is_active"`
	RoleID           *uint                `json:"role_id"`
	Role             *models.Role         `json:"role,omitempty"`
	TemplateID       *uint                `json:"template_id"`
	Template         *models.UserTemplate `json:"template,omitempty"`
	LoginCount       int                  `json:"login_count"`
	CreatedAt        string               `json:"created_at"`
	UpdatedAt        string               `json:"updated_at"`
}

// GetUsers возвращает список пользователей с фильтрацией и пагинацией
func GetUsers(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	// Параметры пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	// Убираем ограничение на лимит для возможности вывода всех записей
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Параметры фильтрации
	role := c.Query("role")
	active := c.Query("active")
	search := c.Query("search")
	ordering := c.Query("ordering")

	// Построение запроса
	query := db.Model(&models.User{}).Preload("Role").Preload("Template")

	// Фильтр по роли
	if role != "" {
		query = query.Joins("JOIN roles ON users.role_id = roles.id").
			Where("roles.name = ?", role)
	}

	// Фильтр по активности
	if active != "" {
		isActive := active == "true"
		query = query.Where("is_active = ?", isActive)
	}

	// Поиск по имени, email или username
	if search != "" {
		// Проверяем, есть ли запятые для множественного поиска
		if strings.Contains(search, ",") {
			// Множественный поиск по точному совпадению
			searchTerms := strings.Split(search, ",")
			var trimmedTerms []string
			for _, term := range searchTerms {
				trimmed := strings.TrimSpace(term)
				if trimmed != "" {
					trimmedTerms = append(trimmedTerms, trimmed)
				}
			}
			if len(trimmedTerms) > 0 {
				// Поиск по точному совпадению username, email или полному имени
				var conditions []string
				var args []interface{}

				for _, term := range trimmedTerms {
					lowerTerm := strings.ToLower(term)
					// Используем два варианта поиска: с LOWER для латиницы и без LOWER для кириллицы
					conditions = append(conditions,
						"(LOWER(username) = ? OR username = ?) OR "+
							"(LOWER(email) = ? OR email = ?) OR "+
							"(LOWER(first_name) = ? OR first_name = ?) OR "+
							"(LOWER(last_name) = ? OR last_name = ?) OR "+
							"((LOWER(first_name) || ' ' || LOWER(last_name)) = ? OR (first_name || ' ' || last_name) = ?)")
					args = append(args, lowerTerm, term, lowerTerm, term, lowerTerm, term, lowerTerm, term, lowerTerm, term)
				}

				query = query.Where(strings.Join(conditions, " OR "), args...)
			}
		} else {
			// Обычный поиск по частичному совпадению
			// Используем два варианта поиска: с LOWER для латиницы и без LOWER для кириллицы
			searchPatternLower := "%" + strings.ToLower(search) + "%"
			searchPatternOriginal := "%" + search + "%"

			query = query.Where(
				"(LOWER(username) LIKE ? OR username LIKE ?) OR "+
					"(LOWER(email) LIKE ? OR email LIKE ?) OR "+
					"(LOWER(first_name) LIKE ? OR first_name LIKE ?) OR "+
					"(LOWER(last_name) LIKE ? OR last_name LIKE ?) OR "+
					"((LOWER(first_name) || ' ' || LOWER(last_name)) LIKE ? OR (first_name || ' ' || last_name) LIKE ?)",
				searchPatternLower, searchPatternOriginal,
				searchPatternLower, searchPatternOriginal,
				searchPatternLower, searchPatternOriginal,
				searchPatternLower, searchPatternOriginal,
				searchPatternLower, searchPatternOriginal,
			)
		}
	}

	// Сортировка
	if ordering != "" {
		// Поддерживаемые поля для сортировки
		allowedFields := map[string]string{
			"id":                 "users.id",
			"-id":                "users.id DESC",
			"username":           "users.username",
			"-username":          "users.username DESC",
			"email":              "users.email",
			"-email":             "users.email DESC",
			"name":               "users.name",
			"-name":              "users.name DESC",
			"first_name":         "users.first_name",
			"-first_name":        "users.first_name DESC",
			"last_name":          "users.last_name",
			"-last_name":         "users.last_name DESC",
			"creation_datetime":  "users.created_at",
			"-creation_datetime": "users.created_at DESC",
			"creator_name":       "users.first_name, users.last_name",
			"-creator_name":      "users.first_name DESC, users.last_name DESC",
		}

		if orderBy, exists := allowedFields[ordering]; exists {
			query = query.Order(orderBy)
		} else {
			// По умолчанию сортируем по дате создания (новые вверху)
			query = query.Order("users.created_at DESC")
		}
	} else {
		// Сортировка по умолчанию - по дате создания (новые вверху)
		query = query.Order("users.created_at DESC")
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета пользователей: " + err.Error(),
		})
		return
	}

	// Получение данных с пагинацией
	var users []models.User
	if err := query.Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователей: " + err.Error(),
		})
		return
	}

	// Преобразование в response format
	userResponses := make([]UserResponse, len(users))
	for i, user := range users {
		// Определяем полное имя
		fullName := user.Name
		if fullName == "" && (user.FirstName != "" || user.LastName != "") {
			// Если поле Name пустое, формируем его из FirstName и LastName
			if user.FirstName != "" && user.LastName != "" {
				fullName = user.FirstName + " " + user.LastName
			} else if user.FirstName != "" {
				fullName = user.FirstName
			} else if user.LastName != "" {
				fullName = user.LastName
			}
		}

		// Определяем имя создателя
		creatorName := ""
		if user.FirstName != "" && user.LastName != "" {
			creatorName = user.FirstName + " " + user.LastName
		} else if user.FirstName != "" {
			creatorName = user.FirstName
		} else if user.LastName != "" {
			creatorName = user.LastName
		} else {
			creatorName = user.Username // Fallback на username
		}

		// Форматируем дату последнего входа
		var lastLoginFormatted *string
		if user.LastLogin != nil {
			formatted := user.LastLogin.Format("2006-01-02T15:04:05Z")
			lastLoginFormatted = &formatted
		}

		userResponses[i] = UserResponse{
			ID:               user.ID,
			Username:         user.Username,
			Email:            user.Email,
			FirstName:        user.FirstName,
			LastName:         user.LastName,
			Name:             fullName,        // Полное имя (сформированное или из модели)
			Phone:            user.Phone,      // Телефон
			TelegramID:       user.TelegramID, // Telegram ID
			UserType:         user.UserType,   // Тип пользователя
			CreatorName:      creatorName,     // Имя создателя
			LastLogin:        lastLoginFormatted,
			CreationDatetime: user.CreatedAt.Format("2006-01-02T15:04:05Z"), // Дата создания пользователя
			IsActive:         user.IsActive,
			RoleID:           user.RoleID,
			Role:             user.Role,
			TemplateID:       user.TemplateID,
			Template:         user.Template,
			LoginCount:       user.LoginCount,
			CreatedAt:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:        user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if user.LastLogin != nil {
			lastLogin := user.LastLogin.Format("2006-01-02T15:04:05Z")
			userResponses[i].LastLogin = &lastLogin
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": userResponses,
			"total": total,
			"page":  page,
			"limit": limit,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetUser возвращает данные конкретного пользователя
func GetUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный ID пользователя",
		})
		return
	}

	var user models.User
	if err := db.Preload("Role").Preload("Template").First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Пользователь не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователя: " + err.Error(),
		})
		return
	}

	// Определяем полное имя
	fullName := user.Name
	if fullName == "" && (user.FirstName != "" || user.LastName != "") {
		// Если поле Name пустое, формируем его из FirstName и LastName
		if user.FirstName != "" && user.LastName != "" {
			fullName = user.FirstName + " " + user.LastName
		} else if user.FirstName != "" {
			fullName = user.FirstName
		} else if user.LastName != "" {
			fullName = user.LastName
		}
	}

	// Определяем имя создателя
	creatorName := ""
	if user.FirstName != "" && user.LastName != "" {
		creatorName = user.FirstName + " " + user.LastName
	} else if user.FirstName != "" {
		creatorName = user.FirstName
	} else if user.LastName != "" {
		creatorName = user.LastName
	} else {
		creatorName = user.Username // Fallback на username
	}

	// Форматируем дату последнего входа
	var lastLoginFormatted *string
	if user.LastLogin != nil {
		formatted := user.LastLogin.Format("2006-01-02T15:04:05Z")
		lastLoginFormatted = &formatted
	}

	userResponse := UserResponse{
		ID:               user.ID,
		Username:         user.Username,
		Email:            user.Email,
		FirstName:        user.FirstName,
		LastName:         user.LastName,
		Name:             fullName,
		Phone:            user.Phone,
		TelegramID:       user.TelegramID,
		UserType:         user.UserType,
		CreatorName:      creatorName,
		LastLogin:        lastLoginFormatted,
		CreationDatetime: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		IsActive:         user.IsActive,
		RoleID:           user.RoleID,
		Role:             user.Role,
		TemplateID:       user.TemplateID,
		Template:         user.Template,
		LoginCount:       user.LoginCount,
		CreatedAt:        user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// CreateUser создает нового пользователя
func CreateUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Проверяем, что роль существует
	var role models.Role
	if err := db.First(&role, req.RoleID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Роль не найдена",
		})
		return
	}

	// Проверяем шаблон, если указан
	if req.TemplateID != nil {
		var template models.UserTemplate
		if err := db.First(&template, *req.TemplateID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Шаблон пользователя не найден",
			})
			return
		}
	}

	// Хешируем пароль
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка хеширования пароля",
		})
		return
	}

	// Создаем пользователя
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	user := models.User{
		Username:   req.Username,
		Email:      req.Email,
		Password:   string(hashedPassword),
		FirstName:  req.FirstName,
		LastName:   req.LastName,
		Name:       req.Name,
		Phone:      req.Phone,
		TelegramID: req.TelegramID,
		UserType:   req.UserType,
		IsActive:   isActive,
		RoleID:     &req.RoleID,
		TemplateID: req.TemplateID,
		LoginCount: 0,
	}

	if err := db.Create(&user).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Пользователь с таким именем пользователя или email уже существует",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания пользователя: " + err.Error(),
		})
		return
	}

	// Загружаем созданного пользователя с связями
	if err := db.Preload("Role").Preload("Template").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка загрузки созданного пользователя: " + err.Error(),
		})
		return
	}

	userResponse := UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		IsActive:   user.IsActive,
		RoleID:     user.RoleID,
		Role:       user.Role,
		TemplateID: user.TemplateID,
		Template:   user.Template,
		LoginCount: user.LoginCount,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusCreated, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// UpdateUser обновляет данные пользователя
func UpdateUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный ID пользователя",
		})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Находим пользователя
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Пользователь не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователя: " + err.Error(),
		})
		return
	}

	// Проверяем роль, если указана
	if req.RoleID != nil {
		var role models.Role
		if err := db.First(&role, *req.RoleID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Роль не найдена",
			})
			return
		}
	}

	// Проверяем шаблон, если указан
	if req.TemplateID != nil {
		var template models.UserTemplate
		if err := db.First(&template, *req.TemplateID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": "error",
				"error":  "Шаблон пользователя не найден",
			})
			return
		}
	}

	// Обновляем поля
	updates := make(map[string]interface{})
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Email != "" {
		updates["email"] = req.Email
	}
	if req.FirstName != "" {
		updates["first_name"] = req.FirstName
	}
	if req.LastName != "" {
		updates["last_name"] = req.LastName
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Phone != "" {
		updates["phone"] = req.Phone
	}
	if req.TelegramID != "" {
		updates["telegram_id"] = req.TelegramID
	}
	if req.UserType != "" {
		updates["user_type"] = req.UserType
	}
	if req.RoleID != nil {
		updates["role_id"] = *req.RoleID
	}
	if req.TemplateID != nil {
		updates["template_id"] = *req.TemplateID
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if err := db.Model(&user).Updates(updates).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
			strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "constraint") {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "Пользователь с таким именем пользователя или email уже существует",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления пользователя: " + err.Error(),
		})
		return
	}

	// Загружаем обновленного пользователя с связями
	if err := db.Preload("Role").Preload("Template").First(&user, user.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка загрузки обновленного пользователя: " + err.Error(),
		})
		return
	}

	userResponse := UserResponse{
		ID:         user.ID,
		Username:   user.Username,
		Email:      user.Email,
		FirstName:  user.FirstName,
		LastName:   user.LastName,
		IsActive:   user.IsActive,
		RoleID:     user.RoleID,
		Role:       user.Role,
		TemplateID: user.TemplateID,
		Template:   user.Template,
		LoginCount: user.LoginCount,
		CreatedAt:  user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  user.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   userResponse,
	})
}

// GetUsersStats возвращает статистику пользователей
func GetUsersStats(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	// Подсчет общего количества пользователей
	var totalUsers int64
	if err := db.Model(&models.User{}).Count(&totalUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета общего количества пользователей: " + err.Error(),
		})
		return
	}

	// Подсчет активных пользователей
	var activeUsers int64
	if err := db.Model(&models.User{}).Where("is_active = ?", true).Count(&activeUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета активных пользователей: " + err.Error(),
		})
		return
	}

	// Подсчет неактивных пользователей
	inactiveUsers := totalUsers - activeUsers

	// Подсчет администраторов (пользователи с ролью admin)
	var adminUsers int64
	if err := db.Table("users").
		Joins("LEFT JOIN roles ON users.role_id = roles.id").
		Where("users.deleted_at IS NULL AND users.is_active = ? AND roles.name = ?", true, "admin").
		Count(&adminUsers).Error; err != nil {
		// Если ошибка, пробуем альтернативный способ
		if err := db.Table("users").
			Joins("LEFT JOIN roles ON users.role_id = roles.id").
			Where("users.deleted_at IS NULL AND users.is_active = ? AND (roles.display_name LIKE ? OR roles.name LIKE ?)", true, "%админ%", "%admin%").
			Count(&adminUsers).Error; err != nil {
			// Если и это не работает, устанавливаем 0
			adminUsers = 0
		}
	}

	// Подсчет обычных пользователей (все активные минус администраторы)
	regularUsers := activeUsers - adminUsers

	// Подсчет пользователей по ролям
	type RoleStats struct {
		RoleName string `json:"role_name"`
		Count    int64  `json:"count"`
	}

	var roleStats []RoleStats
	if err := db.Table("users").
		Select("roles.display_name as role_name, COUNT(users.id) as count").
		Joins("LEFT JOIN roles ON users.role_id = roles.id").
		Where("users.deleted_at IS NULL").
		Group("roles.id, roles.display_name").
		Scan(&roleStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения статистики ролей: " + err.Error(),
		})
		return
	}

	// Подсчет пользователей, созданных за последние 30 дней
	var recentUsers int64
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	if err := db.Model(&models.User{}).
		Where("created_at >= ?", thirtyDaysAgo).
		Count(&recentUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета новых пользователей: " + err.Error(),
		})
		return
	}

	// Подсчет недавних входов (за последние 7 дней)
	var recentLogins int64
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	if err := db.Model(&models.User{}).
		Where("last_login >= ?", sevenDaysAgo).
		Count(&recentLogins).Error; err != nil {
		// Если ошибка, устанавливаем 0
		recentLogins = 0
	}

	// Создаем карту статистики по ролям
	byRole := make(map[string]int64)
	for _, roleStat := range roleStats {
		byRole[roleStat.RoleName] = roleStat.Count
	}

	// Создаем карту статистики по типам
	byType := map[string]int64{
		"active":   activeUsers,
		"inactive": inactiveUsers,
		"admin":    adminUsers,
		"regular":  regularUsers,
	}

	stats := gin.H{
		"total":          totalUsers,
		"active":         activeUsers,
		"inactive":       inactiveUsers,
		"admins":         adminUsers,
		"regular_users":  regularUsers,
		"total_users":    totalUsers,
		"active_users":   activeUsers,
		"inactive_users": inactiveUsers,
		"recent_users":   recentUsers,
		"recent_logins":  recentLogins,
		"by_role":        byRole,
		"by_type":        byType,
		"role_stats":     roleStats,
		"last_updated":   time.Now().Format("2006-01-02T15:04:05Z"),
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// DeleteUser удаляет пользователя (soft delete)
func DeleteUser(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный ID пользователя",
		})
		return
	}

	// Находим пользователя
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Пользователь не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователя: " + err.Error(),
		})
		return
	}

	// Soft delete
	if err := db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка удаления пользователя: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "User deleted successfully",
	})
}

// BulkDeleteUsersRequest представляет запрос на массовое удаление пользователей
type BulkDeleteUsersRequest struct {
	UserIDs []uint `json:"user_ids" binding:"required,min=1"`
}

// BulkDeleteUsers массово удаляет пользователей (soft delete)
func BulkDeleteUsers(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	var req BulkDeleteUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Не указаны ID пользователей",
		})
		return
	}

	// Проверяем, что все пользователи существуют и загружаем их с ролями
	var existingUsers []models.User
	if err := db.Preload("Role").Where("id IN ?", req.UserIDs).Find(&existingUsers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователей: " + err.Error(),
		})
		return
	}

	if len(existingUsers) != len(req.UserIDs) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Некоторые пользователи не найдены",
		})
		return
	}

	// Проверяем, что среди пользователей нет администраторов
	var adminUsers []string
	for _, user := range existingUsers {
		if user.Role != nil && (user.Role.Name == "admin" || user.Role.Name == "administrator") {
			adminUsers = append(adminUsers, user.Username)
		}
	}

	if len(adminUsers) > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Нельзя удалить администраторов: " + strings.Join(adminUsers, ", "),
		})
		return
	}

	// Выполняем массовое мягкое удаление
	result := db.Where("id IN ?", req.UserIDs).Delete(&models.User{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка удаления пользователей: " + result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Users deleted successfully",
		"deleted": result.RowsAffected,
	})
}

// UpdateUserPasswordRequest представляет запрос на смену пароля пользователя
type UpdateUserPasswordRequest struct {
	UserID             int    `json:"userId" binding:"required,min=1"`
	NewPassword        string `json:"newPassword" binding:"required,min=6,max=255"`
	ConfirmNewPassword string `json:"confirmNewPassword" binding:"required,min=6,max=255"`
}

// UpdateUserPassword изменяет пароль пользователя (только для администраторов)
func UpdateUserPassword(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	var req UpdateUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  translateValidationError(err),
		})
		return
	}

	// Проверяем, что новый пароль и подтверждение совпадают
	if req.NewPassword != req.ConfirmNewPassword {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Новый пароль и подтверждение не совпадают",
		})
		return
	}

	// Получаем информацию о текущем пользователе из токена
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Пользователь не аутентифицирован",
		})
		return
	}

	// Получаем текущего пользователя (администратора)
	var currentUser models.User
	if err := db.Preload("Role").First(&currentUser, userID).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Current user not found",
		})
		return
	}

	// Проверяем, что текущий пользователь - администратор
	if currentUser.Role == nil || (currentUser.Role.Name != "admin" && currentUser.Role.Name != "administrator") {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Only administrators can change user passwords",
		})
		return
	}

	// Находим пользователя, для которого меняем пароль
	var targetUser models.User
	if err := db.First(&targetUser, req.UserID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Пользователь не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения пользователя: " + err.Error(),
		})
		return
	}

	// Устанавливаем новый пароль
	if err := targetUser.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to hash password: " + err.Error(),
		})
		return
	}

	// Сохраняем изменения
	if err := db.Save(&targetUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to update password: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "Пароль изменён",
	})
}

// CheckUsernameRequest представляет запрос на проверку имени пользователя
type CheckUsernameRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
}

// CheckUsernameResponse представляет ответ проверки имени пользователя
type CheckUsernameResponse struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"` // "crm" или "axenta"
}

// CheckUsername проверяет доступность имени пользователя в CRM и Axenta
func CheckUsername(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	var req CheckUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный формат запроса: " + err.Error(),
		})
		return
	}

	// Проверяем в локальной базе данных CRM
	var existingUser models.User
	err := db.Where("username = ?", req.Username).First(&existingUser).Error
	if err == nil {
		// Пользователь найден в CRM
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": CheckUsernameResponse{
				Available: false,
				Message:   "Пользователь с таким именем уже существует в CRM",
				Source:    "crm",
			},
		})
		return
	}

	if err != gorm.ErrRecordNotFound {
		// Ошибка базы данных
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка проверки имени пользователя: " + err.Error(),
		})
		return
	}

	// Проверяем в Axenta Cloud (если доступен токен)
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Создаем сервис для работы с Axenta
		axentaUserService := services.NewAxentaUserService(db)

		// Пытаемся получить пользователя из Axenta
		axentaUser, err := axentaUserService.GetUserFromAxenta(token)
		if err == nil && axentaUser.Username == req.Username {
			// Пользователь найден в Axenta
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"data": CheckUsernameResponse{
					Available: false,
					Message:   "Пользователь с таким именем уже существует в Axenta",
					Source:    "axenta",
				},
			})
			return
		}
	}

	// Имя пользователя доступно
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": CheckUsernameResponse{
			Available: true,
			Message:   "Имя пользователя доступно",
		},
	})
}

// UserActivateRequest представляет запрос на активацию/деактивацию пользователя
type UserActivateRequest struct {
	State bool `json:"state"`
}

// ActivateUser активирует/деактивирует пользователя через прокси к Axenta Cloud API
func ActivateUser(c *gin.Context) {
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

	// Получаем ID пользователя из параметров URL
	idStr := c.Param("id")
	if idStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "User ID is required",
		})
		return
	}

	// Читаем тело запроса
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Failed to read request body: " + err.Error(),
		})
		return
	}

	// Строим URL для Axenta API
	axentaURL := fmt.Sprintf("https://axenta.cloud/api/cms/users/%s/activate/", idStr)

	// Создаем HTTP клиент
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Создаем запрос
	req, err := http.NewRequest("POST", axentaURL, bytes.NewBuffer(body))
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

	// Логируем запрос
	fmt.Printf("🔄 Proxy POST request to Axenta API: %s\n", axentaURL)
	authHeaderPreview := authHeader
	if len(authHeader) > 20 {
		authHeaderPreview = authHeader[:20] + "..."
	}
	fmt.Printf("📋 Headers: Authorization=%s, X-Tenant-ID=%s\n", authHeaderPreview, tenantID)
	fmt.Printf("📦 Body: %s\n", string(body))

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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to read response from Axenta API: " + err.Error(),
		})
		return
	}

	// Логируем ответ
	fmt.Printf("📥 Response from Axenta API: Status=%d, Body=%s\n", resp.StatusCode, string(respBody))

	// Возвращаем ответ от Axenta API
	c.Data(resp.StatusCode, "application/json", respBody)
}
