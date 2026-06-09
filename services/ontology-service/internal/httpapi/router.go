package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"

	"cubicle/services/ontology-service/internal/graphstore"
)

// NewRouter wires the HTTP framework layer.
//
// The split is deliberate:
//   - Gin handles server mechanics such as middleware, recovery, and routing.
//   - Huma registers typed operations and generates the OpenAPI contract.
//   - graphstore.Expander is the only graph behavior this first API slice uses.
//
// That shape keeps framework code thin and prevents product/query logic from
// being embedded inside Gin handlers.
func NewRouter(store graphstore.Expander, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	config := huma.DefaultConfig("Cubicle Ontology Service", "0.1.0")
	config.OpenAPIPath = "/openapi"
	config.DocsPath = "/docs"
	config.Servers = []*huma.Server{{URL: "http://127.0.0.1:48080"}}
	// Huma's default create hook adds schema links into response bodies. That is
	// useful for browsable APIs, but Cubicle's Swift contract should stay as
	// plain DTO JSON: no transport metadata mixed into product responses.
	config.CreateHooks = nil
	config.Transformers = nil

	api := humagin.New(router, config)
	registerHealth(api)
	registerGraph(api, store)

	return router
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		logger.InfoContext(
			c.Request.Context(),
			"ontology_http_request",
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
}
