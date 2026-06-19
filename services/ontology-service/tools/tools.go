//go:build tools

// Package tools pins command-line generators used by go:generate.
package tools

import (
	_ "entgo.io/ent/cmd/ent"        // ent generates typed ORM code from schemas.
	_ "github.com/99designs/gqlgen" // gqlgen generates the GraphQL execution and model packages.
)
