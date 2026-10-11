package messaging

import (
	"bytes"
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

func TestFailedReplyWritePreservesParentAndRetryAuthority(t *testing.T) {
	t.Parallel()
	f := startHost(t, t.TempDir(), t.TempDir(), `{"messaging":{"limits":{"sendsPerChainPerHour":1}}}`, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	parent := f.cliSend(t, lane, "lead", "--request-id", "parent", "--body", "durable parent")
	path := filepath.Join(f.home, ".you-agent-factory", "messages", "messages.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup := path + ".preserved"
	// A directory at the selected file path is a real, portable write fault.
	// Preserve the customer's bytes while making only this test-owned path
	// unwritable; restore it before retrying the same public request.
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"requestId": "retry-reply", "inReplyTo": parent.MessageId, "body": "retryable reply"}
	assertError(t, f, http.MethodPost, "/messages", request, lead, http.StatusServiceUnavailable, "MESSAGE_STORE_UNAVAILABLE")
	page := readPage(t, f, "/messages?threadId="+parent.ThreadId, lane)
	if len(page.Messages) != 1 || page.Messages[0].Status != "QUEUED" || page.Messages[0].RepliedByMessageId != nil {
		t.Fatal("failed flush published reply or changed its parent")
	}
	preserved, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatal("failed write altered previously durable records")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	reply := f.send(t, request, lead)
	if reply.ThreadId != parent.ThreadId || reply.To.WorkerSessionId != "lane" {
		t.Fatal("write failure spent request alias or successful-send authority")
	}
	assertRepliedParent(t, f, parent.MessageId, reply.MessageId)
	if replay := f.send(t, request, lead); replay.MessageId != reply.MessageId {
		t.Fatal("successful retry was not idempotent")
	}
	f.server.Close(t)
	second := startHost(t, f.home, f.dir, "")
	assertRepliedParent(t, second, parent.MessageId, reply.MessageId)
	if page := readPage(t, second, "/messages?threadId="+parent.ThreadId, nil); len(page.Messages) != 2 {
		t.Fatal("reconstruction lost atomic reply or retained a failed attempt")
	}
}
