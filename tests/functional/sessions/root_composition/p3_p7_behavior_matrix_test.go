package root_composition_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	p3p7CorpusWorkerOutput  = "p3-p7 canonical corpus COMPLETE"
	p3p7CorpusTerminalLimit = 60 * time.Second
	p3p7CorpusReleaseLimit  = 20 * time.Second
	p3p7CorpusCloseTimeout  = 15 * time.Second
	p3p7CorpusEntryLimit    = 45 * time.Second
)

// TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation is the
// cross-packet behavior corpus for the build-time/runtime composition program.
// Every operation it drives enters through root.BuildProcess and
// Process.Execute, with external effects replaced only through edges.Edges, so
// what it observes is the canonical composition graph P3-P6 converged on rather
// than a test-only assembly.
//
// It closes four claims that P3-P6 depend on but that no single owner test
// covers together:
//
//   - one terminal outcome: a canonical run reaches exactly one terminal (or
//     failed) Work state, and the retained canonical stream enters that state
//     exactly once;
//   - post-activation cleanup: the transport resource acquired during
//     activation is released exactly once when the process shuts down, a
//     repeated Close never releases it twice, and the released listener really
//     stops serving;
//   - cancellation and failure: a provider failure terminalizes as FAILED and
//     is never reported as success, and a dispatch held inside the provider
//     boundary observes cancellation when the process is shut down instead of
//     being left running;
//   - replay isolation: two Factory Sessions built from the same authored
//     Factory in two independent root processes replay to the same canonical
//     facts while sharing no session or Work identity.
func TestP3P7CanonicalPathPreservesTerminalCleanupAndReplayIsolation(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	t.Run("isolated sessions reach one terminal outcome and replay equivalent facts", func(t *testing.T) {
		first := runP3P7CanonicalCorpus(t, "alpha", support.NewStaticSuccessCommandRunner(p3p7CorpusWorkerOutput))
		second := runP3P7CanonicalCorpus(t, "beta", support.NewStaticSuccessCommandRunner(p3p7CorpusWorkerOutput))

		assertP3P7SingleTerminalOutcome(t, first, factoryapi.WorkStateTypeTERMINAL)
		assertP3P7SingleTerminalOutcome(t, second, factoryapi.WorkStateTypeTERMINAL)
		assertP3P7SessionsAreIsolated(t, first, second)
		assertP3P7ReplayedFactsAreEquivalent(t, first, second)
	})

	t.Run("provider failure terminalizes as failed and is never reported as success", func(t *testing.T) {
		run := runP3P7CanonicalCorpus(t, "provider-failure", support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte("provider refused the canonical corpus dispatch"),
			},
		))

		assertP3P7SingleTerminalOutcome(t, run, factoryapi.WorkStateTypeFAILED)
	})

	t.Run("cancellation stops the held dispatch and releases the activated process", func(t *testing.T) {
		held := &p3p7HeldCommandRunner{entered: make(chan struct{})}
		running := startP3P7CanonicalProcess(t, "cancellation", held)

		select {
		case <-held.entered:
		case <-time.After(p3p7CorpusEntryLimit):
			t.Fatalf("provider boundary was never reached within %s, so cancellation would prove nothing", p3p7CorpusEntryLimit)
		}

		status := support.WaitForStatus(t, running.baseURL, p3p7CorpusTerminalLimit, func(factoryapi.StatusResponse) bool {
			return true
		})
		if status.Categories.Terminal != 0 {
			t.Fatalf("status categories while the dispatch is held = %+v, want zero terminal outcomes", status.Categories)
		}

		running.shutdownAndAssertCleanupHappensExactlyOnce(t)

		if !held.observedCancellation() {
			t.Fatal("held provider dispatch never observed cancellation; process shutdown left in-flight work running")
		}
		if err := running.daemon.Err(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Process.Execute() after cancellation error = %v, want a cancellation classification", err)
		}
	})
}

// p3p7CorpusRun is everything one completed canonical run makes observable at
// the public boundary, captured before the process is shut down.
type p3p7CorpusRun struct {
	label     string
	sessionID string
	workID    string
	status    factoryapi.StatusResponse
	work      factoryapi.Work
	events    []factoryapi.FactoryEvent
}

// runP3P7CanonicalCorpus drives one full canonical lifecycle: activation, Work
// admission from the customer `--work` file, dispatch through the replaced
// provider boundary, terminal observation, then shutdown with the cleanup
// assertions.
func runP3P7CanonicalCorpus(
	t *testing.T,
	label string,
	runner platformprocess.CommandRunner,
) p3p7CorpusRun {
	t.Helper()

	running := startP3P7CanonicalProcess(t, label, runner)
	status := support.WaitForSessionTerminalStatus(t, running.baseURL, running.sessionID, p3p7CorpusTerminalLimit)

	listed := support.ListDefaultSessionWork(t, running.baseURL)
	if len(listed.Results) != 1 {
		t.Fatalf("%s: listed work = %d, want exactly one submitted work item", label, len(listed.Results))
	}
	admitted := listed.Results[0]
	workID := support.StringPointerValue(admitted.WorkId)
	if workID == "" {
		t.Fatalf("%s: listed work has no public identity: %#v", label, admitted)
	}
	if admitted.State == nil {
		t.Fatalf("%s: terminal work has no served state: %#v", label, admitted)
	}
	events := waitForP3P7RetainedTerminalDispatch(t, running, workID)

	running.shutdownAndAssertCleanupHappensExactlyOnce(t)

	return p3p7CorpusRun{
		label:     label,
		sessionID: running.sessionID,
		workID:    workID,
		status:    status,
		work:      admitted,
		events:    events,
	}
}

// waitForP3P7RetainedTerminalDispatch returns the retained canonical history
// once it carries the workstation outcome for one Work identity. The Work
// projection and the canonical stream are separate reads, so the corpus waits
// for the canonical fact itself instead of assuming a terminal projection read
// implies the outcome is already committed to history. The ceiling is a failure
// guard, not synchronization: the loop returns on the first read that carries
// the outcome.
func waitForP3P7RetainedTerminalDispatch(
	t *testing.T,
	running *p3p7CanonicalProcess,
	workID string,
) []factoryapi.FactoryEvent {
	t.Helper()

	deadline := time.Now().Add(p3p7CorpusTerminalLimit)
	for {
		events := support.GetFactoryEventsForSessionAt(t, running.baseURL, running.sessionID)
		if len(p3p7DispatchOutcomes(t, running.label, events, workID)) > 0 {
			return events
		}
		if !time.Now().Before(deadline) {
			t.Fatalf(
				"%s: retained canonical history never recorded a workstation outcome for work %q within %s",
				running.label,
				workID,
				p3p7CorpusTerminalLimit,
			)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// p3p7DispatchOutcomes derives, from the retained canonical stream alone, the
// ordered workstation outcomes recorded for one Work identity. This is the
// replay side of the corpus: world state read back from canonical facts,
// independent of the served Work projection.
func p3p7DispatchOutcomes(
	t *testing.T,
	label string,
	events []factoryapi.FactoryEvent,
	workID string,
) []factoryapi.WorkOutcome {
	t.Helper()

	var outcomes []factoryapi.WorkOutcome
	for index, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if !slices.Contains(p3p7EventWorkIDs(event), workID) {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("%s: decode DISPATCH_RESPONSE payload at event %d: %v", label, index, err)
		}
		outcomes = append(outcomes, payload.Outcome)
	}
	return outcomes
}

// p3p7CanonicalProcess is a live root-built process plus the two effect
// recorders the cleanup assertions read.
type p3p7CanonicalProcess struct {
	label       string
	process     support.Process
	daemon      *support.ProcessCommand
	baseURL     string
	sessionID   string
	transport   *p3p7TransportRecorder
	activations *atomic.Int32
}

func startP3P7CanonicalProcess(
	t *testing.T,
	label string,
	runner platformprocess.CommandRunner,
) *p3p7CanonicalProcess {
	t.Helper()

	dir := support.ScaffoldFactory(t, p3p7CorpusFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"worker-a",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	workFile := filepath.Join(dir, "corpus-work.json")
	support.WriteWorkRequestFile(t, workFile, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title":"p3-p7 canonical corpus"}`),
	})

	transport := &p3p7TransportRecorder{server: support.NewProcessAPIServer(), release: make(chan struct{})}
	activations := &atomic.Int32{}
	edges := serviceedges.Edges{
		APIServerStarter: transport.start,
		// Runtime instance identity is minted while a Factory Session runtime is
		// being opened, so a non-zero count is direct evidence that activation
		// happened and the cleanup assertions below are not vacuous.
		FactorySessionRuntimeInstanceIDGenerator: func() string {
			return fmt.Sprintf("p3p7-corpus-%s-runtime-%d", label, activations.Add(1))
		},
	}
	support.ConfigureWorkerCommands(t, &edges, runner, nil)

	process := support.BuildProcess(t, edges)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
		"--work", workFile,
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("%s daemon stderr:\n%s", label, stderr)
		}
	})

	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := transport.server.WaitForURL(t)
	return &p3p7CanonicalProcess{
		label:       label,
		process:     process,
		daemon:      daemon,
		baseURL:     baseURL,
		sessionID:   support.GetDefaultSession(t, baseURL).Id,
		transport:   transport,
		activations: activations,
	}
}

// shutdownAndAssertCleanupHappensExactlyOnce proves the post-activation half of
// the corpus: the one external transport resource this process acquired during
// activation is released exactly once by shutdown, a repeated Close neither
// fails nor releases it again, and the released listener genuinely stops
// serving.
func (running *p3p7CanonicalProcess) shutdownAndAssertCleanupHappensExactlyOnce(t *testing.T) {
	t.Helper()

	if got := running.transport.started.Load(); got != 1 {
		t.Fatalf("%s: API transport activations = %d, want exactly one", running.label, got)
	}
	if got := running.activations.Load(); got < 1 {
		t.Fatalf(
			"%s: Factory Session runtime activations = %d, want at least one so the cleanup claim is not vacuous",
			running.label,
			got,
		)
	}

	running.daemon.Stop(t)
	closer, ok := running.process.(interface{ Close(context.Context) error })
	if !ok {
		t.Fatalf("%s: root-built process does not expose Close", running.label)
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), p3p7CorpusCloseTimeout)
	defer cancel()
	if err := closer.Close(closeCtx); err != nil {
		t.Fatalf("%s: Process.Close() after activation error = %v, want nil", running.label, err)
	}

	select {
	case <-running.transport.release:
	case <-time.After(p3p7CorpusReleaseLimit):
		t.Fatalf("%s: activated API transport was never released after process shutdown", running.label)
	}
	if got := running.transport.released.Load(); got != 1 {
		t.Fatalf("%s: API transport releases = %d, want exactly one", running.label, got)
	}
	if err := closer.Close(closeCtx); err != nil {
		t.Fatalf("%s: repeated Process.Close() error = %v, want nil", running.label, err)
	}
	if got := running.transport.released.Load(); got != 1 {
		t.Fatalf(
			"%s: API transport releases after a repeated close = %d, want exactly one",
			running.label,
			got,
		)
	}
	assertP3P7EndpointStoppedServing(t, running.label, running.baseURL)
}

func assertP3P7EndpointStoppedServing(t *testing.T, label, baseURL string) {
	t.Helper()

	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   p3p7CorpusCloseTimeout,
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("%s: build released-listener probe: %v", label, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return
	}
	defer response.Body.Close()
	t.Fatalf(
		"%s: GET %s after process close status = %d, want the released listener to refuse the connection",
		label,
		endpoint,
		response.StatusCode,
	)
}

// assertP3P7SingleTerminalOutcome requires exactly one terminal outcome of the
// expected classification, and requires the canonical event stream replayed on
// its own to agree with the served projection about that outcome.
func assertP3P7SingleTerminalOutcome(t *testing.T, run p3p7CorpusRun, want factoryapi.WorkStateType) {
	t.Helper()

	categories := run.status.Categories
	switch want {
	case factoryapi.WorkStateTypeTERMINAL:
		if categories.Terminal != 1 || categories.Failed != 0 {
			t.Fatalf("%s: status categories = %+v, want exactly one terminal and zero failed", run.label, categories)
		}
	case factoryapi.WorkStateTypeFAILED:
		if categories.Failed != 1 || categories.Terminal != 0 {
			t.Fatalf("%s: status categories = %+v, want exactly one failed and zero terminal", run.label, categories)
		}
	default:
		t.Fatalf("%s: unsupported terminal expectation %q", run.label, want)
	}
	if run.work.State == nil || run.work.State.Type != want {
		t.Fatalf("%s: terminal work state = %#v, want state type %q", run.label, run.work.State, want)
	}

	outcomes := p3p7DispatchOutcomes(t, run.label, run.events, run.workID)
	if len(outcomes) != 1 {
		t.Fatalf(
			"%s: replayed workstation outcomes for work %q = %v, want exactly one terminal outcome",
			run.label,
			run.workID,
			outcomes,
		)
	}
	if got := p3p7StateTypeForOutcome(outcomes[0]); got != want {
		t.Fatalf(
			"%s: replayed workstation outcome %q classifies as %q but the served projection reports %q; the canonical stream and the projection disagree",
			run.label,
			outcomes[0],
			got,
			want,
		)
	}
}

func p3p7StateTypeForOutcome(outcome factoryapi.WorkOutcome) factoryapi.WorkStateType {
	switch outcome {
	case factoryapi.WorkOutcomeAccepted:
		return factoryapi.WorkStateTypeTERMINAL
	case factoryapi.WorkOutcomeFailed, factoryapi.WorkOutcomeRejected:
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

// assertP3P7SessionsAreIsolated requires two independently built root processes
// to have produced two disjoint canonical histories.
func assertP3P7SessionsAreIsolated(t *testing.T, first, second p3p7CorpusRun) {
	t.Helper()

	if first.sessionID == "" || first.sessionID == second.sessionID {
		t.Fatalf(
			"canonical session identities = %q and %q, want two distinct Factory Sessions",
			first.sessionID,
			second.sessionID,
		)
	}
	if first.workID == second.workID {
		t.Fatalf("canonical work identities = %q and %q, want two distinct Work items", first.workID, second.workID)
	}
	assertP3P7EventsStayWithinTheirSession(t, first, second)
	assertP3P7EventsStayWithinTheirSession(t, second, first)
}

func assertP3P7EventsStayWithinTheirSession(t *testing.T, run, other p3p7CorpusRun) {
	t.Helper()

	if len(run.events) == 0 {
		t.Fatalf("%s: retained canonical events = 0, want an observable history", run.label)
	}
	previousSequence := 0
	for index, event := range run.events {
		session := event.Context.SessionId
		// Session-scoped canonical events carry the run's own canonical identity;
		// the customer-facing default alias is the one accepted alternative and is
		// resolved per process, so it can never name another process's session.
		if session != nil && *session != run.sessionID && *session != factorysessions.DefaultSessionID {
			t.Fatalf(
				"%s: event %d carries session %q, want its own session %q or the default alias",
				run.label,
				index,
				*session,
				run.sessionID,
			)
		}
		if slices.Contains(p3p7EventWorkIDs(event), other.workID) {
			t.Fatalf("%s: event %d references the other session's work %q", run.label, index, other.workID)
		}
		sequence := event.Context.SessionSequence
		if session == nil || *session != run.sessionID || sequence == nil {
			continue
		}
		if *sequence <= previousSequence {
			t.Fatalf(
				"%s: session sequence %d at event %d does not advance past %d; replay deduplication would be ambiguous",
				run.label,
				*sequence,
				index,
				previousSequence,
			)
		}
		previousSequence = *sequence
	}
}

// assertP3P7ReplayedFactsAreEquivalent proves the two isolated sessions
// recorded the same canonical facts under different identities: the same
// authored Factory replays to the same ordered workstation outcomes and the
// same terminal state, and the canonical vocabulary correlated with each
// session's own Work is identical.
func assertP3P7ReplayedFactsAreEquivalent(t *testing.T, first, second p3p7CorpusRun) {
	t.Helper()

	firstOutcomes := p3p7DispatchOutcomes(t, first.label, first.events, first.workID)
	secondOutcomes := p3p7DispatchOutcomes(t, second.label, second.events, second.workID)
	if !slices.Equal(firstOutcomes, secondOutcomes) {
		t.Fatalf(
			"replayed workstation outcomes = %v and %v, want equivalent facts from two isolated sessions of one authored Factory",
			firstOutcomes,
			secondOutcomes,
		)
	}
	if first.work.State.Name != second.work.State.Name {
		t.Fatalf(
			"terminal states = %q and %q, want one authored Factory to terminalize the same way in both isolated sessions",
			first.work.State.Name,
			second.work.State.Name,
		)
	}

	firstVocabulary := p3p7WorkEventVocabulary(first)
	secondVocabulary := p3p7WorkEventVocabulary(second)
	if len(firstVocabulary) == 0 {
		t.Fatalf("%s: no canonical event is correlated with work %q", first.label, first.workID)
	}
	if !maps.Equal(firstVocabulary, secondVocabulary) {
		t.Fatalf(
			"work-correlated canonical vocabularies = %v and %v, want the same canonical facts in both isolated sessions",
			slices.Sorted(maps.Keys(firstVocabulary)),
			slices.Sorted(maps.Keys(secondVocabulary)),
		)
	}
}

// p3p7WorkEventVocabulary is the set of canonical event types correlated with
// one run's own Work identity. Identity-free by construction, so two isolated
// sessions of the same authored Factory must produce the same set.
func p3p7WorkEventVocabulary(run p3p7CorpusRun) map[factoryapi.FactoryEventType]struct{} {
	vocabulary := make(map[factoryapi.FactoryEventType]struct{})
	for _, event := range run.events {
		if slices.Contains(p3p7EventWorkIDs(event), run.workID) {
			vocabulary[event.Type] = struct{}{}
		}
	}
	return vocabulary
}

func p3p7EventWorkIDs(event factoryapi.FactoryEvent) []string {
	if event.Context.WorkIds == nil {
		return nil
	}
	return *event.Context.WorkIds
}

// p3p7TransportRecorder wraps the process API-server edge so the corpus can
// observe activation and release of the one external transport resource the
// canonical process acquires.
type p3p7TransportRecorder struct {
	server   *support.ProcessAPIServer
	started  atomic.Int32
	released atomic.Int32
	release  chan struct{}
	closeIt  sync.Once
}

func (recorder *p3p7TransportRecorder) start(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	recorder.started.Add(1)
	err := recorder.server.Start(ctx, request)
	recorder.released.Add(1)
	recorder.closeIt.Do(func() { close(recorder.release) })
	return err
}

// p3p7HeldCommandRunner parks the provider boundary until the invocation is
// cancelled, then records that cancellation is what released it.
type p3p7HeldCommandRunner struct {
	entered   chan struct{}
	enterOnce sync.Once
	cancelled atomic.Bool
}

func (runner *p3p7HeldCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.enterOnce.Do(func() { close(runner.entered) })
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.Canceled) {
		runner.cancelled.Store(true)
	}
	return platformprocess.CommandResult{}, ctx.Err()
}

func (runner *p3p7HeldCommandRunner) observedCancellation() bool {
	return runner.cancelled.Load()
}

func p3p7CorpusFactoryConfig() map[string]any {
	return map[string]any{
		"name": "p3-p7-canonical-corpus",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
			"handlingBehavior": []string{"DEFAULT"},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

var _ platformprocess.CommandRunner = (*p3p7HeldCommandRunner)(nil)
var _ platformhttpserver.Starter = (&p3p7TransportRecorder{}).start
