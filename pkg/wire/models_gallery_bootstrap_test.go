package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadLocalAIBinaryVerifiesAndReusesCache(t *testing.T) {
	t.Parallel()
	const tag = "v9.9.9"
	const name = "local-ai-v9.9.9-linux-amd64"
	content := []byte("test LocalAI binary")
	digest := sha256.Sum256(content)
	var downloads atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(localAIRelease{Tag: tag, Assets: []localAIReleaseAsset{
				{Name: name, URL: server.URL + "/binary", Size: int64(len(content))},
				{Name: "LocalAI-v9.9.9-checksums.txt", URL: server.URL + "/checksums"},
			}})
		case "/checksums":
			_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(digest[:]), name)
		case "/binary":
			downloads.Add(1)
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache := t.TempDir()
	path, err := downloadLocalAIBinary(context.Background(), server.Client(), cache, server.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(cache, tag, name) {
		t.Fatalf("unexpected cache path %q", path)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(content) {
		t.Fatalf("unexpected cached binary %q: %v", data, err)
	}
	if _, err := downloadLocalAIBinary(context.Background(), server.Client(), cache, server.URL+"/latest"); err != nil {
		t.Fatal(err)
	}
	if got := downloads.Load(); got != 1 {
		t.Fatalf("downloaded cached binary %d times, want 1", got)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := downloadLocalAIBinary(context.Background(), server.Client(), cache, server.URL+"/latest"); err != nil {
		t.Fatal(err)
	}
	if got := downloads.Load(); got != 2 {
		t.Fatalf("downloaded corrupt cached binary %d times, want 2", got)
	}
}

func TestDownloadLocalAIBinaryRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()
	const name = "local-ai-v1-linux-amd64"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			_ = json.NewEncoder(w).Encode(localAIRelease{Tag: "v1", Assets: []localAIReleaseAsset{
				{Name: name, URL: server.URL + "/binary", Size: 4},
				{Name: "LocalAI-v1-checksums.txt", URL: server.URL + "/checksums"},
			}})
		case "/checksums":
			_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), name)
		case "/binary":
			_, _ = w.Write([]byte("test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache := t.TempDir()
	_, err := downloadLocalAIBinary(context.Background(), server.Client(), cache, server.URL+"/latest")
	if err == nil || !strings.Contains(err.Error(), "SHA256") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "v1", name)); !os.IsNotExist(err) {
		t.Fatalf("unexpected installed binary: %v", err)
	}
}

func TestResolveLocalAIBinaryUsesOverride(t *testing.T) {
	t.Setenv(localAIBinaryEnvironment, "/custom/local-ai")
	command, err := resolveLocalAIBinary(context.Background(), nil, "")
	if err != nil || command != "/custom/local-ai" {
		t.Fatalf("override = %q, %v", command, err)
	}
}
