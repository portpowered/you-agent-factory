package service_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	content "github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/content_materialization/internal/service"
)

func TestMaterializeContentURL_LocalFileOK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	rawURL, err := content.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("file URL: %v", err)
	}
	got, cleanup, err := service.MaterializeContentURL(context.Background(), rawURL, &service.Options{
		HostPlatform: content.ContentHostPlatform(runtime.GOOS),
		InspectPath:  os.Stat,
	})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	cleanup()
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("stat materialized path: %v", err)
	}
}

func TestMaterializeContentURL_LocalMissing(t *testing.T) {
	rawURL, err := content.FilesystemPathToContentURL(filepath.Join(t.TempDir(), "missing.png"))
	if err != nil {
		t.Fatalf("file URL: %v", err)
	}
	_, cleanup, err := service.MaterializeContentURL(context.Background(), rawURL, &service.Options{
		HostPlatform: content.ContentHostPlatform(runtime.GOOS),
		InspectPath:  os.Stat,
	})
	defer cleanup()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "media url not readable") {
		t.Fatalf("error = %v, want not readable", err)
	}
}

func TestNewRequiresContentHostPlatform(t *testing.T) {
	t.Parallel()

	service, err := service.New("", 0, 0, 0, false, nil, "", nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("New error = nil, want missing host-platform dependency")
	}
	if service != nil {
		t.Fatalf("New service = %#v, want nil on missing dependency", service)
	}
	if !strings.Contains(err.Error(), "host platform is required") {
		t.Fatalf("New error = %v, want required host-platform detail", err)
	}
}

func TestNewRequiresEveryExternalEffect(t *testing.T) {
	t.Parallel()

	valid := newMaterializeTestOptions()
	tests := []struct {
		name string
		new  func() (*service.Service, error)
	}{
		{name: "HTTP doer", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, nil, "", valid.InspectPath, valid.CreateTempFile, valid.RemovePath, valid.WriteFile, valid.OpenFile)
		}},
		{name: "inspect path", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, valid.HTTPDoer, "", nil, valid.CreateTempFile, valid.RemovePath, valid.WriteFile, valid.OpenFile)
		}},
		{name: "create temporary file", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, valid.HTTPDoer, "", valid.InspectPath, nil, valid.RemovePath, valid.WriteFile, valid.OpenFile)
		}},
		{name: "remove path", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, valid.HTTPDoer, "", valid.InspectPath, valid.CreateTempFile, nil, valid.WriteFile, valid.OpenFile)
		}},
		{name: "write file", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, valid.HTTPDoer, "", valid.InspectPath, valid.CreateTempFile, valid.RemovePath, nil, valid.OpenFile)
		}},
		{name: "open file", new: func() (*service.Service, error) {
			return service.New(valid.HostPlatform, 0, 0, 0, false, valid.HTTPDoer, "", valid.InspectPath, valid.CreateTempFile, valid.RemovePath, valid.WriteFile, nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service, err := test.new()
			if err == nil || service != nil {
				t.Fatalf("New = (%#v, %v), want fail-closed missing %s", service, err, test.name)
			}
		})
	}
}

func TestInjectedWriteFailureCleansUpTemporaryFile(t *testing.T) {
	t.Parallel()

	opts := newMaterializeTestOptions()
	removed := false
	opts.RemovePath = func(path string) error {
		removed = true
		return os.Remove(path)
	}
	opts.WriteFile = func(string, []byte, os.FileMode) error {
		return errors.New("injected write failure")
	}
	_, cleanup, err := service.MaterializeContentURL(
		context.Background(), "data:text/plain;base64,aGVsbG8=", opts,
	)
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("MaterializeContentURL error = %v, want injected failure", err)
	}
	if !removed {
		t.Fatal("temporary path was not removed after injected write failure")
	}
}

func TestMaterializeFileURLRequiresContentHostPlatform(t *testing.T) {
	t.Parallel()

	_, cleanup, err := service.MaterializeContentURL(
		context.Background(),
		"file:///tmp/content.png",
		nil,
	)
	defer cleanup()
	if err == nil || !strings.Contains(err.Error(), "host platform is required") {
		t.Fatalf("MaterializeContentURL error = %v, want missing host-platform dependency", err)
	}
}

func TestMaterializeContentURL_RemoteOK(t *testing.T) {
	body := []byte("remote-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()
	got, cleanup, err := service.MaterializeContentURL(context.Background(), server.URL, opts)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	defer cleanup()

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read temp: %v", err)
	}
	if string(data) != string(body) {
		t.Fatalf("body = %q, want %q", data, body)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal("temp should exist before cleanup")
	}
	cleanup()
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("temp should be removed after cleanup, stat err=%v", err)
	}
}

func TestMaterializeContentURL_Remote404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()
	_, cleanup, err := service.MaterializeContentURL(context.Background(), server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "media url inaccessible") {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "http 404") {
		t.Fatalf("error = %v, want http 404 reason", err)
	}
}

func TestMaterializeContentURL_RemoteTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()
	opts.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, cleanup, err := service.MaterializeContentURL(ctx, server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "media url inaccessible") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaterializeContentURL_RemoteCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	_, cleanup, err := service.MaterializeContentURL(ctx, "https://example.com/image.png", opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "media url inaccessible") || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want explicit cancellation", err)
	}
}

func TestMaterializeContentURL_DataURL(t *testing.T) {
	// 1x1 PNG
	const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	got, cleanup, err := service.MaterializeContentURL(context.Background(), dataURL, newMaterializeTestOptions())
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	defer cleanup()

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty temp file")
	}
	cleanup()
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("expected temp removed, err=%v", err)
	}
}

func TestMaterializeContentURL_SSRFRejected(t *testing.T) {
	cases := []string{
		"http://127.0.0.1/test.png",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/secret",
		"http://[::1]/secret",
	}
	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			_, cleanup, err := service.MaterializeContentURL(context.Background(), rawURL, nil)
			defer cleanup()
			if err == nil {
				t.Fatal("expected ssrf error")
			}
			if !errors.Is(err, content.ErrUnsafeContentURL) {
				t.Fatalf("error = %v, want errors.Is(..., ErrUnsafeContentURL)", err)
			}
			if !strings.Contains(err.Error(), "media url not allowed") {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(err.Error(), "ssrf") {
				t.Fatalf("error = %v, want ssrf marker", err)
			}
		})
	}
}

func TestDispatchCache_ReusesLocalURLWithoutRefetch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL, err := content.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("file URL: %v", err)
	}

	cache := service.NewDispatchCache()
	options := newMaterializeTestOptions()
	p1, cleanup1, err := cache.MaterializeContentURL(context.Background(), rawURL, options)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	p2, cleanup2, err := cache.MaterializeContentURL(context.Background(), rawURL, options)
	if err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("paths differ: %q vs %q", p1, p2)
	}
	cleanup1()
	cleanup2()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("underlying file should remain: %v", err)
	}

	cache.Release()
}

func TestDispatchCache_ReusesRemoteURLWithoutRefetch(t *testing.T) {
	requests := 0
	body := []byte("cached-remote")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(body)
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()

	cache := service.NewDispatchCache()
	p1, _, err := cache.MaterializeContentURL(context.Background(), server.URL, opts)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	p2, _, err := cache.MaterializeContentURL(context.Background(), server.URL, opts)
	if err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("paths differ: %q vs %q", p1, p2)
	}
	if requests != 1 {
		t.Fatalf("remote fetch count = %d, want 1 cache hit without re-fetch", requests)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("cached temp file should exist before release: %v", err)
	}

	cache.Release()
	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Fatalf("cached temp file should be removed after Release, stat err=%v", err)
	}
}

func TestDispatchCache_CancelledMaterializationNotCached(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("cached-after-cancel"))
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()

	cache := service.NewDispatchCache()
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := cache.MaterializeContentURL(cancelledCtx, server.URL, opts)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("error = %v, want explicit cancellation", err)
	}
	if requests != 0 {
		t.Fatalf("cancelled materialization should not fetch, requests = %d", requests)
	}

	got, _, err := cache.MaterializeContentURL(context.Background(), server.URL, opts)
	if err != nil {
		t.Fatalf("materialize after cancellation failure: %v", err)
	}
	if requests != 1 {
		t.Fatalf("retry after cancellation should fetch once, requests = %d", requests)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("materialized temp file should exist: %v", err)
	}
	cache.Release()
}

func TestMaterializeContentURL_RemoteExceedsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", 32))
	}))
	defer server.Close()

	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.HTTPDoer = server.Client()
	opts.MaxBytes = 8
	_, cleanup, err := service.MaterializeContentURL(context.Background(), server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if !errors.Is(err, content.ErrContentURLInaccessible) {
		t.Fatalf("error = %v, want errors.Is(..., ErrContentURLInaccessible)", err)
	}
	if !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaterializeContentURL_RemoteExceedsSizeLimitCleansUpTempFile(t *testing.T) {
	t.Parallel()

	var createdPath string
	opts := newMaterializeTestOptions()
	opts.AllowPrivateURLs = true
	opts.CreateTempFile = func(dir, pattern string) (content.ContentTemporaryFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		createdPath = f.Name()
		return f, nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", 32))
	}))
	defer server.Close()
	opts.HTTPDoer = server.Client()
	opts.MaxBytes = 8

	_, cleanup, err := service.MaterializeContentURL(context.Background(), server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if createdPath == "" {
		t.Fatal("expected temporary file to be created before size-limit failure")
	}
	if _, statErr := os.Stat(createdPath); !os.IsNotExist(statErr) {
		t.Fatalf("temporary file %q should be removed after size-limit failure, stat err=%v", createdPath, statErr)
	}
}

func newMaterializeTestOptions() *service.Options {
	return &service.Options{
		HostPlatform: content.ContentHostPlatform(runtime.GOOS),
		HTTPDoer: &http.Client{
			Timeout:       service.DefaultHTTPTimeout,
			CheckRedirect: service.RedirectPolicy(0, false),
		},
		InspectPath: os.Stat,
		CreateTempFile: func(dir, pattern string) (content.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		RemovePath: os.Remove,
		WriteFile:  os.WriteFile,
		OpenFile: func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		},
	}
}
