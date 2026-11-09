package vanitypages

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Details holds facts about published modules and packages, by import path.
type Details map[string]Detail

// Detail describes the latest published version of a module or package.
type Detail struct {
	Version   string
	Published time.Time
	License   string // license names, comma-separated
	Synopsis  string
}

// FetchDetails asks the API of the documentation site about the path each page
// advertises, skipping paths the site doesn't know. On any other failure it
// stops and returns what it collected along with the error. Pages are complete
// without details, so callers can assign the partial result to
// [Config.Details] and carry on.
func (c Config) FetchDetails(ctx context.Context) (Details, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	s := newSite(c)
	details := make(Details)
	for _, m := range c.Modules {
		for _, page := range s.modulePages(m) {
			if _, done := details[page.Install]; done {
				continue
			}
			detail, found, err := s.fetchDetail(ctx, page.Install)
			if err != nil {
				return details, err
			}
			if found {
				details[page.Install] = detail
			}
		}
	}
	return details, nil
}

func (s site) fetchDetail(ctx context.Context, path string) (Detail, bool, error) {
	var pkg struct {
		ModulePath string       `json:"modulePath"`
		Version    string       `json:"version"`
		Synopsis   string       `json:"synopsis"`
		Licenses   []apiLicense `json:"licenses"`
	}
	if found, err := s.getJSON(ctx, "package/"+path, url.Values{"licenses": {"true"}}, &pkg); !found {
		return Detail{}, false, err
	}
	var mod struct {
		CommitTime time.Time `json:"commitTime"`
	}
	if _, err := s.getJSON(ctx, "module/"+pkg.ModulePath, url.Values{"version": {pkg.Version}}, &mod); err != nil {
		return Detail{}, false, err
	}
	return Detail{
		Version:   pkg.Version,
		Published: mod.CommitTime,
		License:   licenseNames(pkg.Licenses),
		Synopsis:  pkg.Synopsis,
	}, true, nil
}

type apiLicense struct {
	Types []string `json:"types"`
}

func licenseNames(licenses []apiLicense) string {
	var names []string
	for _, license := range licenses {
		for _, name := range license.Types {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	return strings.Join(names, ", ")
}

// getJSON decodes an API response into v and reports whether the site knows
// the endpoint.
func (s site) getJSON(ctx context.Context, endpoint string, query url.Values, v any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.docs+"/v1/"+endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		if err := json.UnmarshalRead(resp.Body, v); err != nil {
			return false, fmt.Errorf("decode %s: %w", req.URL, err)
		}
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("GET %s: %s", req.URL, resp.Status)
	}
}
