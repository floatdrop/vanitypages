package vanitypages

import (
	"bytes"
	"net/http"
	"strings"
	"time"
)

// Handler validates and renders the site once, then returns an HTTP handler
// that serves the pages and assets from memory. No files are written and no
// details are fetched. Changes to the config require creating a new handler.
//
// The handler accepts GET and HEAD requests. Paths include the path in Domain:
// for "example.com/go", mount the handler at "/go" without stripping the prefix.
// Only generated files and their directory index URLs are served; other paths
// return 404.
func (c Config) Handler() (http.Handler, error) {
	files, err := c.Render()
	if err != nil {
		return nil, err
	}

	base := newSite(c).Base
	mux := http.NewServeMux()
	for name, data := range files {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
		})
		path := base + "/" + name
		mux.Handle("GET "+path, handler)
		if name == "index.html" || strings.HasSuffix(name, "/index.html") {
			mux.Handle("GET "+strings.TrimSuffix(path, "index.html")+"{$}", handler)
		}
	}
	return mux, nil
}
