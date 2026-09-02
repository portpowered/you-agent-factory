package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	rootProcessReuseSuccessOutput = "root process reuse success COMPLETE"
	rootProcessReuseFailureOutput = "root process reuse failure"
	rootProcessStreamReadCeiling  = 5 * time.Second
)

// TestRootBuildProcessIsInertAndReusableAcrossFactorySessions proves through
// the public CLI and session observation APIs that one process serves two
// isolated sessions and both terminal outcomes retain their canonical event
// and response streams.
func TestRootBuildProcessIsInertAndReusableAcrossFactorySessions(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	fixture := newRootProcessReuseFixture(t)

	first := runRootProcessCLIInvocation(t, fixture, 0, "run the successful session")
	assertRootProcessSuccess(t, first)

	second := runRootProcessCLIInvocation(t, fixture, 1, "run the failing session")
	assertRootProcessFailure(t, second)
	assertRootProcessReuse(t, fixture, first.session, second.session)
	t.Run("direct JavaScript transport start failure", func(t *testing.T) {
		assertRootProcessReportsDirectJavaScriptTransportStartFailure(t, fixture)
	})
}

func assertRootProcessReportsDirectJavaScriptTransportStartFailure(t *testing.T, fixture *rootProcessReuseFixture) {
	t.Helper()
	workingDirectory := t.TempDir()
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "unreachable";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	const failureText = "injected direct JavaScript host failure"
	startsBefore := fixture.router.starts.Load()
	fixture.router.setFailure(errors.New(failureText))

	var stdout, stderr bytes.Buffer
	err := fixture.process.Execute(root.Input{
		Args: []string{
			"you", "run", "--factory", workflowPath, "--with-mock-workers", "--with-server",
		},
		Env:              append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir()),
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
	})
	if err == nil || !strings.Contains(err.Error(), failureText) {
		t.Fatalf("Process.Execute(direct JavaScript host failure) error = %v, want injected failure; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if got := fixture.router.starts.Load() - startsBefore; got != 1 {
		t.Fatalf("injected API server starter calls = %d, want exactly one", got)
	}
	if strings.Contains(stdout.String(), "completed (SUCCEEDED)") {
		t.Fatalf("direct JavaScript failure stdout = %q, want no success result", stdout.String())
	}
}

type rootProcessReuseFixture struct {
	process        support.Process
	dir            string
	identities     *rootProcessReuseIdentities
	providerRunner *gatedRootProviderCommandRunner
	router         *reusableRootAPIServerStarter
	logsRoot       string
	metricsRoot    string
}

func newRootProcessReuseFixture(t *testing.T) *rootProcessReuseFixture {
	t.Helper()
	dir := support.ScaffoldFactory(t, rootProcessReuseFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	identities := &rootProcessReuseIdentities{}
	providerRunner := newGatedRootProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(rootProcessReuseSuccessOutput)},
		platformprocess.CommandResult{Stderr: []byte(rootProcessReuseFailureOutput), ExitCode: 1},
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
	return &rootProcessReuseFixture{
		process: process, dir: dir, identities: identities,
		providerRunner: providerRunner, router: router,
		logsRoot: logsRoot, metricsRoot: metricsRoot,
	}
}

type rootProcessInvocation struct {
	response       factoryapi.InvocationResponse
	session        factoryapi.FactorySession
	events         []factoryapi.FactoryEvent
	responseEvents []factoryapi.FactoryResponseEvent
	executeErr     error
}

func runRootProcessCLIInvocation(
	t *testing.T,
	fixture *rootProcessReuseFixture,
	callIndex int,
	text string,
) rootProcessInvocation {
	t.Helper()
	shutdown := make(chan struct{})
	server := support.NewProcessAPIServer()
	server.HoldShutdownUntilSignaled(shutdown)
	fixture.router.setCurrent(server)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "--json", "run", "--factory", filepath.Join(fixture.dir, "factory.json"),
		"--with-server", "--server", "http://127.0.0.1:1", "--no-record",
		"--runtime-log-dir", fixture.logsRoot, "--runtime-metrics-dir", fixture.metricsRoot, text,
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = fixture.dir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	baseURL := server.WaitForURL(t)
	session := support.GetDefaultSession(t, baseURL)
	eventStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, session.Id))
	responseStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, session.Id))
	fixture.providerRunner.Release(callIndex)
	events := readRootProcessEventsUntilDispatchResponse(t, eventStream)
	responseEvents := readRootProcessResponseEventsUntilTerminal(t, responseStream)
	eventStream.Close()
	responseStream.Close()
	close(shutdown)
	<-command.Done()
	executeErr := command.Err()
	if executeErr != nil {
		command.AcceptError()
	}
	return rootProcessInvocation{
		response: support.DecodeInvocationResponseJSON(t, inputs.Stdout()),
		session:  session, events: events, responseEvents: responseEvents,
		executeErr: executeErr,
	}
}

func assertRootProcessSuccess(t *testing.T, invocation rootProcessInvocation) {
	t.Helper()
	if invocation.executeErr != nil {
		t.Fatalf("successful CLI invocation error = %v", invocation.executeErr)
	}
	if invocation.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("successful CLI invocation status = %q, want COMPLETED", invocation.response.Status)
	}
	assertInvocationPrimaryResultText(t, invocation.response, rootProcessReuseSuccessOutput)
	assertRootProcessFactoryEvents(t, invocation.events, factorysessions.DefaultSessionID, factoryapi.WorkOutcomeAccepted)
	assertRootProcessResponseEvents(t, invocation.responseEvents, invocation.session.Id, factoryapi.FactoryResponseEventPhaseCompleted)
}

func assertRootProcessFailure(t *testing.T, invocation rootProcessInvocation) {
	t.Helper()
	if invocation.response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("failing CLI invocation status = %q, want FAILED", invocation.response.Status)
	}
	if invocation.response.SessionId != nil && *invocation.response.SessionId != invocation.session.Id {
		t.Fatalf("failing invocation session id = %#v, want nil or %q", invocation.response.SessionId, invocation.session.Id)
	}
	if invocation.response.Message == nil && invocation.response.ErrorCode == nil {
		t.Fatalf("failing invocation = %#v, want a terminal error summary", invocation.response)
	}
	assertRootProcessFactoryEvents(t, invocation.events, factorysessions.DefaultSessionID, factoryapi.WorkOutcomeFailed)
	// A failed invocation may still have a completed provider-native response
	// stream: the canonical dispatch outcome and InvocationResponse carry the
	// failure, while Response Events describe provider activity.
	assertRootProcessResponseEvents(t, invocation.responseEvents, invocation.session.Id, "")
}

func assertRootProcessReuse(
	t *testing.T,
	fixture *rootProcessReuseFixture,
	first, second factoryapi.FactorySession,
) {
	t.Helper()
	if first.Id == "" || first.Runtime.StreamIdentity == nil {
		t.Fatalf("first default session = %#v, want session and stream identities", first)
	}
	if second.Id == "" || second.Runtime.StreamIdentity == nil {
		t.Fatalf("second default session = %#v, want session and stream identities", second)
	}
	if second.Id == first.Id {
		t.Fatalf("second session id = %q, want distinct from first %q", second.Id, first.Id)
	}
	if second.Runtime.StreamIdentity.StreamGenerationID == first.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf("second stream generation = %q, want distinct from first %q", second.Runtime.StreamIdentity.StreamGenerationID, first.Runtime.StreamIdentity.StreamGenerationID)
	}
	if got := fixture.providerRunner.CallCount(); got != 2 {
		t.Fatalf("injected provider runner calls = %d, want exactly one per session invocation", got)
	}
	if got := fixture.identities.runtime.Load(); got != 2 {
		t.Fatalf("runtime IDs generated after two process executions = %d, want exactly 2", got)
	}
	if got := fixture.identities.responseEvent.Load(); got == 0 {
		t.Fatalf("response-event IDs generated after invocations = %d, want > 0", got)
	}
	assertPathExists(t, fixture.logsRoot, "runtime log root after execution")
	assertPathExists(t, fixture.metricsRoot, "runtime metrics root after execution")
}

type rootProcessReuseIdentities struct {
	session       atomic.Int32
	runtime       atomic.Int32
	responseEvent atomic.Int32
}

type gatedRootProviderCommandRunner struct {
	mu       sync.Mutex
	releases []chan struct{}
	results  []platformprocess.CommandResult
	calls    int
}

func newGatedRootProviderCommandRunner(results ...platformprocess.CommandResult) *gatedRootProviderCommandRunner {
	releases := make([]chan struct{}, len(results))
	for index := range releases {
		releases[index] = make(chan struct{})
	}
	return &gatedRootProviderCommandRunner{releases: releases, results: results}
}

func (runner *gatedRootProviderCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	index := runner.calls
	runner.calls++
	if index >= len(runner.results) {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider command call %d", index+1)
	}
	release := runner.releases[index]
	result := runner.results[index]
	runner.mu.Unlock()
	select {
	case <-release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	if result.ExitCode == 0 {
		result.Stdout = support.CodexSuccessStdout(string(result.Stdout))
	}
	return result, nil
}

func (runner *gatedRootProviderCommandRunner) Release(index int) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	close(runner.releases[index])
}

func (runner *gatedRootProviderCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

var _ platformprocess.CommandRunner = (*gatedRootProviderCommandRunner)(nil)

type reusableRootAPIServerStarter struct {
	mu      sync.Mutex
	current *support.ProcessAPIServer
	failure error
	starts  atomic.Int32
}

func (s *reusableRootAPIServerStarter) setCurrent(server *support.ProcessAPIServer) {
	s.mu.Lock()
	s.current = server
	s.failure = nil
	s.mu.Unlock()
}

func (s *reusableRootAPIServerStarter) setFailure(err error) {
	s.mu.Lock()
	s.current = nil
	s.failure = err
	s.mu.Unlock()
}

func (s *reusableRootAPIServerStarter) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	s.mu.Lock()
	server, failure := s.current, s.failure
	s.mu.Unlock()
	if failure != nil {
		s.starts.Add(1)
		return failure
	}
	if server == nil {
		return fmt.Errorf("reusable root API server is not selected")
	}
	s.starts.Add(1)
	return server.Start(ctx, request)
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

func readRootProcessEventsUntilDispatchResponse(
	t *testing.T,
	stream *support.FactoryEventStream,
) []factoryapi.FactoryEvent {
	t.Helper()

	const maxEvents = 256
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for len(events) < maxEvents {
		// The provider command is held on an explicit release channel until both
		// streams are open. This timeout is only a failure ceiling, not polling
		// or synchronization padding.
		event := stream.NextEvent(rootProcessStreamReadCeiling)
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
		// The provider command gate makes terminal publication deterministic; the
		// bounded read protects the test from a malformed or stalled stream.
		event := stream.NextFrame(rootProcessStreamReadCeiling).Event
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
		if event.Kind == factoryapi.FactoryResponseEventKindError &&
			(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) &&
			wantTerminalPhase == "" {
			// A direct CLI provider failure may publish a synthesized ERROR terminal
			// event instead of a provider-native RUN terminal event.
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
