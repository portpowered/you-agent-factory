package factory_events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryWebhooksInitialRecordedSessions proves the supported initial CLI
// activation path, rather than the retained unsupported replacement attempt.
func TestFactoryWebhooksInitialRecordedSessions(t *testing.T) {
	t.Parallel()
	receiverA := newFunctionalWebhookReceiver(t, func(functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: 204}
	})
	receiverB := newFunctionalWebhookReceiver(t, func(functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: 204}
	})
	resolverA := newFunctionalWebhookSecretResolver()
	a := startRecordedWebhookConfig(t, []map[string]any{
		functionalWebhook("a", true, receiverA.URL()+"/a", "secrets/a", []string{"WORK_STATE_CHANGE"}, nil),
		functionalWebhook("filtered", true, receiverA.URL()+"/filtered", "secrets/filtered", []string{"DISPATCH_RESPONSE"}, []string{"FAILED"}),
		functionalWebhook("disabled", false, receiverA.URL()+"/disabled", "secrets/disabled", []string{"WORK_STATE_CHANGE"}, nil),
	}, resolverA)
	b := startInitialRecordedWebhookSession(t, receiverB.URL(), "b")
	for _, test := range []struct {
		session  *initialRecordedWebhookSession
		receiver *functionalWebhookReceiver
	}{{a, receiverA}, {b, receiverB}} {
		stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(test.session.url, test.session.id))
		payload := filepath.Join(t.TempDir(), "request.md")
		if err := os.WriteFile(payload, []byte("# scoped webhook witness"), 0600); err != nil {
			t.Fatal(err)
		}
		response := executeInitialWebhookJSON(t, test.session, "submit", "--name", "scoped", "--work-type-name", "task", "--payload", payload)
		var work struct {
			WorkID string `json:"workId"`
		}
		if err := json.Unmarshal(response, &work); err != nil || work.WorkID == "" {
			t.Fatalf("submit response = %s, error = %v", response, err)
		}
		executeInitialWebhookJSON(t, test.session, "work", "move", work.WorkID, "complete")
		event := waitForTerminalWorkEvent(t, stream, work.WorkID, 30*time.Second)
		delivery := test.receiver.waitForEvent(t, event.Id, 30*time.Second)
		assertSignedCanonicalWebhook(t, delivery, event, functionalWebhookSecret)
		for _, observed := range test.receiver.requestsSnapshot() {
			var envelope factoryapi.FactoryEvent
			if err := json.Unmarshal(observed.body, &envelope); err != nil {
				t.Fatal(err)
			}
			if observed.path == "/filtered" || observed.path == "/disabled" {
				t.Fatalf("nonmatching/disabled endpoint delivered: %s", observed.path)
			}
			if envelope.Context.SessionId == nil || *envelope.Context.SessionId != test.session.id {
				t.Fatalf("foreign or startup event delivered: %s", observed.body)
			}
		}
		events := support.GetFactoryEventsForSessionAt(t, test.session.url, test.session.id)
		found := false
		for _, publicEvent := range events {
			if publicEvent.Id == event.Id {
				found = true
				assertSignedCanonicalWebhook(t, delivery, publicEvent, functionalWebhookSecret)
			}
		}
		if !found {
			t.Fatalf("delivered event %s absent from canonical recorded-session read", event.Id)
		}
	}
	resolverA.assertResolved(t, "secrets/a", "secrets/filtered")
	resolverA.assertNotResolved(t, "secrets/disabled")
	a.command.Stop(t)
	b.command.Stop(t)
	for _, session := range []*initialRecordedWebhookSession{a, b} {
		info, err := os.Stat(session.recordPath)
		if err != nil || info.Size() == 0 {
			t.Fatalf("recorded artifact %s missing/empty: %v", session.recordPath, err)
		}
	}
}

type initialRecordedWebhookSession struct {
	id, url, dir, profile, recordPath string
	command                           *support.ProcessCommand
}

func startInitialRecordedWebhookSession(t *testing.T, receiver, name string) *initialRecordedWebhookSession {
	t.Helper()
	return startRecordedWebhookConfig(t, []map[string]any{functionalWebhook(name, true, receiver+"/"+name, "secrets/"+name, []string{"WORK_STATE_CHANGE"}, nil)}, newFunctionalWebhookSecretResolver())
}

func startRecordedWebhookConfig(t *testing.T, configs []map[string]any, resolver *functionalWebhookSecretResolver) *initialRecordedWebhookSession {
	t.Helper()
	return startRecordedWebhookDirectory(t, scaffoldWebhookFactory(t, configs), configs, resolver)
}

func startRecordedWebhookDirectory(t *testing.T, dir string, configs []map[string]any, resolver *functionalWebhookSecretResolver) *initialRecordedWebhookSession {
	t.Helper()
	session := &initialRecordedWebhookSession{id: uuid.NewString(), dir: dir, profile: t.TempDir(), recordPath: filepath.Join(dir, "session-recording.json")}
	api := support.NewProcessAPIServer()
	recordedWebhookEffects.Lock()
	port := recordedWebhookEffects.nextPort
	recordedWebhookEffects.nextPort++
	recordedWebhookEffects.servers[port] = api
	recordedWebhookEffects.resolvers[filepath.Clean(dir)] = resolver
	recordedWebhookEffects.Unlock()
	t.Cleanup(func() {
		recordedWebhookEffects.Lock()
		delete(recordedWebhookEffects.servers, port)
		delete(recordedWebhookEffects.resolvers, filepath.Clean(dir))
		recordedWebhookEffects.Unlock()
	})
	inputs := support.FakeInputs(context.Background(), []string{"you", "run", "--dir", dir, "--session", session.id, "--record", session.recordPath, "--continuously", "--with-server", "--listen", "127.0.0.1:" + strconv.Itoa(port), "--quiet"})
	inputs.WorkingDirectory = dir
	inputs.Env = initialWebhookEnvironment(session.profile)
	session.command = support.StartProcessCommand(t, factoryEventsCLIProcess, inputs.Input)
	session.url = api.WaitForURL(t)
	enabled := 0
	for _, config := range configs {
		if active, _ := config["enabled"].(bool); active {
			enabled++
		}
	}
	resolver.waitForCount(t, enabled, 30*time.Second)
	return session
}

func initialWebhookEnvironment(profile string) []string {
	var result []string
	for _, entry := range os.Environ() {
		switch strings.ToUpper(strings.SplitN(entry, "=", 2)[0]) {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH":
		default:
			result = append(result, entry)
		}
	}
	return append(result, "HOME="+profile, "USERPROFILE="+profile)
}

func executeInitialWebhookJSON(t *testing.T, session *initialRecordedWebhookSession, args ...string) []byte {
	t.Helper()
	command := append([]string{"you", "--server", session.url, "--json"}, args...)
	command = append(command, "--session", session.id)
	inputs := support.FakeInputs(t.Context(), command)
	inputs.WorkingDirectory = session.dir
	inputs.Env = initialWebhookEnvironment(session.profile)
	if err := factoryEventsCLIProcess.Execute(inputs.Input); err != nil {
		t.Fatalf("CLI %v: %v\nstdout: %s\nstderr: %s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return []byte(strings.TrimSpace(inputs.Stdout()))
}

// One ordered recovery journey owns the selected process clock. Its stages
// deliberately observe successive advances of that same customer-visible time
// source; other session journeys still run in parallel and own their effects.
func TestFactoryWebhooksRecordedFaultsAndPeerContinuation(t *testing.T) {
	t.Parallel()
	peerReceiver := newFunctionalWebhookReceiver(t, func(functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: 204}
	})
	peer := startInitialRecordedWebhookSession(t, peerReceiver.URL(), "peer")
	for _, fault := range []string{"F18b retry success", "F18c outage", "F18d secret", "F18e append", "F18f close", "terminal 400", "empty"} {
		t.Run(fault, func(t *testing.T) {
			runRecordedWebhookFault(t, peer, peerReceiver, fault)
		})
	}
}

func runRecordedWebhookFault(t *testing.T, peer *initialRecordedWebhookSession, peerReceiver *functionalWebhookReceiver, fault string) {
	t.Helper()
	var mu sync.Mutex
	firstID := ""
	attempts := 0
	receiver := newFunctionalWebhookReceiver(t, func(request functionalWebhookRequest) functionalWebhookResponse {
		mu.Lock()
		defer mu.Unlock()
		if firstID == "" {
			firstID = request.eventID
		}
		if request.eventID != firstID {
			return functionalWebhookResponse{status: 204}
		}
		attempts++
		switch fault {
		case "F18b retry success":
			if attempts > 1 {
				return functionalWebhookResponse{status: 204}
			}
		case "terminal 400":
			return functionalWebhookResponse{status: 400}
		}
		return functionalWebhookResponse{status: 503, body: "receiver-body-canary"}
	})
	resolver := newFunctionalWebhookSecretResolver()
	if fault == "F18d secret" {
		resolver.failure = errors.New("resolver-secret-canary")
	}
	maxAttempts := 3
	if fault == "F18b retry success" {
		maxAttempts = 2
	}
	configs := []map[string]any{functionalWebhookWithPolicy("fault", true, receiver.URL()+"/fault?token=query-canary", "secrets/fault", []string{"WORK_STATE_CHANGE"}, nil,
		map[string]any{"maxAttempts": maxAttempts, "initialBackoff": "1s", "maxBackoff": "2s", "backoffMultiplier": 2.0, "requestTimeout": "30s"})}
	if fault == "empty" {
		configs = nil
	}
	session := startRecordedWebhookConfig(t, configs, resolver)
	path := filepath.Join(session.dir, filepath.FromSlash(webhooks.DeadLetterRelativePath))
	appendCalls := make(chan []byte, 4)
	if fault == "F18e append" {
		recordedWebhookEffects.Lock()
		recordedWebhookEffects.appenders[filepath.Clean(path)] = func(_ string, line []byte) error {
			appendCalls <- append([]byte(nil), line...)
			return errors.New("appender-secret-canary")
		}
		recordedWebhookEffects.Unlock()
		t.Cleanup(func() {
			recordedWebhookEffects.Lock()
			delete(recordedWebhookEffects.appenders, filepath.Clean(path))
			recordedWebhookEffects.Unlock()
		})
	}
	event := completeInitialWebhookWork(t, session)
	if fault != "F18d secret" && fault != "empty" {
		first := receiver.waitForWorkEvent(t, eventWorkID(t, event), 30*time.Second)
		retries := 2
		if fault == "F18b retry success" {
			retries = 1
		}
		if fault == "terminal 400" {
			retries = 0
		}
		for retry := 0; retry < retries; retry++ {
			waitRecordedWebhookTimer(t)
			if fault == "F18f close" {
				break
			}
			recordedWebhookClock.Advance(time.Duration(1<<retry) * time.Second)
			receiver.waitForEvent(t, first.eventID, 30*time.Second)
		}
		if fault == "F18f close" {
			response, err := http.Post(session.url+"/factory-sessions/"+session.id+"/terminate", "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatalf("terminate status=%d: %s", response.StatusCode, body)
			}
		} else {
			// The later terminal Work event is processed only after the first event's
			// persistence/success. This observes a deterministic appender completion.
			followup := completeInitialWebhookWork(t, session)
			receiver.waitForEvent(t, followup.Id, 30*time.Second)
			events := support.GetFactoryEventsForSessionAt(t, session.url, session.id)
			found := false
			for _, canonical := range events {
				if canonical.Id == first.eventID {
					found = true
					assertSignedCanonicalWebhook(t, first, canonical, functionalWebhookSecret)
				}
			}
			if !found {
				t.Fatal("first delivery absent from canonical session read")
			}
			verifyRecordedWebhookFault(t, receiver, first, path, appendCalls, fault)
		}
	}
	session.command.Stop(t)
	before := len(receiver.requestsSnapshot())
	if fault == "F18f close" && before != 1 {
		t.Fatalf("close attempts = %d, want one before retry cancellation", before)
	}
	peerEvent := completeInitialWebhookWork(t, peer)
	delivery := peerReceiver.waitForEvent(t, peerEvent.Id, 30*time.Second)
	assertSignedCanonicalWebhook(t, delivery, peerEvent, functionalWebhookSecret)
	if len(receiver.requestsSnapshot()) != before {
		t.Fatal("closed session delivered while its peer progressed")
	}
	if fault == "F18d secret" || fault == "empty" {
		if before != 0 {
			t.Fatalf("inert subscription delivered %d requests", before)
		}
	}
}

func completeInitialWebhookWork(t *testing.T, session *initialRecordedWebhookSession) factoryapi.FactoryEvent {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(session.url, session.id))
	path := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(path, []byte("# webhook fault witness"), 0600); err != nil {
		t.Fatal(err)
	}
	var response struct {
		WorkID string `json:"workId"`
	}
	raw := executeInitialWebhookJSON(t, session, "submit", "--name", "fault-"+uuid.NewString(), "--work-type-name", "task", "--payload", path)
	if err := json.Unmarshal(raw, &response); err != nil || response.WorkID == "" {
		t.Fatalf("submit response: %s (%v)", raw, err)
	}
	executeInitialWebhookJSON(t, session, "work", "move", response.WorkID, "complete")
	return waitForTerminalWorkEvent(t, stream, response.WorkID, 30*time.Second)
}

func eventWorkID(t *testing.T, event factoryapi.FactoryEvent) string {
	t.Helper()
	var payload struct {
		WorkID string `json:"workId"`
	}
	encoded, marshalErr := json.Marshal(event.Payload)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || payload.WorkID == "" {
		t.Fatalf("Work event payload: %s (%v)", event.Payload, err)
	}
	return payload.WorkID
}

func waitRecordedWebhookTimer(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := recordedWebhookClock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("retry timer: %v", err)
	}
}

func verifyRecordedWebhookFault(t *testing.T, receiver *functionalWebhookReceiver, first functionalWebhookRequest, path string, appendCalls <-chan []byte, fault string) {
	t.Helper()
	var selected []functionalWebhookRequest
	for _, request := range receiver.requestsSnapshot() {
		if request.eventID == first.eventID {
			selected = append(selected, request)
		}
	}
	expected := 3
	if fault == "F18b retry success" {
		expected = 2
	}
	if fault == "terminal 400" {
		expected = 1
	}
	if len(selected) != expected {
		t.Fatalf("attempt count=%d, want %d", len(selected), expected)
	}
	for index, request := range selected {
		if !bytes.Equal(first.body, request.body) {
			t.Fatal("retry canonical body changed")
		}
		assertWebhookSignature(t, request, functionalWebhookSecret)
		if index > 0 && request.headers.Get(webhooks.TimestampHeader) == selected[index-1].headers.Get(webhooks.TimestampHeader) {
			t.Fatal("retry timestamp was not refreshed")
		}
	}
	if fault == "F18b retry success" {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retry success dead-letter result: %v", err)
		}
		return
	}
	var line []byte
	if fault == "F18e append" {
		select {
		case line = <-appendCalls:
		case <-time.After(30 * time.Second):
			t.Fatal("no failed append effect")
		}
		select {
		case <-appendCalls:
			t.Fatal("append storm")
		default:
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed append claimed persistence: %v", err)
		}
	} else {
		var err error
		line, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	var record struct {
		AttemptCount   int             `json:"attemptCount"`
		TerminalReason string          `json:"terminalReason"`
		CanonicalBody  json.RawMessage `json:"canonicalBody"`
		EventID        string          `json:"eventId"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatal(err)
	}
	reason := "retry_exhausted"
	if fault == "terminal 400" {
		reason = "non_retryable_http_status"
	}
	if record.AttemptCount != expected || record.TerminalReason != reason || record.EventID != first.eventID || !bytes.Equal(record.CanonicalBody, first.body) || bytes.Count(line, []byte("\n")) != 1 {
		t.Fatalf("terminal record: %s", line)
	}
	for _, canary := range []string{functionalWebhookSecret, "receiver-body-canary", "query-canary", "appender-secret-canary"} {
		if bytes.Contains(line, []byte(canary)) {
			t.Fatalf("terminal record leaked %q", canary)
		}
	}
}

func TestFactoryWebhooksInitialRecordedDispatchFailure(t *testing.T) {
	t.Parallel()
	receiver := newFunctionalWebhookReceiver(t, func(functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: 204}
	})
	configs := []map[string]any{functionalWebhook("dispatch", true, receiver.URL()+"/dispatch", "secrets/dispatch", []string{"DISPATCH_RESPONSE"}, []string{"FAILED"})}
	session := startRecordedWebhookDirectory(t, scaffoldWebhookDispatchFailureFactory(t, configs), configs, newFunctionalWebhookSecretResolver())
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(session.url, session.id))
	path := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(path, []byte("# controlled dispatch failure"), 0600); err != nil {
		t.Fatal(err)
	}
	executeInitialWebhookJSON(t, session, "submit", "--name", "dispatch-failure", "--work-type-name", "task", "--payload", path)
	event := waitForDispatchResponseEvent(t, stream, 30*time.Second)
	request := receiver.waitForEvent(t, event.Id, 30*time.Second)
	assertSignedCanonicalWebhook(t, request, event, functionalWebhookSecret)
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil || payload.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch failure payload = %#v (%v)", payload, err)
	}
	session.command.Stop(t)
	for _, delivery := range receiver.requestsSnapshot() {
		if delivery.path != "/dispatch" {
			t.Fatalf("unexpected dispatch route: %s", delivery.path)
		}
	}
}
