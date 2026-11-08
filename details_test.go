package vanitypages

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeDocs serves the documentation API from canned responses, by import path.
// Unlisted paths are not found; an empty response stands for a server error.
func fakeDocs(t *testing.T, packages, modules map[string]string) *httptest.Server {
	t.Helper()
	serve := func(responses map[string]string, wantParam string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !r.URL.Query().Has(wantParam) {
				t.Errorf("%s has no %s parameter", r.URL, wantParam)
			}
			body, ok := responses[r.PathValue("path")]
			switch {
			case !ok:
				http.NotFound(w, r)
			case body == "":
				http.Error(w, "down", http.StatusInternalServerError)
			default:
				io.WriteString(w, body)
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/package/{path...}", serve(packages, "licenses"))
	mux.HandleFunc("GET /v1/module/{path...}", serve(modules, "version"))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sameDetail(a, b Detail) bool {
	return a.Version == b.Version && a.Published.Equal(b.Published) && a.License == b.License && a.Synopsis == b.Synopsis
}

func TestFetchDetails(t *testing.T) {
	srv := fakeDocs(t, map[string]string{
		"go.example.com/cache/v2":     `{"modulePath":"go.example.com/cache/v2","version":"v2.1.0","synopsis":"Package cache caches.","licenses":[{"types":["MIT"]},{"types":["MIT","Apache-2.0"]}]}`,
		"go.example.com/cache/v2/lru": `{"modulePath":"go.example.com/cache/v2","version":"v2.1.0","synopsis":"Package lru evicts."}`,
	}, map[string]string{
		"go.example.com/cache/v2": `{"commitTime":"2026-03-16T17:32:55Z"}`,
	})
	c := Config{
		Domain: "go.example.com",
		Docs:   srv.URL,
		Modules: []Module{{
			Path:     "cache",
			Repo:     "https://github.com/acme/cache",
			Versions: []string{"v2"},
			Packages: []string{"lru", "unpublished"},
		}},
	}

	got, err := c.FetchDetails(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 3, 16, 17, 32, 55, 0, time.UTC)
	want := Details{
		"go.example.com/cache/v2":     {Version: "v2.1.0", Published: published, License: "MIT, Apache-2.0", Synopsis: "Package cache caches."},
		"go.example.com/cache/v2/lru": {Version: "v2.1.0", Published: published, Synopsis: "Package lru evicts."},
	}
	if !maps.EqualFunc(got, want, sameDetail) {
		t.Errorf("details = %+v, want %+v", got, want)
	}
}

func TestFetchDetailsKeepsResultsOnFailure(t *testing.T) {
	for name, failure := range map[string]string{"server error": "", "invalid response": "<html>"} {
		t.Run(name, func(t *testing.T) {
			srv := fakeDocs(t, map[string]string{
				"go.example.com/a": `{"modulePath":"go.example.com/a","version":"v1.0.0"}`,
				"go.example.com/b": failure,
				"go.example.com/c": `{"modulePath":"go.example.com/c","version":"v1.0.0"}`,
			}, map[string]string{
				"go.example.com/a": `{}`,
				"go.example.com/c": `{}`,
			})
			c := Config{Domain: "go.example.com", Docs: srv.URL}
			for _, path := range []string{"a", "b", "c"} {
				c.Modules = append(c.Modules, Module{Path: path, Repo: "https://github.com/acme/" + path})
			}

			got, err := c.FetchDetails(t.Context())
			if err == nil {
				t.Fatal("want an error")
			}
			want := Details{"go.example.com/a": {Version: "v1.0.0"}}
			if !maps.EqualFunc(got, want, sameDetail) {
				t.Errorf("details = %+v, want %+v", got, want)
			}
		})
	}
}

func TestRenderDetails(t *testing.T) {
	c := Config{
		Domain: "go.example.com",
		Modules: []Module{{
			Path:       "cache",
			Repo:       "https://github.com/acme/cache",
			Versions:   []string{"v2"},
			Packages:   []string{"lru"},
			Submodules: []Submodule{{Path: "redis", Description: "Redis backend"}, {Path: "memcached"}},
		}},
		Details: Details{
			"go.example.com/cache/v2": {
				Version:   "v2.1.0",
				Published: time.Date(2026, 3, 16, 23, 30, 0, 0, time.UTC),
				License:   "MIT, Apache-2.0",
				Synopsis:  "Package cache caches.",
			},
			"go.example.com/cache/v2/lru":    {Version: "v2.1.0", Synopsis: "Package lru evicts."},
			"go.example.com/cache/redis":     {Version: "v0.4.0", Synopsis: "Loses to the configured description."},
			"go.example.com/cache/memcached": {Version: "v0.1.0", Synopsis: "Package memcached talks to memcached."},
		},
	}
	files, err := c.Render()
	if err != nil {
		t.Fatal(err)
	}

	module := string(files["cache/index.html"])
	for _, want := range []string{
		"Package cache caches.",
		`https://pkg.go.dev/go.example.com/cache/v2?tab=versions">v2.1.0<`,
		"Published: Mar 16, 2026",
		`https://pkg.go.dev/go.example.com/cache/v2?tab=licenses">MIT, Apache-2.0<`,
		"Package lru evicts.",
		"Redis backend",
		"Package memcached talks to memcached.",
	} {
		if !strings.Contains(module, want) {
			t.Errorf("module page is missing %q", want)
		}
	}
	if strings.Contains(module, "Loses to the configured") {
		t.Error("a synopsis replaced the configured description")
	}
	if index := string(files["index.html"]); !strings.Contains(index, "Package cache caches.") {
		t.Error("index does not fall back to the synopsis")
	}

	pkg := string(files["cache/v2/lru/index.html"])
	if !strings.Contains(pkg, "v2.1.0") {
		t.Error("package page has no version")
	}
	for _, unwanted := range []string{"Published:", "License:"} {
		if strings.Contains(pkg, unwanted) {
			t.Errorf("package page shows %q without data for it", unwanted)
		}
	}
}

func TestRenderWithoutDetails(t *testing.T) {
	files, err := exampleConfig().Render()
	if err != nil {
		t.Fatal(err)
	}
	if page := string(files["cache/index.html"]); strings.Contains(page, "Version:") {
		t.Error("page shows a version without details")
	}
}
