package factory_events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const functionalWebhookSecret = "functional-webhook-secret"

// TestFactoryWebhooksRunThroughRootProcess proves the customer path from a
// root-built Process through the public API and canonical Factory Event stream.
// It also proves filtering, disabled-endpoint isolation, independent signature
// verification, and that transient receiver failures are retried without
// delaying the Work terminal event.
func TestFactoryWebhooksRunThroughRootProcess(t *testing.T) {
	t.Run("transition filtering and signature", testFunctionalWebhookTransition)
	t.Run("retry then success", testFunctionalWebhookRetrySuccess)
	t.Run("retry exhaustion writes one redacted dead letter", testFunctionalWebhookRetryExhaustion)
}

func testFunctionalWebhookTransition(t *testing.T) {
	receiver := newFunctionalWebhookReceiver(t, func(request functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: http.StatusOK}
	})
	resolver := newFunctionalWebhookSecretResolver()
	dir := scaffoldWebhookFactory(t, []map[string]any{
		functionalWebhook("transition", true, receiver.URL()+"/transition", "secrets/transition", []string{"WORK_STATE_CHANGE"}, nil),
		functionalWebhook("filtered", true, receiver.URL()+"/filtered", "secrets/filtered", []string{"DISPATCH_RESPONSE"}, []string{"FAILED"}),
		functionalWebhook("disabled", false, receiver.URL()+"/disabled", "secrets/disabled", []string{"WORK_STATE_CHANGE"}, nil),
	})
	server := startWebhookFunctionalServer(t, dir, serviceedges.Edges{
		FactoryWebhookSecretResolver: resolver.resolve,
	})
	resolver.waitForCount(t, 2, 5*time.Second)

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	workID := submitFunctionalWebhookWork(t, process, server.URL(), "transition-filtering")
	moveFunctionalWebhookWork(t, process, server.URL(), workID, "complete")
	terminalEvent := waitForTerminalWorkEvent(t, stream, workID, 15*time.Second)
	request := receiver.waitForEvent(t, terminalEvent.Id, 5*time.Second)

	assertSignedCanonicalWebhook(t, request, terminalEvent, functionalWebhookSecret)
	if request.path != "/transition" {
		t.Fatalf("matching request path = %q, want /transition", request.path)
	}
	for _, observed := range receiver.requestsSnapshot() {
		if observed.path != "/transition" {
			t.Fatalf("nonmatching or disabled endpoint received request at %q", observed.path)
		}
	}
	resolver.assertResolved(t, "secrets/transition", "secrets/filtered")
	resolver.assertNotResolved(t, "secrets/disabled")
}

func testFunctionalWebhookRetrySuccess(t *testing.T) {
	fakeClock := clockwork.NewFakeClockAt(time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC))
	var attemptsMu sync.Mutex
	attempts := make(map[string]int)
	receiver := newFunctionalWebhookReceiver(t, func(request functionalWebhookRequest) functionalWebhookResponse {
		attemptsMu.Lock()
		attempts[request.eventID]++
		attempt := attempts[request.eventID]
		attemptsMu.Unlock()
		if attempt == 1 {
			return functionalWebhookResponse{status: http.StatusServiceUnavailable, body: "temporary receiver response"}
		}
		return functionalWebhookResponse{status: http.StatusOK}
	})
	dir := scaffoldWebhookFactory(t, []map[string]any{
		functionalWebhookWithPolicy(
			"retry", true, receiver.URL()+"/retry", "secrets/retry", []string{"WORK_STATE_CHANGE"}, nil,
			map[string]any{"maxAttempts": 2, "initialBackoff": "1s", "maxBackoff": "1s", "requestTimeout": "1s"},
		),
	})
	resolver := newFunctionalWebhookSecretResolver()
	server := startWebhookFunctionalServer(t, dir, serviceedges.Edges{
		Clock: fakeClock, FactoryWebhookSecretResolver: resolver.resolve,
	})
	resolver.waitForCount(t, 1, 5*time.Second)

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	workID := submitFunctionalWebhookWork(t, process, server.URL(), "retry-success")
	moveFunctionalWebhookWork(t, process, server.URL(), workID, "complete")
	first := receiver.waitForWorkEvent(t, workID, 5*time.Second)
	fakeClock.BlockUntil(1)
	fakeClock.Advance(time.Second)
	second := receiver.waitForEvent(t, first.eventID, 5*time.Second)
	terminalEvent := waitForTerminalWorkEvent(t, stream, workID, 15*time.Second)

	if first.path != "/retry" || second.path != "/retry" {
		t.Fatalf("retry paths = %q, %q; want /retry", first.path, second.path)
	}
	if !bytes.Equal(first.body, second.body) {
		t.Fatalf("retry body changed between attempts: first=%s second=%s", first.body, second.body)
	}
	if first.eventID != second.eventID || first.eventID != terminalEvent.Id {
		t.Fatalf("retry event IDs = %q, %q, terminal=%q", first.eventID, second.eventID, terminalEvent.Id)
	}
	if first.headers.Get(webhooks.TimestampHeader) == second.headers.Get(webhooks.TimestampHeader) {
		t.Fatalf("retry timestamps did not advance: %q", first.headers.Get(webhooks.TimestampHeader))
	}
	assertWebhookSignature(t, first, functionalWebhookSecret)
	assertWebhookSignature(t, second, functionalWebhookSecret)
	attemptsMu.Lock()
	gotAttempts := attempts[first.eventID]
	attemptsMu.Unlock()
	if gotAttempts != 2 {
		t.Fatalf("retry attempts for event %q = %d, want 2", first.eventID, gotAttempts)
	}
}

func testFunctionalWebhookRetryExhaustion(t *testing.T) {
	fakeClock := clockwork.NewFakeClockAt(time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC))
	var attemptsMu sync.Mutex
	attempts := make(map[string]int)
	var failingEventID string
	deadLetterStore := newFunctionalDeadLetterStore()
	receiver := newFunctionalWebhookReceiver(t, func(request functionalWebhookRequest) functionalWebhookResponse {
		attemptsMu.Lock()
		if failingEventID == "" {
			failingEventID = request.eventID
		}
		attempts[request.eventID]++
		isFailing := request.eventID == failingEventID
		attemptsMu.Unlock()
		if isFailing {
			return functionalWebhookResponse{status: http.StatusServiceUnavailable, body: "receiver response must not be retained"}
		}
		return functionalWebhookResponse{status: http.StatusOK}
	})
	dir := scaffoldWebhookFactory(t, []map[string]any{
		functionalWebhookWithPolicy(
			"exhaust", true, receiver.URL()+"/exhaust", "secrets/exhaust", []string{"WORK_STATE_CHANGE"}, nil,
			map[string]any{"maxAttempts": 3, "initialBackoff": "1s", "backoffMultiplier": 2.0, "maxBackoff": "2s", "requestTimeout": "1s"},
		),
	})
	resolver := newFunctionalWebhookSecretResolver()
	server := startWebhookFunctionalServer(t, dir, serviceedges.Edges{
		Clock: fakeClock, FactoryWebhookSecretResolver: resolver.resolve, FactoryWebhookDeadLetterAppender: deadLetterStore.append,
	})
	resolver.waitForCount(t, 1, 5*time.Second)

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	workID := submitFunctionalWebhookWork(t, process, server.URL(), "retry-exhaustion")
	moveFunctionalWebhookWork(t, process, server.URL(), workID, "complete")
	first := receiver.waitForWorkEvent(t, workID, 5*time.Second)
	for _, delay := range []time.Duration{time.Second, 2 * time.Second} {
		fakeClock.BlockUntil(1)
		fakeClock.Advance(delay)
		receiver.waitForEvent(t, first.eventID, 5*time.Second)
	}
	terminalEvent := waitForTerminalWorkEvent(t, stream, workID, 15*time.Second)
	record := deadLetterStore.wait(t, 5*time.Second)

	var deadLetter struct {
		EndpointName      string          `json:"endpointName"`
		DestinationOrigin string          `json:"destinationOrigin"`
		DestinationPath   string          `json:"destinationPath"`
		EventID           string          `json:"eventId"`
		EventType         string          `json:"eventType"`
		AttemptCount      int             `json:"attemptCount"`
		TerminalReason    string          `json:"terminalReason"`
		StatusCode        int             `json:"statusCode"`
		CanonicalBody     json.RawMessage `json:"canonicalBody"`
	}
	if err := json.Unmarshal(record.line, &deadLetter); err != nil {
		t.Fatalf("decode dead-letter line: %v: %s", err, record.line)
	}
	if record.path != filepath.Join(dir, filepath.FromSlash(webhooks.DeadLetterRelativePath)) {
		t.Fatalf("dead-letter path = %q, want session path below %q", record.path, dir)
	}
	if deadLetter.EndpointName != "exhaust" || deadLetter.EventID != first.eventID || deadLetter.EventID != terminalEvent.Id {
		t.Fatalf("dead-letter identity = %#v, want exhaust and event %q", deadLetter, first.eventID)
	}
	if deadLetter.EventType != string(factoryapi.FactoryEventTypeWorkStateChange) || deadLetter.AttemptCount != 3 || deadLetter.StatusCode != http.StatusServiceUnavailable || deadLetter.TerminalReason != "retry_exhausted" {
		t.Fatalf("dead-letter delivery details = %#v", deadLetter)
	}
	if !bytes.Equal(deadLetter.CanonicalBody, first.body) {
		t.Fatalf("dead-letter canonical body differs from request: got=%s want=%s", deadLetter.CanonicalBody, first.body)
	}
	if bytes.Contains(record.line, []byte(functionalWebhookSecret)) || bytes.Contains(record.line, []byte("receiver response must not be retained")) {
		t.Fatalf("dead-letter retained secret or receiver response: %s", record.line)
	}
	if got := deadLetterStore.count(); got != 1 {
		t.Fatalf("dead-letter append count = %d, want 1", got)
	}
}

type functionalWebhookRequest struct {
	path    string
	eventID string
	body    []byte
	headers http.Header
}

type functionalWebhookResponse struct {
	status int
	body   string
}

type functionalWebhookReceiver struct {
	server   *httptest.Server
	requests chan functionalWebhookRequest

	mu  sync.Mutex
	all []functionalWebhookRequest
}

func (receiver *functionalWebhookReceiver) URL() string {
	return receiver.server.URL
}

func newFunctionalWebhookReceiver(
	t *testing.T,
	handler func(functionalWebhookRequest) functionalWebhookResponse,
) *functionalWebhookReceiver {
	t.Helper()
	receiver := &functionalWebhookReceiver{requests: make(chan functionalWebhookRequest, 64)}
	receiver.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		observed := functionalWebhookRequest{
			path:    request.URL.Path,
			eventID: request.Header.Get(webhooks.EventIDHeader),
			body:    append([]byte(nil), body...),
			headers: request.Header.Clone(),
		}
		response := handler(observed)
		if response.status == 0 {
			response.status = http.StatusOK
		}
		writer.WriteHeader(response.status)
		_, _ = io.WriteString(writer, response.body)
		receiver.mu.Lock()
		receiver.all = append(receiver.all, observed)
		receiver.mu.Unlock()
		receiver.requests <- observed
	}))
	t.Cleanup(receiver.server.Close)
	return receiver
}

func (receiver *functionalWebhookReceiver) waitForEvent(
	t *testing.T,
	eventID string,
	timeout time.Duration,
) functionalWebhookRequest {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case request := <-receiver.requests:
			if request.eventID == eventID {
				return request
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for webhook event %q", eventID)
		}
	}
}

func (receiver *functionalWebhookReceiver) waitForWorkEvent(
	t *testing.T,
	workID string,
	timeout time.Duration,
) functionalWebhookRequest {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case request := <-receiver.requests:
			var event factoryapi.FactoryEvent
			if err := json.Unmarshal(request.body, &event); err != nil {
				t.Fatalf("decode webhook event: %v", err)
			}
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err == nil && payload.WorkId == workID {
				return request
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for webhook Work event for %q", workID)
		}
	}
}

func (receiver *functionalWebhookReceiver) requestsSnapshot() []functionalWebhookRequest {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return append([]functionalWebhookRequest(nil), receiver.all...)
}

type functionalWebhookSecretResolver struct {
	mu       sync.Mutex
	resolved []string
	ready    chan struct{}
}

func newFunctionalWebhookSecretResolver() *functionalWebhookSecretResolver {
	return &functionalWebhookSecretResolver{ready: make(chan struct{}, 16)}
}

func (resolver *functionalWebhookSecretResolver) resolve(
	_ context.Context,
	_ factorydefinitions.LoadedFactorySource,
	ref string,
) (string, error) {
	resolver.mu.Lock()
	resolver.resolved = append(resolver.resolved, ref)
	resolver.mu.Unlock()
	resolver.ready <- struct{}{}
	return functionalWebhookSecret, nil
}

func (resolver *functionalWebhookSecretResolver) waitForCount(t *testing.T, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for resolved := 0; resolved < count; resolved++ {
		select {
		case <-resolver.ready:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d webhook secret resolutions", count)
		}
	}
}

func (resolver *functionalWebhookSecretResolver) assertResolved(t *testing.T, refs ...string) {
	t.Helper()
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	for _, ref := range refs {
		found := false
		for _, resolved := range resolver.resolved {
			if resolved == ref {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("secret reference %q was not resolved; resolved=%v", ref, resolver.resolved)
		}
	}
}

func (resolver *functionalWebhookSecretResolver) assertNotResolved(t *testing.T, ref string) {
	t.Helper()
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	for _, resolved := range resolver.resolved {
		if resolved == ref {
			t.Fatalf("disabled secret reference %q was resolved; resolved=%v", ref, resolver.resolved)
		}
	}
}

type functionalDeadLetterRecord struct {
	path string
	line []byte
}

type functionalDeadLetterStore struct {
	mu      sync.Mutex
	records []functionalDeadLetterRecord
	ready   chan functionalDeadLetterRecord
}

func newFunctionalDeadLetterStore() *functionalDeadLetterStore {
	return &functionalDeadLetterStore{ready: make(chan functionalDeadLetterRecord, 8)}
}

func (store *functionalDeadLetterStore) append(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	record := functionalDeadLetterRecord{path: path, line: append([]byte(nil), line...)}
	store.mu.Lock()
	store.records = append(store.records, record)
	store.mu.Unlock()
	store.ready <- record
	return nil
}

func (store *functionalDeadLetterStore) wait(t *testing.T, timeout time.Duration) functionalDeadLetterRecord {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case record := <-store.ready:
		return record
	case <-timer.C:
		t.Fatal("timed out waiting for dead-letter append")
		return functionalDeadLetterRecord{}
	}
}

func (store *functionalDeadLetterStore) count() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.records)
}

func startWebhookFunctionalServer(
	t *testing.T,
	dir string,
	edges serviceedges.Edges,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            false,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
}

func submitFunctionalWebhookWork(t *testing.T, process support.Process, baseURL, name string) string {
	t.Helper()
	payloadPath := filepath.Join(t.TempDir(), "webhook-request.md")
	if err := os.WriteFile(payloadPath, []byte("# functional webhook delivery\n"), 0o600); err != nil {
		t.Fatalf("write functional webhook payload: %v", err)
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", baseURL, "--json", "submit",
		"--session", factorysessions.DefaultSessionID,
		"--name", name,
		"--work-type-name", "task",
		"--payload", payloadPath,
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(submit) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response struct {
		WorkID *string `json:"workId"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode CLI submit response: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	if response.WorkID == nil || strings.TrimSpace(*response.WorkID) == "" {
		t.Fatalf("CLI submit response missing workId: %s", inputs.Stdout())
	}
	return *response.WorkID
}

func moveFunctionalWebhookWork(t *testing.T, process support.Process, baseURL, workID, stateName string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", baseURL, "--json", "work", "move",
		workID, stateName, "--session", factorysessions.DefaultSessionID,
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(work move %s) error = %v\nstdout:\n%s\nstderr:\n%s", stateName, err, inputs.Stdout(), inputs.Stderr())
	}
}

func scaffoldWebhookFactory(t *testing.T, webhooksConfig []map[string]any) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "functional-webhooks",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"webhooks": webhooksConfig,
	})
}

func functionalWebhook(name string, enabled bool, url, secretRef string, eventTypes, dispatchStatuses []string) map[string]any {
	config := map[string]any{
		"name":             name,
		"enabled":          enabled,
		"url":              url,
		"signingSecretRef": secretRef,
		"filter": map[string]any{
			"eventTypes": eventTypes,
		},
	}
	if dispatchStatuses != nil {
		config["filter"].(map[string]any)["dispatchStatuses"] = dispatchStatuses
	}
	return config
}

func functionalWebhookWithPolicy(
	name string,
	enabled bool,
	url, secretRef string,
	eventTypes, dispatchStatuses []string,
	policy map[string]any,
) map[string]any {
	config := functionalWebhook(name, enabled, url, secretRef, eventTypes, dispatchStatuses)
	config["deliveryPolicy"] = policy
	return config
}

func waitForTerminalWorkEvent(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for terminal WORK_STATE_CHANGE for %q", workID)
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			t.Fatalf("Factory Event stream closed before terminal Work event for %q", workID)
		}
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil || payload.WorkId != workID || !strings.EqualFold(payload.ToState, "complete") {
			continue
		}
		return event
	}
}

func assertSignedCanonicalWebhook(
	t *testing.T,
	request functionalWebhookRequest,
	event factoryapi.FactoryEvent,
	secret string,
) {
	t.Helper()
	if request.eventID != event.Id {
		t.Fatalf("event ID header = %q, want %q", request.eventID, event.Id)
	}
	if request.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", request.headers.Get("Content-Type"))
	}
	var received factoryapi.FactoryEvent
	if err := json.Unmarshal(request.body, &received); err != nil {
		t.Fatalf("decode canonical webhook body: %v", err)
	}
	if received.Id != event.Id || received.Type != event.Type || received.Context.Sequence != event.Context.Sequence {
		t.Fatalf("webhook envelope = %#v, want canonical event %#v", received, event)
	}
	canonicalBody, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal canonical event: %v", err)
	}
	if !bytes.Equal(request.body, canonicalBody) {
		t.Fatalf("webhook body differs from canonical event: got=%s want=%s", request.body, canonicalBody)
	}
	assertWebhookSignature(t, request, secret)
}

func assertWebhookSignature(t *testing.T, request functionalWebhookRequest, secret string) {
	t.Helper()
	timestamp := request.headers.Get(webhooks.TimestampHeader)
	if _, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		t.Fatalf("timestamp header = %q: %v", timestamp, err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(request.body)
	want := webhooks.SignatureVersionV1 + "=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(request.headers.Get(webhooks.SignatureHeader)), []byte(want)) {
		t.Fatalf("signature = %q, want independently verified %q", request.headers.Get(webhooks.SignatureHeader), want)
	}
}

var _ webhooks.DeadLetterAppender = (*functionalDeadLetterStore)(nil).append
