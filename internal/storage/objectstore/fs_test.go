package objectstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExtFor(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"application/json", ".json"},
		{"application/json; charset=utf-8", ".json"},
		{"text/xml", ".xml"},
		{"application/xml", ".xml"},
		{"application/soap+xml", ".xml"},
		{"text/html", ".html"},
		{"text/vtt", ".vtt"},
		{"text/plain", ".txt"},
		{"text/csv", ".csv"},
		{"image/png", ".bin"},
		{"", ".bin"},
		{"  TEXT/HTML  ; boundary=x", ".html"},
	}
	for _, c := range cases {
		if got := extFor(c.in); got != c.want {
			t.Errorf("extFor(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanPrefix(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"/":        "",
		"/raw/":    "raw",
		"raw/sub":  "raw/sub",
		"/raw/sub": "raw/sub",
		".":        "",
	}
	for in, want := range cases {
		if got := cleanPrefix(in); got != want {
			t.Errorf("cleanPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewFS_EmptyRoot(t *testing.T) {
	if _, err := NewFS(""); err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestNewFS_CreatesRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "root")
	if _, err := NewFS(root); err != nil {
		t.Fatalf("NewFS: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("root not created: %v", err)
	}
}

func TestFSPutGet_RoundTrip(t *testing.T) {
	fs, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body := []byte(`{"hello":"world"}`)
	rel, err := fs.Put(ctx, "lws", "abc123", "application/json", body)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := filepath.Join("lws", "abc123.json")
	if rel != want {
		t.Errorf("rel = %q, want %q", rel, want)
	}
	got, err := fs.Get(ctx, rel)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("Get returned %q, want %q", got, body)
	}
}

func TestFSPut_HashRequired(t *testing.T) {
	fs, _ := NewFS(t.TempDir())
	if _, err := fs.Put(context.Background(), "lws", "", "application/json", []byte("x")); err == nil {
		t.Fatal("expected error for empty hash")
	}
}

func TestFSPut_Idempotent(t *testing.T) {
	root := t.TempDir()
	fs, _ := NewFS(root)
	ctx := context.Background()
	rel, err := fs.Put(ctx, "lws", "h1", "text/plain", []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	// Second put with the same hash must NOT overwrite the existing bytes.
	if _, err := fs.Put(ctx, "lws", "h1", "text/plain", []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := fs.Get(ctx, rel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first" {
		t.Errorf("body = %q, want %q (immutable on rewrite)", got, "first")
	}
}

func TestFSGet_Missing(t *testing.T) {
	fs, _ := NewFS(t.TempDir())
	if _, err := fs.Get(context.Background(), "does/not/exist.bin"); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestFSPut_DistinctSystemsIsolated(t *testing.T) {
	fs, _ := NewFS(t.TempDir())
	ctx := context.Background()
	relA, _ := fs.Put(ctx, "a", "h", "text/plain", []byte("A"))
	relB, _ := fs.Put(ctx, "b", "h", "text/plain", []byte("B"))
	if relA == relB {
		t.Fatalf("expected distinct paths for different systems, got %q", relA)
	}
	a, _ := fs.Get(ctx, relA)
	b, _ := fs.Get(ctx, relB)
	if string(a) != "A" || string(b) != "B" {
		t.Errorf("isolation broken: a=%q b=%q", a, b)
	}
}
