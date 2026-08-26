package runtimeopening

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
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
	products, err := factory.openForRequest(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("openForRequest() error = %v", err)
	}

	if _, err := products.execution.Execution.Resume(
		t.Context(),
		"session-js-checkpoint-001",
		factorysessions.ControlRequest{RequestID: "resume-application-cleanup"},
	); err != nil {
		t.Fatalf("checkpoint Resume() error = %v", err)
	}
	if err := products.application.Resources.Close(); err != nil {
		t.Fatalf("application cleanup error = %v", err)
	}
	if err := products.application.Resources.Close(); err != nil {
		t.Fatalf("repeated application cleanup error = %v", err)
	}

	wantEvents := []string{"durable-owner-close", "runtime-artifacts-close"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("checkpoint application cleanup events = %v, want %v", events, wantEvents)
	}
}

func TestPortableReplayRuntimeCleanupJoinsOwnerAndArtifactErrors(t *testing.T) {
	ownerErr := errors.New("durable owner close failed")
	artifactErr := errors.New("replay artifacts close failed")
	events := []string{}
	owner := &portableReplayRuntimeOwner{events: &events, closeErr: ownerErr}
	cleanup := newPortableReplayRuntimeCleanup()
	cleanup.SetOwner(owner)
	cleanup.Set(&portableReplayRuntimeRecord{
		closeArtifacts: func() error {
			events = append(events, "runtime-artifacts-close")
			return artifactErr
		},
	})

	err := cleanup.Close()
	if !errors.Is(err, ownerErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("cleanup error = %v, want both owner and artifact errors", err)
	}
	if !reflect.DeepEqual(events, []string{"durable-owner-close", "runtime-artifacts-close"}) {
		t.Fatalf("cleanup ordering events = %v, want owner before artifacts", events)
	}
	if err := cleanup.Close(); !errors.Is(err, ownerErr) || !errors.Is(err, artifactErr) {
		t.Fatalf("repeated cleanup error = %v, want the joined errors", err)
	}
	if !reflect.DeepEqual(events, []string{"durable-owner-close", "runtime-artifacts-close"}) {
		t.Fatalf("repeated cleanup ordering events = %v, want no duplicate closes", events)
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
	opened, err := factory.OpenExecutionRuntime(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.Execution
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
	opened, err := factory.OpenInvocationRuntime(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("OpenInvocationRuntime() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.Execution
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
	if owner.workerInvoker == nil || owner.progressPublisher == nil || owner.attemptStarter == nil || !owner.attemptStarted || !owner.attemptCompleted {
		t.Fatalf("resumed child bindings = invoker:%v progress:%v attemptStarter:%v started:%v completed:%v, want all live bindings", owner.workerInvoker != nil, owner.progressPublisher != nil, owner.attemptStarter != nil, owner.attemptStarted, owner.attemptCompleted)
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
	opened, err := factory.OpenExecutionRuntime(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.Execution

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
	opened, err := factory.OpenExecutionRuntime(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.Execution

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
	opened, err := factory.OpenExecutionRuntime(t.Context(), portableCheckpointRuntimeOpeningRequest(t))
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	var execution factorysessions.DurableExecutionService = opened.Execution
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

func portableCheckpointRuntimeOpeningRequest(t *testing.T) *factorysessions.RuntimeOpeningRequest {
	t.Helper()
	return &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
		Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "checkpoint.json"},
	}
}

func newPortableCheckpointRuntimeOpeningFactory(t *testing.T, owner *portableReplayRuntimeOwner) *Factory {
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
	dependencies := validRuntimeOpeningOwnerPorts(&calls)
	dependencies.Recordings.Service = recordingsRoot
	dependencies.Recordings.Runtime = recordingsRoot
	dependencies.FactoryRuntime.ResolveClock = func(clock factoryruntime.Clock) factoryruntime.Clock {
		return clock
	}
	dependencies.FactoryRuntime.NewSessionLogger = func(*zap.Logger, string, string, string) *zap.Logger {
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
	dependencies.FactoryRuntime.FactoryRuntimeAssembler = portableReplayRuntimeAssemblerStub{runtime: runtimeRecord}
	dependencies.FactorySessions.GenerateRuntimeInstanceID = func() string {
		return "portable-replay-runtime"
	}
	dependencies.Workers.Service = &portableReplayWorkerService{}
	dependencies.FactorySessions.DurableExecutionFactory = func(
		_ factorydefinitions.RuntimeOpeningRequest,
		_ factorysessions.SessionRuntimeOpeningRequest,
		_ operatorconfig.ResolvedDefaults,
		_ RuntimeRoot,
		_ factoryruntime.Clock,
		_ providers.Service,
		_ *workers.MockWorkersConfig,
		_ FactorySessionExecutionFactory,
		_ factorysessions.ProviderIdentityResolver,
	) (DurableExecution, error) {
		return DurableExecution{Service: owner}, nil
	}
	factory, err := dependencies.newFactory()
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
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
	workerInvoker           factoryruntime.Service
	workerExecution         interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	}
	progressPublisher   workers.ProgressPublisher
	attemptStarter      func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error)
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
		var terminal func(context.Context, workers.ExecuteResult, error) error
		var err error
		if owner.attemptStarter != nil {
			terminal, err = owner.attemptStarter(ctx, executionRequest)
			if err != nil {
				return factorysessions.LifecycleControlResult{}, err
			}
			owner.attemptStarted = true
		}
		result, err := owner.workerExecution.Execute(ctx, executionRequest)
		if err != nil {
			if terminal != nil {
				_ = terminal(ctx, result, err)
			}
			return factorysessions.LifecycleControlResult{}, err
		}
		owner.childExecutionCalls++
		if terminal != nil {
			if err := terminal(ctx, result, nil); err != nil {
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

func (owner *portableReplayRuntimeOwner) SetWorkerInvoker(runtime factoryruntime.Service) {
	owner.workerInvoker = runtime
}

func (owner *portableReplayRuntimeOwner) SetWorkerExecution(
	execution interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	},
	_ factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	_ providers.Service,
	_ *workers.MockWorkersConfig,
	_ platformprocess.CommandRunner,
) {
	owner.workerExecution = execution
	owner.workerRuntimeID = runtimeID
	owner.workerGenerationID = generationID
}

func (owner *portableReplayRuntimeOwner) SetWorkerProgressPublisher(publisher workers.ProgressPublisher) {
	owner.progressPublisher = publisher
}

func (owner *portableReplayRuntimeOwner) SetWorkerAttemptStarter(
	starter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) {
	owner.attemptStarter = starter
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
	workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) error, error) {
	return func(context.Context, workers.ExecuteResult, error) error { return nil }, nil
}

type portableReplayRuntimeAssemblerStub struct {
	runtime runtimeports.RuntimeInstance
}

func (assembler portableReplayRuntimeAssemblerStub) Assemble(
	context.Context,
	string,
	string,
	bool,
	string,
	string,
	string,
	factorydefinitions.WorkstationLoader,
	factoryruntime.LoadedFactoryLoader,
	providers.Service,
	platformprocess.CommandRunner,
	platformprocess.CommandRunner,
	*workers.MockWorkersConfig,
	factorydefinitions.RuntimeMode,
	factoryruntime.Scheduler,
	bool,
	recordings.SubmissionRecorder,
	recordings.DispatchRecorder,
	string,
	factoryruntime.RuntimeLogStorageConfig,
	factoryruntime.RuntimeFileLoggingPolicy,
	factoryruntime.RuntimeMetricsPolicy,
	string,
	factoryruntime.RuntimeMetricsStorageConfig,
	time.Duration,
	string,
	string,
	bool,
	bool,
	*bool,
	factoryruntime.Clock,
	*zap.Logger,
	factoryruntime.WorkersMockCommandRunnerFactory,
	func(string) workers.ProgressPublisher,
	func(string) func(string),
	factoryruntime.PetriMutationRecorder,
	factoryruntime.WorldStateProjector,
	recordings.RuntimeOpening,
	factorydefinitions.InitialFactorySnapshotFactory,
	string,
	string,
	string,
	factorydefinitions.MutableLoadedFactorySource,
	string,
	*factorydefinitions.ReplayArtifact,
	*recordings.LoadResumeInputResult,
	*factorydefinitions.FactoryWorldState,
	[]factorydefinitions.FactoryEvent,
	automations.Service,
	bool,
) (
	runtimeports.RuntimeReplacementBuilder,
	runtimeports.RuntimeInstance,
	factoryruntime.SessionBuildSpec,
	runtimeports.RuntimeLifecycle,
	runtimeports.RuntimeSidecarService,
	error,
) {
	return nil, assembler.runtime, factoryruntime.SessionBuildSpec{}, nil, nil, nil
}

var _ durableexecution.Service = (*portableReplayRuntimeOwner)(nil)
