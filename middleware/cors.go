package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CustomCORSConfig represents CORS configuration
type CustomCORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// CustomCORS creates a custom CORS middleware
func CustomCORS(config CustomCORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		if origin != "" {
			allowed := false
			for _, allowedOrigin := range config.AllowOrigins {
				if origin == allowedOrigin {
					allowed = true
					break
				}
			}
			if allowed {
				c.Header("Access-Control-Allow-Origin", origin)
			}
		}

		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ","))

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			// Get requested headers from Access-Control-Request-Headers
			requestedHeaders := c.Request.Header.Get("Access-Control-Request-Headers")

			// Check if x-tenant-id is in requested headers
			if strings.Contains(strings.ToLower(requestedHeaders), "x-tenant-id") {
				// Add x-tenant-id to allowed headers
				headers := make([]string, len(config.AllowHeaders))
				copy(headers, config.AllowHeaders)
				headers = append(headers, "x-tenant-id")
				c.Header("Access-Control-Allow-Headers", strings.Join(headers, ","))
			} else {
				c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ","))
			}

			c.Header("Access-Control-Max-Age", "43200") // 12 hours
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// For non-preflight requests, set expose headers
		if len(config.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ","))
		}

		c.Next()
	}
}
