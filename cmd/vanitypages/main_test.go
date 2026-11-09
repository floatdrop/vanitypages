package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDocsConfig writes a config whose documentation site is the given URL.
func writeDocsConfig(t *testing.T, docs string) string {
	t.Helper()
	config := filepath.Join(t.TempDir(), "config.json")
	data := fmt.Sprintf(`{"domain":"example.com","docs":%q,"modules":[{"path":"m","repo":"https://github.com/acme/m"}]}`, docs)
	if err := os.WriteFile(config, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return config
}

func TestRunDetails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/package/example.com/m", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"modulePath":"example.com/m","version":"v1.2.3"}`); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("GET /v1/module/example.com/m", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, `{"commitTime":"2026-03-16T17:32:55Z"}`); err != nil {
			t.Error(err)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "site")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", writeDocsConfig(t, srv.URL), "-out", out, "-details"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	page, err := os.ReadFile(filepath.Join(out, "m", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "v1.2.3") {
		t.Error("page has no version")
	}
}

func TestRunDetailsFailureIsOnlyAWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "site")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", writeDocsConfig(t, srv.URL), "-out", out, "-details"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Errorf("stderr = %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "m", "index.html")); err != nil {
		t.Error(err)
	}
}

func TestRunDetailsReportsInvalidConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(config, []byte(`{"domain":"example.com","modules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", config, "-details"}, &stdout, &stderr); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunGeneratesSite(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	data := `{"domain":"example.com","modules":[{"path":"m","repo":"https://github.com/acme/m","submodules":[{"path":"sub"}]}]}`
	if err := os.WriteFile(config, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "site")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", config, "-out", out}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "generated example.com") {
		t.Errorf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(out, "m", "sub", "index.html")); err != nil {
		t.Error(err)
	}
}

func TestRunReportsInvalidConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(config, []byte(`{"domain":"example.com","modules":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", config}, &stdout, &stderr); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "modules:") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRunMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", filepath.Join(t.TempDir(), "nope.json")}, &stdout, &stderr); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
}

func TestRunPrintsSchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-schema"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "draft-07") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestRunReportsOutputErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"schema":  {"-schema"},
		"version": {"-version"},
		"generate": {"-config", writeDocsConfig(t, "https://pkg.go.dev"),
			"-out", filepath.Join(t.TempDir(), "site")},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := run(args, failingWriter{}, &stderr); code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), io.ErrClosedPipe.Error()) {
				t.Errorf("stderr = %q, want output error", stderr.String())
			}
		})
	}
}
