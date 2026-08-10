package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type javaScriptRuntimeServiceConfig struct {
	ProjectRoot        string
	ChildExecutorMode  string
	InvocationExecutor workerexecution.InvocationExecutor
	Persistence        runtimepersist.Store
	Clock              factory.Clock
	Workflows          factory.JavaScriptWorkflows
}

func testRuntimePersistenceStoreFactory(projectRoot string) (runtimepersist.Store, error) {
	return runtimepersist.NewProjectStore(projectRoot, platformfilesystem.Local{})
}

func mustTestRuntimePersistenceStore(t *testing.T, dir string) runtimepersist.Store {
	t.Helper()
	store, err := runtimepersist.NewDirectoryStore(dir, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	return store
}

func newConfiguredJavaScriptRuntimeService(config javaScriptRuntimeServiceConfig) *JavaScriptRuntimeService {
	workflows := config.Workflows
	if workflows == nil {
		workflows = factoryruntimefixtures.ScriptedJavaScriptWorkflows{}
	}
	clock := config.Clock
	if clock == nil {
		clock = durableFixedClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	}
	return NewJavaScriptRuntimeService(
		config.ProjectRoot, config.ChildExecutorMode, config.InvocationExecutor,
		config.Persistence, clock, testSyncWaitScheduler{}, checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		workflows, orchestrationJavaScriptFromWorkflows(workflows), workflows,
		nil, factory.JavaScriptWorkerSettings{}, mustTestRecordingWriter(),
		testSessionIDGenerator,
		nil, nil,
	)
}

func mustTestRecordingWriter() recordings.PortableRecordingWriter {
	return portableRecordingTestWriter{}
}

type portableRecordingTestWriter struct{}

func (portableRecordingTestWriter) Write(path string, value recordings.PortableRecording) error {
	if err := recordings.ValidatePortableRecording(value); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func seedRuntimeSessionWithRunningDispatch(
	service *JavaScriptRuntimeService,
	sessionID, dispatchID, label string,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return NewValidationError("dispatchId", "dispatchId is required")
	}

	now := service.now()
	session := SessionReadResult{
		SessionID:        id,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Phase:            "execute",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
		Links:            InspectionLinksForSession(id, true),
		Progress: &ProgressCounts{
			TotalDispatches:    1,
			InFlightDispatches: 1,
		},
	}
	result := ResultReadResult{
		SessionID:     id,
		SessionStatus: LifecycleStatusRunning,
		ResultStatus:  ResultStatusNotReady,
		Availability: &ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
	dispatches := []DispatchSummary{{
		ID: dispatchID, Status: DispatchStatusRunning, Phase: "execute", Label: label,
	}}
	state := &runtimeSessionState{
		session:    session,
		result:     result,
		dispatches: dispatches,
		dispatchStatusTransitions: map[string][]DispatchStatus{
			dispatchID: {DispatchStatusQueued, DispatchStatusRunning},
		},
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessions[id] = state
	return nil
}

func applyRuntimeTerminalOutcome(
	service *JavaScriptRuntimeService,
	sessionID string,
	outcome factory.JavaScriptRuntimeOutcome,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	state, ok := service.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	finishedAt := service.now()
	terminal := runtimeSessionState{
		session: cloneSessionRead(state.session),
		result:  cloneResultRead(state.result),
	}
	if outcome.OK {
		applyRuntimeSuccessProjection(&terminal, id, outcome, finishedAt)
	} else if len(outcome.Records) > 0 {
		applyRuntimeExecutionRecordProjection(&terminal, id, outcome.Records, finishedAt)
		projectRuntimeFailure(&terminal.session, &terminal.result, outcome)
	}
	applyTerminalRuntimeProjection(state, terminal, outcome)
	return nil
}

type stubCancelAwareService struct{}

func (stubCancelAwareService) GetSession(ctx context.Context, _ string) (SessionReadResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionReadResult{}, err
	}
	return SessionReadResult{}, nil
}

func (stubCancelAwareService) StartAsync(context.Context, StartRequest) (AsyncStartResult, error) {
	return AsyncStartResult{}, nil
}

func (stubCancelAwareService) StartSync(context.Context, StartRequest) (SyncStartResult, error) {
	return SyncStartResult{}, nil
}

func (stubCancelAwareService) Pause(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) Resume(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) Cancel(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) Terminate(context.Context, string, ControlRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) Approve(context.Context, string, ApproveRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) RetryDispatch(context.Context, string, RetryDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) InterruptDispatch(context.Context, string, InterruptDispatchRequest) (LifecycleControlResult, error) {
	return LifecycleControlResult{}, nil
}

func (stubCancelAwareService) GetResult(context.Context, string, ResultRequest) (ResultReadResult, error) {
	return ResultReadResult{}, nil
}

func (stubCancelAwareService) ListDispatches(context.Context, string) (ListDispatchesResult, error) {
	return ListDispatchesResult{}, nil
}

func (stubCancelAwareService) GetDispatch(context.Context, string, string) (DispatchDetail, error) {
	return DispatchDetail{}, nil
}

func (stubCancelAwareService) ListArtifacts(context.Context, string) (ListArtifactsResult, error) {
	return ListArtifactsResult{}, nil
}

func (stubCancelAwareService) GetArtifact(context.Context, string, string) (ArtifactDetail, error) {
	return ArtifactDetail{}, nil
}

func (stubCancelAwareService) ReadEvents(context.Context, string, EventReconnectRequest) (EventReadResult, error) {
	return EventReadResult{}, nil
}

func (stubCancelAwareService) ListSessions(context.Context, ListSessionsRequest) (ListSessionsResult, error) {
	return ListSessionsResult{}, nil
}

const simpleFinalWorkflowSource = `return {
  label: meta.name,
  description: meta.description,
  subject: args.subject,
  repeat: args.count,
  echo: args.prefix + ":" + args.subject,
};
`

const busyLoopWorkflowSource = `while (true) {}`

const throwErrorWorkflowSource = `throw new Error("workflow execution failed: " + args.subject);`

const progressThenFinalWorkflowSource = `
phase("execute");

const artifactRef = workflow.artifact({
  kind: "log",
  label: "unpersisted-output",
  content: { message: "must roll back" },
});
workflow.checkpoint({
  label: "before-final",
  state: { artifactRef: artifactRef },
});
return { artifactRef: artifactRef };
`

type durableFixedClock struct{ now time.Time }

func (c durableFixedClock) Now() time.Time { return c.now }

func newDefaultJavaScriptRuntimeService(t *testing.T, workflows ...factory.JavaScriptWorkflows) *JavaScriptRuntimeService {
	t.Helper()

	config := javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Clock:       durableFixedClock{now: time.Now()},
	}
	if len(workflows) > 0 {
		config.Workflows = workflows[0]
	}
	return newConfiguredJavaScriptRuntimeService(config)
}

func scriptedRuntimeWorkflows(
	run func(context.Context, factory.JavaScriptRuntimeRequest, factory.JavaScriptRuntimeHooks) (factory.JavaScriptRuntimeOutcome, error),
) factory.JavaScriptWorkflows {
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{RunFunc: run}
}

func scriptedSuccessfulRuntimeWorkflows(value map[string]any) factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		context.Context,
		factory.JavaScriptRuntimeRequest,
		factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		encoded, err := json.Marshal(value)
		if err != nil {
			return factory.JavaScriptRuntimeOutcome{}, err
		}
		return factory.JavaScriptRuntimeOutcome{
			OK:    true,
			Value: factory.TypedValue{JSON: encoded},
		}, nil
	})
}

func scriptedBlockingRuntimeWorkflows() factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		ctx context.Context,
		_ factory.JavaScriptRuntimeRequest,
		_ factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		<-ctx.Done()
		code := factory.JavaScriptRuntimeCodeCanceled
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			code = factory.JavaScriptRuntimeCodeTimeout
		}
		return factory.JavaScriptRuntimeOutcome{
			Failure: factory.JavaScriptRuntimeFailure{Code: code, Message: ctx.Err().Error()},
		}, nil
	})
}

func scriptedFailedRuntimeWorkflows() factory.JavaScriptWorkflows {
	return scriptedRuntimeWorkflows(func(
		context.Context,
		factory.JavaScriptRuntimeRequest,
		factory.JavaScriptRuntimeHooks,
	) (factory.JavaScriptRuntimeOutcome, error) {
		return factory.JavaScriptRuntimeOutcome{
			Failure: factory.JavaScriptRuntimeFailure{
				Code:    factory.JavaScriptRuntimeCodeScriptError,
				Message: "scripted workflow execution failure",
			},
		}, nil
	})
}

func inlineWorkflowStartRequest(
	requestID string,
	source string,
	args map[string]any,
	requestedPolicy map[string]any,
) StartRequest {
	return StartRequest{
		RequestID: requestID,
		Source: Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &InlineWorkflowSource{
				InlineSource: source,
				Dialect:      "you-workflow-v1",
				Metadata: map[string]string{
					"name":        "runtime-async-fixture",
					"description": "returns a structured final value",
				},
			},
		},
		Args:            args,
		RequestedPolicy: requestedPolicy,
	}
}

func waitUntilSessionStatus(
	t *testing.T,
	service Service,
	sessionID string,
	want LifecycleStatus,
	timeout time.Duration,
) SessionReadResult {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if session.Status == want {
			return session
		}
		if IsTerminalLifecycleStatus(session.Status) && session.Status != want {
			t.Fatalf("session %s reached terminal %q before %q", sessionID, session.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %q within %s", sessionID, want, timeout)
	return SessionReadResult{}
}

func decodePrimaryResultMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()

	var content []struct {
		Type string          `json:"type"`
		JSON json.RawMessage `json:"json,omitempty"`
	}
	if err := json.Unmarshal(raw, &content); err != nil {
		t.Fatalf("unmarshal primary result content: %v", err)
	}
	for _, part := range content {
		if part.Type == "JSON" && len(part.JSON) > 0 {
			var projected map[string]any
			if err := json.Unmarshal(part.JSON, &projected); err != nil {
				t.Fatalf("unmarshal primary result json part: %v", err)
			}
			return projected
		}
	}
	t.Fatalf("primary result content = %#v, want JSON part", content)
	return nil
}

func writeSimpleFinalWorkflowProject(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	workflowPath := filepath.Join(workflowDir, "simple-final.workflow.js")
	if err := os.WriteFile(workflowPath, []byte(simpleFinalWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

type orchestrationJavaScriptAdapter struct {
	factory.JavaScriptWorkflowRuntime
}

func orchestrationJavaScriptFromWorkflows(workflows factory.JavaScriptWorkflows) factory.OrchestrationJavaScriptExecution {
	if workflows == nil {
		return nil
	}
	return orchestrationJavaScriptAdapter{workflows}
}

func (a orchestrationJavaScriptAdapter) RunJavaScript(
	ctx context.Context,
	req factory.JavaScriptRuntimeRequest,
	hooks factory.JavaScriptRuntimeHooks,
) (factory.JavaScriptRuntimeOutcome, error) {
	return a.Run(ctx, req, hooks)
}

func (a orchestrationJavaScriptAdapter) ResumeJavaScript(
	summary factory.JavaScriptCompletedCheckpointSummary,
	records []factory.JavaScriptRuntimeRecord,
) factory.JavaScriptResumeContext {
	return a.ResumeContext(summary, records)
}
