package http_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Recovery owns a fresh store with one healthy and one identifiable but
// invalid snapshot. The customer must distinguish unavailable from unknown;
// a damaged sibling must not hide a healthy captured history.
func assertDamagedCapturedRecovery(t *testing.T, config support.FunctionalAPIServerConfig, current factoryapi.WorkerSessionLogPage) {
	t.Helper()
	root := t.TempDir()
	path := writeLegacyCapturedFixture(t, root, current)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var damaged recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(data, &damaged); err != nil {
		t.Fatal(err)
	}
	damaged.RecordingID = "damaged-recording"
	damaged.Sessions[0].WorkerSessionID = "damaged-worker"
	damaged.Sessions[0].Records[len(damaged.Sessions[0].Records)-1].SchemaID = ""
	data, err = json.Marshal(damaged)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(damaged.RecordingID))
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), hex.EncodeToString(digest[:])+".worker.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	config.Edges.FactorySessionsWorkingDirectory = capturedRecordingDirectory(root)
	server := support.StartFunctionalAPIServer(t, config)
	for range 2 {
		assertDamagedCapturedRead(t, server)
		page := assertLegacyLogsCLIHTTPParity(t, server, current.WorkerSessionId)
		assertLegacyCapturedEvents(t, page, current)
	}
}

func assertDamagedCapturedRead(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	response, err := http.Get(server.URL() + "/worker-sessions/damaged-worker/logs")
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
	if strings.Contains(string(data), "damaged-recording") || strings.Contains(string(data), "schemaId") {
		t.Fatal("damaged history leaked storage identity or decoder diagnostics")
	}
	inputs := support.FakeInputs(t.Context(), []string{"you", "worker-sessions", "read", "--worker-session-id", "damaged-worker", "--view", "logs", "--server", server.URL(), "--output", "json"})
	if err := server.Execute(t, inputs.Input); err == nil || inputs.Stdout() != "" {
		t.Fatalf("CLI fabricated a damaged page: error=%v output=%s", err, inputs.Stdout())
	}
}
