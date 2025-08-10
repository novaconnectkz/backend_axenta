package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TemplateHandler представляет обработчик для шаблонных операций
type TemplateHandler struct {
	db *gorm.DB
}

// NewTemplateHandler создает новый экземпляр TemplateHandler
func NewTemplateHandler(db *gorm.DB) *TemplateHandler {
	return &TemplateHandler{
		db: db,
	}
}

// APIResponse представляет стандартную структуру ответа API
type APIResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// SuccessResponse возвращает успешный ответ
func SuccessResponse(c *gin.Context, statusCode int, data interface{}) {
	c.JSON(statusCode, APIResponse{
		Status: "success",
		Data:   data,
	})
}

// ErrorResponse возвращает ошибку
func ErrorResponse(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, APIResponse{
		Status: "error",
		Error:  message,
	})
}

// GetTemplate получает шаблонную запись по ID
// GET /api/template/:id
func (h *TemplateHandler) GetTemplate(c *gin.Context) {
	// 1. Валидация входных данных
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ErrorResponse(c, http.StatusBadRequest, "Некорректный ID")
		return
	}

	// 2. Получение данных из базы
	var template interface{} // Замените на вашу модель
	if err := h.db.First(&template, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ErrorResponse(c, http.StatusNotFound, "Запись не найдена")
			return
		}
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при получении данных")
		return
	}

	// 3. Возврат ответа
	SuccessResponse(c, http.StatusOK, template)
}

// GetTemplates получает список всех шаблонных записей
// GET /api/templates
func (h *TemplateHandler) GetTemplates(c *gin.Context) {
	// 1. Парсинг query параметров для пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	// 2. Получение данных из базы
	var templates []interface{} // Замените на вашу модель
	var total int64

	// Подсчет общего количества записей
	h.db.Model(&templates).Count(&total)

	// Получение записей с пагинацией
	if err := h.db.Offset(offset).Limit(limit).Find(&templates).Error; err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при получении данных")
		return
	}

	// 3. Формирование ответа с метаданными пагинации
	response := gin.H{
		"items": templates,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	}

	SuccessResponse(c, http.StatusOK, response)
}

// CreateTemplate создает новую шаблонную запись
// POST /api/template
func (h *TemplateHandler) CreateTemplate(c *gin.Context) {
	// 1. Валидация входных данных
	var template interface{} // Замените на вашу модель
	if err := c.ShouldBindJSON(&template); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "Некорректные входные данные: "+err.Error())
		return
	}

	// 2. Дополнительная валидация (если необходима)
	// if template.Name == "" {
	//     ErrorResponse(c, http.StatusBadRequest, "Поле 'name' обязательно")
	//     return
	// }

	// 3. Сохранение в базу данных
	if err := h.db.Create(&template).Error; err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при создании записи")
		return
	}

	// 4. Возврат ответа
	SuccessResponse(c, http.StatusCreated, template)
}

// UpdateTemplate обновляет существующую шаблонную запись
// PUT /api/template/:id
func (h *TemplateHandler) UpdateTemplate(c *gin.Context) {
	// 1. Валидация ID
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ErrorResponse(c, http.StatusBadRequest, "Некорректный ID")
		return
	}

	// 2. Проверка существования записи
	var existingTemplate interface{} // Замените на вашу модель
	if err := h.db.First(&existingTemplate, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ErrorResponse(c, http.StatusNotFound, "Запись не найдена")
			return
		}
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при получении данных")
		return
	}

	// 3. Валидация входных данных
	var updateData interface{} // Замените на вашу модель
	if err := c.ShouldBindJSON(&updateData); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "Некорректные входные данные: "+err.Error())
		return
	}

	// 4. Обновление записи
	if err := h.db.Model(&existingTemplate).Updates(updateData).Error; err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при обновлении записи")
		return
	}

	// 5. Получение обновленной записи
	h.db.First(&existingTemplate, id)

	// 6. Возврат ответа
	SuccessResponse(c, http.StatusOK, existingTemplate)
}

// DeleteTemplate удаляет шаблонную запись
// DELETE /api/template/:id
func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	// 1. Валидация ID
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ErrorResponse(c, http.StatusBadRequest, "Некорректный ID")
		return
	}

	// 2. Проверка существования записи
	var template interface{} // Замените на вашу модель
	if err := h.db.First(&template, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ErrorResponse(c, http.StatusNotFound, "Запись не найдена")
			return
		}
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при получении данных")
		return
	}

	// 3. Удаление записи (мягкое удаление с GORM)
	if err := h.db.Delete(&template, id).Error; err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при удалении записи")
		return
	}

	// 4. Возврат ответа
	SuccessResponse(c, http.StatusOK, gin.H{"message": "Запись успешно удалена"})
}

// SearchTemplates выполняет поиск по шаблонным записям
// GET /api/templates/search?q=query
func (h *TemplateHandler) SearchTemplates(c *gin.Context) {
	// 1. Получение поискового запроса
	query := c.Query("q")
	if query == "" {
		ErrorResponse(c, http.StatusBadRequest, "Параметр 'q' обязателен")
		return
	}

	// 2. Парсинг параметров пагинации
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	offset := (page - 1) * limit

	// 3. Выполнение поиска
	var templates []interface{} // Замените на вашу модель
	var total int64

	// Поиск по полям модели (замените на соответствующие поля)
	searchQuery := h.db.Where("name ILIKE ?", "%"+query+"%")

	searchQuery.Model(&templates).Count(&total)

	if err := searchQuery.Offset(offset).Limit(limit).Find(&templates).Error; err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "Ошибка при выполнении поиска")
		return
	}

	// 4. Формирование ответа
	response := gin.H{
		"items": templates,
		"query": query,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	}

	SuccessResponse(c, http.StatusOK, response)
}

// RegisterRoutes регистрирует маршруты для данного хендлера
func (h *TemplateHandler) RegisterRoutes(router *gin.RouterGroup) {
	templates := router.Group("/templates")
	{
		templates.GET("", h.GetTemplates)
		templates.GET("/search", h.SearchTemplates)
		templates.POST("", h.CreateTemplate)
		templates.GET("/:id", h.GetTemplate)
		templates.PUT("/:id", h.UpdateTemplate)
		templates.DELETE("/:id", h.DeleteTemplate)
	}
}

/*
ПРИМЕР ИСПОЛЬЗОВАНИЯ:

1. В main.go или router.go:

   func setupRoutes(db *gorm.DB) *gin.Engine {
       r := gin.Default()
       api := r.Group("/api")

       templateHandler := handlers.NewTemplateHandler(db)
       templateHandler.RegisterRoutes(api)

       return r
   }

2. Замените interface{} на вашу модель:

   type YourModel struct {
       gorm.Model
       Name        string `json:"name" gorm:"type:varchar(100)"`
       Description string `json:"description" gorm:"type:text"`
   }

3. Обновите поля поиска в SearchTemplates:

   searchQuery := h.db.Where("name ILIKE ? OR description ILIKE ?",
                            "%"+query+"%", "%"+query+"%")

4. Добавьте дополнительную валидацию в CreateTemplate и UpdateTemplate:

   if template.Name == "" {
       ErrorResponse(c, http.StatusBadRequest, "Поле 'name' обязательно")
       return
   }

СТАНДАРТНЫЕ КОДЫ ОТВЕТОВ:
- 200: Успешное получение данных
- 201: Успешное создание записи
- 400: Некорректные входные данные
- 404: Запись не найдена
- 500: Внутренняя ошибка сервера

СТРУКТУРА JSON ОТВЕТА:
{
    "status": "success|error",
    "data": {}, // для успешных ответов
    "error": "message" // для ошибок
}
*/
