package projections

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestIncrementalSessionProjection_ReplaysSessionFactsAndDetachesSnapshots(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, time.August, 24, 10, 0, 0, 0, time.FixedZone("fixture", -7*60*60))
	pausedAt := startedAt.Add(time.Minute)
	resumedAt := pausedAt.Add(time.Minute)
	completedAt := resumedAt.Add(time.Minute)
	sessionID := "session-incremental"
	orchestratorKind := "javascript"
	orchestratorDialect := "workflow-v2"
	dispatchID := "dispatch-incremental"
	phaseName := "review"
	checkpointID := "checkpoint-incremental"
	argsDigest := "sha256:args"
	factoryID := "factory-incremental"
	sourceRef := "factory.js"
	sourceHash := "sha256:source"
	policyHash := "sha256:policy"
	approvalWorkIDs := []string{"work-2", "work-1"}
	approvalTraceIDs := []string{"trace-1"}
	label := "review dispatch"
	model := "gpt-5"
	provider := "openai"
	runnerID := "runner-1"
	promptDigest := "sha256:prompt"
	schemaDigest := "sha256:schema"
	skipPermissions := true
	artifactHash := "sha256:artifact"
	artifactSize := int64(256)
	redactedSecrets := int32(2)
	redactedPaths := int32(1)
	redactedTokens := int32(3)
	mimeType := "application/json"
	durationMillis := int64(180000)
	inputTokens := int64(10)
	outputTokens := int64(20)
	totalTokens := int64(30)
	costUSD := 0.25
	retryCount := int32(1)
	dispatchCounts := interfaces.FactorySessionChildDispatchCounts{Queued: 1, Running: 2, Completed: 3}
	resultStatus := interfaces.FactorySessionResultStatusFailedWithPartial

	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "incremental-factory",
		"workstations": []any{map[string]any{
			"id":   "approval-workstation",
			"name": "Release Approval",
			"description": map[string]any{
				"type":    interfaces.NameValueTypeLocalizableAsset,
				"value":   "release-approval",
				"locales": []any{"en-US", "fr-FR"},
				"values":  map[string]any{"en-US": "Approve release", "fr-FR": "Approuver la version"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	artifactCapturedAt := resumedAt.Add(10 * time.Second)
	checkpointTimestamp := resumedAt.Add(20 * time.Second)
	projection := NewIncrementalSessionProjection()
	events := []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInitialStructureRequest,
			interfaces.FactoryEventContext{EventTime: startedAt},
			interfaces.InitialStructureRequestEventPayload{Factory: snapshot}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionStarted,
			interfaces.FactoryEventContext{
				EventTime:           startedAt,
				SessionID:           &sessionID,
				OrchestratorKind:    &orchestratorKind,
				OrchestratorDialect: &orchestratorDialect,
			},
			interfaces.FactorySessionStartedEventPayload{
				ArgsDigest: &argsDigest, FactoryID: &factoryID, SourceRef: &sourceRef,
				SourceHash: &sourceHash, PolicyHash: &policyHash, StartedAt: startedAt,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionPaused,
			interfaces.FactoryEventContext{EventTime: pausedAt, SessionID: &sessionID},
			interfaces.FactorySessionPausedEventPayload{PausedAt: pausedAt, Status: interfaces.FactorySessionLifecycleStatusPaused}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResumed,
			interfaces.FactoryEventContext{EventTime: resumedAt, SessionID: &sessionID},
			interfaces.FactorySessionResumedEventPayload{ResumedAt: resumedAt, Status: interfaces.FactorySessionLifecycleStatusRunning}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeJavaScriptPhaseChange,
			interfaces.FactoryEventContext{EventTime: resumedAt, SessionID: &sessionID},
			interfaces.JavaScriptPhaseChangeEventPayload{
				ArgsDigest: &argsDigest, ChildDispatchCounts: dispatchCounts, Phase: phaseName,
				Phases: []string{"plan", phaseName}, ScriptStatus: interfaces.FactorySessionJavaScriptScriptStatusRunning,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
			interfaces.FactoryEventContext{EventTime: checkpointTimestamp, SessionID: &sessionID, CheckpointID: &checkpointID},
			interfaces.OrchestratorCheckpointWrittenEventPayload{
				ArtifactRef: &interfaces.FactoryArtifactRef{ID: "artifact-checkpoint", Kind: "CHECKPOINT", Visibility: "INTERNAL", ContentHash: &artifactHash, SizeBytes: &artifactSize},
				Label:       "after review", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
				Timestamp: &checkpointTimestamp, Warnings: []interfaces.FactoryDispatchWarning{{Code: "WARN", Message: "checkpoint retained"}},
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeArtifactCreated,
			interfaces.FactoryEventContext{EventTime: artifactCapturedAt, SessionID: &sessionID},
			interfaces.ArtifactCreatedEventPayload{
				Artifact: interfaces.FactoryArtifact{
					ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC", Label: &label,
					Summary: &sourceRef, AuditMode: &orchestratorKind, ContentHash: &artifactHash, SizeBytes: &artifactSize,
					RedactionCounts: &interfaces.FactoryArtifactRedactionCounts{Secrets: &redactedSecrets, Paths: &redactedPaths, Tokens: &redactedTokens},
					CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{CapturedAt: &artifactCapturedAt, SourceDispatchID: &dispatchID, MIMEType: &mimeType},
				},
				CapturedAt: &artifactCapturedAt,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchQueued,
			interfaces.FactoryEventContext{EventTime: resumedAt, SessionID: &sessionID, DispatchID: &dispatchID, PhaseName: &phaseName},
			interfaces.DispatchQueuedEventPayload{
				DispatchKind: interfaces.FactoryDispatchKindJavaScriptTool, InputWorkIDs: &approvalWorkIDs,
				Label: &label, Model: &model, Provider: &provider, RunnerID: &runnerID,
				PromptDigest: &promptDigest, SchemaDigest: &schemaDigest, SkipPermissions: &skipPermissions,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchReconciled,
			interfaces.FactoryEventContext{EventTime: completedAt, SessionID: &sessionID, DispatchID: &dispatchID, PhaseName: &phaseName},
			interfaces.DispatchReconciledEventPayload{
				ArtifactIDs: &[]string{"artifact-result"}, ReconciledStatus: interfaces.FactoryDispatchStatusCompleted,
				ReconciliationSource: interfaces.DispatchReconciliationSourceStreamReplay, Replayed: true,
				Usage: &interfaces.FactoryDispatchUsage{
					CostUSD: &costUSD, DurationMillis: &durationMillis, InputTokens: &inputTokens,
					OutputTokens: &outputTokens, RetryCount: &retryCount, TotalTokens: &totalTokens,
				},
				FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureType("provider"), Message: "provider failed after partial output"},
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeHumanApprovalRequested,
			interfaces.FactoryEventContext{
				EventTime: completedAt, SessionID: &sessionID, DispatchID: &dispatchID,
				WorkIDs: &approvalWorkIDs, TraceIDs: &approvalTraceIDs,
			},
			interfaces.HumanApprovalRequestedEventPayload{
				ApprovalID: "approval-1", WorkstationID: "approval-workstation",
				Decisions: []interfaces.HumanApprovalDecision{interfaces.HumanApprovalDecisionApprove, interfaces.HumanApprovalDecisionReject},
				Status:    interfaces.HumanApprovalStatusPending,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResultUpdated,
			interfaces.FactoryEventContext{EventTime: completedAt, SessionID: &sessionID},
			interfaces.FactorySessionResultUpdatedEventPayload{
				ArtifactIDs: []string{"artifact-result"}, ResultStatus: resultStatus,
				ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial result"}},
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionCompleted,
			interfaces.FactoryEventContext{EventTime: completedAt, SessionID: &sessionID},
			interfaces.FactorySessionCompletedEventPayload{
				ArtifactIDs: []string{"artifact-result"}, CompletedAt: completedAt, DurationMillis: &durationMillis,
				DispatchCounts: &dispatchCounts, FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureType("provider"), Message: "partial result"},
				FinalStatus: interfaces.FactorySessionLifecycleStatusFailed, ResultStatus: &resultStatus,
			}),
	}

	for _, event := range events {
		if err := projection.Apply(event); err != nil {
			t.Fatalf("Apply(%s): %v", event.Type, err)
		}
	}

	facts := projection.SnapshotSessionProjectionFacts()
	if approval := facts.PendingHumanApprovals["approval-1"]; approval.WorkstationName != "Release Approval" || approval.WorkstationDescription == nil ||
		approval.WorkstationDescription.Values["fr-FR"] != "Approuver la version" || approval.WorkItemIDs[0] != "work-2" {
		t.Fatalf("pending approval facts = %#v, want resolved topology and correlation facts", approval)
	}
	if facts.SessionBracket == nil || facts.SessionBracket.LifecycleControlStatus != string(interfaces.FactorySessionLifecycleStatusRunning) ||
		!facts.SessionBracket.Terminal || facts.SessionBracket.FinalStatus != string(interfaces.FactorySessionLifecycleStatusFailed) ||
		facts.SessionBracket.DispatchCounts.Completed != 3 || facts.SessionBracket.FailureDetail == nil {
		t.Fatalf("session bracket facts = %#v, want terminal failed bracket with controls and counts", facts.SessionBracket)
	}
	if facts.JavaScriptRuntime == nil || facts.JavaScriptRuntime.Phase != phaseName || facts.JavaScriptRuntime.ArgsDigest != argsDigest ||
		len(facts.JavaScriptRuntime.Checkpoints) != 1 || len(facts.JavaScriptRuntime.Dispatches) != 1 || len(facts.JavaScriptRuntime.Artifacts) != 1 ||
		facts.JavaScriptRuntime.PrimaryResult[0].Text != "partial result" {
		t.Fatalf("JavaScript runtime facts = %#v, want detached runtime result", facts.JavaScriptRuntime)
	}
	dispatch := facts.JavaScriptRuntime.Dispatches[0]
	if dispatch.Status != string(interfaces.FactoryDispatchStatusCompleted) || dispatch.JavaScript == nil || !dispatch.JavaScript.SkipPermissions ||
		dispatch.Usage == nil || dispatch.Usage.TotalTokens != totalTokens || dispatch.FailureDetail == nil || dispatch.FailureDetail.Message == "" {
		t.Fatalf("dispatch facts = %#v, want reconciled usage/failure metadata", dispatch)
	}

	facts.PendingHumanApprovals["approval-1"].Decisions[0] = "MUTATED"
	facts.PendingHumanApprovals["approval-1"].WorkstationDescription.Values["fr-FR"] = "MUTATED"
	facts.JavaScriptRuntime.Phases[0] = "MUTATED"
	facts.JavaScriptRuntime.Checkpoints[0].Warnings[0].Code = "MUTATED"
	facts.JavaScriptRuntime.Checkpoints[0].ArtifactRef.ID = "MUTATED"
	facts.JavaScriptRuntime.Dispatches[0].RelatedWorkIDs[0] = "MUTATED"
	facts.JavaScriptRuntime.Dispatches[0].Usage.TotalTokens = -1
	facts.JavaScriptRuntime.Dispatches[0].FailureDetail.Message = "MUTATED"
	facts.JavaScriptRuntime.Artifacts[0].RedactionCounts["secrets"] = -1
	facts.JavaScriptRuntime.Artifacts[0].CaptureMetadata["mimeType"] = "MUTATED"
	facts.JavaScriptRuntime.PrimaryResult[0].Text = "MUTATED"
	facts.SessionBracket.ResultSummary[0].Text = "MUTATED"
	facts.SessionBracket.ArtifactIDs[0] = "MUTATED"
	facts.SessionBracket.DispatchCounts.Completed = -1
	facts.SessionBracket.FailureDetail.Message = "MUTATED"

	fresh := projection.SnapshotSessionProjectionFacts()
	if fresh.PendingHumanApprovals["approval-1"].Decisions[0] != interfaces.HumanApprovalDecisionApprove ||
		fresh.PendingHumanApprovals["approval-1"].WorkstationDescription.Values["fr-FR"] != "Approuver la version" ||
		fresh.JavaScriptRuntime.Phases[0] != "plan" || fresh.JavaScriptRuntime.Checkpoints[0].Warnings[0].Code != "WARN" ||
		fresh.JavaScriptRuntime.Checkpoints[0].ArtifactRef.ID != "artifact-checkpoint" || fresh.JavaScriptRuntime.Dispatches[0].RelatedWorkIDs[0] != "work-2" ||
		fresh.JavaScriptRuntime.Dispatches[0].Usage.TotalTokens != totalTokens || fresh.JavaScriptRuntime.Dispatches[0].FailureDetail.Message == "MUTATED" ||
		fresh.JavaScriptRuntime.Artifacts[0].RedactionCounts["secrets"] != int(redactedSecrets) || fresh.JavaScriptRuntime.Artifacts[0].CaptureMetadata["mimeType"] != mimeType ||
		fresh.JavaScriptRuntime.PrimaryResult[0].Text != "partial result" || fresh.SessionBracket.ResultSummary[0].Text != "partial result" ||
		fresh.SessionBracket.ArtifactIDs[0] != "artifact-result" || fresh.SessionBracket.DispatchCounts.Completed != 3 ||
		fresh.SessionBracket.FailureDetail.Message == "MUTATED" {
		t.Fatalf("SnapshotSessionProjectionFacts returned aliased state: %#v", fresh)
	}

	var zeroValue IncrementalSessionProjection
	if err := zeroValue.Apply(events[1]); err != nil {
		t.Fatalf("zero-value Apply: %v", err)
	}
	if zeroValue.SnapshotSessionProjectionFacts().SessionBracket == nil {
		t.Fatal("zero-value projection should initialize its reducer on Apply")
	}
	var nilProjection *IncrementalSessionProjection
	if err := nilProjection.Apply(events[1]); err != nil {
		t.Fatalf("nil projection Apply: %v", err)
	}
	if facts := nilProjection.SnapshotSessionProjectionFacts(); facts.SessionBracket != nil || facts.JavaScriptRuntime != nil || facts.PendingHumanApprovals != nil {
		t.Fatalf("nil projection snapshot = %#v, want empty facts", facts)
	}
}
