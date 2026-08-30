package http_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestWorkerSessionHTTPDisconnectKeepsAdmittedWorkerAlive(t *testing.T) {
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := startDirectWorkerSessionServer(t, runner)

	connection := openDirectWorkerSessionConnection(t, server.URL(), "disconnect-request", "disconnect-session", "disconnect-dispatch")
	t.Cleanup(func() { _ = connection.Close() })
	runner.waitStarted(t)
	// Keep the submitting HTTP request unread and close its actual TCP
	// connection after the deterministic Workers admission edge. This proves
	// the server-owned execution survives a client disconnect before the
	// caller receives or reads the 202 response.
	if err := connection.Close(); err != nil {
		t.Fatalf("close submitting HTTP connection: %v", err)
	}
	if runner.wasCanceled() {
		t.Fatal("admitted worker was canceled when the submitting context closed")
	}

	eventResponse, eventCancel := openWorkerSessionEventStream(t, server.URL(), "disconnect-session")
	defer eventCancel()
	defer eventResponse.Body.Close()
	eventsResult := make(chan workerSessionEventsResult, 1)
	go func() {
		frames, err := readWorkerSessionEventStream(eventResponse)
		eventsResult <- workerSessionEventsResult{frames: frames, err: err}
	}()

	close(gate)
	runner.waitCompleted(t)
	if runner.wasCanceled() {
		t.Fatal("admitted worker was canceled before its gated completion")
	}
	events := <-eventsResult
	if events.err != nil {
		t.Fatalf("read Worker Session event stream: %v", events.err)
	}
	assertCompletedWorkerSessionEvents(t, events.frames, "disconnect-session")
	replay := postDirectWorkerSession(t, context.Background(), server.URL(), "disconnect-request", "disconnect-session", "disconnect-dispatch")
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusAccepted {
		t.Fatalf("same-key replay status = %d, want 202", replay.StatusCode)
	}
	var replayPayload factoryapi.WorkerSessionStartResponse
	if err := json.NewDecoder(replay.Body).Decode(&replayPayload); err != nil {
		t.Fatalf("decode same-key replay: %v", err)
	}
	if replayPayload.WorkerSessionId != "disconnect-session" || replayPayload.RequestId != "disconnect-request" {
		t.Fatalf("same-key replay = %#v, want original accepted identity", replayPayload)
	}
	if runner.callCount() != 1 {
		t.Fatalf("worker command calls = %d, want one after disconnect and replay", runner.callCount())
	}
	functionalevidence.Covers(t, "rest/startWorkerSession")
}

func TestWorkerSessionHTTPShutdownJoinsAdmittedWorker(t *testing.T) {
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := startDirectWorkerSessionServer(t, runner)

	response := postDirectWorkerSession(t, context.Background(), server.URL(), "shutdown-request", "shutdown-session", "shutdown-dispatch")
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /worker-sessions status = %d, want 202", response.StatusCode)
	}
	runner.waitStarted(t)

	server.Stop(t)
	if !runner.wasCanceled() {
		t.Fatal("server shutdown did not cancel the admitted worker")
	}
	if runner.callCount() != 1 {
		t.Fatalf("worker command calls = %d, want one during joined shutdown", runner.callCount())
	}
}

func TestWorkerSessionHTTPInterruptRejectsUnassociatedActiveSource(t *testing.T) {
	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := startDirectWorkerSessionServer(t, runner)

	start := postDirectWorkerSession(t, context.Background(), server.URL(), "interrupt-source-request", "interrupt-source", "interrupt-source-dispatch")
	start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /worker-sessions status = %d, want 202", start.StatusCode)
	}
	runner.waitStarted(t)

	payload := factoryapi.WorkerSessionInterruptRequest{
		RequestId:                "interrupt-request",
		SuccessorWorkerSessionId: "interrupt-successor",
		ReplacementMessage:       "replace the active work",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal interrupt request: %v", err)
	}
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost,
		server.URL()+"/worker-sessions/interrupt-source/interrupt",
		strings.NewReader(string(body)),
	)
	if err != nil {
		t.Fatalf("construct interrupt request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	interrupt, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST Worker Session interrupt: %v", err)
	}
	defer interrupt.Body.Close()
	if interrupt.StatusCode != http.StatusBadRequest {
		responseBody, _ := io.ReadAll(interrupt.Body)
		t.Fatalf("interrupt status = %d, want 400; body = %s", interrupt.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result factoryapi.WorkerSessionInterruptError
	if err := json.NewDecoder(interrupt.Body).Decode(&result); err != nil {
		t.Fatalf("decode interrupt error: %v", err)
	}
	if result.Code != string(factoryapi.ErrorResponseCodeBADREQUEST) ||
		result.Phase != factoryapi.WorkerSessionInterruptErrorPhaseValidation ||
		result.RequestId == nil || *result.RequestId != payload.RequestId ||
		result.SourceWorkerSessionId == nil || *result.SourceWorkerSessionId != "interrupt-source" ||
		result.SuccessorWorkerSessionId == nil || *result.SuccessorWorkerSessionId != "interrupt-successor" ||
		result.Source == nil || result.Source.State != factoryapi.WorkerSessionInterruptSnapshotStateRunning {
		t.Fatalf("interrupt error = %#v, want validation snapshot for unassociated active source", result)
	}
	if runner.callCount() != 1 || runner.wasCanceled() {
		t.Fatalf("provider command state after rejected interrupt = calls %d canceled=%t, want one active source", runner.callCount(), runner.wasCanceled())
	}
	functionalevidence.Covers(t, "rest/interruptWorkerSession")
}

// WSR-FT-013: root.BuildProcess/Process.Execute hosts a customer-facing
// Worker Session whose public history records request, outcome, and the
// resulting terminal consequence in aggregate order.
func TestWorkerSessionHTTPControlCancelConvergesTerminalSnapshot(t *testing.T) {
	runner := newFunctionalWorkerGate(make(chan struct{}))
	server := startDirectWorkerSessionServer(t, runner)

	start := postDirectWorkerSession(t, context.Background(), server.URL(), "control-request", "control-session", "control-dispatch")
	start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /worker-sessions status = %d, want 202", start.StatusCode)
	}
	runner.waitStarted(t)

	eventResponse, eventCancel := openWorkerSessionEventStream(t, server.URL(), "control-session")
	defer eventCancel()
	defer eventResponse.Body.Close()
	eventsResult := make(chan workerSessionTerminalEventsResult, 1)
	go func() {
		frames, phase, err := readWorkerSessionEventStreamUntilTerminal(eventResponse)
		eventsResult <- workerSessionTerminalEventsResult{frames: frames, phase: phase, err: err}
	}()

	cancel := postWorkerSessionControl(t, server.URL(), "control-session", "cancel")
	defer cancel.Body.Close()
	if cancel.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(cancel.Body)
		t.Fatalf("POST /worker-sessions/control-session/cancel status = %d, body = %s", cancel.StatusCode, strings.TrimSpace(string(body)))
	}
	var cancelResult factoryapi.WorkerSessionControlResponse
	if err := json.NewDecoder(cancel.Body).Decode(&cancelResult); err != nil {
		t.Fatalf("decode cancel result: %v", err)
	}
	if cancelResult.WorkerSessionId != "control-session" || cancelResult.Action != factoryapi.WorkerSessionControlResponseActionCancel ||
		cancelResult.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied || cancelResult.DispatchId != "control-dispatch" {
		t.Fatalf("cancel result = %#v, want applied exact dispatch control", cancelResult)
	}
	runner.waitCanceled(t)

	events := <-eventsResult
	if events.err != nil {
		t.Fatalf("read canceled Worker Session event stream: %v", events.err)
	}
	if events.phase != "CANCELED" {
		t.Fatalf("Worker Session terminal phase = %q, want CANCELED; frames=%#v", events.phase, events.frames)
	}
	assertSingleTerminalWorkerSessionEvent(t, events.frames, "control-session", "CANCELED")
	assertOrderedWorkerSessionControlBracket(t, events.frames, "control-session", "CANCELED")

	repeated := postWorkerSessionControl(t, server.URL(), "control-session", "cancel")
	defer repeated.Body.Close()
	if repeated.StatusCode != http.StatusOK {
		t.Fatalf("repeated cancel status = %d, want 200", repeated.StatusCode)
	}
	var repeatedResult factoryapi.WorkerSessionControlResponse
	if err := json.NewDecoder(repeated.Body).Decode(&repeatedResult); err != nil {
		t.Fatalf("decode repeated cancel result: %v", err)
	}
	if repeatedResult.Outcome != factoryapi.WorkerSessionControlResponseOutcomeNoop ||
		repeatedResult.State != factoryapi.WorkerSessionControlResponseStateCanceled ||
		repeatedResult.DispatchId != "control-dispatch" {
		t.Fatalf("repeated cancel result = %#v, want canonical canceled no-op", repeatedResult)
	}

	terminated := postWorkerSessionControl(t, server.URL(), "control-session", "terminate")
	defer terminated.Body.Close()
	if terminated.StatusCode != http.StatusOK {
		t.Fatalf("mixed terminate status = %d, want 200", terminated.StatusCode)
	}
	var terminateResult factoryapi.WorkerSessionControlResponse
	if err := json.NewDecoder(terminated.Body).Decode(&terminateResult); err != nil {
		t.Fatalf("decode mixed terminate result: %v", err)
	}
	if terminateResult.Outcome != factoryapi.WorkerSessionControlResponseOutcomeNoop ||
		terminateResult.State != factoryapi.WorkerSessionControlResponseStateCanceled ||
		terminateResult.DispatchId != "control-dispatch" {
		t.Fatalf("mixed terminate result = %#v, want canonical canceled no-op", terminateResult)
	}
	for _, action := range []string{"pause", "resume"} {
		control := postWorkerSessionControl(t, server.URL(), "control-session", action)
		defer control.Body.Close()
		if control.StatusCode != http.StatusOK {
			t.Fatalf("mixed %s status = %d, want 200", action, control.StatusCode)
		}
		var result factoryapi.WorkerSessionControlResponse
		if err := json.NewDecoder(control.Body).Decode(&result); err != nil {
			t.Fatalf("decode mixed %s result: %v", action, err)
		}
		if result.Outcome != factoryapi.WorkerSessionControlResponseOutcomeNoop ||
			result.State != factoryapi.WorkerSessionControlResponseStateCanceled || result.DispatchId != "control-dispatch" {
			t.Fatalf("mixed %s result = %#v, want canonical canceled no-op", action, result)
		}
	}
	if runner.callCount() != 1 {
		t.Fatalf("worker command calls after repeated/mixed controls = %d, want one", runner.callCount())
	}
	functionalevidence.Covers(t, "rest/cancelWorkerSession")
}

func assertOrderedWorkerSessionControlBracket(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	wantWorkerSessionID, wantTerminalPhase string,
) {
	t.Helper()
	var controls []factoryapi.WorkerSessionEvent
	var terminal *factoryapi.WorkerSessionEvent
	for index := range frames {
		frame := frames[index]
		if frame.WorkerSessionId != wantWorkerSessionID {
			t.Fatalf("Worker Session frame[%d] identity = %q, want %q", index, frame.WorkerSessionId, wantWorkerSessionID)
		}
		if frame.Event.SourceType == "worker_session_control" {
			controls = append(controls, frame)
		}
		if frame.Event.SourceType == "worker_session_lifecycle" && workerSessionEventPhase(frame) == wantTerminalPhase {
			terminal = &frame
		}
	}
	if len(controls) != 2 {
		t.Fatalf("Worker Session control records = %d, want request/outcome pair; frames=%#v", len(controls), frames)
	}
	if controls[0].Event.SourceEventId != "request" || controls[0].Event.SourceSequence != 1 ||
		controls[1].Event.SourceEventId != "outcome" || controls[1].Event.SourceSequence != 2 ||
		controls[0].Event.SourceId != controls[1].Event.SourceId {
		t.Fatalf("Worker Session control identities = %#v, want matching request sequence 1 before outcome sequence 2", controls)
	}
	requestPayload := workerSessionControlPayload(controls[0])
	outcomePayload := workerSessionControlPayload(controls[1])
	if requestPayload["recordType"] != "REQUEST" || requestPayload["action"] != "CANCEL" ||
		requestPayload["workerSessionId"] != wantWorkerSessionID ||
		requestPayload["dispatchId"] != "control-dispatch" ||
		requestPayload["attemptId"] != "control-dispatch" {
		t.Fatalf("Worker Session control request payload = %#v, want exact cancel correlation", requestPayload)
	}
	if outcomePayload["recordType"] != "OUTCOME" || outcomePayload["action"] != "CANCEL" ||
		outcomePayload["outcome"] != "APPLIED" || outcomePayload["state"] != "CANCELED" ||
		outcomePayload["correlationId"] != requestPayload["correlationId"] {
		t.Fatalf("Worker Session control outcome payload = %#v, want applied correlated cancel", outcomePayload)
	}
	if requestPayload["correlationId"] == "" || requestPayload["requestId"] == "" {
		t.Fatalf("Worker Session control request payload = %#v, want stable correlation and request IDs", requestPayload)
	}
	if terminal == nil || controls[1].Event.Position >= terminal.Event.Position {
		t.Fatalf("Worker Session control positions = request %d outcome %d terminal %#v, want bracket before terminal", controls[0].Event.Position, controls[1].Event.Position, terminal)
	}
}

func workerSessionControlPayload(frame factoryapi.WorkerSessionEvent) map[string]interface{} {
	if payload, ok := frame.Event.Payload["payload"].(map[string]interface{}); ok {
		return payload
	}
	return frame.Event.Payload
}

func startDirectWorkerSessionServer(t *testing.T, runner platformprocess.CommandRunner) *support.FunctionalAPIServer {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "direct-worker-lifecycle")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
}

func postDirectWorkerSession(
	t *testing.T,
	ctx context.Context,
	baseURL, requestID, sessionID, dispatchID string,
) *http.Response {
	t.Helper()
	payload := directWorkerSessionPayload(requestID, sessionID, dispatchID)
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal direct Worker Session request: %v", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/worker-sessions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("construct direct Worker Session request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /worker-sessions: %v", err)
	}
	return response
}

func postWorkerSessionControl(t *testing.T, baseURL, workerSessionID, action string) *http.Response {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/worker-sessions/" + url.PathEscape(workerSessionID) + "/" + action
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, nil)
	if err != nil {
		t.Fatalf("construct Worker Session %s request: %v", action, err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST Worker Session %s: %v", action, err)
	}
	return response
}

func openDirectWorkerSessionConnection(
	t *testing.T,
	baseURL, requestID, sessionID, dispatchID string,
) net.Conn {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		t.Fatalf("parse direct Worker Session server URL %q: %v", baseURL, err)
	}
	connection, err := (&net.Dialer{Timeout: functionalWorkerSignalTimeout}).DialContext(t.Context(), "tcp", parsed.Host)
	if err != nil {
		t.Fatalf("dial direct Worker Session server: %v", err)
	}
	payload, err := json.Marshal(directWorkerSessionPayload(requestID, sessionID, dispatchID))
	if err != nil {
		_ = connection.Close()
		t.Fatalf("marshal raw direct Worker Session request: %v", err)
	}
	request := fmt.Sprintf(
		"POST /worker-sessions HTTP/1.1\r\nHost: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		parsed.Host,
		len(payload),
		payload,
	)
	if _, err := io.WriteString(connection, request); err != nil {
		_ = connection.Close()
		t.Fatalf("write raw direct Worker Session request: %v", err)
	}
	return connection
}

func directWorkerSessionPayload(requestID, sessionID, dispatchID string) factoryapi.WorkerSessionStartRequest {
	return factoryapi.WorkerSessionStartRequest{
		RequestId:       requestID,
		WorkerSessionId: sessionID,
		Execution: factoryapi.WorkerSessionResolvedExecution{
			WorkstationName: "process",
			WorkerType:      functionalStringPtr("processor"),
			Dispatch: factoryapi.WorkerSessionResolvedDispatch{
				DispatchId:      dispatchID,
				WorkstationName: "process",
				WorkerType:      functionalStringPtr("processor"),
			},
		},
	}
}

func openWorkerSessionEventStream(t *testing.T, baseURL, workerSessionID string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), functionalWorkerSignalTimeout)
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/~default/worker-sessions/" + url.PathEscape(workerSessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatalf("construct Worker Session event request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("GET Worker Session events: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		cancel()
		t.Fatalf("GET Worker Session events status = %d, body = %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response, cancel
}

type workerSessionEventsResult struct {
	frames []factoryapi.WorkerSessionEvent
	err    error
}

func readWorkerSessionEventStream(response *http.Response) ([]factoryapi.WorkerSessionEvent, error) {
	var frames []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			return frames, fmt.Errorf("decode Worker Session event: %w", err)
		}
		frames = append(frames, frame)
		if frame.Event.SourceType == "worker_session_lifecycle" && workerSessionEventPhase(frame) == "COMPLETED" {
			return frames, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return frames, err
	}
	return frames, fmt.Errorf("Worker Session event stream ended before lifecycle terminal event")
}

type workerSessionTerminalEventsResult struct {
	frames []factoryapi.WorkerSessionEvent
	phase  string
	err    error
}

func readWorkerSessionEventStreamUntilTerminal(response *http.Response) ([]factoryapi.WorkerSessionEvent, string, error) {
	var frames []factoryapi.WorkerSessionEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var frame factoryapi.WorkerSessionEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
			return frames, "", fmt.Errorf("decode Worker Session terminal event: %w", err)
		}
		frames = append(frames, frame)
		if frame.Event.SourceType != "worker_session_lifecycle" {
			continue
		}
		phase := workerSessionEventPhase(frame)
		switch phase {
		case "COMPLETED", "FAILED", "CANCELED", "TERMINATED":
			return frames, phase, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return frames, "", err
	}
	return frames, "", fmt.Errorf("Worker Session event stream ended before lifecycle terminal event")
}

func functionalStringPtr(value string) *string { return &value }

type functionalWorkerGate struct {
	gate         <-chan struct{}
	started      chan struct{}
	callSignals  chan struct{}
	canceled     chan struct{}
	completed    chan struct{}
	mu           sync.Mutex
	calls        int
	canceledFlag bool
	startOnce    sync.Once
	canceledOnce sync.Once
	completeOnce sync.Once
}

func newFunctionalWorkerGate(gate <-chan struct{}) *functionalWorkerGate {
	return &functionalWorkerGate{
		gate:        gate,
		started:     make(chan struct{}),
		callSignals: make(chan struct{}, 64),
		canceled:    make(chan struct{}),
		completed:   make(chan struct{}),
	}
}

func (r *functionalWorkerGate) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.startOnce.Do(func() { close(r.started) })
	r.mu.Unlock()
	select {
	case r.callSignals <- struct{}{}:
	default:
	}
	select {
	case <-r.gate:
		r.completeOnce.Do(func() { close(r.completed) })
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("functional worker completed. COMPLETE")}, nil
	case <-ctx.Done():
		r.mu.Lock()
		r.canceledFlag = true
		r.mu.Unlock()
		r.canceledOnce.Do(func() { close(r.canceled) })
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

// functionalWorkerSignalTimeout is only a bounded diagnostic ceiling for a
// root-built process crossing into the injected command runner. The channel
// is the deterministic synchronization edge; a sleep or polling loop would
// not prove that the provider invocation reached the intended lifecycle point.
const functionalWorkerSignalTimeout = 10 * time.Second

func (r *functionalWorkerGate) waitCompleted(t testing.TB) {
	t.Helper()
	select {
	case <-r.completed:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatalf("functional worker did not reach deterministic completion")
	}
}

func (r *functionalWorkerGate) waitStarted(t testing.TB) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatalf("functional worker did not reach the deterministic execution gate")
	}
}

func (r *functionalWorkerGate) waitCallCount(t testing.TB, want int) {
	t.Helper()
	deadline := time.NewTimer(functionalWorkerSignalTimeout)
	defer deadline.Stop()
	for {
		if r.callCount() >= want {
			return
		}
		select {
		case <-r.callSignals:
		case <-deadline.C:
			t.Fatalf("functional worker calls = %d, want at least %d", r.callCount(), want)
		}
	}
}

func (r *functionalWorkerGate) waitCanceled(t testing.TB) {
	t.Helper()
	select {
	case <-r.canceled:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatalf("functional worker did not observe deterministic cancellation")
	}
}

func (r *functionalWorkerGate) wasCanceled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.canceledFlag
}

func (r *functionalWorkerGate) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func assertCompletedWorkerSessionEvents(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	wantWorkerSessionID string,
) {
	t.Helper()
	if len(frames) < 3 {
		t.Fatalf("Worker Session replay frames = %#v, want opening, provider, terminal, and replay summary", frames)
	}
	started := false
	terminalCount := 0
	for index, frame := range frames {
		if frame.WorkerSessionId != wantWorkerSessionID {
			t.Fatalf("Worker Session frame[%d] identity = %q, want %q", index, frame.WorkerSessionId, wantWorkerSessionID)
		}
		if frame.Delivery == "REPLAY_SUMMARY" {
			continue
		}
		if frame.Event.SourceType != "worker_session_lifecycle" {
			continue
		}
		phase := workerSessionEventPhase(frame)
		switch phase {
		case "STARTED":
			started = true
		case "COMPLETED", "FAILED", "CANCELED":
			terminalCount++
			if phase != "COMPLETED" {
				t.Fatalf("Worker Session terminal phase = %q, want COMPLETED; frames=%#v", phase, frames)
			}
		}
	}
	if !started {
		t.Fatalf("Worker Session replay has no STARTED lifecycle event: %#v", frames)
	}
	if terminalCount != 1 {
		t.Fatalf("Worker Session terminal lifecycle event count = %d, want one; frames=%#v", terminalCount, frames)
	}
}

func assertSingleTerminalWorkerSessionEvent(
	t *testing.T,
	frames []factoryapi.WorkerSessionEvent,
	wantWorkerSessionID, wantPhase string,
) {
	t.Helper()
	terminalCount := 0
	for index, frame := range frames {
		if frame.WorkerSessionId != wantWorkerSessionID {
			t.Fatalf("Worker Session frame[%d] identity = %q, want %q", index, frame.WorkerSessionId, wantWorkerSessionID)
		}
		if frame.Event.SourceType != "worker_session_lifecycle" {
			continue
		}
		phase := workerSessionEventPhase(frame)
		if phase == "COMPLETED" || phase == "FAILED" || phase == "CANCELED" || phase == "TERMINATED" {
			terminalCount++
			if phase != wantPhase {
				t.Fatalf("Worker Session terminal phase = %q, want %q; frames=%#v", phase, wantPhase, frames)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("Worker Session terminal lifecycle event count = %d, want one; frames=%#v", terminalCount, frames)
	}
}

func workerSessionEventPhase(frame factoryapi.WorkerSessionEvent) string {
	phase, _ := frame.Event.Payload["phase"].(string)
	return phase
}
