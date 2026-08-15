package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	responsestreamwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/testing/eventsstub"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestValidateCheckpointSummaryForResume_RejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-checkpoint-validation-001"
	valid := &factory.JavaScriptCheckpointSummary{
		Kind:           factory.JavaScriptCheckpointSummaryKind,
		SchemaVersion:  factory.JavaScriptCheckpointSummarySchemaVersion,
		CheckpointID:   "checkpoint-1",
		SessionID:      sessionID,
		ResumeStrategy: factory.JavaScriptResumeStrategy,
	}
	if err := validateCheckpointSummaryForResume(valid, sessionID); err != nil {
		t.Fatalf("validateCheckpointSummaryForResume(valid): %v", err)
	}

	cases := []struct {
		name    string
		summary *factory.JavaScriptCheckpointSummary
		field   string
	}{
		{
			name:    "missing checkpoint",
			summary: nil,
			field:   "checkpointSummary",
		},
		{
			name: "invalid kind",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind:         "invalid-kind",
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.kind",
		},
		{
			name: "unsupported schema version",
			summary: &factory.JavaScriptCheckpointSummary{
				SchemaVersion: 99,
				CheckpointID:  "checkpoint-1",
			},
			field: "checkpointSummary.schemaVersion",
		},
		{
			name: "missing checkpoint id",
			summary: &factory.JavaScriptCheckpointSummary{
				Kind: factory.JavaScriptCheckpointSummaryKind,
			},
			field: "checkpointSummary.checkpointId",
		},
		{
			name: "session id mismatch",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID:   "checkpoint-1",
				SessionID:      "dur-sess-other",
				ResumeStrategy: factory.JavaScriptResumeStrategy,
			},
			field: "checkpointSummary.sessionId",
		},
		{
			name: "checkpoint not approved for resume",
			summary: &factory.JavaScriptCheckpointSummary{
				CheckpointID: "checkpoint-1",
			},
			field: "checkpointSummary.resumeStrategy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCheckpointSummaryForResume(tc.summary, sessionID)
			resumeErr, ok := err.(*ResumeError)
			if !ok {
				t.Fatalf("error = %T (%v), want *ResumeError", err, err)
			}
			if resumeErr.Outcome != ResumeOutcomeInvalidState && resumeErr.Outcome != ResumeOutcomeMissingCheckpoint {
				t.Fatalf("outcome = %q, want typed resume failure", resumeErr.Outcome)
			}
			if tc.field != "" && resumeErr.Field != tc.field {
				t.Fatalf("field = %q, want %q", resumeErr.Field, tc.field)
			}
		})
	}
}

func TestApplyRuntimeCheckpointPartialProjection_SurfacesPartialResultWhileRunning(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-checkpoint-partial-001"
	state := &runtimeSessionState{
		session: SessionReadResult{
			SessionID: sessionID,
			Status:    LifecycleStatusRunning,
		},
		artifacts: []ArtifactSummary{{ID: "artifact-checkpoint-1", Kind: "text"}},
	}
	checkpoint := &factory.JavaScriptCheckpointRecord{
		ID:    "checkpoint-1",
		Label: "after-step-one",
		State: map[string]any{"text": "checkpoint partial output"},
	}

	applyRuntimeCheckpointPartialProjection(state, checkpoint)

	if state.session.ResultSummary == nil || state.session.ResultSummary.ResultStatus != string(ResultStatusPartial) {
		t.Fatalf("result summary = %#v, want PARTIAL", state.session.ResultSummary)
	}
	if state.result.ResultStatus != ResultStatusPartial {
		t.Fatalf("result status = %q, want PARTIAL", state.result.ResultStatus)
	}
	if state.result.Mode != ResultModePartial {
		t.Fatalf("result mode = %q, want partial", state.result.Mode)
	}
	if state.result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", state.result.SessionStatus)
	}
	if len(state.result.PrimaryResult) == 0 {
		t.Fatal("primary result missing")
	}
	if len(state.result.ArtifactIDs) != 1 || state.result.ArtifactIDs[0] != "artifact-checkpoint-1" {
		t.Fatalf("artifact IDs = %#v, want checkpoint artifact", state.result.ArtifactIDs)
	}
}

func TestApplyRuntimeCheckpointPartialProjection_NoopsForTerminalOrEmptyCheckpoint(t *testing.T) {
	t.Parallel()
	checkpoint := &factory.JavaScriptCheckpointRecord{
		ID:    "checkpoint-1",
		Label: "after-step-one",
		State: map[string]any{"text": "checkpoint partial output"},
	}
	running := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-checkpoint-partial-002", Status: LifecycleStatusRunning},
		result:  ResultReadResult{SessionID: "dur-sess-checkpoint-partial-002", ResultStatus: ResultStatusNotReady},
	}
	terminal := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-checkpoint-partial-003", Status: LifecycleStatusSucceeded},
		result:  ResultReadResult{SessionID: "dur-sess-checkpoint-partial-003", ResultStatus: ResultStatusFinal},
	}

	applyRuntimeCheckpointPartialProjection(nil, checkpoint)
	applyRuntimeCheckpointPartialProjection(running, nil)
	applyRuntimeCheckpointPartialProjection(terminal, checkpoint)
	applyRuntimeCheckpointPartialProjection(running, &factory.JavaScriptCheckpointRecord{ID: "checkpoint-empty"})

	if running.result.ResultStatus != ResultStatusNotReady {
		t.Fatalf("running result status = %q, want NOT_READY", running.result.ResultStatus)
	}
	if terminal.result.ResultStatus != ResultStatusFinal {
		t.Fatalf("terminal result status = %q, want FINAL", terminal.result.ResultStatus)
	}
}

func TestJavaScriptRuntimeService_ApplyRunningRuntimeRecord_CheckpointProjectsPartialResult(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-checkpoint-running-record-001"
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	if err := seedRuntimeSessionWithRunningDispatch(service, sessionID, "dispatch-1", "step-one"); err != nil {
		t.Fatalf("seedRuntimeSessionWithRunningDispatch: %v", err)
	}

	service.applyRunningRuntimeRecord(sessionID, factory.JavaScriptRuntimeRecord{
		Sequence: 2,
		Kind:     factory.JavaScriptRecordKindCheckpoint,
		Checkpoint: &factory.JavaScriptCheckpointRecord{
			ID:    "checkpoint-1",
			Label: "after-step-one",
			State: map[string]any{"text": "checkpoint partial output"},
		},
	})

	result, err := service.GetResult(context.Background(), sessionID, ResultRequest{Mode: ResultModePartial})
	if err != nil {
		t.Fatalf("GetResult partial: %v", err)
	}
	if result.ResultStatus != ResultStatusPartial {
		t.Fatalf("partial status = %q, want PARTIAL", result.ResultStatus)
	}
	if len(result.PrimaryResult) == 0 {
		t.Fatal("partial primaryResult missing")
	}
	if result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("session status = %q, want RUNNING", result.SessionStatus)
	}
}

func TestFinalizeInterruptedTerminalSession_PreservesPartialAndUnavailableResults(t *testing.T) {
	t.Parallel()
	sessionID := "dur-sess-interrupted-finalize-001"
	interruptedAt := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	t.Run("partial result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{InterruptedAt: &interruptedAt},
			},
		}
		priorSession := SessionReadResult{
			SessionID:     sessionID,
			ResultSummary: &ResultSummary{ResultStatus: string(ResultStatusPartial), Summary: "partial output"},
		}
		priorResult := ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusPartial,
			PrimaryResult: json.RawMessage(`{"text":"partial output"}`),
		}
		finalizeInterruptedTerminalSession(state, priorSession, priorResult)
		if state.session.Status != LifecycleStatusInterrupted {
			t.Fatalf("status = %q, want INTERRUPTED", state.session.Status)
		}
		if state.result.ResultStatus != ResultStatusPartial {
			t.Fatalf("result status = %q, want PARTIAL", state.result.ResultStatus)
		}
		if state.session.ResultSummary == nil || state.session.ResultSummary.ResultStatus != string(ResultStatusPartial) {
			t.Fatalf("result summary = %#v, want PARTIAL", state.session.ResultSummary)
		}
	})

	t.Run("unavailable result", func(t *testing.T) {
		state := &runtimeSessionState{
			session: SessionReadResult{
				SessionID: sessionID,
				Status:    LifecycleStatusRunning,
				Lifecycle: &LifecycleTimestamps{FinishedAt: &interruptedAt},
			},
		}
		finalizeInterruptedTerminalSession(state, SessionReadResult{SessionID: sessionID}, ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		})
		if state.result.ResultStatus != ResultStatusUnavailable {
			t.Fatalf("result status = %q, want UNAVAILABLE", state.result.ResultStatus)
		}
		if state.result.Availability == nil || state.result.Availability.Reason != "SESSION_INTERRUPTED" {
			t.Fatalf("availability = %#v, want SESSION_INTERRUPTED", state.result.Availability)
		}
	})
}

func TestResumeHelperFunctions_CoverMergeCloneAndPolicyPaths(t *testing.T) {
	t.Parallel()
	existing := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindCheckpoint, Checkpoint: &factory.JavaScriptCheckpointRecord{ID: "cp-1"}}}
	resumed := []factory.JavaScriptRuntimeRecord{{Kind: factory.JavaScriptRecordKindChildDispatch, ChildDispatch: &factory.JavaScriptChildDispatchRecord{DispatchID: "dispatch-1"}}}
	merged := mergeRuntimeRecords(existing, resumed)
	if len(merged) != 2 {
		t.Fatalf("merged records = %d, want 2", len(merged))
	}
	if len(mergeRuntimeRecords(nil, resumed)) != 1 {
		t.Fatal("mergeRuntimeRecords(nil, resumed) should clone resumed records")
	}
	if len(mergeRuntimeRecords(existing, nil)) != 1 {
		t.Fatal("mergeRuntimeRecords(existing, nil) should clone existing records")
	}

	policy := workflowPolicyFromSessionPolicy(PolicyProjection{})
	defaultPolicy := factory.DefaultJavaScriptPolicy()
	if policy.Mode != defaultPolicy.Mode {
		t.Fatalf("policy mode = %q, want default %q", policy.Mode, defaultPolicy.Mode)
	}
	customPolicy := workflowPolicyFromSessionPolicy(PolicyProjection{
		Effective: map[string]any{"mode": factory.JavaScriptPolicyModeReadOnly},
	})
	if customPolicy.Mode != factory.JavaScriptPolicyModeReadOnly {
		t.Fatalf("policy mode = %q, want %q", customPolicy.Mode, factory.JavaScriptPolicyModeReadOnly)
	}

	summary := &factory.JavaScriptCheckpointSummary{
		CheckpointID:         "checkpoint-1",
		CompletedDispatchIDs: []string{"dispatch-1"},
		PendingDispatchIDs:   []string{"dispatch-2"},
		ArtifactIDs:          []string{"artifact-1"},
		CheckpointState:      map[string]any{"phase": "execute"},
		CreatedAt:            time.Now().UTC(),
	}
	cloned := cloneCheckpointSummary(summary)
	if cloned == nil || cloned.CheckpointID != summary.CheckpointID {
		t.Fatalf("cloneCheckpointSummary = %#v", cloned)
	}
	cloned.CompletedDispatchIDs[0] = "mutated"
	if summary.CompletedDispatchIDs[0] != "dispatch-1" {
		t.Fatal("cloneCheckpointSummary should deep-copy completed dispatch ids")
	}

	if latestCheckpointSummaryFromRuntime(checkpointfixtures.CheckpointSummariesFixture{}, "dur-sess-1", nil, nil) != nil {
		t.Fatal("latestCheckpointSummaryFromRuntime(nil state) = summary, want nil")
	}
}

func TestFakeService_ResumeInterruptedSession_ReturnsUnsupported(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	_, err := service.ResumeInterruptedSession(context.Background(), "dur-sess-petri-run-001", ResumeSessionRequest{
		RequestID: "req-fake-resume-unsupported-001",
	})
	if !errors.Is(err, ErrUnsupportedControl) {
		t.Fatalf("ResumeInterruptedSession error = %v, want ErrUnsupportedControl", err)
	}
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func TestJavaScriptRuntimeService_ResumeInterruptedSession_PackageLocalCoverage(t *testing.T) {
	t.Parallel()
	const sessionID = "dur-sess-0123456789abcdef0123456789abcdef"
	projectRoot := t.TempDir()
	store := mustTestRuntimePersistenceStore(t, runtimepersist.DirForProjectRoot(projectRoot))
	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	startRequest := StartRequest{
		RequestID: "req-package-resume-start-001",
		Source: Source{
			Kind:         factory.WorkflowSourceKindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
	}
	checkpointSummary := checkpointfixtures.ResumableCheckpointSummaryResult()
	state := runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusInterrupted,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:scripted",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, InterruptedAt: &startedAt},
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusInterrupted,
			ResultStatus:  ResultStatusPartial,
		},
		dispatches: []DispatchSummary{{
			ID: "dispatch-1", Status: DispatchStatusCompleted, Attempt: 1,
		}},
		runtimeRecords: []factory.JavaScriptRuntimeRecord{
			{
				Sequence: 1,
				Kind:     factory.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factory.JavaScriptChildDispatchRecord{
					DispatchID: "dispatch-1", ChildIndex: 1,
					Status: factory.JavaScriptChildDispatchStatusCompleted,
					Output: map[string]any{"text": "step one"},
				},
			},
			{
				Sequence: 2,
				Kind:     factory.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factory.JavaScriptCheckpointRecord{
					ID: "checkpoint-1", Label: "after-step-one",
				},
			},
		},
		checkpointSummary: checkpointSummary,
		startRequest:      &startRequest,
		resolvedSource: ResolvedSource{
			Kind:       factory.WorkflowSourceKindWorkflowName,
			SourceRef:  "resumable-two-step-fake-children.workflow.js",
			SourceHash: "sha256:scripted",
			Dialect:    "you-workflow-v1",
		},
		sourceContent: "scripted resumable workflow",
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(&state)
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeState(state))
	if err != nil {
		t.Fatalf("marshal interrupted snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("persist interrupted snapshot: %v", err)
	}

	var resumeContextCalls int
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResumeContextFunc: func(
			summary factory.JavaScriptCompletedCheckpointSummary,
			records []factory.JavaScriptRuntimeRecord,
		) factory.JavaScriptResumeContext {
			resumeContextCalls++
			if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-1" || len(records) != 2 {
				t.Fatalf("resume inputs = %#v / %#v", summary, records)
			}
			return factory.JavaScriptResumeContext{
				CompletedDispatchIDs: []string{"dispatch-1"},
			}
		},
		RunFunc: func(
			_ context.Context,
			request factory.JavaScriptRuntimeRequest,
			_ factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			if request.Resume == nil || len(request.Resume.CompletedDispatchIDs) != 1 {
				t.Fatalf("runtime resume context = %#v", request.Resume)
			}
			value, marshalErr := json.Marshal(map[string]any{"status": "resumed"})
			return factory.JavaScriptRuntimeOutcome{
				OK: true, Value: factory.TypedValue{JSON: value},
			}, marshalErr
		},
	}
	resumedService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
		Workflows:   workflows,
	})

	resumed, err := resumedService.ResumeInterruptedSession(context.Background(), sessionID, ResumeSessionRequest{
		RequestID: "req-package-resume-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.Status != string(LifecycleStatusResuming) && resumed.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("resumed status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}

	if resumed.Status != string(LifecycleStatusSucceeded) {
		waitForResumeCoverageSessionStatus(t, resumedService, sessionID, LifecycleStatusSucceeded, 5*time.Second)
	}
	if resumeContextCalls != 1 {
		t.Fatalf("resume context calls = %d, want 1", resumeContextCalls)
	}
}

type resumeCoverageBlockingProvider struct {
	mu              sync.Mutex
	callCount       int
	blockedOnce     bool
	contextCanceled int
}

func newResumeCoverageBlockingProvider() *resumeCoverageBlockingProvider {
	return &resumeCoverageBlockingProvider{}
}

func (p *resumeCoverageBlockingProvider) Execute(
	ctx context.Context,
	input workerexecution.InvocationInput,
) (workerexecution.InvocationResult, error) {
	p.mu.Lock()
	p.callCount++
	call := p.callCount
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		session := &providers.SessionMetadata{Provider: "mock", Kind: providers.SessionIDKind, ID: "live-provider-session-1"}
		continuation := (session).ContinuationRef()
		response := workerexecution.InferenceResponse{
			Content:      `{"text":"live:resumable-two-step-fake-children:step-one:step-one:workflows","label":"step-one"}`,
			Continuation: continuation,
		}
		return workerexecution.InvocationResult{
			Response: response, Attempt: input.Attempt,
			Continuation: (response.Continuation).ClonePtr(),
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InvocationResult{Attempt: input.Attempt}, ctx.Err()
	}

	session := &providers.SessionMetadata{Provider: "mock", Kind: providers.SessionIDKind, ID: "live-provider-session-2"}
	continuation := (session).ContinuationRef()
	response := workerexecution.InferenceResponse{
		Content:      `{"text":"live:resumable-two-step-fake-children:step-two:step-two:workflows","label":"step-two"}`,
		Continuation: continuation,
	}
	return workerexecution.InvocationResult{
		Response: response, Attempt: input.Attempt,
		Continuation: (response.Continuation).ClonePtr(),
	}, nil
}

func (p *resumeCoverageBlockingProvider) resumeCoverageCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

var _ workerexecution.InvocationExecutor = (*resumeCoverageBlockingProvider)(nil)

func (p *resumeCoverageBlockingProvider) waitForCanceledResumeCoverageInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled > 0
		p.mu.Unlock()
		if canceled {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for blocked provider infer cancellation")
}

func setupResumeCoverageWorkflowFixture(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	path := filepath.Join("..", "..", "..", "..", "..", "tests", "fixtures", "javascript_runtime", "resumable-two-step-fake-children.workflow.js")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "resumable-two-step-fake-children.js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func waitForResumeCoverageSessionStatus(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
	want LifecycleStatus,
	timeout time.Duration,
) SessionReadResult {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := service.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return read
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("session %s did not reach status %s within %s", sessionID, want, timeout)
	return SessionReadResult{}
}

func waitForResumeCoverageDispatchStatus(
	t *testing.T,
	service Service,
	sessionID, dispatchID string,
	want DispatchStatus,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
		if err == nil && dispatch.Status == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach status %s within %s", dispatchID, want, timeout)
}

func newDurableResponseEventsService(t *testing.T) *JavaScriptRuntimeService {
	t.Helper()

	eventsService := eventsstub.New()
	var next atomic.Uint64
	streams, err := responsestreamwire.NewService(func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}, nil, eventsService)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{})
	service.responseStreams = streams
	service.generateResponseEventID = func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}
	return service
}

type progressCapturingChildExecutor struct {
	publisher workers.ProgressPublisher
}

func (e *progressCapturingChildExecutor) Execute(context.Context, workers.InvocationInput) (workers.InvocationResult, error) {
	return workers.InvocationResult{}, nil
}

func seedResponseEventSession(t *testing.T, service *JavaScriptRuntimeService, sessionID string) *runtimeSessionState {
	t.Helper()

	state := &runtimeSessionState{
		session: SessionReadResult{SessionID: sessionID},
	}
	service.mu.Lock()
	service.sessions[sessionID] = state
	service.mu.Unlock()
	return state
}

func validMessageDeltaDraft(dispatchID string) responseevents.Draft {
	payload, _ := json.Marshal(responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         "hello",
	})
	return responseevents.Draft{
		RunID:      dispatchID,
		DispatchID: dispatchID,
		ItemID:     "message-1",
		Kind:       responseevents.KindMessage,
		Phase:      responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "cursor",
			NativeEventType: "content_block_delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload: payload,
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_PublishesAndStreamsChildProgress(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-response-events"
	state := seedResponseEventSession(t, service, sessionID)

	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{
		DispatchID:     "dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dispatch-1"),
	})

	cursor, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{
		SessionID: sessionID,
		Kinds:     []factorysessions.ResponseEventKind{factorysessions.ResponseEventKindMessage},
	})
	if err != nil {
		t.Fatalf("SubscribeResponseEvents: %v", err)
	}
	defer cursor.Detach()

	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("cursor.Next: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one MESSAGE delta", events)
	}
	if events[0].Kind != responseevents.KindMessage || events[0].DispatchID != "dispatch-1" {
		t.Fatalf("event = %#v, want MESSAGE dispatch-1", events[0])
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsMissingStore(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	seedResponseEventSession(t, service, "dur-sess-empty")

	_, err := service.SubscribeResponseEvents(context.Background(), "dur-sess-empty", factorysessions.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-empty",
	})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrSessionNotFound", err)
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RejectsInvalidCursor(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-invalid-cursor"
	state := seedResponseEventSession(t, service, sessionID)
	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{CanonicalDraft: validMessageDeltaDraft("dispatch-1")})

	_, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{
		SessionID:     sessionID,
		AfterSequence: -1,
	})
	if !errors.Is(err, factorysessions.ErrInvalidResponseEventCursor) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrInvalidResponseEventCursor", err)
	}
}

func TestJavaScriptRuntimeService_SessionProgressPublisher_MapsGenericFragments(t *testing.T) {
	t.Parallel()

	service := newDurableResponseEventsService(t)
	sessionID := "dur-sess-ignore"
	state := seedResponseEventSession(t, service, sessionID)

	publisher := service.sessionProgressPublisher(sessionID, state)
	publisher(workers.ProgressFragment{DispatchID: "dispatch-1", Kind: workers.ProgressFragmentKind})
	publisher(workers.ProgressFragment{CanonicalDraft: "not-a-draft"})

	if state.responseEvents == nil {
		t.Fatal("response event store was not initialized")
	}
	if accounting := state.responseEvents.RetentionAccounting(); accounting.EventCount != 1 {
		t.Fatalf("retained events = %#v, want mapped generic progress fragment only", accounting)
	}
}

func TestJavaScriptRuntimeService_SubscribeResponseEvents_RequiresRuntime(t *testing.T) {
	t.Parallel()

	var service *JavaScriptRuntimeService
	_, err := service.SubscribeResponseEvents(context.Background(), "dur-sess-1", factorysessions.ResponseEventSubscriptionRequest{})
	if !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func TestEnsureSessionResponseEvents_RequiresIDGenerator(t *testing.T) {
	t.Parallel()

	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{})
	state := &runtimeSessionState{session: SessionReadResult{SessionID: "dur-sess-missing-id"}}
	if err := service.ensureSessionResponseEvents("dur-sess-missing-id", state); err == nil {
		t.Fatal("ensureSessionResponseEvents succeeded without ID generator")
	}
}

// TestPublishWorkerProgress_ReachesOnlyTheSessionThatStartedTheWorker pins the
// routing a JavaScript child's output depends on.
//
// A child is a Worker, so its output arrives from Workers addressed only by
// dispatch. Two durable sessions of one Factory share that Factory's Workers
// pool, so the dispatch identity is the only thing that can tell their output
// apart -- and a fragment delivered to the wrong session would show one
// customer's provider output inside another's session.
func TestPublishWorkerProgress_ReachesOnlyTheSessionThatStartedTheWorker(t *testing.T) {
	service := newDurableResponseEventsService(t)
	first := seedResponseEventSession(t, service, "dur-sess-first")
	second := seedResponseEventSession(t, service, "dur-sess-second")
	if err := service.ensureSessionResponseEvents("dur-sess-first", first); err != nil {
		t.Fatalf("ensure first response events: %v", err)
	}
	if err := service.ensureSessionResponseEvents("dur-sess-second", second); err != nil {
		t.Fatalf("ensure second response events: %v", err)
	}

	release := service.observeWorkerDispatch("dur-sess-first/dispatch-1", "dur-sess-first")
	defer release()

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "dur-sess-first/dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dur-sess-first/dispatch-1"),
	})

	if got := first.responseEvents.RetentionAccounting().EventCount; got != 1 {
		t.Fatalf("owning session response events = %d, want 1", got)
	}
	if got := second.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("other session response events = %d, want 0", got)
	}
}

// TestPublishWorkerProgress_IgnoresADispatchNoSessionOwns keeps the fan-out
// safe for every other Worker in the process: a Petri Worker's progress passes
// through the same publisher and must land nowhere here.
func TestPublishWorkerProgress_IgnoresADispatchNoSessionOwns(t *testing.T) {
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, "dur-sess-first")
	if err := service.ensureSessionResponseEvents("dur-sess-first", state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "petri-dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("petri-dispatch-1"),
	})

	if got := state.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("response events for an unowned dispatch = %d, want 0", got)
	}
}

// TestPublishWorkerProgress_StopsOnceTheWorkerIsReleased proves the claim is
// scoped to the Worker's run. Holding it forever would grow the index by one
// entry per child for the life of the process and keep routing output from a
// dispatch identity Workers may hand to nobody.
func TestPublishWorkerProgress_StopsOnceTheWorkerIsReleased(t *testing.T) {
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, "dur-sess-first")
	if err := service.ensureSessionResponseEvents("dur-sess-first", state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	release := service.observeWorkerDispatch("dur-sess-first/dispatch-1", "dur-sess-first")
	release()

	service.PublishWorkerProgress(workers.ProgressFragment{
		DispatchID:     "dur-sess-first/dispatch-1",
		CanonicalDraft: validMessageDeltaDraft("dur-sess-first/dispatch-1"),
	})

	if got := state.responseEvents.RetentionAccounting().EventCount; got != 0 {
		t.Fatalf("response events after release = %d, want 0", got)
	}
}

// TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession pins what makes
// those routing keys distinct in the first place.
//
// Child dispatch identities are minted per session and start again at
// dispatch-1 for each, while the Workers pool they share treats a dispatch ID
// as single-use for its whole life. Two sessions submitting an unqualified
// dispatch-1 would leave the second refused outright.
func TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession(t *testing.T) {
	first := newChildWorkerExecutor("dur-sess-first", nil, nil, nil, nil, 0, "")
	second := newChildWorkerExecutor("dur-sess-second", nil, nil, nil, nil, 0, "")

	firstID := first.workerDispatchIdentity("dispatch-1")
	secondID := second.workerDispatchIdentity("dispatch-1")
	if firstID == secondID {
		t.Fatalf("two sessions submitted the same Workers dispatch identity %q", firstID)
	}
	if firstID != "dur-sess-first/dispatch-1" {
		t.Fatalf("Workers dispatch identity = %q, want the session-scoped identity", firstID)
	}
}
