package customer_journeys_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type capturedReadFault struct {
	platformreplay.Local
	unavailable atomic.Bool
}

func (storage *capturedReadFault) ReadFile(path string) ([]byte, error) {
	if storage.unavailable.Load() {
		return nil, errors.New(capturedStorageFault)
	}
	return storage.Local.ReadFile(path)
}

func assertUnreadableCapturedRecovery(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	root := t.TempDir()
	copyCurrentCapturedJournal(t, config, root)
	local := platformreplay.NewLocal(runtime.GOOS)
	storage := &capturedReadFault{Local: local}
	storage.unavailable.Store(true)
	config.Edges.RecordingReadFile = storage.ReadFile
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	server := support.StartFunctionalAPIServer(t, config)
	for range 2 {
		assertUnavailableCapturedRead(t, server, current.WorkerSessionId)
	}
	storage.unavailable.Store(false)
	page := assertRestoredLogsCLIHTTPParity(t, server, current.WorkerSessionId)
	assertRestoredCapturedEvents(t, page, current)
}

// Recovery owns a fresh store with one healthy and one identifiable but
// invalid snapshot. The customer must distinguish unavailable from unknown;
// a damaged sibling must not hide a healthy captured history.
func assertDamagedCapturedRecovery(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	root := t.TempDir()
	path, journal := copyCurrentCapturedJournal(t, config, root)
	recordingID := journal[0]["recordingId"].(string)
	for _, row := range journal {
		row["recordingId"] = "damaged-recording"
		row["workerSessionId"] = "damaged-worker"
	}
	journal[len(journal)-1]["record"].(map[string]any)["SchemaID"] = ""
	digest := sha256.Sum256([]byte("damaged-recording"))
	damagedPath := filepath.Join(filepath.Dir(path), hex.EncodeToString(digest[:])+".worker.jsonl")
	writeCapturedJournal(t, damagedPath, journal)
	data, err := os.ReadFile(damagedPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(strings.ReplaceAll(string(data), current.WorkerSessionId, "damaged-worker"), recordingID, "damaged-recording"))
	if err := os.WriteFile(damagedPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	server := support.StartFunctionalAPIServer(t, config)
	for range 2 {
		assertDamagedCapturedRead(t, server)
		page := assertRestoredLogsCLIHTTPParity(t, server, current.WorkerSessionId)
		assertRestoredCapturedEvents(t, page, current)
	}
}

func assertDamagedCapturedRead(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	assertUnavailableCapturedRead(t, server, "damaged-worker")
}

func assertUnavailableCapturedRead(t *testing.T, server *support.FunctionalAPIServer, id string) {
	t.Helper()
	response, err := http.Get(server.URL() + "/worker-sessions/" + id + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var failure factoryapi.ErrorResponse
	if err := json.Unmarshal(data, &failure); err != nil || response.StatusCode != http.StatusServiceUnavailable || failure.Code != factoryapi.ErrorResponseCodeWORKERSESSIONRECORDINGUNAVAILABLE {
		t.Fatalf("damaged history response: status=%d body=%s error=%v", response.StatusCode, data, err)
	}
	if strings.Contains(string(data), "damaged-recording") || strings.Contains(string(data), "schemaId") || strings.Contains(string(data), "captured-storage-sentinel") {
		t.Fatal("damaged history leaked storage identity or decoder diagnostics")
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" {
		t.Fatalf("CLI fabricated a damaged page: error=%v output=%s", err, inputs.Stdout())
	}
	assertCapturedFollowFailure(t, server, id, "", "WORKER_SESSION_RECORDING_UNAVAILABLE", nil)
}
