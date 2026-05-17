// Package httpx is the shared HTTP client used by every source connector.
//
// Responsibilities (per the wiki's Recommended Tech Stack §Key Decisions
// and the per-source spec docs §"Boundaries"):
//
//  1. Per-host token-bucket rate limiting — public legislative APIs have no
//     stated SLA; be polite.
//  2. Bounded retry/backoff for transient HTTP errors (5xx, network errors).
//  3. A RawSink hook fired on every successful fetch, so connectors can record
//     immutable provenance (raw bytes + URL + fetched_at + content hash) before
//     parsing. This makes Architectural Decision #2 (raw records immutable)
//     and #3 (every fact has provenance) impossible to forget.
//
// Connectors should use Client.Do directly and inspect the returned RawFetch
// for provenance metadata. The four-layer split (Fetch / StoreRaw / Parse /
// Normalize) from the per-source specs is enforced at the call sites.
package httpx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RawFetch captures everything a connector or its caller needs to record
// provenance for a single HTTP exchange.
type RawFetch struct {
	System      string    // "lws", "csi", "tvw", "invintus", "committee_schedules", "pdc_socrata"
	Endpoint    string    // logical endpoint name (e.g. "LegislationService.GetLegislation")
	Method      string    // "GET", "POST"
	URL         string    // canonical request URL
	Status      int       // HTTP status code
	ContentType string    // response Content-Type
	Body        []byte    // response body bytes (verbatim)
	Hash        string    // sha256 hex of Body
	FetchedAt   time.Time // when the request returned

	// SourceRecordID is the id assigned to this fetch in the source_record
	// table by the RawSink. Sinks that don't talk to a database leave it 0.
	SourceRecordID int64
}

// RawSink is invoked once per completed HTTP response, including non-2xx
// responses. Implementations typically write the body to object storage and
// insert a source_record row. Network failures with no HTTP response cannot be
// recorded because there are no raw response bytes.
//
// Errors from the sink propagate back to the caller — provenance must not
// silently fail.
//
// The fetch is passed by pointer so the sink can populate
// f.SourceRecordID after persisting the row.
type RawSink interface {
	Record(ctx context.Context, f *RawFetch) error
}

// NopSink discards everything. Useful for tests and read-only exploratory
// source discovery that does not need durable provenance.
type NopSink struct{}

func (NopSink) Record(ctx context.Context, f *RawFetch) error { return nil }

// Config configures a Client.
type Config struct {
	UserAgent     string             // sent on every request
	Timeout       time.Duration      // per-request timeout (default 30s)
	MaxRetries    int                // additional attempts beyond the first (default 2)
	RetryBackoff  time.Duration      // base backoff; doubles each retry (default 500ms)
	RetryOn       []int              // status codes to retry (default 429, 500, 502, 503, 504)
	HostRateLimit map[string]float64 // requests/sec per host; missing host => unlimited
	Sink          RawSink            // raw-bytes hook; required (use NopSink to opt out)
	HTTP          *http.Client       // optional override
}

// Client is a polite HTTP client wrapping a per-host rate limiter, retries,
// and the RawSink hook.
type Client struct {
	cfg      Config
	http     *http.Client
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// New builds a Client. Required: cfg.Sink (use NopSink{} explicitly to opt out).
func New(cfg Config) *Client {
	if cfg.Sink == nil {
		// Provenance is non-negotiable; force callers to think about it.
		// Tests can pass NopSink{} explicitly.
		panic("httpx.New: cfg.Sink is required (use httpx.NopSink{} to opt out)")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = 500 * time.Millisecond
	}
	if len(cfg.RetryOn) == 0 {
		cfg.RetryOn = []int{429, 500, 502, 503, 504}
	}
	httpC := cfg.HTTP
	if httpC == nil {
		httpC = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{
		cfg:      cfg,
		http:     httpC,
		limiters: map[string]*rate.Limiter{},
	}
}

// Request describes a single fetch.
type Request struct {
	System   string
	Endpoint string
	Method   string
	URL      string
	Headers  http.Header
	Body     []byte
}

// Do performs req, retries transient failures, records each raw HTTP response
// via the configured Sink, and returns the final captured RawFetch.
//
// On HTTP errors, the response is still recorded — the body usually contains
// diagnostic information worth keeping.
func (c *Client) Do(ctx context.Context, req Request) (RawFetch, error) {
	if req.Method == "" {
		req.Method = http.MethodGet
	}

	host, err := hostOf(req.URL)
	if err != nil {
		return RawFetch{}, err
	}

	if err := c.waitForLimiter(ctx, host); err != nil {
		return RawFetch{}, err
	}

	var lastErr error
	var lastFetch RawFetch
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.cfg.RetryBackoff * (1 << (attempt - 1))
			select {
			case <-ctx.Done():
				return RawFetch{}, ctx.Err()
			case <-time.After(backoff):
			}
			// Each retry consumes another token.
			if err := c.waitForLimiter(ctx, host); err != nil {
				return RawFetch{}, err
			}
		}

		fetch, retry, err := c.attempt(ctx, req)
		if fetch.Status != 0 {
			if sinkErr := c.cfg.Sink.Record(ctx, &fetch); sinkErr != nil {
				return fetch, fmt.Errorf("raw sink record: %w", sinkErr)
			}
			lastFetch = fetch
		}
		if err == nil {
			return fetch, nil
		}
		lastErr = err
		if !retry {
			return fetch, err
		}
	}
	if lastFetch.Status != 0 {
		return lastFetch, fmt.Errorf("after %d attempts: %w", c.cfg.MaxRetries+1, lastErr)
	}
	return RawFetch{}, fmt.Errorf("after %d attempts: %w", c.cfg.MaxRetries+1, lastErr)
}

func (c *Client) attempt(ctx context.Context, req Request) (RawFetch, bool, error) {
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return RawFetch{}, false, err
	}
	for k, vs := range req.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}
	if c.cfg.UserAgent != "" && httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", c.cfg.UserAgent)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return RawFetch{}, !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded), err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RawFetch{}, true, fmt.Errorf("read body: %w", err)
	}

	fetch := RawFetch{
		System:      req.System,
		Endpoint:    req.Endpoint,
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		Hash:        sha256Hex(body),
		FetchedAt:   time.Now().UTC(),
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return fetch, false, nil
	}
	if c.shouldRetry(resp.StatusCode) {
		return fetch, true, fmt.Errorf("status %d: %s", resp.StatusCode, truncate(body, 200))
	}
	return fetch, false, fmt.Errorf("status %d: %s", resp.StatusCode, truncate(body, 200))
}

func (c *Client) shouldRetry(status int) bool {
	for _, s := range c.cfg.RetryOn {
		if s == status {
			return true
		}
	}
	return false
}

func (c *Client) waitForLimiter(ctx context.Context, host string) error {
	rps, ok := c.cfg.HostRateLimit[host]
	if !ok || rps <= 0 {
		return nil
	}
	c.mu.Lock()
	lim, found := c.limiters[host]
	if !found {
		lim = rate.NewLimiter(rate.Limit(rps), 1)
		c.limiters[host] = lim
	}
	c.mu.Unlock()
	return lim.Wait(ctx)
}

func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("no host in URL: %q", raw)
	}
	return u.Host, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
