# vanitypages

Generates a static site of Go vanity import pages, built with
[Gravity UI](https://gravity-ui.com) and styled after go.dev. Host the output
anywhere that serves static files, and `go get your.domain/module` resolves to
your repository.

## Gallery

|               | Light                                                                     | Dark                                                                     |
| ------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| **Index**     | ![Index page listing modules](docs/screenshots/index-light.png)           | ![Index page in the dark theme](docs/screenshots/index-dark.png)         |
| **Module**    | ![Module page with install details](docs/screenshots/module-light.png)    | ![Module page in the dark theme](docs/screenshots/module-dark.png)       |
| **Submodule** | ![Submodule page](docs/screenshots/submodule-light.png)                   | ![Submodule page in the dark theme](docs/screenshots/submodule-dark.png) |
| **Phone**     | <img src="docs/screenshots/phone-light.png" width="220" alt="Index page on a phone"> | <img src="docs/screenshots/phone-dark.png" width="220" alt="Module page on a phone in the dark theme"> |

## Usage

```sh
go install github.com/floatdrop/vanitypages/cmd/vanitypages@latest
```

Create `vanitypages.json`. The `$schema` line gives you completion and
validation in editors:

```json
{
  "$schema": "https://raw.githubusercontent.com/floatdrop/vanitypages/main/vanitypages.schema.json",
  "domain": "go.example.com",
  "modules": [
    {
      "path": "cache",
      "repo": "https://github.com/acme/cache",
      "description": "Typed in-memory cache",
      "submodules": [
        { "path": "contrib/redis", "description": "Redis backend" }
      ]
    }
  ]
}
```

Run `vanitypages` to write the site to `public/`.

| Flag       | Default            | Description                           |
| ---------- | ------------------ | ------------------------------------- |
| `-config`  | `vanitypages.json` | Path to the config file               |
| `-out`     | `public`           | Directory to write the site to        |
| `-schema`  |                    | Print the config JSON Schema and exit |
| `-version` |                    | Print the version and exit            |

Generating overwrites earlier output but never deletes files, so clean the
output directory if you remove a module from the config.

## Configuration

| Field         | Description                                                                 |
| ------------- | --------------------------------------------------------------------------- |
| `domain`      | Host the site is served from, optionally with a path. Required.             |
| `title`       | Index page heading. Defaults to `domain`.                                   |
| `description` | Text at the top of the index page.                                          |
| `docs`        | Base URL of the documentation site. Defaults to `https://pkg.go.dev`.       |
| `modules`     | The modules to serve. Required.                                             |

Each module has:

| Field         | Description                                                                 |
| ------------- | --------------------------------------------------------------------------- |
| `path`        | Module path relative to the domain. Required.                               |
| `repo`        | Repository URL. Required.                                                   |
| `vcs`         | `git` (default), `hg`, `svn`, `bzr`, `fossil` or `mod`.                     |
| `description` | Text shown on the index and module pages.                                   |
| `versions`    | Major versions above v1 developed at the repository root, such as `["v2"]`. |
| `submodules`  | Go modules nested in the repository, each with `path` and `description`.    |
| `packages`    | Package paths that should get a page of their own.                          |
| `source`      | `home`, `dir` and `file` URL templates for source links.                    |
| `hidden`      | Keep the module off the index page while it stays importable.               |

Source links are detected for repositories on github.com, gitlab.com,
bitbucket.org and codeberg.org. Use `source` for other hosts.

### Which paths need an entry

Static hosting can't match path prefixes, so every **module root** needs a page.
Packages inside a module don't: the go command retries shorter paths until it
reaches the module.

- A module whose `go.mod` at the repository root says `example.com/foo/v2`:
  add `"versions": ["v2"]`.
- A module with its own `go.mod` in a subdirectory, tagged `contrib/redis/v1.2.3`
  or kept in a `v2/` directory: add it to `submodules`. Its `path` must match the
  directory.
- A package you want a browsable page for: add it to `packages`.

## Go API

```go
site := vanitypages.Config{
	Domain: "go.example.com",
	Modules: []vanitypages.Module{{
		Path:       "cache",
		Repo:       "https://github.com/acme/cache",
		Submodules: []vanitypages.Submodule{{Path: "contrib/redis"}},
	}},
}
if err := site.Generate("public"); err != nil {
	log.Fatal(err)
}
```

`Config.Render` returns the files in memory instead, and `vanitypages.Load`
reads a JSON config file.

## Deployment

`.github/workflows/pages.yml` builds `vanitypages.json` from this repository and
deploys it to GitHub Pages on every push to `main`.

## Third-party assets

Generated sites include Gravity UI styles and icons (MIT), with licenses at
`/-/THIRD_PARTY_LICENSES.txt`. To update them, change the versions in
`internal/assets/vendor.sh` and run it.
