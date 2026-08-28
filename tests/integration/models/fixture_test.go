package models_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	story001ModelRepository = "Qwen/Qwen3-Embedding-0.6B"
	story001ModelRevision   = "97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3"
	story001ModelAsset      = "Qwen3-Embedding-0.6B.gguf"
	story001BackendAsset    = "localai-backend-localai-llamacpp-functional.tar.gz"
	story001ModelInput      = "story-001-controlled-input"
)

var (
	story001ModelBody   = []byte("story-001-model-weights")
	story001BackendBody = []byte("story-004-localai-backend")
)

type characterizationOriginOptions struct {
	failManifest bool
	blockModel   bool
}

type characterizationOrigin struct {
	server  *httptest.Server
	options characterizationOriginOptions

	mu          sync.Mutex
	exchanges   []originExchange
	modelStarts int

	modelStarted chan struct{}
	releaseModel chan struct{}
	releaseOnce  sync.Once
}

type originExchange struct {
	Sequence          int    `json:"sequence"`
	Method            string `json:"method"`
	Path              string `json:"path"`
	Query             string `json:"query,omitempty"`
	StatusCode        int    `json:"statusCode"`
	ResponseBodyBytes int64  `json:"responseBodyBytes"`
}

func newCharacterizationOrigin(t testing.TB, options characterizationOriginOptions) *characterizationOrigin {
	t.Helper()
	origin := &characterizationOrigin{
		options:      options,
		modelStarted: make(chan struct{}, 16),
		releaseModel: make(chan struct{}),
	}
	origin.server = httptest.NewServer(origin)
	t.Cleanup(func() {
		origin.releaseModelContent()
		origin.server.Close()
	})
	return origin
}

func (origin *characterizationOrigin) URL() string {
	if origin == nil || origin.server == nil {
		return ""
	}
	return origin.server.URL
}

func (origin *characterizationOrigin) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Body != nil {
		defer request.Body.Close()
	}
	index := origin.recordRequest(request)
	switch {
	case request.URL.Path == "/health":
		origin.respond(index, writer, http.StatusOK, "text/plain", nil)
	case request.URL.Path == story001ManifestPath():
		origin.serveManifest(index, writer)
	case request.URL.Path == story001ModelResolvePath():
		origin.serveModel(index, writer, request)
	case strings.HasSuffix(request.URL.Path, "/"+story001BackendAsset):
		origin.respond(index, writer, http.StatusOK, "application/octet-stream", story001BackendBody)
	case request.URL.Path == "/embed":
		origin.respondJSON(index, writer, http.StatusOK, map[string]any{
			"embeddings": []float64{0.1, 0.2, 0.3, 0.4},
		})
	default:
		origin.respond(index, writer, http.StatusNotFound, "text/plain", []byte("story-001 origin route not found\n"))
	}
}

func (origin *characterizationOrigin) serveManifest(index int, writer http.ResponseWriter) {
	if origin.options.failManifest {
		origin.respond(index, writer, http.StatusServiceUnavailable, "text/plain", []byte("story-001 controlled origin failure\n"))
		return
	}
	manifest := map[string]any{
		"sha": story001ModelRevision,
		"siblings": []map[string]any{{
			"rfilename": story001ModelAsset,
			"size":      len(story001ModelBody),
			"lfs": map[string]any{
				"oid":  sha256Hex(story001ModelBody),
				"size": len(story001ModelBody),
			},
		}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		origin.respond(index, writer, http.StatusInternalServerError, "text/plain", []byte("story-001 manifest encoding failed\n"))
		return
	}
	origin.respond(index, writer, http.StatusOK, "application/json", body)
}

func (origin *characterizationOrigin) serveModel(index int, writer http.ResponseWriter, request *http.Request) {
	if origin.options.blockModel {
		origin.noteModelStart()
		select {
		case <-origin.releaseModel:
		case <-request.Context().Done():
			return
		}
	}
	origin.respond(index, writer, http.StatusOK, "application/octet-stream", story001ModelBody)
}

func (origin *characterizationOrigin) recordRequest(request *http.Request) int {
	origin.mu.Lock()
	defer origin.mu.Unlock()
	origin.exchanges = append(origin.exchanges, originExchange{
		Sequence: len(origin.exchanges) + 1,
		Method:   request.Method,
		Path:     request.URL.Path,
		Query:    request.URL.RawQuery,
	})
	return len(origin.exchanges) - 1
}

func (origin *characterizationOrigin) respond(
	index int,
	writer http.ResponseWriter,
	status int,
	contentType string,
	body []byte,
) {
	if contentType != "" {
		writer.Header().Set("Content-Type", contentType)
	}
	writer.WriteHeader(status)
	written, _ := writer.Write(body)
	origin.recordResponse(index, status, int64(written))
}

func (origin *characterizationOrigin) respondJSON(index int, writer http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		origin.respond(index, writer, http.StatusInternalServerError, "text/plain", []byte("story-001 JSON encoding failed\n"))
		return
	}
	origin.respond(index, writer, status, "application/json", body)
}

func (origin *characterizationOrigin) recordResponse(index, status int, bodyBytes int64) {
	origin.mu.Lock()
	defer origin.mu.Unlock()
	if index < 0 || index >= len(origin.exchanges) {
		return
	}
	origin.exchanges[index].StatusCode = status
	origin.exchanges[index].ResponseBodyBytes = bodyBytes
}

func (origin *characterizationOrigin) noteModelStart() {
	origin.mu.Lock()
	origin.modelStarts++
	origin.mu.Unlock()
	origin.modelStarted <- struct{}{}
}

func (origin *characterizationOrigin) releaseModelContent() {
	origin.releaseOnce.Do(func() { close(origin.releaseModel) })
}

func (origin *characterizationOrigin) exchangesSnapshot() []originExchange {
	origin.mu.Lock()
	defer origin.mu.Unlock()
	return append([]originExchange(nil), origin.exchanges...)
}

func (origin *characterizationOrigin) assetExchanges() []originExchange {
	all := origin.exchangesSnapshot()
	assets := make([]originExchange, 0, len(all))
	for _, exchange := range all {
		if isStory001AssetPath(exchange.Path) {
			assets = append(assets, exchange)
		}
	}
	return assets
}

func (origin *characterizationOrigin) modelStartCount() int {
	origin.mu.Lock()
	defer origin.mu.Unlock()
	return origin.modelStarts
}

func story001ManifestPath() string {
	return "/models/" + story001ModelRepository
}

func story001ModelResolvePath() string {
	return "/" + story001ModelRepository + "/resolve/" + story001ModelRevision + "/" + story001ModelAsset
}

func isStory001AssetPath(path string) bool {
	return path == story001ManifestPath() || path == story001ModelResolvePath() ||
		strings.HasSuffix(path, "/"+story001BackendAsset)
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func compactJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<marshal-error:%v>", err)
	}
	return string(body)
}

func writeStory001Factory(t testing.TB, workDir string) {
	t.Helper()
	factoryDir := filepath.Join(workDir, "factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create story-001 factory directory: %v", err)
	}
	const factoryJSON = `{
  "name": "models-story-001",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }]
}`
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write story-001 factory: %v", err)
	}
}
