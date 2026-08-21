package runtimeopening

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
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
	dependencies.FactorySessions.GenerateRuntimeInstanceID = func() string {
		return "portable-replay-runtime"
	}
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

func (owner *portableReplayRuntimeOwner) ResumeInterruptedSession(
	context.Context,
	string,
	factorysessions.ResumeSessionRequest,
) (factorysessions.AsyncStartResult, error) {
	owner.resumeInterruptedCalls++
	return owner.resumeInterruptedResult, owner.resumeInterruptedErr
}

func (owner *portableReplayRuntimeOwner) Resume(
	context.Context,
	string,
	factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	owner.resumeCalls++
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

var _ durableexecution.Service = (*portableReplayRuntimeOwner)(nil)

func TestRuntimeOpeningExposesOnlyOperationSpecificOpenedViews(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"../../opened_runtime.go", "factory.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, forbidden := range []string{
			"type OpenedRuntime struct",
			") (factorysessions.OpenedRuntime, error)",
			"func (f *Factory) OpenRuntime(",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s exposes broad runtime product %q", path, forbidden)
			}
		}
	}
}

func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	m.Run()
}
