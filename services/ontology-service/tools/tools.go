//go:build tools

// Package tools pins command-line generators used by go:generate.
package tools

import (
	_ "github.com/99designs/gqlgen" // gqlgen generates the GraphQL execution and model packages.
)
