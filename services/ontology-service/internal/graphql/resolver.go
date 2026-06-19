package graphql

// Resolver is the gqlgen dependency root for Cubicle's GraphQL API.
//
// It is intentionally empty in the first GraphQL slice. Later ontology schema
// work will add query services and storage dependencies here instead of putting
// product logic inside HTTP router setup.
type Resolver struct{}
