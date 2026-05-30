// Package connector defines the cross-cutting contract every source
// connector under internal/sources/* shares: a Descriptor describing the
// upstream service, a Source interface for runtime discovery, and shared
// HTTP request helpers (Accept headers) so connectors don't redefine
// them locally.
//
// Connectors register themselves at init time via Register, which lets
// the CLI and observability layer iterate every shipped source without
// importing each subpackage by name.
//
// This package intentionally has no dependencies outside the standard
// library — it must be importable from any source connector without
// creating cycles through httpx.
package connector

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
)

// Descriptor describes a single upstream data source.
//
// System matches the value written to source_record.source_system. It
// is the stable identifier used across raw object storage paths,
// provenance rows, and operator commands.
//
// BaseURL is the canonical upstream root. Tests and staging deployments
// override the live client's BaseURL; this field is the documented
// production default, not a runtime configuration knob.
//
// RateLimitHz is the polite ceiling we self-impose against the upstream.
// 0 means "unset — use the global httpx default of 10 req/sec." Values
// here are advisory; the live limit lives in the httpx.Config the
// process is built with.
type Descriptor struct {
	System      string
	BaseURL     string
	RateLimitHz float64
	Description string
}

// Source is implemented by every connector's *Client. The CLI uses this
// to print and inspect the registered source set without importing
// each connector package directly.
type Source interface {
	Descriptor() Descriptor
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Descriptor{}
)

// Register records d under d.System. Calling Register twice with the
// same System panics — descriptors are package-level constants and a
// duplicate indicates a copy/paste bug.
func Register(d Descriptor) {
	if d.System == "" {
		panic("connector.Register: empty Descriptor.System")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[d.System]; dup {
		panic(fmt.Sprintf("connector.Register: duplicate System %q", d.System))
	}
	registry[d.System] = d
}

// Lookup returns the Descriptor previously registered for system.
func Lookup(system string) (Descriptor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := registry[system]
	return d, ok
}

// Registered returns every registered Descriptor sorted by System.
func Registered() []Descriptor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Descriptor, 0, len(registry))
	for _, d := range registry {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].System < out[j].System })
	return out
}

// JSONAccept returns headers requesting an application/json response.
// Use for REST endpoints that respect content negotiation.
func JSONAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "application/json")
	return h
}

// HTMLAccept returns headers requesting an HTML response. Use for
// scraping endpoints that return text/html.
func HTMLAccept() http.Header {
	h := http.Header{}
	h.Set("Accept", "text/html")
	return h
}
