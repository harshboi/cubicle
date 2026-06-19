package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RouterOptions controls process-level HTTP behavior.
type RouterOptions struct {
	GraphQLPlaygroundEnabled bool // GraphQLPlaygroundEnabled mounts GET /playground for local development.
}

// NewRouter wires the HTTP framework layer.
//
// Gin owns process-level HTTP concerns. GraphQL owns the product contract, so
// REST stays limited to health and local developer ergonomics.
func NewRouter(logger *slog.Logger) http.Handler {
	return NewRouterWithOptions(logger, RouterOptions{
		GraphQLPlaygroundEnabled: true,
	})
}

// NewRouterWithOptions wires the HTTP framework layer with explicit runtime
// settings from service config.
func NewRouterWithOptions(logger *slog.Logger, options RouterOptions) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	registerHealth(router)
	registerGraphQL(router, options)

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
			"path", requestPath(c),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
}

// requestPath returns the route pattern for matched routes and the URL path for
// unmatched routes where Gin has no route pattern.
func requestPath(c *gin.Context) string {
	if path := c.FullPath(); path != "" {
		return path
	}
	if c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}
