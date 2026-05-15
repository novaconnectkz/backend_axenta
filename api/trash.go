package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TrashStats представляет статистику корзины
type TrashStats struct {
	TotalItems      int            `json:"total_items"`
	ItemsByType     map[string]int `json:"items_by_type"`
	RestoredCount   int            `json:"restored_count"`
	CanBeRestored   int            `json:"can_be_restored"`
	OldestDeletedAt *time.Time     `json:"oldest_deleted_at"`
	RecentDeletedAt *time.Time     `json:"recent_deleted_at"`
}

// getUserIDFromContext извлекает user_id из контекста
// Сначала пытается получить напрямую из user_id, затем из объекта user
func getUserIDFromContext(c *gin.Context) (uint, error) {
	// Пробуем получить напрямую из user_id
	if userID, exists := c.Get("user_id"); exists {
		log.Printf("🔍 getUserIDFromContext: найден user_id в контексте, тип: %T, значение: %v", userID, userID)
		if id, ok := userID.(uint); ok {
			return id, nil
		}
		// Пробуем преобразовать из других типов
		switch v := userID.(type) {
		case int:
			return uint(v), nil
		case int64:
			return uint(v), nil
		case float64:
			return uint(v), nil
		case string:
			if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
				return uint(parsed), nil
			}
			log.Printf("⚠️ getUserIDFromContext: не удалось распарсить user_id из строки: %s", v)
		default:
			log.Printf("⚠️ getUserIDFromContext: неизвестный тип user_id: %T", v)
		}
	} else {
		log.Printf("🔍 getUserIDFromContext: user_id не найден напрямую в контексте")
	}

	// Пробуем извлечь из объекта user
	user := middleware.GetCurrentUser(c)
	if user != nil {
		log.Printf("🔍 getUserIDFromContext: найден объект user, ключи: %v", getMapKeys(user))
		// Пробуем различные поля
		if id, ok := user["id"]; ok {
			log.Printf("🔍 getUserIDFromContext: найден id в user, тип: %T, значение: %v", id, id)
			switch v := id.(type) {
			case float64:
				return uint(v), nil
			case int:
				return uint(v), nil
			case int64:
				return uint(v), nil
			case string:
				if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
					return uint(parsed), nil
				}
				log.Printf("⚠️ getUserIDFromContext: не удалось распарсить id из строки: %s", v)
			default:
				log.Printf("⚠️ getUserIDFromContext: неизвестный тип id в user: %T", v)
			}
		}
		// Пробуем user_id
		if id, ok := user["user_id"]; ok {
			log.Printf("🔍 getUserIDFromContext: найден user_id в user, тип: %T, значение: %v", id, id)
			switch v := id.(type) {
			case float64:
				return uint(v), nil
			case int:
				return uint(v), nil
			case int64:
				return uint(v), nil
			case string:
				if parsed, err := strconv.ParseUint(v, 10, 32); err == nil {
					return uint(parsed), nil
				}
				log.Printf("⚠️ getUserIDFromContext: не удалось распарсить user_id из строки: %s", v)
			default:
				log.Printf("⚠️ getUserIDFromContext: неизвестный тип user_id в user: %T", v)
			}
		}
	} else {
		log.Printf("🔍 getUserIDFromContext: объект user не найден в контексте")
	}

	return 0, fmt.Errorf("user_id not found in context")
}

// getMapKeys возвращает список ключей из map
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getCompanyIDFromContext извлекает company_id из контекста и преобразует в uint
func getCompanyIDFromContext(c *gin.Context) (uint, error) {
	companyID, exists := c.Get("company_id")
	if !exists {
		return 0, fmt.Errorf("company_id not found in context")
	}

	// Пробуем преобразовать в uint
	switch v := companyID.(type) {
	case uint:
		return v, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("company_id cannot be negative")
		}
		return uint(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("company_id cannot be negative")
		}
		return uint(v), nil
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("company_id cannot be negative")
		}
		return uint(v), nil
	case string:
		parsed, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("failed to parse company_id from string: %w", err)
		}
		return uint(parsed), nil
	default:
		return 0, fmt.Errorf("unknown company_id type: %T", v)
	}
}

// GetTrashItems возвращает список удаленных элементов
func GetTrashItems(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	// Получаем company_id из контекста
	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Company ID not found",
		})
		return
	}

	// Параметры запроса
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	entityType := c.Query("entity_type") // Фильтр по типу
	search := c.Query("search")          // Поиск
	showRestored := c.DefaultQuery("show_restored", "false") == "true"
	showPermanentlyDeleted := c.DefaultQuery("show_permanently_deleted", "false") == "true"

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	offset := (page - 1) * limit

	// Базовый запрос
	query := db.Model(&models.DeletedItem{}).Where("company_id = ?", companyID)

	// Фильтры
	if entityType != "" {
		query = query.Where("entity_type = ?", entityType)
	}

	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(
			"entity_name ILIKE ? OR entity_description ILIKE ? OR entity_preview ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// Фильтр по статусу восстановления
	if !showRestored {
		query = query.Where("is_restored = ?", false)
	}

	if !showPermanentlyDeleted {
		query = query.Where("is_permanently_deleted = ?", false)
	}

	// Подсчет общего количества
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка подсчета элементов: " + err.Error(),
		})
		return
	}

	// Получение элементов
	var items []models.DeletedItem
	if err := query.
		Order("deleted_at_custom DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения элементов: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items": items,
			"pagination": gin.H{
				"page":  page,
				"limit": limit,
				"total": total,
				"pages": (total + int64(limit) - 1) / int64(limit),
			},
		},
	})
}

// GetTrashStats возвращает статистику корзины
func GetTrashStats(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	companyID, exists := c.Get("company_id")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Company ID not found",
		})
		return
	}

	var stats TrashStats
	stats.ItemsByType = make(map[string]int)

	// Общее количество (только не восстановленные и не удаленные навсегда)
	// Это соответствует фильтрам по умолчанию в GetTrashItems
	var total int64
	db.Model(&models.DeletedItem{}).
		Where("company_id = ? AND is_restored = ? AND is_permanently_deleted = ?", companyID, false, false).
		Count(&total)
	stats.TotalItems = int(total)

	// Количество по типам (только не восстановленные и не удаленные навсегда)
	type TypeCount struct {
		EntityType string
		Count      int64
	}
	var typeCounts []TypeCount
	db.Model(&models.DeletedItem{}).
		Select("entity_type, COUNT(*) as count").
		Where("company_id = ? AND is_restored = ? AND is_permanently_deleted = ?", companyID, false, false).
		Group("entity_type").
		Scan(&typeCounts)

	for _, tc := range typeCounts {
		stats.ItemsByType[tc.EntityType] = int(tc.Count)
	}

	// Количество восстановленных
	var restoredCount int64
	db.Model(&models.DeletedItem{}).
		Where("company_id = ? AND is_restored = ?", companyID, true).
		Count(&restoredCount)
	stats.RestoredCount = int(restoredCount)

	// Количество доступных для восстановления
	var canBeRestoredCount int64
	db.Model(&models.DeletedItem{}).
		Where("company_id = ? AND is_restored = ? AND is_permanently_deleted = ?", companyID, false, false).
		Count(&canBeRestoredCount)
	stats.CanBeRestored = int(canBeRestoredCount)

	// Самое старое удаление (только не восстановленные и не удаленные навсегда)
	var oldestItem models.DeletedItem
	if err := db.Where("company_id = ? AND is_restored = ? AND is_permanently_deleted = ?", companyID, false, false).
		Order("deleted_at_custom ASC").
		First(&oldestItem).Error; err == nil {
		stats.OldestDeletedAt = &oldestItem.DeletedAtCustom
	}

	// Самое недавнее удаление (только не восстановленные и не удаленные навсегда)
	var recentItem models.DeletedItem
	if err := db.Where("company_id = ? AND is_restored = ? AND is_permanently_deleted = ?", companyID, false, false).
		Order("deleted_at_custom DESC").
		First(&recentItem).Error; err == nil {
		stats.RecentDeletedAt = &recentItem.DeletedAtCustom
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// RestoreItemRequest представляет запрос на восстановление элемента
type RestoreItemRequest struct {
	RestoreNote string `json:"restore_note"` // Опциональная заметка о восстановлении
}

// RestoreItem восстанавливает удаленный элемент
func RestoreItem(c *gin.Context) {
	db := database.GetTenantDB(c)
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	itemID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный ID элемента",
		})
		return
	}

	companyID, err := getCompanyIDFromContext(c)
	if err != nil {
		log.Printf("❌ RestoreItem: ошибка получения company_id: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Company ID not found: " + err.Error(),
		})
		return
	}

	// Получаем user_id из контекста
	userID, err := getUserIDFromContext(c)
	if err != nil {
		log.Printf("⚠️ RestoreItem: user_id не найден, продолжаем без user_id: %v", err)
		// Продолжаем без user_id - поле RestoredBy будет nil
		userID = 0
	}

	// Находим запись об удалении
	var deletedItem models.DeletedItem
	if err := db.Where("id = ? AND company_id = ?", itemID, companyID).First(&deletedItem).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Элемент не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения элемента: " + err.Error(),
		})
		return
	}

	// Проверяем, можно ли восстановить
	if !deletedItem.CanBeRestored() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Элемент не может быть восстановлен",
		})
		return
	}

	// Получаем имя пользователя
	var user models.User
	var restoredByName string
	if err := db.First(&user, userID).Error; err == nil {
		if user.FirstName != "" || user.LastName != "" {
			restoredByName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
		} else {
			restoredByName = user.Username
		}
	} else {
		// Если не нашли в БД, пробуем получить из контекста
		userMap := middleware.GetCurrentUser(c)
		if userMap != nil {
			if username, ok := userMap["username"].(string); ok {
				restoredByName = username
			} else if firstName, ok := userMap["first_name"].(string); ok {
				restoredByName = firstName
				if lastName, ok := userMap["last_name"].(string); ok {
					restoredByName = fmt.Sprintf("%s %s", firstName, lastName)
				}
			}
		}
		if restoredByName == "" {
			restoredByName = fmt.Sprintf("User %d", userID)
		}
	}

	// Начинаем транзакцию
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Восстанавливаем запись в оригинальной таблице
	var restored bool
	var restoreError error

	switch deletedItem.EntityType {
	case "user":
		restored, restoreError = restoreUser(tx, deletedItem.EntityID)
	case "contract":
		restored, restoreError = restoreContract(tx, deletedItem.EntityID)
	case "object":
		restored, restoreError = restoreObject(tx, deletedItem.EntityID)
	case "warehouse":
		restored, restoreError = restoreWarehouse(tx, deletedItem.EntityID)
	case "subscription":
		restored, restoreError = restoreSubscription(tx, deletedItem.EntityID)
	case "template":
		restored, restoreError = restoreTemplate(tx, deletedItem.EntityType, deletedItem.EntityID)
	default:
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Восстановление типа '%s' не поддерживается", deletedItem.EntityType),
		})
		return
	}

	if restoreError != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка восстановления элемента: " + restoreError.Error(),
		})
		return
	}

	if !restored {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Элемент не найден в базе данных",
		})
		return
	}

	// Обновляем запись об удалении
	now := time.Now()
	deletedItem.IsRestored = true
	deletedItem.RestoredAt = &now
	if userID > 0 {
		deletedItem.RestoredBy = &userID
	} else {
		deletedItem.RestoredBy = nil
	}
	deletedItem.RestoredByName = restoredByName

	if err := tx.Save(&deletedItem).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления записи: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Элемент успешно восстановлен",
		"data":    deletedItem,
	})
}

// PermanentlyDeleteItem окончательно удаляет элемент
func PermanentlyDeleteItem(c *gin.Context) {
	fmt.Printf("🔍 PermanentlyDeleteItem: начало обработки запроса, путь: %s\n", c.Request.URL.Path)
	log.Printf("🔍 PermanentlyDeleteItem: начало обработки запроса, путь: %s", c.Request.URL.Path)

	db := database.GetTenantDB(c)
	if db == nil {
		log.Printf("❌ PermanentlyDeleteItem: подключение к БД недоступно")
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Подключение к базе данных недоступно",
		})
		return
	}

	itemID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	log.Printf("🔍 PermanentlyDeleteItem: itemID = %d", itemID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Неверный ID элемента",
		})
		return
	}

	companyID, err := getCompanyIDFromContext(c)
	if err != nil {
		log.Printf("❌ PermanentlyDeleteItem: ошибка получения company_id: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Company ID not found: " + err.Error(),
		})
		return
	}
	log.Printf("🔍 PermanentlyDeleteItem: company_id получен: %d", companyID)

	// Получаем user_id из контекста
	userID, err := getUserIDFromContext(c)
	if err != nil {
		log.Printf("⚠️ PermanentlyDeleteItem: user_id не найден, продолжаем без user_id: %v", err)
		// Продолжаем без user_id - поле PermanentlyDeletedBy будет nil
		userID = 0
	} else {
		log.Printf("✅ PermanentlyDeleteItem: user_id получен: %d", userID)
	}

	// Находим запись об удалении
	var deletedItem models.DeletedItem
	if err := db.Where("id = ? AND company_id = ?", itemID, companyID).First(&deletedItem).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status": "error",
				"error":  "Элемент не найден",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка получения элемента: " + err.Error(),
		})
		return
	}

	// Проверяем, можно ли окончательно удалить
	if !deletedItem.CanBePermanentlyDeleted() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Элемент уже окончательно удален",
		})
		return
	}

	// Начинаем транзакцию
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Удаляем запись из оригинальной таблицы (окончательное удаление)
	var deleteError error

	switch deletedItem.EntityType {
	case "user":
		deleteError = permanentlyDeleteUser(tx, deletedItem.EntityID)
	case "contract":
		deleteError = permanentlyDeleteContract(tx, deletedItem.EntityID)
	case "object":
		deleteError = permanentlyDeleteObject(tx, deletedItem.EntityID)
	case "warehouse":
		deleteError = permanentlyDeleteWarehouse(tx, deletedItem.EntityID)
	case "subscription":
		deleteError = permanentlyDeleteSubscription(tx, deletedItem.EntityID)
	case "template":
		deleteError = permanentlyDeleteTemplate(tx, deletedItem.EntityType, deletedItem.EntityID)
	default:
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Окончательное удаление типа '%s' не поддерживается", deletedItem.EntityType),
		})
		return
	}

	if deleteError != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка окончательного удаления: " + deleteError.Error(),
		})
		return
	}

	// Обновляем запись об удалении
	now := time.Now()
	deletedItem.IsPermanentlyDeleted = true
	deletedItem.PermanentlyDeletedAt = &now
	if userID > 0 {
		deletedItem.PermanentlyDeletedBy = &userID
	} else {
		deletedItem.PermanentlyDeletedBy = nil
		fmt.Printf("⚠️ PermanentlyDeleteItem: PermanentlyDeletedBy установлен в nil, так как user_id не найден\n")
	}

	if err := tx.Save(&deletedItem).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка обновления записи: " + err.Error(),
		})
		return
	}

	tx.Commit()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Элемент окончательно удален",
	})
}

// Вспомогательные функции для восстановления

func restoreUser(db *gorm.DB, entityID uint) (bool, error) {
	var user models.User
	if err := db.Unscoped().First(&user, entityID).Error; err != nil {
		return false, err
	}
	if err := db.Unscoped().Model(&user).Update("deleted_at", nil).Error; err != nil {
		return false, err
	}
	return true, nil
}

func restoreContract(db *gorm.DB, entityID uint) (bool, error) {
	var contract models.Contract
	if err := db.Unscoped().First(&contract, entityID).Error; err != nil {
		return false, err
	}
	if err := db.Unscoped().Model(&contract).Update("deleted_at", nil).Error; err != nil {
		return false, err
	}
	return true, nil
}

func restoreObject(db *gorm.DB, entityID uint) (bool, error) {
	var object models.Object
	if err := db.Unscoped().First(&object, entityID).Error; err != nil {
		return false, err
	}
	if err := db.Unscoped().Model(&object).Update("deleted_at", nil).Error; err != nil {
		return false, err
	}
	return true, nil
}

func restoreWarehouse(db *gorm.DB, entityID uint) (bool, error) {
	var item models.Equipment
	if err := db.Unscoped().First(&item, entityID).Error; err != nil {
		return false, err
	}
	if err := db.Unscoped().Model(&item).Update("deleted_at", nil).Error; err != nil {
		return false, err
	}
	return true, nil
}

func restoreSubscription(db *gorm.DB, entityID uint) (bool, error) {
	var subscription models.Subscription
	if err := db.Unscoped().First(&subscription, entityID).Error; err != nil {
		return false, err
	}
	if err := db.Unscoped().Model(&subscription).Update("deleted_at", nil).Error; err != nil {
		return false, err
	}
	return true, nil
}

func restoreTemplate(db *gorm.DB, entityType string, entityID uint) (bool, error) {
	// Определяем тип шаблона и восстанавливаем
	switch entityType {
	case "report_template":
		var template models.ReportTemplate
		if err := db.Unscoped().First(&template, entityID).Error; err != nil {
			return false, err
		}
		if err := db.Unscoped().Model(&template).Update("deleted_at", nil).Error; err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unknown template type: %s", entityType)
	}
	return true, nil
}

// Вспомогательные функции для окончательного удаления

func permanentlyDeleteUser(db *gorm.DB, entityID uint) error {
	return db.Unscoped().Delete(&models.User{}, entityID).Error
}

func permanentlyDeleteContract(db *gorm.DB, entityID uint) error {
	return db.Unscoped().Delete(&models.Contract{}, entityID).Error
}

func permanentlyDeleteObject(db *gorm.DB, entityID uint) error {
	return db.Unscoped().Delete(&models.Object{}, entityID).Error
}

func permanentlyDeleteWarehouse(db *gorm.DB, entityID uint) error {
	return db.Unscoped().Delete(&models.Equipment{}, entityID).Error
}

func permanentlyDeleteSubscription(db *gorm.DB, entityID uint) error {
	return db.Unscoped().Delete(&models.Subscription{}, entityID).Error
}

func permanentlyDeleteTemplate(db *gorm.DB, entityType string, entityID uint) error {
	switch entityType {
	case "report_template":
		return db.Unscoped().Delete(&models.ReportTemplate{}, entityID).Error
	default:
		return fmt.Errorf("unknown template type: %s", entityType)
	}
}

// RecordDeletion записывает информацию об удалении в таблицу deleted_items
// Эта функция должна вызываться при каждом удалении
// sanitizeString удаляет все невалидные UTF-8 последовательности и NULL байты
func sanitizeString(s string) string {
	// Удаляем NULL байты
	s = strings.ReplaceAll(s, "\x00", "")
	// Заменяем невалидные UTF-8 последовательности
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	return s
}

func RecordDeletion(db *gorm.DB, entityType string, entityID uint, entityData interface{}, deletedBy uint, deletedByName string, companyID uint, entityName, entityDescription string) error {
	log.Printf("🗑️ RecordDeletion вызвана: type=%s, entityID=%d, companyID=%d, deletedBy=%d",
		entityType, entityID, companyID, deletedBy)

	// Сериализуем данные в JSON
	jsonData, err := json.Marshal(entityData)
	if err != nil {
		log.Printf("❌ Ошибка сериализации данных: %v", err)
		return fmt.Errorf("failed to serialize entity data: %w", err)
	}

	// Очищаем JSON от невалидных символов
	jsonString := sanitizeString(string(jsonData))

	// Если очистка не помогла, сохраняем в base64
	if !utf8.ValidString(jsonString) || strings.Contains(jsonString, "\x00") {
		log.Printf("⚠️ Использую base64 для данных с невалидными символами")
		jsonString = "base64:" + base64.StdEncoding.EncodeToString(jsonData)
	}

	// Создаем превью данных (первые 200 символов)
	preview := jsonString
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}

	// Очищаем название и описание
	entityName = sanitizeString(entityName)
	entityDescription = sanitizeString(entityDescription)

	deletedItem := models.DeletedItem{
		EntityType:        entityType,
		EntityID:          entityID,
		EntityData:        jsonString,
		DeletedBy:         deletedBy,
		DeletedByName:     deletedByName,
		DeletedAtCustom:   time.Now(),
		CompanyID:         companyID,
		EntityName:        entityName,
		EntityDescription: entityDescription,
		EntityPreview:     sanitizeString(preview),
	}

	log.Printf("🗑️ Создание записи в deleted_items: Name=%s, Type=%s, DataLen=%d", entityName, entityType, len(jsonString))

	if err := db.Create(&deletedItem).Error; err != nil {
		log.Printf("❌ Ошибка создания записи в deleted_items: %v", err)
		previewLen := 100
		if len(jsonString) < previewLen {
			previewLen = len(jsonString)
		}
		log.Printf("❌ Проблемные данные (первые %d символов): %s", previewLen, jsonString[:previewLen])
		return err
	}

	log.Printf("✅ Запись в deleted_items создана успешно, ID=%d", deletedItem.ID)
	return nil
}
