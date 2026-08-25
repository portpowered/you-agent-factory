package projections

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type incrementalProjectionScenario struct {
	startedAt, pausedAt, resumedAt, completedAt time.Time
	artifactCapturedAt, checkpointTimestamp     time.Time

	sessionID, orchestratorKind, orchestratorDialect string
	dispatchID, phaseName, checkpointID              string
	argsDigest, factoryID, sourceRef                 string
	sourceHash, policyHash, artifactHash             string
	label, model, provider, runnerID                 string
	promptDigest, schemaDigest, mimeType             string
	approvalWorkIDs, approvalTraceIDs                []string

	snapshot                                               *interfaces.FactorySnapshot
	skipPermissions                                        bool
	artifactSize                                           int64
	redactedSecrets, redactedPaths, redactedTokens         int32
	durationMillis, inputTokens, outputTokens, totalTokens int64
	costUSD                                                float64
	retryCount                                             int32
	dispatchCounts                                         interfaces.FactorySessionChildDispatchCounts
	resultStatus                                           interfaces.FactorySessionResultStatus
}

func TestIncrementalSessionProjection_ReplaysSessionFactsAndDetachesSnapshots(t *testing.T) {
	t.Parallel()

	scenario := newIncrementalProjectionScenario(t)
	projection := NewIncrementalSessionProjection()
	events := scenario.events(t)
	for _, event := range events {
		if err := projection.Apply(event); err != nil {
			t.Fatalf("Apply(%s): %v", event.Type, err)
		}
	}

	assertIncrementalProjectionFacts(t, projection, scenario)
	mutateIncrementalProjectionSnapshot(projection)
	assertIncrementalProjectionSnapshotIsDetached(t, projection, scenario)
	assertIncrementalProjectionZeroValues(t, events)
}

func newIncrementalProjectionScenario(t *testing.T) incrementalProjectionScenario {
	t.Helper()
	startedAt := time.Date(2026, time.August, 24, 10, 0, 0, 0, time.FixedZone("fixture", -7*60*60))
	pausedAt := startedAt.Add(time.Minute)
	resumedAt := pausedAt.Add(time.Minute)
	completedAt := resumedAt.Add(time.Minute)
	return incrementalProjectionScenario{
		startedAt: startedAt, pausedAt: pausedAt, resumedAt: resumedAt, completedAt: completedAt,
		artifactCapturedAt: resumedAt.Add(10 * time.Second), checkpointTimestamp: resumedAt.Add(20 * time.Second),
		sessionID: "session-incremental", orchestratorKind: "javascript", orchestratorDialect: "workflow-v2",
		dispatchID: "dispatch-incremental", phaseName: "review", checkpointID: "checkpoint-incremental",
		argsDigest: "sha256:args", factoryID: "factory-incremental", sourceRef: "factory.js",
		sourceHash: "sha256:source", policyHash: "sha256:policy", artifactHash: "sha256:artifact",
		approvalWorkIDs: []string{"work-2", "work-1"}, approvalTraceIDs: []string{"trace-1"},
		label: "review dispatch", model: "gpt-5", provider: "openai", runnerID: "runner-1",
		promptDigest: "sha256:prompt", schemaDigest: "sha256:schema", mimeType: "application/json",
		snapshot: newIncrementalProjectionSnapshot(t), skipPermissions: true, artifactSize: 256,
		redactedSecrets: 2, redactedPaths: 1, redactedTokens: 3, durationMillis: 180000,
		inputTokens: 10, outputTokens: 20, totalTokens: 30, costUSD: 0.25, retryCount: 1,
		dispatchCounts: interfaces.FactorySessionChildDispatchCounts{Queued: 1, Running: 2, Completed: 3},
		resultStatus:   interfaces.FactorySessionResultStatusFailedWithPartial,
	}
}

func newIncrementalProjectionSnapshot(t *testing.T) *interfaces.FactorySnapshot {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name": "incremental-factory",
		"workstations": []any{map[string]any{
			"id": "approval-workstation", "name": "Release Approval",
			"description": map[string]any{
				"type": interfaces.NameValueTypeLocalizableAsset, "value": "release-approval",
				"locales": []any{"en-US", "fr-FR"},
				"values":  map[string]any{"en-US": "Approve release", "fr-FR": "Approuver la version"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
}

func (s incrementalProjectionScenario) events(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	events := s.structureEvents(t)
	events = append(events, s.javascriptRuntimeEvents(t)...)
	events = append(events, s.dispatchEvents(t)...)
	return append(events, s.sessionResultEvents(t)...)
}

func (s incrementalProjectionScenario) structureEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	sessionStarted := interfaces.FactorySessionStartedEventPayload{
		ArgsDigest: &s.argsDigest, FactoryID: &s.factoryID, SourceRef: &s.sourceRef,
		SourceHash: &s.sourceHash, PolicyHash: &s.policyHash, StartedAt: s.startedAt,
	}
	return []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeInitialStructureRequest,
			interfaces.FactoryEventContext{EventTime: s.startedAt},
			interfaces.InitialStructureRequestEventPayload{Factory: s.snapshot}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionStarted,
			interfaces.FactoryEventContext{EventTime: s.startedAt, SessionID: &s.sessionID,
				OrchestratorKind: &s.orchestratorKind, OrchestratorDialect: &s.orchestratorDialect}, sessionStarted),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionPaused,
			interfaces.FactoryEventContext{EventTime: s.pausedAt, SessionID: &s.sessionID},
			interfaces.FactorySessionPausedEventPayload{PausedAt: s.pausedAt, Status: interfaces.FactorySessionLifecycleStatusPaused}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResumed,
			interfaces.FactoryEventContext{EventTime: s.resumedAt, SessionID: &s.sessionID},
			interfaces.FactorySessionResumedEventPayload{ResumedAt: s.resumedAt, Status: interfaces.FactorySessionLifecycleStatusRunning}),
	}
}

func (s incrementalProjectionScenario) javascriptRuntimeEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	return []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeJavaScriptPhaseChange,
			interfaces.FactoryEventContext{EventTime: s.resumedAt, SessionID: &s.sessionID},
			interfaces.JavaScriptPhaseChangeEventPayload{
				ArgsDigest: &s.argsDigest, ChildDispatchCounts: s.dispatchCounts, Phase: s.phaseName,
				Phases: []string{"plan", s.phaseName}, ScriptStatus: interfaces.FactorySessionJavaScriptScriptStatusRunning,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
			interfaces.FactoryEventContext{EventTime: s.checkpointTimestamp, SessionID: &s.sessionID, CheckpointID: &s.checkpointID},
			interfaces.OrchestratorCheckpointWrittenEventPayload{
				ArtifactRef: &interfaces.FactoryArtifactRef{ID: "artifact-checkpoint", Kind: "CHECKPOINT", Visibility: "INTERNAL", ContentHash: &s.artifactHash, SizeBytes: &s.artifactSize},
				Label:       "after review", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
				Timestamp: &s.checkpointTimestamp, Warnings: []interfaces.FactoryDispatchWarning{{Code: "WARN", Message: "checkpoint retained"}},
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeArtifactCreated,
			interfaces.FactoryEventContext{EventTime: s.artifactCapturedAt, SessionID: &s.sessionID},
			interfaces.ArtifactCreatedEventPayload{
				Artifact: interfaces.FactoryArtifact{
					ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC", Label: &s.label,
					Summary: &s.sourceRef, AuditMode: &s.orchestratorKind, ContentHash: &s.artifactHash, SizeBytes: &s.artifactSize,
					RedactionCounts: &interfaces.FactoryArtifactRedactionCounts{Secrets: &s.redactedSecrets, Paths: &s.redactedPaths, Tokens: &s.redactedTokens},
					CaptureMetadata: &interfaces.FactoryArtifactCaptureMetadata{CapturedAt: &s.artifactCapturedAt, SourceDispatchID: &s.dispatchID, MIMEType: &s.mimeType},
				},
				CapturedAt: &s.artifactCapturedAt,
			}),
	}
}

func (s incrementalProjectionScenario) dispatchEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	return []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchQueued,
			interfaces.FactoryEventContext{EventTime: s.resumedAt, SessionID: &s.sessionID, DispatchID: &s.dispatchID, PhaseName: &s.phaseName},
			interfaces.DispatchQueuedEventPayload{
				DispatchKind: interfaces.FactoryDispatchKindJavaScriptTool, InputWorkIDs: &s.approvalWorkIDs,
				Label: &s.label, Model: &s.model, Provider: &s.provider, RunnerID: &s.runnerID,
				PromptDigest: &s.promptDigest, SchemaDigest: &s.schemaDigest, SkipPermissions: &s.skipPermissions,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeDispatchReconciled,
			interfaces.FactoryEventContext{EventTime: s.completedAt, SessionID: &s.sessionID, DispatchID: &s.dispatchID, PhaseName: &s.phaseName},
			interfaces.DispatchReconciledEventPayload{
				ArtifactIDs: &[]string{"artifact-result"}, ReconciledStatus: interfaces.FactoryDispatchStatusCompleted,
				ReconciliationSource: interfaces.DispatchReconciliationSourceStreamReplay, Replayed: true,
				Usage: &interfaces.FactoryDispatchUsage{
					CostUSD: &s.costUSD, DurationMillis: &s.durationMillis, InputTokens: &s.inputTokens,
					OutputTokens: &s.outputTokens, RetryCount: &s.retryCount, TotalTokens: &s.totalTokens,
				},
				FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureType("provider"), Message: "provider failed after partial output"},
			}),
	}
}

func (s incrementalProjectionScenario) sessionResultEvents(t *testing.T) []interfaces.FactoryEvent {
	t.Helper()
	return []interfaces.FactoryEvent{
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeHumanApprovalRequested,
			interfaces.FactoryEventContext{EventTime: s.completedAt, SessionID: &s.sessionID, DispatchID: &s.dispatchID,
				WorkIDs: &s.approvalWorkIDs, TraceIDs: &s.approvalTraceIDs},
			interfaces.HumanApprovalRequestedEventPayload{
				ApprovalID: "approval-1", WorkstationID: "approval-workstation",
				Decisions: []interfaces.HumanApprovalDecision{interfaces.HumanApprovalDecisionApprove, interfaces.HumanApprovalDecisionReject},
				Status:    interfaces.HumanApprovalStatusPending,
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionResultUpdated,
			interfaces.FactoryEventContext{EventTime: s.completedAt, SessionID: &s.sessionID},
			interfaces.FactorySessionResultUpdatedEventPayload{
				ArtifactIDs: []string{"artifact-result"}, ResultStatus: s.resultStatus,
				ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "partial result"}},
			}),
		canonicalWorldProjectionEvent(t, interfaces.FactoryEventTypeSessionCompleted,
			interfaces.FactoryEventContext{EventTime: s.completedAt, SessionID: &s.sessionID},
			interfaces.FactorySessionCompletedEventPayload{
				ArtifactIDs: []string{"artifact-result"}, CompletedAt: s.completedAt, DurationMillis: &s.durationMillis,
				DispatchCounts: &s.dispatchCounts, FailureDetail: &workerexecution.FailureDetail{Reason: workerexecution.WorkFailureType("provider"), Message: "partial result"},
				FinalStatus: interfaces.FactorySessionLifecycleStatusFailed, ResultStatus: &s.resultStatus,
			}),
	}
}

func assertIncrementalProjectionFacts(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	assertIncrementalApprovalFacts(t, projection)
	assertIncrementalBracketFacts(t, projection, s)
	assertIncrementalRuntimeFacts(t, projection, s)
}

func assertIncrementalApprovalFacts(t *testing.T, projection *IncrementalSessionProjection) {
	t.Helper()
	facts := projection.SnapshotSessionProjectionFacts()
	approval := facts.PendingHumanApprovals["approval-1"]
	if approval.WorkstationName != "Release Approval" || approval.WorkstationDescription == nil ||
		approval.WorkstationDescription.Values["fr-FR"] != "Approuver la version" || approval.WorkItemIDs[0] != "work-2" {
		t.Fatalf("pending approval facts = %#v, want resolved topology and correlation facts", approval)
	}
}

func assertIncrementalBracketFacts(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	facts := projection.SnapshotSessionProjectionFacts()
	if facts.SessionBracket == nil || facts.SessionBracket.LifecycleControlStatus != string(interfaces.FactorySessionLifecycleStatusRunning) ||
		!facts.SessionBracket.Terminal || facts.SessionBracket.FinalStatus != string(interfaces.FactorySessionLifecycleStatusFailed) ||
		facts.SessionBracket.DispatchCounts.Completed != s.dispatchCounts.Completed || facts.SessionBracket.FailureDetail == nil {
		t.Fatalf("session bracket facts = %#v, want terminal failed bracket with controls and counts", facts.SessionBracket)
	}
}

func assertIncrementalRuntimeFacts(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	facts := projection.SnapshotSessionProjectionFacts()
	if facts.JavaScriptRuntime == nil || facts.JavaScriptRuntime.Phase != s.phaseName || facts.JavaScriptRuntime.ArgsDigest != s.argsDigest ||
		len(facts.JavaScriptRuntime.Checkpoints) != 1 || len(facts.JavaScriptRuntime.Dispatches) != 1 || len(facts.JavaScriptRuntime.Artifacts) != 1 ||
		facts.JavaScriptRuntime.PrimaryResult[0].Text != "partial result" {
		t.Fatalf("JavaScript runtime facts = %#v, want detached runtime result", facts.JavaScriptRuntime)
	}
	dispatch := facts.JavaScriptRuntime.Dispatches[0]
	if dispatch.Status != string(interfaces.FactoryDispatchStatusCompleted) || dispatch.JavaScript == nil || !dispatch.JavaScript.SkipPermissions ||
		dispatch.Usage == nil || dispatch.Usage.TotalTokens != s.totalTokens || dispatch.FailureDetail == nil || dispatch.FailureDetail.Message == "" {
		t.Fatalf("dispatch facts = %#v, want reconciled usage/failure metadata", dispatch)
	}
}

func mutateIncrementalProjectionSnapshot(projection *IncrementalSessionProjection) {
	facts := projection.SnapshotSessionProjectionFacts()
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
}

func assertIncrementalProjectionSnapshotIsDetached(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	assertIncrementalApprovalSnapshotIsDetached(t, projection)
	assertIncrementalRuntimeSnapshotIsDetached(t, projection, s)
	assertIncrementalBracketSnapshotIsDetached(t, projection, s)
}

func assertIncrementalApprovalSnapshotIsDetached(t *testing.T, projection *IncrementalSessionProjection) {
	t.Helper()
	fresh := projection.SnapshotSessionProjectionFacts()
	approval := fresh.PendingHumanApprovals["approval-1"]
	if approval.Decisions[0] != interfaces.HumanApprovalDecisionApprove || approval.WorkstationDescription.Values["fr-FR"] != "Approuver la version" ||
		approval.WorkItemIDs[0] != "work-2" {
		t.Fatalf("SnapshotSessionProjectionFacts returned aliased state: %#v", fresh)
	}
}

func assertIncrementalRuntimeSnapshotIsDetached(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	fresh := projection.SnapshotSessionProjectionFacts()
	runtime := fresh.JavaScriptRuntime
	dispatch := runtime.Dispatches[0]
	if runtime.Phases[0] != "plan" || runtime.Checkpoints[0].Warnings[0].Code != "WARN" || runtime.Checkpoints[0].ArtifactRef.ID != "artifact-checkpoint" ||
		dispatch.RelatedWorkIDs[0] != "work-2" || dispatch.Usage.TotalTokens != s.totalTokens || dispatch.FailureDetail.Message == "MUTATED" ||
		runtime.Artifacts[0].RedactionCounts["secrets"] != int(s.redactedSecrets) || runtime.Artifacts[0].CaptureMetadata["mimeType"] != s.mimeType ||
		runtime.PrimaryResult[0].Text != "partial result" {
		t.Fatalf("SnapshotSessionProjectionFacts returned aliased runtime state: %#v", fresh)
	}
}

func assertIncrementalBracketSnapshotIsDetached(t *testing.T, projection *IncrementalSessionProjection, s incrementalProjectionScenario) {
	t.Helper()
	fresh := projection.SnapshotSessionProjectionFacts()
	bracket := fresh.SessionBracket
	if bracket.ResultSummary[0].Text != "partial result" || bracket.ArtifactIDs[0] != "artifact-result" ||
		bracket.DispatchCounts.Completed != s.dispatchCounts.Completed || bracket.FailureDetail.Message == "MUTATED" {
		t.Fatalf("SnapshotSessionProjectionFacts returned aliased bracket state: %#v", fresh)
	}
}

func assertIncrementalProjectionZeroValues(t *testing.T, events []interfaces.FactoryEvent) {
	t.Helper()
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
