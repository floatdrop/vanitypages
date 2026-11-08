// Package assets embeds the templates and shared files of generated sites.
//
// Files named *.gen.*, the fonts and THIRD_PARTY_LICENSES.txt come from npm
// packages and are refreshed by vendor.sh.
package assets

import (
	"embed"
	"slices"
)

// Templates holds the HTML templates.
//
//go:embed templates/*.html
var Templates embed.FS

var (
	//go:embed gravity.gen.css
	gravityCSS []byte
	//go:embed site.css
	siteCSS []byte
	//go:embed site.js
	siteJS []byte
	//go:embed inter-latin.woff2
	interLatin []byte
	//go:embed inter-cyrillic.woff2
	interCyrillic []byte
	//go:embed THIRD_PARTY_LICENSES.txt
	licenses []byte
)

// Static returns the files shared by all pages, keyed by file name.
func Static() map[string][]byte {
	return map[string][]byte{
		"vanitypages.css":          slices.Concat(gravityCSS, siteCSS),
		"vanitypages.js":           siteJS,
		"inter-latin.woff2":        interLatin,
		"inter-cyrillic.woff2":     interCyrillic,
		"THIRD_PARTY_LICENSES.txt": licenses,
	}
}
