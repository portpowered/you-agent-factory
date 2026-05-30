package cliserver

import (
	"net/url"
	"testing"
)

func TestResolveBase_DefaultLocalURI(t *testing.T) {
	t.Parallel()

	base, err := ResolveBase("")
	if err != nil {
		t.Fatalf("ResolveBase(\"\"): %v", err)
	}
	if got, want := base.String(), DefaultBaseURI; got != want {
		t.Fatalf("base = %q, want %q", got, want)
	}
	if base.URL.Scheme != "http" || base.URL.Host != "localhost:7437" {
		t.Fatalf("URL = %#v, want scheme=http host=localhost:7437", base.URL)
	}
}

func TestResolveBase_CustomHostPort(t *testing.T) {
	t.Parallel()

	base, err := ResolveBase("http://127.0.0.1:9090")
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got, want := base.URL.Host, "127.0.0.1:9090"; got != want {
		t.Fatalf("host = %q, want %q", got, want)
	}
}

func TestResolveBase_HTTPSBase(t *testing.T) {
	t.Parallel()

	base, err := ResolveBase("HTTPS://Factory.Example.com:7443/")
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if got, want := base.URL.Scheme, "https"; got != want {
		t.Fatalf("scheme = %q, want %q", got, want)
	}
	if got, want := base.URL.Host, "factory.example.com:7443"; got != want {
		t.Fatalf("host = %q, want %q", got, want)
	}
	if base.URL.Path != "" {
		t.Fatalf("path = %q, want empty base path", base.URL.Path)
	}
}

func TestJoinPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseRaw string
		path    string
		want    string
	}{
		{
			name:    "default base with absolute path",
			baseRaw: DefaultBaseURI,
			path:    "/work",
			want:    "http://localhost:7437/work",
		},
		{
			name:    "trailing slash on base",
			baseRaw: "http://localhost:7437/",
			path:    "/factory-sessions/~default/work",
			want:    "http://localhost:7437/factory-sessions/~default/work",
		},
		{
			name:    "relative path segment",
			baseRaw: "http://127.0.0.1:9090",
			path:    "models",
			want:    "http://127.0.0.1:9090/models",
		},
		{
			name:    "base path prefix without double slash",
			baseRaw: "https://remote.example.com/api/v1",
			path:    "/work",
			want:    "https://remote.example.com/api/v1/work",
		},
		{
			name:    "empty path returns base",
			baseRaw: "http://localhost:7437",
			path:    "",
			want:    "http://localhost:7437",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base, err := ResolveBase(tt.baseRaw)
			if err != nil {
				t.Fatalf("ResolveBase: %v", err)
			}
			joined, err := base.JoinPath(tt.path)
			if err != nil {
				t.Fatalf("JoinPath: %v", err)
			}
			if got := joined.String(); got != tt.want {
				t.Fatalf("joined = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveBase_InvalidInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{name: "unsupported scheme", raw: "ftp://localhost:7437"},
		{name: "missing host", raw: "http:///work"},
		{name: "missing scheme", raw: "localhost:7437"},
		{name: "malformed URI", raw: "http://%zz"},
		{name: "userinfo", raw: "http://user:pass@localhost:7437"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveBase(tt.raw)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestResolveBase_PreservesDefaultHTTPSPort(t *testing.T) {
	t.Parallel()

	base, err := ResolveBase("https://factory.example.com")
	if err != nil {
		t.Fatalf("ResolveBase: %v", err)
	}
	if base.URL.Port() != "" {
		t.Fatalf("explicit port = %q, want empty for default https port", base.URL.Port())
	}
	if base.URL.Host != "factory.example.com" {
		t.Fatalf("host = %q", base.URL.Host)
	}

	joined, err := base.JoinPath("/work")
	if err != nil {
		t.Fatalf("JoinPath: %v", err)
	}
	parsed, err := url.Parse(joined.String())
	if err != nil {
		t.Fatalf("parse joined: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "factory.example.com" {
		t.Fatalf("joined URL = %q", joined.String())
	}
}
