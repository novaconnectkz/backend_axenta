package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// objectsSnapshotTTL — окно свежести AxentaObjectSnapshot. Сверх него — fallback на live Axenta Cloud.
const objectsSnapshotTTL = 24 * time.Hour

// tryServeObjectsFromSnapshot возвращает true, если страница успешно отдана из axenta_object_snapshots.
// Применяет фильтры/пагинацию на стороне БД. При устаревании snapshot или ошибке — false (caller fallback на live).
func tryServeObjectsFromSnapshot(c *gin.Context, page, perPage int) bool {
	db := middleware.GetTenantDB(c)
	if db == nil {
		db = database.DB
	}
	if db == nil {
		return false
	}

	var lastSync time.Time
	if err := db.Model(&models.AxentaObjectSnapshot{}).
		Select("MAX(last_synced_at)").
		Scan(&lastSync).Error; err != nil || lastSync.IsZero() {
		return false
	}
	if time.Since(lastSync) > objectsSnapshotTTL {
		log.Printf("⏰ AxentaObjectSnapshot устарел (last=%v), fallback на live", lastSync)
		return false
	}

	// Базовый запрос: только не удалённые в Axenta объекты
	q := db.Model(&models.AxentaObjectSnapshot{}).Where("axenta_deleted_at IS NULL")

	// Параметры запроса
	search := c.Query("search")
	status := c.Query("status")
	isActive := c.Query("is_active")
	accountID := c.Query("accountId")
	accountName := c.Query("accountName")
	creatorName := c.Query("creatorName")
	deviceTypeName := c.Query("deviceTypeName")
	uniqueID := c.Query("uniqueId")
	contractID := c.Query("contract_id")
	ordering := c.DefaultQuery("ordering", "name")

	if search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		q = q.Where(
			"LOWER(object_name) LIKE ? OR LOWER(unique_id) LIKE ? OR LOWER(account_name) LIKE ? OR phone_numbers ILIKE ?",
			pattern, pattern, pattern, pattern,
		)
	}
	switch isActive {
	case "true", "1":
		q = q.Where("is_active = ?", true)
	case "false", "0":
		q = q.Where("is_active = ?", false)
	}
	switch status {
	case "active":
		q = q.Where("is_active = ?", true)
	case "inactive":
		q = q.Where("is_active = ?", false)
	case "scheduled_delete":
		q = q.Where("scheduled_delete_at IS NOT NULL")
	}
	if accountID != "" {
		if v, err := strconv.ParseInt(accountID, 10, 64); err == nil {
			q = q.Where("account_external_id = ?", v)
		}
	}
	if accountName != "" {
		q = q.Where("LOWER(account_name) LIKE ?", "%"+strings.ToLower(accountName)+"%")
	}
	if creatorName != "" {
		q = q.Where("LOWER(creator_name) LIKE ?", "%"+strings.ToLower(creatorName)+"%")
	}
	if deviceTypeName != "" {
		q = q.Where("LOWER(device_type_name) LIKE ?", "%"+strings.ToLower(deviceTypeName)+"%")
	}
	if uniqueID != "" {
		q = q.Where("LOWER(unique_id) LIKE ?", "%"+strings.ToLower(uniqueID)+"%")
	}
	if contractID != "" {
		// contract_id в текущей модели не отделен от account_external_id (см. live-маппинг ниже)
		if v, err := strconv.ParseInt(contractID, 10, 64); err == nil {
			q = q.Where("account_external_id = ?", v)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		log.Printf("⚠️ AxentaObjectSnapshot count: %v", err)
		return false
	}

	// Сортировка
	orderClause := "object_name ASC"
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

	offset := (page - 1) * perPage
	var rows []models.AxentaObjectSnapshot
	if err := q.Order(orderClause).Limit(perPage).Offset(offset).Find(&rows).Error; err != nil {
		log.Printf("⚠️ AxentaObjectSnapshot find: %v", err)
		return false
	}

	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var phones []string
		if r.PhoneNumbers != nil && *r.PhoneNumbers != "" {
			_ = json.Unmarshal([]byte(*r.PhoneNumbers), &phones)
		}
		creator := ""
		if r.CreatorName != nil {
			creator = *r.CreatorName
		}
		creatorActive := false
		if r.CreatorIsActive != nil {
			creatorActive = *r.CreatorIsActive
		}
		accountActive := false
		if r.AccountIsActive != nil {
			accountActive = *r.AccountIsActive
		}
		var createdAt string
		if r.AxentaCreatedAt != nil {
			createdAt = r.AxentaCreatedAt.Format(time.RFC3339)
		}
		var lastMsg string
		if r.LastCommunicationAt != nil {
			lastMsg = r.LastCommunicationAt.Format(time.RFC3339)
		}
		statusStr := "inactive"
		if r.IsActive {
			statusStr = "active"
		}
		phoneNumber := ""
		if len(phones) > 0 {
			phoneNumber = phones[0]
		}

		items = append(items, gin.H{
			"id":                  r.ExternalObjectID,
			"name":                r.ObjectName,
			"type":                "vehicle",
			"uniqueId":            r.UniqueID,
			"imei":                r.UniqueID,
			"serial_number":       r.UniqueID,
			"is_active":           r.IsActive,
			"status":              statusStr,
			"accountName":         r.AccountName,
			"deviceTypeName":      r.DeviceTypeName,
			"creatorName":         creator,
			"creatorIsActive":     creatorActive,
			"accountIsActive":     accountActive,
			"phoneNumbers":        phones,
			"phone_number":        phoneNumber,
			"createdAt":           createdAt,
			"created_at":          createdAt,
			"updated_at":          createdAt,
			"lastMessageDatetime": lastMsg,
			"company_id":          r.AccountExternalID,
			"contract_id":         r.AccountExternalID,
			"location_id":         r.AccountExternalID,
			"address":             r.AccountName,
			"description":         r.DeviceTypeName + " - " + r.AccountName,
			"settings":            "{}",
			"tags":                []string{r.DeviceTypeName},
			"notes":               "Создатель: " + creator,
			"external_id":         r.UniqueID,
		})
	}

	totalPages := (int(total) + perPage - 1) / perPage
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"items":                items,
			"total":                total,
			"page":                 page,
			"per_page":             perPage,
			"total_pages":          totalPages,
			"from_snapshot":        true,
			"snapshot_age_seconds": int(time.Since(lastSync).Seconds()),
		},
	})
	return true
}
