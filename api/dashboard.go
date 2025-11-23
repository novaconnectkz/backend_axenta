package api

import (
	"backend_axenta/database"
	"backend_axenta/middleware"
	"backend_axenta/models"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// DashboardStats структура для статистики dashboard
type DashboardStats struct {
	TotalObjects    int64     `json:"total_objects"`
	ActiveObjects   int64     `json:"active_objects"`
	InactiveObjects int64     `json:"inactive_objects"`
	TotalUsers      int64     `json:"total_users"`
	TotalContracts  int64     `json:"total_contracts"`
	ActiveContracts int64     `json:"active_contracts"`
	MonthlyRevenue  float64   `json:"monthly_revenue"`
	PendingPayments float64   `json:"pending_payments"`
	LastUpdated     time.Time `json:"last_updated"`
}

// ActivityItem структура для элемента активности
type ActivityItem struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	UserID      string    `json:"userId,omitempty"` // Опциональное поле для совместимости с фронтендом
	UserName    string    `json:"userName"`         // Изменено с user_name на userName для совместимости
	ObjectName  string    `json:"object_name,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"` // Для дополнительных данных
}

// NotificationItem структура для уведомления
type NotificationItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// GetDashboardStats получает общую статистику для dashboard
func GetDashboardStats(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	stats := DashboardStats{
		LastUpdated: time.Now(),
	}

	// Подсчет объектов
	tenantDB.Model(&models.Object{}).Count(&stats.TotalObjects)
	tenantDB.Model(&models.Object{}).Where("status = ?", "active").Count(&stats.ActiveObjects)
	tenantDB.Model(&models.Object{}).Where("status = ?", "inactive").Count(&stats.InactiveObjects)

	// Подсчет пользователей
	tenantDB.Model(&models.User{}).Count(&stats.TotalUsers)

	// Подсчет контрактов
	tenantDB.Model(&models.Contract{}).Count(&stats.TotalContracts)
	tenantDB.Model(&models.Contract{}).Where("status = ?", "active").Count(&stats.ActiveContracts)

	// Примерные финансовые данные (можно дополнить реальной логикой)
	stats.MonthlyRevenue = 150000.0
	stats.PendingPayments = 25000.0

	c.JSON(200, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetDashboardActivity получает последнюю активность из реальных данных
// Поддерживает фильтрацию источников через query параметр sources (через запятую)
// Доступные источники: objects, users, invoices, contracts, installations, subscriptions
// Пример: /api/dashboard/activity?sources=objects,invoices&limit=20
func GetDashboardActivity(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		log.Printf("❌ GetDashboardActivity: tenantDB is nil для пользователя")
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}
	
	log.Printf("✅ GetDashboardActivity: tenantDB получен успешно")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit > 100 {
		limit = 100 // Увеличиваем максимум для большего охвата
	}

	// Получаем список разрешенных источников из query параметра
	sourcesParam := c.Query("sources")
	enabledSources := make(map[string]bool)
	if sourcesParam != "" {
		sources := strings.Split(sourcesParam, ",")
		for _, s := range sources {
			enabledSources[strings.TrimSpace(s)] = true
		}
	} else {
		// По умолчанию все источники включены
		enabledSources["objects"] = true
		enabledSources["users"] = true
		enabledSources["invoices"] = true
		enabledSources["contracts"] = true
		enabledSources["installations"] = true
		enabledSources["subscriptions"] = true
	}

	var activities []ActivityItem

	// 1. Получаем последние изменения объектов (создание, обновление)
	if enabledSources["objects"] {
		var objects []models.Object
		tenantDB.Model(&models.Object{}).
			Order("updated_at DESC").
			Limit(limit / 3). // Треть от лимита для объектов
			Find(&objects)

		for _, obj := range objects {
			activityType := "object_updated"
			title := "Обновлен объект"
			if obj.CreatedAt.Equal(obj.UpdatedAt) || obj.CreatedAt.After(obj.UpdatedAt.Add(-1*time.Second)) {
				activityType = "object_created"
				title = "Создан новый объект"
			}
			
			activities = append(activities, ActivityItem{
				ID:          fmt.Sprintf("obj_%d", obj.ID),
				Type:        activityType,
				Title:       title,
				Description: fmt.Sprintf("Объект '%s' %s", obj.Name, map[string]string{
					"object_created": "добавлен в систему",
					"object_updated": "был обновлен",
				}[activityType]),
				Timestamp: obj.UpdatedAt,
				UserID:    "system",
				UserName:  "Система",
				Metadata: map[string]interface{}{
					"objectId":   obj.ID,
					"objectName": obj.Name,
				},
			})
		}
	}

	// 2. Получаем последние созданные пользователи
	if enabledSources["users"] {
		var users []models.User
		tenantDB.Model(&models.User{}).
			Order("created_at DESC").
			Limit(limit / 3). // Треть от лимита для пользователей
			Find(&users)

		for _, user := range users {
			activities = append(activities, ActivityItem{
				ID:          fmt.Sprintf("user_%d", user.ID),
				Type:        "user_created",
				Title:       "Добавлен пользователь",
				Description: fmt.Sprintf("Новый пользователь '%s' зарегистрирован", user.Name),
				Timestamp:   user.CreatedAt,
				UserID:      fmt.Sprintf("user_%d", user.ID),
				UserName:    user.Name,
				Metadata: map[string]interface{}{
					"newUserId":   user.ID,
					"newUserName": user.Name,
				},
			})
		}
	}

	// 3. Получаем последние счета (из public схемы, так как invoices там)
	adminAccountID, err := middleware.GetAdminAccountID(c)
	if err == nil && enabledSources["invoices"] {
		// Используем public схему для счетов
		publicDB := database.DB.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err != nil {
			log.Printf("⚠️ Не удалось переключиться на public схему: %v", err)
		}

		var invoices []models.Invoice
		// Получаем больше счетов, чтобы важные события не терялись
		publicDB.Model(&models.Invoice{}).
			Where("admin_account_id = ? AND deleted_at IS NULL", adminAccountID).
			Order("created_at DESC"). // Сортируем по дате создания (новые первыми)
			Limit(limit * 2). // Берем в 2 раза больше, чтобы точно не потерять важные события
			Find(&invoices)

		for _, invoice := range invoices {
			activityType := "invoice_generated"
			title := "Создан счет"
			// Используем номер счета вместо ID
			description := fmt.Sprintf("Выставлен счет %s на сумму %.2f руб.", invoice.Number, invoice.TotalAmount.InexactFloat64())
			
			// Для новых счетов всегда используем created_at
			timestamp := invoice.CreatedAt
			
			// Если счет был оплачен, показываем как платеж с датой оплаты
			if invoice.Status == "paid" {
				activityType = "payment_received"
				title = "Получен платеж"
				description = fmt.Sprintf("Оплата по счету %s на сумму %.2f руб.", invoice.Number, invoice.TotalAmount.InexactFloat64())
				if invoice.PaidAt != nil {
					timestamp = *invoice.PaidAt
				} else if !invoice.UpdatedAt.IsZero() && invoice.UpdatedAt.After(invoice.CreatedAt) {
					// Если PaidAt не установлен, используем updated_at
					timestamp = invoice.UpdatedAt
				}
			} else if invoice.Status == "sent" && !invoice.UpdatedAt.IsZero() && invoice.UpdatedAt.After(invoice.CreatedAt) {
				// Если счет был отправлен, используем updated_at для отображения времени отправки
				timestamp = invoice.UpdatedAt
			}

			activities = append(activities, ActivityItem{
				ID:          fmt.Sprintf("inv_%d", invoice.ID),
				Type:        activityType,
				Title:       title,
				Description: description,
				Timestamp:   timestamp,
				UserID:      "billing_system",
				UserName:    "Биллинг система",
				Metadata: map[string]interface{}{
					"invoiceId":   invoice.ID,
					"invoiceNumber": invoice.Number,
					"amount":      invoice.TotalAmount.InexactFloat64(),
				},
			})
		}
	}

	// 4. Получаем последние договоры
	if enabledSources["contracts"] {
		var contracts []models.Contract
		tenantDB.Model(&models.Contract{}).
			Where("admin_account_id = ?", adminAccountID).
			Order("updated_at DESC, created_at DESC").
			Limit(limit / 3). // Треть от лимита для договоров
			Find(&contracts)

		for _, contract := range contracts {
			activityType := "contract_updated"
			title := "Обновлен договор"
			if contract.CreatedAt.Equal(contract.UpdatedAt) || contract.CreatedAt.After(contract.UpdatedAt.Add(-1*time.Second)) {
				activityType = "contract_created"
				title = "Создан договор"
			}

			activities = append(activities, ActivityItem{
				ID:          fmt.Sprintf("contract_%d", contract.ID),
				Type:        activityType,
				Title:       title,
				Description: fmt.Sprintf("Договор %s '%s' %s", contract.Number, contract.ClientName, map[string]string{
					"contract_created": "создан",
					"contract_updated": "обновлен",
				}[activityType]),
				Timestamp: contract.UpdatedAt,
				UserID:    "system",
				UserName:  "Система",
				Metadata: map[string]interface{}{
					"contractId":   contract.ID,
					"contractNumber": contract.Number,
					"clientName":   contract.ClientName,
				},
			})
		}
	}

	// 5. Получаем последние монтажи
	if enabledSources["installations"] {
		var installations []models.Installation
		tenantDB.Model(&models.Installation{}).
			Preload("Object").
			Preload("Installer").
			Order("updated_at DESC, created_at DESC").
			Limit(limit / 3). // Треть от лимита для монтажей
			Find(&installations)

		for _, inst := range installations {
			activityType := "installation_scheduled"
			title := "Запланирован монтаж"
			description := fmt.Sprintf("Монтаж на объекте")

			if inst.Object != nil {
				description = fmt.Sprintf("Монтаж оборудования на объекте '%s'", inst.Object.Name)
			}

			// Определяем тип активности по статусу
			switch inst.Status {
			case "completed":
				activityType = "installation_completed"
				title = "Завершен монтаж"
				if inst.CompletedAt != nil {
					description = fmt.Sprintf("Монтаж на объекте '%s' завершен", inst.Object.Name)
				}
			case "in_progress":
				activityType = "installation_started"
				title = "Начат монтаж"
				if inst.StartedAt != nil {
					description = fmt.Sprintf("Монтаж на объекте '%s' начат", inst.Object.Name)
				}
			case "cancelled":
				activityType = "installation_cancelled"
				title = "Отменен монтаж"
				description = fmt.Sprintf("Монтаж на объекте '%s' отменен", inst.Object.Name)
			}

			timestamp := inst.UpdatedAt
			if inst.CompletedAt != nil && inst.Status == "completed" {
				timestamp = *inst.CompletedAt
			} else if inst.StartedAt != nil && inst.Status == "in_progress" {
				timestamp = *inst.StartedAt
			} else if inst.CreatedAt.Equal(inst.UpdatedAt) || inst.CreatedAt.After(inst.UpdatedAt.Add(-1*time.Second)) {
				timestamp = inst.CreatedAt
			}

			activities = append(activities, ActivityItem{
				ID:          fmt.Sprintf("inst_%d", inst.ID),
				Type:        activityType,
				Title:       title,
				Description: description,
				Timestamp:   timestamp,
				UserID:      "system",
				UserName:    "Система",
				Metadata: map[string]interface{}{
					"installationId": inst.ID,
					"objectId":      inst.ObjectID,
					"objectName":    inst.Object.Name,
					"status":        inst.Status,
				},
			})
		}
	}

	// 6. Получаем последние подписки (из public схемы)
	if enabledSources["subscriptions"] && adminAccountID > 0 {
		publicDB := database.DB.Session(&gorm.Session{})
		if err := publicDB.Exec("SET search_path TO public").Error; err == nil {
			var subscriptions []models.Subscription
			publicDB.Model(&models.Subscription{}).
				Where("admin_account_id = ? AND deleted_at IS NULL", adminAccountID).
				Preload("BillingPlan").
				Order("updated_at DESC, created_at DESC").
				Limit(limit / 4). // Четверть от лимита для подписок
				Find(&subscriptions)

			for _, sub := range subscriptions {
				activityType := "subscription_created"
				title := "Создана подписка"
				description := fmt.Sprintf("Подписка на тариф '%s'", sub.BillingPlan.Name)

				if sub.Status == "cancelled" {
					activityType = "subscription_cancelled"
					title = "Отменена подписка"
					description = fmt.Sprintf("Подписка на тариф '%s' отменена", sub.BillingPlan.Name)
				} else if sub.Status == "active" && !sub.CreatedAt.Equal(sub.UpdatedAt) {
					activityType = "subscription_updated"
					title = "Обновлена подписка"
					description = fmt.Sprintf("Подписка на тариф '%s' обновлена", sub.BillingPlan.Name)
				}

				timestamp := sub.CreatedAt
				if !sub.UpdatedAt.IsZero() && sub.UpdatedAt.After(sub.CreatedAt) {
					timestamp = sub.UpdatedAt
				}

				activities = append(activities, ActivityItem{
					ID:          fmt.Sprintf("sub_%d", sub.ID),
					Type:        activityType,
					Title:       title,
					Description: description,
					Timestamp:   timestamp,
					UserID:      "billing_system",
					UserName:    "Биллинг система",
					Metadata: map[string]interface{}{
						"subscriptionId": sub.ID,
						"planId":         sub.BillingPlanID,
						"planName":       sub.BillingPlan.Name,
						"status":         sub.Status,
					},
				})
			}
		}
	}

	// Сортируем по времени (новые первыми) и ограничиваем лимитом
	sort.Slice(activities, func(i, j int) bool {
		return activities[i].Timestamp.After(activities[j].Timestamp)
	})

	if len(activities) > limit {
		activities = activities[:limit]
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   activities,
	})
}

// GetDashboardNotifications получает уведомления для dashboard
func GetDashboardNotifications(c *gin.Context) {
	tenantDB := middleware.GetTenantDB(c)
	if tenantDB == nil {
		c.JSON(500, gin.H{"status": "error", "error": "Ошибка подключения к базе данных компании"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	unreadOnly := c.Query("unread_only") == "true"

	var notifications []NotificationItem

	// Примерные уведомления (можно дополнить реальной логикой)
	query := tenantDB.Model(&models.NotificationLog{}).Order("created_at DESC").Limit(limit)

	if unreadOnly {
		query = query.Where("is_read = ?", false)
	}

	var dbNotifications []models.NotificationLog
	query.Find(&dbNotifications)

	for _, notif := range dbNotifications {
		notifications = append(notifications, NotificationItem{
			ID:        strconv.FormatUint(uint64(notif.ID), 10),
			Type:      notif.Type,
			Title:     notif.Subject,
			Message:   notif.Message,
			IsRead:    notif.Status == "sent",
			CreatedAt: notif.CreatedAt,
		})
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   notifications,
	})
}

// GetDashboardLayouts получает макеты dashboard
func GetDashboardLayouts(c *gin.Context) {
	// Пока возвращаем пустой массив, так как макеты не реализованы
	c.JSON(200, gin.H{
		"status": "success",
		"data":   []interface{}{},
	})
}

// GetDefaultDashboardLayout получает макет по умолчанию
func GetDefaultDashboardLayout(c *gin.Context) {
	// Возвращаем базовый макет
	defaultLayout := map[string]interface{}{
		"id":      "default",
		"name":    "По умолчанию",
		"widgets": []interface{}{},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   defaultLayout,
	})
}

// Упрощенные функции без мультитенантности для временного решения

// GetDashboardStatsSimple получает упрощенную статистику без tenant
func GetDashboardStatsSimple(c *gin.Context) {
	stats := DashboardStats{
		TotalObjects:    42,
		ActiveObjects:   38,
		InactiveObjects: 4,
		TotalUsers:      12,
		TotalContracts:  15,
		ActiveContracts: 13,
		MonthlyRevenue:  150000.0,
		PendingPayments: 25000.0,
		LastUpdated:     time.Now(),
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   stats,
	})
}

// GetDashboardActivitySimple получает упрощенную активность (использует реальные данные)
func GetDashboardActivitySimple(c *gin.Context) {
	// Используем реальную функцию GetDashboardActivity
	GetDashboardActivity(c)
}

// GetDashboardNotificationsSimple получает упрощенные уведомления
func GetDashboardNotificationsSimple(c *gin.Context) {
	notifications := []NotificationItem{
		{
			ID:        "1",
			Type:      "info",
			Title:     "Добро пожаловать",
			Message:   "Добро пожаловать в систему Axenta CRM",
			IsRead:    false,
			CreatedAt: time.Now().Add(-30 * time.Minute),
		},
		{
			ID:        "2",
			Type:      "success",
			Title:     "Система готова",
			Message:   "Все сервисы работают нормально",
			IsRead:    true,
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	c.JSON(200, gin.H{
		"status": "success",
		"data":   notifications,
	})
}

// BillingDashboardResponse структура ответа для /api/dashboard?company_id= согласно roadmap
type BillingDashboardResponse struct {
	RevenueTotal      decimal.Decimal `json:"revenue_total"`      // Общий доход
	SubscriptionsActive int64         `json:"subscriptions_active"` // Активные подписки
	Payable           decimal.Decimal `json:"payable"`            // К оплате
	Overdue           decimal.Decimal `json:"overdue"`            // Просрочено
}

// GetBillingDashboard получает статистику биллинга для dashboard согласно roadmap
// GET /api/dashboard?company_id=&demo=1
func GetBillingDashboard(c *gin.Context) {
	// Проверяем demo-режим
	if isDemoMode(c) {
		demoResponse := BillingDashboardResponse{
			RevenueTotal:        decimal.NewFromInt(50000),
			SubscriptionsActive: 1,
			Payable:             decimal.NewFromInt(8000),
			Overdue:             decimal.NewFromInt(2000),
		}

		c.JSON(200, gin.H{
			"status":       "success",
			"data":         demoResponse,
			"demo_notice":  "Это демо-данные. Добавьте ?demo=0 для получения реальных данных.",
		})
		return
	}

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  "Параметр company_id обязателен",
		})
		return
	}

	companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
	if err != nil {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  "Неверный формат company_id",
		})
		return
	}

	response := BillingDashboardResponse{
		RevenueTotal:        decimal.Zero,
		SubscriptionsActive: 0,
		Payable:             decimal.Zero,
		Overdue:             decimal.Zero,
	}

	// Подсчитываем активные подписки
	var activeSubscriptions int64
	database.DB.Model(&models.Subscription{}).
		Where("company_id = ? AND status = ? AND deleted_at IS NULL", companyID, "active").
		Count(&activeSubscriptions)
	response.SubscriptionsActive = activeSubscriptions

	// Подсчитываем общий доход (из оплаченных счетов)
	var paidInvoices []models.Invoice
	database.DB.Model(&models.Invoice{}).
		Where("company_id = ? AND status = ? AND deleted_at IS NULL", companyID, "paid").
		Find(&paidInvoices)
	
	for _, invoice := range paidInvoices {
		response.RevenueTotal = response.RevenueTotal.Add(invoice.TotalAmount)
	}

	// Подсчитываем к оплате (неоплаченные счета, не просроченные)
	var payableInvoices []models.Invoice
	database.DB.Model(&models.Invoice{}).
		Where("company_id = ? AND status IN (?, ?) AND deleted_at IS NULL AND due_date >= ?", 
			companyID, "sent", "draft", time.Now()).
		Find(&payableInvoices)
	
	for _, invoice := range payableInvoices {
		response.Payable = response.Payable.Add(invoice.GetRemainingAmount())
	}

	// Подсчитываем просроченные
	var overdueInvoices []models.Invoice
	database.DB.Model(&models.Invoice{}).
		Where("company_id = ? AND status != ? AND status != ? AND deleted_at IS NULL AND due_date < ?", 
			companyID, "paid", "cancelled", time.Now()).
		Find(&overdueInvoices)
	
	for _, invoice := range overdueInvoices {
		if invoice.IsOverdue() {
			response.Overdue = response.Overdue.Add(invoice.GetRemainingAmount())
		}
	}

	c.JSON(200, response)
}
