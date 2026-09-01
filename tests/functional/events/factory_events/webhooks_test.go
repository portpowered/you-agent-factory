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
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
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
	t.Run("transition filtering and signature", parallelFunctionalWebhookCase(testFunctionalWebhookTransition))
	t.Run("failed dispatch filtering and signature", parallelFunctionalWebhookCase(testFunctionalWebhookDispatchFailure))
	t.Run("retry then success", parallelFunctionalWebhookCase(testFunctionalWebhookRetrySuccess))
	t.Run("retry exhaustion writes one redacted dead letter", parallelFunctionalWebhookCase(testFunctionalWebhookRetryExhaustion))
}

func parallelFunctionalWebhookCase(run func(*testing.T)) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		run(t)
	}
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
	process := factoryEventsCLIProcess
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
		FactoryWebhookClock: fakeClock, FactoryWebhookSecretResolver: resolver.resolve,
	})
	resolver.waitForCount(t, 1, 5*time.Second)

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := factoryEventsCLIProcess
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

func testFunctionalWebhookDispatchFailure(t *testing.T) {
	receiver := newFunctionalWebhookReceiver(t, func(request functionalWebhookRequest) functionalWebhookResponse {
		return functionalWebhookResponse{status: http.StatusOK}
	})
	resolver := newFunctionalWebhookSecretResolver()
	dir := scaffoldWebhookDispatchFailureFactory(t, []map[string]any{
		functionalWebhook("failed-dispatch", true, receiver.URL()+"/failed-dispatch", "secrets/failed-dispatch", []string{"DISPATCH_RESPONSE"}, []string{"FAILED"}),
	})
	runner := &functionalWebhookFailureCommandRunner{activated: resolver.activated}
	server := startWebhookFunctionalServer(t, dir, serviceedges.Edges{
		FactoryWebhookSecretResolver: resolver.resolve,
		ProviderCommandRunner:        runner,
	})
	resolver.waitForCount(t, 1, 5*time.Second)
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := factoryEventsCLIProcess
	submitFunctionalWebhookWork(t, process, server.URL(), "dispatch-failure")
	failedDispatch := waitForDispatchResponseEvent(t, stream, 15*time.Second)
	request := receiver.waitForEvent(t, failedDispatch.Id, 5*time.Second)
	assertSignedCanonicalWebhook(t, request, failedDispatch, functionalWebhookSecret)
	if request.path != "/failed-dispatch" {
		t.Fatalf("failed dispatch webhook path = %q, want /failed-dispatch", request.path)
	}
}

func testFunctionalWebhookRetryExhaustion(t *testing.T) {
	fakeClock := clockwork.NewFakeClockAt(time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC))
	var attemptsMu sync.Mutex
	attempts := make(map[string]int)
	var failingEventID string
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
		FactoryWebhookClock: fakeClock, FactoryWebhookSecretResolver: resolver.resolve,
	})
	resolver.waitForCount(t, 1, 5*time.Second)

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	process := factoryEventsCLIProcess
	workID := submitFunctionalWebhookWork(t, process, server.URL(), "retry-exhaustion")
	moveFunctionalWebhookWork(t, process, server.URL(), workID, "complete")
	first := receiver.waitForWorkEvent(t, workID, 5*time.Second)
	for _, delay := range []time.Duration{time.Second, 2 * time.Second} {
		fakeClock.BlockUntil(1)
		fakeClock.Advance(delay)
		receiver.waitForEvent(t, first.eventID, 5*time.Second)
	}
	terminalEvent := waitForTerminalWorkEvent(t, stream, workID, 15*time.Second)

	// The default appender is synchronous within the endpoint's event loop. A
	// subsequent successful event therefore gives this test a deterministic
	// happens-after point without polling the filesystem.
	followupWorkID := submitFunctionalWebhookWork(t, process, server.URL(), "retry-exhaustion-follow-up")
	moveFunctionalWebhookWork(t, process, server.URL(), followupWorkID, "complete")
	receiver.waitForWorkEvent(t, followupWorkID, 5*time.Second)

	deadLetterPath := filepath.Join(dir, filepath.FromSlash(webhooks.DeadLetterRelativePath))
	recordLine, err := os.ReadFile(deadLetterPath)
	if err != nil {
		t.Fatalf("read default dead-letter file: %v", err)
	}
	if got := bytes.Count(recordLine, []byte{'\n'}); got != 1 || !bytes.HasSuffix(recordLine, []byte{'\n'}) {
		t.Fatalf("dead-letter record count = %d, want one newline-terminated record: %s", got, recordLine)
	}
	recordLine = bytes.TrimSuffix(recordLine, []byte{'\n'})

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
	if err := json.Unmarshal(recordLine, &deadLetter); err != nil {
		t.Fatalf("decode dead-letter line: %v: %s", err, recordLine)
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
	if bytes.Contains(recordLine, []byte(functionalWebhookSecret)) || bytes.Contains(recordLine, []byte("receiver response must not be retained")) {
		t.Fatalf("dead-letter retained secret or receiver response: %s", recordLine)
	}
	assertFunctionalDurableAppenderFailurePaths(t)
}

func assertFunctionalDurableAppenderFailurePaths(t *testing.T) {
	t.Helper()
	local := platformfilesystem.Local{}
	if err := local.AppendDurable(" ", []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(blank path) succeeded, want validation error")
	}

	blockedParent := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	if err := local.AppendDurable(filepath.Join(blockedParent, "child.jsonl"), []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(blocked parent) succeeded, want directory error")
	}

	directoryTarget := filepath.Join(t.TempDir(), "directory-target")
	if err := os.MkdirAll(directoryTarget, 0o700); err != nil {
		t.Fatalf("create directory target: %v", err)
	}
	if err := local.AppendDurable(directoryTarget, []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(directory target) succeeded, want open error")
	}

	if runtime.GOOS != "linux" {
		return
	}
	if err := local.AppendDurable("/dev/full", []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(/dev/full) succeeded, want write error")
	}
	if err := local.AppendDurable(os.DevNull, []byte("ignored")); err == nil {
		t.Fatal("AppendDurable(/dev/null) succeeded, want sync error")
	}
}

// TestFunctionalLocalFilesystemPublishesReplacementAndTemporaryFiles proves local filesystem observations include replacement and temporary files.
func TestFunctionalLocalFilesystemPublishesReplacementAndTemporaryFiles(t *testing.T) {
	t.Parallel()

	local := platformfilesystem.Local{AllowRenameReplacement: true}
	dir := t.TempDir()
	workingDirectory, err := local.Getwd()
	if err != nil || strings.TrimSpace(workingDirectory) == "" {
		t.Fatalf("Getwd() = %q, %v; want a non-empty working directory", workingDirectory, err)
	}
	absolutePath, err := local.Abs(filepath.Join(dir, "artifact.json"))
	if err != nil || !filepath.IsAbs(absolutePath) {
		t.Fatalf("Abs(artifact path) = %q, %v; want an absolute path", absolutePath, err)
	}

	temporary, err := local.CreateTemp(dir, "functional-artifact-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if temporary.Name() == "" {
		t.Fatal("CreateTemp() returned a temporary file without a name")
	}
	if _, err := temporary.WriteString("temporary artifact"); err != nil {
		t.Fatalf("temporary WriteString() error = %v", err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatalf("temporary Close() error = %v", err)
	}
	if err := local.Remove(temporary.Name()); err != nil {
		t.Fatalf("Remove(temporary artifact) error = %v", err)
	}
	createdPath := filepath.Join(dir, "created-artifact.json")
	created, err := local.Create(createdPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := created.Write([]byte("created artifact")); err != nil {
		t.Fatalf("Create().Write() error = %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("Create().Close() error = %v", err)
	}
	opened, err := local.Open(createdPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	openedContents, err := io.ReadAll(opened)
	if err != nil {
		t.Fatalf("Open().Read() error = %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("Open().Close() error = %v", err)
	}
	if got, want := string(openedContents), "created artifact"; got != want {
		t.Fatalf("opened contents = %q, want %q", got, want)
	}
	if err := local.AppendDurable(os.DevNull, []byte("discarded artifact\n")); err == nil {
		t.Fatal("AppendDurable(os.DevNull) succeeded, want durable-flush error")
	}

	sourcePath := filepath.Join(dir, "next-recording.json")
	destinationPath := filepath.Join(dir, "recording.json")
	if err := local.WriteFile(sourcePath, []byte("new recording"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	if _, err := local.Readlink(sourcePath); err == nil {
		t.Fatal("Readlink(regular file) succeeded, want a non-symlink error")
	}
	if err := local.WriteFile(destinationPath, []byte("old recording"), 0o600); err != nil {
		t.Fatalf("WriteFile(destination) error = %v", err)
	}
	matches, err := local.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("Glob() matches = %#v, want the three JSON artifacts", matches)
	}
	if err := local.RenameReplacing(sourcePath, destinationPath); err != nil {
		t.Fatalf("RenameReplacing() error = %v", err)
	}
	contents, err := local.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("ReadFile(replaced destination) error = %v", err)
	}
	if got, want := string(contents), "new recording"; got != want {
		t.Fatalf("replaced destination contents = %q, want %q", got, want)
	}
	if err := local.RenameReplacing(filepath.Join(dir, "missing-recording.json"), destinationPath); err == nil {
		t.Fatal("RenameReplacing(missing source) succeeded, want an error")
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
	mu            sync.Mutex
	resolved      []string
	ready         chan struct{}
	activated     chan struct{}
	activatedOnce sync.Once
}

func newFunctionalWebhookSecretResolver() *functionalWebhookSecretResolver {
	return &functionalWebhookSecretResolver{
		ready:     make(chan struct{}, 16),
		activated: make(chan struct{}),
	}
}

func (resolver *functionalWebhookSecretResolver) resolve(
	_ context.Context,
	_ factorydefinitions.LoadedFactorySource,
	ref string,
) (string, error) {
	resolver.activatedOnce.Do(func() { close(resolver.activated) })
	resolver.mu.Lock()
	resolver.resolved = append(resolver.resolved, ref)
	resolver.mu.Unlock()
	resolver.ready <- struct{}{}
	return functionalWebhookSecret, nil
}

type functionalWebhookFailureCommandRunner struct {
	activated <-chan struct{}
}

func (runner *functionalWebhookFailureCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	select {
	case <-runner.activated:
		return platformprocess.CommandResult{
			ExitCode: 7,
			Stderr:   []byte("functional provider failure"),
		}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
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

func scaffoldWebhookDispatchFailureFactory(t *testing.T, webhooksConfig []map[string]any) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "functional-webhook-dispatch-failure",
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "worker-a"}},
		"workstations": []any{map[string]any{
			"name":      "process",
			"type":      "MODEL_WORKSTATION",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
		"webhooks": webhooksConfig,
	})
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
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

func waitForDispatchResponseEvent(
	t *testing.T,
	stream *support.FactoryEventStream,
	timeout time.Duration,
) factoryapi.FactoryEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatal("timed out waiting for failed DISPATCH_RESPONSE Factory Event")
		}
		event, ok := stream.TryNextEvent(remaining)
		if !ok {
			t.Fatal("Factory Event stream closed before failed DISPATCH_RESPONSE")
		}
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		if string(payload.Outcome) != "FAILED" {
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
