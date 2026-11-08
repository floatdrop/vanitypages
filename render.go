package vanitypages

import (
	"bytes"
	"cmp"
	_ "embed"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/floatdrop/vanitypages/internal/assets"
)

const (
	defaultDocs = "https://pkg.go.dev"
	defaultVCS  = "git"
)

//go:embed vanitypages.schema.json
var schema []byte

// Schema returns the JSON Schema describing the config file format.
func Schema() []byte {
	return bytes.Clone(schema)
}

var templates = template.Must(template.ParseFS(assets.Templates, "templates/*.html"))

// forges maps well-known hosts to their source browsing URL suffixes. HEAD
// resolves to the default branch on all of them.
var forges = map[string]struct{ dir, file string }{
	"github.com":    {"/tree/HEAD{/dir}", "/blob/HEAD{/dir}/{file}#L{line}"},
	"gitlab.com":    {"/-/tree/HEAD{/dir}", "/-/blob/HEAD{/dir}/{file}#L{line}"},
	"bitbucket.org": {"/src/HEAD{/dir}", "/src/HEAD{/dir}/{file}#lines-{line}"},
	"codeberg.org":  {"/src/HEAD{/dir}", "/src/HEAD{/dir}/{file}#L{line}"},
}

// Generate renders the site and writes it to dir, creating dir if needed and
// overwriting files from earlier runs. Nothing is written if the config is
// invalid. Other files are left alone, so pages of modules removed from the
// config stay until dir is cleaned.
func (c Config) Generate(dir string) error {
	files, err := c.Render()
	if err != nil {
		return err
	}
	for name, data := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Render validates the config and returns the contents of every file of the
// site, keyed by slash-separated path relative to the site root.
func (c Config) Render() (map[string][]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	for name, data := range assets.Static() {
		files[assetsDir+"/"+name] = data
	}
	if c.Icon != "" {
		data, err := os.ReadFile(c.Icon)
		if err != nil {
			return nil, fmt.Errorf("icon: %w", err)
		}
		files[c.iconFile()] = data
	}

	s := newSite(c)
	index, err := render("index", s.indexPage(c.Modules))
	if err != nil {
		return nil, err
	}
	files["index.html"] = index

	for _, m := range c.Modules {
		for _, p := range s.modulePages(m) {
			html, err := render("package", p)
			if err != nil {
				return nil, fmt.Errorf("render %s: %w", p.ImportPath, err)
			}
			files[p.path+"/index.html"] = html
		}
	}
	return files, nil
}

func render(name string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type site struct {
	Domain      string
	Base        string // URL path the site is served under, without trailing slash
	Title       string
	Description string
	Icon        string // URL of the custom icon, if any
	docs        string
	details     Details
}

func newSite(c Config) site {
	base := ""
	if _, rest, ok := strings.Cut(c.Domain, "/"); ok {
		base = "/" + rest
	}
	icon := ""
	if c.Icon != "" {
		icon = base + "/" + c.iconFile()
	}
	return site{
		Icon:        icon,
		Domain:      c.Domain,
		Base:        base,
		Title:       cmp.Or(c.Title, c.Domain),
		Description: c.Description,
		docs:        strings.TrimRight(cmp.Or(c.Docs, defaultDocs), "/"),
		details:     c.Details,
	}
}

// synopsis returns the published synopsis of a site-relative path, if any.
func (s site) synopsis(path string) string {
	return s.details[s.Domain+"/"+path].Synopsis
}

func (s site) home() crumb {
	return crumb{Label: s.Domain, Href: s.href("")}
}

func (s site) href(path string) string {
	if path == "" {
		return s.Base + "/"
	}
	return s.Base + "/" + path + "/"
}

type indexPage struct {
	Site    site
	Crumbs  []crumb
	Modules []childLink
}

func (s site) indexPage(modules []Module) indexPage {
	page := indexPage{Site: s, Crumbs: []crumb{{Label: s.Title}}}
	for _, m := range modules {
		if m.Hidden {
			continue
		}
		var labels []string
		if badge := vcsBadge(m); badge != "" {
			labels = append(labels, badge)
		}
		if len(m.Versions) > 0 {
			labels = append(labels, latestVersion(m.Versions))
		}
		if n := len(m.Submodules); n > 0 {
			labels = append(labels, count(n, "submodule"))
		}
		if n := len(m.Packages); n > 0 {
			labels = append(labels, count(n, "package"))
		}
		page.Modules = append(page.Modules, childLink{
			Name:        m.Path,
			Href:        s.href(m.Path),
			Description: cmp.Or(m.Description, s.synopsis(advertised(m))),
			Labels:      labels,
		})
	}
	return page
}

// vcsBadge returns the version control label to show for m. Git is the norm,
// so it gets none.
func vcsBadge(m Module) string {
	if m.VCS == defaultVCS {
		return ""
	}
	return m.VCS
}

func latestVersion(versions []string) string {
	return slices.MaxFunc(versions, semver.Compare)
}

// advertised returns the path to install m from: its newest major version.
func advertised(m Module) string {
	if len(m.Versions) == 0 {
		return m.Path
	}
	return m.Path + "/" + latestVersion(m.Versions)
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// packagePage is the data of a module, submodule or package page.
type packagePage struct {
	Site       site
	Kind       string
	ImportPath string
	Install    string // import path shown in the usage snippets
	// GoImport and GoSource always name the repository root, even on pages
	// for nested paths: that's how the go command locates a package or
	// submodule within a repository.
	GoImport    string
	GoSource    string
	Description string
	Detail      Detail // zero if the path has no published details
	Docs        string
	Browse      string // link to this path in a source browser
	OnGitHub    bool   // Browse points at github.com
	Repo        string // repository URL without its scheme
	RepoHref    string // repository URL, if browsers can open it
	VCSBadge    string
	Crumbs      []crumb // ancestors, starting at the site root
	Current     string  // page heading: the path relative to its parent crumb
	Submodules  []childLink
	Packages    []childLink

	path string // site-relative directory
}

// crumb is a header item: a link to an ancestor or, without Href, the heading
// of the current page.
type crumb struct {
	Label string
	Href  string
}

type childLink struct {
	Name        string
	Description string
	Href        string
	Labels      []string
}

// repository holds what every page of a module shares.
type repository struct {
	root     string // site-relative path of the repository root
	goImport string
	goSource string
	badge    string
	display  string
	href     string
	source   *Source
}

func (s site) repository(m Module) repository {
	importRoot := s.Domain + "/" + m.Path
	r := repository{
		root:     m.Path,
		goImport: strings.Join([]string{importRoot, cmp.Or(m.VCS, defaultVCS), m.Repo}, " "),
		badge:    vcsBadge(m),
		display:  m.Repo,
		source:   m.Source,
	}
	if u, err := url.Parse(m.Repo); err == nil {
		r.display = strings.TrimSuffix(u.Host+u.Path, "/")
		if u.Scheme == "http" || u.Scheme == "https" {
			r.href = m.Repo
		}
		if r.source == nil {
			r.source = detectSource(u)
		}
	}
	if r.source != nil {
		r.goSource = strings.Join([]string{importRoot, r.source.Home, r.source.Dir, r.source.File}, " ")
	}
	return r
}

func detectSource(repo *url.URL) *Source {
	host := strings.ToLower(repo.Hostname())
	forge, ok := forges[host]
	if !ok {
		return nil
	}
	home := "https://" + host + strings.TrimSuffix(strings.TrimRight(repo.Path, "/"), ".git")
	return &Source{Home: home, Dir: home + forge.dir, File: home + forge.file}
}

func onGitHub(link string) bool {
	u, err := url.Parse(link)
	return err == nil && strings.EqualFold(u.Hostname(), "github.com")
}

// browse returns a link to the directory at the given site-relative path.
func (r repository) browse(path string) string {
	if r.source == nil {
		return r.href
	}
	dir := strings.TrimPrefix(strings.TrimPrefix(path, r.root), "/")
	if dir == "" {
		return r.source.Home
	}
	return strings.NewReplacer("{/dir}", "/"+dir, "{dir}", dir).Replace(r.source.Dir)
}

// pageSpec is what differs between the pages of a module.
type pageSpec struct {
	kind        string
	path        string
	install     string // path to advertise, if not path
	browsePath  string // path whose source to link to, if not path
	description string
	trail       []crumb
	submodules  []Submodule
	packages    []string
}

func (s site) modulePages(m Module) []packagePage {
	repo := s.repository(m)
	moduleTrail := []crumb{s.home(), {Label: m.Path, Href: s.href(m.Path)}}
	newest := advertised(m)

	specs := []pageSpec{{
		kind:        "module",
		path:        m.Path,
		install:     newest,
		description: m.Description,
		trail:       moduleTrail,
		submodules:  m.Submodules,
		packages:    m.Packages,
	}}
	packageTrail := moduleTrail
	for _, version := range m.Versions {
		path := m.Path + "/" + version
		spec := pageSpec{
			kind:        "module",
			path:        path,
			browsePath:  m.Path,
			description: m.Description,
			trail:       append(slices.Clip(moduleTrail), crumb{Label: version, Href: s.href(path)}),
		}
		if path == newest {
			spec.packages = m.Packages
			packageTrail = spec.trail
		}
		specs = append(specs, spec)
	}
	specs = appendPackages(specs, s, newest, m.Path, packageTrail, m.Packages)
	for _, sub := range m.Submodules {
		path := m.Path + "/" + sub.Path
		trail := append(slices.Clip(moduleTrail), crumb{Label: sub.Path, Href: s.href(path)})
		specs = append(specs, pageSpec{
			kind:        "submodule",
			path:        path,
			description: sub.Description,
			trail:       trail,
			packages:    sub.Packages,
		})
		specs = appendPackages(specs, s, path, path, trail, sub.Packages)
	}

	pages := make([]packagePage, 0, len(specs))
	for _, spec := range specs {
		pages = append(pages, s.packagePage(repo, spec))
	}
	return pages
}

// appendPackages adds pages for packages under the import path owner, whose
// code is at sourceDir.
func appendPackages(specs []pageSpec, s site, owner, sourceDir string, trail []crumb, packages []string) []pageSpec {
	for _, p := range packages {
		path := owner + "/" + p
		specs = append(specs, pageSpec{
			kind:       "package",
			path:       path,
			browsePath: sourceDir + "/" + p,
			trail:      append(slices.Clip(trail), crumb{Label: p, Href: s.href(path)}),
		})
	}
	return specs
}

func (s site) packagePage(repo repository, spec pageSpec) packagePage {
	importPath := s.Domain + "/" + spec.path
	last := len(spec.trail) - 1
	install := s.Domain + "/" + cmp.Or(spec.install, spec.path)
	browse := repo.browse(cmp.Or(spec.browsePath, spec.path))
	detail := s.details[install]
	page := packagePage{
		Site:        s,
		Kind:        spec.kind,
		ImportPath:  importPath,
		Install:     install,
		GoImport:    repo.goImport,
		GoSource:    repo.goSource,
		Description: cmp.Or(spec.description, detail.Synopsis),
		Detail:      detail,
		Docs:        s.docs + "/" + install,
		Browse:      browse,
		OnGitHub:    onGitHub(browse),
		Repo:        repo.display,
		RepoHref:    repo.href,
		VCSBadge:    repo.badge,
		Crumbs:      spec.trail[:last],
		Current:     spec.trail[last].Label,
		path:        spec.path,
	}
	for _, sub := range spec.submodules {
		path := spec.path + "/" + sub.Path
		page.Submodules = append(page.Submodules, childLink{
			Name:        sub.Path,
			Description: cmp.Or(sub.Description, s.synopsis(path)),
			Href:        s.href(path),
		})
	}
	for _, p := range spec.packages {
		path := cmp.Or(spec.install, spec.path) + "/" + p
		page.Packages = append(page.Packages, childLink{Name: p, Description: s.synopsis(path), Href: s.href(path)})
	}
	return page
}
