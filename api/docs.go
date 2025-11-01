package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// GetSwaggerUI отдает HTML страницу с Swagger UI для просмотра OpenAPI документации
func GetSwaggerUI(c *gin.Context) {
	// Определяем какой файл показывать
	specFile := c.DefaultQuery("spec", "billing")
	
	// Определяем URL для спецификации
	var specURL string
	switch specFile {
	case "billing":
		specURL = "/api/docs/billing-openapi.yaml"
	case "main":
		specURL = "/api/docs/openapi.yaml"
	default:
		specURL = "/api/docs/billing-openapi.yaml"
	}

	// HTML страница с встроенным Swagger UI через CDN
	html := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <title>Axenta API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui.css" />
    <style>
        html {
            box-sizing: border-box;
            overflow: -moz-scrollbars-vertical;
            overflow-y: scroll;
        }
        *, *:before, *:after {
            box-sizing: inherit;
        }
        body {
            margin:0;
            background: #fafafa;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.9.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            const ui = SwaggerUIBundle({
                url: "` + specURL + `",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout",
                validatorUrl: null,
                defaultModelsExpandDepth: 1,
                defaultModelExpandDepth: 1,
                docExpansion: "list",
                filter: true,
                showExtensions: true,
                showCommonExtensions: true
            });
        };
    </script>
</body>
</html>`

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// GetOpenAPISpec возвращает основную OpenAPI спецификацию
func GetOpenAPISpec(c *gin.Context) {
	specPath := filepath.Join(".", "openapi.yaml")
	
	data, err := os.ReadFile(specPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "OpenAPI спецификация не найдена",
		})
		return
	}

	c.Data(http.StatusOK, "application/x-yaml", data)
}

// GetBillingOpenAPISpec возвращает биллинговую OpenAPI спецификацию
func GetBillingOpenAPISpec(c *gin.Context) {
	specPath := filepath.Join(".", "configs", "billing-openapi.yaml")
	
	data, err := os.ReadFile(specPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": "error",
			"error":  "Биллинговая OpenAPI спецификация не найдена",
		})
		return
	}

	c.Data(http.StatusOK, "application/x-yaml", data)
}

