// Package domain defines the API-safe ontology graph shapes used by the
// service boundary.
//
// These types are intentionally storage-neutral. The Swift app, HTTP handlers,
// query services, and tests can all speak in terms of objects, associations,
// and neighborhoods without knowing whether the backing store is memory, Ent, or
// SQLite. That separation is the main Go design pattern here: keep stable domain
// contracts away from persistence details.
package domain
