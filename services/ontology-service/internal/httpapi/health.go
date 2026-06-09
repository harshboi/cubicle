package httpapi

import "github.com/gin-gonic/gin"

// registerHealth mounts the process health endpoint outside GraphQL.
func registerHealth(router gin.IRouter) {
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, HealthResponse{
			OK:      true,
			Service: "ontology-service",
		})
	})
}
