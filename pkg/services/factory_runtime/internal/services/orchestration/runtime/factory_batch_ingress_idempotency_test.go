package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestConcurrentBatchIngressSameRequestIDReplayIsIdempotent_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	batchRequest := ingressBatchRequest(
		"request-idempotent-replay",
		"work-idempotent-replay",
		"trace-idempotent-replay",
	)

	first, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, batchRequest.RequestID, "work-idempotent-replay")

	replayed, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("replay SubmitWorkRequest: %v", err)
	}
	if replayed.Accepted {
		t.Fatalf("replay accepted = true, want idempotent no-op")
	}
	if replayed.RequestID != first.RequestID || replayed.WorkID != first.WorkID {
		t.Fatalf("replay identity = %#v, want preserved %#v", replayed, first)
	}
	if countWorkRequestRecords(history, batchRequest.RequestID) != 1 {
		t.Fatalf("WORK_REQUEST count for %q = %d, want 1", batchRequest.RequestID, countWorkRequestRecords(history, batchRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, "work-idempotent-replay") != 1 {
		t.Fatalf("observed work materializations for work-idempotent-replay = %d, want 1", countObservedWorkMaterializations(snap, "work-idempotent-replay"))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func TestConcurrentBatchIngressDifferentRequestIDAfterVisibleAcceptanceDoesNotDuplicateWork_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	const workID = "work-visible-retry"
	firstRequest := ingressBatchRequest("request-visible-first", workID, "trace-visible-first")
	secondRequest := ingressBatchRequest("request-visible-retry", workID, "trace-visible-retry")

	first, err := h.Factory.SubmitWorkRequest(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, firstRequest.RequestID, workID)

	second, err := h.Factory.SubmitWorkRequest(context.Background(), secondRequest)
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("retry SubmitWorkRequest error = %v, want Work Request conflict", err)
	}
	if !reflect.DeepEqual(second, work.WorkRequestSubmitResult{}) {
		t.Fatalf("retry SubmitWorkRequest result = %#v, want zero result", second)
	}
	if second.Accepted {
		t.Fatalf("different-request-ID retry accepted = true, want rejection after visible acceptance")
	}
	if countWorkRequestRecords(history, secondRequest.RequestID) != 0 {
		t.Fatalf("WORK_REQUEST count for retry request %q = %d, want 0", secondRequest.RequestID, countWorkRequestRecords(history, secondRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, workID) != 1 {
		t.Fatalf("observed work materializations for %q = %d, want 1 durable materialization", workID, countObservedWorkMaterializations(snap, workID))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func startBlockedDispatchIngressHarness(t *testing.T) (*ingressBlockingExecutor, *recordingfixtures.ScriptedRuntimeLedger, *serviceModeRunHarness) {
	t.Helper()

	executor := &ingressBlockingExecutor{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	return executor, history, h
}

func releaseBlockedDispatchHarness(t *testing.T, executor *ingressBlockingExecutor, h *serviceModeRunHarness) {
	t.Helper()
	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func ingressBatchRequest(requestID, workID, traceID string) work.WorkRequest {
	return work.WorkRequest{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "ingress-batch",
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    traceID,
		}},
	}
}

func assertIngressObservable(
	t *testing.T,
	history *recordingfixtures.ScriptedRuntimeLedger,
	factoryInstance factoryhost.Engine,
	requestID string,
	workID string,
) {
	t.Helper()
	if !workRequestRecorded(history, requestID) {
		t.Fatalf("WORK_REQUEST not recorded for %q; work requests=%#v", requestID, history.WorkRequests)
	}
	snap, err := factoryInstance.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, workID) {
		t.Fatalf("work %q not observable before retry; marking=%#v", workID, snap.Marking.Tokens)
	}
}

func countWorkRequestRecords(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) int {
	count := 0
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			count++
		}
	}
	return count
}

func countMarkingTokensForWorkID(marking *petri.MarkingSnapshot, workID string) int {
	count := 0
	for _, token := range marking.Tokens {
		if token != nil && token.Color.WorkID == workID {
			count++
		}
	}
	return count
}

func countObservedWorkMaterializations(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) int {
	if snap == nil {
		return 0
	}
	count := countMarkingTokensForWorkID(&snap.Marking, workID)
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				count++
			}
		}
	}
	return count
}

type ingressBlockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *ingressBlockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func TestConcurrentBatchIngressProjectsWhileDispatchBlocked_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}

	opts := []testFactoryOption{
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	}

	h := startServiceModeRunHarness(t, opts...)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	batchRequest := work.WorkRequest{
		RequestID: "request-concurrent-ingress",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "concurrent-ingress",
			WorkID:     "work-concurrent-ingress",
			WorkTypeID: "task",
			TraceID:    "trace-concurrent-ingress",
		}},
	}
	if _, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest); err != nil {
		t.Fatalf("SubmitWorkRequest concurrent batch: %v", err)
	}

	if !workRequestRecorded(history, batchRequest.RequestID) {
		t.Fatalf("WORK_REQUEST not recorded before submit returned; work requests=%#v", history.WorkRequests)
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, "work-concurrent-ingress") {
		t.Fatalf(
			"submit returned before work became observable; work requests=%#v marking tokens=%#v",
			history.WorkRequests,
			snap.Marking.Tokens,
		)
	}

	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func workRequestRecorded(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) bool {
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			return true
		}
	}
	return false
}

func snapshotObservesWork(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) bool {
	if snap == nil {
		return false
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:init") {
		return true
	}
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				return true
			}
		}
	}
	return false
}

func TestInvocationScheduleFactoryRecoversDurableControllerWithoutInitialRetrigger(t *testing.T) {
	t.Parallel()
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
			"controller": {
				PlaceID: "loop-controller:active",
				Color: factorytoken.Color{
					Name: "dependency scan", RequestID: "request-loop", WorkID: "controller-1",
					WorkTypeID: "loop-controller", CurrentChainingTraceID: "trace-loop",
					Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "check dependencies"}},
					Tags: map[string]string{
						automations.InvocationScheduleEveryTag:                  "1h0m0s",
						automations.InvocationScheduleWorkstationTag:            "schedule-loop-request",
						automations.InvocationScheduleTriggerAtStartTag:         "true",
						automations.InvocationScheduleMaxConsecutiveFailuresTag: "3",
					},
				},
			},
			"execution-7": {
				PlaceID: "scheduled-execution:complete",
				Color: factorytoken.Color{
					WorkID: "controller-1/scheduled/000007", WorkTypeID: "scheduled-execution",
					CurrentChainingTraceID: "trace-loop",
					Tags:                   map[string]string{"agent_factory.time.sequence": "7"},
				},
			},
		}},
	}
	schedules := &recordingInvocationSchedules{}
	wrapped := &factoryImpl{cfg: &runtimeConfig{prepareSchedules: schedules.PrepareInvocationSchedules, scheduleConfig: invocationScheduleRecoveryLookup{}, runtimeConfig: invocationScheduleRecoveryLookup{}, requestFactoryDirectory: "factory-loop"}}

	failure := errors.New("selected schedule preparation failure")
	schedules.failure = failure
	if err := wrapped.recoverInvocationScheduleSnapshot(context.Background(), snapshot); !errors.Is(err, failure) {
		t.Fatalf("recovery failure = %v, want selected cause", err)
	}
	if wrapped.recovered || len(schedules.commits) != 0 {
		t.Fatal("failed recovery committed or prevented retry")
	}
	schedules.failure = nil
	schedules.requests = nil
	if err := wrapped.recoverInvocationScheduleSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("recover schedules: %v", err)
	}
	if len(schedules.requests) != 1 || len(schedules.commits) != 1 {
		t.Fatalf("recovery prepares/commits = %d/%d, want 1/1", len(schedules.requests), len(schedules.commits))
	}
	assertRecoveredInvocationSchedule(t, schedules)

	if err := wrapped.recoverInvocationScheduleSnapshot(context.Background(), snapshot); err != nil {
		t.Fatalf("second recovery: %v", err)
	}
	if len(schedules.requests) != 1 {
		t.Fatalf("idempotent recovery prepares = %d, want 1", len(schedules.requests))
	}
}

type recordingInvocationSchedules struct {
	requests []automations.InvocationScheduleRequest
	commits  []work.WorkRequestSubmitResult
	failure  error
}

func (s *recordingInvocationSchedules) PrepareInvocationSchedules(
	_ context.Context,
	request automations.InvocationScheduleRequest,
) (automations.PreparedInvocationSchedules, error) {
	s.requests = append(s.requests, request)
	if s.failure != nil {
		return automations.PreparedInvocationSchedules{}, s.failure
	}
	return automations.PreparedInvocationSchedules{
		CommitFunc: func(result work.WorkRequestSubmitResult) {
			s.commits = append(s.commits, result)
		},
	}, nil
}

func invocationScheduleRecoveryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{
		{Name: "loop-controller", States: []interfaces.StateConfig{
			{Name: "active", Type: interfaces.StateTypeInitial},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		}},
		{Name: "scheduled-execution", States: []interfaces.StateConfig{
			{Name: "complete", Type: interfaces.StateTypeTerminal},
		}},
	}}
}

type invocationScheduleRecoveryLookup struct{ interfaces.RuntimeConfigLookup }

func (invocationScheduleRecoveryLookup) FactoryConfig() *interfaces.FactoryConfig {
	return invocationScheduleRecoveryConfig()
}

func assertRecoveredInvocationSchedule(t *testing.T, schedules *recordingInvocationSchedules) {
	t.Helper()
	request := schedules.requests[0]
	if request.ResumeSequence != 7 || !request.SuppressTriggerAtStart {
		t.Fatalf("recovery controls = sequence %d suppress-start %v, want 7/true", request.ResumeSequence, request.SuppressTriggerAtStart)
	}
	if len(request.WorkRequest.Works) != 1 {
		t.Fatalf("recovered works = %d, want 1", len(request.WorkRequest.Works))
	}
	controller := request.WorkRequest.Works[0]
	if controller.WorkID != "controller-1" || controller.CurrentChainingTraceID != "trace-loop" {
		t.Fatalf("recovered controller identity = %#v", controller)
	}
	if got := controller.InvocationArguments.Arguments["every"].Values; len(got) != 1 || got[0] != "1h0m0s" {
		t.Fatalf("recovered duration = %#v, want 1h0m0s", got)
	}
	if got := controller.InvocationArguments.Arguments["triggerAtStart"].Values; len(got) != 1 || got[0] != "true" {
		t.Fatalf("recovered trigger-at-start = %#v, want retained true metadata", got)
	}
	commit := schedules.commits[0]
	if !commit.Accepted || commit.WorkID != "controller-1" || commit.TraceID != "trace-loop" {
		t.Fatalf("recovery commit = %#v", commit)
	}

}
