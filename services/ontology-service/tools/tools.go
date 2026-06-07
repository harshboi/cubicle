//go:build tools

// Package tools pins developer tools used by go:generate.
package tools

import (
	_ "entgo.io/ent/cmd/ent"
	// Ent's CLI dependency tree currently needs a newer displaywidth release to
	// compile cleanly on the local Go 1.25.1 toolchain. Importing it here keeps
	// the pin stable across `go mod tidy`.
	_ "github.com/clipperhouse/displaywidth"
)
