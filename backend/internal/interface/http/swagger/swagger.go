// Package swagger serve a documentação OpenAPI da API.
//
// A spec (openapi.yaml) é embutida no binário via go:embed, então não há dependência
// de arquivos em disco nem de bibliotecas externas de geração. A interface Swagger UI
// é carregada de uma CDN e aponta para a spec servida localmente.
package swagger

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed openapi.yaml
var specYAML []byte

// specPath é o caminho onde a spec é servida (referenciado pelo Swagger UI).
const specPath = "/openapi.yaml"

// uiHTML é a página do Swagger UI. Os assets vêm da CDN; a spec vem de specPath.
const uiHTML = `<!DOCTYPE html>
<html lang="pt-br">
<head>
  <meta charset="UTF-8">
  <title>Secullum Compliance API — Swagger</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "` + specPath + `",
        dom_id: "#swagger-ui",
        deepLinking: true
      });
    };
  </script>
</body>
</html>`

// SpecHandler serve o arquivo openapi.yaml embutido.
func SpecHandler(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", specYAML)
}

// UIHandler serve a página HTML do Swagger UI.
func UIHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(uiHTML))
}

// Register registra as rotas da documentação no engine informado.
func Register(router *gin.Engine) {
	router.GET(specPath, SpecHandler)
	router.GET("/swagger", UIHandler)
}
