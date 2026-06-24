package httpapi

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"

	"cubicle/services/ontology-service/internal/entgraph"
	ontologygraphql "cubicle/services/ontology-service/internal/graphql"
	"cubicle/services/ontology-service/internal/graphql/generated"
)

// registerGraphQL mounts Cubicle's GraphQL API and local development playground.
func registerGraphQL(router gin.IRouter, options RouterOptions) {
	graphExpander := options.GraphExpander
	if graphExpander == nil && options.EntClient != nil {
		graphExpander = entgraph.NewRoutedExpander(options.EntClient)
	}
	executableSchema := generated.NewExecutableSchema(generated.Config{
		Resolvers: &ontologygraphql.Resolver{
			EntClient:                   options.EntClient,
			GraphExpander:               graphExpander,
			BoundedGraphSourceAuthority: options.BoundedGraphSourceAuthority,
		},
	})
	server := handler.NewDefaultServer(executableSchema)

	handlers := []gin.HandlerFunc{}
	if options.BoundedGraphPrincipalAccessProvider != nil {
		handlers = append(handlers, boundedGraphPrincipalAccessMiddleware(options.BoundedGraphPrincipalAccessProvider))
	}
	handlers = append(handlers, gin.WrapH(server))
	router.POST("/graphql", handlers...)
	if options.GraphQLPlaygroundEnabled {
		router.GET("/playground", gin.WrapH(playground.Handler("Cubicle Ontology GraphQL", "/graphql")))
	}
}

func boundedGraphPrincipalAccessMiddleware(provider func(*http.Request) (ontologygraphql.BoundedGraphPrincipalAccess, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil {
			if access, ok := provider(c.Request); ok {
				ctx := ontologygraphql.WithBoundedGraphPrincipalAccess(c.Request.Context(), access)
				c.Request = c.Request.WithContext(ctx)
			}
		}
		c.Next()
	}
}
