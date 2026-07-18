package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const scalarAPIReferenceHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Orion X Manager API</title>
</head>
<body>
  <script
    id="api-reference"
    data-url="/swagger/doc.json"
    data-configuration='{"layout":"modern","hideDownloadButton":false,"hideModels":false}'
  ></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

func apiDocs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarAPIReferenceHTML))
}
