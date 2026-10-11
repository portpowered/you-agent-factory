package messaging

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMessagingFaultsLeaveFactoryWorkHealthy(t *testing.T) {
	t.Parallel()
	for _, cell := range []struct{ name, settings, journal, code string }{
		{"enabled", "", "", ""},
		{"disabled", `{"messaging":{"enabled":false}}`, "", "MESSAGING_DISABLED"},
		{"invalid settings", `{"messaging":{"limits":{"maxBodyBytes":0}}}`, "", "MESSAGE_STORE_UNAVAILABLE"},
		{"corrupt store", "", "existing-customer-bytes\n", "MESSAGE_STORE_CORRUPT"},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			home, dir := t.TempDir(), t.TempDir()
			path := filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl")
			if cell.journal != "" {
				writeJournal(t, path, cell.journal)
			}
			f := startHostMode(t, home, dir, cell.settings, true)
			if cell.code != "" {
				assertError(t, f, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, cell.code)
			} else {
				status, body := f.request(t, http.MethodGet, "/messages", nil, nil)
				if status != http.StatusOK || !strings.Contains(string(body), `"messages":[]`) {
					t.Fatalf("empty configured inbox: %d %s", status, body)
				}
			}
			assertHealthyWork(t, f)
			if cell.journal != "" {
				bytes, err := os.ReadFile(path)
				if err != nil || string(bytes) != cell.journal {
					t.Fatal("quarantine repaired or erased customer bytes")
				}
			}
		})
	}
}

func writeJournal(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertHealthyWork(t *testing.T, f *fixture) {
	t.Helper()
	request := map[string]any{"name": "healthy-work", "workTypeName": "task", "payload": map[string]string{"title": "healthy Factory independent of inbox"}}
	status, body := f.request(t, http.MethodPost, "/factory-sessions/~default/work", request, nil)
	if status != http.StatusCreated {
		t.Fatalf("Factory admission after Messaging fault: %d %s", status, body)
	}
	var submitted api.SubmitWorkResponse
	if err := json.Unmarshal(body, &submitted); err != nil {
		t.Fatal(err)
	}
	_, err := support.WaitForObservation(15*time.Second, func() (api.ListWorkResponse, error) {
		return support.GetJSON[api.ListWorkResponse](t, f.server.URL()+"/factory-sessions/~default/work"), nil
	}, func(page api.ListWorkResponse) bool { return support.CountWorkAtCustomerState(page, "task:done") == 1 })
	if err != nil {
		t.Fatalf("Factory Work did not finish after Messaging outcome: %v", err)
	}
}
