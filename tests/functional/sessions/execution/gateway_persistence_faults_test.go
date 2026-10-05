package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The API owns durable admission and persisted inspection. Both hosts enter
// through root.BuildProcess + Process.Execute in StartFunctionalAPIServer.
// Restart is necessary to cross the real snapshot read boundary rather than
// reading the first host's retained in-memory projection. Only the selected
// filesystem operation fails; every other operation uses local atomic storage.
func TestGatewayPersistenceFaultsPreserveRetryAndPeerReads(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	dir := scaffoldFSCP03ProbeFactory(t)
	home := t.TempDir()
	files := &gatewaySnapshotFaults{storage: platformreplay.NewLocal(runtime.GOOS)}
	cfg := support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Env:        append(os.Environ(), "HOME="+home, "USERPROFILE="+home),
		Edges: serviceedges.Edges{
			FactorySessionResolveHomeDirectory:         func() (string, error) { return home, nil },
			FactorySessionRuntimePersistenceFileSystem: files,
			BrowserOpener: func(context.Context, string) error { return nil },
		},
	}
	server := support.StartFunctionalAPIServer(t, cfg)
	t.Cleanup(func() { server.Close(t) })
	peer := startGatewaySnapshotSession(t, server.URL(), "gateway-peer")
	files.failNextWrite()
	status, body := gatewaySnapshotRequest(t, http.MethodPost, server.URL()+"/factory-sessions/sync", gatewaySnapshotStartBody("gateway-retry"))
	if status != http.StatusInternalServerError || !strings.Contains(string(body), `"code":"INTERNAL_ERROR"`) || !strings.Contains(string(body), "durable factory session execution failed") {
		t.Fatalf("failed snapshot admission = %d %s, want visible write failure", status, body)
	}
	failedPath := files.failedWritePath()
	if failedPath == "" {
		t.Fatal("admission did not reach the controlled snapshot write")
	}
	if _, err := os.Stat(failedPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("failed snapshot stat = %v, want no successful persistence artifact", err)
	}
	assertGatewaySnapshotResult(t, server.URL(), peer.SessionId)
	// Recovery submits a fresh operation; reusing a failed idempotency tuple
	// has separate admission semantics and is not a new retry policy here.
	retried := startGatewaySnapshotSession(t, server.URL(), "gateway-retry-success")
	assertGatewaySnapshotResult(t, server.URL(), retried.SessionId)
	assertGatewaySnapshotInventory(t, server.URL(), peer.SessionId, retried.SessionId)
	server.Close(t)

	restarted := support.StartFunctionalAPIServer(t, cfg)
	t.Cleanup(func() { restarted.Close(t) })
	files.failRead(retried.SessionId)
	status, body = gatewaySnapshotRequest(t, http.MethodGet, restarted.URL()+"/factory-sessions/"+retried.SessionId, nil)
	// Existing inspection maps snapshot read failures to NOT_FOUND; resume
	// has a separate corrupted-persistence outcome. Preserve that distinction.
	if status != http.StatusNotFound || !strings.Contains(string(body), `"code":"NOT_FOUND"`) {
		t.Fatalf("failed persisted read = %d %s, want visible persistence error", status, body)
	}
	if !files.readFaultObserved() {
		t.Fatal("persisted inspection did not reach the selected snapshot read")
	}
	assertGatewaySnapshotResult(t, restarted.URL(), peer.SessionId)
	files.failRead("")
	assertGatewaySnapshotResult(t, restarted.URL(), retried.SessionId)
}

func gatewaySnapshotStartBody(requestID string) []byte {
	return []byte(`{"requestId":"` + requestID + `","source":{"kind":"INLINE_WORKFLOW","inlineWorkflow":{"inlineSource":{"encoding":"utf8","inline":"return 'gateway snapshot COMPLETE';"}}}}`)
}

func startGatewaySnapshotSession(t *testing.T, baseURL, requestID string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	status, body := gatewaySnapshotRequest(t, http.MethodPost, baseURL+"/factory-sessions/sync", gatewaySnapshotStartBody(requestID))
	if status != http.StatusOK {
		t.Fatalf("durable start %q = %d %s", requestID, status, body)
	}
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode durable start: %v", err)
	}
	if result.SessionId == "" || string(result.Status) != "SUCCEEDED" {
		t.Fatalf("durable start = %#v (%s), want terminal success", result, body)
	}
	return result
}

func assertGatewaySnapshotResult(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	status, body := gatewaySnapshotRequest(t, http.MethodGet, baseURL+"/factory-sessions/"+sessionID+"/results", nil)
	if status != http.StatusOK || !strings.Contains(string(body), "gateway snapshot COMPLETE") || !strings.Contains(string(body), sessionID) {
		t.Fatalf("selected result %q = %d %s, want retained attributed output", sessionID, status, body)
	}
}

func assertGatewaySnapshotInventory(t *testing.T, baseURL string, sessionIDs ...string) {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, baseURL+"/factory-sessions?scope=persisted")
	for _, id := range sessionIDs {
		found := false
		if listed.DurableSessions != nil {
			for _, row := range *listed.DurableSessions {
				if row.SessionId == id && string(row.Status) == "SUCCEEDED" {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("persisted inventory = %#v, want succeeded session %q", listed, id)
		}
	}
}

func gatewaySnapshotRequest(t *testing.T, method, endpoint string, body []byte) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build snapshot request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("snapshot request: %v", err)
	}
	defer response.Body.Close()
	encoded, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read snapshot response: %v", err)
	}
	return response.StatusCode, encoded
}

type gatewaySnapshotFaults struct {
	platformfilesystem.Local
	storage      platformreplay.Local
	mu           sync.Mutex
	nextWrite    bool
	writePath    string
	readSession  string
	readObserved bool
}

func (f *gatewaySnapshotFaults) WriteFile(path string, data []byte, _ fs.FileMode) error {
	f.mu.Lock()
	fail := f.nextWrite
	if fail {
		f.nextWrite = false
		f.writePath = path
	}
	f.mu.Unlock()
	if fail {
		return errors.New("injected gateway snapshot write failure")
	}
	return f.storage.WriteFile(path, data)
}

func (f *gatewaySnapshotFaults) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	fail := f.readSession != "" && filepath.Base(path) == f.readSession+".json"
	if fail {
		f.readObserved = true
	}
	f.mu.Unlock()
	if fail {
		return nil, errors.New("injected gateway snapshot read failure")
	}
	return f.Local.ReadFile(path)
}

func (f *gatewaySnapshotFaults) failNextWrite() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextWrite = true
}

func (f *gatewaySnapshotFaults) failedWritePath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writePath
}

func (f *gatewaySnapshotFaults) failRead(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readSession = sessionID
	f.readObserved = false
}

func (f *gatewaySnapshotFaults) readFaultObserved() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readObserved
}
