package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestChildWorkerExecutor_PassesDetachedCorrelationAndOneProgressPublisher(t *testing.T) {
	invoker := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: `{"text":"done"}`,
		}}},
	}}
	var progress []workers.ProgressFragment
	executor := newTestChildWorkerExecutor(invoker, newChildRecordSink(), nil)
	executor.runtimeID = "runtime-child"
	executor.generationID = "generation-child"
	executor.publish = func(dispatchID string, fragment workers.ProgressFragment) {
		if fragment.DispatchID != dispatchID {
			t.Fatalf("progress dispatch = %q, want %q", fragment.DispatchID, dispatchID)
		}
		progress = append(progress, fragment)
	}
	invoker.onExecute = func(request workers.ExecuteRequest) {
		if request.Input.ProgressPublisher == nil {
			t.Fatal("Workers Execute request has no request-scoped progress publisher")
		}
		request.Input.ProgressPublisher(workers.ProgressFragment{
			Kind:    workers.CompletedFragmentKind,
			Payload: "terminal",
		})
	}

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "summarize",
		Preset:        "child-worker",
		ModelProvider: "codex",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertSchemaLessChildResult(t, result)
	assertDetachedChildRequest(t, invoker.request)
	if len(progress) != 1 || progress[0].Kind != workers.CompletedFragmentKind {
		t.Fatalf("progress observations = %#v, want one terminal fragment", progress)
	}
}

func assertSchemaLessChildResult(t *testing.T, result factory.JavaScriptChildExecutionResult) {
	t.Helper()
	if result.SchemaValidated {
		t.Fatal("schema-less child schemaValidated = true, want false")
	}
	if result.Output["text"] != `{"text":"done"}` {
		t.Fatalf("schema-less child output = %#v, want prose-compatible text", result.Output)
	}
	if _, exists := result.Output["schemaValidated"]; exists {
		t.Fatalf("schema-less child output = %#v, want metadata outside customer output", result.Output)
	}
}

func assertDetachedChildRequest(t *testing.T, request workers.ExecuteRequest) {
	t.Helper()
	assertChildCorrelation(t, request)
	if request.Input.Dispatch.DispatchID != request.Correlation.DispatchID {
		t.Fatalf("child dispatch input = %#v, want correlation dispatch ID", request.Input.Dispatch)
	}
	if request.Target.WorkstationName != workers.ProviderInvocationRoute || request.Target.WorkerName != "child-worker" {
		t.Fatalf("child detached routing = %#v, want provider-invocation route and preset", request.Target)
	}
}

func assertChildCorrelation(t *testing.T, request workers.ExecuteRequest) {
	t.Helper()
	correlation := request.Correlation
	if correlation.FactorySessionID != "dur-sess-1" || correlation.RuntimeID != "runtime-child" {
		t.Fatalf("child correlation session/runtime = %#v, want session/runtime identity", correlation)
	}
	if correlation.GenerationID != "generation-child" || correlation.DispatchID != "dur-sess-1/dispatch-1" {
		t.Fatalf("child correlation generation/dispatch = %#v, want generation/dispatch identity", correlation)
	}
}

func TestDirectChildExecutor_StructuredMismatchRetriesWithinPolicy(t *testing.T) {
	const diagnostic = "structured output schema violation: instance /answer; expected string"
	attempts := 0
	var requests []workers.ExecuteRequest
	involver := &recordingWorkerExecution{
		result: workers.ExecuteResult{
			Outcome: workers.ExecutionOutcomeFailed,
			Failure: &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: diagnostic,
				Detail: &workers.FailureDetail{
					Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
					Message: diagnostic,
				},
			},
		},
	}
	involver.onExecute = func(request workers.ExecuteRequest) {
		attempts++
		requests = append(requests, request)
		if attempts == 2 {
			involver.result = workers.ExecuteResult{
				Outcome: workers.ExecutionOutcomeAccepted,
				StructuredResult: map[string]any{
					"answer": "validated answer",
				},
				StructuredResultPresent: true,
			}
		}
	}
	sink := newChildRecordSink()
	service := &JavaScriptRuntimeService{
		projectRoot: "/project",
		childValues: childTestValues{},
	}
	service.SetDirectWorkerExecution(involver)
	policy := factory.DefaultJavaScriptPolicy()
	policy.MaxRetries = 1
	hooks := service.childExecutorHooks(ChildExecutorModeLive, "direct-structured-retry")
	executor := hooks.NewChildExecutor("direct-structured-retry", sink, policy)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "return a structured answer",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if attempts != 2 || len(requests) != 2 {
		t.Fatalf("attempts = %d, requests = %d, want two", attempts, len(requests))
	}
	if requests[0].Attempt.Number != 1 || requests[1].Attempt.Number != 2 || requests[0].Correlation.AttemptID != "dispatch-1/attempt/1" || requests[1].Correlation.AttemptID != "dispatch-1/attempt/2" {
		t.Fatalf("attempt requests = %#v, want numbered detached attempts", requests)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusCompleted || !result.SchemaValidated {
		t.Fatalf("child result = %#v, want completed schema-validated result", result)
	}
	if result.Output["answer"] != "validated answer" {
		t.Fatalf("child answer = %#v, want validated native output", result.Output["answer"])
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 2 || !terminal.SchemaValidated {
		t.Fatalf("terminal record = %#v, want final attempt 2 and validated metadata", terminal)
	}
	if len(sink.statuses) != 2 {
		t.Fatalf("dispatch statuses = %v, want one queued and one running record", sink.statuses)
	}
}

func TestDirectChildExecutor_ExhaustedStructuredMismatchFailsWithoutOutput(t *testing.T) {
	const diagnostic = "structured output schema violation: instance /answer; expected string"
	attempts := 0
	involver := &recordingWorkerExecution{result: workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeStructuredOutputSchemaViolation,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: diagnostic,
			Detail: &workers.FailureDetail{
				Reason:  workers.WorkFailureTypeStructuredOutputSchemaViolation,
				Message: diagnostic,
			},
		},
	}}
	involver.onExecute = func(_ workers.ExecuteRequest) { attempts++ }
	sink := newChildRecordSink()
	executor := newDirectChildExecutor(
		"direct-structured-exhausted",
		involver,
		sink,
		childTestValues{},
		"/project",
		2,
	)

	result, err := executor.Execute(context.Background(), factory.JavaScriptChildExecutionRequest{
		Prompt:        "do not expose invalid output",
		ModelProvider: "codex",
		OutputSchema:  map[string]any{"type": "object"},
	})
	if err == nil || !strings.Contains(err.Error(), "/answer") {
		t.Fatalf("Execute error = %v, want the safe schema path diagnostic", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want initial attempt plus two retries", attempts)
	}
	if result.Status != factory.JavaScriptChildDispatchStatusFailed || len(result.Output) != 0 || result.SchemaValidated {
		t.Fatalf("child result = %#v, want failed with no output or validation metadata", result)
	}
	if strings.Contains(err.Error(), "do not expose invalid output") {
		t.Fatalf("Execute error = %q, must not expose the prompt", err)
	}
	terminal := sink.terminalChildDispatch(t)
	if terminal.Attempt != 3 || terminal.Output != nil || terminal.SchemaValidated {
		t.Fatalf("terminal record = %#v, want final failed attempt without output", terminal)
	}
}

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
	if policy.MaxAgents != defaultPolicy.MaxAgents || policy.Concurrency != defaultPolicy.Concurrency {
		t.Fatalf("policy budgets = %d/%d, want default %d/%d", policy.MaxAgents, policy.Concurrency, defaultPolicy.MaxAgents, defaultPolicy.Concurrency)
	}
	customPolicy := workflowPolicyFromSessionPolicy(PolicyProjection{
		Effective: map[string]any{"allowedPermissions": []any{factory.JavaScriptPolicyPermissionDefault}},
	})
	if len(customPolicy.AllowedPermissions) != 1 || customPolicy.AllowedPermissions[0] != factory.JavaScriptPolicyPermissionDefault {
		t.Fatalf("allowedPermissions = %#v, want DEFAULT", customPolicy.AllowedPermissions)
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
	persistResumeCoverageSnapshot(t, store, sessionID, state)

	var resumeContextCalls int
	workflows := resumeCoverageWorkflows(t, &resumeContextCalls)
	resumedService := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
		Persistence: store,
		Workflows:   workflows,
	})
	assertResumeCoverageEligibility(t, resumedService, sessionID)
	completeResumeCoverage(t, resumedService, sessionID, &resumeContextCalls)
}

func persistResumeCoverageSnapshot(
	t *testing.T,
	store runtimepersist.Store,
	sessionID string,
	state runtimeSessionState,
) {
	t.Helper()
	encoded, err := json.Marshal(persistedSnapshotFromRuntimeStateWithFailureLogCapacity(state, defaultPersistedTokenFailureLogCapacity))
	if err != nil {
		t.Fatalf("marshal interrupted snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("persist interrupted snapshot: %v", err)
	}
}

func resumeCoverageWorkflows(
	t *testing.T,
	resumeContextCalls *int,
) factory.JavaScriptWorkflows {
	t.Helper()
	return factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResumeContextFunc: func(
			summary factory.JavaScriptCompletedCheckpointSummary,
			records []factory.JavaScriptRuntimeRecord,
		) factory.JavaScriptResumeContext {
			*resumeContextCalls++
			if len(summary.CompletedDispatchIDs) != 1 || summary.CompletedDispatchIDs[0] != "dispatch-1" || len(records) != 2 {
				t.Fatalf("resume inputs = %#v / %#v", summary, records)
			}
			return factory.JavaScriptResumeContext{CompletedDispatchIDs: []string{"dispatch-1"}}
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
}

func assertResumeCoverageEligibility(t *testing.T, service *JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	available, err := service.HasRestorableState(context.Background(), sessionID)
	if err != nil || !available {
		t.Fatalf("HasRestorableState() = %t, %v, want true without mutation", available, err)
	}
	if read, err := service.GetSession(context.Background(), sessionID); err != nil || read.Status != LifecycleStatusInterrupted {
		t.Fatalf("session after HasRestorableState() = %#v, %v, want INTERRUPTED", read, err)
	}
}

func completeResumeCoverage(
	t *testing.T,
	service *JavaScriptRuntimeService,
	sessionID string,
	resumeContextCalls *int,
) {
	t.Helper()
	resumed, err := service.ResumeInterruptedSession(context.Background(), sessionID, ResumeSessionRequest{
		RequestID: "req-package-resume-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.Status != string(LifecycleStatusResuming) && resumed.Status != string(LifecycleStatusSucceeded) {
		t.Fatalf("resumed status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}
	if resumed.Status != string(LifecycleStatusSucceeded) {
		waitForResumeCoverageSessionStatus(t, service, sessionID, LifecycleStatusSucceeded, 5*time.Second)
	}
	if *resumeContextCalls != 1 {
		t.Fatalf("resume context calls = %d, want 1", *resumeContextCalls)
	}
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
	first := newChildWorkerExecutor("dur-sess-first", nil, nil, nil, nil, "", 0)
	second := newChildWorkerExecutor("dur-sess-second", nil, nil, nil, nil, "", 0)

	firstID := first.workerDispatchIdentity("dispatch-1")
	secondID := second.workerDispatchIdentity("dispatch-1")
	if firstID == secondID {
		t.Fatalf("two sessions submitted the same Workers dispatch identity %q", firstID)
	}
	if firstID != "dur-sess-first/dispatch-1" {
		t.Fatalf("Workers dispatch identity = %q, want the session-scoped identity", firstID)
	}
}
