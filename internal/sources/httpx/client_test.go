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

type recordingSink struct{ calls int32 }

func (r *recordingSink) Record(ctx context.Context, f *RawFetch) error {
	atomic.AddInt32(&r.calls, 1)
	f.SourceRecordID = int64(atomic.LoadInt32(&r.calls)) // simulate id assignment
	return nil
}

func TestClient_DoRecordsRawFetchOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	sink := &recordingSink{}
	c := New(Config{Sink: sink})
	got, err := c.Do(context.Background(), Request{
		System:   "test",
		Endpoint: "ping",
		URL:      srv.URL,
	})
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
	if atomic.LoadInt32(&sink.calls) != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.calls)
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

	sink := &recordingSink{}
	c := New(Config{
		Sink:         sink,
		MaxRetries:   3,
		RetryBackoff: 1 * time.Millisecond,
	})
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

	c := New(Config{Sink: NopSink{}, MaxRetries: 5, RetryBackoff: time.Millisecond})
	_, err := c.Do(context.Background(), Request{System: "t", Endpoint: "e", URL: srv.URL})
	if err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %v, want substring 400", err)
	}
}

func TestClient_NewPanicsWithoutSink(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	_ = New(Config{})
}
