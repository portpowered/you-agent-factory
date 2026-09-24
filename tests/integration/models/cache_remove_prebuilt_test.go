package models_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

type story004RemoveResponse struct {
	ModelName                string `json:"modelName"`
	Revision                 string `json:"revision"`
	CachePath                string `json:"cachePath"`
	Outcome                  string `json:"outcome"`
	BytesRemoved             int64  `json:"bytesRemoved"`
	ReclaimedCacheBytes      *int64 `json:"reclaimedCacheBytes"`
	RetainedSharedCacheBytes *int64 `json:"retainedSharedCacheBytes"`
}

type story004HTTPResponse struct {
	mu     sync.Mutex
	status int
	body   []byte
}

func (capture *story004HTTPResponse) record(status int, body []byte) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	capture.status = status
	capture.body = append(capture.body[:0], body...)
}

func (capture *story004HTTPResponse) snapshot() (int, []byte) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.status, append([]byte(nil), capture.body...)
}

// TestStory004PrebuiltRemoveMatchesConfiguredServerAndReclaimsOnlyCandidates
// exercises the shipped command and its HTTP request against separate tiny,
// isolated cache fixtures. TestMain admits only a supplied prebuilt artifact.
func TestStory004PrebuiltRemoveMatchesConfiguredServerAndReclaimsOnlyCandidates(t *testing.T) {
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	assertStory004PrebuiltHelp(t, binaryPath, workDir)

	localFixture := writeStory004ASRRemovalFixture(t, t.TempDir())
	localResponse := runStory004LocalRemove(t, binaryPath, workDir, localFixture)
	assertStory004ReclaimedEffects(t, localFixture)

	serverFixture := writeStory004ASRRemovalFixture(t, t.TempDir())
	remoteResponse := runStory004ConfiguredServerRemove(t, binaryPath, workDir, serverFixture)
	assertStory004ReclaimedEffects(t, serverFixture)
	assertStory004EquivalentRemove(t, localResponse, remoteResponse)
	t.Logf("STORY-004-EVIDENCE artifactSHA256=%s localAndHTTP=equivalent reclaimedCacheBytes=%d retainedSharedCacheBytes=%d",
		story001Binary.identity.sha256, *remoteResponse.ReclaimedCacheBytes, *remoteResponse.RetainedSharedCacheBytes)
}

func assertStory004PrebuiltHelp(t testing.TB, binaryPath, workDir string) {
	t.Helper()
	home, cache := t.TempDir(), t.TempDir()
	help := runStory001Command(t, t.Context(), binaryPath, workDir,
		story001Environment(home, cache, "http://127.0.0.1:1"), "models", "remove", "--help")
	if help.exitCode != 0 || !help.processExited || !strings.Contains(string(help.stdout), "--reclaim-unused-cache") {
		t.Fatalf("prebuilt remove help = %s stdout=%q stderr=%q, want success and documented opt-in flag",
			summarizeProcess(help), help.stdout, help.stderr)
	}
}

func runStory004LocalRemove(
	t testing.TB,
	binaryPath, workDir string,
	fixture story004ASRRemovalFixture,
) story004RemoveResponse {
	t.Helper()
	home := t.TempDir()
	result := runStory001Command(t, t.Context(), binaryPath, workDir,
		story001Environment(home, fixture.cacheRoot, "http://127.0.0.1:1"),
		"--json", "models", "remove", "asr", "--reclaim-unused-cache")
	if result.exitCode != 0 || !result.processExited {
		t.Fatalf("prebuilt local remove = %s stdout=%q stderr=%q", summarizeProcess(result), result.stdout, result.stderr)
	}
	response := decodeStory004RemoveResponse(t, result.stdout)
	assertStory004ReclaimedResponse(t, response, fixture)
	return response
}

func runStory004ConfiguredServerRemove(
	t testing.TB,
	binaryPath, workDir string,
	fixture story004ASRRemovalFixture,
) story004RemoveResponse {
	t.Helper()
	serverHome := t.TempDir()
	environment := story001Environment(serverHome, fixture.cacheRoot, "http://127.0.0.1:1")
	address := reserveStory001Loopback(t)
	process := startStory001Command(t, t.Context(), binaryPath, workDir, environment, "server", "--listen", address)
	t.Cleanup(process.stop)
	serverURL := "http://" + address
	if ready := waitForStory001HTTP200(t, t.Context(), serverURL+"/status"); ready.status != 200 {
		process.stop()
		t.Fatalf("prebuilt server readiness = %s", summarizeHTTP(ready))
	}
	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse prebuilt server URL: %v", err)
	}
	capture := &story004HTTPResponse{}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			return readErr
		}
		capture.record(response.StatusCode, body)
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return nil
	}
	proxyServer := httptest.NewServer(proxy)
	t.Cleanup(proxyServer.Close)
	remote := runStory001Command(t, t.Context(), binaryPath, workDir,
		story001Environment(t.TempDir(), fixture.cacheRoot, "http://127.0.0.1:1"),
		"--server", proxyServer.URL, "--json", "models", "remove", "asr", "--reclaim-unused-cache")
	if remote.exitCode != 0 || !remote.processExited {
		t.Fatalf("prebuilt configured-server remove = %s stdout=%q stderr=%q", summarizeProcess(remote), remote.stdout, remote.stderr)
	}
	response := decodeStory004RemoveResponse(t, remote.stdout)
	assertStory004ReclaimedResponse(t, response, fixture)
	status, body := capture.snapshot()
	if status != http.StatusOK {
		t.Fatalf("prebuilt DELETE HTTP status = %d, want 200; body=%s", status, body)
	}
	var httpResponse story004RemoveResponse
	if err := json.Unmarshal(body, &httpResponse); err != nil {
		t.Fatalf("decode prebuilt DELETE HTTP response: %v; body=%s", err, body)
	}
	assertStory004EquivalentRemove(t, response, httpResponse)
	process.stop()
	if process.command.ProcessState == nil || !process.command.ProcessState.Exited() {
		t.Fatalf("prebuilt server process state = %#v, want exited after release", process.command.ProcessState)
	}
	return response
}

type story004ASRRemovalFixture struct {
	cacheRoot       string
	revision        string
	revisionPath    string
	revisionBytes   int64
	siblingPath     string
	modelSnapshot   string
	modelBytes      int64
	backendSnapshot string
	backendBytes    int64
}

func writeStory004ASRRemovalFixture(t testing.TB, cacheRoot string) story004ASRRemovalFixture {
	t.Helper()
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameASR)
	if !ok {
		t.Fatal("built-in catalog did not publish ASR")
	}
	sourceParts := strings.SplitN(definition.Source, "@", 2)
	if len(sourceParts) != 2 || strings.TrimSpace(sourceParts[1]) == "" {
		t.Fatalf("built-in ASR source %q has no pinned revision", definition.Source)
	}
	revision, assetName := sourceParts[1], path.Base(sourceParts[0])
	modelBody := []byte("prebuilt ASR model fixture")
	modelDigest := sha256Hex(modelBody)
	modelRoot := filepath.Join(cacheRoot, "ASR")
	revisionPath := filepath.Join(modelRoot, revision)
	writeStory004File(t, filepath.Join(revisionPath, assetName), modelBody)
	siblingPath := filepath.Join(modelRoot, "sibling-revision", "sibling.bin")
	writeStory004File(t, siblingPath, []byte("preserved sibling revision"))

	modelIdentity := fmt.Sprintf("model|%s|%s:%d:%s", definition.Source, assetName, len(modelBody), modelDigest)
	modelIdentityHash := sha256.Sum256([]byte(modelIdentity))
	modelSnapshot := filepath.Join(cacheRoot, ".you-content-addressed", "model", fmt.Sprintf("%x", modelIdentityHash[:]))
	writeStory004File(t, filepath.Join(modelSnapshot, assetName), modelBody)
	writeStory004Metadata(t, filepath.Join(modelSnapshot, ".you-assets.json"), map[string]any{
		"kind": "model", "identity": modelIdentity, "source": definition.Source, "sourceKey": definition.Source,
		"artifacts": []map[string]any{{"Name": assetName, "Bytes": len(modelBody), "SHA256": modelDigest}},
	})

	backendBody := []byte("prebuilt ASR backend fixture")
	backendDigest := sha256Hex(backendBody)
	backendLocation := "https://example.invalid/localai-asr-backend.tar.gz"
	backendLocationHash := sha256.Sum256([]byte(backendLocation))
	backendSource := fmt.Sprintf("backend://%s/release://%x", definition.Backend, backendLocationHash[:])
	backendName := "localai-asr-backend-fixture.tar.gz"
	backendIdentity := fmt.Sprintf("backend|%s|%s:%d:%s", backendSource, backendName, len(backendBody), backendDigest)
	backendIdentityHash := sha256.Sum256([]byte(backendIdentity))
	backendSnapshot := filepath.Join(cacheRoot, "backend-artifacts", ".you-content-addressed", "backend", fmt.Sprintf("%x", backendIdentityHash[:]))
	writeStory004File(t, filepath.Join(backendSnapshot, backendName), backendBody)
	writeStory004Metadata(t, filepath.Join(backendSnapshot, ".you-assets.json"), map[string]any{
		"kind": "backend", "identity": backendIdentity, "source": backendSource, "sourceKey": backendSource,
		"artifacts": []map[string]any{{"Name": backendName, "Bytes": len(backendBody), "SHA256": backendDigest}},
	})
	backendRelative, err := filepath.Rel(cacheRoot, backendSnapshot)
	if err != nil {
		t.Fatalf("relative prebuilt backend snapshot: %v", err)
	}
	managedMetadata := map[string]any{
		"modelName": "ASR", "revision": revision,
		"files": []map[string]any{{"path": assetName, "bytes": len(modelBody), "sha256": modelDigest}},
		"backend": map[string]any{
			"cachePath": filepath.ToSlash(backendRelative), "revision": "fixture-backend",
			"files": []map[string]any{{"path": backendName, "bytes": len(backendBody), "sha256": backendDigest}},
		},
	}
	writeStory004Metadata(t, filepath.Join(modelRoot, ".managed-cache.json"), managedMetadata)
	return story004ASRRemovalFixture{
		cacheRoot: cacheRoot, revision: revision, revisionPath: revisionPath,
		revisionBytes: story004RegularFileBytes(t, revisionPath), siblingPath: siblingPath,
		modelSnapshot: modelSnapshot, modelBytes: story004RegularFileBytes(t, modelSnapshot),
		backendSnapshot: backendSnapshot, backendBytes: story004RegularFileBytes(t, backendSnapshot),
	}
}

func writeStory004File(t testing.TB, filePath string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("create prebuilt removal fixture directory: %v", err)
	}
	if err := os.WriteFile(filePath, body, 0o644); err != nil {
		t.Fatalf("write prebuilt removal fixture %q: %v", filePath, err)
	}
}

func writeStory004Metadata(t testing.TB, filePath string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode prebuilt removal metadata: %v", err)
	}
	writeStory004File(t, filePath, body)
}

func story004RegularFileBytes(t testing.TB, root string) int64 {
	t.Helper()
	var total int64
	if err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	}); err != nil {
		t.Fatalf("measure prebuilt removal fixture %q: %v", root, err)
	}
	return total
}

func decodeStory004RemoveResponse(t testing.TB, body []byte) story004RemoveResponse {
	t.Helper()
	var response story004RemoveResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode prebuilt remove response: %v; body=%s", err, body)
	}
	return response
}

func assertStory004ReclaimedResponse(t testing.TB, response story004RemoveResponse, fixture story004ASRRemovalFixture) {
	t.Helper()
	if response.ModelName != "ASR" || response.Revision != fixture.revision || response.Outcome != "REMOVED" ||
		response.BytesRemoved != fixture.revisionBytes || response.ReclaimedCacheBytes == nil ||
		*response.ReclaimedCacheBytes != fixture.modelBytes+fixture.backendBytes ||
		response.RetainedSharedCacheBytes == nil || *response.RetainedSharedCacheBytes != 0 {
		t.Fatalf("prebuilt remove response = %#v, want revision=%q bytes=%d reclaimed=%d retained=0",
			response, fixture.revision, fixture.revisionBytes, fixture.modelBytes+fixture.backendBytes)
	}
}

func assertStory004ReclaimedEffects(t testing.TB, fixture story004ASRRemovalFixture) {
	t.Helper()
	for _, removed := range []string{fixture.revisionPath, fixture.modelSnapshot, fixture.backendSnapshot} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("prebuilt removed cache path %q stat error = %v, want not-exist", removed, err)
		}
	}
	if body, err := os.ReadFile(fixture.siblingPath); err != nil || string(body) != "preserved sibling revision" {
		t.Fatalf("prebuilt sibling revision body=%q error=%v, want preserved", body, err)
	}
}

func assertStory004EquivalentRemove(t testing.TB, left, right story004RemoveResponse) {
	t.Helper()
	if left.ModelName != right.ModelName || left.Revision != right.Revision || left.Outcome != right.Outcome ||
		left.BytesRemoved != right.BytesRemoved || left.ReclaimedCacheBytes == nil || right.ReclaimedCacheBytes == nil ||
		*left.ReclaimedCacheBytes != *right.ReclaimedCacheBytes || left.RetainedSharedCacheBytes == nil ||
		right.RetainedSharedCacheBytes == nil || *left.RetainedSharedCacheBytes != *right.RetainedSharedCacheBytes {
		t.Fatalf("prebuilt remove values differ: left=%#v right=%#v", left, right)
	}
}
