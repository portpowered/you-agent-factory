package internal_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

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
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	factoryruntimeorchestrationowner "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestrationowner"
	runtimeopening "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
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
		_ recordings.SubmissionRecorder,
		_ recordings.DispatchRecorder,
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
		_ factory.WorldStateProjector,
		_ recordings.RuntimeScopeService,
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
	opening, err := factoryinternal.NewBundleOpening(selected, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("constructor executed resource opening")
	}
	result, err := openTestBundle(t.Context(), opening.Open, spec, &testRuntimeScopeServiceStub{})
	if !errors.Is(err, openingErr) || result != partial || calls != 1 {
		t.Fatalf("opening = %p, %v, calls %d; want selected partial record %p and failure", result, err, calls, partial)
	}
	if result.RuntimeService() != nil {
		t.Fatal("partial failure published a runnable service")
	}
}

// The resource boundary retains callbacks just as an opened engine does. Emit
// after both admissions, so a reusable owner's latest-session substitution
// would route the first session's observations into its peer.
func TestAssemblyKeepsMutationAndProgressObservationsScopedAcrossCalls(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	resources := &observationResourceOpening{failure: errors.New("stop at controlled resource boundary")}
	sessions := &observationWorkerSessions{}
	opening, err := factoryinternal.NewBundleOpening(resources.Open, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, nil, nil, runtimebuild.New(nil, nil, testRuntimeID, zap.NewNop(), nil, nil, nil, nil),
		&testRuntimeScopeServiceStub{}, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	observations := [2]*openingSessionObservations{{}, {}}
	for index, sessionID := range []string{"candidate", "peer"} {
		result, openErr := assembleTestInitialOpening(t.Context(), assembly, dir, loaded, clockwork.NewFakeClock(),
			factory.RuntimeActivationRequest{FactorySessionID: sessionID, RuntimeID: "runtime-" + sessionID}, nil, observations[index])
		assertUnpublishedOpeningFailure(t, result, openErr, resources.failure)
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
	if !reflect.DeepEqual(sessions.runtimeIDs, []string{"runtime-candidate", "runtime-peer"}) {
		t.Fatalf("progress supervision identities = %v; want each admitted runtime", sessions.runtimeIDs)
	}
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
	opening, err := factoryinternal.NewBundleOpening(resources.Open, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("selected provider")})
	script := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte("selected script")})
	mock := &workers.MockWorkersConfig{}
	decorations := 0
	decorate := func(selected *workers.MockWorkersConfig, _ interfaces.RuntimeDefinitionLookup, next platformprocess.CommandRunner) platformprocess.CommandRunner {
		if selected != mock {
			t.Fatal("preparation changed the selected mock configuration")
		}
		decorations++
		return next
	}
	var effects []string
	projector := func(events []interfaces.FactoryEvent, tick int) (interfaces.FactoryWorldState, error) {
		if len(events) != 1 || events[0].Id != "selected-event" {
			t.Fatalf("projection events = %#v, want selected event", events)
		}
		return interfaces.FactoryWorldState{Tick: tick}, nil
	}
	submit := func(work.FactorySubmissionRecord) { effects = append(effects, "submission") }
	dispatch := func(recordings.FactoryDispatchRecord) { effects = append(effects, "dispatch") }
	loader := func(path string, _ interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		return loadedFactoryFixture(path)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, nil, nil, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, provider, script, decorate),
		&testRuntimeScopeServiceStub{}, nil, submit, dispatch, nil, projector, fixedTestProgress(t, &effects), fixedTestCompletion(&effects))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := assembleTestInitialOpening(t.Context(), assembly, dir, loaded, clockwork.NewFakeClock(),
		factory.RuntimeActivationRequest{FactorySessionID: "candidate", RuntimeID: "initial"}, mock)
	assertUnpublishedOpeningFailure(t, initial, err, resources.failure)
	for runner, output := range map[platformprocess.CommandRunner]string{
		initial.Spec.ProviderCommandRunner: "selected provider", initial.Spec.CommandRunnerOverride: "selected script",
	} {
		result, runErr := runner.Run(t.Context(), platformprocess.CommandRequest{Command: "controlled"})
		if runErr != nil || string(result.Stdout) != output {
			t.Fatalf("selected command output = %q, %v; want %q", result.Stdout, runErr, output)
		}
	}
	if _, err := initial.ReplacementBuilder.BuildReplacement(t.Context(), dir, dir, "successor", dir); !errors.Is(err, resources.failure) {
		t.Fatalf("replacement error = %v, want controlled resource failure", err)
	}
	for index := range resources.submissions {
		resources.submissions[index](work.FactorySubmissionRecord{})
		resources.dispatches[index](recordings.FactoryDispatchRecord{})
		assertSelectedOpeningProjection(t, resources.projectors[index], index+1)
		resources.progress[index](workers.ProgressFragment{Payload: "output"})
		resources.completions[index]("dispatch-id")
	}
	if decorations != 4 || !reflect.DeepEqual(effects, []string{"submission", "dispatch", "candidate:output", "candidate:dispatch-id", "submission", "dispatch", "successor:output", "successor:dispatch-id"}) {
		t.Fatalf("fixed effects = %v, decorations = %d; want both openings", effects, decorations)
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

func assertSelectedOpeningProjection(t *testing.T, projector factory.WorldStateProjector, tick int) {
	t.Helper()
	if projector == nil {
		t.Fatal("opening lost the fixed world-state projector")
	}
	state, err := projector([]interfaces.FactoryEvent{{Id: "selected-event"}}, tick)
	if err != nil || state.Tick != tick {
		t.Fatalf("opening projection = %#v, %v; want tick %d", state, err, tick)
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
	submissions []recordings.SubmissionRecorder
	dispatches  []recordings.DispatchRecorder
	projectors  []factory.WorldStateProjector
	completions []func(string)
}

func (opening *observationResourceOpening) Open(
	_ context.Context,
	_ *zap.Logger,
	_, _, _, _, _ string,
	_ interfaces.RuntimeMode,
	_ bool,
	_ factory.Scheduler,
	_ bool,
	submission recordings.SubmissionRecorder,
	dispatch recordings.DispatchRecorder,
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
	_ *interfaces.FactorySnapshot,
	_ *interfaces.FactoryWorldState,
	_ bool,
	_ []factory.SubmissionHook,
	_ factory.CompletionDeliveryPlanner,
	mutations factory.PetriMutationRecorder,
	projector factory.WorldStateProjector,
	_ recordings.RuntimeScopeService,
	worker workers.Service,
	_ workersessions.Service,
	_ factory.WorkerAttemptOpener,
	completion func(string),
	_ ...*workers.MockWorkersConfig,
) (*factoryhost.Bundle, error) {
	opening.mutations = append(opening.mutations, mutations)
	opening.submissions = append(opening.submissions, submission)
	opening.dispatches = append(opening.dispatches, dispatch)
	opening.projectors = append(opening.projectors, projector)
	opening.completions = append(opening.completions, completion)
	opening.progress = append(opening.progress, worker.(interface {
		RuntimeProgressPublisher() workers.ProgressPublisher
	}).RuntimeProgressPublisher())
	return nil, opening.failure
}

func TestBundleOpeningReusesBehaviorAfterFailureWithoutChangingPeer(t *testing.T) {
	t.Parallel()
	sessions := &stubWorkerSessionsService{}
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory().Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	peer, err := openTestBundle(t.Context(), opening.Open, peerSpec, peerScopes)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.CloseArtifacts() })
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
	failed, err := openTestBundle(t.Context(), opening.Open, spec, scopes)
	if !errors.Is(err, openingErr) || failed != nil || finalized != 1 {
		t.Fatalf("failed opening = %#v, %v, finalizations %d; want owned unwind without a runnable record", failed, err, finalized)
	}
	assertOpeningRetainsPendingCleanup(t, opening.Open, spec, scopes, recorder, openingErr)
	scopes.openErr = nil
	candidate, err := openTestBundle(t.Context(), opening.Open, spec, scopes)
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

func assertOpeningRetainsPendingCleanup(t *testing.T, opening factoryinternal.BundleOpeningOperation, spec factory.SessionBuildSpec, scopes recordings.RuntimeScopeService, recorder *runtimeRecordingsRecorderStub, openingErr error) {
	t.Helper()
	releaseErr := errors.New("candidate recording release failed")
	recorder.finalizeErr = releaseErr
	partial, err := openTestBundle(t.Context(), opening, spec, scopes)
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

func openTestBundle(ctx context.Context, opening factoryinternal.BundleOpeningOperation, spec factory.SessionBuildSpec, scopes recordings.RuntimeScopeService, progress ...workers.ProgressPublisher) (*factoryhost.Bundle, error) {
	var publisher workers.ProgressPublisher
	if len(progress) > 0 {
		publisher = progress[0]
	}
	record, err := opening(ctx, spec, "", factory.RuntimeLogStorageConfig{}, factory.RuntimeFileLoggingPolicyDisabled,
		factory.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, 0, spec.SessionID,
		interfaces.RuntimeModeBatch, nil, false, nil, nil, "", "", false, false, nil, nil, publisher, nil, nil, scopes, nil)
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
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory().Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
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
		record, err := openTestBundle(ctx, opening.Open, spec, scopes)
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

func TestBuild_ConstructsRecordingsRootLedgerAndHostingCapabilities(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "runtime-recordings-root"}
	var capturedSource recordings.InitialStructureSource
	recorder := &runtimeRecordingsRecorderStub{}
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:         ledger,
		recorder:       recorder,
		capturedSource: &capturedSource,
	}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-recordings-root", "", clockwork.NewFakeClock(),
		"/recordings/session.json", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if capturedSource == nil {
		t.Fatal("ledger factory InitialStructureSource = nil")
	}
	if bundle.RecordingLedger() != ledger {
		t.Fatalf("RecordingLedger = %T, want injected root ledger", bundle.RecordingLedger())
	}
	if bundle.Recording != recorder {
		t.Fatalf("Recording = %T, want injected RuntimeRecorder", bundle.Recording)
	}
	if bundle.StreamGeneration() != "runtime-recordings-root" {
		t.Fatalf("stream generation = %q, want runtime-recordings-root", bundle.StreamGeneration())
	}
}

func TestBuild_ConstructsRunnableBundleWithoutRootService(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-test", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.Factory == nil {
		t.Fatal("bundle.Factory = nil, want runnable factory runtime")
	}
	if bundle.EventHistory == nil {
		t.Fatal("bundle.EventHistory = nil")
	}
	if bundle.Net == nil {
		t.Fatal("bundle.Net = nil")
	}
}

func TestBuild_SeparatesCompatibilitySelectorFromCanonicalRuntimeIdentity(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	const (
		compatibilitySessionID = "~default"
		canonicalSessionID     = "550e8400-e29b-41d4-a716-446655440000"
	)
	var capturedRequest recordings.RuntimeScopeRequest
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:          &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "identity-handoff"},
		capturedRequest: &capturedRequest,
	}

	bundle, err := testRuntimeFactory().Build(
		context.Background(), zap.NewNop(), dir, dir, compatibilitySessionID, canonicalSessionID,
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-handoff", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if bundle.FactorySessionID != compatibilitySessionID {
		t.Fatalf("bundle FactorySessionID = %q, want compatibility selector %q", bundle.FactorySessionID, compatibilitySessionID)
	}
	workflowContext, ok := bundle.Factory.(factory.WorkflowContextProvider)
	if !ok {
		t.Fatalf("Factory = %T, want WorkflowContextProvider", bundle.Factory)
	}
	contextValue := workflowContext.WorkflowContext()
	if contextValue == nil {
		t.Fatal("workflow context = nil")
	}
	if got := contextValue.SessionID; got != canonicalSessionID {
		t.Fatalf("workflow context session ID = %q, want canonical identity %q", got, canonicalSessionID)
	}
	if capturedRequest.FactorySessionID != compatibilitySessionID || capturedRequest.CanonicalSessionID != canonicalSessionID {
		t.Fatalf("Recordings scope identities = factory=%q canonical=%q, want factory=%q canonical=%q",
			capturedRequest.FactorySessionID, capturedRequest.CanonicalSessionID,
			compatibilitySessionID, canonicalSessionID,
		)
	}
}

func TestBuild_UsesCompatibilityIdentityWhenCanonicalIdentityIsEmpty(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactory().Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		"", factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-identity-fallback", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	workflowContext, ok := bundle.Factory.(factory.WorkflowContextProvider)
	if !ok {
		t.Fatalf("Factory = %T, want WorkflowContextProvider", bundle.Factory)
	}
	if got := workflowContext.WorkflowContext().SessionID; got != "~default" {
		t.Fatalf("workflow context session ID = %q, want effective compatibility identity %q", got, "~default")
	}
}

func TestBuild_FinalizesRecordingBeforeClosingRuntimeSinksOnPartialFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	events := make([]string, 0, 3)
	recorder := &runtimeRecordingsRecorderStub{onFinalize: func() {
		events = append(events, "recording.finalize")
	}}
	runtimeScopes := &testRuntimeScopeServiceStub{
		ledger:   &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "partial-runtime"},
		recorder: recorder,
	}
	_, err = testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir,
		func() { events = append(events, "log.close") },
		func() { events = append(events, "metrics.close") },
	).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "partial-runtime", "", clockwork.NewFakeClock(), "recording.json", nil, nil, false, nil, nil, nil, nil,
		runtimeScopes,
		testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
	)
	if err == nil {
		t.Fatal("Build succeeded, want partial-opening failure")
	}
	if got, want := events, []string{"recording.finalize", "log.close", "metrics.close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("partial-opening cleanup order = %#v, want %#v", got, want)
	}
}

func TestBuild_PreservesOpeningAndCleanupFailures(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{"log", "metrics", "recording", "engine"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
			loaded, err := loadedFactoryFixture(dir)
			if err != nil {
				t.Fatal(err)
			}
			openingErr := errors.New("opening failed")
			logErr := errors.New("log remains open")
			metricsErr := errors.New("metrics remains open")
			recordingErr := errors.New("recording remains open")
			var events []string
			logOwner := testRuntimeLogOwner{root: dir, closeErr: logErr, onClose: func() { events = append(events, "log.close") }}
			metricsOwner := testRuntimeMetricsOwner{root: dir, closeErr: metricsErr, onClose: func() { events = append(events, "metrics.close") }}
			scopes := &testRuntimeScopeServiceStub{
				ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: stage},
				recorder: &runtimeRecordingsRecorderStub{finalizeErr: recordingErr, onFinalize: func() {
					events = append(events, "recording.finalize")
				}},
			}
			wantEvents := []string{"recording.finalize", "log.close", "metrics.close"}
			wantErrors := []error{logErr, metricsErr, recordingErr}
			switch stage {
			case "log":
				logOwner.openErr = openingErr
				wantEvents = []string{"log.close"}
				wantErrors = []error{openingErr, logErr}
			case "metrics":
				metricsOwner.openErr = openingErr
				wantEvents = []string{"log.close", "metrics.close"}
				wantErrors = []error{openingErr, logErr, metricsErr}
			case "recording":
				scopes.openErr = openingErr
				wantErrors = append(wantErrors, openingErr)
			}
			bundle, err := testRuntimeFactoryWithOwners(logOwner, metricsOwner).Build(
				context.Background(), zap.NewNop(), dir, dir, "~default", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, nil, nil, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, stage, "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil, nil, scopes,
				testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
			)
			if bundle == nil || bundle.RuntimeService() != nil || err == nil {
				t.Fatalf("Build = (%v, %v), want partial ownership without a live service", bundle, err)
			}
			for _, cause := range wantErrors {
				if !errors.Is(err, cause) {
					t.Errorf("Build error %v lost cause %v", err, cause)
				}
			}
			if !reflect.DeepEqual(events, wantEvents) {
				t.Fatalf("cleanup order = %v, want %v", events, wantEvents)
			}
		})
	}
}

func TestBuild_FailedOpeningRetainsRetryableCleanupWithoutRepeatingReleasedResources(t *testing.T) {
	t.Parallel()
	for _, failed := range []string{"none", "recording", "log", "metrics"} {
		t.Run(failed, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
			loaded, err := loadedFactoryFixture(dir)
			if err != nil {
				t.Fatal(err)
			}
			openingErr, releaseErr := errors.New("recording opening failed"), errors.New("release failed")
			calls := map[string]int{}
			logSink := &testRuntimeLogSink{logger: zap.NewNop(), onClose: func() { calls["log"]++ }}
			metricsSink := &testRuntimeMetricsSink{onClose: func() { calls["metrics"]++ }}
			recorder := &runtimeRecordingsRecorderStub{onFinalize: func() { calls["recording"]++ }}
			switch failed {
			case "recording":
				recorder.finalizeErr = releaseErr
			case "log":
				logSink.closeErr = releaseErr
			case "metrics":
				metricsSink.closeErr = releaseErr
			}
			owner := testRuntimeFactoryWithOwners(
				testRuntimeLogOwnerFunc(func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) { return logSink, nil }),
				testRuntimeMetricsOwnerFunc(func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) { return metricsSink, nil }),
			)
			bundle, err := owner.Build(
				context.Background(), zap.NewNop(), dir, dir, "candidate", "", "", interfaces.RuntimeModeBatch,
				false, nil, false, nil, nil, dir, factory.RuntimeLogStorageConfig{}, "", "", dir,
				factory.RuntimeMetricsStorageConfig{}, loaded, "candidate-runtime", "", clockwork.NewFakeClock(),
				"recording.json", nil, nil, false, nil, nil, nil, nil,
				&testRuntimeScopeServiceStub{recorder: recorder, openErr: openingErr},
				testRuntimeWorkers{}, &stubWorkerSessionsService{}, nil, nil,
			)
			if !errors.Is(err, openingErr) || errors.Is(err, releaseErr) != (failed != "none") {
				t.Fatalf("Build error = %v, lost opening or release cause", err)
			}
			if failed == "none" {
				if bundle != nil {
					t.Fatal("successful unwind retained resources")
				}
				return
			}
			if bundle == nil || bundle.RuntimeService() != nil || bundle.RuntimeInstanceID != "candidate-runtime" {
				t.Fatalf("partial ownership = %#v, want addressed cleanup without a service", bundle)
			}
			if err := errors.Join(bundle.FinalizeRecording(time.Time{}), bundle.CloseArtifacts()); !errors.Is(err, releaseErr) {
				t.Fatalf("failed cleanup retry = %v, want retained release failure", err)
			}
			recorder.finalizeErr, logSink.closeErr, metricsSink.closeErr = nil, nil, nil
			if err := errors.Join(bundle.FinalizeRecording(time.Time{}), bundle.CloseArtifacts(), bundle.CloseArtifacts()); err != nil {
				t.Fatalf("successful cleanup retry: %v", err)
			}
			assertOpeningResourceReleaseCounts(t, calls, failed)
		})
	}
}

func assertOpeningResourceReleaseCounts(t *testing.T, calls map[string]int, failed string) {
	t.Helper()
	for _, resource := range []string{"recording", "log", "metrics"} {
		want := 1
		if resource == failed {
			want = 3
		}
		if calls[resource] != want {
			t.Errorf("%s releases = %d, want %d", resource, calls[resource], want)
		}
	}
}

func TestBuild_AssemblyOpeningFailureRetainsCleanupAtRootAndRetriesSameIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := clockwork.NewFakeClock()
	openingErr, cleanupErr := errors.New("opening failed"), errors.New("finalization failed")
	finalizations, attempts := 0, 0
	recorder := &runtimeRecordingsRecorderStub{finalizeErr: cleanupErr, onFinalize: func() { finalizations++ }}
	scopes := &testRuntimeScopeServiceStub{recorder: recorder, openErr: openingErr,
		ledger: &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "candidate-runtime"}}
	sessions := &stubWorkerSessionsService{}
	loader := func(string, interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		return loaded, nil
	}
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory().Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, factoryinternal.NewSidecarOpening(nil, platformclock.Real{}), testCleanupAssemblyHost{}, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, nil, nil, nil), scopes, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := factoryinternal.NewRoot(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := factory.RuntimeActivationRequest{RuntimeID: "candidate-runtime", FactorySessionID: "candidate",
		Snapshot: interfaces.RuntimeSnapshot{FactoryDir: dir, RuntimeBaseDir: dir,
			DefinitionVersion: &interfaces.FactoryVersion{Logical: 1}, EffectiveFactory: *loaded.FactoryConfig()}}
	start := func(ctx context.Context, request factory.RuntimeActivationRequest) (*factory.RuntimeActivation, error) {
		attempts++
		return assembleCleanupTestRuntime(ctx, assembly, dir, loaded, clock, request)
	}
	result, err := root.Activate(t.Context(), request, start)
	assertOpeningFailureWithoutPublication(t, result, err, openingErr, cleanupErr)
	if _, err := root.Activate(t.Context(), request, start); !errors.Is(err, factory.ErrRuntimeActivationConflict) || attempts != 1 {
		t.Fatalf("pending cleanup activation = %v, attempts %d, want conflict without another opening", err, attempts)
	}
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); !errors.Is(err, cleanupErr) {
		t.Fatalf("failed explicit cleanup = %v, want retained cause", err)
	}
	if finalizations != 3 {
		t.Fatalf("failed finalizations = %d, want build unwind, Root unwind, and explicit retry", finalizations)
	}
	recorder.finalizeErr, scopes.openErr = nil, nil
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{RuntimeID: request.RuntimeID}); err != nil {
		t.Fatalf("successful explicit cleanup: %v", err)
	}
	result, err = root.Activate(t.Context(), request, start)
	if err != nil || result.Binding.IsZero() || result.State != factory.RuntimeLifecycleStateActive || attempts != 2 {
		t.Fatalf("same-identity retry = %#v, %v, attempts %d", result, err, attempts)
	}
	if _, err := root.Deactivate(t.Context(), factory.RuntimeDeactivationRequest{Binding: result.Binding}); err != nil {
		t.Fatalf("close retried generation: %v", err)
	}
}

func assertOpeningFailureWithoutPublication(t *testing.T, result factory.RuntimeActivationResult, err, openingErr, cleanupErr error) {
	t.Helper()
	if !errors.Is(err, openingErr) || !errors.Is(err, cleanupErr) || !result.Binding.IsZero() {
		t.Fatalf("failed activation = %#v, %v, want both causes and no publication", result, err)
	}
}

// This component cell runs the real Runtime assembly/build/Root chain with
// controlled Recordings effects. Sessions publication and public commands are
// proved separately by the functional lane.
func assembleCleanupTestRuntime(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource,
	clock factory.Clock, request factory.RuntimeActivationRequest,
) (*factory.RuntimeActivation, error) {
	opening, err := assembleTestInitialOpening(ctx, assembly, dir, loaded, clock, request, nil)
	if opening == nil {
		return nil, err
	}
	return opening.Activation, err
}

func assembleTestRuntimeRecord(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource,
	clock factory.Clock, request factory.RuntimeActivationRequest,
) (factory.RuntimeReplacementBuilder, factory.RuntimeRecord, factory.SessionBuildSpec, error) {
	opening, err := assembleTestInitialOpening(ctx, assembly, dir, loaded, clock, request, nil)
	if opening == nil {
		return nil, nil, factory.SessionBuildSpec{}, err
	}
	return opening.ReplacementBuilder, opening.Record, opening.Spec, err
}

func assembleTestInitialOpening(
	ctx context.Context, assembly *factoryinternal.Assembly, dir string,
	loaded interfaces.MutableLoadedFactorySource,
	clock factory.Clock, request factory.RuntimeActivationRequest,
	mockWorkers *workers.MockWorkersConfig,
	observations ...factory.SessionObservations,
) (*factory.RuntimeInitialOpening, error) {
	var observe factory.SessionObservations
	if len(observations) > 0 {
		observe = observations[0]
	}
	return assembly.Assemble(
		ctx,
		"",
		"",
		false,
		"recording.json",
		"",
		request.FactorySessionID,
		request.FactorySessionID,
		mockWorkers,
		interfaces.RuntimeModeBatch,
		nil,
		false,
		"",
		factory.RuntimeLogStorageConfig{},
		factory.RuntimeFileLoggingPolicyDisabled,
		factory.RuntimeMetricsPolicyDisabled,
		"",
		factory.RuntimeMetricsStorageConfig{},
		0,
		"",
		"",
		false,
		false,
		nil,
		clock,
		zap.NewNop(),
		true,
		observe,
		dir,
		dir,
		dir,
		loaded,
		request.RuntimeID,
		nil,
		nil,
		nil,
		nil,
		false,
	)
}

func TestInitialActivationReplacementRetainsSelectionsAndCanRetry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := clockwork.NewFakeClock()
	loadErr := errors.New("replacement load failed")
	failLoad := true
	loader := func(path string, _ interfaces.WorkstationLoader) (interfaces.MutableLoadedFactorySource, error) {
		if failLoad {
			return nil, loadErr
		}
		return loadedFactoryFixture(path)
	}
	sessions := &stubWorkerSessionsService{}
	opening, err := factoryinternal.NewBundleOpening(testRuntimeFactory().Build, platformclock.Real{}, testRuntimeWorkers{}, sessions, sessions, nil)
	if err != nil {
		t.Fatal(err)
	}
	var recorded recordings.RuntimeScopeRequest
	scopes := &testRuntimeScopeServiceStub{capturedRequest: &recorded,
		ledgerFactory: func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger {
			return &recordingfixtures.ScriptedRuntimeLedger{GenerationID: recorded.RecordingID}
		}}
	var snapshots []interfaces.LoadedFactorySource
	snapshot := func(source interfaces.LoadedFactorySource) (*interfaces.FactorySnapshot, error) {
		snapshots = append(snapshots, source)
		return nil, nil
	}
	assembly, err := factoryinternal.NewAssembly(opening.Open, factoryinternal.NewSidecarOpening(nil, platformclock.Real{}), testCleanupAssemblyHost{}, runtimebuild.New(nil, loader, testRuntimeID, zap.NewNop(), nil, nil, nil, nil), scopes, snapshot, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	builder, initial, spec, err := assembleTestRuntimeRecord(t.Context(), assembly, dir, loaded, clock,
		factory.RuntimeActivationRequest{FactorySessionID: "candidate", RuntimeID: "initial-runtime"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = initial.CloseArtifacts() })
	if spec.RuntimeInstanceID != "initial-runtime" || recorded.RecordingID != "initial-runtime" {
		t.Fatalf("initial identity = %q/%q", spec.RuntimeInstanceID, recorded.RecordingID)
	}
	if replacement, err := builder.BuildReplacement(t.Context(), dir, dir, "candidate", dir); !errors.Is(err, loadErr) || replacement != nil {
		t.Fatalf("failed replacement = %#v, %v, want load failure without resource", replacement, err)
	}
	failLoad = false
	clock.Advance(time.Minute)
	replacement, err := builder.BuildReplacement(t.Context(), dir, dir, "candidate", dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.CloseArtifacts() })
	assertReplacementSelections(t, initial, replacement, recorded, clock)
	assertReplacementBuildValues(t, replacement, recorded, spec, dir)
	if len(snapshots) != 2 || snapshots[0] != initial.LoadedRuntimeConfig() || snapshots[1] != replacement.LoadedRuntimeConfig() {
		t.Fatalf("selected snapshot sources = %#v, want independent initial and replacement candidates", snapshots)
	}
}

func assertReplacementBuildValues(t *testing.T, replacement factory.RuntimeRecord, recorded recordings.RuntimeScopeRequest, spec factory.SessionBuildSpec, dir string) {
	t.Helper()
	if recorded.FactorySessionID != "candidate" || recorded.RecordPath != "recording.candidate.json" {
		t.Fatalf("replacement recording selections = %#v", recorded)
	}
	if replacement.LoadedRuntimeConfig().FactoryDir() != dir || replacement.LoadedRuntimeConfig().RuntimeBaseDir() != dir {
		t.Fatal("replacement changed the selected factory or execution directory")
	}
	if spec.RuntimeInstanceID != "initial-runtime" || spec.PetriMutationRecorder != nil {
		t.Fatal("replacement mutated the initial caller's spec")
	}
}

func assertReplacementSelections(t *testing.T, initial, replacement factory.RuntimeRecord, recorded recordings.RuntimeScopeRequest, clock factory.Clock) {
	t.Helper()
	if !replacement.StartTime().Equal(clock.Now()) || recorded.RecordingID != testRuntimeID() {
		t.Fatalf("replacement time/identity = %v/%q", replacement.StartTime(), recorded.RecordingID)
	}
	if replacement.LoadedRuntimeConfig() == initial.LoadedRuntimeConfig() || replacement.RecordingLedger() == initial.RecordingLedger() {
		t.Fatal("replacement shares candidate or history with the initial generation")
	}
	if initial.StartTime().Equal(replacement.StartTime()) || initial.StreamGeneration() != "initial-runtime" {
		t.Fatal("replacement changed the initial generation's clock or identity")
	}
}

type testCleanupAssemblyHost struct{ instancehost.Service }

func (host testCleanupAssemblyHost) Scope(factory.Clock) instancehost.Service { return host }

func TestBuild_FileLoggingRetainsSelectedLogger(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	core, logs := observer.New(zap.InfoLevel)
	selected := zap.New(core).With(zap.String("invocation_id", "selected-invocation"))
	var received *zap.Logger
	owner := testRuntimeLogOwnerFunc(func(logger *zap.Logger, _ factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
		received = logger
		return &testRuntimeLogSink{logger: logger}, nil
	})
	sessions := &stubWorkerSessionsService{}
	bundle, err := testRuntimeFactoryWithOwners(owner, nil).Build(
		t.Context(), selected, dir, dir, "selected-session", "", "", interfaces.RuntimeModeBatch,
		false, nil, false, nil, nil, dir, factory.RuntimeLogStorageConfig{},
		factory.RuntimeFileLoggingPolicyEnabled, factory.RuntimeMetricsPolicyDisabled,
		"", factory.RuntimeMetricsStorageConfig{}, loaded, "selected-runtime", "", clockwork.NewFakeClock(),
		"", nil, nil, false, nil, nil, nil, nil, testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{}, sessions, sessions, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.CloseArtifacts() })
	if received != selected {
		t.Fatal("runtime log owner did not receive the selected scoped logger")
	}
	bundle.Logger.Info("file-logging-selection")
	entries := logs.FilterMessage("file-logging-selection").All()
	if len(entries) != 1 {
		t.Fatalf("selected backend records = %d, want one", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["invocation_id"] != "selected-invocation" || fields["session_id"] != "selected-session" {
		t.Fatalf("file-backed runtime log attribution = %#v", fields)
	}
}

func TestBuild_ProductionObservabilityPoliciesEnableRuntimeSinksByDefault(t *testing.T) {
	sessions := &stubWorkerSessionsService{}
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	bundle, err := testRuntimeFactoryWithSinks(logDir, metricsDir).Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		"", "", metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-observability", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.LogSink == nil {
		t.Fatal("LogSink = nil, want runtime log sink when production policy is unset")
	}
	if bundle.MetricsSink == nil {
		t.Fatal("MetricsSink = nil, want runtime metrics sink when production policy is unset")
	}
	if bundle.LogSink.Artifact().RootDir != logDir {
		t.Fatalf("LogSink root = %q, want %q", bundle.LogSink.Artifact().RootDir, logDir)
	}
	if bundle.MetricsSink.Artifact().RootDir != metricsDir {
		t.Fatalf("MetricsSink artifact root = %q, want %q", bundle.MetricsSink.Artifact().RootDir, metricsDir)
	}
	if filepath.Base(bundle.LogSink.Artifact().Path) == "" {
		t.Fatal("LogSink.Path() = empty")
	}
	if filepath.Base(bundle.MetricsSink.Path()) == "" {
		t.Fatal("MetricsSink.Path() = empty")
	}

	disabledBundle, err := testRuntimeFactory().Build(
		context.Background(), zap.NewNop(), dir, dir, "~default", "",
		"", interfaces.RuntimeModeBatch, false, nil, false, nil, nil,
		logDir, factory.RuntimeLogStorageConfig{},
		factoryinternal.RuntimeFileLoggingPolicyDisabled,
		factoryinternal.RuntimeMetricsPolicyDisabled,
		metricsDir, factory.RuntimeMetricsStorageConfig{},
		loaded, "runtime-disabled", "", clockwork.NewFakeClock(), "", nil, nil, false, nil, nil, nil, nil,
		testRuntimeScopeService(newTestRuntimeLedger),
		testRuntimeWorkers{},
		sessions, sessions,
		nil,
	)
	if err != nil {
		t.Fatalf("Build disabled policy: %v", err)
	}
	if disabledBundle == nil {
		t.Fatal("disabled policy bundle = nil")
	}
	if disabledBundle.LogSink != nil {
		t.Fatal("LogSink = non-nil, want nil when runtime file logging is explicitly disabled")
	}
	if disabledBundle.MetricsSink != nil {
		t.Fatal("MetricsSink = non-nil, want nil when runtime metrics policy is explicitly disabled")
	}
}

type testRuntimeWorkers struct{ workers.Service }

// stubWorkerSessionsService is a minimal workersessions.Service double for
// build-composition tests: Start hands the request straight to the resolved
// Workers execution boundary, mirroring the real cutover seam's shape
// without pulling in the peer worker_sessions implementation package.
type stubWorkerSessionsService struct {
	factory.WorkerAttemptOpener
	execution workers.Service
}

// This execution-only double has no durable captured activity.
func (*stubWorkerSessionsService) GetCapturedObservation(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, workersessions.ErrObservationSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogs(context.Context, workersessions.ReadLogsRequest) (workersessions.LogPage, error) {
	return workersessions.LogPage{}, workersessions.ErrSessionNotFound
}

func (*stubWorkerSessionsService) ReadLogsArtifact(context.Context, string, string) (io.ReadCloser, error) {
	return nil, workersessions.ErrSessionNotFound
}

func (s *stubWorkerSessionsService) Reserve(context.Context, workersessions.ReserveRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) Get(context.Context, workersessions.GetRequest) (workersessions.Session, error) {
	return workersessions.Session{}, nil
}

func (s *stubWorkerSessionsService) List(context.Context, workersessions.ListRequest) (workersessions.ListResult, error) {
	return workersessions.ListResult{}, nil
}

func (s *stubWorkerSessionsService) ListObservations(context.Context, workersessions.ListObservationsRequest) (workersessions.ListObservationsResult, error) {
	return workersessions.ListObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) GetObservation(context.Context, workersessions.GetObservationRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) GetObservationByWorkerSessionID(context.Context, workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	return workersessions.Observation{}, nil
}

func (s *stubWorkerSessionsService) ListWorkerSessionObservations(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return workersessions.ListWorkerSessionObservationsResult{}, nil
}

func (s *stubWorkerSessionsService) StreamObservations(context.Context, workersessions.StreamObservationsRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) StreamObservationsByWorkerSessionID(context.Context, workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	return workersessions.ObservationSubscription{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscript(context.Context, workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) ReadTranscriptByWorkerSessionID(context.Context, workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	return workersessions.ReadTranscriptResult{}, nil
}

func (s *stubWorkerSessionsService) InvokeSession(ctx context.Context, req workersessions.InvokeSessionRequest) (workersessions.InvokeSessionResult, error) {
	return workersessions.InvokeSessionResult{
		Session: workersessions.Session{ID: req.ID, State: workersessions.StateCompleted},
	}, nil
}

func (s *stubWorkerSessionsService) Start(ctx context.Context, req workersessions.StartRequest) (workersessions.StartResult, error) {
	result, err := s.InvokeSession(ctx, workersessions.InvokeSessionRequest{
		ID:        req.ID,
		Execution: req.Execution,
		Retry:     req.Retry,
	})
	return workersessions.StartResult{Session: result.Session}, err
}

func (s *stubWorkerSessionsService) Continue(context.Context, workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	return workersessions.ContinueResult{}, nil
}

func (s *stubWorkerSessionsService) Interrupt(context.Context, workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	return workersessions.InterruptResult{}, nil
}

func (s *stubWorkerSessionsService) PublishRecord(context.Context, workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
	return workersessions.PublishRecordResult{}, nil
}

func (s *stubWorkerSessionsService) AssociateProviderSession(context.Context, workersessions.ProviderSessionAssociationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) ObserveProviderSession(context.Context, workersessions.ProviderSessionObservationRequest) (workersessions.ProviderSessionAssociationResult, error) {
	return workersessions.ProviderSessionAssociationResult{}, nil
}

func (s *stubWorkerSessionsService) EnsureProviderBinding(context.Context, workersessions.ProviderBindingRequest) (workersessions.ProviderBindingResult, error) {
	return workersessions.ProviderBindingResult{}, nil
}

func (s *stubWorkerSessionsService) WorkerSessionIDForDispatch(_ context.Context, dispatchID string) (string, error) {
	return dispatchID, nil
}

func (s *stubWorkerSessionsService) Pause(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Resume(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Cancel(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func (s *stubWorkerSessionsService) Terminate(context.Context, workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return workersessions.ControlResult{}, nil
}

func loadedFactoryFixture(dir string) (interfaces.MutableLoadedFactorySource, error) {
	payload, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		return nil, err
	}
	return factorydefinitionfixtures.NewLoadedSource(dir, config, nil, nil)
}

func testOrchestrationCompilation() factory.OrchestrationCompilation {
	return factoryruntimeorchestrationowner.NewCompilation(orchestrationwire.New(testDefinitionMapper(), nil, nil))
}

func testDefinitionMapper() *definitionmapping.Mapper {
	mapper, err := definitionmapping.New(testRuntimeID)
	if err != nil {
		panic(err) // The fixed test ID generator is required by this fixture.
	}
	return mapper
}

func testRuntimeFactory() *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		testRuntimeLoggerFactory, nil, nil,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		platformclock.Real{}, testDefinitionMapper(),
		runtimeopening.NewEngineOpening(nil, nil, nil, nil, outputAsPayloadPolicy(), nil, testRuntimeID, testRuntimeID, localRuntimeFiles{}, nil, dispatchplanningwire.NewOpening()),
	)
}

func testRuntimeFactoryWithSinks(logDir, metricsDir string) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithSinkCallbacks(logDir, metricsDir, nil, nil)
}

func testRuntimeFactoryWithSinkCallbacks(
	logDir string,
	metricsDir string,
	onLogClose func(),
	onMetricsClose func(),
) *factoryinternal.RuntimeFactory {
	return testRuntimeFactoryWithOwners(
		testRuntimeLogOwner{root: logDir, onClose: onLogClose},
		testRuntimeMetricsOwner{root: metricsDir, onClose: onMetricsClose},
	)
}

func testRuntimeFactoryWithOwners(logOwner factory.RuntimeLogOwner, metricsOwner factory.RuntimeMetricsOwner) *factoryinternal.RuntimeFactory {
	return factoryinternal.NewRuntimeFactory(
		testRuntimeLoggerFactory,
		logOwner, metricsOwner,
		testRuntimeID, testRuntimeID, localRuntimeFiles{}, localRuntimeFiles{}, filepath.WalkDir,
		testOrchestrationCompilation(),
		platformclock.Real{}, testDefinitionMapper(),
		runtimeopening.NewEngineOpening(nil, nil, nil, nil, outputAsPayloadPolicy(), nil, testRuntimeID, testRuntimeID, localRuntimeFiles{}, nil, dispatchplanningwire.NewOpening()),
	)
}

func outputAsPayloadPolicy() interfaces.WorkPropagationPolicyService {
	return interfaces.WorkPropagationPolicyFunc(func(
		*interfaces.FactoryWorkstationConfig,
	) interfaces.WorkPropagationMode {
		return interfaces.WorkPropagationModeOutputAsPayload
	})
}

func newTestRuntimeLedger(
	recordings.InitialStructureSource,
	func() time.Time,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return &recordingfixtures.ScriptedRuntimeLedger{}
}

func testRuntimeScopeService(
	ledgerFactory func(
		recordings.InitialStructureSource,
		func() time.Time,
		interfaces.RuntimeDefinitionLookup,
	) recordings.RuntimeEventLedger,
) recordings.RuntimeScopeService {
	return &testRuntimeScopeServiceStub{ledgerFactory: ledgerFactory}
}

type testRuntimeScopeServiceStub struct {
	openErr         error
	ledger          recordings.RuntimeEventLedger
	ledgerFactory   func(recordings.InitialStructureSource, func() time.Time, interfaces.RuntimeDefinitionLookup) recordings.RuntimeEventLedger
	recorder        recordings.RuntimeRecorder
	capturedSource  *recordings.InitialStructureSource
	capturedRequest *recordings.RuntimeScopeRequest
}

func (runtimeScopes *testRuntimeScopeServiceStub) OpenRuntime(
	_ context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	if runtimeScopes.capturedSource != nil {
		*runtimeScopes.capturedSource = request.Topology
	}
	if runtimeScopes.capturedRequest != nil {
		*runtimeScopes.capturedRequest = request
	}
	ledger := runtimeScopes.ledger
	if runtimeScopes.ledgerFactory != nil {
		ledger = runtimeScopes.ledgerFactory(request.Topology, request.Now, request.Definitions)
	}
	return recordings.RuntimeScopeResult{Ledger: ledger, Recorder: runtimeScopes.recorder}, runtimeScopes.openErr
}

func (*testRuntimeScopeServiceStub) Projection() recordings.ProjectionService { return nil }

func (*testRuntimeScopeServiceStub) ReconstructCanonicalFactoryWorldState(
	[]interfaces.FactoryEvent,
	int,
) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (*testRuntimeScopeServiceStub) ReplayClock(*recordings.ReplayArtifact) recordings.Clock {
	return nil
}

func (*testRuntimeScopeServiceStub) ReplayExecution(
	*recordings.ReplayArtifact,
) (providers.Service, platformprocess.CommandRunner, []recordings.ReplayHook, recordings.CompletionDeliveryPlanner, error) {
	return nil, nil, nil, nil, nil
}

func (*testRuntimeScopeServiceStub) LoadReplayInput(recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	return recordings.LoadReplayInputResult{}, nil
}

func (*testRuntimeScopeServiceStub) LoadResumeInput(recordings.LoadResumeInputRequest) (recordings.LoadResumeInputResult, error) {
	return recordings.LoadResumeInputResult{}, nil
}

var _ recordings.RuntimeScopeService = (*testRuntimeScopeServiceStub)(nil)

func testRuntimeLoggerFactory(*zap.Logger, bool) factory.Logger { return factory.NoopLogger{} }

type testRuntimeLogSink struct {
	logger   *zap.Logger
	artifact factory.RuntimeLogArtifact
	onClose  func()
	closeErr error
}

func (sink *testRuntimeLogSink) Logger() *zap.Logger                  { return sink.logger }
func (sink *testRuntimeLogSink) Artifact() factory.RuntimeLogArtifact { return sink.artifact }
func (sink *testRuntimeLogSink) Close() error {
	if sink != nil && sink.onClose != nil {
		sink.onClose()
	}
	return sink.closeErr
}

type testRuntimeLogOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeLogOwnerFunc func(*zap.Logger, factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error)

func (owner testRuntimeLogOwnerFunc) Open(logger *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return owner(logger, request)
}

func (owner testRuntimeLogOwner) Open(logger *zap.Logger, request factory.RuntimeLogScopeRequest) (factory.RuntimeLogSink, error) {
	return &testRuntimeLogSink{logger: logger, onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeLogArtifact{
		Path: filepath.Join(owner.root, request.RuntimeInstanceID+".runtime.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(), Config: request.Config,
	}}, owner.openErr
}

type testRuntimeMetricsSink struct {
	artifact factory.RuntimeMetricsArtifact
	onClose  func()
	closeErr error
}

func (s *testRuntimeMetricsSink) Counter(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Gauge(context.Context, string, float64, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Sample(context.Context, string, float64, string, factory.Fields) error {
	return nil
}
func (s *testRuntimeMetricsSink) Close() error {
	if s != nil && s.onClose != nil {
		s.onClose()
	}
	return s.closeErr
}
func (s *testRuntimeMetricsSink) Path() string { return s.artifact.Path }
func (s *testRuntimeMetricsSink) Artifact() factory.RuntimeMetricsArtifact {
	return s.artifact
}

type testRuntimeMetricsOwner struct {
	root     string
	onClose  func()
	openErr  error
	closeErr error
}

type testRuntimeMetricsOwnerFunc func(factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error)

func (owner testRuntimeMetricsOwnerFunc) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return owner(request)
}

func (owner testRuntimeMetricsOwner) Open(request factory.RuntimeMetricsScopeRequest) (factory.RuntimeMetricsSink, error) {
	return &testRuntimeMetricsSink{onClose: owner.onClose, closeErr: owner.closeErr, artifact: factory.RuntimeMetricsArtifact{
		Path: filepath.Join(owner.root, request.Scope.RuntimeInstanceID+".runtime-metrics.log"), RootDir: owner.root,
		StartTimeUTC: time.Now().UTC(),
	}}, owner.openErr
}

type runtimeRecordingsRecorderStub struct {
	onFinalize  func()
	finalizeErr error
}

func (*runtimeRecordingsRecorderStub) BindRecordingLifecycle(
	recordings.RecordingLifecycle,
	recordings.CanonicalEventScope,
) error {
	return nil
}

func (*runtimeRecordingsRecorderStub) Start(context.Context)               {}
func (*runtimeRecordingsRecorderStub) Stop()                               {}
func (*runtimeRecordingsRecorderStub) RecordEvent(interfaces.FactoryEvent) {}
func (*runtimeRecordingsRecorderStub) RecordError(error)                   {}
func (*runtimeRecordingsRecorderStub) Finish(time.Time)                    {}
func (*runtimeRecordingsRecorderStub) Flush() error                        { return nil }
func (*runtimeRecordingsRecorderStub) Err() error                          { return nil }
func (recorder *runtimeRecordingsRecorderStub) Finalize(time.Time) error {
	if recorder != nil && recorder.onFinalize != nil {
		recorder.onFinalize()
	}
	return recorder.finalizeErr
}

var _ recordings.RuntimeRecorder = (*runtimeRecordingsRecorderStub)(nil)
