package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func newRootContractTestFactory(t *testing.T) *factoryImpl {
	t.Helper()
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("newTestFactory: %v", err)
	}
	impl, ok := f.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", f)
	}
	return impl
}

func TestFactoryImpl_Terminate_MapsLifecycleStates(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateRunning
	got, err := impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	if err != nil {
		t.Fatalf("Terminate(running) error = %v, want nil", err)
	}
	if got.Outcome != factory.ControlOutcomeAccepted {
		t.Fatalf("Terminate(running) outcome = %q, want ACCEPTED", got.Outcome)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	if !errors.Is(err, factory.ErrAlreadyStopped) {
		t.Fatalf("Terminate(completed) error = %v, want ErrAlreadyStopped", err)
	}

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.Terminate(ctx, factory.TerminateRequest{Reason: "stop"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Terminate(unknown) error = %v, want ErrNotRunning", err)
	}
}

func TestFactoryImpl_Observe_ProjectsSanitizedObservation(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateIdle
	got, err := impl.Observe(ctx, factory.ObserveRequest{Scope: factory.ObservationScopeStatus})
	if err != nil {
		t.Fatalf("Observe(idle) error = %v, want nil", err)
	}
	if got.Observation.Status == "" {
		t.Fatal("Observe(idle) status is empty, want projected status")
	}
	if got.Observation.Progress != (factory.ObservationProgress{}) {
		t.Fatalf("Observe(STATUS) progress = %#v, want empty scoped view", got.Observation.Progress)
	}
	if len(got.Observation.InFlightDispatches) != 0 {
		t.Fatalf("Observe(STATUS) dispatches = %#v, want empty scoped view", got.Observation.InFlightDispatches)
	}

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.Observe(ctx, factory.ObserveRequest{})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("Observe(unknown) error = %v, want ErrNotRunning", err)
	}
}

func TestFactoryImpl_PlanAndAcceptDispatch_MapsLifecycleAvailability(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStateRunning
	planned, err := impl.PlanDispatch(ctx, factory.PlanDispatchRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("PlanDispatch(running) error = %v, want nil", err)
	}
	if planned != (factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcomeAccepted,
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	}) {
		t.Fatalf("PlanDispatch result = %#v, want accepted root shape", planned)
	}

	accepted, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("AcceptDispatchResult(running) error = %v, want nil", err)
	}
	if accepted.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult outcome = %q, want RETIRED", accepted.Outcome)
	}

	impl.state = interfaces.FactoryStateFailed
	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-2", CorrelationID: "corr-2"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("PlanDispatch(failed) error = %v, want ErrNotRunning", err)
	}
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-2", CorrelationID: "corr-2"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("AcceptDispatchResult(failed) error = %v, want ErrNotRunning", err)
	}

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.PlanDispatch(ctx, factory.PlanDispatchRequest{DispatchID: "dispatch-3", CorrelationID: "corr-3"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("PlanDispatch(unknown) error = %v, want ErrNotRunning", err)
	}
	_, err = impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{DispatchID: "dispatch-3", CorrelationID: "corr-3"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("AcceptDispatchResult(unknown) error = %v, want ErrNotRunning", err)
	}
}

func TestFactoryImpl_CheckpointContracts_MapLifecycleAndOpaquePayload(t *testing.T) {
	impl := newRootContractTestFactory(t)
	ctx := context.Background()

	impl.state = interfaces.FactoryStatePaused
	captured, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{})
	if err != nil {
		t.Fatalf("CaptureCheckpoint(paused) error = %v, want nil", err)
	}
	if captured.Outcome != factory.CheckpointOutcomeCaptured {
		t.Fatalf("CaptureCheckpoint outcome = %q, want CAPTURED", captured.Outcome)
	}
	if captured.Checkpoint.CheckpointID == "" || len(captured.Checkpoint.Payload) == 0 {
		t.Fatalf("CaptureCheckpoint checkpoint = %#v, want opaque payload and stub id", captured.Checkpoint)
	}

	named, err := impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-1"})
	if err != nil {
		t.Fatalf("CaptureCheckpoint(named) error = %v, want nil", err)
	}
	if named.Checkpoint.CheckpointID != "cp-1" {
		t.Fatalf("CaptureCheckpoint named id = %q, want cp-1", named.Checkpoint.CheckpointID)
	}

	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{})
	if !errors.Is(err, factory.ErrCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint(empty) error = %v, want ErrCheckpointNotFound", err)
	}
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "missing"})
	if !errors.Is(err, factory.ErrCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint(missing) error = %v, want ErrCheckpointNotFound", err)
	}

	restored, err := impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-1", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	if err != nil {
		t.Fatalf("RestoreCheckpoint(paused) error = %v, want nil", err)
	}
	if restored.Outcome != factory.CheckpointOutcomeRestored || restored.CheckpointID != "cp-1" {
		t.Fatalf("RestoreCheckpoint result = %#v, want restored cp-1", restored)
	}

	impl.state = interfaces.FactoryStateCompleted
	_, err = impl.CaptureCheckpoint(ctx, factory.CaptureCheckpointRequest{CheckpointID: "cp-2"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("CaptureCheckpoint(completed) error = %v, want ErrNotRunning", err)
	}
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-2"})
	if !errors.Is(err, factory.ErrCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint(completed) error = %v, want ErrCheckpointNotFound", err)
	}
	_, err = impl.RestoreCheckpoint(ctx, factory.RestoreCheckpointRequest{
		Checkpoint: factory.Checkpoint{CheckpointID: "cp-2", SchemaVersion: 1, Payload: []byte(`{}`)},
	})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("RestoreCheckpoint(completed) error = %v, want ErrNotRunning", err)
	}

	impl.state = interfaces.FactoryState("unknown")
	_, err = impl.LoadCheckpoint(ctx, factory.LoadCheckpointRequest{CheckpointID: "cp-3"})
	if !errors.Is(err, factory.ErrNotRunning) {
		t.Fatalf("LoadCheckpoint(unknown) error = %v, want ErrNotRunning", err)
	}
}

func TestProjectRootObservation_OmitsPetriVocabularyAndHonorsScope(t *testing.T) {
	empty := projectRootObservation(nil, factory.ObservationScopeFull)
	if empty.Status != "" || empty.Progress != (factory.ObservationProgress{}) || len(empty.InFlightDispatches) != 0 || len(empty.Results) != 0 {
		t.Fatalf("projectRootObservation(nil) = %#v, want empty", empty)
	}

	snap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus:          interfaces.RuntimeStatusActive,
		InFlightCount:          1,
		TickCount:              7,
		FactoryState:           string(interfaces.FactoryStateRunning),
		LifecycleControlStatus: "RUNNING",
		StreamGenerationID:     "gen-1",
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d1": {
				DispatchID:      "d1",
				WorkstationName: "desk",
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-1"}},
					{Color: workerexecution.Color{WorkID: ""}},
				},
			},
			"nil": nil,
		},
		DispatchHistory: []interfaces.CompletedDispatch{
			{
				DispatchID: "done-1",
				Outcome:    workerexecution.WorkOutcome("SUCCESS"),
				ConsumedTokens: []workerexecution.Token{
					{Color: workerexecution.Color{WorkID: "work-done"}},
				},
			},
		},
	}

	full := projectRootObservation(snap, factory.ObservationScopeFull)
	if full.Status != factory.ObservationStatusActive {
		t.Fatalf("status = %q, want ACTIVE", full.Status)
	}
	if full.Progress.InFlightDispatchCount != 1 || full.Progress.TickCount != 7 {
		t.Fatalf("progress = %#v, want in-flight=1 tick=7", full.Progress)
	}
	if len(full.InFlightDispatches) != 1 {
		t.Fatalf("in-flight count = %d, want 1 (nil entry skipped)", len(full.InFlightDispatches))
	}
	if len(full.InFlightDispatches[0].WorkIDs) != 1 || full.InFlightDispatches[0].WorkIDs[0] != "work-1" {
		t.Fatalf("in-flight work ids = %#v, want [work-1]", full.InFlightDispatches[0].WorkIDs)
	}
	if len(full.Results) != 1 || full.Results[0].WorkID != "work-done" {
		t.Fatalf("results = %#v, want work-done", full.Results)
	}
	if full.Health.StreamGenerationID != "gen-1" {
		t.Fatalf("health = %#v, want gen-1", full.Health)
	}

	if statusOnly := projectRootObservation(snap, factory.ObservationScopeStatus); statusOnly.Status != factory.ObservationStatusActive || statusOnly.Progress != (factory.ObservationProgress{}) {
		t.Fatalf("STATUS scope = %#v, want status-only", statusOnly)
	}
	if progressOnly := projectRootObservation(snap, factory.ObservationScopeProgress); progressOnly.Progress.TickCount != 7 || progressOnly.Status != "" {
		t.Fatalf("PROGRESS scope = %#v, want progress-only", progressOnly)
	}
	if dispatchOnly := projectRootObservation(snap, factory.ObservationScopeDispatches); len(dispatchOnly.InFlightDispatches) != 1 || dispatchOnly.Status != "" {
		t.Fatalf("DISPATCHES scope = %#v, want dispatches-only", dispatchOnly)
	}
	if resultsOnly := projectRootObservation(snap, factory.ObservationScopeResults); len(resultsOnly.Results) != 1 || resultsOnly.Status != "" {
		t.Fatalf("RESULTS scope = %#v, want results-only", resultsOnly)
	}
	if resourcesOnly := projectRootObservation(snap, factory.ObservationScopeResources); resourcesOnly.Status != "" || len(resourcesOnly.Resources) != 0 {
		t.Fatalf("RESOURCES scope = %#v, want resources-only empty", resourcesOnly)
	}
	if healthOnly := projectRootObservation(snap, factory.ObservationScopeHealth); healthOnly.Health.FactoryState == "" || healthOnly.Status != "" {
		t.Fatalf("HEALTH scope = %#v, want health-only", healthOnly)
	}
	if unknownScope := projectRootObservation(snap, factory.ObservationScope("OTHER")); unknownScope.Status != factory.ObservationStatusActive {
		t.Fatalf("unknown scope = %#v, want full observation fallback", unknownScope)
	}

	idleSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusIdle}
	if got := projectRootObservation(idleSnap, ""); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("idle status = %q, want IDLE", got.Status)
	}
	finishedSnap := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusFinished}
	if got := projectRootObservation(finishedSnap, factory.ObservationScopeFull); got.Status != factory.ObservationStatusFinished {
		t.Fatalf("finished status = %q, want FINISHED", got.Status)
	}
	unknownStatus := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatus("weird")}
	if got := projectRootObservation(unknownStatus, factory.ObservationScopeFull); got.Status != factory.ObservationStatusIdle {
		t.Fatalf("unknown runtime status = %q, want IDLE default", got.Status)
	}
}
