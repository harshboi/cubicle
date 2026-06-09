// Package storage owns the SQLite process boundary for ontology-service.
//
// This package deliberately exposes a small database/sql foundation instead of
// Ent entities. The next Ent PR can build an Ent client over Store.DB(), while
// HTTP/query packages continue to depend on higher-level graph interfaces.
package storage
