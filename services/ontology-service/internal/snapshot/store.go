package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	root string
}

type WrittenBody struct {
	BodySHA256 string
	BodyRef    string
}

func NewStore(root string) Store {
	return Store{root: root}
}

func (s Store) Root() string {
	return s.root
}

func (s Store) Write(ctx context.Context, _ string, body []byte) (WrittenBody, error) {
	select {
	case <-ctx.Done():
		return WrittenBody{}, ctx.Err()
	default:
	}

	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])
	bodyRef := filepath.Join("sha256", hexSum)
	path := filepath.Join(s.root, bodyRef)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WrittenBody{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return WrittenBody{BodySHA256: "sha256:" + hexSum, BodyRef: filepath.ToSlash(bodyRef)}, nil
	} else if !os.IsNotExist(err) {
		return WrittenBody{}, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return WrittenBody{}, err
	}
	return WrittenBody{BodySHA256: "sha256:" + hexSum, BodyRef: filepath.ToSlash(bodyRef)}, nil
}

func (s Store) Read(ctx context.Context, bodyRef string) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if filepath.IsAbs(bodyRef) || strings.Contains(filepath.Clean(bodyRef), "..") {
		return nil, fmt.Errorf("invalid body ref %q", bodyRef)
	}
	return os.ReadFile(filepath.Join(s.root, filepath.FromSlash(bodyRef)))
}
