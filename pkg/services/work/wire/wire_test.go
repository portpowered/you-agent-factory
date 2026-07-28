package wire_test

import (
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	contentmaterializationwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/wire"
)

func TestDefaultContentMaterializationHTTPTimeoutMatchesNestedWire(t *testing.T) {
	t.Parallel()

	if workwire.DefaultContentMaterializationHTTPTimeout != contentmaterializationwire.DefaultHTTPTimeout {
		t.Fatalf(
			"DefaultContentMaterializationHTTPTimeout = %v, want %v",
			workwire.DefaultContentMaterializationHTTPTimeout,
			contentmaterializationwire.DefaultHTTPTimeout,
		)
	}
}

func TestContentMaterializationRedirectPolicyDelegatesToNestedWire(t *testing.T) {
	t.Parallel()

	policy := workwire.ContentMaterializationRedirectPolicy(2, false)
	if policy == nil {
		t.Fatal("ContentMaterializationRedirectPolicy() = nil")
	}
	if err := policy(&http.Request{URL: mustParseURL(t, "http://example.com/a")}, []*http.Request{
		{URL: mustParseURL(t, "http://example.com/b")},
		{URL: mustParseURL(t, "http://example.com/c")},
	}); err == nil {
		t.Fatal("ContentMaterializationRedirectPolicy() error = nil, want redirect limit")
	}
}

func TestNewContentStagingServiceConstructsPublishedRole(t *testing.T) {
	t.Parallel()

	staging, err := workwire.NewContentStagingService(
		&stubStagingFileSystem{root: t.TempDir()},
		stubStagingRandom{},
		&stubStagingClock{now: time.Unix(0, 0)},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("NewContentStagingService() error = %v", err)
	}
	var role work.ContentStagingService = staging
	if role == nil {
		t.Fatal("constructed value is not assignable to work.ContentStagingService")
	}
}

func TestNewContentMaterializationServiceConstructsPublishedRole(t *testing.T) {
	t.Parallel()

	materializer, err := workwire.NewContentMaterializationService(
		work.ContentHostPlatform(runtime.GOOS),
		validMaterializationHTTPClient(),
		os.Stat,
		validCreateTempFile,
		os.Remove,
		os.WriteFile,
		validOpenFile,
	)
	if err != nil {
		t.Fatalf("NewContentMaterializationService() error = %v", err)
	}
	var role work.ContentMaterializer = materializer
	if role == nil {
		t.Fatal("constructed value is not assignable to work.ContentMaterializer")
	}
}

func TestNewServicePropagatesContentStagingConstructionErrors(t *testing.T) {
	t.Parallel()

	service, err := workwire.NewService(
		stubRuntimeResolver{},
		nil,
		stubStagingRandom{},
		&stubStagingClock{now: time.Unix(0, 0)},
		time.Minute,
		work.ContentHostPlatform(runtime.GOOS),
		validMaterializationHTTPClient(),
		os.Stat,
		validCreateTempFile,
		os.Remove,
		os.WriteFile,
		validOpenFile,
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want content staging construction failure")
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func TestNewServicePropagatesContentMaterializationConstructionErrors(t *testing.T) {
	t.Parallel()

	service, err := workwire.NewService(
		stubRuntimeResolver{},
		&stubStagingFileSystem{root: t.TempDir()},
		stubStagingRandom{},
		&stubStagingClock{now: time.Unix(0, 0)},
		time.Minute,
		"",
		validMaterializationHTTPClient(),
		os.Stat,
		validCreateTempFile,
		os.Remove,
		os.WriteFile,
		validOpenFile,
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want content materialization construction failure")
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := workwire.NewService(
		stubRuntimeResolver{},
		&stubStagingFileSystem{root: t.TempDir()},
		stubStagingRandom{},
		&stubStagingClock{now: time.Unix(0, 0)},
		time.Minute,
		work.ContentHostPlatform(runtime.GOOS),
		validMaterializationHTTPClient(),
		os.Stat,
		validCreateTempFile,
		os.Remove,
		os.WriteFile,
		validOpenFile,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root work.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to work.Service")
	}
}

type stubRuntimeResolver struct{}

func (stubRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return nil, nil
}

type stubStagingFileSystem struct {
	root string
}

func (f *stubStagingFileSystem) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(f.root, pattern)
}

func (f *stubStagingFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func (f *stubStagingFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (f *stubStagingFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

type stubStagingRandom struct{}

func (stubStagingRandom) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = 0x11
	}
	return len(buffer), nil
}

type stubStagingClock struct {
	now time.Time
}

func (c *stubStagingClock) Now() time.Time { return c.now }

func validMaterializationHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       workwire.DefaultContentMaterializationHTTPTimeout,
		CheckRedirect: workwire.ContentMaterializationRedirectPolicy(0, false),
	}
}

func validCreateTempFile(dir, pattern string) (work.ContentTemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func validOpenFile(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed
}
