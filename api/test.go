package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

func TestAxentaAPI(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": "Invalid request"})
		return
	}

	log.Printf("TestAPI: Testing with user: %s", req.Username)

	// Step 1: Test login
	axentaReq, _ := json.Marshal(req)
	resp, err := http.Post("https://axenta.cloud/api/auth/login/", "application/json", bytes.NewBuffer(axentaReq))
	if err != nil {
		log.Printf("TestAPI: Connection error: %v", err)
		c.JSON(500, gin.H{"status": "error", "error": "Connection failed", "details": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("TestAPI: Login status: %d", resp.StatusCode)
	log.Printf("TestAPI: Login response: %s", string(body))

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{
			"status":          "error",
			"error":           "Axenta login failed",
			"axenta_status":   resp.StatusCode,
			"axenta_response": string(body),
		})
		return
	}

	// Parse login response
	var loginResp map[string]interface{}
	if err := json.Unmarshal(body, &loginResp); err != nil {
		log.Printf("TestAPI: Failed to parse login response: %v", err)
		c.JSON(500, gin.H{"status": "error", "error": "Parse error", "raw_response": string(body)})
		return
	}

	// Extract token
	token, ok := loginResp["token"].(string)
	if !ok {
		log.Printf("TestAPI: No token found in response")
		c.JSON(500, gin.H{"status": "error", "error": "No token", "response": loginResp})
		return
	}

	log.Printf("TestAPI: Got token: %s", token[:10]+"...")

	// Step 2: Test user data fetch
	client := &http.Client{}
	userReq, _ := http.NewRequest("GET", "https://axenta.cloud/api/current_user/", nil)
	userReq.Header.Set("Authorization", "Bearer "+token)

	userResp, err := client.Do(userReq)
	if err != nil {
		log.Printf("TestAPI: User request error: %v", err)
		c.JSON(500, gin.H{"status": "error", "error": "User fetch failed", "details": err.Error()})
		return
	}
	defer userResp.Body.Close()

	userBody, _ := io.ReadAll(userResp.Body)
	log.Printf("TestAPI: User status: %d", userResp.StatusCode)
	log.Printf("TestAPI: User response: %s", string(userBody))

	c.JSON(200, gin.H{
		"status":         "success",
		"login_status":   resp.StatusCode,
		"login_response": loginResp,
		"user_status":    userResp.StatusCode,
		"user_response":  string(userBody),
		"token_preview":  token[:10] + "...",
	})
}
