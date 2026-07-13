package materialize_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/work/content"
	"github.com/portpowered/infinite-you/pkg/workcontent/materialize"
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
	got, cleanup, err := materialize.MaterializeContentURL(context.Background(), rawURL, nil)
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
	_, cleanup, err := materialize.MaterializeContentURL(context.Background(), rawURL, nil)
	defer cleanup()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "media url not readable") {
		t.Fatalf("error = %v, want not readable", err)
	}
}

func TestMaterializeContentURL_RemoteOK(t *testing.T) {
	body := []byte("remote-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	opts := &materialize.Options{
		AllowPrivateURLs: true,
		HTTPClient:       server.Client(),
	}
	got, cleanup, err := materialize.MaterializeContentURL(context.Background(), server.URL, opts)
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

	opts := &materialize.Options{AllowPrivateURLs: true, HTTPClient: server.Client()}
	_, cleanup, err := materialize.MaterializeContentURL(context.Background(), server.URL, opts)
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

	opts := &materialize.Options{
		AllowPrivateURLs: true,
		HTTPClient:       server.Client(),
		Timeout:          50 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, cleanup, err := materialize.MaterializeContentURL(ctx, server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "media url inaccessible") {
		t.Fatalf("error = %v", err)
	}
}

func TestMaterializeContentURL_DataURL(t *testing.T) {
	// 1x1 PNG
	const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	got, cleanup, err := materialize.MaterializeContentURL(context.Background(), dataURL, nil)
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
	}
	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			_, cleanup, err := materialize.MaterializeContentURL(context.Background(), rawURL, nil)
			defer cleanup()
			if err == nil {
				t.Fatal("expected ssrf error")
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

func TestDispatchCache_ReusesURLAndCleansUpOnRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rawURL, err := content.FilesystemPathToContentURL(path)
	if err != nil {
		t.Fatalf("file URL: %v", err)
	}

	cache := materialize.NewDispatchCache()
	p1, cleanup1, err := cache.MaterializeContentURL(context.Background(), rawURL, nil)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	p2, cleanup2, err := cache.MaterializeContentURL(context.Background(), rawURL, nil)
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

func TestMaterializeContentURL_RemoteExceedsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, strings.Repeat("x", 32))
	}))
	defer server.Close()

	opts := &materialize.Options{
		AllowPrivateURLs: true,
		HTTPClient:       server.Client(),
		MaxBytes:         8,
	}
	_, cleanup, err := materialize.MaterializeContentURL(context.Background(), server.URL, opts)
	defer cleanup()
	if err == nil {
		t.Fatal("expected size limit error")
	}
	if !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("error = %v", err)
	}
}
