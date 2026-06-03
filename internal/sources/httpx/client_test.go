package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_DoReturnsFetchOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c := New(Config{})
	got, err := c.Do(context.Background(), Request{System: "test", Endpoint: "ping", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(got.Body) != "hello" {
		t.Fatalf("body = %q, want %q", got.Body, "hello")
	}
	if got.Hash == "" {
		t.Fatal("hash empty")
	}
	if got.FetchedAt.IsZero() {
		t.Fatal("fetched_at zero")
	}
}

func TestClient_DoRetriesTransient(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(Config{MaxRetries: 3, RetryBackoff: time.Millisecond})
	got, err := c.Do(context.Background(), Request{System: "t", Endpoint: "e", URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got.Status != http.StatusOK {
		t.Fatalf("final status = %d", got.Status)
	}
	if hits != 3 {
		t.Fatalf("server hits = %d, want 3", hits)
	}
}

func TestClient_DoNonRetryableErrorReturnsImmediately(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()

	c := New(Config{MaxRetries: 5, RetryBackoff: time.Millisecond})
	got, err := c.Do(context.Background(), Request{System: "t", Endpoint: "e", URL: srv.URL})
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %v, want substring 400", err)
	}
	if got.Status != http.StatusBadRequest || string(got.Body) != "bad" {
		t.Fatalf("got status/body = %d/%q, want 400/bad", got.Status, got.Body)
	}
}
