package internal

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestInvocationScheduleFactoryRecoversDurableControllerWithoutInitialRetrigger(t *testing.T) {
	underlying := &invocationScheduleRecoveryFactory{snapshot: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
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
	}}
	schedules := &recordingInvocationSchedules{}
	wrapped := &invocationScheduleFactory{
		Factory: underlying, schedules: schedules, factoryDir: "factory-loop",
		factoryConfig: invocationScheduleRecoveryConfig(), ctx: context.Background(),
	}

	if err := wrapped.recoverInvocationSchedules(context.Background()); err != nil {
		t.Fatalf("recover schedules: %v", err)
	}
	if len(schedules.requests) != 1 || len(schedules.commits) != 1 {
		t.Fatalf("recovery prepares/commits = %d/%d, want 1/1", len(schedules.requests), len(schedules.commits))
	}
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

	if err := wrapped.recoverInvocationSchedules(context.Background()); err != nil {
		t.Fatalf("second recovery: %v", err)
	}
	if len(schedules.requests) != 1 {
		t.Fatalf("idempotent recovery prepares = %d, want 1", len(schedules.requests))
	}
}

type invocationScheduleRecoveryFactory struct {
	factory.Factory
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

func (f *invocationScheduleRecoveryFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return f.snapshot, nil
}

type recordingInvocationSchedules struct {
	requests []automations.InvocationScheduleRequest
	commits  []work.WorkRequestSubmitResult
}

func (s *recordingInvocationSchedules) PrepareInvocationSchedules(
	_ context.Context,
	request automations.InvocationScheduleRequest,
) (automations.PreparedInvocationSchedules, error) {
	s.requests = append(s.requests, request)
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

var _ factory.Factory = (*invocationScheduleRecoveryFactory)(nil)
var _ invocationScheduleService = (*recordingInvocationSchedules)(nil)
