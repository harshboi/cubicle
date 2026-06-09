package httpapi

import (
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"

	ontologygraphql "cubicle/services/ontology-service/internal/graphql"
	"cubicle/services/ontology-service/internal/graphql/generated"
)

// registerGraphQL mounts Cubicle's GraphQL API and local development playground.
func registerGraphQL(router gin.IRouter, options RouterOptions) {
	executableSchema := generated.NewExecutableSchema(generated.Config{
		Resolvers: &ontologygraphql.Resolver{},
	})
	server := handler.NewDefaultServer(executableSchema)

	router.POST("/graphql", gin.WrapH(server))
	if options.GraphQLPlaygroundEnabled {
		router.GET("/playground", gin.WrapH(playground.Handler("Cubicle Ontology GraphQL", "/graphql")))
	}
}
