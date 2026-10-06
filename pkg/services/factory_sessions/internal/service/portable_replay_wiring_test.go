package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestCheckpointPortableReplayWiresPublicDurableExecutionHandoff(t *testing.T) {
	t.Run("ResumeInterruptedSession", testPortableReplayResumeInterruptedSession)
	t.Run("Resume", testPortableReplayResume)
	t.Run("typed restoration failure is forwarded", testPortableReplayTypedRestorationFailure)
	t.Run("checkpoint summary without restorable state stays historical", testPortableReplayWithoutRestorableState)
}

func TestCheckpointPortableReplayApplicationCleanupClosesOwnerBeforeArtifacts(t *testing.T) {
	events := []string{}
	owner := &portableReplayRuntimeOwner{
		restorable: true,
		events:     &events,
		resumeResult: factorysessions.LifecycleControlResult{
			SessionID: "session-js-checkpoint-001",
			Outcome:   "RESUMED",
		},
	}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	products, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}

	if _, err := products.execution.Resume(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ControlRequest{RequestID: "resume-application-cleanup"},
	); err != nil {
		t.Fatalf("checkpoint Resume() error = %v", err)
	}
	if err := products.closeArtifacts(); err != nil {
		t.Fatalf("application cleanup error = %v", err)
	}
	if err := products.closeArtifacts(); err != nil {
		t.Fatalf("repeated application cleanup error = %v", err)
	}

	wantEvents := []string{"durable-owner-close", "runtime-artifacts-close"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("checkpoint application cleanup events = %v, want %v", events, wantEvents)
	}
}

func TestCheckpointPortableReplayFailedOpeningPreservesPartialCleanup(t *testing.T) {
	t.Parallel()
	openingErr := errors.New("partial replay resource opening failed")
	artifactErr := errors.New("partial replay artifact release failed")
	var events []string
	owner := &portableReplayRuntimeOwner{restorable: true, events: &events}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	factory.initialActivation = portableReplayRuntimeAssemblerStub{
		runtime: &portableReplayRuntimeRecord{closeArtifacts: func() error {
			events = append(events, "partial-runtime-close")
			return artifactErr
		}},
		err: openingErr,
	}.Open
	products, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = products.execution.Resume(t.Context(), "session-js-checkpoint-001",
		factorysessions.ControlRequest{RequestID: "resume-partial-opening"})
	if !errors.Is(err, openingErr) {
		t.Fatalf("resume error = %v, want original opening cause", err)
	}
	if owner.resumeCalls != 0 || len(events) != 0 {
		t.Fatalf("failed opening resumed or closed outside its owner: resumes=%d events=%v", owner.resumeCalls, events)
	}
	for range 2 {
		if err := products.closeArtifacts(); !errors.Is(err, artifactErr) {
			t.Fatalf("application cleanup = %v, want retained partial artifact cause", err)
		}
	}
	if !reflect.DeepEqual(events, []string{"durable-owner-close", "partial-runtime-close"}) {
		t.Fatalf("partial cleanup = %v, want owner before artifacts without repeated effects", events)
	}
}

func TestPortableReplayRuntimeCleanupJoinsOwnerAndArtifactErrors(t *testing.T) {
	ownerErr := errors.New("durable owner close failed")
	artifactErr := errors.New("replay artifacts close failed")
	events := []string{}
	owner := &portableReplayRuntimeOwner{events: &events, closeErr: ownerErr}
	cleanup := newPortableReplayRuntimeCleanup()
	cleanup.SetOwner(owner)
	cleanup.releaseScope = func() { events = append(events, "worker-scope-release") }
	cleanup.Set(portableReplayCleanupOpening(&portableReplayRuntimeRecord{
		closeArtifacts: func() error {
			events = append(events, "runtime-artifacts-close")
			return artifactErr
		},
	}))

	err := cleanup.Close()
	if !errors.Is(err, ownerErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("cleanup error = %v, want both owner and artifact errors", err)
	}
	if !reflect.DeepEqual(events, []string{"durable-owner-close", "worker-scope-release", "runtime-artifacts-close"}) {
		t.Fatalf("cleanup ordering events = %v, want owner before artifacts", events)
	}
	if err := cleanup.Close(); !errors.Is(err, ownerErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("repeated cleanup error = %v, want the joined errors", err)
	}
	if !reflect.DeepEqual(events, []string{"durable-owner-close", "worker-scope-release", "runtime-artifacts-close"}) {
		t.Fatalf("repeated cleanup ordering events = %v, want no duplicate closes", events)
	}
}

func TestCheckpointPortableReplayFailedDurableAcquisitionReleasesOwnerAndRetries(t *testing.T) {
	t.Parallel()
	for _, failsClose := range []bool{false, true} {
		t.Run(map[bool]string{false: "released", true: "cleanup error retained"}[failsClose], func(t *testing.T) {
			t.Parallel()
			failure := errors.New("durable acquisition failed after opening resources")
			var closeErr error
			if failsClose {
				closeErr = errors.New("durable resource release failed")
			}
			var events []string
			failedOwner := &portableReplayRuntimeOwner{events: &events, closeErr: closeErr}
			retryOwner := &portableReplayRuntimeOwner{}
			factory := newPortableCheckpointRuntimeOpeningFactory(t, retryOwner)
			acquire := factory.durableExecutionFactory
			attempts := 0
			factory.durableExecutionFactory = func(definition factorydefinitions.RuntimeSelection,
				policy factorysessions.PersistencePolicy, home, path string,
				defaults operatorconfig.ResolvedDefaults, root RuntimeRoot, clock factoryruntime.Clock,
				provider providers.Service, mocks *workers.MockWorkersConfig,
				identities factorysessions.ProviderIdentityResolver,
			) (DurableExecution, error) {
				attempts++
				if attempts == 1 {
					return DurableExecution{Service: failedOwner}, failure
				}
				return acquire(definition, policy, home, path, defaults, root, clock, provider, mocks, identities)
			}
			request := portableCheckpointOwnerFixture(t).startRequest()
			failed, err := factory.openForRequest(t.Context(), request)
			if !errors.Is(err, failure) || (failsClose && !errors.Is(err, closeErr)) {
				t.Fatalf("failed opening error = %v, want acquisition and cleanup causes", err)
			}
			if failed.execution != nil || failed.process != nil {
				t.Fatal("failed acquisition published usable session roles")
			}
			if !reflect.DeepEqual(events, []string{"durable-owner-close"}) {
				t.Fatalf("failed owner cleanup = %v, want immediate release", events)
			}
			if failsClose {
				if failed.closeArtifacts == nil {
					t.Fatal("failed release lost its owned cleanup handle")
				}
				if err := failed.closeArtifacts(); !errors.Is(err, closeErr) {
					t.Fatalf("retained cleanup error = %v, want release cause", err)
				}
			}
			retried, err := factory.openForRequest(t.Context(), request)
			if err != nil || retried.execution == nil || attempts != 2 {
				t.Fatalf("same-request retry = %v, attempts %d, execution present %v", err, attempts, retried.execution != nil)
			}
			if err := retried.closeArtifacts(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(events, []string{"durable-owner-close"}) {
				t.Fatalf("stale cleanup or retry closed failed owner again: %v", events)
			}
		})
	}
}

func testPortableReplayResumeInterruptedSession(t *testing.T) {
	owner := &portableReplayRuntimeOwner{
		restorable: true,
		resumeInterruptedResult: factorysessions.AsyncStartResult{
			SessionID: "session-js-checkpoint-001",
			Status:    "RESUMED",
		},
		pauseResult: factorysessions.LifecycleControlResult{
			SessionID: "session-js-checkpoint-001",
			Outcome:   "PAUSED",
		},
	}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	opened, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.execution
	assertPortableReplayControlWalled(t, execution)

	got, err := execution.ResumeInterruptedSession(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ResumeSessionRequest{RequestID: "resume-1"},
	)
	if err != nil {
		t.Fatalf("ResumeInterruptedSession() error = %v", err)
	}
	if !reflect.DeepEqual(got, owner.resumeInterruptedResult) {
		t.Fatalf("ResumeInterruptedSession() = %#v, want %#v", got, owner.resumeInterruptedResult)
	}
	if owner.probeCalls != 1 || owner.resumeInterruptedCalls != 1 {
		t.Fatalf("owner calls = probe:%d resumeInterrupted:%d, want 1:1", owner.probeCalls, owner.resumeInterruptedCalls)
	}

	if _, err := execution.Pause(t.Context(), "session-js-checkpoint-001", factorysessions.ControlRequest{RequestID: "pause-1"}); err != nil {
		t.Fatalf("Pause() after handoff error = %v", err)
	}
	if owner.pauseCalls != 1 {
		t.Fatalf("owner pause calls = %d, want 1 after handoff", owner.pauseCalls)
	}
}

func testPortableReplayResume(t *testing.T) {
	owner := &portableReplayRuntimeOwner{
		restorable: true,
		resumeResult: factorysessions.LifecycleControlResult{
			SessionID: "session-js-checkpoint-001",
			Outcome:   "RESUMED",
		},
	}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	opened, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.execution
	assertPortableReplayControlWalled(t, execution)

	got, err := execution.Resume(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ControlRequest{RequestID: "resume-2"},
	)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if got != owner.resumeResult {
		t.Fatalf("Resume() = %#v, want %#v", got, owner.resumeResult)
	}
	if owner.probeCalls != 1 || owner.resumeCalls != 1 {
		t.Fatalf("owner calls = probe:%d resume:%d, want 1:1", owner.probeCalls, owner.resumeCalls)
	}
	if owner.childExecutionCalls != 1 || !owner.childCompleted {
		t.Fatalf("resumed child execution = calls:%d completed:%v, want one completed child", owner.childExecutionCalls, owner.childCompleted)
	}
	if owner.workerRuntimeID != "portable-replay-runtime" || owner.workerGenerationID != "portable-replay-generation" {
		t.Fatalf("child execution identity = runtime:%q generation:%q, want portable replay identities", owner.workerRuntimeID, owner.workerGenerationID)
	}
	if !owner.liveChangeBound || owner.progressPublisher == nil || owner.attemptStarter == nil || !owner.attemptStarted || !owner.attemptCompleted {
		t.Fatalf("resumed child bindings = liveChange:%v progress:%v attemptStarter:%v started:%v completed:%v, want all live bindings", owner.liveChangeBound, owner.progressPublisher != nil, owner.attemptStarter != nil, owner.attemptStarted, owner.attemptCompleted)
	}
}

func testPortableReplayTypedRestorationFailure(t *testing.T) {
	want := &factorysessions.DurableResumeError{
		Outcome:   factorysessions.DurableResumeOutcomeCorruptedPersistence,
		Field:     "checkpointSummary",
		SessionID: "session-js-checkpoint-001",
		Message:   "checkpoint state is unavailable",
	}
	owner := &portableReplayRuntimeOwner{probeErr: want}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	opened, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.execution

	_, err = execution.ResumeInterruptedSession(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ResumeSessionRequest{RequestID: "resume-typed"},
	)
	var got *factorysessions.DurableResumeError
	if !errors.As(err, &got) || got != want {
		t.Fatalf("ResumeInterruptedSession() error = %T %#v, want forwarded %#v", err, err, want)
	}
	if owner.resumeInterruptedCalls != 0 {
		t.Fatalf("owner resume calls = %d, want 0 after probe failure", owner.resumeInterruptedCalls)
	}
}

func testPortableReplayWithoutRestorableState(t *testing.T) {
	owner := &portableReplayRuntimeOwner{}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	opened, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.execution

	_, err = execution.ResumeInterruptedSession(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ResumeSessionRequest{RequestID: "resume-unavailable"},
	)
	if !errors.Is(err, recordingreplay.ErrNonLiveReplay) {
		t.Fatalf("ResumeInterruptedSession() error = %v, want ErrNonLiveReplay", err)
	}
	if owner.probeCalls != 1 || owner.resumeInterruptedCalls != 0 {
		t.Fatalf("owner calls = probe:%d resumeInterrupted:%d, want 1:0", owner.probeCalls, owner.resumeInterruptedCalls)
	}
}

func TestCheckpointPortableReplayWiresPublicDispatchHandoff(t *testing.T) {
	sessionID := "session-js-checkpoint-001"
	owner := &portableReplayRuntimeOwner{
		restorable: true,
		resumeResult: factorysessions.LifecycleControlResult{
			SessionID: sessionID,
			Outcome:   "RESUMED",
		},
		listResult: factorysessions.ListDispatchesResult{
			SessionID: sessionID,
			Dispatches: []factorysessions.DispatchSummary{{
				ID:     "restored-dispatch",
				Status: factorysessions.DispatchStatus("RUNNING"),
			}},
		},
		queryResult: factorysessions.ListDispatchesResult{
			SessionID: sessionID,
			Dispatches: []factorysessions.DispatchSummary{{
				ID:     "filtered-restored-dispatch",
				Status: factorysessions.DispatchStatus("COMPLETED"),
			}},
		},
	}
	factory := newPortableCheckpointRuntimeOpeningFactory(t, owner)
	opened, err := factory.openForRequest(t.Context(), portableCheckpointOwnerFixture(t).startRequest())
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.execution
	assertHistoricalDispatchReads(t, execution, owner, sessionID)
	assertUnknownDispatchReads(t, execution)

	if _, err := execution.Resume(t.Context(), sessionID, factorysessions.ControlRequest{RequestID: "resume-dispatches"}); err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	assertLiveDispatchReads(t, execution, owner, sessionID)
}

func assertHistoricalDispatchReads(
	t *testing.T,
	execution factorysessions.DurableExecutionService,
	owner *portableReplayRuntimeOwner,
	sessionID string,
) {
	t.Helper()
	historical, err := execution.ListDispatches(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("historical ListDispatches() error = %v", err)
	}
	if historical.SessionID != sessionID || historical.Dispatches == nil || len(historical.Dispatches) != 0 {
		t.Fatalf("historical ListDispatches() = %#v, want a non-nil empty result", historical)
	}

	filtered, err := execution.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{
		SessionID: sessionID,
		Filters: factorysessions.DispatchFilters{
			Phase:  "omitted-phase",
			Status: factorysessions.DispatchStatus("COMPLETED"),
		},
	})
	if err != nil {
		t.Fatalf("historical QueryDispatches() error = %v", err)
	}
	if filtered.SessionID != sessionID || filtered.Dispatches == nil || len(filtered.Dispatches) != 0 {
		t.Fatalf("historical QueryDispatches() = %#v, want a non-nil empty result", filtered)
	}
	if owner.listCalls != 0 || owner.queryCalls != 0 {
		t.Fatalf("historical dispatch reads reached live owner: list=%d query=%d", owner.listCalls, owner.queryCalls)
	}
}

func assertUnknownDispatchReads(t *testing.T, execution factorysessions.DurableExecutionService) {
	t.Helper()
	if _, err := execution.ListDispatches(t.Context(), "missing-session"); !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("unknown ListDispatches() error = %v, want ErrDurableSessionNotFound", err)
	}
	if _, err := execution.QueryDispatches(t.Context(), factorysessions.DispatchQueryRequest{SessionID: "missing-session"}); !errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatalf("unknown QueryDispatches() error = %v, want ErrDurableSessionNotFound", err)
	}
}

func assertLiveDispatchReads(
	t *testing.T,
	execution factorysessions.DurableExecutionService,
	owner *portableReplayRuntimeOwner,
	sessionID string,
) {
	t.Helper()
	live, err := execution.ListDispatches(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("live ListDispatches() error = %v", err)
	}
	if !reflect.DeepEqual(live, owner.listResult) || owner.listCalls != 1 {
		t.Fatalf("live ListDispatches() = %#v, calls = %d, want %#v and one live call", live, owner.listCalls, owner.listResult)
	}

	queryRequest := factorysessions.DispatchQueryRequest{
		SessionID: sessionID,
		Filters:   factorysessions.DispatchFilters{Status: factorysessions.DispatchStatus("COMPLETED")},
	}
	liveFiltered, err := execution.QueryDispatches(t.Context(), queryRequest)
	if err != nil {
		t.Fatalf("live QueryDispatches() error = %v", err)
	}
	if !reflect.DeepEqual(liveFiltered, owner.queryResult) || owner.queryCalls != 1 || !reflect.DeepEqual(owner.queryRequest, queryRequest) {
		t.Fatalf("live QueryDispatches() = %#v, calls = %d, request = %#v; want %#v, one call, and %#v", liveFiltered, owner.queryCalls, owner.queryRequest, owner.queryResult, queryRequest)
	}
}

func assertPortableReplayControlWalled(t *testing.T, execution factorysessions.DurableExecutionService) {
	t.Helper()
	_, err := execution.Pause(t.Context(), "session-js-checkpoint-001", factorysessions.ControlRequest{RequestID: "pause-before-resume"})
	if !errors.Is(err, recordingreplay.ErrNonLiveReplay) {
		t.Fatalf("Pause() before handoff error = %v, want ErrNonLiveReplay", err)
	}
}

func portableCheckpointOwnerFixture(t *testing.T) *runtimeOwnerFixture {
	t.Helper()
	return &runtimeOwnerFixture{
		FactoryDefinition: factorydefinitions.RuntimeSelection{Directory: t.TempDir()},
		Recordings:        recordings.RuntimeSelection{ReplayPath: "checkpoint.json"},
	}
}

func newPortableCheckpointRuntimeOpeningFactory(t *testing.T, owner *portableReplayRuntimeOwner) *Root {
	t.Helper()
	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2-checkpoint.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint recording: %v", err)
	}
	portable, err := recordings.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode checkpoint recording: %v", err)
	}
	var events []string
	replayInputs := &historicalReplayInputsRecorder{portable: portable, events: &events}
	recordingsRoot := &recordingsRootConstructionStub{replayInputs: replayInputs}
	calls := 0
	dependencies := validRuntimeOpeningCollaborators(&calls)
	dependencies.RecordingsService = recordingsRoot
	dependencies.RecordingsRuntime = recordingsRoot
	dependencies.RuntimeRoot = &replayRoutingRoot{}
	dependencies.ResolveClock = func(clock factoryruntime.Clock) factoryruntime.Clock {
		return clock
	}
	dependencies.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
		return zap.NewNop()
	}
	runtimeRecord := &portableReplayRuntimeRecord{
		service:    &portableReplayRuntimeService{},
		generation: "portable-replay-generation",
		progress:   func(workers.ProgressFragment) {},
		closeArtifacts: func() error {
			if owner.events != nil {
				*owner.events = append(*owner.events, "runtime-artifacts-close")
			}
			return nil
		},
	}
	dependencies.InitialActivation = portableReplayRuntimeAssemblerStub{runtime: runtimeRecord}.Open
	dependencies.Definitions = activationDefinitionsStub{snapshot: activationSnapshot()}
	dependencies.GenerateRuntimeInstanceID = func() string {
		return "portable-replay-runtime"
	}
	dependencies.WorkerService = &portableReplayWorkerService{}
	owner.workerExecution = dependencies.WorkerService
	dependencies.DurableExecutionFactory = func(
		_ factorydefinitions.RuntimeSelection,
		_ factorysessions.PersistencePolicy,
		_ string,
		_ string,
		_ operatorconfig.ResolvedDefaults,
		_ RuntimeRoot,
		_ factoryruntime.Clock,
		_ providers.Service,
		_ *workers.MockWorkersConfig,
		_ factorysessions.ProviderIdentityResolver,
	) (DurableExecution, error) {
		return DurableExecution{Service: owner}, nil
	}
	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	factory.namedPaths = nil
	return factory
}

type portableReplayRuntimeOwner struct {
	durableexecution.Service
	restorable bool
	probeErr   error

	resumeInterruptedResult factorysessions.AsyncStartResult
	resumeInterruptedErr    error
	resumeResult            factorysessions.LifecycleControlResult
	resumeErr               error
	pauseResult             factorysessions.LifecycleControlResult
	pauseErr                error
	listResult              factorysessions.ListDispatchesResult
	queryResult             factorysessions.ListDispatchesResult
	queryRequest            factorysessions.DispatchQueryRequest
	liveChangeBound         bool
	workerExecution         interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	}
	progressPublisher   workers.ProgressPublisher
	attemptStarter      func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)
	workerRuntimeID     string
	workerGenerationID  string
	childExecutionCalls int
	childCompleted      bool
	attemptStarted      bool
	attemptCompleted    bool
	events              *[]string
	closeErr            error

	probeCalls             int
	resumeInterruptedCalls int
	resumeCalls            int
	pauseCalls             int
	listCalls              int
	queryCalls             int
}

func (owner *portableReplayRuntimeOwner) HasRestorableState(context.Context, string) (bool, error) {
	owner.probeCalls++
	return owner.restorable, owner.probeErr
}

func (owner *portableReplayRuntimeOwner) Close() error {
	if owner.events != nil {
		*owner.events = append(*owner.events, "durable-owner-close")
	}
	return owner.closeErr
}

func (owner *portableReplayRuntimeOwner) ResumeInterruptedSession(
	context.Context,
	string,
	factorysessions.ResumeSessionRequest,
) (factorysessions.AsyncStartResult, error) {
	owner.resumeInterruptedCalls++
	return owner.resumeInterruptedResult, owner.resumeInterruptedErr
}

func (owner *portableReplayRuntimeOwner) Resume(
	ctx context.Context,
	sessionID string,
	request factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	owner.resumeCalls++
	if owner.workerExecution != nil {
		executionRequest := workers.ExecuteRequest{
			Correlation: workers.ExecutionCorrelation{
				FactorySessionID: sessionID,
				RuntimeID:        owner.workerRuntimeID,
				GenerationID:     owner.workerGenerationID,
				DispatchID:       "restored-dispatch",
				AttemptID:        "restored-attempt",
				RequestID:        request.RequestID,
			},
		}
		var terminal func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error)
		var err error
		if owner.attemptStarter != nil {
			terminal, err = owner.attemptStarter(ctx, &executionRequest)
			if err != nil {
				return factorysessions.LifecycleControlResult{}, err
			}
			owner.attemptStarted = true
		}
		result, err := owner.workerExecution.Execute(ctx, executionRequest)
		if err != nil {
			if terminal != nil {
				_, _ = terminal(ctx, result, err)
			}
			return factorysessions.LifecycleControlResult{}, err
		}
		owner.childExecutionCalls++
		if terminal != nil {
			if _, err := terminal(ctx, result, nil); err != nil {
				return factorysessions.LifecycleControlResult{}, err
			}
			owner.attemptCompleted = true
		}
		owner.childCompleted = true
	}
	return owner.resumeResult, owner.resumeErr
}

func (owner *portableReplayRuntimeOwner) Pause(
	context.Context,
	string,
	factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	owner.pauseCalls++
	return owner.pauseResult, owner.pauseErr
}

func (owner *portableReplayRuntimeOwner) ListDispatches(
	context.Context,
	string,
) (factorysessions.ListDispatchesResult, error) {
	owner.listCalls++
	return owner.listResult, nil
}

func (owner *portableReplayRuntimeOwner) QueryDispatches(
	_ context.Context,
	request factorysessions.DispatchQueryRequest,
) (factorysessions.ListDispatchesResult, error) {
	owner.queryCalls++
	owner.queryRequest = request
	return owner.queryResult, nil
}

func (*portableReplayRuntimeOwner) RecordPetriTokenMutations(
	string,
	[]factorydefinitions.TokenMutationRecord,
) error {
	return nil
}

func (owner *portableReplayRuntimeOwner) BindLiveChangeScope(string, factorysessions.LiveChangeApplication, factorysessions.LiveChangeAdmission, bool, func(int)) func() {
	owner.liveChangeBound = true
	return func() { owner.liveChangeBound = false }
}

func (owner *portableReplayRuntimeOwner) BindWorkerScope(
	_ string,
	_ factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	_ providers.Service,
	_ *workers.MockWorkersConfig,
	_ platformprocess.CommandRunner,
	publisher workers.ProgressPublisher,
	starter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) (func(), error) {
	owner.progressPublisher = publisher
	owner.attemptStarter = starter
	owner.workerRuntimeID = runtimeID
	owner.workerGenerationID = generationID
	return func() {}, nil
}

type portableReplayWorkerService struct {
	workers.Service
}

func (*portableReplayWorkerService) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, nil
}

type portableReplayRuntimeService struct {
	factoryruntime.Service
}

type portableReplayRuntimeRecord struct {
	inertHostedInstance
	service        factoryruntime.Service
	generation     string
	progress       workers.ProgressPublisher
	closeArtifacts func() error
}

func (record *portableReplayRuntimeRecord) RuntimeService() factoryruntime.Service {
	return record.service
}

func (record *portableReplayRuntimeRecord) StreamGeneration() string {
	return record.generation
}

func (record *portableReplayRuntimeRecord) RuntimeProgressPublisher() workers.ProgressPublisher {
	return record.progress
}

func (record *portableReplayRuntimeRecord) CloseArtifacts() error {
	if record.closeArtifacts == nil {
		return nil
	}
	return record.closeArtifacts()
}

func (*portableReplayRuntimeRecord) BeginWorkerAttempt(
	context.Context,
	*workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
	return func(_ context.Context, result workers.ExecuteResult, err error) (workers.ExecuteResult, error) {
		return result, err
	}, nil
}

type portableReplayRuntimeAssemblerStub struct {
	runtime runtimeports.RuntimeInstance
	err     error
}

func (assembler portableReplayRuntimeAssemblerStub) Open(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
	factoryruntime.SessionObservations,
) (*factoryruntime.RuntimeInitialOpening, error) {
	return &factoryruntime.RuntimeInitialOpening{Record: assembler.runtime,
		Activation: &factoryruntime.RuntimeActivation{Close: func(context.Context) error {
			if assembler.runtime == nil {
				return nil
			}
			return assembler.runtime.CloseArtifacts()
		}},
	}, assembler.err
}

var _ durableexecution.Service = (*portableReplayRuntimeOwner)(nil)

func TestOpenForRequestConsumesResumeSourceBeforeLiveSuccessorActivation(t *testing.T) {
	t.Parallel()

	root := &resumeRoutingRoot{}
	factorySnapshot := factorydefinitions.FactorySnapshot(`{"factoryDirectory":"/factory","name":"legacy"}`)
	resumeInput := recordings.LoadResumeInputResult{
		RecoveryMetadata: recordings.ResumeRecoveryMetadata{
			SourceRecordingID:    "sha256:source-recording",
			RecordedDefinitionID: "sha256:recorded-definition",
			PreviousRecordedAt:   time.Date(2026, 9, 19, 18, 0, 0, 0, time.UTC),
		},
		Input: recordings.LoadReplayInputResult{
			Legacy: &factorydefinitions.ReplayArtifact{
				Factory: &factorySnapshot,
				Events: []factorydefinitions.FactoryEvent{{
					Id:      "resume-event",
					Context: factorydefinitions.FactoryEventContext{Tick: 7},
				}},
			},
		},
	}
	resumeRuntime := &resumeInputRuntime{result: resumeInput}
	factory := &Root{
		runtimeRoot:               root,
		recordingsRuntime:         resumeRuntime,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
		decodeReplayConfig: func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{}, nil
		},
	}
	opened, err := factory.openForRequest(context.Background(), (runtimeOwnerFixture{
		FactoryDefinition: factorydefinitions.RuntimeSelection{Directory: "/factory"},
		Recordings: recordings.RuntimeSelection{
			RecordPath: "successor.recording.json",
			ResumePath: "source.recording.json",
		},
	}).startRequest())
	if err != nil {
		t.Fatalf("openForRequest(resume) error = %v", err)
	}
	if resumeRuntime.path != "source.recording.json" {
		t.Fatalf("resume source path = %q, want source.recording.json", resumeRuntime.path)
	}
	if resumeRuntime.calls != 1 {
		t.Fatalf("resume source loads = %d, want one", resumeRuntime.calls)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want one", root.activations)
	}
	if root.activation.Inputs.ResumeInput != resumeInput {
		t.Fatalf("activation resume input = %#v, want %#v", root.activation.Inputs.ResumeInput, resumeInput)
	}
	if len(root.activation.Inputs.ResumeInput.Input.Legacy.Events) != 1 ||
		root.activation.Inputs.ResumeInput.Input.Legacy.Events[0].Id != "resume-event" {
		t.Fatalf("activation resume events = %#v, want selected recording event", root.activation.Inputs.ResumeInput.Input.Legacy.Events)
	}
	if root.activation.Inputs.Recordings.ResumePath != "source.recording.json" {
		t.Fatalf("activation resume path = %q, want source.recording.json", root.activation.Inputs.Recordings.ResumePath)
	}
	if root.activation.Inputs.Recordings.RecordPath != "successor.recording.json" {
		t.Fatalf("activation successor path = %q, want successor.recording.json", root.activation.Inputs.Recordings.RecordPath)
	}
	if root.activation.Inputs.Recordings.ReplayPath != "" {
		t.Fatalf("activation replay path = %q, want empty for resume", root.activation.Inputs.Recordings.ReplayPath)
	}
	assertResumeRecoveryMetadata(t, opened.resumeRecoveryMetadata, resumeInput.RecoveryMetadata)
}

func assertResumeRecoveryMetadata(
	t *testing.T,
	metadata *recordings.ResumeRecoveryMetadata,
	want recordings.ResumeRecoveryMetadata,
) {
	t.Helper()
	if metadata == nil || metadata.SourceRecordingID != want.SourceRecordingID ||
		metadata.RecordedDefinitionID != want.RecordedDefinitionID ||
		metadata.SuccessorRecordingID != recoveryRecordingID("runtime-1") ||
		!metadata.PreviousRecordedAt.Equal(want.PreviousRecordedAt) {
		t.Fatalf("opened resume recovery metadata = %#v, want selected source and successor identities", metadata)
	}
}

func portableReplayCleanupOpening(record runtimeports.RuntimeInstance) *factoryruntime.RuntimeInitialOpening {
	return &factoryruntime.RuntimeInitialOpening{Record: record,
		Activation: &factoryruntime.RuntimeActivation{Close: func(context.Context) error { return record.CloseArtifacts() }},
	}
}

// Replay keeps explicit provider selection and request mock policy while nil
// selection inherits the already composed Workers provider. Acquisition errors
// still leave no usable durable owner.
func TestPortableReplayDurableOwnerPreservesProviderSelectionAndFailure(t *testing.T) {
	t.Parallel()
	selected := testutil.NewMockProvider(workers.InferenceResponse{Content: "selected"})
	failure := errors.New("durable persistence unavailable")
	for _, tc := range []struct {
		name     string
		provider providers.Service
		failure  error
	}{{name: "inherit"}, {name: "explicit", provider: selected}, {name: "acquisition failure", provider: selected, failure: failure}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			configured := preparedRuntime{}
			configured.Workers.MockWorkers = &workers.MockWorkersConfig{UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough}
			owner := &portableReplayRuntimeOwner{}
			acquire := func(_ factorydefinitions.RuntimeSelection, _ factorysessions.PersistencePolicy,
				_, _ string, _ operatorconfig.ResolvedDefaults, _ RuntimeRoot, _ factoryruntime.Clock,
				provider providers.Service, mocks *workers.MockWorkersConfig,
				_ factorysessions.ProviderIdentityResolver,
			) (DurableExecution, error) {
				if provider != tc.provider {
					t.Fatalf("provider = %v, want selected %v", provider, tc.provider)
				}
				if mocks == nil || !mocks.UnmatchedDispatchPolicy.PassthroughUnmatched() {
					t.Fatal("request mock policy was lost")
				}
				if tc.failure != nil {
					return DurableExecution{}, tc.failure
				}
				return DurableExecution{Service: owner}, nil
			}
			durable, provider, err := constructPortableReplayDurableOwner(configured, RuntimeRoot{}, openingCoordinatorClock{}, tc.provider, acquire, nil)
			if !errors.Is(err, tc.failure) {
				t.Fatalf("error = %v, want %v", err, tc.failure)
			}
			if tc.failure != nil {
				if durable.Service != nil || provider != nil {
					t.Fatal("failed acquisition returned usable owner/provider")
				}
				return
			}
			if durable.Service != owner || provider != tc.provider {
				t.Fatalf("acquisition result = %v/%v, want admitted owner and selected provider", durable.Service, provider)
			}
		})
	}
}

func TestDurableCapabilityRegistrationOwnsCompletedFlushRelease(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "failed worker registration"}[fail], func(t *testing.T) {
			owner := &durabilityRegistrationOwner{workerErr: nil}
			if fail {
				owner.workerErr = errors.New("worker registration failed")
			}
			release, err := bindDurableExecutionCapabilities("session-owned-flush", owner, &portableReplayRuntimeService{}, nil,
				"runtime-owned-flush", "generation-owned-flush", nil, nil, nil, nil, nil, nil)
			if fail {
				if !errors.Is(err, owner.workerErr) || release != nil || len(owner.events) != 0 {
					t.Fatalf("failed registration = %v, release present %v, events %v", err, release != nil, owner.events)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			release()
			if !reflect.DeepEqual(owner.events, []string{"live-change-register", "flush-register", "worker-release", "flush-release", "live-change-release"}) {
				t.Fatalf("registration lifecycle = %v", owner.events)
			}
		})
	}
}

type durabilityRegistrationOwner struct {
	portableReplayRuntimeOwner
	workerErr error
	events    []string
}

func (owner *durabilityRegistrationOwner) BindWorkerScope(
	string, factoryruntime.ResourceCapacityLeaseAdmission, string, string, providers.Service,
	*workers.MockWorkersConfig, platformprocess.CommandRunner, workers.ProgressPublisher,
	func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) (func(), error) {
	if owner.workerErr != nil {
		return nil, owner.workerErr
	}
	return func() { owner.events = append(owner.events, "worker-release") }, nil
}

func (owner *durabilityRegistrationOwner) BindDispatchDurability(
	string, recordings.CompletedFlushWatermarkReader, string,
) func() {
	owner.events = append(owner.events, "flush-register")
	return func() { owner.events = append(owner.events, "flush-release") }
}

func (owner *durabilityRegistrationOwner) BindLiveChangeScope(
	string, factorysessions.LiveChangeApplication, factorysessions.LiveChangeAdmission, bool, func(int),
) func() {
	owner.events = append(owner.events, "live-change-register")
	return func() { owner.events = append(owner.events, "live-change-release") }
}
