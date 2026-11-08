// Package vanitypages generates static vanity import pages for Go modules,
// styled with the Gravity UI design system.
//
// Describe a site with a [Config], either in Go or in a JSON file read by
// [Load], then call [Config.Generate] to write it to a directory:
//
//	site := vanitypages.Config{
//		Domain: "go.example.com",
//		Modules: []vanitypages.Module{
//			{Path: "foo", Repo: "https://github.com/acme/foo"},
//		},
//	}
//	err := site.Generate("public")
package vanitypages

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/mod/module"
)

// assetsDir is the site directory holding shared assets. Import paths can't
// start with it, so it never collides with a generated page.
const assetsDir = "-"

var (
	vcsTypes     = []string{"git", "hg", "svn", "bzr", "fossil", "mod"}
	iconTypes    = []string{".svg", ".png", ".ico", ".webp"}
	majorVersion = regexp.MustCompile(`^v([2-9]|[1-9][0-9]+)$`)
)

// Config describes a vanity import site.
type Config struct {
	// Schema is the JSON Schema reference editors use. It has no effect on
	// the generated site.
	Schema string `json:"$schema,omitempty"`
	// Domain is the host the site is served from, optionally with a path,
	// such as "go.example.com" or "example.com/go".
	Domain string `json:"domain"`
	// Title is the index page heading, shown in the header. It defaults to
	// Domain.
	Title string `json:"title,omitempty"`
	// Description is shown at the top of the index page.
	Description string `json:"description,omitempty"`
	// Icon is the path of an .svg, .png, .ico or .webp image shown in the
	// header and as the favicon, instead of a generic icon. [Load] resolves
	// it against the directory of the config file; otherwise it is relative
	// to the working directory.
	Icon string `json:"icon,omitempty"`
	// Docs is the base URL of the documentation site; import paths are
	// appended to it. It defaults to https://pkg.go.dev.
	Docs string `json:"docs,omitempty"`
	// Modules are the modules served from Domain, listed in this order on
	// the index page.
	Modules []Module `json:"modules"`
	// Details adds published versions, licenses and synopses to the pages. It
	// isn't part of the config file; [Config.FetchDetails] provides it.
	Details Details `json:"-"`
}

// Module is a Go module whose root is the root of a repository.
type Module struct {
	// Path is the module path relative to the domain.
	Path string `json:"path"`
	// Repo is the URL of the repository.
	Repo string `json:"repo"`
	// VCS is the version control system: git, hg, svn, bzr, fossil or mod.
	// It defaults to git.
	VCS string `json:"vcs,omitempty"`
	// Description is shown on the index and module pages.
	Description string `json:"description,omitempty"`
	// Versions lists major versions above v1, such as "v2", developed at the
	// repository root with a go.mod declaring the /vN module path. Each one
	// gets a page, which the go command needs, and the module's page
	// advertises the newest. A major version kept in its own directory is a
	// [Submodule] instead.
	Versions []string `json:"versions,omitempty"`
	// Packages lists package paths inside the module, relative to its root,
	// that should get a page of their own. With Versions, the pages are for
	// the newest major version, at import paths such as foo/v2/pkg.
	Packages []string `json:"packages,omitempty"`
	// Submodules lists Go modules nested in this module's repository.
	Submodules []Submodule `json:"submodules,omitempty"`
	// Source overrides the source links, which are detected automatically
	// for repositories on github.com, gitlab.com, bitbucket.org and
	// codeberg.org.
	Source *Source `json:"source,omitzero"`
	// Hidden keeps the module off the index page. Its pages are still
	// generated, so it remains importable.
	Hidden bool `json:"hidden,omitzero"`
}

// Submodule is a Go module with its own go.mod inside the parent module's
// repository, versioned with tags prefixed by its directory, such as
// contrib/redis/v1.2.3.
type Submodule struct {
	// Path is relative to the parent module, and is also the module's
	// directory in the repository.
	Path string `json:"path"`
	// Description is shown on the parent module's page and the submodule's
	// own page.
	Description string `json:"description,omitempty"`
	// Packages lists package paths inside the submodule, relative to Path,
	// that should get a page of their own.
	Packages []string `json:"packages,omitempty"`
}

// Source holds the URL templates of a go-source meta tag. Dir and File may use
// the placeholders {dir}, {/dir}, {file} and {line}.
type Source struct {
	Home string `json:"home"`
	Dir  string `json:"dir"`
	File string `json:"file"`
}

// Load reads the JSON config file at path. See [Parse].
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	c, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Icon != "" && !filepath.IsAbs(c.Icon) {
		c.Icon = filepath.Join(filepath.Dir(path), filepath.FromSlash(c.Icon))
	}
	return c, nil
}

// Parse decodes a JSON config, rejecting unknown fields. It does not check the
// values; [Config.Validate], [Config.Render] and [Config.Generate] do.
func Parse(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c, json.RejectUnknownMembers(true)); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate reports every problem in the config, using the same path rules as
// the go command.
func (c Config) Validate() error {
	if err := module.CheckPath(c.Domain); err != nil {
		return fmt.Errorf("domain: %w", err)
	}
	v := validator{domain: c.Domain, pages: make(map[string]string)}
	if c.Icon != "" && !slices.Contains(iconTypes, filepath.Ext(c.Icon)) {
		v.add("icon", fmt.Errorf("%q must end in one of %s", c.Icon, strings.Join(iconTypes, ", ")))
	}
	if c.Docs != "" {
		v.add("docs", checkWebURL(c.Docs))
	}
	if len(c.Modules) == 0 {
		v.add("modules", errors.New("at least one module is required"))
	}
	for i, m := range c.Modules {
		v.module(fmt.Sprintf("modules[%d]", i), m)
	}
	return errors.Join(v.errs...)
}

// iconFile returns the site-relative path the icon is published at.
func (c Config) iconFile() string {
	return assetsDir + "/icon" + filepath.Ext(c.Icon)
}

type validator struct {
	domain string
	pages  map[string]string // lowercase page path to the field that declares it
	errs   []error
}

func (v *validator) add(field string, err error) {
	if err != nil {
		v.errs = append(v.errs, fmt.Errorf("%s: %w", field, err))
	}
}

// path checks a path relative to the domain and claims its page.
func (v *validator) path(field, path string, check func(string) error) {
	// Pages become files, and file systems may ignore case.
	key := strings.ToLower(path)
	switch {
	case strings.HasSuffix(path, "/") || strings.HasPrefix(path, "/"):
		v.add(field, fmt.Errorf("%q must not start or end with a slash", path))
	case v.pages[key] != "":
		v.add(field, fmt.Errorf("%q is already declared by %s", path, v.pages[key]))
	case slices.Contains(strings.Split(key, "/"), "index.html"):
		v.add(field, fmt.Errorf("%q must not have an index.html element: pages are written to files of that name", path))
	default:
		if err := check(v.domain + "/" + path); err != nil {
			v.add(field, err)
			return
		}
		v.pages[key] = field
	}
}

func (v *validator) module(field string, m Module) {
	if first, _, _ := strings.Cut(m.Path, "/"); first == assetsDir {
		v.add(field+".path", fmt.Errorf("%q is reserved for generated assets", assetsDir))
	} else if m.Path == "" {
		v.add(field+".path", errors.New("required"))
	} else {
		v.path(field+".path", m.Path, module.CheckPath)
	}

	v.add(field+".repo", checkRepoURL(m.Repo))
	if m.VCS != "" && !slices.Contains(vcsTypes, m.VCS) {
		v.add(field+".vcs", fmt.Errorf("%q is not one of %s", m.VCS, strings.Join(vcsTypes, ", ")))
	}
	if m.Source != nil {
		v.add(field+".source.home", checkWebURL(m.Source.Home))
		v.add(field+".source.dir", checkWebURL(m.Source.Dir))
		v.add(field+".source.file", checkWebURL(m.Source.File))
	}

	for i, version := range m.Versions {
		versionField := fmt.Sprintf("%s.versions[%d]", field, i)
		if !majorVersion.MatchString(version) {
			v.add(versionField, fmt.Errorf("%q is not a major version above v1, such as v2", version))
			continue
		}
		v.path(versionField, m.Path+"/"+version, module.CheckPath)
	}

	v.packages(field, advertised(m), m.Packages)
	for i, sub := range m.Submodules {
		subField := fmt.Sprintf("%s.submodules[%d]", field, i)
		if sub.Path == "" {
			v.add(subField+".path", errors.New("required"))
			continue
		}
		v.path(subField+".path", m.Path+"/"+sub.Path, module.CheckPath)
		v.packages(subField, m.Path+"/"+sub.Path, sub.Packages)
	}
}

func (v *validator) packages(field, owner string, packages []string) {
	for i, p := range packages {
		pkgField := fmt.Sprintf("%s.packages[%d]", field, i)
		if p == "" {
			v.add(pkgField, errors.New("must not be empty"))
			continue
		}
		v.path(pkgField, owner+"/"+p, module.CheckImportPath)
	}
}

func parseURL(s string) (*url.URL, error) {
	if s == "" {
		return nil, errors.New("required")
	}
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return nil, fmt.Errorf("%q must not contain whitespace", s)
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%q must be an absolute URL", s)
	}
	return u, nil
}

// checkRepoURL applies the rules the go command has for a go-import repo.
func checkRepoURL(s string) error {
	u, err := parseURL(s)
	if err != nil {
		return err
	}
	if u.Scheme == "file" {
		return fmt.Errorf("%q: the go command refuses file URLs", s)
	}
	return nil
}

func checkWebURL(s string) error {
	u, err := parseURL(s)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must be an http or https URL", s)
	}
	return nil
}
