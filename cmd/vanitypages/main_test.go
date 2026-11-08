package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
