package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFactoryService_SimpleDashboardRenderInputUsesRenderData(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	provider := newDashboardWorldViewProvider()
	rendered := make(chan SimpleDashboardRenderInput, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                     dir,
		RuntimeMode:             interfaces.RuntimeModeService,
		Logger:                  zap.NewNop(),
		ProviderOverride:        provider,
		SimpleDashboardRenderer: func(input SimpleDashboardRenderInput) { rendered <- input },
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	defer stopServiceModeRun(t, cancelRun, errCh)

	submitDashboardWorldViewWork(t, svc, "dashboard-world-active", "trace-dashboard-active")
	provider.nextDispatch(t)
	active := renderSimpleDashboardForTest(t, svc, rendered)
	assertDashboardRenderDataActive(t, active.RenderData, "dashboard-world-active")
	assertSimpleDashboardActiveOutput(t, active)

	provider.respond(interfaces.InferenceResponse{
		Content: "COMPLETE",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-dashboard-success",
		},
	}, nil)
	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "dashboard-world-active", time.Second)
	completed := renderSimpleDashboardForTest(t, svc, rendered)
	if completed.EngineState.Topology == nil {
		t.Fatal("renderer input lost aggregate snapshot topology")
	}
	assertDashboardRenderDataCompleted(t, completed.RenderData, "sess-dashboard-success")

	submitDashboardWorldViewWork(t, svc, "dashboard-world-failed", "trace-dashboard-failed")
	provider.nextDispatch(t)
	provider.respond(interfaces.InferenceResponse{}, workers.NewProviderErrorWithSession(
		interfaces.ProviderErrorTypePermanentBadRequest,
		"provider rejected dashboard world-view work",
		errors.New("provider rejected"),
		&interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-dashboard-failed",
		},
	))
	waitForTokenInPlaceByWorkID(t, svc, "task:failed", "dashboard-world-failed", time.Second)
	failed := renderSimpleDashboardForTest(t, svc, rendered)
	assertDashboardRenderDataFailed(t, failed.RenderData, "dashboard-world-failed")
	assertSimpleDashboardTerminalOutput(t, failed)
	assertSimpleDashboardSessionRowsMatchRenderData(t, failed)
}

// pkgmaintcheck:ignore-cyclomatic-complexity this service-mode API seam test keeps callback, submit, subscribe, and snapshot assertions in one flow.
func TestFactoryService_Run_APIServerStarterReceivesWorkingAPISurface(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	type starterObservation struct {
		submitResult interfaces.WorkRequestSubmitResult
		submitErr    error
		stream       *interfaces.FactoryEventStream
		streamErr    error
		snapshot     *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
		snapshotErr  error
		current      factoryapi.Factory
		currentErr   error
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	observedCh := make(chan starterObservation, 1)
	svc, err := BuildFactoryService(runCtx, &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Port:              9999,
		Logger:            zap.NewNop(),
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, port int, l *zap.Logger) error {
			observation := starterObservation{}
			workRequest := requests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
				WorkID:     "starter-task",
				Name:       "starter-task",
				WorkTypeID: "task",
				TraceID:    "trace-api-surface-starter",
				Payload:    json.RawMessage(`{"title":"Starter task"}`),
			}})
			observation.submitResult, observation.submitErr = runtime.SubmitWorkRequest(ctx, workRequest)
			observation.stream, observation.streamErr = runtime.SubscribeFactoryEvents(ctx)
			observation.snapshot, observation.snapshotErr = runtime.GetEngineStateSnapshot(ctx)
			observation.current, observation.currentErr = runtime.GetCurrentFactory(ctx)
			observedCh <- observation
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	observation := <-observedCh
	if observation.submitErr != nil {
		t.Fatalf("APIServerStarter runtime.SubmitWorkRequest: %v", observation.submitErr)
	}
	if !observation.submitResult.Accepted {
		t.Fatal("APIServerStarter submit accepted = false, want true")
	}
	if observation.submitResult.TraceID != "trace-api-surface-starter" {
		t.Fatalf("APIServerStarter submit trace_id = %q, want trace-api-surface-starter", observation.submitResult.TraceID)
	}
	if observation.streamErr != nil {
		t.Fatalf("APIServerStarter runtime.SubscribeFactoryEvents: %v", observation.streamErr)
	}
	if observation.stream == nil || observation.stream.Events == nil {
		t.Fatalf("APIServerStarter stream = %#v, want live event stream", observation.stream)
	}
	if observation.snapshotErr != nil {
		t.Fatalf("APIServerStarter runtime.GetEngineStateSnapshot: %v", observation.snapshotErr)
	}
	if observation.snapshot == nil || observation.snapshot.Topology == nil {
		t.Fatalf("APIServerStarter snapshot = %#v, want topology-backed snapshot", observation.snapshot)
	}
	if _, ok := observation.snapshot.Topology.WorkTypes["task"]; !ok {
		t.Fatalf("APIServerStarter snapshot work types = %#v, want task", observation.snapshot.Topology.WorkTypes)
	}
	if observation.currentErr != nil {
		t.Fatalf("APIServerStarter runtime.GetCurrentFactory: %v", observation.currentErr)
	}
	if observation.current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("APIServerStarter current factory name = %q, want %q", observation.current.Name, apisurface.DefaultCurrentFactoryName)
	}

	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "starter-task", time.Second)
	cancelRun()
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestFactoryService_BuildSimpleDashboardRenderInputProjectsSelectedTickFromEvents(t *testing.T) {
	topology := &state.Net{ID: "aggregate-topology"}
	engineState := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		Topology:      topology,
		TickCount:     2,
		ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
			LaneID:      "codex/gpt-5-codex",
			Provider:    "codex",
			Model:       "gpt-5-codex",
			PausedAt:    time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
			PausedUntil: time.Date(2026, 4, 30, 10, 5, 0, 0, time.UTC),
		}},
	}
	dispatch := dashboardProjectionDispatchForTest()
	mock := &aggregateSnapshotFactory{
		engineState:   engineState,
		factoryEvents: dashboardProjectionEventsForTest(t, dispatch),
	}
	svc := &FactoryService{factory: mock, logger: zap.NewNop()}

	input, err := svc.buildSimpleDashboardRenderInput(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("buildSimpleDashboardRenderInput: %v", err)
	}

	if mock.engineStateSnapshotCalls != 1 {
		t.Fatalf("engine snapshot calls = %d, want 1", mock.engineStateSnapshotCalls)
	}
	if mock.factoryEventsCalls != 1 {
		t.Fatalf("factory event calls = %d, want 1", mock.factoryEventsCalls)
	}
	if input.EngineState.Topology != topology {
		t.Fatalf("engine-state topology = %#v, want aggregate topology %#v", input.EngineState.Topology, topology)
	}
	if input.RenderData.InFlightDispatchCount != 1 {
		t.Fatalf("in-flight count = %d, want selected tick active dispatch", input.RenderData.InFlightDispatchCount)
	}
	if input.RenderData.Session.CompletedCount != 0 {
		t.Fatalf("completed count = %d, want future completion excluded", input.RenderData.Session.CompletedCount)
	}
	if len(input.RenderData.Session.ProviderSessions) != 0 {
		t.Fatalf("provider sessions = %#v, want future provider session excluded", input.RenderData.Session.ProviderSessions)
	}
	if got := len(input.RenderData.Session.DispatchHistory); got != 0 {
		t.Fatalf("dispatch history length = %d, want selected tick to exclude future completion", got)
	}
	if len(input.RenderData.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(input.RenderData.ActiveThrottlePauses))
	}
	pause := input.RenderData.ActiveThrottlePauses[0]
	if pause.LaneID != "codex/gpt-5-codex" || pause.Provider != "codex" || pause.Model != "gpt-5-codex" {
		t.Fatalf("active throttle pause = %#v, want codex/gpt-5-codex lane", pause)
	}
	if len(pause.AffectedTransitionIDs) != 1 || pause.AffectedTransitionIDs[0] != dispatch.TransitionID {
		t.Fatalf("affected transition IDs = %#v, want [%s]", pause.AffectedTransitionIDs, dispatch.TransitionID)
	}
}

func TestFactoryService_RenderDashboardLogsEventProjectionErrors(t *testing.T) {
	tests := []struct {
		name          string
		factoryEvents []factoryapi.FactoryEvent
		factoryErr    error
	}{
		{
			name:       "event retrieval",
			factoryErr: errors.New("event history unavailable"),
		},
		{
			name: "event reconstruction",
			factoryEvents: []factoryapi.FactoryEvent{{
				Id:            "factory-event/work-request/malformed",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeWorkRequest,
				Context:       factoryapi.FactoryEventContext{Tick: 1, EventTime: time.Now()},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, observedLogs := observer.New(zap.ErrorLevel)
			renderCalls := 0
			svc := &FactoryService{
				factory: &aggregateSnapshotFactory{
					engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
						Topology:  &state.Net{ID: "aggregate-topology"},
						TickCount: 1,
					},
					factoryEvents:    tt.factoryEvents,
					factoryEventsErr: tt.factoryErr,
				},
				cfg: &FactoryServiceConfig{
					SimpleDashboardRenderer: func(SimpleDashboardRenderInput) { renderCalls++ },
				},
				logger: zap.New(core),
			}

			svc.renderDashboard(context.Background())

			if renderCalls != 0 {
				t.Fatalf("renderer calls = %d, want 0 after projection error", renderCalls)
			}
			if observedLogs.FilterMessage("simple dashboard render failed").Len() != 1 {
				t.Fatalf("render error log count = %d, want 1", observedLogs.FilterMessage("simple dashboard render failed").Len())
			}
		})
	}
}
