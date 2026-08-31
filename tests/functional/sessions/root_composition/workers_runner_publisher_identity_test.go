package root_composition_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runnerIdentityGoldenCase   = "success"
	runnerIdentityScriptOutput = "runner-identity-script-tagged-output"

	runnerIdentityScriptWorkerAgentConfig = "---\n" +
		"type: SCRIPT_WORKER\n" +
		"command: echo\n" +
		"args:\n" +
		"    - default-output\n" +
		"---\n"

	runnerIdentityFailureModel          = "runner-identity-failure-model"
	runnerIdentityProviderFailureStderr = "runner-identity-provider-failure"
	runnerIdentityScriptFailureStderr   = "runner-identity-script-failure"
)

// TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances
// proves a single Factory Runtime build constructed only through
// root.BuildProcess (support.RunFactoryToCompletionWithEdgesAndResponseEvents
// composes the process through exactly one such call) uses the exact injected
// provider command runner, script command runner, and session progress
// publisher for the full lifetime of the built runtime, with no
// post-construction mutation, derived Workers view, or second Workers-root
// construction.
//
// A provider-backed dispatch and a script-backed dispatch both run against
// the SAME root-built process. Provider identity is proven directly: the
// injected providerRunner is a distinguishable Go instance, and its CallCount
// only increments when Workers execution actually invokes that exact pointer
// -- a derived/second Workers view built with its own default runner would
// never touch it, and no real "codex" binary exists in the test sandbox for
// a default runner to fall back to, so a passing provider-task completion
// with CallCount() == 1 is only possible through this exact instance.
// Script identity is proven by exact output-text equality: "echo" is a real
// executable that WOULD succeed if a derived/second Workers view fell back to
// an un-injected default script runner, so CallCount alone would not rule
// that out, but the dispatch's public primary result is asserted to be the
// literal tagged output only the injected scriptRunner instance returns
// (which is different from the AGENTS.md-configured real echo argument).
// Progress publisher identity is proven behaviorally: the provider fixture
// stdout streams a multi-record partial turn, and the resulting PROGRESS
// response events are asserted to reach the SAME session's public
// response-event stream that support.RunFactoryToCompletionWithEdgesAndResponseEvents
// subscribes to before dispatch, which is only possible if the progress
// publisher supplied to this exact Workers construction was the one that
// received progress fragments during execution.
func TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	loaded := loadRunnerIdentityCodexGoldenCase(t)
	dir := support.ScaffoldFactory(t, runnerIdentityFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"provider-worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)
	support.WriteAgentConfig(t, dir, "script-worker", runnerIdentityScriptWorkerAgentConfig)
	testutil.WriteSeedFile(t, dir, "provider-task", []byte("runner-identity-provider-seed"))
	testutil.WriteSeedFile(t, dir, "script-task", []byte("runner-identity-script-seed"))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	providerRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})
	scriptRunner := support.NewRecordingCommandRunner(runnerIdentityScriptOutput)

	_, listed, factoryEvents, responseEvents := runRunnerIdentityScenario(
		t, dir, providerRunner, scriptRunner, false,
	)

	if got := support.CountWorkAtCustomerState(listed, "provider-task:complete"); got != 1 {
		t.Fatalf("provider-task completed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "provider-task:failed"); got != 0 {
		t.Fatalf("provider-task failed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:complete"); got != 1 {
		t.Fatalf("script-task completed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:failed"); got != 0 {
		t.Fatalf("script-task failed work tokens = %d, want 0", got)
	}

	if got := providerRunner.CallCount(); got != 1 {
		t.Fatalf(
			"provider command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled)",
			got,
		)
	}
	if got := scriptRunner.CallCount(); got != 1 {
		t.Fatalf(
			"script command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled)",
			got,
		)
	}

	assertRunnerIdentityScriptDispatchOutput(t, factoryEvents, runnerIdentityScriptOutput)
	assertRunnerIdentityProviderResponseEventSequence(t, responseEvents)
	assertRunnerIdentityDispatchSequence(t, factoryEvents, listed, "provider-task", factoryapi.WorkOutcomeAccepted)
	assertRunnerIdentityDispatchSequence(t, factoryEvents, listed, "script-task", factoryapi.WorkOutcomeAccepted)
}

// TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance
// proves the same single-Workers-root construction path used by
// TestRootBuildProcessRoutesProviderAndScriptWorkThroughInjectedRunnerInstances
// preserves existing failed-dispatch semantics after the cutover: a
// representative provider and script command runner failure each remain a
// failed dispatch with a non-empty public error, not a successful
// construction or execution, and each failure is observed on the exact
// injected runner instance (CallCount() == 1), ruling out a derived or
// second Workers view handling the failure path instead.
func TestRootBuildProcessRunnerFailureRoutesToFailedDispatchThroughInjectedInstance(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	dir := support.ScaffoldFactory(t, runnerIdentityFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"provider-worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, runnerIdentityFailureModel),
	)
	support.WriteAgentConfig(t, dir, "script-worker", runnerIdentityScriptWorkerAgentConfig)
	testutil.WriteSeedFile(t, dir, "provider-task", []byte("runner-identity-provider-failure-seed"))
	testutil.WriteSeedFile(t, dir, "script-task", []byte("runner-identity-script-failure-seed"))

	providerRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte(runnerIdentityProviderFailureStderr),
		ExitCode: 1,
	})
	scriptRunner := &runnerIdentityFailingScriptCommandRunner{
		stderr:   runnerIdentityScriptFailureStderr,
		exitCode: 1,
	}

	_, listed, factoryEvents, _ := runRunnerIdentityScenario(
		t, dir, providerRunner, scriptRunner, true,
	)

	if got := support.CountWorkAtCustomerState(listed, "provider-task:failed"); got != 1 {
		t.Fatalf("provider-task failed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "provider-task:complete"); got != 0 {
		t.Fatalf("provider-task completed work tokens = %d, want 0", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:failed"); got != 1 {
		t.Fatalf("script-task failed work tokens = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "script-task:complete"); got != 0 {
		t.Fatalf("script-task completed work tokens = %d, want 0", got)
	}

	if got := providerRunner.CallCount(); got != 1 {
		t.Fatalf(
			"provider command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled on the failure path too)",
			got,
		)
	}
	if got := scriptRunner.CallCount(); got != 1 {
		t.Fatalf(
			"script command runner calls on the injected instance = %d, want exactly 1 "+
				"(a derived or second Workers root would leave this exact instance uncalled on the failure path too)",
			got,
		)
	}

	assertRunnerIdentityFailedDispatchCount(t, factoryEvents, 2)
	assertRunnerIdentityDispatchSequence(t, factoryEvents, listed, "provider-task", factoryapi.WorkOutcomeFailed)
	assertRunnerIdentityDispatchSequence(t, factoryEvents, listed, "script-task", factoryapi.WorkOutcomeFailed)
}

// runRunnerIdentityScenario runs the original two-seed dispatch scenario on
// one long-lived package process. Each scenario owns a distinct public
// Factory Session while the route selects the exact provider and script
// runner by the Factory path.
func runRunnerIdentityScenario(
	t *testing.T,
	dir string,
	providerRunner platformprocess.CommandRunner,
	scriptRunner platformprocess.CommandRunner,
	failure bool,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	fixture := ensureRootCompositionFixture(t)
	var session factoryapi.FactorySession
	var listed factoryapi.ListWorkResponse
	var factoryEvents []factoryapi.FactoryEvent
	var responseEvents []factoryapi.FactoryResponseEvent
	label := "runner-identity-success"
	if failure {
		label = "runner-identity-failure"
	}
	providerGate := &runnerIdentityReleaseGate{
		next:    providerRunner,
		release: make(chan struct{}),
	}
	scriptGate := &runnerIdentityReleaseGate{
		next:    scriptRunner,
		release: make(chan struct{}),
	}
	fixture.withRootCompositionRoute(t, rootCompositionRouteSpec{
		label:          label,
		homeDir:        t.TempDir(),
		workingDir:     dir,
		providerRunner: providerGate,
		scriptRunner:   scriptGate,
	}, func() {
		server := startRootCompositionServer(t, fixture, support.NewProcessAPIServer(), []string{
			"you", "run",
			"--dir", dir,
			"--continuously",
			"--with-server",
			"--quiet",
			"--no-record",
		}, nil, dir)
		baseURL := server.URL(t)
		sessionID := server.SessionID(t)
		responseStream := support.OpenFactoryResponseEventStreamAt(
			t,
			support.SessionResponseEventsURL(baseURL, sessionID),
		)
		providerGate.Release()
		scriptGate.Release()
		// The two seed files preserve the original discovery semantics; the
		// response stream is opened before waiting for their dispatches.
		waitForDefaultSessionWorkCount(t, baseURL, sessionID, 2, 10*time.Second)
		support.WaitForSessionTerminalStatus(t, baseURL, sessionID, 20*time.Second)
		session = getRootCompositionSession(t, baseURL, sessionID)
		listed = support.GetJSON[factoryapi.ListWorkResponse](t, rootCompositionSessionWorkURL(baseURL, sessionID, "/work"))
		factoryEvents = support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
		if !failure {
			// The Work projection can become terminal before the session-owned
			// response publisher has flushed the provider's final native record
			// and the dispatch progress marker. Read the complete expected
			// success stream while the session is still live; canceling at the
			// projection boundary can otherwise discard that last publication
			// under race-detector scheduling.
			for range len(runnerIdentityExpectedNativeStreamEvents) + 1 {
				responseEvents = append(responseEvents, responseStream.NextFrame(10*time.Second).Event)
			}
		}
		server.Stop(t)
		responseStream.WaitClosed(5 * time.Second)
		for {
			frame, ok := responseStream.TryNextFrame(time.Nanosecond)
			if !ok {
				break
			}
			responseEvents = append(responseEvents, frame.Event)
		}
	})
	return session, listed, factoryEvents, responseEvents
}

// runnerIdentityReleaseGate holds the provider effect until the public
// response-event subscription is established, preserving the original
// pre-dispatch capture boundary when Process.Execute is reused.
type runnerIdentityReleaseGate struct {
	next    platformprocess.CommandRunner
	release chan struct{}
	once    sync.Once
}

func (gate *runnerIdentityReleaseGate) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	select {
	case <-gate.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return gate.next.Run(ctx, request)
}

func (gate *runnerIdentityReleaseGate) Release() {
	gate.once.Do(func() { close(gate.release) })
}

// runnerIdentityFailingScriptCommandRunner is a minimal script-worker
// platformprocess.CommandRunner that always fails, mirroring the established
// nonZeroExitScriptCommandRunner pattern used for petri dispatch
// terminal-routing tests.
type runnerIdentityFailingScriptCommandRunner struct {
	stderr   string
	exitCode int

	mu    sync.Mutex
	calls int
}

func (r *runnerIdentityFailingScriptCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()

	return platformprocess.CommandResult{
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func (r *runnerIdentityFailingScriptCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func assertRunnerIdentityFailedDispatchCount(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want int,
) {
	t.Helper()

	got := 0
	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil {
			continue
		}
		if observation.Response.Outcome != factoryapi.WorkOutcomeFailed {
			continue
		}
		if observation.Response.Error == nil || *observation.Response.Error == "" {
			continue
		}
		got++
	}
	if got != want {
		t.Fatalf("failed dispatch responses with a public error = %d, want %d", got, want)
	}
}

func runnerIdentityFactoryConfig() map[string]any {
	return map[string]any{
		"name": "runner-publisher-identity",
		"workTypes": []map[string]any{
			{
				"name": "provider-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "script-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "provider-worker", "executorProvider": "codex"},
			{"name": "script-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "provider-station",
				"worker":    "provider-worker",
				"inputs":    []map[string]string{{"workType": "provider-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "provider-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "provider-task", "state": "failed"}},
			},
			{
				"name":      "script-station",
				"worker":    "script-worker",
				"inputs":    []map[string]string{{"workType": "script-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "script-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "script-task", "state": "failed"}},
			},
		},
	}
}

func loadRunnerIdentityCodexGoldenCase(t *testing.T) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath(
			string(modelprovider.ProviderCodex),
			runnerIdentityGoldenCase,
		)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", runnerIdentityGoldenCase, err)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}
	return loaded
}

func assertRunnerIdentityScriptDispatchOutput(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want string,
) {
	t.Helper()

	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil || observation.Response.Output == nil {
			continue
		}
		if *observation.Response.Output == want {
			return
		}
	}
	t.Fatalf("Factory Event history has no dispatch response with script-tagged output %q", want)
}

// runnerIdentityExpectedNativeStreamEvents is the exact, ordered kind/phase
// sequence the codex/success golden fixture (loadRunnerIdentityCodexGoldenCase)
// produces from its single native provider stream: run.started, then the one
// MCP tool call's started/completed pair, then run.completed, then the final
// message snapshot. These five events all derive from sequential records in
// one native stream and are observed in this exact order across repeated runs.
var runnerIdentityExpectedNativeStreamEvents = []runnerIdentityKindPhase{
	{Kind: factoryapi.FactoryResponseEventKindRun, Phase: factoryapi.FactoryResponseEventPhaseStarted},
	{Kind: factoryapi.FactoryResponseEventKindTool, Phase: factoryapi.FactoryResponseEventPhaseStarted},
	{Kind: factoryapi.FactoryResponseEventKindTool, Phase: factoryapi.FactoryResponseEventPhaseCompleted},
	{Kind: factoryapi.FactoryResponseEventKindRun, Phase: factoryapi.FactoryResponseEventPhaseCompleted},
	{Kind: factoryapi.FactoryResponseEventKindMessage, Phase: factoryapi.FactoryResponseEventPhaseCompleted},
}

type runnerIdentityKindPhase struct {
	Kind  factoryapi.FactoryResponseEventKind
	Phase factoryapi.FactoryResponseEventPhase
}

// assertRunnerIdentityProviderResponseEventSequence proves both exactly-once
// progress delivery and the relevant ordered response-event sequence for the
// provider dispatch. The dispatch's own generic "work started" PROGRESS
// marker is published from the Factory Session progress publisher
// independently of the provider's native stream fragments, so its position
// relative to the native-stream events is not itself ordered by any public
// contract -- repeated runs of this exact scenario observe it at every
// position from first to last. Its COUNT (exactly one) is what proves
// exactly-once delivery; the five native-stream events (which share one
// ordered source) are asserted in their exact relative sequence with the
// PROGRESS marker filtered out.
func assertRunnerIdentityProviderResponseEventSequence(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	if got, want := len(events), len(runnerIdentityExpectedNativeStreamEvents)+1; got != want {
		t.Fatalf("captured Response Events = %#v, want exactly %d events (got %d)", events, want, got)
	}

	progressCount := 0
	nativeStream := make([]runnerIdentityKindPhase, 0, len(runnerIdentityExpectedNativeStreamEvents))
	for _, event := range events {
		if event.Kind == factoryapi.FactoryResponseEventKindProgress {
			progressCount++
			if event.Phase != factoryapi.FactoryResponseEventPhaseUpdated {
				t.Fatalf("PROGRESS response event phase = %q, want %q", event.Phase, factoryapi.FactoryResponseEventPhaseUpdated)
			}
			continue
		}
		nativeStream = append(nativeStream, runnerIdentityKindPhase{Kind: event.Kind, Phase: event.Phase})
	}
	if progressCount != 1 {
		t.Fatalf(
			"captured Response Events = %#v, want exactly 1 PROGRESS event delivered through "+
				"the session progress publisher supplied to this Workers construction, got %d",
			events,
			progressCount,
		)
	}
	if len(nativeStream) != len(runnerIdentityExpectedNativeStreamEvents) {
		t.Fatalf(
			"non-PROGRESS Response Events = %#v, want exactly %d native-stream events",
			nativeStream,
			len(runnerIdentityExpectedNativeStreamEvents),
		)
	}
	for i, want := range runnerIdentityExpectedNativeStreamEvents {
		if nativeStream[i] != want {
			t.Fatalf(
				"non-PROGRESS Response Events at ordered index %d = %+v, want %+v (full ordered sequence = %#v)",
				i,
				nativeStream[i],
				want,
				events,
			)
		}
	}
}

// assertRunnerIdentityDispatchSequence proves the relevant terminal dispatch
// sequence for one work type: the DISPATCH_REQUEST Factory Event for its
// dispatch precedes the matching DISPATCH_RESPONSE in the canonical event
// ledger (not merely that a terminal outcome was eventually counted), and
// that response carries the expected outcome.
func assertRunnerIdentityDispatchSequence(
	t *testing.T,
	factoryEvents []factoryapi.FactoryEvent,
	listed factoryapi.ListWorkResponse,
	workType string,
	wantOutcome factoryapi.WorkOutcome,
) {
	t.Helper()

	workID := runnerIdentityWorkIDForType(t, listed, workType)

	var dispatchID string
	for _, observation := range support.ObserveDispatchEvents(t, factoryEvents) {
		if support.DispatchObservationIncludesWork(observation, workID) {
			dispatchID = observation.DispatchID
			break
		}
	}
	if dispatchID == "" {
		t.Fatalf("%s: no dispatch observation includes work %q", workType, workID)
	}

	requestIndex, responseIndex := -1, -1
	var responseOutcome factoryapi.WorkOutcome
	for i, event := range factoryEvents {
		if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if requestIndex == -1 {
				requestIndex = i
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if responseIndex == -1 {
				responseIndex = i
				response, err := event.Payload.AsDispatchResponseEventPayload()
				if err != nil {
					t.Fatalf("%s: decode DISPATCH_RESPONSE: %v", workType, err)
				}
				responseOutcome = response.Outcome
			}
		}
	}
	if requestIndex == -1 || responseIndex == -1 {
		t.Fatalf(
			"%s: dispatch %q missing DISPATCH_REQUEST (index %d) or DISPATCH_RESPONSE (index %d) event",
			workType,
			dispatchID,
			requestIndex,
			responseIndex,
		)
	}
	if requestIndex >= responseIndex {
		t.Fatalf(
			"%s: DISPATCH_REQUEST at Factory Event index %d did not precede DISPATCH_RESPONSE at index %d",
			workType,
			requestIndex,
			responseIndex,
		)
	}
	if responseOutcome != wantOutcome {
		t.Fatalf("%s: dispatch outcome = %q, want %q", workType, responseOutcome, wantOutcome)
	}
}

func runnerIdentityWorkIDForType(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType string,
) string {
	t.Helper()

	for _, item := range listed.Results {
		if item.WorkTypeName != nil && *item.WorkTypeName == workType && item.WorkId != nil {
			return *item.WorkId
		}
	}
	t.Fatalf("no listed Work item found for work type %q", workType)
	return ""
}
