package messaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
			f := startHostEffects(t, home, dir, cell.settings, healthyProviderEffects())
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

func healthyProviderEffects() serviceedges.Edges {
	return serviceedges.Edges{ProviderCommandRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("healthy Factory independent of inbox COMPLETE"),
	})}
}

func assertHealthyWork(t *testing.T, f *fixture) {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(f.server.URL(), "~default"))
	defer stream.Close()
	request := map[string]any{"name": "healthy-work", "workTypeName": "task", "payload": map[string]string{"title": "healthy Factory independent of inbox"}}
	status, body := f.request(t, http.MethodPost, "/factory-sessions/~default/work", request, nil)
	if status != http.StatusCreated {
		t.Fatalf("Factory admission after Messaging fault: %d %s", status, body)
	}
	var submitted api.SubmitWorkResponse
	if err := json.Unmarshal(body, &submitted); err != nil {
		t.Fatal(err)
	}
	// This isolated session admits exactly one Work. Its public dispatch
	// completion signal precedes the single terminal Work assertion.
	deadline := time.Now().Add(15 * time.Second)
	for {
		event := stream.NextEvent(time.Until(deadline))
		if event.Type == api.FactoryEventTypeDispatchResponse {
			break
		}
	}
	page := support.GetJSON[api.ListWorkResponse](t, f.server.URL()+"/factory-sessions/~default/work")
	if support.CountWorkAtCustomerState(page, "task:done") != 1 {
		t.Fatal("Factory Work did not finish after Messaging outcome")
	}
}

func TestMessagingSettingsPreserveCanonicalFactoryOutcome(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var baseline []string
	for _, cell := range []struct{ name, settings, journal, code string }{
		{"disabled", `{"messaging":{"enabled":false}}`, "", "MESSAGING_DISABLED"},
		{"enabled", `{"messaging":{"enabled":true}}`, "", ""},
		{"corrupt", "", "preserved corrupt bytes\n", "MESSAGE_STORE_CORRUPT"},
		{"malformed", `{"messaging":{"limits":{"maxBodyBytes":0}}}`, "", "MESSAGE_STORE_UNAVAILABLE"},
	} {
		clock := platformclock.NewDeterministic(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Second)
		// Control timestamps while keeping production lifecycle timers real.
		effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
		home := t.TempDir()
		if cell.journal != "" {
			writeJournal(t, filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl"), cell.journal)
		}
		f := startHostEffects(t, home, dir, cell.settings, serviceedges.Merge(healthyProviderEffects(), effects))
		if cell.code != "" {
			assertError(t, f, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, cell.code)
		} else if page := readPage(t, f, "/messages", nil); len(page.Messages) != 0 {
			t.Fatal("isolated inbox is not empty")
		}
		assertHealthyWork(t, f)
		events := comparableFactoryEvents(t, f.server.GetFactoryEvents(t))
		f.server.Close(t)
		if baseline == nil {
			baseline = events
		} else if !reflect.DeepEqual(baseline, events) {
			t.Fatalf("messaging %s changed canonical Factory events:\ndisabled: %s\nactual: %s", cell.name, baseline, events)
		}
	}
}

func comparableFactoryEvents(t *testing.T, events []api.FactoryEvent) []string {
	t.Helper()
	// Replace only identities taken from canonical context, everywhere they
	// occur. Relationships, event order, ticks and all payload fields survive.
	identities := make(map[string]string)
	for _, event := range events {
		for _, identity := range []*string{event.Context.RequestId, event.Context.DispatchId} {
			if identity != nil {
				if _, exists := identities[*identity]; !exists {
					identities[*identity] = fmt.Sprintf("identity-%d", len(identities))
				}
			}
		}
	}
	result := make([]string, 0, len(events))
	for _, event := range events {
		event.Id = ""
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		value := string(encoded)
		for identity, replacement := range identities {
			value = strings.ReplaceAll(value, identity, replacement)
		}
		result = append(result, value)
	}
	return result
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

func TestReadOnlyRetentionStorePreservesInboxAndFactoryWork(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not revoke Windows write access; hosted Linux proves the real filesystem refusal")
	}
	clock := platformclock.NewDeterministic(time.Now().UTC(), time.Second)
	effects := serviceedges.Edges{Clock: struct{ platformclock.Source }{clock}}
	home, dir := t.TempDir(), t.TempDir()
	settings := `{"messaging":{"retentionDays":1,"limits":{"defaultExpirySeconds":60}}}`
	f := startHostEffects(t, home, dir, settings, effects, "lead", "lane")
	lead := f.invoke(t, "lead", nil)
	lane := f.invoke(t, "lane", lead)
	old := f.cliSend(t, lane, "lead", "--request-id", "old", "--body", "settled before retention")
	clock.SetTick(60)
	if page := readPage(t, f, "/messages?threadId="+old.ThreadId, lane); len(page.Messages) != 1 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("retention fixture did not durably settle its old message")
	}
	clock.SetTick(23 * 60 * 60)
	recent := f.cliSend(t, lane, "lead", "--request-id", "recent", "--body", "retain through refusal")
	f.server.Close(t)
	path := filepath.Join(home, ".you-agent-factory", "messages", "messages.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	storeDir := filepath.Dir(path)
	if err := os.Chmod(storeDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(storeDir, 0o700); err != nil {
			t.Error(err)
		}
	})
	clock.SetTick(25 * 60 * 60)
	second := startHostEffects(t, home, dir, settings, serviceedges.Merge(healthyProviderEffects(), effects))
	assertError(t, second, http.MethodGet, "/messages", nil, nil, http.StatusServiceUnavailable, "MESSAGE_STORE_UNAVAILABLE")
	assertHealthyWork(t, second)
	second.server.Close(t)
	preserved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, preserved) {
		t.Fatal("failed retention replaced the committed journal")
	}
	if err := os.Chmod(storeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	third := startHostEffects(t, home, dir, settings, effects)
	assertError(t, third, http.MethodGet, "/messages/"+old.MessageId, nil, nil, http.StatusNotFound, "MESSAGE_NOT_FOUND")
	if page := readPage(t, third, "/messages", nil); len(page.Messages) != 1 || page.Messages[0].MessageId != recent.MessageId || page.Messages[0].BodySha256 != recent.BodySha256 || page.Messages[0].Status != "EXPIRED" {
		t.Fatal("retention retry lost the surviving committed message")
	}
}
