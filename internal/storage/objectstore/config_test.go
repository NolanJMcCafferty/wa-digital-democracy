package objectstore

import (
	"context"
	"testing"
)

func TestNew_LocalBackends(t *testing.T) {
	for _, backend := range []string{"", "local", "fs", "filesystem", "LOCAL", " local "} {
		store, err := New(context.Background(), Config{Backend: backend, LocalRoot: t.TempDir()})
		if err != nil {
			t.Errorf("backend=%q: %v", backend, err)
			continue
		}
		if _, ok := store.(*FS); !ok {
			t.Errorf("backend=%q: expected *FS, got %T", backend, store)
		}
	}
}

func TestNew_UnsupportedBackend(t *testing.T) {
	if _, err := New(context.Background(), Config{Backend: "gcs"}); err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

func TestNew_S3RequiresBucket(t *testing.T) {
	if _, err := New(context.Background(), Config{Backend: "s3"}); err == nil {
		t.Fatal("expected error when S3 bucket is missing")
	}
	if _, err := New(context.Background(), Config{Backend: "r2"}); err == nil {
		t.Fatal("expected error when R2 bucket is missing")
	}
}

func TestNew_S3RequiresBothCreds(t *testing.T) {
	_, err := New(context.Background(), Config{
		Backend:       "s3",
		S3Bucket:      "b",
		S3AccessKeyID: "id",
		// SecretKey missing
	})
	if err == nil {
		t.Fatal("expected error when only one of access/secret is set")
	}
}

func TestNewConfigured_DefaultsLocal(t *testing.T) {
	t.Setenv("OBJECT_STORE", "")
	store, err := NewConfigured(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*FS); !ok {
		t.Errorf("expected *FS, got %T", store)
	}
}

func TestNewConfigured_BadPathStyleEnv(t *testing.T) {
	t.Setenv("OBJECT_STORE", "s3")
	t.Setenv("S3_BUCKET_RAW", "b")
	t.Setenv("S3_FORCE_PATH_STYLE", "not-a-bool")
	if _, err := NewConfigured(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected error for invalid bool env")
	}
}

func TestEnv(t *testing.T) {
	t.Setenv("WADD_TEST_X", "")
	if got := env("WADD_TEST_X", "def"); got != "def" {
		t.Errorf("env empty = %q, want def", got)
	}
	t.Setenv("WADD_TEST_X", "set")
	if got := env("WADD_TEST_X", "def"); got != "set" {
		t.Errorf("env set = %q, want set", got)
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("WADD_TEST_B", "")
	if v, err := parseBoolEnv("WADD_TEST_B", true); err != nil || v != true {
		t.Errorf("default not used: v=%v err=%v", v, err)
	}
	t.Setenv("WADD_TEST_B", "true")
	if v, err := parseBoolEnv("WADD_TEST_B", false); err != nil || v != true {
		t.Errorf("true not parsed: v=%v err=%v", v, err)
	}
	t.Setenv("WADD_TEST_B", "junk")
	if _, err := parseBoolEnv("WADD_TEST_B", false); err == nil {
		t.Error("expected parse error")
	}
}
