package graphql

import "cubicle/services/ontology-service/internal/graphcontext"

func boundedGraphSourceAuthorityPolicy() graphcontext.SourceAuthorityPolicy {
	return graphcontext.DefaultSourceAuthorityPolicy()
}
