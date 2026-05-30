package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/dashboard"
	"github.com/portpowered/infinite-you/pkg/cli/dashboardrender"
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
	svc := &FactoryService{logger: zap.NewNop()}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{factory: mock})

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
				cfg: &FactoryServiceConfig{
					SimpleDashboardRenderer: func(SimpleDashboardRenderInput) { renderCalls++ },
				},
				logger: zap.New(core),
			}
			bindServiceStartupRuntime(svc, &factoryRuntimeBundle{
				factory: &aggregateSnapshotFactory{
					engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
						Topology:  &state.Net{ID: "aggregate-topology"},
						TickCount: 1,
					},
					factoryEvents:    tt.factoryEvents,
					factoryEventsErr: tt.factoryErr,
				},
			})

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

func TestBuildFactoryService_ConfigWithAllOptions(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	// Create a valid work file.
	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title":"test"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	dashRendered := false
	apiStarted := false

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Port:              9999,
		Logger:            zap.NewNop(),
		WorkFile:          workFile,
		SimpleDashboardRenderer: func(_ SimpleDashboardRenderInput) {
			dashRendered = true
		},
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, port int, l *zap.Logger) error {
			apiStarted = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Verify config was preserved.
	if svc.cfg.MockWorkersConfig == nil {
		t.Error("expected MockWorkersConfig to be set")
	}
	if svc.cfg.Port != 9999 {
		t.Errorf("expected Port 9999, got %d", svc.cfg.Port)
	}
	if svc.cfg.WorkFile != workFile {
		t.Errorf("expected WorkFile %q, got %q", workFile, svc.cfg.WorkFile)
	}
	if svc.cfg.SimpleDashboardRenderer == nil {
		t.Error("expected SimpleDashboardRenderer to be set")
	}
	if svc.cfg.APIServerStarter == nil {
		t.Error("expected APIServerStarter to be set")
	}

	// Verify callbacks are callable (but don't test Run here — that needs a full engine).
	_ = dashRendered
	_ = apiStarted
}

func dashboardProjectionEventsForTest(t *testing.T, dispatch interfaces.WorkDispatch) []factoryapi.FactoryEvent {
	t.Helper()
	result := interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "COMPLETE",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-future-completion",
		},
	}
	return []factoryapi.FactoryEvent{
		dashboardInitialStructureEventForTest(t),
		serviceReplayWorkRequestEvent(t, dispatch.Execution.RequestID, 1, "dashboard-test", serviceReplayWorksFromDispatch(dispatch), nil),
		serviceReplayDispatchCreatedEvent(t, dispatch, 2),
		serviceReplayDispatchCompletedEvent(t, "future-completion", result, 3),
	}
}

func dashboardInitialStructureEventForTest(t *testing.T) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				},
			}},
			Workers: &[]factoryapi.Worker{{
				Name:          "worker-a",
				ModelProvider: serviceEnumPtr(factoryapi.WorkerModelProviderCodex),
				Model:         serviceStringPtr("gpt-5-codex"),
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:      serviceStringPtr("process"),
				Name:    "process",
				Worker:  "worker-a",
				Inputs:  []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
				Outputs: &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &[]factoryapi.WorkstationIO{{
					WorkType: "task",
					State:    "failed",
				}},
			}},
		},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromInitialStructureRequestEventPayload(payload); err != nil {
		t.Fatalf("encode initial structure event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/initial-structure/dashboard-test",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInitialStructureRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC),
			Tick:      0,
		},
		Payload: union,
	}
}

func dashboardProjectionDispatchForTest() interfaces.WorkDispatch {
	token := interfaces.Token{
		ID:      "work-selected",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			Name:       "Selected Tick Work",
			RequestID:  "request-selected",
			WorkID:     "work-selected",
			WorkTypeID: "task",
			DataType:   interfaces.DataTypeWork,
			TraceID:    "trace-selected",
		},
	}
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-selected",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		InputTokens:     workers.InputTokens(token),
		Execution: interfaces.ExecutionMetadata{
			DispatchCreatedTick: 2,
			RequestID:           "request-selected",
			TraceID:             "trace-selected",
			WorkIDs:             []string{"work-selected"},
		},
	}
}

func assertSimpleDashboardActiveOutput(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	assertSimpleDashboardOutputContains(t, input, []string{
		"Active Workstations (1)",
		"process",
		"dashboard-world-active",
		"Workstation Activity",
		"Session Metrics",
		"Workstations Dispatched:  1",
		"Workstations Completed:   0",
		"Workstations Failed:      0",
	})
}

func assertSimpleDashboardTerminalOutput(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	assertSimpleDashboardOutputContains(t, input, []string{
		"Completed Workstations",
		"Success",
		"Failed",
		"dashboard-world-active",
		"dashboard-world-failed",
		"provider rejected dashboard world-view work",
		"Queue Counts",
		"task:complete",
		"task:failed",
		"Session Metrics",
		"Workstations Dispatched:  2",
		"Workstations Completed:   1",
		"Workstations Failed:      1",
		"Failed work: 1",
		"Provider sessions:",
		"codex / session_id / sess-dashboard-success",
		"codex / session_id / sess-dashboard-failed",
	})
}

func assertSimpleDashboardSessionRowsMatchRenderData(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	session := input.RenderData.Session
	if session.DispatchedCount != len(session.DispatchHistory) {
		t.Fatalf("dispatched count = %d, dispatch history rows = %d", session.DispatchedCount, len(session.DispatchHistory))
	}
	if terminalRows := session.CompletedCount + session.FailedCount; terminalRows != len(session.DispatchHistory) {
		t.Fatalf("terminal count = %d, dispatch history rows = %d", terminalRows, len(session.DispatchHistory))
	}
	output := dashboard.FormatSimpleDashboardWithRenderData(
		input.EngineState,
		input.RenderData,
		time.Now(),
	)
	if renderedProviderRows := strings.Count(output, "codex / session_id /"); renderedProviderRows != len(session.ProviderSessions) {
		t.Fatalf("rendered provider rows = %d, render-data provider sessions = %d\n%s",
			renderedProviderRows,
			len(session.ProviderSessions),
			output,
		)
	}
}

// writeWorkstationAgentsMDWithPrompt writes a MODEL_WORKSTATION AGENTS.md with a
// custom prompt template body into the given workstation directory.
func writeWorkstationAgentsMDWithPrompt(t *testing.T, factoryDir, workstationName, promptBody string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\n" + promptBody + "\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

// writeWorkstationAgentsMDWithPromptFile writes a MODEL_WORKSTATION AGENTS.md that
// references a prompt_file, and writes the prompt file alongside it.
func writeWorkstationAgentsMDWithPromptFile(t *testing.T, factoryDir, workstationName, promptFileName, promptContent string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\npromptFile: " + promptFileName + "\n---\nThis body should be ignored.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, promptFileName), []byte(promptContent), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
}

func writeRuntimeLookupWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\nworker: script-worker\nworkingDirectory: workspace\n---\nRun the script.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

type stubCommandRunner struct{}

func (stubCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

type capturingCommandRunner struct {
	request workers.CommandRequest
}

func (r *capturingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.request = workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

func mustLoadWorkerConfig(t *testing.T, dir string) *interfaces.WorkerConfig {
	t.Helper()
	def, err := config.LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkerConfig(%s): %v", dir, err)
	}
	return def
}

func mustLoadWorkstationConfig(t *testing.T, dir string) *interfaces.FactoryWorkstationConfig {
	t.Helper()
	def, err := config.LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkstationConfig(%s): %v", dir, err)
	}
	return def
}

type blockingInferenceProvider struct {
	releaseCh <-chan struct{}
	content   string
}

func (p *blockingInferenceProvider) Infer(context.Context, interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	<-p.releaseCh
	return interfaces.InferenceResponse{Content: p.content}, nil
}

type dashboardWorldViewProvider struct {
	requests  chan interfaces.ProviderInferenceRequest
	responses chan dashboardWorldViewProviderResponse
}

type dashboardWorldViewProviderResponse struct {
	response interfaces.InferenceResponse
	err      error
}

func newDashboardWorldViewProvider() *dashboardWorldViewProvider {
	return &dashboardWorldViewProvider{
		requests:  make(chan interfaces.ProviderInferenceRequest, 2),
		responses: make(chan dashboardWorldViewProviderResponse, 2),
	}
}

func (p *dashboardWorldViewProvider) Infer(ctx context.Context, request interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
	select {
	case response := <-p.responses:
		return response.response, response.err
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
}

func (p *dashboardWorldViewProvider) nextDispatch(t *testing.T) interfaces.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider dispatch")
		return interfaces.ProviderInferenceRequest{}
	}
}

func (p *dashboardWorldViewProvider) respond(response interfaces.InferenceResponse, err error) {
	p.responses <- dashboardWorldViewProviderResponse{response: response, err: err}
}

func submitDashboardWorldViewWork(t *testing.T, svc *FactoryService, workID, traceID string) {
	t.Helper()
	err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    json.RawMessage(`{"title":"dashboard world view"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
}

func renderSimpleDashboardForTest(
	t *testing.T,
	svc *FactoryService,
	rendered <-chan SimpleDashboardRenderInput,
) SimpleDashboardRenderInput {
	t.Helper()
	svc.renderDashboard(context.Background())
	select {
	case input := <-rendered:
		return input
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dashboard renderer input")
		return SimpleDashboardRenderInput{}
	}
}

func waitForTokenInPlaceByWorkID(t *testing.T, svc *FactoryService, placeID, workID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot output token: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.WorkID == workID || token.Color.ParentID == workID {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work %q in %s", workID, placeID)
}

func assertDashboardRenderDataActive(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, workID string) {
	t.Helper()
	if renderData.InFlightDispatchCount != 1 || len(renderData.ActiveExecutionsByDispatchID) != 1 {
		t.Fatalf("active executions = %#v, in-flight=%d, want one active dispatch",
			renderData.ActiveExecutionsByDispatchID,
			renderData.InFlightDispatchCount,
		)
	}
	if renderData.Session.DispatchedCount != 1 {
		t.Fatalf("dispatched count = %d, want 1", renderData.Session.DispatchedCount)
	}
	for _, execution := range renderData.ActiveExecutionsByDispatchID {
		for _, item := range execution.WorkItems {
			if item.WorkID == workID {
				if got := len(renderData.Session.DispatchHistory); got != 0 {
					t.Fatalf("dispatch history length = %d, want no completed dispatches during request-only tick", got)
				}
				return
			}
		}
	}
	t.Fatalf("active execution did not include work %q: %#v", workID, renderData.ActiveExecutionsByDispatchID)
}

func assertDashboardRenderDataCompleted(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, providerSessionID string) {
	t.Helper()
	session := renderData.Session
	if session.CompletedCount != 1 || session.DispatchedCount != 1 {
		t.Fatalf("session counts after completion = %#v, want dispatched=1 completed=1", session)
	}
	if len(session.ProviderSessions) != 1 || session.ProviderSessions[0].ProviderSession.ID != providerSessionID {
		t.Fatalf("provider sessions = %#v, want %q", session.ProviderSessions, providerSessionID)
	}
	assertRenderDataPlaceOccupancyContainsWork(t, renderData, "task:complete", "dashboard-world-active")
	if len(session.DispatchHistory) != 1 || session.DispatchHistory[0].Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("dispatch history = %#v, want one accepted completion", session.DispatchHistory)
	}
	if !dispatchHistoryContainsWork(t, session.DispatchHistory, "dashboard-world-active") {
		t.Fatalf("dispatch history = %#v, want completed work dashboard-world-active", session.DispatchHistory)
	}
}

func assertDashboardRenderDataFailed(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, workID string) {
	t.Helper()
	session := renderData.Session
	if session.DispatchedCount != 2 || session.CompletedCount != 1 || session.FailedCount != 1 {
		t.Fatalf("session counts after failure = %#v, want dispatched=2 completed=1 failed=1", session)
	}
	assertRenderDataPlaceOccupancyContainsWork(t, renderData, "task:failed", workID)
	if len(session.DispatchHistory) != 2 {
		t.Fatalf("dispatch history = %#v, want both successful and failed completions", session.DispatchHistory)
	}
	if !dispatchHistoryContainsWork(t, session.DispatchHistory, workID) {
		t.Fatalf("dispatch history = %#v, want failed work %q", session.DispatchHistory, workID)
	}
	if !providerSessionsContainID(session.ProviderSessions, "sess-dashboard-failed") {
		t.Fatalf("provider sessions = %#v, want retained failed provider session", session.ProviderSessions)
	}
}

func assertRenderDataPlaceOccupancyContainsWork(
	t *testing.T,
	renderData dashboardrender.SimpleDashboardRenderData,
	placeID, workID string,
) {
	t.Helper()

	for _, item := range renderData.PlaceOccupancyWorkItemsByPlaceID[placeID] {
		if item.WorkID == workID {
			return
		}
	}
	t.Fatalf("place occupancy[%s] = %#v, want work %q", placeID, renderData.PlaceOccupancyWorkItemsByPlaceID[placeID], workID)
}

func dispatchHistoryContainsWork(t *testing.T, history []interfaces.FactoryWorldDispatchCompletion, workID string) bool {
	t.Helper()

	for _, dispatch := range history {
		for _, item := range dispatch.InputWorkItems {
			if item.ID == workID {
				return true
			}
		}
		for _, item := range dispatch.OutputWorkItems {
			if item.ID == workID {
				return true
			}
		}
		if dispatch.TerminalWork != nil && dispatch.TerminalWork.WorkItem.ID == workID {
			return true
		}
		for _, itemID := range dispatch.WorkItemIDs {
			if itemID == workID {
				return true
			}
		}
	}
	return false
}

func providerSessionsContainID(sessions []interfaces.FactoryWorldProviderSessionRecord, sessionID string) bool {
	for _, session := range sessions {
		if session.ProviderSession.ID == sessionID {
			return true
		}
	}
	return false
}

func assertSimpleDashboardOutputContains(t *testing.T, input SimpleDashboardRenderInput, wants []string) {
	t.Helper()
	output := dashboard.FormatSimpleDashboardWithRenderData(
		input.EngineState,
		input.RenderData,
		time.Now(),
	)
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("simple dashboard output missing %q:\n%s", want, output)
		}
	}
}
