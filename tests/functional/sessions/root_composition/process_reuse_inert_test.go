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

// TestRootBuildProcessIsInertAndReusableAcrossFactorySessions proves the full
// P1 process boundary through the public CLI and session observation APIs:
// BuildProcess does not activate injected effects, one process serves two
// isolated sessions, and both terminal outcomes retain their canonical event
// and response streams.
func TestRootBuildProcessIsInertAndReusableAcrossFactorySessions(t *testing.T) {
	acquireRootCompositionFixtureSlot(t)

	shared := ensureRootCompositionFixture(t)
	fixture := newRootProcessReuseFixture(t, shared)
	assertRootProcessBuildIsInert(t, fixture)

	t.Run("direct JavaScript transport start failure", func(t *testing.T) {
		assertRootProcessReportsDirectJavaScriptTransportStartFailure(t, fixture)
	})

	first := runRootProcessCLIInvocation(t, fixture, 0, "run the successful session")
	assertRootProcessSuccess(t, first)

	second := runRootProcessCLIInvocation(t, fixture, 1, "run the failing session")
	assertRootProcessFailure(t, second)
	assertRootProcessReuse(t, fixture, first.session, second.session)
}

func assertRootProcessReportsDirectJavaScriptTransportStartFailure(t *testing.T, fixture *rootProcessReuseFixture) {
	t.Helper()
	workingDirectory := fixture.failureDir
	workflowPath := filepath.Join(workingDirectory, "workflow.js")
	if err := os.WriteFile(workflowPath, []byte(`return "unreachable";`), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	const failureText = "injected direct JavaScript host failure"
	fixture.shared.withRootCompositionRouteValue(t, rootCompositionRouteSpec{
		label:      "root-process-reuse-javascript-failure",
		homeDir:    t.TempDir(),
		workingDir: workingDirectory,
		apiStarter: func(_ context.Context, _ platformhttpserver.StartRequest) error {
			fixture.apiFailureStarts.Add(1)
			return fixture.apiFailure
		},
		providerRunner: nil,
	}, func(route *rootCompositionRoute) {
		var stdout, stderr bytes.Buffer
		err := fixture.shared.process.Execute(root.Input{
			Args: []string{
				"you", "run", "--factory", workflowPath, "--with-mock-workers", "--with-server",
			},
			Env:              append(os.Environ(), "HOME="+route.homeDir, "USERPROFILE="+route.homeDir),
			Stdin:            strings.NewReader(""),
			Stdout:           &stdout,
			Stderr:           &stderr,
			Context:          withRootCompositionRouteContext(t.Context(), route),
			WorkingDirectory: workingDirectory,
		})
		if err == nil || !strings.Contains(err.Error(), failureText) {
			t.Fatalf("Process.Execute(direct JavaScript host failure) error = %v, want injected failure; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), "completed (SUCCEEDED)") {
			t.Fatalf("direct JavaScript failure stdout = %q, want no success result", stdout.String())
		}
	})
	if got := fixture.apiFailureStarts.Load(); got != 1 {
		t.Fatalf("injected API server starter calls = %d, want exactly one", got)
	}
}

type rootProcessReuseFixture struct {
	shared           *rootCompositionFixture
	failureDir       string
	invocationDirs   []string
	providerRunner   *gatedRootProviderCommandRunner
	logsRoot         string
	metricsRoot      string
	apiFailure       error
	apiFailureStarts atomic.Int32
}

func newRootProcessReuseFixture(t *testing.T, shared *rootCompositionFixture) *rootProcessReuseFixture {
	t.Helper()
	invocationDirs := make([]string, 2)
	for index := range invocationDirs {
		dir := support.ScaffoldFactory(t, rootProcessReuseFactoryConfig())
		support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		invocationDirs[index] = dir
	}
	providerRunner := newGatedRootProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(rootProcessReuseSuccessOutput)},
		platformprocess.CommandResult{Stderr: []byte(rootProcessReuseFailureOutput), ExitCode: 1},
	)
	return &rootProcessReuseFixture{
		shared: shared, failureDir: t.TempDir(), invocationDirs: invocationDirs,
		providerRunner: providerRunner,
		logsRoot:       filepath.Join(t.TempDir(), "logs"), metricsRoot: filepath.Join(t.TempDir(), "metrics"),
		apiFailure: errors.New("injected direct JavaScript host failure"),
	}
}

func assertRootProcessBuildIsInert(t *testing.T, fixture *rootProcessReuseFixture) {
	t.Helper()
	if got := fixture.providerRunner.CallCount(); got != 0 {
		t.Fatalf("provider command calls during BuildProcess = %d, want 0", got)
	}
	snapshot := fixture.shared.constructionSnapshot()
	if got := snapshot.total(); got != 0 {
		t.Fatalf("shared root construction effect calls = %d, want 0", got)
	}
	assertPathDoesNotExist(t, fixture.logsRoot, "runtime log root")
	assertPathDoesNotExist(t, fixture.metricsRoot, "runtime metrics root")
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
	if callIndex < 0 || callIndex >= len(fixture.invocationDirs) {
		t.Fatalf("root process invocation index = %d, want [0,%d)", callIndex, len(fixture.invocationDirs))
	}
	workingDirectory := fixture.invocationDirs[callIndex]
	var invocation rootProcessInvocation
	fixture.shared.withRootCompositionRouteValue(t, rootCompositionRouteSpec{
		label:          fmt.Sprintf("root-process-reuse-invocation-%d", callIndex),
		homeDir:        t.TempDir(),
		workingDir:     workingDirectory,
		providerRunner: fixture.providerRunner,
	}, func(route *rootCompositionRoute) {
		server := startRootCompositionDirectServer(t, fixture.shared, route, support.NewProcessAPIServer(), []string{
			"you", "--json", "run", "--factory", filepath.Join(workingDirectory, "factory.json"),
			"--with-server", "--server", "http://127.0.0.1:1", "--no-record",
			"--runtime-log-dir", fixture.logsRoot, "--runtime-metrics-dir", fixture.metricsRoot, text,
		}, nil, workingDirectory)
		baseURL := server.URL(t)
		session := support.GetDefaultSession(t, baseURL)
		eventStream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(baseURL, session.Id))
		responseStream := support.OpenFactoryResponseEventStreamAt(t, support.SessionResponseEventsURL(baseURL, session.Id))
		fixture.providerRunner.Release(callIndex)
		events := readRootProcessEventsUntilDispatchResponse(t, eventStream)
		responseEvents := readRootProcessResponseEventsUntilTerminal(t, responseStream)
		eventStream.Close()
		responseStream.Close()
		if callIndex == 1 {
			server.command.AcceptError()
		}
		server.Finish(t)
		invocation = rootProcessInvocation{
			response:       support.DecodeInvocationResponseJSON(t, server.inputs.Stdout()),
			session:        session,
			events:         events,
			responseEvents: responseEvents,
			executeErr:     server.Err(),
		}
	})
	return invocation
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
		t.Fatalf("first explicit session = %#v, want session and stream identities", first)
	}
	if second.Id == "" || second.Runtime.StreamIdentity == nil {
		t.Fatalf("second explicit session = %#v, want session and stream identities", second)
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
	assertPathExists(t, fixture.logsRoot, "runtime log root after execution")
	assertPathExists(t, fixture.metricsRoot, "runtime metrics root after execution")
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

func readRootProcessEventsUntilDispatchResponse(
	t *testing.T,
	stream *support.FactoryEventStream,
) []factoryapi.FactoryEvent {
	t.Helper()

	const maxEvents = 256
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for len(events) < maxEvents {
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
		t.Fatalf("%s path %q stat error = %v, want path to exist", label, path, err)
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
		"workers": []map[string]string{{"name": "processor", "executorProvider": "codex"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
