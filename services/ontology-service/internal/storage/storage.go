package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const sqliteDriver = "sqlite3"

// Config controls the local SQLite store.
type Config struct {
	DatabasePath string
	BusyTimeout  time.Duration
}

// Store is the storage foundation shared by future Ent and snapshot code.
type Store struct {
	db *sql.DB
}

// Tx is the tiny transaction surface used by WithTx callbacks.
//
// Using a small interface here makes tests and future mapper code clearer: the
// callback can execute SQL inside the transaction, but it cannot commit or roll
// back behind Store's back.
type Tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Open creates a SQLite-backed Store and applies the local-first pragmas this
// service relies on.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.DatabasePath == "" {
		return nil, errors.New("database path is required")
	}
	if cfg.BusyTimeout == 0 {
		cfg.BusyTimeout = 5 * time.Second
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open(sqliteDriver, cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.applyPragmas(ctx, cfg.BusyTimeout); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

// DB exposes the underlying database/sql handle for infrastructure layers.
//
// This is intentionally not used by HTTP code. The future Ent store can build a
// generated client from this handle, while product code should continue through
// graph/query interfaces.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) applyPragmas(ctx context.Context, busyTimeout time.Duration) error {
	pragmas := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeout.Milliseconds()),
		"PRAGMA synchronous=NORMAL",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	return nil
}

// WithTx runs fn inside a transaction and owns commit/rollback.
func (s *Store) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
