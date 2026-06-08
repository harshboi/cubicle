package snapshot

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreWritesContentAddressedBodyAndReplaysIt(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	body := []byte(`{"key":"FLINK-39743"}`)

	written, err := store.Write(ctx, "custom-object", body)
	if err != nil {
		t.Fatalf("write snapshot body: %v", err)
	}
	if written.BodySHA256 == "" {
		t.Fatal("body hash was empty")
	}
	if filepath.IsAbs(written.BodyRef) {
		t.Fatalf("body ref should be relative, got %q", written.BodyRef)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), written.BodyRef)); err != nil {
		t.Fatalf("body was not written under root: %v", err)
	}

	replayed, err := store.Read(ctx, written.BodyRef)
	if err != nil {
		t.Fatalf("read snapshot body: %v", err)
	}
	if !bytes.Equal(replayed, body) {
		t.Fatalf("replayed body = %q", string(replayed))
	}

	again, err := store.Write(ctx, "custom-object", body)
	if err != nil {
		t.Fatalf("rewrite same body: %v", err)
	}
	if again.BodyRef != written.BodyRef || again.BodySHA256 != written.BodySHA256 {
		t.Fatalf("content address changed: first=%#v second=%#v", written, again)
	}
}
