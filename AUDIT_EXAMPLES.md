# Примеры интеграции audit логирования в разные части системы

## 1. Логирование в API handlers

### Пример 1: Создание объекта
```go
func CreateObject(c *gin.Context) {
    var req CreateObjectRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // Логируем ошибку валидации
        audit.LogError(c, "object.create.validation_error", err, gin.H{
            "errors": err.Error(),
        })
        c.JSON(400, gin.H{"status": "error", "error": err.Error()})
        return
    }
    
    object := &models.Object{
        Name: req.Name,
        Type: req.Type,
    }
    
    if err := db.Create(object).Error; err != nil {
        // Логируем ошибку БД
        audit.LogError(c, "object.create.db_error", err, gin.H{
            "object_name": req.Name,
        })
        c.JSON(500, gin.H{"status": "error", "error": "Database error"})
        return
    }
    
    // Логируем успешное создание
    audit.LogSuccess(c, "object.created", gin.H{
        "object_id": object.ID,
        "object_name": object.Name,
        "object_type": object.Type,
    })
    
    c.JSON(200, gin.H{"status": "success", "data": object})
}
```

### Пример 2: Обновление пользователя
```go
func UpdateUser(c *gin.Context) {
    userID := c.Param("id")
    
    var req UpdateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        audit.LogError(c, "user.update.validation_error", err, gin.H{
            "user_id": userID,
        })
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    var user models.User
    if err := db.First(&user, userID).Error; err != nil {
        audit.LogError(c, "user.update.not_found", err, gin.H{
            "user_id": userID,
        })
        c.JSON(404, gin.H{"error": "User not found"})
        return
    }
    
    // Сохраняем старые значения для логирования изменений
    oldValues := gin.H{
        "username": user.Username,
        "email": user.Email,
        "is_active": user.IsActive,
    }
    
    // Обновляем поля
    if req.Username != "" {
        user.Username = req.Username
    }
    if req.Email != "" {
        user.Email = req.Email
    }
    if req.IsActive != nil {
        user.IsActive = *req.IsActive
    }
    
    if err := db.Save(&user).Error; err != nil {
        audit.LogError(c, "user.update.db_error", err, gin.H{
            "user_id": userID,
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Логируем успешное обновление с деталями изменений
    audit.LogSuccess(c, "user.updated", gin.H{
        "user_id": user.ID,
        "old_values": oldValues,
        "new_values": gin.H{
            "username": user.Username,
            "email": user.Email,
            "is_active": user.IsActive,
        },
    })
    
    c.JSON(200, gin.H{"data": user})
}
```

### Пример 3: Удаление с каскадом
```go
func DeleteContract(c *gin.Context) {
    contractID := c.Param("id")
    
    var contract models.Contract
    if err := db.Preload("Objects").First(&contract, contractID).Error; err != nil {
        audit.LogError(c, "contract.delete.not_found", err, gin.H{
            "contract_id": contractID,
        })
        c.JSON(404, gin.H{"error": "Contract not found"})
        return
    }
    
    // Сохраняем информацию для логирования
    objectsCount := len(contract.Objects)
    objectIDs := make([]uint, objectsCount)
    for i, obj := range contract.Objects {
        objectIDs[i] = obj.ID
    }
    
    if err := db.Delete(&contract).Error; err != nil {
        audit.LogError(c, "contract.delete.db_error", err, gin.H{
            "contract_id": contractID,
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Логируем удаление с деталями
    audit.LogSuccess(c, "contract.deleted", gin.H{
        "contract_id": contract.ID,
        "contract_number": contract.Number,
        "objects_detached": objectsCount,
        "object_ids": objectIDs,
    })
    
    c.JSON(200, gin.H{"message": "Contract deleted"})
}
```

## 2. Логирование в Services

### Пример: Сервис биллинга
```go
type BillingService struct {
    db *gorm.DB
}

func (s *BillingService) GenerateInvoice(ctx context.Context, contractID uint, userID string) (*models.Invoice, error) {
    // Логируем начало генерации
    audit.LogWithContext(ctx, userID, "invoice.generation.started", true, gin.H{
        "contract_id": contractID,
    })
    
    var contract models.Contract
    if err := s.db.First(&contract, contractID).Error; err != nil {
        audit.LogWithContext(ctx, userID, "invoice.generation.contract_not_found", false, gin.H{
            "contract_id": contractID,
            "error": err.Error(),
        })
        return nil, err
    }
    
    // Расчет суммы
    amount := s.calculateAmount(&contract)
    
    invoice := &models.Invoice{
        ContractID: contractID,
        Amount: amount,
        Status: "pending",
    }
    
    if err := s.db.Create(invoice).Error; err != nil {
        audit.LogWithContext(ctx, userID, "invoice.generation.failed", false, gin.H{
            "contract_id": contractID,
            "amount": amount,
            "error": err.Error(),
        })
        return nil, err
    }
    
    // Логируем успешную генерацию
    audit.LogWithContext(ctx, userID, "invoice.generated", true, gin.H{
        "invoice_id": invoice.ID,
        "contract_id": contractID,
        "amount": amount,
        "currency": "RUB",
    })
    
    return invoice, nil
}
```

## 3. Логирование системных событий

### Пример: Фоновая задача очистки
```go
func CleanupExpiredData() {
    startTime := time.Now()
    
    audit.Log("system", "cleanup.started", map[string]interface{}{
        "task_type": "expired_data_cleanup",
    })
    
    // Удаляем старые токены
    var deletedTokens int64
    if err := db.Where("expires_at < ?", time.Now()).
        Delete(&models.RefreshToken{}).
        Count(&deletedTokens).Error; err != nil {
        
        audit.Log("system", "cleanup.tokens.failed", map[string]interface{}{
            "error": err.Error(),
            "duration_seconds": time.Since(startTime).Seconds(),
        })
        return
    }
    
    // Архивируем старые логи
    var archivedLogs int64
    cutoffDate := time.Now().AddDate(0, -6, 0)
    if err := db.Where("timestamp < ?", cutoffDate).
        Model(&audit.AuditLog{}).
        Update("deleted_at", time.Now()).
        Count(&archivedLogs).Error; err != nil {
        
        audit.Log("system", "cleanup.audit_logs.failed", map[string]interface{}{
            "error": err.Error(),
        })
    }
    
    // Логируем завершение
    audit.Log("system", "cleanup.completed", map[string]interface{}{
        "deleted_tokens": deletedTokens,
        "archived_logs": archivedLogs,
        "duration_seconds": time.Since(startTime).Seconds(),
    })
}
```

## 4. Логирование изменений настроек

### Пример: Обновление системных настроек
```go
func UpdateSystemSettings(c *gin.Context) {
    var req UpdateSettingsRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        audit.LogError(c, "settings.update.validation_error", err, nil)
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    var settings models.SystemSettings
    if err := db.First(&settings).Error; err != nil {
        audit.LogError(c, "settings.update.not_found", err, nil)
        c.JSON(404, gin.H{"error": "Settings not found"})
        return
    }
    
    // Собираем изменения для логирования
    changes := make(map[string]interface{})
    
    if req.BillingEnabled != nil && *req.BillingEnabled != settings.BillingEnabled {
        changes["billing_enabled"] = map[string]interface{}{
            "old": settings.BillingEnabled,
            "new": *req.BillingEnabled,
        }
        settings.BillingEnabled = *req.BillingEnabled
    }
    
    if req.AutoInvoiceGeneration != nil && *req.AutoInvoiceGeneration != settings.AutoInvoiceGeneration {
        changes["auto_invoice_generation"] = map[string]interface{}{
            "old": settings.AutoInvoiceGeneration,
            "new": *req.AutoInvoiceGeneration,
        }
        settings.AutoInvoiceGeneration = *req.AutoInvoiceGeneration
    }
    
    if len(changes) == 0 {
        c.JSON(200, gin.H{"message": "No changes"})
        return
    }
    
    if err := db.Save(&settings).Error; err != nil {
        audit.LogError(c, "settings.update.db_error", err, gin.H{
            "attempted_changes": changes,
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Логируем изменения
    audit.LogSuccess(c, "settings.updated", gin.H{
        "changes": changes,
        "changed_count": len(changes),
    })
    
    c.JSON(200, gin.H{"data": settings})
}
```

## 5. Логирование интеграций с внешними API

### Пример: Синхронизация с внешней системой
```go
func SyncWithExternalSystem(c *gin.Context) {
    startTime := time.Now()
    
    audit.LogFromContext(c, "integration.sync.started", gin.H{
        "system": "external_crm",
    })
    
    client := &http.Client{Timeout: 30 * time.Second}
    req, _ := http.NewRequest("GET", "https://external-api.com/data", nil)
    
    resp, err := client.Do(req)
    if err != nil {
        audit.LogError(c, "integration.sync.connection_error", err, gin.H{
            "system": "external_crm",
            "duration_seconds": time.Since(startTime).Seconds(),
        })
        c.JSON(500, gin.H{"error": "Connection failed"})
        return
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        audit.LogError(c, "integration.sync.api_error", 
            fmt.Errorf("status code: %d", resp.StatusCode), gin.H{
            "system": "external_crm",
            "status_code": resp.StatusCode,
        })
        c.JSON(500, gin.H{"error": "API error"})
        return
    }
    
    // Обработка данных
    var data ExternalData
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        audit.LogError(c, "integration.sync.parse_error", err, gin.H{
            "system": "external_crm",
        })
        c.JSON(500, gin.H{"error": "Parse error"})
        return
    }
    
    // Сохранение данных
    syncedCount := 0
    for _, item := range data.Items {
        if err := db.Create(&item).Error; err == nil {
            syncedCount++
        }
    }
    
    audit.LogSuccess(c, "integration.sync.completed", gin.H{
        "system": "external_crm",
        "total_items": len(data.Items),
        "synced_items": syncedCount,
        "duration_seconds": time.Since(startTime).Seconds(),
    })
    
    c.JSON(200, gin.H{
        "synced": syncedCount,
        "total": len(data.Items),
    })
}
```

## 6. Логирование массовых операций

### Пример: Массовое удаление пользователей
```go
func BulkDeleteUsers(c *gin.Context) {
    var req struct {
        UserIDs []uint `json:"user_ids" binding:"required,min=1,max=100"`
    }
    
    if err := c.ShouldBindJSON(&req); err != nil {
        audit.LogError(c, "users.bulk_delete.validation_error", err, nil)
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    // Получаем пользователей для логирования
    var users []models.User
    if err := db.Where("id IN ?", req.UserIDs).Find(&users).Error; err != nil {
        audit.LogError(c, "users.bulk_delete.fetch_error", err, gin.H{
            "user_ids": req.UserIDs,
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Собираем информацию для логирования
    userDetails := make([]gin.H, len(users))
    for i, user := range users {
        userDetails[i] = gin.H{
            "id": user.ID,
            "username": user.Username,
            "email": user.Email,
        }
    }
    
    // Удаляем
    result := db.Delete(&models.User{}, req.UserIDs)
    if result.Error != nil {
        audit.LogError(c, "users.bulk_delete.db_error", result.Error, gin.H{
            "requested_count": len(req.UserIDs),
            "users": userDetails,
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Логируем успешное удаление
    audit.LogSuccess(c, "users.bulk_deleted", gin.H{
        "requested_count": len(req.UserIDs),
        "deleted_count": result.RowsAffected,
        "users": userDetails,
    })
    
    c.JSON(200, gin.H{
        "deleted": result.RowsAffected,
    })
}
```

## 7. Логирование экспорта данных

### Пример: Экспорт объектов в Excel
```go
func ExportObjectsToXLSX(c *gin.Context) {
    startTime := time.Now()
    
    audit.LogFromContext(c, "objects.export.started", gin.H{
        "format": "xlsx",
    })
    
    var objects []models.Object
    query := db.Model(&models.Object{})
    
    // Применяем фильтры если есть
    if status := c.Query("status"); status != "" {
        query = query.Where("status = ?", status)
    }
    
    if err := query.Find(&objects).Error; err != nil {
        audit.LogError(c, "objects.export.db_error", err, gin.H{
            "format": "xlsx",
        })
        c.JSON(500, gin.H{"error": "Database error"})
        return
    }
    
    // Создаем Excel файл
    file := xlsx.NewFile()
    sheet, _ := file.AddSheet("Objects")
    
    // ... добавляем данные ...
    
    // Сохраняем во временный файл
    tempFile := fmt.Sprintf("/tmp/objects_export_%d.xlsx", time.Now().Unix())
    if err := file.Save(tempFile); err != nil {
        audit.LogError(c, "objects.export.file_error", err, gin.H{
            "format": "xlsx",
            "objects_count": len(objects),
        })
        c.JSON(500, gin.H{"error": "File creation error"})
        return
    }
    
    // Логируем успешный экспорт
    audit.LogSuccess(c, "objects.export.completed", gin.H{
        "format": "xlsx",
        "objects_count": len(objects),
        "file_size_kb": getFileSize(tempFile) / 1024,
        "duration_seconds": time.Since(startTime).Seconds(),
    })
    
    c.FileAttachment(tempFile, "objects.xlsx")
}
```

## Рекомендации

1. **Логируйте критичные операции**: создание, изменение, удаление данных
2. **Включайте контекст**: кто, что, когда, почему
3. **Не логируйте пароли и токены**: только их хеши или маски
4. **Используйте структурированные данные**: JSON в поле details
5. **Логируйте и успехи и ошибки**: полная картина важнее
6. **Добавляйте метрики**: длительность операций, количество записей
7. **Логируйте изменения**: old_value -> new_value
8. **Группируйте связанные события**: используйте единый action prefix

