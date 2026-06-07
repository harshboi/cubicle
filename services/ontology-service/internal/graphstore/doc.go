// Package graphstore contains implementations of Cubicle's graph-serving
// contract.
//
// The in-memory store is a POC implementation. Its job is not to be clever; its
// job is to make the graph API behavior precise before Ent code generation and
// SQLite enter the system. Later, an Ent-backed store should implement the same
// small interfaces so HTTP and query services do not change when persistence
// changes.
package graphstore
