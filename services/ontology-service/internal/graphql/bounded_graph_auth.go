package graphql

import (
	"context"
	"strings"

	"cubicle/services/ontology-service/internal/domain"
)

type boundedGraphPrincipalAccessContextKey struct{}

// BoundedGraphPrincipalAccess is the transport-neutral auth input used by
// boundedGraphContext before workplace-specific ACL providers exist.
//
// The visibility classes are intentionally coarse. Source connectors can later
// translate real ACLs into this read filter without changing the graph traversal
// contract.
type BoundedGraphPrincipalAccess struct {
	PrincipalKey                 string
	GroupKeys                    []string
	AllowedVisibilityClasses     []string
	CoverageCompleteForPrincipal bool
}

// WithBoundedGraphPrincipalAccess attaches principal visibility access to a
// request context. HTTP middleware or tests can use this without changing the
// public GraphQL schema.
func WithBoundedGraphPrincipalAccess(ctx context.Context, access BoundedGraphPrincipalAccess) context.Context {
	return context.WithValue(ctx, boundedGraphPrincipalAccessContextKey{}, access)
}

// BoundedGraphPrincipalAccessFromContext returns the principal access attached
// to a request context.
func BoundedGraphPrincipalAccessFromContext(ctx context.Context) (BoundedGraphPrincipalAccess, bool) {
	access, ok := ctx.Value(boundedGraphPrincipalAccessContextKey{}).(BoundedGraphPrincipalAccess)
	return access, ok
}

func (r *Resolver) boundedGraphReadFilter(ctx context.Context) domain.ExpandReadFilter {
	if r != nil && r.BoundedGraphReadFilterProvider != nil {
		return r.BoundedGraphReadFilterProvider(ctx)
	}
	access, ok := BoundedGraphPrincipalAccessFromContext(ctx)
	if !ok {
		return domain.ExpandReadFilter{}
	}
	return boundedGraphReadFilterFromPrincipalAccess(access)
}

func boundedGraphReadFilterFromPrincipalAccess(access BoundedGraphPrincipalAccess) domain.ExpandReadFilter {
	allowedVisibility := map[string]bool{
		domain.VisibilityPublic: true,
	}
	for _, value := range access.AllowedVisibilityClasses {
		value = strings.TrimSpace(value)
		if value != "" {
			allowedVisibility[value] = true
		}
	}
	allowed := func(visibility string) bool {
		return allowedVisibility[strings.TrimSpace(visibility)]
	}
	return domain.ExpandReadFilter{
		PrincipalKey: strings.TrimSpace(access.PrincipalKey),
		ObjectAllowed: func(object domain.Object) bool {
			return allowed(object.Visibility)
		},
		AssociationAllowed: func(association domain.Association) bool {
			return allowed(association.Metadata.Visibility)
		},
	}
}
