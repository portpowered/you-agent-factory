package cli_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkerSessionsStreamAbortReturnsTypedDiagnosticThroughRootProcess(t *testing.T) {
	const workerSessionID = "worker-session-root-abort"
	streamServer := newAbortedWorkerSessionStreamServer(t, workerSessionID)
	defer streamServer.Close()

	fixture := workerSessionsCLIProcess(t)
	inputs := support.FakeInputs(t.Context(), workerSessionAbortArgs(streamServer.URL, "provider-session-root-abort"))
	inputs.Input.Env = functionalEnvironment(fixture.homeDir)
	inputs.Input.WorkingDirectory = fixture.hostFactory

	err := fixture.process.Execute(inputs.Input)
	var typed *workersessionscli.CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_CLOSED" {
		t.Fatalf("Process.Execute() error = %v, want typed WORKER_SESSION_STREAM_CLOSED", err)
	}
	assertWorkerSessionStreamAbortOutput(t, inputs.Stdout(), inputs.Stderr(), workerSessionID)
	assertNoActiveProviderCommandRoutes(t, fixture.runner, "root-process abrupt stream close")
}

func newAbortedWorkerSessionStreamServer(t *testing.T, workerSessionID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("aborted stream response writer does not support flushing")
			return
		}
		_, _ = fmt.Fprintf(w, "data: {\"delivery\":\"RECORD\",\"workerSessionId\":%q,\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker_session\",\"sourceId\":%q,\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker_session.started\",\"payload\":{\"state\":\"RUNNING\"}},\"errorCode\":null,\"errorMessage\":null}\n\n", workerSessionID, workerSessionID)
		flusher.Flush()
	}))
}

func workerSessionAbortArgs(serverURL, providerSessionID string) []string {
	return []string{
		"you", "--server", serverURL, "worker-sessions", "stream",
		"--provider", "codex", "--kind", "session_id", "--id", providerSessionID, "--output", "json",
	}
}

func assertWorkerSessionStreamAbortOutput(t *testing.T, stdout, stderr, workerSessionID string) {
	t.Helper()
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 {
		t.Fatalf("aborted stream stdout lines = %d, want one retained frame:\n%s", len(lines), stdout)
	}
	var frame streamFrameJSON
	if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil {
		t.Fatalf("decode retained Worker Session frame: %v\nstdout:%s", err, stdout)
	}
	if frame.Delivery != "RECORD" || frame.Event == nil || frame.Event.Position != 1 || frame.WorkerSessionID != workerSessionID {
		t.Fatalf("retained stream frame = %#v, want one RECORD at position 1", frame)
	}
	if strings.Contains(stdout, "TERMINAL") || strings.Contains(stdout, "REPLAY_SUMMARY") {
		t.Fatalf("aborted stream synthesized a terminal/success frame:\n%s", stdout)
	}

	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &diagnostic); err != nil {
		t.Fatalf("decode one Worker Session stream diagnostic: %v\nstderr:%s", err, stderr)
	}
	if diagnostic.Code != "WORKER_SESSION_STREAM_CLOSED" || strings.TrimSpace(diagnostic.Message) == "" {
		t.Fatalf("stream diagnostic = %#v, want one coded safe diagnostic", diagnostic)
	}
	if strings.Count(stderr, "WORKER_SESSION_STREAM_CLOSED") != 1 {
		t.Fatalf("stream diagnostic code occurrences = %d, want exactly one:\n%s", strings.Count(stderr, "WORKER_SESSION_STREAM_CLOSED"), stderr)
	}
}

type workerSessionReplaySummaryJSON struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	Reason        string `json:"reason"`
	EventsEmitted int64  `json:"eventsEmitted"`
}
type streamFrameJSON struct {
	Delivery         string                          `json:"delivery"`
	ErrorCode        *string                         `json:"errorCode"`
	ErrorMessage     *string                         `json:"errorMessage"`
	Event            *streamEventJSON                `json:"event"`
	FactorySessionID *string                         `json:"factorySessionId"`
	ProviderSession  *providerSessionJSON            `json:"providerSession"`
	ReplaySummary    *workerSessionReplaySummaryJSON `json:"replaySummary"`
	WorkIDs          []string                        `json:"workIds"`
	WorkerSessionID  string                          `json:"workerSessionId"`
}

type streamEventJSON struct {
	Position   uint64          `json:"position"`
	SourceType string          `json:"sourceType"`
	SchemaID   string          `json:"schemaId"`
	Payload    json.RawMessage `json:"payload"`
}

func assertCanonicalWorkerSessionHistory(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	workID string,
	wantTerminal string,
) {
	t.Helper()
	if len(frames) < 2 {
		t.Fatalf("canonical Worker Session history = %#v, want opening and terminal records", frames)
	}
	opening := frames[0]
	if opening.Event.SchemaId != "DISPATCH_REQUEST" {
		t.Fatalf("canonical opening schema = %q, want DISPATCH_REQUEST", opening.Event.SchemaId)
	}
	providerOutputSeen := false
	for index, frame := range frames {
		if frame.Event.SourceType != "factory_event" {
			t.Fatalf("canonical frame[%d] source type = %q, want factory_event", index, frame.Event.SourceType)
		}
		if frame.WorkerSessionId != opening.WorkerSessionId || !containsString(frame.WorkIds, workID) {
			t.Fatalf("canonical frame[%d] correlation = %#v, want worker %s and Work %s", index, frame, opening.WorkerSessionId, workID)
		}
		if index > 0 && frame.Event.Position <= frames[index-1].Event.Position {
			t.Fatalf("canonical positions are not increasing: frame[%d]=%d previous=%d", index, frame.Event.Position, frames[index-1].Event.Position)
		}
		if frame.Event.SchemaId == "MODEL_RESPONSE" {
			providerOutputSeen = true
		}
	}
	if !providerOutputSeen {
		t.Fatalf("canonical Worker Session history has no provider response: %#v", frames)
	}
	terminal := frames[len(frames)-1]
	if terminal.Event.SchemaId != "DISPATCH_RESPONSE" || terminal.Delivery != "TERMINAL_REPLAY" {
		t.Fatalf("canonical terminal = %#v, want DISPATCH_RESPONSE TERMINAL_REPLAY", terminal)
	}
	payload := stringValue(terminal.Event.Payload, "outcome")
	if wantTerminal == "COMPLETED" && payload != "ACCEPTED" {
		t.Fatalf("canonical terminal outcome = %q, want ACCEPTED", payload)
	}
	if wantTerminal == "FAILED" && payload == "ACCEPTED" {
		t.Fatalf("canonical terminal outcome = %q, want failed outcome", payload)
	}
}
func assertCanonicalWorkerSessionStream(t *testing.T, frames []streamFrameJSON, providerID, terminalState string) {
	t.Helper()
	if len(frames) < 2 {
		t.Fatalf("canonical stream frames = %#v, want opening and terminal", frames)
	}
	opening := frames[0]
	if opening.Event == nil || opening.Event.SchemaID != "DISPATCH_REQUEST" {
		t.Fatalf("canonical stream opening = %#v, want DISPATCH_REQUEST", opening)
	}
	providerResponse := false
	for index, frame := range frames {
		if frame.Event == nil || frame.Event.SourceType != "factory_event" {
			t.Fatalf("canonical stream frame[%d] = %#v, want factory event", index, frame)
		}
		if frame.ProviderSession == nil || frame.ProviderSession.ID != providerID {
			t.Fatalf("canonical stream frame[%d] provider session = %#v, want %s", index, frame.ProviderSession, providerID)
		}
		if index > 0 && frame.Event.Position <= frames[index-1].Event.Position {
			t.Fatalf("canonical stream positions are not increasing: frame[%d]=%d previous=%d", index, frame.Event.Position, frames[index-1].Event.Position)
		}
		providerResponse = providerResponse || frame.Event.SchemaID == "MODEL_RESPONSE"
	}
	if !providerResponse {
		t.Fatalf("canonical stream omitted MODEL_RESPONSE: %#v", frames)
	}
	terminal := frames[len(frames)-1]
	if terminal.Delivery != "TERMINAL_REPLAY" || terminal.Event.SchemaID != "DISPATCH_RESPONSE" {
		t.Fatalf("canonical stream terminal = %#v, want DISPATCH_RESPONSE TERMINAL_REPLAY", terminal)
	}
	var payload struct {
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(terminal.Event.Payload, &payload); err != nil {
		t.Fatalf("decode canonical stream terminal: %v", err)
	}
	if terminalState == "COMPLETED" && payload.Outcome != "ACCEPTED" {
		t.Fatalf("canonical stream terminal outcome = %q, want ACCEPTED", payload.Outcome)
	}
	if terminalState == "FAILED" && payload.Outcome == "ACCEPTED" {
		t.Fatalf("canonical stream terminal outcome = %q, want failed outcome", payload.Outcome)
	}
}
