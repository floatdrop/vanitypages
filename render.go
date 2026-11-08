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
	docs        string
}

func newSite(c Config) site {
	base := ""
	if _, rest, ok := strings.Cut(c.Domain, "/"); ok {
		base = "/" + rest
	}
	return site{
		Domain:      c.Domain,
		Base:        base,
		Title:       cmp.Or(c.Title, c.Domain),
		Description: c.Description,
		docs:        strings.TrimRight(cmp.Or(c.Docs, defaultDocs), "/"),
	}
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
	Modules []moduleCard
}

type moduleCard struct {
	Name        string
	Href        string
	Description string
	Labels      []string
}

func (s site) indexPage(modules []Module) indexPage {
	page := indexPage{Site: s}
	if s.Title != s.Domain {
		page.Crumbs = []crumb{s.home()}
	}
	for _, m := range modules {
		if m.Hidden {
			continue
		}
		labels := []string{cmp.Or(m.VCS, defaultVCS)}
		if len(m.Versions) > 0 {
			labels = append(labels, latestVersion(m.Versions))
		}
		if n := len(m.Submodules); n > 0 {
			labels = append(labels, count(n, "submodule"))
		}
		if n := len(m.Packages); n > 0 {
			labels = append(labels, count(n, "package"))
		}
		page.Modules = append(page.Modules, moduleCard{
			Name:        m.Path,
			Href:        s.href(m.Path),
			Description: m.Description,
			Labels:      labels,
		})
	}
	return page
}

func latestVersion(versions []string) string {
	return slices.MaxFunc(versions, semver.Compare)
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
	Docs        string
	Browse      string // link to this path in a source browser
	OnGitHub    bool   // Browse points at github.com
	Repo        string // repository URL without its scheme
	RepoHref    string // repository URL, if browsers can open it
	VCS         string
	Crumbs      []crumb // ancestors, starting at the site root
	Current     string  // page heading: the path relative to its parent crumb
	Submodules  []childLink
	Packages    []childLink

	path string // site-relative directory
}

type crumb struct {
	Label string
	Href  string
}

type childLink struct {
	Name        string
	Description string
	Href        string
}

// repository holds what every page of a module shares.
type repository struct {
	root     string // site-relative path of the repository root
	goImport string
	goSource string
	vcs      string
	display  string
	href     string
	source   *Source
}

func (s site) repository(m Module) repository {
	vcs := cmp.Or(m.VCS, defaultVCS)
	importRoot := s.Domain + "/" + m.Path
	r := repository{
		root:     m.Path,
		goImport: strings.Join([]string{importRoot, vcs, m.Repo}, " "),
		vcs:      vcs,
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

	root := pageSpec{
		kind:        "Module",
		path:        m.Path,
		description: m.Description,
		trail:       moduleTrail,
		submodules:  m.Submodules,
		packages:    m.Packages,
	}
	if len(m.Versions) > 0 {
		root.install = m.Path + "/" + latestVersion(m.Versions)
	}
	specs := []pageSpec{root}
	for _, version := range m.Versions {
		path := m.Path + "/" + version
		specs = append(specs, pageSpec{
			kind:        "Module",
			path:        path,
			browsePath:  m.Path,
			description: m.Description,
			trail:       append(slices.Clip(moduleTrail), crumb{Label: version, Href: s.href(path)}),
		})
	}
	specs = appendPackages(specs, s, m.Path, moduleTrail, m.Packages)
	for _, sub := range m.Submodules {
		path := m.Path + "/" + sub.Path
		trail := append(slices.Clip(moduleTrail), crumb{Label: sub.Path, Href: s.href(path)})
		specs = append(specs, pageSpec{
			kind:        "Submodule",
			path:        path,
			description: sub.Description,
			trail:       trail,
			packages:    sub.Packages,
		})
		specs = appendPackages(specs, s, path, trail, sub.Packages)
	}

	pages := make([]packagePage, 0, len(specs))
	for _, spec := range specs {
		pages = append(pages, s.packagePage(repo, spec))
	}
	return pages
}

func appendPackages(specs []pageSpec, s site, owner string, trail []crumb, packages []string) []pageSpec {
	for _, p := range packages {
		path := owner + "/" + p
		specs = append(specs, pageSpec{
			kind:  "Package",
			path:  path,
			trail: append(slices.Clip(trail), crumb{Label: p, Href: s.href(path)}),
		})
	}
	return specs
}

func (s site) packagePage(repo repository, spec pageSpec) packagePage {
	importPath := s.Domain + "/" + spec.path
	last := len(spec.trail) - 1
	install := s.Domain + "/" + cmp.Or(spec.install, spec.path)
	browse := repo.browse(cmp.Or(spec.browsePath, spec.path))
	page := packagePage{
		Site:        s,
		Kind:        spec.kind,
		ImportPath:  importPath,
		Install:     install,
		GoImport:    repo.goImport,
		GoSource:    repo.goSource,
		Description: spec.description,
		Docs:        s.docs + "/" + install,
		Browse:      browse,
		OnGitHub:    onGitHub(browse),
		Repo:        repo.display,
		RepoHref:    repo.href,
		VCS:         repo.vcs,
		Crumbs:      spec.trail[:last],
		Current:     spec.trail[last].Label,
		path:        spec.path,
	}
	for _, sub := range spec.submodules {
		page.Submodules = append(page.Submodules, childLink{
			Name:        sub.Path,
			Description: sub.Description,
			Href:        s.href(spec.path + "/" + sub.Path),
		})
	}
	for _, p := range spec.packages {
		page.Packages = append(page.Packages, childLink{Name: p, Href: s.href(spec.path + "/" + p)})
	}
	return page
}
