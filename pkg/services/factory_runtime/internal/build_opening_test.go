package internal_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestBundleOpeningInvokesSelectedResourceOperationAndRetainsPartialFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := clockwork.NewFakeClock()
	logger := zap.NewNop()
	spec := factory.SessionBuildSpec{Dir: dir, FolderPath: dir, SessionID: "candidate",
		RuntimeInstanceID: "candidate-runtime", LoadedFactoryCfg: loaded, Clock: clock, BaseLogger: logger}
	openingErr := errors.New("selected resource opening failed")
	partial := &factoryhost.Bundle{FactorySessionID: spec.SessionID, RuntimeInstanceID: spec.RuntimeInstanceID}
	calls := 0
	selected := func(
		ctx context.Context,
		baseLogger *zap.Logger,
		_, _ string,
		sessionID string,
		_, _ string,
		_ interfaces.RuntimeMode,
		_ bool,
		_ factory.Scheduler,
		_ bool,
		_ string,
		_ factory.RuntimeLogStorageConfig,
		_ factory.RuntimeFileLoggingPolicy,
		_ factory.RuntimeMetricsPolicy,
		_ string,
		_ factory.RuntimeMetricsStorageConfig,
		_ factory.LoadedConfig,
		runtimeInstanceID string,
		_ string,
		clock factory.Clock,
		_ string,
		_ *interfaces.FactorySnapshot,
		_ *interfaces.FactoryWorldState,
		_ bool,
		_ []factory.SubmissionHook,
		_ factory.CompletionDeliveryPlanner,
		_ factory.PetriMutationRecorder,
		_ time.Duration,
		_ []interfaces.FactoryEvent,
		_ workers.Service,
		_ workersessions.Service,
		_ factory.WorkerAttemptOpener,
		_ func(string),
		_ ...*workers.MockWorkersConfig,
	) (*factoryhost.Bundle, error) {
		calls++
		if ctx != t.Context() || baseLogger != logger || clock != spec.Clock || sessionID != spec.SessionID || runtimeInstanceID != spec.RuntimeInstanceID {
			t.Fatal("resource opening substituted admitted context, logger, clock or identity")
		}
		return partial, openingErr
	}
	sessions := &stubWorkerSessionsService{}
	opening, err := factoryinternal.NewBundleOpening(selected, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, &testRuntimeScopeServiceStub{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("constructor executed resource opening")
	}
	result, err := openTestBundle(t.Context(), opening.Open, spec)
	if !errors.Is(err, openingErr) || result != partial || calls != 1 {
		t.Fatalf("opening = %p, %v, calls %d; want selected partial record %p and failure", result, err, calls, partial)
	}
	if result.RuntimeService() != nil {
		t.Fatal("partial failure published a runnable service")
	}
}

// The controlled assembly retains callbacks just as an opened engine does. Emit
// after both admissions, so a reusable owner's latest-session substitution
// would route the first session's observations into its peer.
func TestInitialActivationKeepsMutationAndProgressObservationsScopedAcrossCalls(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	definitionDir := filepath.Join(dir, "factory")
	loaded, err := factorydefinitionfixtures.NewLoadedSource(definitionDir, &interfaces.FactoryConfig{Name: "nested"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resources := &controlledInitialAssembly{failure: errors.New("stop at controlled assembly boundary")}
	observations := [2]*openingSessionObservations{{}, {}}
	clock := clockwork.NewFakeClock()
	initial := factoryinternal.NewInitialActivation(resources, clock, zap.NewNop(),
		func(*interfaces.RuntimeSnapshot, string) (interfaces.MutableLoadedFactorySource, error) {
			return loaded, nil
		})
	for index, sessionID := range []string{"candidate", "peer"} {
		result, openErr := initial.Open(t.Context(), factory.RuntimeActivationRequest{
			FactorySessionID: sessionID, RuntimeID: "runtime-" + sessionID,
			Snapshot: interfaces.RuntimeSnapshot{FactoryDir: definitionDir},
			Runtime: factory.RuntimeSelection{Mode: interfaces.RuntimeModeBatch,
				FileLoggingPolicy: factory.RuntimeFileLoggingPolicyDisabled, MetricsPolicy: factory.RuntimeMetricsPolicyDisabled},
			Inputs: factory.RuntimeActivationInputs{Definition: factory.RuntimeActivationDefinitionInputs{Directory: dir, ExecutionBaseDir: dir},
				Recordings: factory.RuntimeActivationRecordingInputs{RecordPath: "recording.json"}},
		}, observations[index])
		assertUnpublishedOpeningFailure(t, result, openErr, resources.failure)
		if resources.folderPaths[index] != dir || resources.definitionDirs[index] != definitionDir {
			t.Fatalf("opening workspace/definition = %q/%q, want %q/%q", resources.folderPaths[index], resources.definitionDirs[index], dir, definitionDir)
		}
	}
	for index, sessionID := range []string{"candidate", "peer"} {
		mutation := interfaces.TokenMutationRecord{}
		if err := resources.mutations[index](sessionID, []interfaces.TokenMutationRecord{mutation}); err != nil {
			t.Fatal(err)
		}
		resources.progress[index](workers.ProgressFragment{Kind: workers.ProgressFragmentKind, Payload: sessionID})
		if index == 0 && (len(observations[1].sessions) != 0 || len(observations[1].progress) != 0) {
			t.Fatal("candidate callback reached the peer's observation owner")
		}
	}
	for index, sessionID := range []string{"candidate", "peer"} {
		assertScopedOpeningObservations(t, observations[index], sessionID)
	}
	if !reflect.DeepEqual(resources.runtimeIDs, []string{"runtime-candidate", "runtime-peer"}) {
		t.Fatalf("progress supervision identities = %v; want each admitted runtime", resources.runtimeIDs)
	}
	// Scoped callbacks may retain observations while their opening is owned.
	// Once those callbacks are discarded, a live reusable owner must not keep
	// the durable observation capability reachable.
	first, second := weak.Make(observations[0]), weak.Make(observations[1])
	resources.mutations, resources.progress = nil, nil
	runtime.GC()
	if first.Value() != nil || second.Value() != nil {
		t.Fatal("reusable initial operation retained discarded session observations")
	}
	runtime.KeepAlive(initial)
}

func TestInitialActivationKeepsMutationAndProgressObservationsScopedAcrossOutcomes(t *testing.T) {
	t.Parallel()
	loaded, err := factorydefinitionfixtures.NewLoadedSource("factory", &interfaces.FactoryConfig{Name: "factory"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resources := &controlledInitialAssembly{failure: errors.New("controlled opening failure")}
	var cancelDuringMaterialization context.CancelFunc
	initial := factoryinternal.NewInitialActivation(resources, clockwork.NewFakeClock(), zap.NewNop(),
		func(*interfaces.RuntimeSnapshot, string) (interfaces.MutableLoadedFactorySource, error) {
			if cancelDuringMaterialization != nil {
				cancelDuringMaterialization()
			}
			return loaded, nil
		})
	for _, outcome := range []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"failure", resources.failure},
		{"cancellation", context.Canceled},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			resources.failure = outcome.err
			ctx := t.Context()
			if errors.Is(outcome.err, context.Canceled) {
				canceled, cancel := context.WithCancel(ctx)
				cancelDuringMaterialization = cancel
				t.Cleanup(cancel)
				ctx = canceled
			}
			observation := discardedInitialOpeningObservation(t, initial, ctx, outcome.err)
			// Drop the fake engine's scoped callbacks and the result while the
			// same reusable operation remains live across every outcome.
			resources.mutations, resources.progress = nil, nil
			runtime.GC()
			if observation.Value() != nil {
				t.Fatal("reusable initial operation retained discarded session observations")
			}
			runtime.KeepAlive(initial)
		})
	}
}

func discardedInitialOpeningObservation(t *testing.T, initial *factoryinternal.InitialActivation,
	ctx context.Context, cause error,
) weak.Pointer[openingSessionObservations] {
	t.Helper()
	observations := &openingSessionObservations{}
	result, err := initial.Open(ctx, factory.RuntimeActivationRequest{
		FactorySessionID: "discarded", RuntimeID: "discarded-runtime",
	}, observations)
	if !errors.Is(err, cause) {
		t.Fatalf("initial opening error = %v, want %v", err, cause)
	}
	if errors.Is(cause, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("cancellation case did not cancel its opening context")
	}
	if cause == nil && (result == nil || result.Activation == nil) {
		t.Fatal("successful controlled opening did not return its activation")
	}
	return weak.Make(observations)
}

func TestAssemblyUsesFixedExecutionAndRecordingEffectsForInitialAndReplacement(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	resources := &observationResourceOpening{failure: errors.New("controlled resource failure")}
	sessions := &observationWorkerSessions{}
	provider := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("selected provider")})
	script := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("selected script")})
	provider.Queue(platformprocess.CommandResult{Stdout: []byte("selected provider")})
	script.Queue(platformprocess.CommandResult{Stdout: []byte("selected script")})
	mock := &factory.RuntimeActivationMockWorkersConfig{}
	var selectedMock *workers.MockWorkersConfig
	decorations := 0
	decorate := func(selected *workers.MockWorkersConfig, _ interfaces.RuntimeDefinitionLookup, next platformprocess.CommandRunner) platformprocess.CommandRunner {
		if selected == nil || (selectedMock != nil && selected != selectedMock) {
			t.Fatal("preparation changed the selected mock configuration")
		}
		selectedMock = selected
		output := "selected script"
		if next == provider {
			output = "selected provider"
		}
		result, runErr := next.Run(t.Context(), platformprocess.CommandRequest{Command: "controlled"})
		if runErr != nil || string(result.Stdout) != output {
			t.Fatalf("selected command output = %q, %v; want %q", result.Stdout, runErr, output)
		}
		decorations++
		return next
	}
	var effects []string
	loader := func(path string, _ interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		return loadedFactoryFixture(path)
	}
	scopes := &testRuntimeScopeServiceStub{ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "selected-recording"}}
	var snapshotSources []interfaces.LoadedFactorySource
	snapshot := &interfaces.FactorySnapshot{}
	capture := captureSelectedOpeningSnapshot(&snapshotSources, snapshot)
	opening, err := factoryinternal.NewBundleOpening(resources.Open, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, scopes, capture)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, nil, nil, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, provider, script, decorate), &testRuntimeScopeServiceStub{}, nil, fixedTestProgress(t, &effects), fixedTestCompletion(&effects), nil)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := assembleTestInitialOpening(t.Context(), assembly, dir, loaded, clockwork.NewFakeClock(),
		factory.RuntimeActivationRequest{FactorySessionID: "candidate", RuntimeID: "initial",
			Inputs: factory.RuntimeActivationInputs{Workers: factory.RuntimeActivationWorkerInputs{MockWorkers: mock}}})
	assertUnpublishedOpeningFailure(t, initial, err, resources.failure)
	if _, err := initial.ReplacementBuilder.BuildReplacement(t.Context(), dir, dir, "successor", dir); !errors.Is(err, resources.failure) {
		t.Fatalf("replacement error = %v, want controlled resource failure", err)
	}
	assertSelectedOpeningSnapshots(t, resources, snapshot, snapshotSources)
	for index := range resources.progress {
		resources.progress[index](workers.ProgressFragment{Payload: "output"})
		resources.completions[index]("dispatch-id")
	}
	if decorations != 4 || !reflect.DeepEqual(effects, []string{"candidate:output", "candidate:dispatch-id", "successor:output", "successor:dispatch-id"}) {
		t.Fatalf("fixed effects = %v, decorations = %d; want both openings", effects, decorations)
	}
}

// Build owns the selected effect handoff. Engine execution is controlled here;
// composed Work/dispatch/history behavior is covered through public sessions.
func TestBuildUsesFixedRecordingAndProjectionForIndependentOpenings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	var submissions []work.FactorySubmissionRecord
	var dispatches []recordings.FactoryDispatchRecord
	var projected []string
	var candidateRequest, peerRequest recordings.RuntimeScopeRequest
	scopes := &testRuntimeScopeServiceStub{bySession: map[string]*testRuntimeScopeServiceStub{
		"candidate": {capturedRequest: &candidateRequest, ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "candidate", Events: []interfaces.FactoryEvent{{Id: "candidate"}}}},
		"peer":      {capturedRequest: &peerRequest, ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "peer", Events: []interfaces.FactoryEvent{{Id: "peer"}}}},
	}}
	opening := &controlledEngineOpening{}
	owner := testRuntimeFactoryWithEffectsAndOpening(nil, nil,
		func(record work.FactorySubmissionRecord) { submissions = append(submissions, record) },
		func(record recordings.FactoryDispatchRecord) { dispatches = append(dispatches, record) },
		func(events []interfaces.FactoryEvent, tick int) (interfaces.FactoryWorldState, error) {
			if len(events) != 1 {
				t.Fatalf("projection events = %#v", events)
			}
			projected = append(projected, events[0].Id)
			return interfaces.FactoryWorldState{Tick: tick}, nil
		}, scopes, opening,
	)
	for _, sessionID := range []string{"candidate", "peer"} {
		bundle := openFixedEffectsBundle(t, owner, dir, sessionID)
		if bundle.RecordingLedger() != scopes.bySession[sessionID].ledger {
			t.Fatal("opening substituted its selected recording ledger")
		}
	}
	assertFixedRecordingSelections(t, candidateRequest, peerRequest)
	for index, opened := range opening.calls {
		name := []string{"candidate", "peer"}[index]
		opened.submit(work.FactorySubmissionRecord{Request: work.SubmitRequest{WorkID: "work-" + name}})
		opened.dispatch(recordings.FactoryDispatchRecord{Dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{RequestID: "request-" + name}}})
		if _, err := opened.project(opened.history.CanonicalEvents(), index); err != nil {
			t.Fatal(err)
		}
	}

	if len(submissions) != 2 || len(dispatches) != 2 {
		t.Fatalf("fixed recording calls = %d submissions/%d dispatches; want two each", len(submissions), len(dispatches))
	}
	if submissions[0].Request.WorkID != "work-candidate" || submissions[1].Request.WorkID != "work-peer" {
		t.Fatalf("submission attribution = %#v", submissions)
	}
	if dispatches[0].Dispatch.Execution.RequestID != "request-candidate" || dispatches[1].Dispatch.Execution.RequestID != "request-peer" {
		t.Fatalf("dispatch attribution = %#v", dispatches)
	}
	if !reflect.DeepEqual(projected, []string{"candidate", "peer"}) {
		t.Fatalf("projection history = %v; want independently addressed histories", projected)
	}
}

// Port the former flush-wrapper assertions to the actual resource owner.
// A later opening must not retarget the earlier request, and the resume prefix
// must remain detached after the caller changes its input.
func assertFixedRecordingSelections(t *testing.T, candidate, peer recordings.RuntimeScopeRequest) {
	t.Helper()
	if candidate.FactorySessionID != "candidate" || peer.FactorySessionID != "peer" || candidate.FlushInterval != time.Second || peer.FlushInterval != 2*time.Second {
		t.Fatalf("recording selections = %#v / %#v; want independently addressed intervals", candidate, peer)
	}
	if len(candidate.ReplayEvents) != 1 || candidate.ReplayEvents[0].Id != "resume-event" || len(peer.ReplayEvents) != 0 {
		t.Fatalf("resume prefixes = %#v / %#v; want detached resume event and unseeded ordinary opening", candidate.ReplayEvents, peer.ReplayEvents)
	}
}

func openFixedEffectsBundle(t *testing.T, owner *factoryinternal.RuntimeFactory, dir, sessionID string) *factoryhost.Bundle {
	t.Helper()
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &fixedEffectsSessions{stubWorkerSessionsService: &stubWorkerSessionsService{}}
	flushInterval := time.Second
	var resumeEvents []interfaces.FactoryEvent
	if sessionID == "candidate" {
		resumeEvents = []interfaces.FactoryEvent{{Id: "resume-event"}}
	} else {
		flushInterval = 2 * time.Second
	}
	bundle, err := owner.Build(t.Context(), zap.NewNop(), dir, dir, sessionID, sessionID,
		"", interfaces.RuntimeModeBatch, false, nil, true,
		"", factory.RuntimeLogStorageConfig{}, factory.RuntimeFileLoggingPolicyDisabled,
		factory.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-"+sessionID, "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil,
		flushInterval, resumeEvents, fixedEffectsWorker{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumeEvents) > 0 {
		resumeEvents[0].Id = "caller-changed"
	}
	t.Cleanup(func() {
		if err := bundle.CloseArtifacts(); err != nil {
			t.Error(err)
		}
	})
	return bundle
}

type fixedEffectsWorker struct{ workers.Service }

func (fixedEffectsWorker) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
}

type fixedEffectsSessions struct{ *stubWorkerSessionsService }

func (*fixedEffectsSessions) BeginRuntimeAttempt(_ context.Context, _ workersessions.RuntimeAttemptRequest,
	_ workers.Service, _ platformclock.Source, _ platformclock.TimerSource,
	_ func(context.Context) (workers.WorkstationDispatchCancelOutcome, error),
) (workersessions.RuntimeAttempt, error) {
	return workersessions.RuntimeAttempt(func(_ context.Context, result workers.WorkstationDispatchResult, _ error) (workers.WorkstationDispatchResult, bool, error) {
		return result, false, nil
	}), nil
}

func captureSelectedOpeningSnapshot(sources *[]interfaces.LoadedFactorySource, snapshot *interfaces.FactorySnapshot) interfaces.InitialFactorySnapshotFactory {
	return func(source interfaces.LoadedFactorySource) (*interfaces.FactorySnapshot, error) {
		*sources = append(*sources, source)
		return snapshot, nil
	}
}

func assertSelectedOpeningSnapshots(t *testing.T, resources *observationResourceOpening, snapshot *interfaces.FactorySnapshot, sources []interfaces.LoadedFactorySource) {
	t.Helper()
	if len(sources) != 2 || sources[0] == sources[1] {
		t.Fatalf("snapshot sources = %#v; want two detached opening sources", sources)
	}
	for index := range 2 {
		if resources.snapshots[index] != snapshot || sources[index].FactoryDir() != sources[0].FactoryDir() {
			t.Fatal("opening lost selected snapshot behavior or source")
		}
	}
}

func fixedTestProgress(t *testing.T, effects *[]string) func(*zap.Logger) func(string) workers.ProgressPublisher {
	t.Helper()
	return func(logger *zap.Logger) func(string) workers.ProgressPublisher {
		if logger == nil {
			t.Fatal("fixed progress behavior lost selected logger")
		}
		return func(sessionID string) workers.ProgressPublisher {
			return func(fragment workers.ProgressFragment) { *effects = append(*effects, sessionID+":"+fragment.Payload) }
		}
	}
}

func fixedTestCompletion(effects *[]string) func(string) func(string) {
	return func(sessionID string) func(string) {
		return func(dispatchID string) { *effects = append(*effects, sessionID+":"+dispatchID) }
	}
}

func assertUnpublishedOpeningFailure(t *testing.T, opening *factory.RuntimeInitialOpening, err, cause error) {
	t.Helper()
	if !errors.Is(err, cause) || opening == nil || opening.Record != nil || opening.Activation == nil || opening.Activation.Service != nil {
		t.Fatalf("opening = %#v, %v; want unpublished controlled failure", opening, err)
	}
}

func assertScopedOpeningObservations(t *testing.T, observations *openingSessionObservations, sessionID string) {
	t.Helper()
	if !reflect.DeepEqual(observations.sessions, []string{sessionID}) ||
		!reflect.DeepEqual(observations.progress, []string{sessionID}) || observations.mutationCount != 1 {
		t.Fatalf("%s observations = %#v; want exactly its own mutation and progress", sessionID, observations)
	}
}

type openingSessionObservations struct {
	sessions      []string
	progress      []string
	mutationCount int
}

var _ factory.SessionObservations = (*openingSessionObservations)(nil)

func (observations *openingSessionObservations) RecordPetriTokenMutations(sessionID string, mutations []interfaces.TokenMutationRecord) error {
	observations.sessions = append(observations.sessions, sessionID)
	observations.mutationCount += len(mutations)
	return nil
}

func (observations *openingSessionObservations) PublishWorkerProgress(fragment workers.ProgressFragment) {
	observations.progress = append(observations.progress, fragment.Payload)
}

type observationWorkerSessions struct {
	stubWorkerSessionsService
	runtimeIDs []string
}

func (sessions *observationWorkerSessions) PublishRuntimeProgress(_ context.Context, key workersessions.RuntimeAttemptKey, fragment workers.ProgressFragment, next workers.ProgressPublisher) error {
	sessions.runtimeIDs = append(sessions.runtimeIDs, key.RuntimeID)
	next(fragment)
	return nil
}

type observationResourceOpening struct {
	failure     error
	mutations   []factory.PetriMutationRecorder
	progress    []workers.ProgressPublisher
	completions []func(string)
	snapshots   []*interfaces.FactorySnapshot
}

func (opening *observationResourceOpening) Open(
	_ context.Context,
	_ *zap.Logger,
	_, _, _, _, _ string,
	_ interfaces.RuntimeMode,
	_ bool,
	_ factory.Scheduler,
	_ bool,
	_ string,
	_ factory.RuntimeLogStorageConfig,
	_ factory.RuntimeFileLoggingPolicy,
	_ factory.RuntimeMetricsPolicy,
	_ string,
	_ factory.RuntimeMetricsStorageConfig,
	_ factory.LoadedConfig,
	_, _ string,
	_ factory.Clock,
	_ string,
	snapshot *interfaces.FactorySnapshot,
	_ *interfaces.FactoryWorldState,
	_ bool,
	_ []factory.SubmissionHook,
	_ factory.CompletionDeliveryPlanner,
	mutations factory.PetriMutationRecorder,
	_ time.Duration,
	_ []interfaces.FactoryEvent,
	worker workers.Service,
	_ workersessions.Service,
	_ factory.WorkerAttemptOpener,
	completion func(string),
	_ ...*workers.MockWorkersConfig,
) (*factoryhost.Bundle, error) {
	opening.snapshots = append(opening.snapshots, snapshot)
	opening.mutations = append(opening.mutations, mutations)
	opening.completions = append(opening.completions, completion)
	opening.progress = append(opening.progress, worker.(interface {
		RuntimeProgressPublisher() workers.ProgressPublisher
	}).RuntimeProgressPublisher())
	return nil, opening.failure
}

func TestBundleOpeningReusesBehaviorAfterFailureWithoutChangingPeer(t *testing.T) {
	t.Parallel()
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	peerClock := clockwork.NewFakeClockAt(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	core, logs := observer.New(zap.InfoLevel)
	selectedLogger := zap.New(core)
	peerSpec := factory.SessionBuildSpec{Dir: dir, FolderPath: dir, SessionID: "peer", MetricsSessionID: "peer-canonical",
		RuntimeInstanceID: "peer-runtime", LoadedFactoryCfg: loaded, Clock: peerClock, BaseLogger: selectedLogger.With(zap.String("opening_selection", "peer"))}
	peerScopes := &testRuntimeScopeServiceStub{ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "peer-runtime"}}
	clock := clockwork.NewFakeClockAt(peerClock.Now().Add(time.Hour))
	spec := peerSpec
	spec.BaseLogger = selectedLogger.With(zap.String("opening_selection", "candidate"))
	spec.SessionID, spec.MetricsSessionID, spec.RuntimeInstanceID, spec.Clock = "candidate", "candidate-canonical", "candidate-runtime", clock
	openingErr := errors.New("candidate recording open failed")
	finalized := 0
	recorder := &runtimeRecordingsRecorderStub{onFinalize: func() { finalized++ }}
	var captured recordings.RuntimeScopeRequest
	scopes := &testRuntimeScopeServiceStub{openErr: openingErr, recorder: recorder, capturedRequest: &captured,
		ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "candidate-runtime"}}
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory(&testRuntimeScopeServiceStub{bySession: map[string]*testRuntimeScopeServiceStub{"peer": peerScopes, "candidate": scopes}}).Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, &testRuntimeScopeServiceStub{bySession: map[string]*testRuntimeScopeServiceStub{"peer": peerScopes, "candidate": scopes}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := openTestBundle(t.Context(), opening.Open, peerSpec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.CloseArtifacts() })
	failed, err := openTestBundle(t.Context(), opening.Open, spec)
	if !errors.Is(err, openingErr) || failed != nil || finalized != 1 {
		t.Fatalf("failed opening = %#v, %v, finalizations %d; want owned unwind without a runnable record", failed, err, finalized)
	}
	assertOpeningRetainsPendingCleanup(t, opening.Open, spec, recorder, openingErr)
	scopes.openErr = nil
	candidate, err := openTestBundle(t.Context(), opening.Open, spec)
	if err != nil {
		t.Fatalf("same-owner, same-identity retry: %v", err)
	}
	t.Cleanup(func() { _ = candidate.CloseArtifacts() })
	assertBundleOpeningSelections(t, candidate, peer, captured, spec, peerClock)
	assertBundleOpeningLogAttribution(t, candidate, peer, logs)
	if err := candidate.CloseArtifacts(); err != nil {
		t.Fatal(err)
	}
	if snapshot, err := peer.Factory.GetEngineStateSnapshot(t.Context()); err != nil || snapshot == nil {
		t.Fatalf("candidate close changed peer observation: %#v, %v", snapshot, err)
	}
}

func assertOpeningRetainsPendingCleanup(t *testing.T, opening factoryinternal.BundleOpeningOperation, spec factory.SessionBuildSpec, recorder *runtimeRecordingsRecorderStub, openingErr error) {
	t.Helper()
	releaseErr := errors.New("candidate recording release failed")
	recorder.finalizeErr = releaseErr
	partial, err := openTestBundle(t.Context(), opening, spec)
	if !errors.Is(err, openingErr) || !errors.Is(err, releaseErr) || partial == nil {
		t.Fatalf("partial opening = %#v, %v; want both failures and retained cleanup", partial, err)
	}
	if partial.RuntimeService() != nil {
		t.Fatal("failed opening returned a runnable service")
	}
	if err := errors.Join(partial.FinalizeRecording(spec.Clock.Now()), partial.CloseArtifacts()); !errors.Is(err, releaseErr) {
		t.Fatalf("pending cleanup = %v, want release failure", err)
	}
	recorder.finalizeErr = nil
	if err := errors.Join(partial.FinalizeRecording(spec.Clock.Now()), partial.CloseArtifacts()); err != nil {
		t.Fatalf("explicit cleanup retry = %v", err)
	}
}

func assertBundleOpeningSelections(t *testing.T, candidate, peer *factoryhost.Bundle, captured recordings.RuntimeScopeRequest, spec factory.SessionBuildSpec, peerClock factory.Clock) {
	t.Helper()
	if !candidate.StartTime().Equal(spec.Clock.Now()) || !peer.StartTime().Equal(peerClock.Now()) {
		t.Fatal("opening substituted the selected clock or changed its peer start time")
	}
	if captured.FactorySessionID != spec.SessionID || captured.CanonicalSessionID != spec.MetricsSessionID || captured.RecordingID != spec.RuntimeInstanceID {
		t.Fatalf("retry lost selected recording identities: %#v", captured)
	}
	if candidate.Factory == peer.Factory || candidate.Net == peer.Net || candidate.EventHistory == peer.EventHistory {
		t.Fatal("openings shared scoped engine, net or event state")
	}
}

// Emitted records prove the opening retained the selected backend and scoped
// fields after a failed candidate and retry, without replacing the live peer.
func assertBundleOpeningLogAttribution(t *testing.T, candidate, peer *factoryhost.Bundle, logs *observer.ObservedLogs) {
	t.Helper()
	candidate.Logger.Info("selected-opening")
	peer.Logger.Info("selected-opening")
	entries := logs.FilterMessage("selected-opening").All()
	if len(entries) != 2 {
		t.Fatalf("selected backend entries = %d, want candidate and peer", len(entries))
	}
	for index, sessionID := range []string{"candidate", "peer"} {
		fields := entries[index].ContextMap()
		if fields["opening_selection"] != sessionID || fields["session_id"] != sessionID {
			t.Fatalf("opening log %d attribution = %#v, want selected %s scope", index, fields, sessionID)
		}
	}
}

func openTestBundle(ctx context.Context, opening factoryinternal.BundleOpeningOperation, spec factory.SessionBuildSpec, progress ...workers.ProgressPublisher) (*factoryhost.Bundle, error) {
	var publisher workers.ProgressPublisher
	if len(progress) > 0 {
		publisher = progress[0]
	}
	record, err := opening(ctx, spec, "", factory.RuntimeLogStorageConfig{}, factory.RuntimeFileLoggingPolicyDisabled,
		factory.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, 0, spec.SessionID,
		interfaces.RuntimeModeBatch, nil, false, "", "", false, false, nil, nil, publisher, nil)
	if record == nil {
		return nil, err
	}
	return record.(*factoryhost.Bundle), err
}

// Ports the retired compatibility builder's error/cancellation and caller-value
// assertions onto the fixed owner that actually opens runtime resources.
func TestBundleOpeningPreservesCallerSpecOnFailureAndCancellation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	sessions := &stubWorkerSessionsService{}
	openingErr := errors.New("recording unavailable")
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		if canceled {
			cancel()
		}
		finalized := 0
		spec := factory.SessionBuildSpec{Dir: dir, FolderPath: dir, SessionID: "caller-owned", RuntimeInstanceID: "runtime",
			LoadedFactoryCfg: loaded, Clock: clockwork.NewFakeClock(), BaseLogger: zap.NewNop()}
		before := spec
		scopes := &testRuntimeScopeServiceStub{ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "runtime"},
			recorder: &runtimeRecordingsRecorderStub{onFinalize: func() { finalized++ }}}
		wantErr := error(context.Canceled)
		if !canceled {
			scopes.openErr = openingErr
			wantErr = openingErr
		}
		opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory(scopes).Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil, scopes, nil)
		if err != nil {
			t.Fatal(err)
		}
		record, err := openTestBundle(ctx, opening.Open, spec)
		cancel()
		if !errors.Is(err, wantErr) || record != nil {
			t.Fatalf("canceled=%t opening = (%v, %v), want %v", canceled, record, err, wantErr)
		}
		if !reflect.DeepEqual(spec, before) || spec.PetriMutationRecorder != nil {
			t.Fatal("opening mutated caller-owned spec")
		}
		if !canceled && finalized != 1 {
			t.Fatalf("failure finalizations = %d, want 1", finalized)
		}
	}
}

func TestInitialActivationRequestDetachesSelectedRecoveryFacts(t *testing.T) {
	t.Parallel()
	config := interfaces.FactorySnapshot(`{"name":"recorded"}`)
	at := time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC)
	request := factory.RuntimeActivationRequest{RuntimeID: "runtime", FactorySessionID: "candidate",
		Snapshot: interfaces.RuntimeSnapshot{FactoryDir: "/factory", RuntimeBaseDir: "/factory",
			EffectiveFactory: interfaces.FactoryConfig{Name: "factory"}, DefinitionVersion: &interfaces.FactoryVersion{Logical: 1}},
		Inputs: factory.RuntimeActivationInputs{RecoveryInput: factory.RuntimeActivationRecoveryInput{
			WorldState:   &interfaces.FactoryWorldState{WorkItemsByID: map[string]work.FactoryWorkItem{"work": {ID: "work"}}},
			EventHistory: []interfaces.FactoryEvent{{Id: "prefix"}},
			ReplayArtifact: &interfaces.ReplayArtifact{Factory: &config, Events: []interfaces.FactoryEvent{{Id: "replayed"}},
				Diagnostics: interfaces.ReplayDiagnostics{Notes: []string{"selected"}}, WallClock: &interfaces.ReplayWallClockMetadata{StartedAt: at}},
		}},
	}
	detached, err := request.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	request.Inputs.RecoveryInput.WorldState.WorkItemsByID["work"] = work.FactoryWorkItem{ID: "changed"}
	request.Inputs.RecoveryInput.EventHistory[0].Id = "changed"
	request.Inputs.RecoveryInput.ReplayArtifact.Events[0].Id = "changed"
	request.Inputs.RecoveryInput.ReplayArtifact.Diagnostics.Notes[0] = "changed"
	request.Inputs.RecoveryInput.ReplayArtifact.WallClock.StartedAt = time.Time{}
	(*request.Inputs.RecoveryInput.ReplayArtifact.Factory)[0] = ' '
	got := detached.Inputs.RecoveryInput
	if got.WorldState.WorkItemsByID["work"].ID != "work" || got.EventHistory[0].Id != "prefix" || got.ReplayArtifact.Events[0].Id != "replayed" {
		t.Fatalf("recovery retained caller mutations: %#v", got)
	}
	if got.ReplayArtifact.Diagnostics.Notes[0] != "selected" || !got.ReplayArtifact.WallClock.StartedAt.Equal(at) || string(*got.ReplayArtifact.Factory) != `{"name":"recorded"}` {
		t.Fatalf("replay metadata lost or shared: %#v", got.ReplayArtifact)
	}
}
