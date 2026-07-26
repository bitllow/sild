package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /docs renders the generated spec for a human. Non-production only: the page
// pulls its renderer from a CDN, which is fine for a dev tool and not something a
// production deployment should serve.
//
// The spec itself (/openapi.json) is always served — clients and codegen need it.

// docsAvailable gates the rendered page on a non-production deployment.
func (h *Handler) docsAvailable() bool { return h.cfg.Env != "production" }

const docsPage = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Sild API</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements@8/styles.min.css">
  <style>html,body{margin:0;height:100%}</style>
</head>
<body>
  <script src="https://unpkg.com/@stoplight/elements@8/web-components.min.js"></script>
  <elements-api apiDescriptionUrl="/openapi.json" router="hash" layout="sidebar"></elements-api>
</body>
</html>`

func (h *Handler) serveDocs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(docsPage))
}
