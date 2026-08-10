package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rootProcessReuseSuccessOutput = "root process reuse success COMPLETE"
	rootProcessReuseFailureOutput = "root process reuse failure"
)

// TestRootBuildProcessIsInertAndReusableAcrossFactorySessions proves the full
// P1 process boundary through public session APIs: BuildProcess does not
// activate injected effects, one process serves two isolated sessions, and
// both terminal outcomes retain their canonical event and response streams.
func TestRootBuildProcessIsInertAndReusableAcrossFactorySessions(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, rootProcessReuseFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"processor",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)

	identities := &rootProcessReuseIdentities{}
	providerRunner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(rootProcessReuseSuccessOutput)},
		platformprocess.CommandResult{
			Stderr:   []byte(rootProcessReuseFailureOutput),
			ExitCode: 1,
		},
	)
	logsRoot := filepath.Join(t.TempDir(), "logs")
	metricsRoot := filepath.Join(t.TempDir(), "metrics")
	edges := serviceedges.Edges{
		FactorySessionIDGenerator:                identities.generateSessionID,
		FactorySessionRuntimeInstanceIDGenerator: identities.generateRuntimeID,
		FactorySessionResponseEventIDGenerator:   identities.generateResponseEventID,
	}
	support.ConfigureWorkerCommands(t, &edges, providerRunner, nil)
	router := &reusableRootAPIServerStarter{}
	edges.APIServerStarter = router.start
	process := support.BuildProcess(t, edges)
	support.CleanupProcess(t, process)
	if got := providerRunner.CallCount(); got != 0 {
		t.Fatalf("provider command calls during BuildProcess = %d, want 0", got)
	}
	if got := identities.session.Load(); got != 0 {
		t.Fatalf("session IDs generated during BuildProcess = %d, want 0", got)
	}
	if got := identities.runtime.Load(); got != 0 {
		t.Fatalf("runtime IDs generated during BuildProcess = %d, want 0", got)
	}
	if got := identities.responseEvent.Load(); got != 0 {
		t.Fatalf("response-event IDs generated during BuildProcess = %d, want 0", got)
	}
	if got := router.starts.Load(); got != 0 {
		t.Fatalf("API server starts during BuildProcess = %d, want 0", got)
	}
	assertPathDoesNotExist(t, logsRoot, "runtime log root")
	assertPathDoesNotExist(t, metricsRoot, "runtime metrics root")

	firstServer := support.NewProcessAPIServer()
	router.setCurrent(firstServer)
	firstCommand := startReusableRootProcessServer(t, process, firstServer, dir, logsRoot, metricsRoot)
	firstURL := firstServer.WaitForURL(t)
	firstSession := support.GetDefaultSession(t, firstURL)
	if firstSession.Id == "" || firstSession.Runtime.StreamIdentity == nil {
		t.Fatalf("first default session = %#v, want session and stream identities", firstSession)
	}

	successInvocation, successEvents, successResponseEvents := runRootProcessSessionInvocation(
		t,
		firstURL,
		factorysessions.DefaultSessionID,
		"run the successful session",
	)
	if successInvocation.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("successful session invocation status = %q, want COMPLETED", successInvocation.Status)
	}
	assertInvocationPrimaryResultText(t, successInvocation, rootProcessReuseSuccessOutput)
	assertRootProcessFactoryEvents(t, successEvents, factorysessions.DefaultSessionID, factoryapi.WorkOutcomeAccepted)
	assertRootProcessResponseEvents(t, successResponseEvents, firstSession.Id, factoryapi.FactoryResponseEventPhaseCompleted)
	firstCommand.Stop(t)

	secondServer := support.NewProcessAPIServer()
	router.setCurrent(secondServer)
	secondCommand := startReusableRootProcessServer(t, process, secondServer, dir, logsRoot, metricsRoot)
	secondURL := secondServer.WaitForURL(t)
	secondSession := support.GetDefaultSession(t, secondURL)
	if secondSession.Id == "" || secondSession.Runtime.StreamIdentity == nil {
		t.Fatalf("second default session = %#v, want session and stream identities", secondSession)
	}
	if secondSession.Id == firstSession.Id {
		t.Fatalf("second session id = %q, want distinct from first %q", secondSession.Id, firstSession.Id)
	}
	if secondSession.Runtime.StreamIdentity.StreamGenerationID == firstSession.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("second stream generation = %q, want distinct from first %q", secondSession.Runtime.StreamIdentity.StreamGenerationID, firstSession.Runtime.StreamIdentity.StreamGenerationID)
	}

	failureInvocation, failureEvents, failureResponseEvents := runRootProcessSessionInvocation(
		t,
		secondURL,
		factorysessions.DefaultSessionID,
		"run the failing session",
	)
	if failureInvocation.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("failing session invocation status = %q, want FAILED", failureInvocation.Status)
	}
	if failureInvocation.SessionId != nil && *failureInvocation.SessionId != secondSession.Id {
		t.Fatalf("failing invocation session id = %#v, want nil or %q", failureInvocation.SessionId, secondSession.Id)
	}
	if failureInvocation.Message == nil && failureInvocation.ErrorCode == nil {
		t.Fatalf("failing invocation = %#v, want a terminal error summary", failureInvocation)
	}
	assertRootProcessFactoryEvents(t, failureEvents, factorysessions.DefaultSessionID, factoryapi.WorkOutcomeFailed)
	// A failed invocation may still have a completed provider-native response
	// stream: the canonical dispatch outcome and InvocationResponse carry the
	// failure, while Response Events describe provider activity.
	assertRootProcessResponseEvents(t, failureResponseEvents, secondSession.Id, "")
	secondCommand.Stop(t)

	if got := providerRunner.CallCount(); got != 2 {
		t.Fatalf("injected provider runner calls = %d, want exactly one per session invocation", got)
	}
	if got := identities.runtime.Load(); got != 2 {
		t.Fatalf("runtime IDs generated after two process executions = %d, want exactly 2", got)
	}
	if got := identities.responseEvent.Load(); got == 0 {
		t.Fatalf("response-event IDs generated after invocations = %d, want > 0", got)
	}
	assertPathExists(t, logsRoot, "runtime log root after execution")
	assertPathExists(t, metricsRoot, "runtime metrics root after execution")
}

type rootProcessReuseIdentities struct {
	session       atomic.Int32
	runtime       atomic.Int32
	responseEvent atomic.Int32
}

type reusableRootAPIServerStarter struct {
	mu      sync.Mutex
	current *support.ProcessAPIServer
	starts  atomic.Int32
}

func (s *reusableRootAPIServerStarter) setCurrent(server *support.ProcessAPIServer) {
	s.mu.Lock()
	s.current = server
	s.mu.Unlock()
}

func (s *reusableRootAPIServerStarter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	s.mu.Lock()
	server := s.current
	s.mu.Unlock()
	if server == nil {
		return fmt.Errorf("reusable root API server is not selected")
	}
	s.starts.Add(1)
	return server.Start(ctx, request)
}

func startReusableRootProcessServer(
	t *testing.T,
	process support.Process,
	server *support.ProcessAPIServer,
	dir string,
	logsRoot string,
	metricsRoot string,
) *support.ProcessCommand {
	t.Helper()

	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run", "--continuously", "--with-server", "--quiet",
		"--dir", dir,
		"--no-record",
		"--runtime-log-dir", logsRoot,
		"--runtime-metrics-dir", metricsRoot,
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	return command
}

func (r *rootProcessReuseIdentities) generateSessionID() string {
	return fmt.Sprintf("story006-session-%d", r.session.Add(1))
}

func (r *rootProcessReuseIdentities) generateRuntimeID() string {
	return fmt.Sprintf("story006-runtime-%d", r.runtime.Add(1))
}

func (r *rootProcessReuseIdentities) generateResponseEventID() string {
	return fmt.Sprintf("story006-response-event-%d", r.responseEvent.Add(1))
}

func runRootProcessSessionInvocation(
	t *testing.T,
	baseURL string,
	sessionID string,
	text string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()

	eventStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, sessionID))
	responseStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, sessionID))
	invocation := postRootProcessSessionInvocation(t, baseURL, sessionID, sessionsTextInvocationRequest(t, text))
	events := readRootProcessEventsUntilDispatchResponse(t, eventStream)
	responseEvents := readRootProcessResponseEventsUntilTerminal(t, responseStream)
	eventStream.Close()
	responseStream.Close()
	return invocation, events, responseEvents
}

func postRootProcessSessionInvocation(
	t *testing.T,
	baseURL string,
	sessionID string,
	request factoryapi.InvocationRequest,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal session invocation request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + sessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, payload)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode session invocation response: %v", err)
	}
	return decoded
}

func readRootProcessEventsUntilDispatchResponse(
	t *testing.T,
	stream *support.FactoryEventStream,
) []factoryapi.FactoryEvent {
	t.Helper()

	const maxEvents = 256
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for len(events) < maxEvents {
		event := stream.NextEvent(10 * time.Second)
		events = append(events, event)
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			return events
		}
	}
	t.Fatalf("read %d Factory Events without DISPATCH_RESPONSE", maxEvents)
	return nil
}

func readRootProcessResponseEventsUntilTerminal(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
) []factoryapi.FactoryResponseEvent {
	t.Helper()

	const maxEvents = 256
	events := make([]factoryapi.FactoryResponseEvent, 0, 16)
	for len(events) < maxEvents {
		event := stream.NextFrame(10 * time.Second).Event
		events = append(events, event)
		if event.Kind == factoryapi.FactoryResponseEventKindRun &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
				event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return events
		}
		if event.Kind == factoryapi.FactoryResponseEventKindError &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return events
		}
	}
	t.Fatalf("read %d Response Events without a terminal event", maxEvents)
	return nil
}

func assertRootProcessFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	wantOutcome factoryapi.WorkOutcome,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("Factory Event stream = empty, want canonical invocation events")
	}

	requestIndex, responseIndex := -1, -1
	for index, event := range events {
		if event.Id == "" {
			t.Fatalf("Factory Event %d has empty id", index)
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event %q session id = %q, want %q", event.Id, *event.Context.SessionId, sessionID)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if requestIndex == -1 {
				requestIndex = index
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if responseIndex == -1 {
				responseIndex = index
			}
		}
	}
	if requestIndex == -1 || responseIndex == -1 || requestIndex >= responseIndex {
		t.Fatalf(
			"Factory Event dispatch order = request:%d response:%d, want request before response",
			requestIndex,
			responseIndex,
		)
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one completed dispatch", dispatches)
	}
	if dispatches[0].Response.Outcome != wantOutcome {
		t.Fatalf("dispatch outcome = %q, want %q", dispatches[0].Response.Outcome, wantOutcome)
	}
}

func assertRootProcessResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	sessionID string,
	wantTerminalPhase factoryapi.FactoryResponseEventPhase,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("Response Event stream = empty, want session-scoped activity")
	}

	terminal := false
	for _, event := range events {
		if event.EventId == "" {
			t.Fatal("Response Event has empty event id")
		}
		if event.FactorySessionId != sessionID {
			t.Fatalf("Response Event session id = %q, want %q", event.FactorySessionId, sessionID)
		}
		if event.Kind == factoryapi.FactoryResponseEventKindRun &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
				event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) &&
			(wantTerminalPhase == "" || event.Phase == wantTerminalPhase) {
			terminal = true
		}
	}
	if !terminal {
		t.Fatalf("Response Events = %#v, want terminal RUN phase %q", events, wantTerminalPhase)
	}
}

func assertInvocationPrimaryResultText(
	t *testing.T,
	response factoryapi.InvocationResponse,
	want string,
) {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("successful invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("successful invocation primaryResult as text part: %v", err)
	}
	if part.Text != want {
		t.Fatalf("successful invocation primaryResult text = %q, want %q", part.Text, want)
	}
}

func assertPathDoesNotExist(t testing.TB, path, label string) {
	t.Helper()
	_, err := os.Stat(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s stat error = %v, want path not to exist", label, err)
	}
}

func assertPathExists(t testing.TB, path, label string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s stat error = %v, want path to exist", label, err)
	}
}

func rootProcessReuseFactoryConfig() map[string]any {
	return map[string]any{
		"name": "story006-root-process-reuse",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
