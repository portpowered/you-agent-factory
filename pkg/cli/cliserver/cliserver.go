// Package cliserver resolves factory API server base URIs and builds request URLs.
package cliserver

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	// DefaultBaseURI is the factory API base when no --server value is provided.
	DefaultBaseURI = "http://localhost:7437"
)

// Base is a validated http or https factory API server base URI.
type Base struct {
	URL url.URL
}

// ResolveBase parses raw into a validated server base URI.
// An empty or whitespace-only raw value defaults to DefaultBaseURI.
func ResolveBase(raw string) (Base, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = DefaultBaseURI
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return Base{}, fmt.Errorf("invalid server URI %q: %w", trimmed, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		if parsed.Scheme == "" {
			return Base{}, fmt.Errorf("invalid server URI %q: missing scheme (use http:// or https://)", trimmed)
		}
		return Base{}, fmt.Errorf("invalid server URI %q: scheme %q must be http or https", trimmed, parsed.Scheme)
	}
	if parsed.Host == "" {
		return Base{}, fmt.Errorf("invalid server URI %q: missing host", trimmed)
	}
	if parsed.User != nil {
		return Base{}, fmt.Errorf("invalid server URI %q: userinfo in server URI is not supported", trimmed)
	}

	normalized := *parsed
	normalized.Scheme = strings.ToLower(normalized.Scheme)
	normalized.Host = strings.ToLower(normalized.Host)
	normalized.Path = normalizeBasePath(normalized.Path)
	normalized.RawPath = ""
	normalized.RawQuery = ""
	normalized.Fragment = ""
	normalized.Opaque = ""

	return Base{URL: normalized}, nil
}

// String returns the base URI without a trailing slash on the path component.
func (b Base) String() string {
	u := b.URL
	if u.Path == "" || u.Path == "/" {
		return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	}
	return u.String()
}

// JoinPath joins an API path onto the base without producing double slashes.
// path may be absolute (leading slash) or relative; an empty path returns the base URL.
func (b Base) JoinPath(path string) (url.URL, error) {
	joined := b.URL
	if path == "" {
		return joined, nil
	}

	apiPath := path
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}

	basePath := strings.TrimSuffix(joined.Path, "/")
	if basePath == "" {
		joined.Path = apiPath
	} else {
		joined.Path = basePath + apiPath
	}
	return joined, nil
}

func normalizeBasePath(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	return strings.TrimSuffix(path, "/")
}
