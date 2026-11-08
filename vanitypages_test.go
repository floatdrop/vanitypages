package vanitypages

import (
	"encoding/json/v2"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func exampleConfig() Config {
	return Config{
		Domain: "go.example.com",
		Modules: []Module{{
			Path:        "cache",
			Repo:        "https://github.com/acme/cache",
			Description: "Fast <cache> & more",
			Packages:    []string{"lru"},
			Submodules: []Submodule{{
				Path:        "contrib/redis",
				Description: "Redis backend",
				Packages:    []string{"cluster"},
			}},
		}},
	}
}

// parseMeta extracts meta tags the way cmd/go does: a non-strict XML decoder
// that stops at the first body element.
func parseMeta(t *testing.T, html []byte) map[string]string {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(string(html)))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	meta := make(map[string]string)
	for {
		tok, err := d.RawToken()
		if err == io.EOF {
			return meta
		}
		if err != nil {
			t.Fatalf("parse html: %v", err)
		}
		if e, ok := tok.(xml.EndElement); ok && strings.EqualFold(e.Name.Local, "head") {
			return meta
		}
		e, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if strings.EqualFold(e.Name.Local, "body") {
			return meta
		}
		if !strings.EqualFold(e.Name.Local, "meta") {
			continue
		}
		var name, content string
		for _, a := range e.Attr {
			switch a.Name.Local {
			case "name":
				name = a.Value
			case "content":
				content = a.Value
			}
		}
		meta[name] = content
	}
}

func TestRenderMetaTags(t *testing.T) {
	files, err := exampleConfig().Render()
	if err != nil {
		t.Fatal(err)
	}
	const (
		goImport = "go.example.com/cache git https://github.com/acme/cache"
		goSource = "go.example.com/cache https://github.com/acme/cache " +
			"https://github.com/acme/cache/tree/HEAD{/dir} " +
			"https://github.com/acme/cache/blob/HEAD{/dir}/{file}#L{line}"
	)
	// Every page names the repository root, including nested modules.
	for _, name := range []string{
		"cache/index.html",
		"cache/lru/index.html",
		"cache/contrib/redis/index.html",
		"cache/contrib/redis/cluster/index.html",
	} {
		html, ok := files[name]
		if !ok {
			t.Fatalf("missing page %s", name)
		}
		meta := parseMeta(t, html)
		if got := meta["go-import"]; got != goImport {
			t.Errorf("%s go-import = %q, want %q", name, got, goImport)
		}
		if got := meta["go-source"]; got != goSource {
			t.Errorf("%s go-source = %q, want %q", name, got, goSource)
		}
	}
	if meta := parseMeta(t, files["index.html"]); meta["go-import"] != "" {
		t.Errorf("index has go-import %q", meta["go-import"])
	}
}

func TestRenderSubmodulePage(t *testing.T) {
	files, err := exampleConfig().Render()
	if err != nil {
		t.Fatal(err)
	}

	parent := string(files["cache/index.html"])
	for _, want := range []string{"Submodules", `href="/cache/contrib/redis/"`, "Redis backend"} {
		if !strings.Contains(parent, want) {
			t.Errorf("module page is missing %q", want)
		}
	}

	sub := string(files["cache/contrib/redis/index.html"])
	for _, want := range []string{
		">Submodule<",
		"go get go.example.com/cache/contrib/redis",
		"https://pkg.go.dev/go.example.com/cache/contrib/redis",
		"https://github.com/acme/cache/tree/HEAD/contrib/redis",
		`href="/cache/contrib/redis/cluster/"`,
	} {
		if !strings.Contains(sub, want) {
			t.Errorf("submodule page is missing %q", want)
		}
	}
}

func TestRenderEscapesConfigValues(t *testing.T) {
	files, err := exampleConfig().Render()
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if strings.HasSuffix(name, ".html") && strings.Contains(string(data), "ZgotmplZ") {
			t.Errorf("%s contains a rejected template value", name)
		}
	}
	page := string(files["cache/index.html"])
	if strings.Contains(page, "<cache>") || !strings.Contains(page, "Fast &lt;cache&gt; &amp; more") {
		t.Error("description is not HTML-escaped")
	}
}

func TestRenderDomainWithPath(t *testing.T) {
	cfg := exampleConfig()
	cfg.Domain = "example.com/go"
	files, err := cfg.Render()
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["cache/lru/index.html"])
	for _, want := range []string{`href="/go/-/vanitypages.css"`, `href="/go/cache/"`, "go get example.com/go/cache/lru"} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q", want)
		}
	}
}

func TestRenderMajorVersions(t *testing.T) {
	cfg := Config{Domain: "example.com", Modules: []Module{{
		Path:     "m",
		Repo:     "https://github.com/acme/m",
		Versions: []string{"v2", "v10", "v3"},
	}}}
	files, err := cfg.Render()
	if err != nil {
		t.Fatal(err)
	}

	const goImport = "example.com/m git https://github.com/acme/m"
	for _, name := range []string{"m/index.html", "m/v2/index.html", "m/v3/index.html", "m/v10/index.html"} {
		if got := parseMeta(t, files[name])["go-import"]; got != goImport {
			t.Errorf("%s go-import = %q, want %q", name, got, goImport)
		}
	}

	root := string(files["m/index.html"])
	for _, want := range []string{"go get example.com/m/v10", "https://pkg.go.dev/example.com/m/v10"} {
		if !strings.Contains(root, want) {
			t.Errorf("module page is missing %q", want)
		}
	}
	if !strings.Contains(string(files["index.html"]), ">v10<") {
		t.Error("index does not label the newest major version")
	}

	// The code of a major version lives at the repository root.
	version := string(files["m/v2/index.html"])
	if !strings.Contains(version, "go get example.com/m/v2") {
		t.Error("version page does not advertise its own path")
	}
	if !strings.Contains(version, `href="https://github.com/acme/m"`) || strings.Contains(version, "tree/HEAD/v2") {
		t.Error("version page should link to the repository root")
	}
}

func TestHiddenModuleIsImportableButNotListed(t *testing.T) {
	cfg := exampleConfig()
	cfg.Modules = append(cfg.Modules, Module{Path: "secret", Repo: "https://github.com/acme/secret", Hidden: true})
	files, err := cfg.Render()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(files["index.html"]), "secret") {
		t.Error("hidden module is listed on the index")
	}
	if _, ok := files["secret/index.html"]; !ok {
		t.Error("hidden module has no page")
	}
}

func TestSource(t *testing.T) {
	tests := []struct {
		name         string
		repo         string
		source       *Source
		wantGoSource string
		wantBrowse   string
	}{
		{
			name:         "strips .git and normalizes ssh",
			repo:         "ssh://git@github.com/acme/m.git",
			wantGoSource: "example.com/m https://github.com/acme/m https://github.com/acme/m/tree/HEAD{/dir} https://github.com/acme/m/blob/HEAD{/dir}/{file}#L{line}",
			wantBrowse:   "https://github.com/acme/m/tree/HEAD/sub",
		},
		{
			name:         "gitlab",
			repo:         "https://gitlab.com/group/sub/m",
			wantGoSource: "example.com/m https://gitlab.com/group/sub/m https://gitlab.com/group/sub/m/-/tree/HEAD{/dir} https://gitlab.com/group/sub/m/-/blob/HEAD{/dir}/{file}#L{line}",
			wantBrowse:   "https://gitlab.com/group/sub/m/-/tree/HEAD/sub",
		},
		{
			name:         "bitbucket",
			repo:         "https://bitbucket.org/acme/m",
			wantGoSource: "example.com/m https://bitbucket.org/acme/m https://bitbucket.org/acme/m/src/HEAD{/dir} https://bitbucket.org/acme/m/src/HEAD{/dir}/{file}#lines-{line}",
			wantBrowse:   "https://bitbucket.org/acme/m/src/HEAD/sub",
		},
		{
			name:         "codeberg",
			repo:         "https://codeberg.org/acme/m",
			wantGoSource: "example.com/m https://codeberg.org/acme/m https://codeberg.org/acme/m/src/HEAD{/dir} https://codeberg.org/acme/m/src/HEAD{/dir}/{file}#L{line}",
			wantBrowse:   "https://codeberg.org/acme/m/src/HEAD/sub",
		},
		{
			name:       "unknown host has no go-source",
			repo:       "https://git.example.org/m",
			wantBrowse: "https://git.example.org/m",
		},
		{
			name:         "explicit source wins",
			repo:         "https://github.com/acme/m",
			source:       &Source{Home: "https://h.dev/m", Dir: "https://h.dev/m{/dir}", File: "https://h.dev/m{/dir}/{file}"},
			wantGoSource: "example.com/m https://h.dev/m https://h.dev/m{/dir} https://h.dev/m{/dir}/{file}",
			wantBrowse:   "https://h.dev/m/sub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Domain: "example.com", Modules: []Module{{
				Path: "m", Repo: tt.repo, Source: tt.source, Packages: []string{"sub"},
			}}}
			files, err := cfg.Render()
			if err != nil {
				t.Fatal(err)
			}
			html := files["m/sub/index.html"]
			if got := parseMeta(t, html)["go-source"]; got != tt.wantGoSource {
				t.Errorf("go-source = %q, want %q", got, tt.wantGoSource)
			}
			if !strings.Contains(string(html), `href="`+tt.wantBrowse+`"`) {
				t.Errorf("page does not link to %s", tt.wantBrowse)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"scheme in domain", func(c *Config) { c.Domain = "https://example.com" }, "domain:"},
		{"no domain", func(c *Config) { c.Domain = "" }, "domain:"},
		{"no modules", func(c *Config) { c.Modules = nil }, "modules:"},
		{"bad docs", func(c *Config) { c.Docs = "pkg.go.dev" }, "docs:"},
		{"no path", func(c *Config) { c.Modules[0].Path = "" }, "modules[0].path:"},
		{"traversal", func(c *Config) { c.Modules[0].Path = "../evil" }, "modules[0].path:"},
		{"leading slash", func(c *Config) { c.Modules[0].Path = "/cache" }, "modules[0].path:"},
		{"reserved", func(c *Config) { c.Modules[0].Path = "-/assets" }, "reserved"},
		{"bad vcs", func(c *Config) { c.Modules[0].VCS = "cvs" }, "modules[0].vcs:"},
		{"scp repo", func(c *Config) { c.Modules[0].Repo = "git@github.com:acme/x.git" }, "modules[0].repo:"},
		{"file repo", func(c *Config) { c.Modules[0].Repo = "file:///srv/x" }, "modules[0].repo:"},
		{"space in repo", func(c *Config) { c.Modules[0].Repo = "https://a.dev/b c" }, "whitespace"},
		{"partial source", func(c *Config) { c.Modules[0].Source = &Source{Home: "https://a.dev"} }, "source.dir:"},
		{"package traversal", func(c *Config) { c.Modules[0].Packages = []string{"../../x"} }, "packages[0]:"},
		{"empty package", func(c *Config) { c.Modules[0].Packages = []string{""} }, "packages[0]:"},
		{"submodule traversal", func(c *Config) { c.Modules[0].Submodules[0].Path = "../x" }, "submodules[0].path:"},
		{"empty submodule", func(c *Config) { c.Modules[0].Submodules[0].Path = "" }, "submodules[0].path:"},
		{"bad major version", func(c *Config) { c.Modules[0].Submodules[0].Path = "v1" }, "submodules[0].path:"},
		{"submodule package traversal", func(c *Config) { c.Modules[0].Submodules[0].Packages = []string{"../.."} }, "packages[0]:"},
		{"version v1", func(c *Config) { c.Modules[0].Versions = []string{"v1"} }, "versions[0]:"},
		{"full version", func(c *Config) { c.Modules[0].Versions = []string{"v2.1.0"} }, "versions[0]:"},
		{"version collides with submodule", func(c *Config) {
			c.Modules[0].Versions = []string{"v2"}
			c.Modules[0].Submodules[0].Path = "v2"
		}, "already declared"},
		{"duplicate package", func(c *Config) { c.Modules[0].Packages = []string{"lru", "lru"} }, "already declared"},
		{"submodule collides with package", func(c *Config) { c.Modules[0].Packages = []string{"contrib/redis"} }, "already declared"},
		{"duplicate module", func(c *Config) { c.Modules = append(c.Modules, c.Modules[0]) }, "already declared"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := exampleConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate succeeded")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	cfg := exampleConfig()
	cfg.Modules[0].Submodules = append(cfg.Modules[0].Submodules, Submodule{Path: "v2"})
	cfg.Modules = append(cfg.Modules, Module{Path: "cache/other", Repo: "https://github.com/acme/other", VCS: "hg"})
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	cfg := exampleConfig()
	cfg.Modules[0].Repo = ""
	cfg.Modules[0].VCS = "cvs"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate succeeded")
	}
	for _, want := range []string{"modules[0].repo:", "modules[0].vcs:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestParse(t *testing.T) {
	cfg, err := Parse([]byte(`{"$schema":"x","domain":"example.com","modules":[{"path":"m","repo":"https://a.dev/m"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Schema != "x" || cfg.Domain != "example.com" || cfg.Modules[0].Path != "m" {
		t.Errorf("Parse = %+v", cfg)
	}
	if _, err := Parse([]byte(`{"domain":"example.com","moduels":[]}`)); err == nil {
		t.Error("Parse accepted an unknown field")
	}
}

func TestGenerateOverwrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "public")
	cfg := exampleConfig()
	if err := cfg.Generate(dir); err != nil {
		t.Fatal(err)
	}
	cfg.Modules[0].Description = "Updated"
	if err := cfg.Generate(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cache", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Updated") {
		t.Error("second Generate did not overwrite the page")
	}
	if _, err := os.Stat(filepath.Join(dir, "-", "vanitypages.css")); err != nil {
		t.Error(err)
	}
}

func TestGenerateWritesNothingWhenInvalid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "public")
	if err := (Config{Domain: "example.com"}).Generate(dir); err == nil {
		t.Fatal("Generate succeeded")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("output directory exists: %v", err)
	}
}

func TestRepositoryConfig(t *testing.T) {
	cfg, err := Load("vanitypages.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

// TestSchemaMatchesConfig keeps the JSON Schema in step with the Go types.
func TestSchemaMatchesConfig(t *testing.T) {
	var doc struct {
		Definitions map[string]schemaObject `json:"definitions"`
		schemaObject
	}
	if err := json.Unmarshal(Schema(), &doc); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		object schemaObject
		typ    reflect.Type
	}{
		{"config", doc.schemaObject, reflect.TypeFor[Config]()},
		{"module", doc.Definitions["module"], reflect.TypeFor[Module]()},
		{"submodule", doc.Definitions["submodule"], reflect.TypeFor[Submodule]()},
		{"source", doc.Definitions["source"], reflect.TypeFor[Source]()},
	} {
		var fields, required []string
		for field := range tt.typ.Fields() {
			name, opts, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields = append(fields, name)
			if opts == "" {
				required = append(required, name)
			}
		}
		var props []string
		for name := range tt.object.Properties {
			props = append(props, name)
		}
		slices.Sort(fields)
		slices.Sort(props)
		slices.Sort(required)
		wantRequired := slices.Sorted(slices.Values(tt.object.Required))
		if !slices.Equal(fields, props) {
			t.Errorf("%s: Go fields %v, schema properties %v", tt.name, fields, props)
		}
		if !slices.Equal(required, wantRequired) {
			t.Errorf("%s: Go required %v, schema required %v", tt.name, required, wantRequired)
		}
	}
}

type schemaObject struct {
	Properties map[string]any `json:"properties"`
	Required   []string       `json:"required"`
}
