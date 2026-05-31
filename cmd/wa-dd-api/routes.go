package main

import (
	"fmt"
	"net/http"

	"github.com/nolan-mccafferty/wa-digital-democracy/internal/storage/db"
)

type routeScope string

const (
	scopeRoot          routeScope = "root"
	scopeAPI           routeScope = "api"
	scopeAdminViewer   routeScope = "admin-viewer"
	scopeAdminReviewer routeScope = "admin-reviewer"
)

type route struct {
	Method string
	Path   string
	Scope  routeScope
	Plain  func() http.HandlerFunc
	Store  func(*db.Store) http.HandlerFunc
}

var routeRegistry []route

func registerRoute(r route) {
	if r.Method == "" || r.Path == "" {
		panic(fmt.Sprintf("registerRoute: method and path required (%+v)", r))
	}
	if (r.Plain == nil) == (r.Store == nil) {
		panic(fmt.Sprintf("registerRoute: exactly one of Plain/Store must be set (%s %s)", r.Method, r.Path))
	}
	routeRegistry = append(routeRegistry, r)
}
