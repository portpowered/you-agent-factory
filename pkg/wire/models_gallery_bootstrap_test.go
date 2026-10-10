package wire

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

type localAISelectedHTTPClient struct {
	do func(*http.Request) (*http.Response, error)
}

func (client localAISelectedHTTPClient) Do(request *http.Request) (*http.Response, error) {
	return client.do(request)
}

func TestDownloadLocalAIBinaryPreservesSelectedHTTPFailure(t *testing.T) {
	t.Parallel()
	for _, failedPath := range []string{"/latest", "/checksums", "/binary"} {
		t.Run(failedPath, func(t *testing.T) {
			t.Parallel()
			selectedError := errors.New("selected HTTP failure")
			var paths []string
			client := localAISelectedHTTPClient{do: func(request *http.Request) (*http.Response, error) {
				paths = append(paths, request.URL.Path)
				if request.Method != http.MethodGet || request.URL.Host != "selected.invalid" || request.Context().Err() != nil {
					t.Fatalf("selected HTTP request = %#v", request)
				}
				if _, ok := request.Context().Deadline(); !ok {
					t.Fatal("download request has no bounded deadline")
				}
				if request.URL.Path == failedPath {
					return nil, selectedError
				}
				return localAISelectedReleaseResponse(request.URL.Path), nil
			}}
			cache := t.TempDir()
			path, err := downloadLocalAIBinary(t.Context(), client, cache, "https://selected.invalid/latest")
			if path != "" || !errors.Is(err, selectedError) {
				t.Fatalf("failed download = %q, %v, want original selected error", path, err)
			}
			want := []string{"/latest"}
			if failedPath != "/latest" {
				want = append(want, "/checksums")
			}
			if failedPath == "/binary" {
				want = append(want, "/binary")
			}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("HTTP requests = %v, want %v", paths, want)
			}
			if _, statErr := os.Stat(filepath.Join(cache, "v1", "local-ai-v1-linux-amd64")); !os.IsNotExist(statErr) {
				t.Fatalf("failed download published binary: %v", statErr)
			}
		})
	}
}

func localAISelectedReleaseResponse(path string) *http.Response {
	metadata := `{"tag_name":"v1","assets":[{"name":"local-ai-v1-linux-amd64","browser_download_url":"https://selected.invalid/binary","size":4},{"name":"LocalAI-v1-checksums.txt","browser_download_url":"https://selected.invalid/checksums"}]}`
	body := metadata
	if path == "/checksums" {
		digest := sha256.Sum256([]byte("test"))
		body = fmt.Sprintf("%x  local-ai-v1-linux-amd64\n", digest)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestDownloadLocalAIBinaryCancellationDoesNotUseSelectedClient(t *testing.T) {
	t.Parallel()
	client := localAISelectedHTTPClient{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("cancelled download used HTTP client")
		return nil, nil
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	path, err := downloadLocalAIBinary(ctx, client, t.TempDir(), "https://selected.invalid/latest")
	if path != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled download = %q, %v", path, err)
	}
}

// The downloader owns release validation and atomic cache publication. Each
// scenario controls only HTTP responses and its cache; no model host is started.
func TestDownloadLocalAIBinaryRejectsUnsafeReleaseAndRecovers(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name string
		path string
		body string
		want string
	}{
		{"escaping_release_tag", "/latest", `{"tag_name":"../outside"}`, "invalid LocalAI release tag"},
		{"missing_checksum_asset", "/latest", `{"tag_name":"v1","assets":[{"name":"local-ai-v1-linux-amd64","browser_download_url":"https://selected.invalid/binary","size":4}]}`, "lacks a bounded Linux amd64 binary or checksums asset"},
		{"invalid_checksum", "/checksums", strings.Repeat("z", 64) + "  local-ai-v1-linux-amd64\n", "lack SHA256"},
		{"truncated_binary", "/binary", "tes", "size or SHA256"},
		{"interrupted_binary", "/binary", "", "write LocalAI binary"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			cache := t.TempDir()
			customerPath := filepath.Join(cache, "customer-notes.txt")
			if err := os.WriteFile(customerPath, []byte("keep customer bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			readFailure := errors.New("selected binary stream interrupted")
			failed := true
			client := localAISelectedHTTPClient{do: func(request *http.Request) (*http.Response, error) {
				response := localAISelectedReleaseResponse(request.URL.Path)
				if request.URL.Path == "/binary" {
					response.Body = io.NopCloser(strings.NewReader("test"))
				}
				if failed && request.URL.Path == scenario.path {
					response.Body = io.NopCloser(strings.NewReader(scenario.body))
					if scenario.name == "interrupted_binary" {
						response.Body = io.NopCloser(io.MultiReader(strings.NewReader("te"), localAIFailedBody{err: readFailure}))
					}
				}
				return response, nil
			}}
			path, err := downloadLocalAIBinary(t.Context(), client, cache, "https://selected.invalid/latest")
			if path != "" || err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("rejected release = %q, %v, want %q", path, err, scenario.want)
			}
			if scenario.name == "interrupted_binary" && !errors.Is(err, readFailure) {
				t.Fatalf("binary read failure lost cause: %v", err)
			}
			installed := filepath.Join(cache, "v1", "local-ai-v1-linux-amd64")
			if _, err := os.Stat(installed); !os.IsNotExist(err) {
				t.Fatalf("rejected binary was published: %v", err)
			}
			assertLocalAIStagingRemoved(t, cache)
			assertLocalAIFileContent(t, customerPath, "keep customer bytes")
			failed = false
			path, err = downloadLocalAIBinary(t.Context(), client, cache, "https://selected.invalid/latest")
			if err != nil || path != installed {
				t.Fatalf("healthy retry = %q, %v, want %q", path, err, installed)
			}
			assertLocalAIFileContent(t, path, "test")
			assertLocalAIFileContent(t, customerPath, "keep customer bytes")
			assertLocalAIStagingRemoved(t, cache)
		})
	}
}

type localAIFailedBody struct{ err error }

func (body localAIFailedBody) Read([]byte) (int, error) { return 0, body.err }

func assertLocalAIFileContent(t *testing.T, path, want string) {
	t.Helper()
	if content, err := os.ReadFile(path); err != nil || string(content) != want {
		t.Fatalf("file %q content = %q, %v, want %q", path, content, err, want)
	}
}

func assertLocalAIStagingRemoved(t *testing.T, cache string) {
	t.Helper()
	staged, err := filepath.Glob(filepath.Join(cache, "v1", ".local-ai-*"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("staged binary files = %v, %v, want none", staged, err)
	}
}
