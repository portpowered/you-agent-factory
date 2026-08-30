package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryEventHistory_RecordDispatchLifecycle_EmitsReconstructableQueueInterruptReconcileAndArtifactSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	recordDispatchLifecycleSequence(t, history, t0)

	events := generatedHistoryEvents(t, history)
	if len(events) != 4 {
		t.Fatalf("events = %d, want queued, interrupted, reconciled, artifact", len(events))
	}
	assertDispatchLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeDispatchQueued)
	assertDispatchLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeDispatchInterrupted)
	assertDispatchLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeDispatchReconciled)
	assertDispatchLifecycleEventType(t, events[3], factoryapi.FactoryEventTypeArtifactCreated)
	assertDispatchLifecycleOptionalMetadata(t, events)

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertDispatchLifecycleWorldState(t, worldState)
}

func TestFactoryEventHistory_RecordDispatchResultIgnoredIsRedactedAndIdempotent(t *testing.T) {
	t0 := time.Date(2026, 8, 29, 12, 32, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	input := recordings.DispatchResultIgnoredInput{
		SessionID:     "session-runtime",
		DispatchID:    "dispatch-late",
		Source:        "runtime",
		Tick:          7,
		WorkIDs:       []string{"work-terminal", "work-sibling"},
		Reason:        interfaces.DispatchResultIgnoredReasonWorkAlreadyTerminal,
		ResultOutcome: workerexecution.OutcomeFailed,
		ObservedState: interfaces.ObservedWorkState{Name: "complete", Type: interfaces.StateTypeTerminal},
	}
	history.RecordDispatchResultIgnored(input, t0)
	input.ResultOutcome = workerexecution.OutcomeAccepted
	input.ObservedState = interfaces.ObservedWorkState{Name: "failed", Type: interfaces.StateTypeFailed}
	history.RecordDispatchResultIgnored(input, t0.Add(time.Second))

	assertDispatchResultIgnoredEvent(t, generatedHistoryEvents(t, history))
}

func assertDispatchResultIgnoredEvent(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("canonical events = %#v, want one diagnostic after equivalent redelivery", events)
	}
	event := events[0]
	if event.Type != factoryapi.FactoryEventTypeDispatchResultIgnored {
		t.Fatalf("event type = %q, want DISPATCH_RESULT_IGNORED", event.Type)
	}
	if event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-late" {
		t.Fatalf("dispatch id = %#v, want dispatch-late", event.Context.DispatchId)
	}
	if event.Context.WorkIds == nil || len(*event.Context.WorkIds) != 1 || (*event.Context.WorkIds)[0] != "work-terminal" {
		t.Fatalf("work ids = %#v, want first terminal Work only", event.Context.WorkIds)
	}
	payload, err := event.Payload.AsDispatchResultIgnoredEventPayload()
	if err != nil {
		t.Fatalf("decode ignored payload: %v", err)
	}
	if string(payload.Reason) != interfaces.DispatchResultIgnoredReasonWorkAlreadyTerminal ||
		string(payload.ResultOutcome) != string(workerexecution.OutcomeFailed) ||
		payload.ObservedState.Name != "complete" || string(payload.ObservedState.Type) != string(interfaces.StateTypeTerminal) {
		t.Fatalf("ignored payload = %#v, want first redelivery facts", payload)
	}
	assertIgnoredEventPayloadIsRedacted(t, event)
}

func assertIgnoredEventPayloadIsRedacted(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal ignored event: %v", err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{"worker output", "worker error", "work-sibling"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("ignored event retained forbidden value %q: %s", forbidden, serialized)
		}
	}
}

func recordDispatchLifecycleSequence(
	t *testing.T,
	history *FactoryEventHistory,
	t0 time.Time,
) {
	t.Helper()
	queuedAt := t0.Add(2 * time.Second)
	interruptedAt := t0.Add(3 * time.Second)
	reconciledAt, artifactAt := t0.Add(4*time.Second), t0.Add(5*time.Second)
	kind := interfaces.OrchestratorKindJavaScript
	queuePosition := 0
	hash := "sha256:result-body"
	size := int64(512)
	label := "Review summary"
	summary := "Completed review findings"
	checkpointLabel := "provider handoff"
	inputTokens, outputTokens, totalTokens := int64(12), int64(8), int64(20)
	auditMode := "REDACTED"
	redactedSecrets := int32(2)

	history.RecordDispatchQueued(DispatchQueuedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             2,
		DispatchKind:     interfaces.FactoryDispatchKindJavaScriptAgent,
		Label:            "summarize findings",
		RunnerID:         "cursor",
		Model:            "gpt-4.1",
		Provider:         "openai",
		QueuePosition:    &queuePosition,
		PromptDigest:     "sha256:prompt",
		SchemaDigest:     "sha256:schema",
		InputWorkIDs:     []string{"work-1"},
		SkipPermissions:  true,
	}, queuedAt)
	history.RecordDispatchInterrupted(DispatchInterruptedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             2,
		Reason:           "provider disconnected",
		ObservedStatus:   interfaces.FactoryDispatchStatusFailed,
		RetryPlanned:     true,
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: "cursor", Kind: "session_id", ID: "provider-session-1",
		},
		CheckpointRef: &interfaces.FactorySessionJavaScriptCheckpointEventRef{
			ID: "checkpoint-1", Label: &checkpointLabel,
		},
	}, interruptedAt)
	history.RecordDispatchReconciled(DispatchReconciledInput{
		SessionID:            "session-js",
		OrchestratorKind:     kind,
		PhaseID:              "phase-execute",
		PhaseName:            "execute",
		DispatchID:           "dispatch-js-1",
		Source:               "replay",
		Tick:                 3,
		ReconciledStatus:     interfaces.FactoryDispatchStatusCompleted,
		ReconciliationSource: interfaces.DispatchReconciliationSourceProviderSession,
		Replayed:             true,
		ArtifactIDs:          []string{"artifact-result-1"},
		Usage: &interfaces.FactoryDispatchUsage{
			InputTokens: &inputTokens, OutputTokens: &outputTokens, TotalTokens: &totalTokens,
		},
		ResultArtifactRef: &interfaces.FactoryArtifactRef{
			ID: "artifact-result-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC", ContentHash: &hash, SizeBytes: &size,
		},
	}, reconciledAt)
	history.RecordArtifactCreated(ArtifactCreatedInput{
		SessionID:        "session-js",
		OrchestratorKind: kind,
		PhaseID:          "phase-execute",
		PhaseName:        "execute",
		DispatchID:       "dispatch-js-1",
		Source:           "runtime",
		Tick:             3,
		Artifact: interfaces.FactoryArtifact{
			ID:          "artifact-result-1",
			Kind:        "FINAL_RESULT",
			Visibility:  "PUBLIC",
			Label:       &label,
			Summary:     &summary,
			ContentHash: &hash,
			SizeBytes:   &size,
			AuditMode:   &auditMode,
			RedactionCounts: &interfaces.FactoryArtifactRedactionCounts{
				Secrets: &redactedSecrets,
			},
		},
		CapturedAt: &artifactAt,
	}, artifactAt)
}

func assertDispatchLifecycleOptionalMetadata(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	assertQueuedOptionalMetadata(t, events[0])
	assertInterruptedOptionalMetadata(t, events[1])
	assertReconciledOptionalMetadata(t, events[2])
	assertArtifactOptionalMetadata(t, events[3])
}

func assertQueuedOptionalMetadata(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	queued, err := event.Payload.AsDispatchQueuedEventPayload()
	if err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	if queued.SkipPermissions == nil || !*queued.SkipPermissions {
		t.Fatalf("queued skipPermissions = %#v, want true", queued.SkipPermissions)
	}
}

func assertInterruptedOptionalMetadata(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	interrupted, err := event.Payload.AsDispatchInterruptedEventPayload()
	if err != nil {
		t.Fatalf("decode interrupted payload: %v", err)
	}
	if interrupted.ProviderSessionRef == nil || interrupted.ProviderSessionRef.Id != "provider-session-1" {
		t.Fatalf("provider session ref = %#v, want provider-session-1", interrupted.ProviderSessionRef)
	}
	if interrupted.CheckpointRef == nil || interrupted.CheckpointRef.Id != "checkpoint-1" {
		t.Fatalf("checkpoint ref = %#v, want checkpoint-1", interrupted.CheckpointRef)
	}
}

func assertReconciledOptionalMetadata(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	reconciled, err := event.Payload.AsDispatchReconciledEventPayload()
	if err != nil {
		t.Fatalf("decode reconciled payload: %v", err)
	}
	if reconciled.Usage == nil || reconciled.Usage.TotalTokens == nil || *reconciled.Usage.TotalTokens != 20 {
		t.Fatalf("reconciled usage = %#v, want totalTokens=20", reconciled.Usage)
	}
	if reconciled.ResultArtifactRef == nil || reconciled.ResultArtifactRef.Id != "artifact-result-1" {
		t.Fatalf("result artifact ref = %#v, want artifact-result-1", reconciled.ResultArtifactRef)
	}
}

func assertArtifactOptionalMetadata(t *testing.T, event factoryapi.FactoryEvent) {
	t.Helper()
	created, err := event.Payload.AsArtifactCreatedEventPayload()
	if err != nil {
		t.Fatalf("decode artifact payload: %v", err)
	}
	if created.Artifact.AuditMode == nil || *created.Artifact.AuditMode != factoryapi.FactoryArtifactAuditModeREDACTED {
		t.Fatalf("artifact audit mode = %#v, want REDACTED", created.Artifact.AuditMode)
	}
	if created.Artifact.RedactionCounts == nil || created.Artifact.RedactionCounts.Secrets == nil || *created.Artifact.RedactionCounts.Secrets != 2 {
		t.Fatalf("artifact redaction counts = %#v, want secrets=2", created.Artifact.RedactionCounts)
	}
}

func assertDispatchLifecycleWorldState(
	t *testing.T,
	worldState interfaces.FactoryWorldState,
) {
	t.Helper()
	if worldState.JavaScriptRuntime == nil {
		t.Fatal("javascript runtime = nil, want dispatch lifecycle projection")
	}
	if worldState.JavaScriptRuntime.QueuedDispatches != 0 || worldState.JavaScriptRuntime.CompletedDispatches != 1 {
		t.Fatalf("dispatch counts = queued:%d completed:%d, want queued=0 completed=1",
			worldState.JavaScriptRuntime.QueuedDispatches, worldState.JavaScriptRuntime.CompletedDispatches)
	}
	if len(worldState.JavaScriptRuntime.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one reconciled dispatch", worldState.JavaScriptRuntime.Dispatches)
	}
	dispatch := worldState.JavaScriptRuntime.Dispatches[0]
	if dispatch.ID != "dispatch-js-1" || dispatch.Status != string(factoryapi.FactoryDispatchStatusCOMPLETED) {
		t.Fatalf("dispatch = %#v, want dispatch-js-1 COMPLETED", dispatch)
	}
	if dispatch.PromptDigest != "sha256:prompt" || dispatch.Label != "summarize findings" {
		t.Fatalf("dispatch metadata = %#v, want prompt digest and label preserved", dispatch)
	}
	if dispatch.JavaScript == nil || !dispatch.JavaScript.SkipPermissions {
		t.Fatalf("javascript dispatch = %#v, want skipPermissions=true", dispatch.JavaScript)
	}
	if len(worldState.Artifacts) != 1 || worldState.Artifacts[0].ID != "artifact-result-1" {
		t.Fatalf("artifacts = %#v, want artifact-result-1", worldState.Artifacts)
	}
}

func assertDispatchLifecycleEventType(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantType factoryapi.FactoryEventType,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.Context.SessionId == nil || *event.Context.SessionId != "session-js" {
		t.Fatalf("session id = %#v, want session-js", event.Context.SessionId)
	}
	if event.Context.DispatchId == nil || *event.Context.DispatchId != "dispatch-js-1" {
		t.Fatalf("dispatch id = %#v, want dispatch-js-1 for %s", event.Context.DispatchId, wantType)
	}
}

func TestFailureDetailsForResult_NonFailedResultsOmitFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:   "dispatch-rejected",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeRejected,
		Feedback:     "needs revision",
	})

	if reason != "" || message != "" {
		t.Fatalf("failure details = %q/%q, want empty for rejected result", reason, message)
	}
}

func TestFailureDetailsForResult_FailedWorkerErrorUsesStableFailureDetails(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:   "dispatch-worker-error",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "script exited with code 1",
	})

	if reason != failureReasonWorkerError {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonWorkerError)
	}
	if message != "script exited with code 1" {
		t.Fatalf("failure message = %q, want script error", message)
	}
}

func TestFailureDetailsForResult_FailureMetadataOverridesWorkerErrorReason(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:      "dispatch-timeout",
		TransitionID:    "build",
		Outcome:         workerexecution.OutcomeFailed,
		Error:           "provider error: timeout: context deadline exceeded",
		FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeTimeout},
	})

	if reason != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("failure reason = %q, want %q", reason, workerexecution.WorkFailureTypeTimeout)
	}
	if message != "provider error: timeout: context deadline exceeded" {
		t.Fatalf("failure message = %q, want preserved rendered timeout text", message)
	}
}

func TestFailureDetailsForResult_TypedDetailOverridesFallbacks(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:      "dispatch-script-failure",
		TransitionID:    "build",
		Outcome:         workerexecution.OutcomeFailed,
		Error:           "script exited with status 1",
		FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeUnknown},
		FailureDetail: &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureTypeInternalServerError,
			Message: "script exited with status 1: repository root is dirty",
		},
	})

	if reason != string(workerexecution.WorkFailureTypeInternalServerError) {
		t.Fatalf("failure reason = %q, want internal server error", reason)
	}
	if message != "script exited with status 1: repository root is dirty" {
		t.Fatalf("failure message = %q, want typed diagnostic", message)
	}
}

func TestFailureDetailsForResult_ClassifierInvalidOutputPreservesRawOutputEvidence(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:   "dispatch-classifier-invalid",
		TransitionID: "classify",
		Outcome:      workerexecution.OutcomeFailed,
		Error:        `classifier output invalid: expected plain string label (raw output: "{\"label\":\"approved\"}")`,
	})

	if reason != failureReasonWorkerError {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonWorkerError)
	}
	if !strings.Contains(message, `raw output: "{\"label\":\"approved\"}"`) {
		t.Fatalf("failure message = %q, want raw output evidence", message)
	}
}

func TestFailureDetailsForResult_FailedWithoutDetailsUsesUnavailableMessage(t *testing.T) {
	reason, message := failureDetailsForResult(workerexecution.WorkResult{
		DispatchID:   "dispatch-unknown",
		TransitionID: "build",
		Outcome:      workerexecution.OutcomeFailed,
	})

	if reason != failureReasonUnknown {
		t.Fatalf("failure reason = %q, want %q", reason, failureReasonUnknown)
	}
	if message != failureMessageUnavailable {
		t.Fatalf("failure message = %q, want unavailable message", message)
	}
}
