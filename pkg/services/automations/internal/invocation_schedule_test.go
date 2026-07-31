package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestInvocationSchedule_FakeClockTriggersDistinctWorkAndSkipsOverlap(t *testing.T) {
	start := time.Date(2026, time.July, 29, 18, 0, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	service := New(zap.NewNop(), clock, nil, "workflow-loop", "", nil, nil, nil)
	config, runtimeConfig := invocationScheduleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var observationMu sync.Mutex
	executionActive := false
	submitted := make(chan work.WorkRequest, 8)
	prepared, err := service.PrepareInvocationSchedules(ctx, automations.InvocationScheduleRequest{
		FactoryDir: "factory-loop", FactoryConfig: config, RuntimeConfig: runtimeConfig,
		WorkRequest: invocationControllerRequest("1m", "true", "0"),
		Submitter: func(_ context.Context, request work.WorkRequest) error {
			submitted <- request
			return nil
		},
		Observe: func(context.Context, automations.InvocationScheduleObservationRequest) (automations.InvocationScheduleObservation, error) {
			observationMu.Lock()
			defer observationMu.Unlock()
			return automations.InvocationScheduleObservation{ControllerActive: true, ExecutionActive: executionActive}, nil
		},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationSchedules() error = %v", err)
	}
	prepared.Commit(work.WorkRequestSubmitResult{Accepted: true, WorkID: "controller-1", TraceID: "trace-1"})

	first := receiveScheduledRequest(t, submitted)
	assertScheduledRequest(t, first, "controller-1/scheduled/000001", "init", "SCHEDULED", "1", start, start)

	waitForInvocationClockWaiters(t, clock, 1)
	observationMu.Lock()
	executionActive = true
	observationMu.Unlock()
	clock.Advance(time.Minute)
	second := receiveScheduledRequest(t, submitted)
	assertScheduledRequest(t, second, "controller-1/scheduled/000002", "skipped", "SKIPPED_OVERLAP", "2", start.Add(time.Minute), start.Add(time.Minute))

	waitForInvocationClockWaiters(t, clock, 1)
	observationMu.Lock()
	executionActive = false
	observationMu.Unlock()
	clock.Advance(time.Minute)
	third := receiveScheduledRequest(t, submitted)
	assertScheduledRequest(t, third, "controller-1/scheduled/000003", "init", "SCHEDULED", "3", start.Add(2*time.Minute), start.Add(2*time.Minute))

	cancel()
	prepared.Abort()
	clock.Advance(5 * time.Minute)
	select {
	case unexpected := <-submitted:
		t.Fatalf("submission after cancellation = %#v", unexpected)
	default:
	}
}

func TestInvocationSchedule_ResumeContinuesSequenceWithoutRepeatingInitialTrigger(t *testing.T) {
	start := time.Date(2026, time.July, 29, 18, 30, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	service := New(zap.NewNop(), clock, nil, "workflow-loop", "", nil, nil, nil)
	config, runtimeConfig := invocationScheduleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	submitted := make(chan work.WorkRequest, 2)

	prepared, err := service.PrepareInvocationSchedules(ctx, automations.InvocationScheduleRequest{
		FactoryConfig: config, RuntimeConfig: runtimeConfig,
		WorkRequest:    invocationControllerRequest("1m", "true", "0"),
		ResumeSequence: 7, SuppressTriggerAtStart: true,
		Submitter: func(_ context.Context, request work.WorkRequest) error { submitted <- request; return nil },
		Observe: func(context.Context, automations.InvocationScheduleObservationRequest) (automations.InvocationScheduleObservation, error) {
			return automations.InvocationScheduleObservation{ControllerActive: true}, nil
		},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationSchedules() error = %v", err)
	}
	prepared.Commit(work.WorkRequestSubmitResult{Accepted: true, WorkID: "controller-resumed", TraceID: "trace-1"})
	select {
	case unexpected := <-submitted:
		t.Fatalf("recovery repeated trigger-at-start = %#v", unexpected)
	default:
	}

	waitForInvocationClockWaiters(t, clock, 1)
	clock.Advance(time.Minute)
	resumed := receiveScheduledRequest(t, submitted)
	assertScheduledRequest(
		t, resumed, "controller-resumed/scheduled/000008", "init", "SCHEDULED", "8",
		start.Add(time.Minute), start.Add(time.Minute),
	)
}

func TestInvocationSchedule_RejectsInvalidDurationBeforeCommit(t *testing.T) {
	service := New(zap.NewNop(), clockwork.NewFakeClock(), nil, "workflow-loop", "", nil, nil, nil)
	config, runtimeConfig := invocationScheduleFixture(t)
	submissions := 0
	_, err := service.PrepareInvocationSchedules(context.Background(), automations.InvocationScheduleRequest{
		FactoryConfig: config, RuntimeConfig: runtimeConfig,
		WorkRequest: invocationControllerRequest("tomorrow", "false", "0"),
		Submitter:   func(context.Context, work.WorkRequest) error { submissions++; return nil },
	})
	if err == nil {
		t.Fatal("PrepareInvocationSchedules() error = nil, want invalid duration")
	}
	if submissions != 0 {
		t.Fatalf("submissions = %d, want zero before valid schedule commit", submissions)
	}
}

func TestInvocationSchedule_FailureCeilingDisablesLaterTriggers(t *testing.T) {
	start := time.Date(2026, time.July, 29, 19, 0, 0, 0, time.UTC)
	clock := clockwork.NewFakeClockAt(start)
	service := New(zap.NewNop(), clock, nil, "workflow-loop", "", nil, nil, nil)
	config, runtimeConfig := invocationScheduleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	submitted := make(chan work.WorkRequest, 2)
	failedController := make(chan string, 1)
	prepared, err := service.PrepareInvocationSchedules(ctx, automations.InvocationScheduleRequest{
		FactoryConfig: config, RuntimeConfig: runtimeConfig,
		WorkRequest: invocationControllerRequest("1m", "true", "2"),
		Submitter:   func(_ context.Context, request work.WorkRequest) error { submitted <- request; return nil },
		Observe: func(context.Context, automations.InvocationScheduleObservationRequest) (automations.InvocationScheduleObservation, error) {
			return automations.InvocationScheduleObservation{ControllerActive: true, ConsecutiveFailures: 2}, nil
		},
		FailController: func(_ context.Context, workID string) error { failedController <- workID; return nil },
	})
	if err != nil {
		t.Fatalf("PrepareInvocationSchedules() error = %v", err)
	}
	prepared.Commit(work.WorkRequestSubmitResult{Accepted: true, WorkID: "controller-2", TraceID: "trace-2"})
	select {
	case unexpected := <-submitted:
		t.Fatalf("submission at exhausted failure ceiling = %#v", unexpected)
	default:
	}
	select {
	case workID := <-failedController:
		if workID != "controller-2" {
			t.Fatalf("failed controller = %q, want controller-2", workID)
		}
	default:
		t.Fatal("failure ceiling did not transition controller to failed")
	}
	waitForInvocationClockWaiters(t, clock, 1)
	clock.Advance(3 * time.Minute)
	select {
	case unexpected := <-submitted:
		t.Fatalf("later submission at exhausted failure ceiling = %#v", unexpected)
	default:
	}
}

func invocationScheduleFixture(t *testing.T) (*interfaces.FactoryConfig, interfaces.RuntimeConfigLookup) {
	t.Helper()
	workstation := interfaces.FactoryWorkstationConfig{
		Name: "schedule-loop-request", Kind: interfaces.WorkstationKindCron,
		Cron:   &interfaces.CronConfig{Every: "${every}"},
		Inputs: []interfaces.IOConfig{{WorkTypeName: "loop-controller", StateName: "active"}},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "loop-controller", StateName: "active"},
			{WorkTypeName: "scheduled-execution", StateName: "init"},
		},
	}
	config := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "loop-controller", States: []interfaces.StateConfig{{Name: "active", Type: interfaces.StateTypeInitial}, {Name: "stopped", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed}}},
			{Name: "scheduled-execution", States: []interfaces.StateConfig{{Name: "init", Type: interfaces.StateTypeInitial}, {Name: "complete", Type: interfaces.StateTypeTerminal}, {Name: "skipped", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed}}},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{workstation},
	}
	runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
		Factory: config, FactoryPath: "factory-loop",
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{workstation.Name: &workstation},
	}
	return config, runtimeConfig
}

func waitForInvocationClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func invocationControllerRequest(every, triggerAtStart, maxFailures string) work.WorkRequest {
	return work.WorkRequest{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "loop controller", WorkTypeID: "loop-controller",
			Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "check dependencies"}},
			InvocationArguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
				"every":                  {Values: []string{every}},
				"triggerAtStart":         {Values: []string{triggerAtStart}},
				"maxConsecutiveFailures": {Values: []string{maxFailures}},
			}},
		}},
	}
}

func receiveScheduledRequest(t *testing.T, requests <-chan work.WorkRequest) work.WorkRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fake-clock scheduled request")
		return work.WorkRequest{}
	}
}

func assertScheduledRequest(
	t *testing.T,
	request work.WorkRequest,
	wantWorkID, wantState, wantOutcome, wantSequence string,
	wantNominal, wantActual time.Time,
) {
	t.Helper()
	if len(request.Works) != 1 {
		t.Fatalf("scheduled works = %d, want 1", len(request.Works))
	}
	got := request.Works[0]
	if got.WorkID != wantWorkID || got.WorkTypeID != "scheduled-execution" || got.State != wantState {
		t.Fatalf("scheduled Work = %#v, want id %q scheduled-execution:%s", got, wantWorkID, wantState)
	}
	if got.Tags[timeTriggerOutcomeTag] != wantOutcome || got.Tags[timeSequenceTag] != wantSequence || got.Tags[timeIntervalTag] != "1m0s" {
		t.Fatalf("scheduled tags = %#v", got.Tags)
	}
	if got.Tags[interfaces.TimeWorkTagKeyNominalAt] != wantNominal.Format(time.RFC3339Nano) || got.Tags[timeActualAtTag] != wantActual.Format(time.RFC3339Nano) {
		t.Fatalf("scheduled timing tags = %#v", got.Tags)
	}
	if got.CurrentChainingTraceID != "trace-1" && got.CurrentChainingTraceID != "trace-2" {
		t.Fatalf("scheduled chaining trace = %q", got.CurrentChainingTraceID)
	}
	if got.InvocationArguments == nil || got.InvocationArguments.Arguments["every"].Values[0] != "1m" {
		t.Fatalf("scheduled invocation arguments = %#v", got.InvocationArguments)
	}
}
