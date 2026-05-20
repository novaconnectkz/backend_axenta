package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"backend_axenta/services"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// splitFullName разделяет полное имя на имя и фамилию
func splitFullName(fullName string) (firstName, lastName string) {
	if fullName == "" {
		return "", ""
	}

	// Убираем лишние пробелы
	fullName = strings.TrimSpace(fullName)

	// Разделяем по пробелу
	parts := strings.Fields(fullName)

	if len(parts) == 0 {
		return "", ""
	} else if len(parts) == 1 {
		return parts[0], ""
	} else {
		// Первая часть - имя, остальное - фамилия
		firstName = parts[0]
		lastName = strings.Join(parts[1:], " ")
		return firstName, lastName
	}
}

// shouldExcludeUserFromSearch проверяет, нужно ли исключить пользователя из результатов поиска
// Исключаем пользователя, если поисковый запрос совпадает только с creator_name,
// но не совпадает с основными полями поиска (username, email, first_name, last_name)
func shouldExcludeUserFromSearch(searchQuery string, user map[string]interface{}) bool {
	if searchQuery == "" {
		return false
	}

	searchLower := strings.ToLower(searchQuery)

	// Получаем основные поля для поиска
	username, _ := user["username"].(string)
	email, _ := user["email"].(string)
	firstName, _ := user["first_name"].(string)
	lastName, _ := user["last_name"].(string)
	name, _ := user["name"].(string)
	creatorName, _ := user["creatorName"].(string)

	// Проверяем совпадение с основными полями
	matchesMainFields := false

	if strings.Contains(strings.ToLower(username), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(email), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(firstName), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(lastName), searchLower) {
		matchesMainFields = true
	}
	if strings.Contains(strings.ToLower(name), searchLower) {
		matchesMainFields = true
	}

	// Проверяем совпадение с creator_name
	matchesCreator := strings.Contains(strings.ToLower(creatorName), searchLower)

	// Исключаем пользователя, если поиск совпадает только с creator_name
	return matchesCreator && !matchesMainFields
}

// AxentaCloudObject представляет объект из Axenta Cloud API
type AxentaCloudObject struct {
	ID                  int         `json:"id"`
	Name                string      `json:"name"`
	UniqueID            string      `json:"uniqueId"`
	CreatorName         string      `json:"creatorName"`
	CreatorID           int         `json:"creatorId"`
	CreatorIsActive     bool        `json:"creatorIsActive"`
	AccountID           int         `json:"accountId"`
	AccountName         string      `json:"accountName"`
	AccountType         string      `json:"accountType"`
	AccountIsActive     bool        `json:"accountIsActive"`
	PhoneNumbers        []string    `json:"phoneNumbers"`
	DeviceTypeName      string      `json:"deviceTypeName"`
	LastMessageDatetime string      `json:"lastMessageDatetime"`
	CreatedAt           string      `json:"createdAt"`
	DeletedAt           string      `json:"deletedAt"`
	IsActive            bool        `json:"isActive"`
	CurrentUserAccess   interface{} `json:"currentUserAccess"` // Может быть []string или number
}

// AxentaCloudResponse представляет ответ от Axenta Cloud API
type AxentaCloudResponse struct {
	Count    int                 `json:"count"`
	Next     *string             `json:"next"`
	Previous *string             `json:"previous"`
	Results  []AxentaCloudObject `json:"results"`
}

// GetObjectsFromAxentaCloud отдаёт страницу объектов из axenta_object_snapshots.
// Ф3-B: snapshot-only, без live-proxy в axenta.cloud (после Ф1 request-токен
// невалиден). Устаревший/пустой snapshot → 200 + degraded:true
// (см. serveObjectsFromSnapshot).
func GetObjectsFromAxentaCloud(c *gin.Context) {
	page := c.DefaultQuery("page", "1")
	perPage := c.DefaultQuery("per_page", "50")

	pageNum, _ := strconv.Atoi(page)
	perPageNum, _ := strconv.Atoi(perPage)
	if pageNum < 1 {
		pageNum = 1
	}
	if perPageNum < 1 || perPageNum > 1000 {
		perPageNum = 50
	}

	t0 := time.Now()
	serveObjectsFromSnapshot(c, pageNum, perPageNum)
	log.Printf("📸 /objects из snapshot за %s", time.Since(t0).Round(time.Millisecond))
}

// GetObjectsStatsFromAxentaCloud получает статистику объектов.
// Ф3-B: snapshot-only (один SQL по axenta_object_snapshots + Redis cache TTL 60s),
// без live-proxy в axenta.cloud — после Ф1 request-токен невалиден для Axenta.
// Устаревший/пустой snapshot → 200 с degraded:true (см. serveObjectsStatsFromSnapshot).
func GetObjectsStatsFromAxentaCloud(c *gin.Context) {
	t0 := time.Now()
	serveObjectsStatsFromSnapshot(c)
	log.Printf("📸 /objects/stats из snapshot за %s", time.Since(t0).Round(time.Millisecond))
}

// GetUsersFromAxentaCloud отдаёт страницу пользователей из axenta_user_snapshots.
// Ф3-B: snapshot-only, без live-proxy в axenta.cloud (после Ф1 request-токен
// невалиден). Устаревший/пустой snapshot → 200 + degraded:true
// (см. serveAxentaUsersFromSnapshot).
func GetUsersFromAxentaCloud(c *gin.Context) {
	t0 := time.Now()
	serveAxentaUsersFromSnapshot(c)
	log.Printf("📸 /users из snapshot за %s", time.Since(t0).Round(time.Millisecond))
}

// GetUsersStatsFromAxentaCloud получает статистику пользователей.
// Ф3-B: snapshot-only (COUNT FILTER одним SQL по axenta_user_snapshots),
// без live-proxy в axenta.cloud — после Ф1 request-токен невалиден для Axenta.
// Устаревший/пустой snapshot → 200 с degraded:true (см. serveUsersStatsFromSnapshot).
func GetUsersStatsFromAxentaCloud(c *gin.Context) {
	t0 := time.Now()
	serveUsersStatsFromSnapshot(c)
	log.Printf("📸 /users/stats из snapshot за %s", time.Since(t0).Round(time.Millisecond))
}

// getRoleByAxentaType получает роль из базы данных на основе типа аккаунта Axenta
func getRoleByAxentaType(db *gorm.DB, accountType string) (*models.Role, gin.H) {
	var roleName string

	switch accountType {
	case "partner":
		roleName = "partner"
	case "client":
		roleName = "client"
	default:
		roleName = "user" // Роль по умолчанию
	}

	var role models.Role
	err := db.Where("name = ?", roleName).First(&role).Error
	if err != nil {
		// Если роль не найдена, создаем роли по умолчанию в этой tenant схеме
		if err == gorm.ErrRecordNotFound {
			log.Printf("⚠️ Role %s not found in tenant schema, creating default roles...", roleName)

			// Создаем сервис и роли по умолчанию
			axentaUserService := services.NewAxentaUserService(db)
			if createErr := axentaUserService.EnsureDefaultRoles(); createErr != nil {
				log.Printf("❌ Failed to create default roles: %v", createErr)
				return nil, nil
			}

			// Пытаемся найти роль снова
			err = db.Where("name = ?", roleName).First(&role).Error
			if err != nil {
				log.Printf("❌ Role %s still not found after creation: %v", roleName, err)
				return nil, nil
			}

			log.Printf("✅ Role %s created and found (ID: %d)", roleName, role.ID)
		} else {
			log.Printf("❌ Database error finding role %s: %v", roleName, err)
			return nil, nil
		}
	}

	// Возвращаем роль и данные для frontend
	roleData := gin.H{
		"id":           role.ID,
		"name":         role.Name,
		"display_name": role.DisplayName,
		"description":  role.Description,
		"color":        role.Color,
		"priority":     role.Priority,
		"is_active":    role.IsActive,
		"is_system":    role.IsSystem,
		"created_at":   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated_at":   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return &role, roleData
}

// mapAccountTypeToAxentaType преобразует тип аккаунта Axenta в тип пользователя системы
func mapAccountTypeToAxentaType(accountType interface{}) string {
	if accountType == nil {
		return "client"
	}

	accountTypeStr, ok := accountType.(string)
	if !ok {
		return "client"
	}

	switch accountTypeStr {
	case "partner":
		return "partner"
	case "client":
		return "client"
	default:
		return "client" // По умолчанию
	}
}

// GetDeletedObjectsFromAxentaCloud отдаёт корзину объектов из axenta_object_snapshots.
// Ф3-B7: snapshot-only, без live-proxy в axenta.cloud (после Ф1 request-токен
// невалиден). Устаревший/пустой snapshot → 200 + degraded:true
// (см. serveDeletedObjectsFromSnapshot).
func GetDeletedObjectsFromAxentaCloud(c *gin.Context) {
	t0 := time.Now()
	serveDeletedObjectsFromSnapshot(c)
	log.Printf("📸 /cms/trash из snapshot за %s", time.Since(t0).Round(time.Millisecond))
}

// GetObjectsStatsOptimizedFromAxentaCloud получает оптимизированную статистику объектов.
// Ф3-B: тот же snapshot-only источник, что и /objects/stats (serveObjectsStatsFromSnapshot).
func GetObjectsStatsOptimizedFromAxentaCloud(c *gin.Context) {
	serveObjectsStatsFromSnapshot(c)
}

// GetUsersStatsOptimizedFromAxentaCloud получает оптимизированную статистику пользователей.
// Ф3-B: тот же snapshot-only источник, что и /users/stats (serveUsersStatsFromSnapshot).
func GetUsersStatsOptimizedFromAxentaCloud(c *gin.Context) {
	serveUsersStatsFromSnapshot(c)
}

// ExportObjectsToXLSX экспортирует список объектов в формат XLSX
func ExportObjectsToXLSX(c *gin.Context) {
	log.Printf("📊 Начало экспорта объектов в XLSX")
	log.Printf("📊 URL запроса: %s", c.Request.URL.String())
	log.Printf("📊 Метод: %s", c.Request.Method)

	// Ф3-B: экспорт из snapshot (axenta_object_snapshots), без live-proxy в
	// axenta.cloud — после Ф1 логин = локальный JWT, request-токен невалиден
	// для Axenta. Фильтры 1:1 со списком через applyObjectSnapshotFilters.
	// Пустой/отсутствующий snapshot → валидный XLSX только с заголовками
	// (не JSON-ошибка: фронтенд ждёт blob).
	db := middleware.GetTenantDB(c)
	if db == nil {
		db = database.DB
	}

	var allObjects []AxentaCloudObject
	if db != nil {
		ordering := c.DefaultQuery("ordering", "name")
		orderClause := "object_name"
		switch strings.TrimPrefix(ordering, "-") {
		case "name", "":
			orderClause = "object_name"
		case "createdAt", "created_at", "axenta_created_at":
			orderClause = "axenta_created_at"
		case "lastMessageDatetime", "last_communication_at":
			orderClause = "last_communication_at"
		case "uniqueId", "unique_id":
			orderClause = "unique_id"
		}
		if strings.HasPrefix(ordering, "-") {
			orderClause += " DESC NULLS LAST"
		} else {
			orderClause += " ASC NULLS LAST"
		}

		var rows []models.AxentaObjectSnapshot
		if err := applyObjectSnapshotFilters(c, db).Order(orderClause).Find(&rows).Error; err != nil {
			log.Printf("⚠️ ExportObjectsToXLSX snapshot find: %v", err)
		}
		fmtTime := func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02 15:04:05")
		}
		allObjects = make([]AxentaCloudObject, 0, len(rows))
		for _, r := range rows {
			var phones []string
			if r.PhoneNumbers != nil && *r.PhoneNumbers != "" {
				_ = json.Unmarshal([]byte(*r.PhoneNumbers), &phones)
			}
			creator := ""
			if r.CreatorName != nil {
				creator = *r.CreatorName
			}
			allObjects = append(allObjects, AxentaCloudObject{
				ID:                  int(r.ExternalObjectID),
				Name:                r.ObjectName,
				UniqueID:            r.UniqueID,
				CreatorName:         creator,
				AccountName:         r.AccountName,
				PhoneNumbers:        phones,
				DeviceTypeName:      r.DeviceTypeName,
				LastMessageDatetime: fmtTime(r.LastCommunicationAt),
				CreatedAt:           fmtTime(r.AxentaCreatedAt),
				DeletedAt:           fmtTime(r.AxentaDeletedAt),
				IsActive:            r.IsActive,
				CurrentUserAccess:   "",
				// AccountType/CurrentUserAccess отсутствуют в object-snapshot — пусто.
			})
		}
	}

	log.Printf("📊 Получено объектов для экспорта из snapshot: %d", len(allObjects))

	// Создаем новый Excel файл
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("⚠️ Ошибка закрытия Excel файла: %v", err)
		}
	}()

	sheetName := "Объекты"
	_ = f.SetSheetName("Sheet1", sheetName)

	// Определяем заголовки
	headers := []string{
		"ID",
		"Название",
		"Уникальный ID",
		"Тип устройства",
		"Аккаунт",
		"Тип аккаунта",
		"Создатель",
		"Телефоны",
		"Последнее сообщение",
		"Дата создания",
		"Дата удаления",
		"Активен",
		"Права доступа",
	}

	// Записываем заголовки
	styleHeader, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold: true,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#E0E0E0"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	if err != nil {
		log.Printf("⚠️ Ошибка создания стиля заголовка: %v", err)
	}

	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, header)
		if styleHeader > 0 {
			_ = f.SetCellStyle(sheetName, cell, cell, styleHeader)
		}
	}

	// Записываем данные
	for rowIdx, obj := range allObjects {
		row := rowIdx + 2 // Начинаем с 2 строки (после заголовков)

		// Форматируем данные
		phones := strings.Join(obj.PhoneNumbers, ", ")

		// Обрабатываем CurrentUserAccess, который может быть []string или number
		accessRights := ""
		if accessArray, ok := obj.CurrentUserAccess.([]interface{}); ok {
			strArray := make([]string, len(accessArray))
			for i, v := range accessArray {
				strArray[i] = fmt.Sprintf("%v", v)
			}
			accessRights = strings.Join(strArray, ", ")
		} else if accessStr, ok := obj.CurrentUserAccess.(string); ok {
			accessRights = accessStr
		} else {
			accessRights = fmt.Sprintf("%v", obj.CurrentUserAccess)
		}

		isActive := "Да"
		if !obj.IsActive {
			isActive = "Нет"
		}

		// Записываем значения
		_ = f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), obj.ID)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), obj.Name)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), obj.UniqueID)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), obj.DeviceTypeName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), obj.AccountName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), obj.AccountType)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), obj.CreatorName)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), phones)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), obj.LastMessageDatetime)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), obj.CreatedAt)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("K%d", row), obj.DeletedAt)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("L%d", row), isActive)
		_ = f.SetCellValue(sheetName, fmt.Sprintf("M%d", row), accessRights)
	}

	// Устанавливаем ширину колонок
	columnWidths := map[string]float64{
		"A": 10, // ID
		"B": 30, // Название
		"C": 20, // Уникальный ID
		"D": 20, // Тип устройства
		"E": 25, // Аккаунт
		"F": 15, // Тип аккаунта
		"G": 20, // Создатель
		"H": 25, // Телефоны
		"I": 20, // Последнее сообщение
		"J": 20, // Дата создания
		"K": 20, // Дата удаления
		"L": 10, // Активен
		"M": 30, // Права доступа
	}

	for col, width := range columnWidths {
		_ = f.SetColWidth(sheetName, col, col, width)
	}

	// Добавляем автофильтр
	if len(allObjects) > 0 {
		endCell := fmt.Sprintf("M%d", len(allObjects)+1)
		if err := f.AutoFilter(sheetName, "A1:"+endCell, []excelize.AutoFilterOptions{}); err != nil {
			log.Printf("⚠️ Ошибка добавления автофильтра: %v", err)
		}
	}

	// Замораживаем первую строку (заголовки)
	if err := f.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		log.Printf("⚠️ Ошибка заморозки строки: %v", err)
	}

	// Генерируем имя файла с датой и временем
	fileName := fmt.Sprintf("objects_export_%s.xlsx", time.Now().Format("20060102_150405"))

	// Устанавливаем заголовки для скачивания файла
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Transfer-Encoding", "binary")

	// Сохраняем файл во временный буфер
	buffer, err := f.WriteToBuffer()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Ошибка создания Excel файла: " + err.Error(),
		})
		return
	}

	log.Printf("✅ Экспорт завершен. Экспортировано объектов: %d", len(allObjects))

	// Отправляем файл
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}
