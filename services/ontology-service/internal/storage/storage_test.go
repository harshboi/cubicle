package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAppliesSQLitePragmas(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if got := queryPragmaInt(t, store, "foreign_keys"); got != 1 {
		t.Fatalf("foreign_keys = %d, want 1", got)
	}
	if got := queryPragmaInt(t, store, "busy_timeout"); got < 5000 {
		t.Fatalf("busy_timeout = %d, want at least 5000", got)
	}
	if got := queryPragmaString(t, store, "journal_mode"); strings.ToLower(got) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", got)
	}
}

func TestWithTxCommitsOnSuccess(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	createEventsTable(t, ctx, store)

	err := store.WithTx(ctx, func(tx Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO events(key) VALUES (?)", "committed")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx returned error: %v", err)
	}

	if got := countEvents(t, ctx, store); got != 1 {
		t.Fatalf("event count = %d, want 1", got)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()
	createEventsTable(t, ctx, store)

	errExpected := errors.New("mapper failed")
	err := store.WithTx(ctx, func(tx Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO events(key) VALUES (?)", "rolled-back"); err != nil {
			return err
		}
		return errExpected
	})
	if !errors.Is(err, errExpected) {
		t.Fatalf("WithTx error = %v, want %v", err, errExpected)
	}

	if got := countEvents(t, ctx, store); got != 0 {
		t.Fatalf("event count = %d, want 0", got)
	}
}

func TestOpenRejectsMissingDatabasePath(t *testing.T) {
	_, err := Open(context.Background(), Config{})
	if err == nil {
		t.Fatal("expected missing database path to fail")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
	})
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	return store
}

func queryPragmaInt(t *testing.T, store *Store, name string) int {
	t.Helper()
	var got int
	if err := store.DB().QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("query PRAGMA %s: %v", name, err)
	}
	return got
}

func queryPragmaString(t *testing.T, store *Store, name string) string {
	t.Helper()
	var got string
	if err := store.DB().QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("query PRAGMA %s: %v", name, err)
	}
	return got
}

func createEventsTable(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	_, err := store.DB().ExecContext(ctx, "CREATE TABLE events (key TEXT PRIMARY KEY)")
	if err != nil {
		t.Fatalf("create events table: %v", err)
	}
}

func countEvents(t *testing.T, ctx context.Context, store *Store) int {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	return count
}
