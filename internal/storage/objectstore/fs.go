// Package objectstore stores raw source-response bytes on disk.
//
// Layout: data/raw/<system>/<sha256>.<ext>
//
// The sha256 of the body is the filename, which makes the store
// content-addressed: identical responses dedupe automatically. Per
// Architectural Decision #2 (raw source records immutable), files are never
// overwritten — if a file with the same hash already exists, Put is a no-op.
//
// In production this will be swapped for an S3/R2-backed implementation
// (see Recommended Tech Stack §"Raw source storage"). The interface stays the
// same.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store is the abstraction every source connector talks to.
type Store interface {
	// Put writes body if no object with the given hash exists yet, and
	// returns the relative path used (e.g. "lws/abc123.xml"). If the path
	// already exists, Put is a no-op and still returns the path.
	Put(ctx context.Context, system, hash, contentType string, body []byte) (string, error)

	// Get reads the body at relPath.
	Get(ctx context.Context, relPath string) ([]byte, error)
}

// FS is a filesystem-backed Store rooted at a configured directory.
type FS struct {
	root string
}

// NewFS returns a Store rooted at root. The root must exist or be creatable.
func NewFS(root string) (*FS, error) {
	if root == "" {
		return nil, errors.New("objectstore: root must not be empty")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir root: %w", err)
	}
	return &FS{root: root}, nil
}

func (f *FS) Put(_ context.Context, system, hash, contentType string, body []byte) (string, error) {
	if hash == "" {
		return "", errors.New("objectstore: hash required")
	}
	ext := extFor(contentType)
	rel := filepath.Join(system, hash+ext)
	abs := filepath.Join(f.root, rel)

	if _, err := os.Stat(abs); err == nil {
		return rel, nil // already present; immutable
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := writeAtomic(abs, body); err != nil {
		return "", err
	}
	return rel, nil
}

func (f *FS) Get(_ context.Context, relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(f.root, relPath))
}

func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".put-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// extFor maps a Content-Type to a friendly file extension. Best-effort —
// when in doubt, use ".bin".
func extFor(contentType string) string {
	if i := strings.IndexByte(contentType, ';'); i > 0 {
		contentType = contentType[:i]
	}
	switch strings.TrimSpace(strings.ToLower(contentType)) {
	case "application/json":
		return ".json"
	case "text/xml", "application/xml", "application/soap+xml":
		return ".xml"
	case "text/html":
		return ".html"
	case "text/vtt":
		return ".vtt"
	case "text/plain":
		return ".txt"
	case "text/csv":
		return ".csv"
	default:
		return ".bin"
	}
}
