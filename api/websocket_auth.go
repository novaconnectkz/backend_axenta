package api

import (
	"backend_axenta/services"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WebSocketAuthAPI API для WebSocket с авторизацией
type WebSocketAuthAPI struct {
	jwtService *services.JWTService
	upgrader   websocket.Upgrader
}

// NewWebSocketAuthAPI создает новый API для WebSocket с авторизацией
func NewWebSocketAuthAPI(jwtService *services.JWTService) *WebSocketAuthAPI {
	return &WebSocketAuthAPI{
		jwtService: jwtService,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// В продакшене здесь должна быть проверка origin
				return true
			},
		},
	}
}

// LiveDataConnection представляет WebSocket соединение с авторизацией
type LiveDataConnection struct {
	conn      *websocket.Conn
	userID    uint
	companyID string
	role      string
	username  string
}

// LiveData обрабатывает WebSocket соединения с авторизацией
func (api *WebSocketAuthAPI) LiveData(c *gin.Context) {
	companyID := c.Param("company_id")
	if companyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Company ID is required",
		})
		return
	}

	// Получаем токен из query параметра или заголовка
	token := c.Query("token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
			}
		}
	}

	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Token is required for WebSocket connection",
		})
		return
	}

	// Валидируем токен
	claims, err := api.jwtService.ValidateAccessToken(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status": "error",
			"error":  "Invalid or expired token",
		})
		return
	}

	// Проверяем принадлежность к компании
	if claims.CompanyID != companyID {
		c.JSON(http.StatusForbidden, gin.H{
			"status": "error",
			"error":  "Access denied for this company",
		})
		return
	}

	// Апгрейдим соединение до WebSocket
	conn, err := api.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		return
	}
	defer conn.Close()

	// Создаем соединение с авторизацией
	wsConn := &LiveDataConnection{
		conn:      conn,
		userID:    claims.UserID,
		companyID: claims.CompanyID,
		role:      claims.Role,
		username:  claims.Username,
	}

	log.Printf("WebSocket connection established for user %s (ID: %d) in company %s",
		wsConn.username, wsConn.userID, wsConn.companyID)

	// Отправляем приветственное сообщение
	if err := wsConn.SendMessage("welcome", map[string]interface{}{
		"message":    "Connected to live data stream",
		"user_id":    wsConn.userID,
		"company_id": wsConn.companyID,
		"role":       wsConn.role,
	}); err != nil {
		log.Printf("Failed to send welcome message: %v", err)
		return
	}

	// Обрабатываем сообщения
	api.handleWebSocketMessages(wsConn)
}

// SendMessage отправляет сообщение через WebSocket
func (conn *LiveDataConnection) SendMessage(messageType string, data interface{}) error {
	message := map[string]interface{}{
		"type":      messageType,
		"data":      data,
		"timestamp": "2025-01-27T12:00:00Z", // В реальном приложении использовать time.Now()
	}

	return conn.conn.WriteJSON(message)
}

// handleWebSocketMessages обрабатывает входящие WebSocket сообщения
func (api *WebSocketAuthAPI) handleWebSocketMessages(conn *LiveDataConnection) {
	for {
		var message map[string]interface{}
		err := conn.conn.ReadJSON(&message)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for user %d: %v", conn.userID, err)
			}
			break
		}

		// Обрабатываем различные типы сообщений
		messageType, ok := message["type"].(string)
		if !ok {
			_ = conn.SendMessage("error", map[string]interface{}{
				"message": "Invalid message format",
			})
			continue
		}

		switch messageType {
		case "ping":
			_ = conn.SendMessage("pong", map[string]interface{}{
				"message": "Connection alive",
			})

		case "subscribe":
			api.handleSubscription(conn, message)

		case "unsubscribe":
			api.handleUnsubscription(conn, message)

		default:
			_ = conn.SendMessage("error", map[string]interface{}{
				"message": "Unknown message type: " + messageType,
			})
		}
	}

	log.Printf("WebSocket connection closed for user %d", conn.userID)
}

// handleSubscription обрабатывает подписку на данные
func (api *WebSocketAuthAPI) handleSubscription(conn *LiveDataConnection, message map[string]interface{}) {
	data, ok := message["data"].(map[string]interface{})
	if !ok {
		conn.SendMessage("error", map[string]interface{}{
			"message": "Invalid subscription data",
		})
		return
	}

	channel, ok := data["channel"].(string)
	if !ok {
		conn.SendMessage("error", map[string]interface{}{
			"message": "Channel is required for subscription",
		})
		return
	}

	// Проверяем права доступа к каналу
	if !api.canAccessChannel(conn, channel) {
		conn.SendMessage("error", map[string]interface{}{
			"message": "Access denied to channel: " + channel,
		})
		return
	}

	// В реальном приложении здесь была бы логика подписки
	log.Printf("User %d subscribed to channel: %s", conn.userID, channel)

	conn.SendMessage("subscribed", map[string]interface{}{
		"channel": channel,
		"message": "Successfully subscribed to " + channel,
	})

	// Отправляем тестовые данные
	go api.sendTestData(conn, channel)
}

// handleUnsubscription обрабатывает отписку от данных
func (api *WebSocketAuthAPI) handleUnsubscription(conn *LiveDataConnection, message map[string]interface{}) {
	data, ok := message["data"].(map[string]interface{})
	if !ok {
		conn.SendMessage("error", map[string]interface{}{
			"message": "Invalid unsubscription data",
		})
		return
	}

	channel, ok := data["channel"].(string)
	if !ok {
		conn.SendMessage("error", map[string]interface{}{
			"message": "Channel is required for unsubscription",
		})
		return
	}

	// В реальном приложении здесь была бы логика отписки
	log.Printf("User %d unsubscribed from channel: %s", conn.userID, channel)

	conn.SendMessage("unsubscribed", map[string]interface{}{
		"channel": channel,
		"message": "Successfully unsubscribed from " + channel,
	})
}

// canAccessChannel проверяет права доступа к каналу
func (api *WebSocketAuthAPI) canAccessChannel(conn *LiveDataConnection, channel string) bool {
	switch channel {
	case "objects", "devices":
		// Все авторизованные пользователи могут подписаться на объекты и устройства
		return true
	case "admin":
		// Только админы могут подписаться на админский канал
		return conn.role == "admin"
	case "warehouse":
		// Менеджеры, админы и техники могут подписаться на склад
		return conn.role == "admin" || conn.role == "manager" || conn.role == "tech"
	default:
		return false
	}
}

// sendTestData отправляет тестовые данные в канал
func (api *WebSocketAuthAPI) sendTestData(conn *LiveDataConnection, channel string) {
	// Отправляем тестовое сообщение через 2 секунды
	// В реальном приложении здесь была бы логика получения реальных данных

	testData := map[string]interface{}{
		"channel": channel,
		"message": "Test data for " + channel,
		"user_id": conn.userID,
		"data": map[string]interface{}{
			"example": "This is test data",
			"count":   42,
		},
	}

	conn.SendMessage("data", testData)
}

// RegisterRoutes регистрирует WebSocket маршруты
func (api *WebSocketAuthAPI) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/live-data/:company_id", api.LiveData)
}
