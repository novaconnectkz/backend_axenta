package api

import (
	"fmt"
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

// GetTelegramIntegrationDocs возвращает документацию по настройке Telegram интеграции
func GetTelegramIntegrationDocs(c *gin.Context) {
	// Пробуем несколько возможных путей
	possiblePaths := []string{
		filepath.Join(".", "docs", "TELEGRAM_INTEGRATION.md"),
		filepath.Join("docs", "TELEGRAM_INTEGRATION.md"),
		filepath.Join("backend_axenta", "docs", "TELEGRAM_INTEGRATION.md"),
	}

	var data []byte
	var err error

	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		// Логируем ошибку для отладки
		c.JSON(http.StatusNotFound, gin.H{
			"status":    "error",
			"error":     "Документация не найдена",
			"details":   "Проверенные пути: " + fmt.Sprintf("%v", possiblePaths),
			"error_msg": err.Error(),
		})
		return
	}

	// Отдаем как HTML с Markdown стилизацией для лучшего отображения
	html := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Инструкция по настройке Telegram Bot</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 900px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .container {
            background: white;
            padding: 40px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 {
            color: #1976d2;
            border-bottom: 3px solid #1976d2;
            padding-bottom: 10px;
        }
        h2 {
            color: #424242;
            margin-top: 30px;
            border-bottom: 1px solid #e0e0e0;
            padding-bottom: 5px;
        }
        h3 {
            color: #616161;
            margin-top: 20px;
        }
        code {
            background: #f5f5f5;
            padding: 2px 6px;
            border-radius: 3px;
            font-family: 'Courier New', monospace;
            font-size: 0.9em;
        }
        pre {
            background: #f5f5f5;
            padding: 15px;
            border-radius: 5px;
            overflow-x: auto;
            border-left: 4px solid #1976d2;
        }
        pre code {
            background: none;
            padding: 0;
        }
        a {
            color: #1976d2;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        ul, ol {
            padding-left: 25px;
        }
        li {
            margin: 8px 0;
        }
        blockquote {
            border-left: 4px solid #1976d2;
            padding-left: 15px;
            margin-left: 0;
            color: #666;
            font-style: italic;
        }
        strong {
            color: #424242;
        }
        .back-link {
            display: inline-block;
            margin-top: 30px;
            padding: 10px 20px;
            background: #1976d2;
            color: white;
            border-radius: 5px;
            text-decoration: none;
        }
        .back-link:hover {
            background: #1565c0;
            text-decoration: none;
        }
    </style>
</head>
<body>
    <div class="container">
        <pre style="white-space: pre-wrap; font-family: inherit;">` + string(data) + `</pre>
        <a href="javascript:window.close()" class="back-link">Закрыть</a>
    </div>
</body>
</html>`

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
