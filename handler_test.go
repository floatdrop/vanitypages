package vanitypages

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestHandlerFiles(t *testing.T) {
	icon := filepath.Join(t.TempDir(), "icon.svg")
	if err := os.WriteFile(icon, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ domain, base string }{
		{"go.example.com", ""},
		{"example.com/go/sdk", "/go/sdk"},
	} {
		t.Run(tt.domain, func(t *testing.T) {
			cfg := exampleConfig()
			cfg.Domain = tt.domain
			cfg.Icon = icon
			cfg.Modules[0].Versions = []string{"v2"}
			files, err := cfg.Render()
			if err != nil {
				t.Fatal(err)
			}
			handler, err := cfg.Handler()
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range files {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					paths := []string{tt.base + "/" + name}
					if name == "index.html" || strings.HasSuffix(name, "/index.html") {
						paths = append(paths, strings.TrimSuffix(paths[0], "index.html"))
					}
					for _, path := range paths {
						for _, method := range []string{http.MethodGet, http.MethodHead} {
							req := httptest.NewRequest(method, path+"?go-get=1", nil)
							res := httptest.NewRecorder()
							handler.ServeHTTP(res, req)
							if res.Code != http.StatusOK {
								t.Fatalf("%s %s: status = %d, want 200", method, path, res.Code)
							}
							if got, want := res.Header().Get("Content-Type"), mime.TypeByExtension(filepath.Ext(name)); got != want {
								t.Errorf("%s %s: Content-Type = %q, want %q", method, path, got, want)
							}
							if got, want := res.Header().Get("Content-Length"), strconv.Itoa(len(data)); got != want {
								t.Errorf("%s %s: Content-Length = %q, want %q", method, path, got, want)
							}
							if method == http.MethodHead {
								if res.Body.Len() != 0 {
									t.Errorf("HEAD %s returned a body", path)
								}
								continue
							}
							if !bytes.Equal(res.Body.Bytes(), data) {
								t.Errorf("GET %s: body differs from Render output", path)
							}
							if strings.HasSuffix(name, "/index.html") {
								want := tt.domain + "/cache git https://github.com/acme/cache"
								if got := parseMeta(t, res.Body.Bytes())["go-import"]; got != want {
									t.Errorf("GET %s: go-import = %q, want %q", path, got, want)
								}
							}
						}
					}
				})
			}
		})
	}
}

func TestHandlerRoutes(t *testing.T) {
	for _, base := range []string{"", "/go"} {
		t.Run("base="+base, func(t *testing.T) {
			cfg := exampleConfig()
			cfg.Domain += base
			cfg.Modules[0].Versions = []string{"v2"}
			handler, err := cfg.Handler()
			if err != nil {
				t.Fatal(err)
			}
			for _, tt := range []struct {
				method, path string
				status       int
				location     string
			}{
				{"GET", "/cache?go-get=1", http.StatusTemporaryRedirect, "/cache/?go-get=1"},
				{"HEAD", "/cache?go-get=1", http.StatusTemporaryRedirect, "/cache/?go-get=1"},
				{"GET", "/cache/v2?go-get=1", http.StatusTemporaryRedirect, "/cache/v2/?go-get=1"},
				{"GET", "/cache/contrib/redis", http.StatusTemporaryRedirect, "/cache/contrib/redis/"},
				{"GET", "/cache//v2/", http.StatusTemporaryRedirect, "/cache/v2/"},
				{"GET", "/cache/../", http.StatusTemporaryRedirect, "/"},
				{"GET", "/missing", 404, ""},
				{"GET", "/missing/", 404, ""},
				{"GET", "/cache2/", 404, ""},
				{"GET", "/cache/missing?go-get=1", 404, ""},
				{"GET", "/cache/contrib/", 404, ""},
				{"GET", "/-/", 404, ""},
				{"GET", "/cache/%2e%2e/", 404, ""},
				{"GET", "/cache%2fv2/", 404, ""},
				{"POST", "/", 405, ""},
				{"POST", "/cache/", 405, ""},
				{"OPTIONS", "/cache/", 405, ""},
				{"DELETE", "/-/vanitypages.css", 405, ""},
			} {
				t.Run(tt.method+" "+tt.path, func(t *testing.T) {
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, httptest.NewRequest(tt.method, base+tt.path, nil))
					if res.Code != tt.status {
						t.Errorf("status = %d, want %d", res.Code, tt.status)
					}
					location := tt.location
					if location != "" {
						location = base + location
					}
					if got := res.Header().Get("Location"); got != location {
						t.Errorf("Location = %q, want %q", got, location)
					}
					if tt.status == http.StatusMethodNotAllowed && res.Header().Get("Allow") != "GET, HEAD" {
						t.Errorf("Allow = %q, want GET, HEAD", res.Header().Get("Allow"))
					}
				})
			}
			if base != "" {
				res := httptest.NewRecorder()
				handler.ServeHTTP(res, httptest.NewRequest("GET", base+"?go-get=1", nil))
				if res.Code != http.StatusTemporaryRedirect || res.Header().Get("Location") != base+"/?go-get=1" {
					t.Errorf("base redirect: status = %d, Location = %q", res.Code, res.Header().Get("Location"))
				}
				for _, path := range []string{"/", "/cache/", base + "-other/cache/"} {
					res := httptest.NewRecorder()
					handler.ServeHTTP(res, httptest.NewRequest("GET", path, nil))
					if res.Code != http.StatusNotFound {
						t.Errorf("GET %s: status = %d, want 404", path, res.Code)
					}
				}
			}
		})
	}
}

func TestHandlerRange(t *testing.T) {
	handler, err := exampleConfig().Handler()
	if err != nil {
		t.Fatal(err)
	}
	full := httptest.NewRecorder()
	handler.ServeHTTP(full, httptest.NewRequest("GET", "/-/vanitypages.css", nil))
	req := httptest.NewRequest("GET", "/-/vanitypages.css", nil)
	req.Header.Set("Range", "bytes=5-14")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.Code)
	}
	if got, want := res.Header().Get("Content-Range"), fmt.Sprintf("bytes 5-14/%d", full.Body.Len()); got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
	if !bytes.Equal(res.Body.Bytes(), full.Body.Bytes()[5:15]) {
		t.Error("range body differs from the requested bytes")
	}
}

func TestHandlerConcurrentRequests(t *testing.T) {
	handler, err := exampleConfig().Handler()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 16 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest("GET", "/cache/?go-get=1", nil))
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.Code)
			}
			if got, want := parseMeta(t, res.Body.Bytes())["go-import"], "go.example.com/cache git https://github.com/acme/cache"; got != want {
				t.Errorf("go-import = %q, want %q", got, want)
			}
		})
	}
}

func TestHandlerSnapshot(t *testing.T) {
	cfg := exampleConfig()
	const icon = `<svg xmlns="http://www.w3.org/2000/svg"/>`
	cfg.Icon = filepath.Join(t.TempDir(), "icon.svg")
	if err := os.WriteFile(cfg.Icon, []byte(icon), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := cfg.Handler()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Modules[0].Repo = "https://github.com/another/repo"
	if err := os.Remove(cfg.Icon); err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "/cache/?go-get=1", nil))
	if got, want := parseMeta(t, res.Body.Bytes())["go-import"], "go.example.com/cache git https://github.com/acme/cache"; got != want {
		t.Errorf("go-import = %q, want %q", got, want)
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest("GET", "/-/icon.svg", nil))
	if res.Code != http.StatusOK || res.Body.String() != icon {
		t.Errorf("icon after removal: status = %d, body = %q", res.Code, res.Body.String())
	}
}

func TestHandlerErrors(t *testing.T) {
	missingIcon := exampleConfig()
	missingIcon.Icon = filepath.Join(t.TempDir(), "missing.svg")
	for _, tt := range []struct {
		name   string
		config Config
		want   string
	}{
		{"invalid config", Config{}, "domain:"},
		{"missing icon", missingIcon, "icon:"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := tt.config.Handler()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want an error containing %q", err, tt.want)
			}
			if handler != nil {
				t.Error("returned a handler on error")
			}
		})
	}
}
