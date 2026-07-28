package wire

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/internal/service"
)

func TestDefaultHTTPTimeoutMatchesInternalService(t *testing.T) {
	t.Parallel()

	if DefaultHTTPTimeout != internalservice.DefaultHTTPTimeout {
		t.Fatalf("DefaultHTTPTimeout = %v, want %v", DefaultHTTPTimeout, internalservice.DefaultHTTPTimeout)
	}
}

func TestRedirectPolicyDelegatesToInternalService(t *testing.T) {
	t.Parallel()

	policy := RedirectPolicy(2, false)
	if policy == nil {
		t.Fatal("RedirectPolicy() = nil")
	}
	if err := policy(&http.Request{URL: mustParseURL("http://example.com/a")}, []*http.Request{
		{URL: mustParseURL("http://example.com/b")},
		{URL: mustParseURL("http://example.com/c")},
	}); err == nil {
		t.Fatal("RedirectPolicy() error = nil, want redirect limit")
	}
}

func TestNewServiceConstructsContentMaterializationSubservice(t *testing.T) {
	t.Parallel()

	service, err := NewService(
		work.ContentHostPlatform(runtime.GOOS),
		0,
		0,
		0,
		false,
		&http.Client{Timeout: DefaultHTTPTimeout},
		"",
		os.Stat,
		func(dir, pattern string) (work.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.WriteFile,
		func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() = nil")
	}
}

func TestNewServiceWrapsConstructionErrors(t *testing.T) {
	t.Parallel()

	service, err := NewService(
		"",
		0,
		0,
		0,
		false,
		&http.Client{Timeout: time.Second},
		"",
		os.Stat,
		func(dir, pattern string) (work.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.WriteFile,
		func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		},
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want construction failure")
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil on error", service)
	}
	if !strings.Contains(err.Error(), "construct Work content materialization") {
		t.Fatalf("NewService() error = %v, want wrapped construction detail", err)
	}
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
