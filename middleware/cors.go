package middleware

import (
	"log"
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
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		path := c.Request.URL.Path

		// Check if origin is allowed
		originAllowed := false
		if origin != "" {
			for _, allowedOrigin := range config.AllowOrigins {
				if origin == allowedOrigin {
					originAllowed = true
					break
				}
			}
		}

		// Log CORS request for debugging
		if !originAllowed && origin != "" {
			log.Printf("⚠️ CORS: Origin not allowed - Origin: %s, Method: %s, Path: %s", origin, method, path)
		}

		// Set CORS headers for all requests (both preflight and actual)
		if originAllowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			log.Printf("✅ CORS: Headers set - Origin: %s, Method: %s, Path: %s", origin, method, path)
		}

		c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ","))

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			log.Printf("🔍 CORS: Handling OPTIONS preflight - Origin: %s, Path: %s", origin, path)

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
			log.Printf("✅ CORS: OPTIONS handled successfully - Origin: %s, Path: %s", origin, path)
			return
		}

		// For non-preflight requests, set allow and expose headers
		c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ","))
		if len(config.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ","))
		}

		c.Next()
	}
}
