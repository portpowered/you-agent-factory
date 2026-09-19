package recordingreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testpath"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestReplayRecordingRestoresCompletedPublicReadModelsWithoutLiveExecution(t *testing.T) {
	t.Parallel()
	value := buildTerminalRecording(t, "SUCCEEDED", &recording.PortableRecordingCanonicalResult{
		Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-result"},
	})

	got, err := ReplayRecording(value)
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	if got.Session.Status != fse.LifecycleStatusSucceeded || got.Session.ResultSummary == nil || got.Session.ResultSummary.ResultStatus != "FINAL" {
		t.Fatalf("session projection = %#v", got.Session)
	}
	if got.Result.ResultStatus != fse.ResultStatusFinal || string(got.Result.PrimaryResult) != `{"answer":"done"}` {
		t.Fatalf("result projection = %#v", got.Result)
	}
	if got.Redaction != value.Redaction {
		t.Fatalf("redaction projection = %#v, want %#v", got.Redaction, value.Redaction)
	}
	assertRecordedInspectionParity(t, value, got)
}

func TestReplayRecordingRestoresFailedPartialReadModelsWithoutManufacturingSuccess(t *testing.T) {
	t.Parallel()
	value := buildTerminalRecording(t, "FAILED", &recording.PortableRecordingCanonicalResult{
		Status: "FAILED_WITH_PARTIAL", Mode: "partial", PrimaryResult: json.RawMessage(`{"partial":true}`),
		ArtifactIDs: []string{"artifact-result"},
		Failure:     &recording.PortableRecordingFailureSummary{Reason: "WORKFLOW_FAILED", Message: "safe failure", PartialResultAvailable: true},
	})

	got, err := ReplayRecording(value)
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	if got.Session.Status != fse.LifecycleStatusFailed || got.Session.Failure == nil || !got.Session.Failure.PartialResultAvailable {
		t.Fatalf("failed session projection = %#v", got.Session)
	}
	if got.Result.ResultStatus != fse.ResultStatusFailedWithPartial || got.Result.Failure == nil || got.Result.Failure.Reason != "WORKFLOW_FAILED" {
		t.Fatalf("failed result projection = %#v", got.Result)
	}
	assertRecordedInspectionParity(t, value, got)
}

func TestReplayRecordingRejectsHashInconsistentResultWithoutPartialProjection(t *testing.T) {
	t.Parallel()
	value := buildTerminalRecording(t, "SUCCEEDED", &recording.PortableRecordingCanonicalResult{
		Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-result"},
	})
	value.Result.PrimaryResult = json.RawMessage(`{"answer":"tampered"}`)

	got, err := ReplayRecording(value)
	var diagnostic *recording.PortableRecordingDiagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Code != recording.PortableRecordingCodeInvalidDigest || diagnostic.Path != "result.contentHash" {
		t.Fatalf("ReplayRecording error = %#v", err)
	}
	if !reflect.DeepEqual(got, RecordingReplayProjection{}) {
		t.Fatalf("untrusted partial projection = %#v", got)
	}
}

func TestReplayRecordingRestoresPausedCheckpointWithoutLiveControls(t *testing.T) {
	t.Parallel()
	value := buildLifecycleRecording(t, "PAUSED", false)

	got, err := ReplayRecording(value)
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	if got.Session.Status != fse.LifecycleStatusPaused || got.Session.Lifecycle == nil || got.Session.Lifecycle.PausedAt == nil {
		t.Fatalf("paused session projection = %#v", got.Session)
	}
	if got.Checkpoint == nil || got.Checkpoint.ID != "checkpoint-public-1" || got.Checkpoint.Summary != "Waiting for operator input" {
		t.Fatalf("checkpoint projection = %#v", got.Checkpoint)
	}
	if outcome := got.ApplyLifecycleControl(fse.LifecycleControlResume); outcome.Outcome != "NON_LIVE_REPLAY" {
		t.Fatalf("replay control outcome = %#v", outcome)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal replay projection: %v", err)
	}
	for _, prohibited := range []string{"checkpointState", "completedDispatchIds", "pendingDispatchIds", "dispatch-secret"} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("replay projection leaked %q: %s", prohibited, encoded)
		}
	}
	assertRecordedInspectionParity(t, value, got)
}

func TestReplayRecordingRestoresResumedHistoryAndFinalAvailability(t *testing.T) {
	t.Parallel()
	value := buildLifecycleRecording(t, "SUCCEEDED", true)

	got, err := ReplayRecording(value)
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	if got.Session.Status != fse.LifecycleStatusSucceeded || got.Session.Lifecycle == nil || got.Session.Lifecycle.PausedAt == nil || got.Session.Lifecycle.ResumedAt == nil {
		t.Fatalf("resumed session projection = %#v", got.Session)
	}
	if !got.Session.Lifecycle.ResumedAt.After(*got.Session.Lifecycle.PausedAt) || got.Result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("resumed lifecycle/result = %#v %#v", got.Session.Lifecycle, got.Result)
	}
	assertRecordedInspectionParity(t, value, got)
}

func TestReplayRecordingPreservesLegacyFactsAndReportsWorkerHistoryUnavailable(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"valid-v1.json", "valid-v2.json", "valid-v2-checkpoint.json"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertLegacyReplayFixture(t, name)
		})
	}
}

func TestReplayRecordingPreservesCurrentWorkerHistoryFacts(t *testing.T) {
	t.Parallel()
	value := loadVersionPinnedRecordingFixture(t, "valid-v3-worker-history.json")
	first := replayVersionPinnedFixture(t, value)
	second := replayVersionPinnedFixture(t, value)
	assertStableWorkerHistory(t, first, second)
	history := first.WorkerHistory
	if history.Availability != recording.PortableRecordingWorkerHistoryAvailable ||
		history.WorkerPortableRecording == nil || len(history.Records) != 3 {
		t.Fatalf("Worker history = %#v, want available ordered history", history)
	}
	if history.Lifecycle.Terminal == nil || history.Lifecycle.Terminal.Status != "COMPLETED" ||
		history.Correlation.FactorySessionID != value.Session.ID || history.Correlation.DispatchID != "dispatch-current-001" {
		t.Fatalf("Worker lifecycle/correlation = %#v", history)
	}
	if history.Records[1].Provenance.Fidelity != workers.FidelityNormalized ||
		history.Records[1].Provenance.Delivery != workers.DeliveryNativeStream {
		t.Fatalf("Worker fidelity facts = %#v", history.Records[1])
	}
	if first.Session.SessionID != value.Session.ID || len(first.Events.Events) != len(value.Events) ||
		first.Result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("Factory Session projection = %#v", first)
	}
	assertInspectionWorkerHistory(t, first)
}

func TestReplayLegacyRecordingMapsCanonicalFailureAndLaterEventFacts(t *testing.T) {
	t.Parallel()

	workID := `work-"failed`
	item := work.FactoryWorkItem{
		ID: workID, WorkTypeID: "review", State: "failed", TraceID: "trace-failed",
		CurrentChainingTraceID: "chain-current", PreviousChainingTraceIDs: []string{"chain-previous"},
		ParentID: "parent-1",
	}
	state := recording.FactoryWorldState{
		FactoryState: "FAILED",
		WorkRequestsByID: map[string]factorydefinitions.WorkRequestPayload{
			"request-1": {
				RequestID: "request-1", Type: work.WorkRequestTypeFactoryRequestBatch,
				WorkItems: []work.FactoryWorkItem{item},
			},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{workID: item},
		FailedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-failed", TransitionID: "review-transition", WorkItemIDs: []string{workID},
			Result: factorydefinitions.WorkstationResult{
				Outcome: string(workers.OutcomeFailed), Error: "{\"decision\":\"not-a-live-envelope\"}",
			},
		}},
		SessionBracket: &factorydefinitions.FactoryWorldSessionBracketState{
			SessionID: "legacy-session", SourceRef: "workflow/legacy.js", LifecycleControlStatus: "FAILED",
			ResultStatus: string(factorydefinitions.FactorySessionResultStatusFailedWithPartial),
			Terminal:     true, FinalStatus: string(factorydefinitions.FactorySessionLifecycleStatusFailed),
		},
	}
	eventTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	value := recording.ReplayArtifact{
		SchemaVersion: "legacy", Events: []recording.FactoryEvent{
			{
				SchemaVersion: recording.FactoryEventSchemaVersionV1,
				Id:            "event-later", Type: recording.FactoryEventTypeFactoryStateResponse,
				Context: recording.FactoryEventContext{Tick: 4, Sequence: 0, EventTime: eventTime},
				Payload: json.RawMessage(`{"state":"FAILED"}`),
			},
		},
	}

	got, err := ReplayLegacyRecording(value, "requested-session", state)
	if err != nil {
		t.Fatalf("ReplayLegacyRecording: %v", err)
	}
	assertLegacyFailureProjection(t, got)
	assertLegacyLaterEvent(t, got)
	assertLegacyFailureDetail(t, got, workID)
}

func assertLegacyFailureProjection(t *testing.T, got RecordingReplayProjection) {
	t.Helper()
	if got.FactoryProjection == nil || got.FactoryProjection.SessionBracket == nil ||
		got.Session.SessionID != "legacy-session" ||
		got.Session.ResolvedSource.SourceRef != "workflow/legacy.js" ||
		got.Session.Status != fse.LifecycleStatusFailed ||
		got.Result.ResultStatus != fse.ResultStatusFailedWithPartial ||
		got.Result.Failure == nil || got.Result.Failure.Reason != string(workers.WorkFailureTypeUnknown) {
		t.Fatalf("legacy session/result projection = %#v, want canonical terminal failure facts", got)
	}
}

func assertLegacyLaterEvent(t *testing.T, got RecordingReplayProjection) {
	t.Helper()
	if len(got.Events.Events) != 1 || !strings.Contains(string(got.Events.Events[0]), `"event-later"`) {
		t.Fatalf("legacy event projection = %#v, want later event identity", got.Events)
	}
}

func assertLegacyFailureDetail(t *testing.T, got RecordingReplayProjection, workID string) {
	t.Helper()
	detail, ok := got.FactoryProjection.FailureDetailsByWorkID[workID]
	if !ok || detail.DispatchID != "dispatch-failed" || detail.TransitionID != "review-transition" ||
		detail.FailureDetail == nil || detail.FailureDetail.Message != "{\"decision\":\"not-a-live-envelope\"}" {
		t.Fatalf("legacy failure detail = %#v, want preserved safe recorded message and dispatch identity", detail)
	}
}

func assertLegacyReplayFixture(t *testing.T, name string) {
	t.Helper()
	value := loadVersionPinnedRecordingFixture(t, name)
	first := replayVersionPinnedFixture(t, value)
	second := replayVersionPinnedFixture(t, value)
	assertStableWorkerHistory(t, first, second)
	assertLegacyWorkerHistory(t, first)
	assertLegacyFactorySessionFacts(t, value, first)
	assertLegacyResultAbsence(t, value, first)
	assertInspectionWorkerHistory(t, first)
}

func replayVersionPinnedFixture(t *testing.T, value recording.PortableRecording) RecordingReplayProjection {
	t.Helper()
	projection, err := ReplayRecording(value)
	if err != nil {
		t.Fatalf("ReplayRecording() error = %v", err)
	}
	return projection
}

func assertStableWorkerHistory(t *testing.T, first, second RecordingReplayProjection) {
	t.Helper()
	if !reflect.DeepEqual(first.WorkerHistory, second.WorkerHistory) {
		t.Fatalf("Worker history changed across replay: first=%#v second=%#v", first.WorkerHistory, second.WorkerHistory)
	}
}

func assertLegacyWorkerHistory(t *testing.T, projection RecordingReplayProjection) {
	t.Helper()
	if projection.WorkerHistory.Availability != recording.PortableRecordingWorkerHistoryUnavailable ||
		projection.WorkerHistory.Reason != recording.PortableRecordingWorkerHistoryReasonLegacySchema {
		t.Fatalf("Worker history = %#v, want unavailable legacy outcome", projection.WorkerHistory)
	}
}

func assertLegacyFactorySessionFacts(t *testing.T, value recording.PortableRecording, projection RecordingReplayProjection) {
	t.Helper()
	if projection.Session.SessionID != value.Session.ID ||
		projection.Session.ResolvedSource.SourceRef != value.Source.Ref ||
		len(projection.Events.Events) != len(value.Events) ||
		len(projection.Artifacts.Artifacts) != len(value.Artifacts) {
		t.Fatalf("legacy Factory Session facts were not preserved: %#v", projection)
	}
}

func assertLegacyResultAbsence(t *testing.T, value recording.PortableRecording, projection RecordingReplayProjection) {
	t.Helper()
	if value.Result != nil {
		return
	}
	if projection.Session.ResultSummary != nil || projection.Result.ResultStatus != "" ||
		projection.Result.Availability != nil || projection.Result.Failure != nil {
		t.Fatalf("schema-1 fabricated result facts: session=%#v result=%#v", projection.Session, projection.Result)
	}
}

func assertInspectionWorkerHistory(t *testing.T, projection RecordingReplayProjection) {
	t.Helper()
	inspection := NewService(projection).Inspection()
	if !reflect.DeepEqual(inspection.WorkerHistory, projection.WorkerHistory) {
		t.Fatalf("inspection Worker history = %#v, want %#v", inspection.WorkerHistory, projection.WorkerHistory)
	}
	if inspection.FactoryProjection.Availability != factorysessions.HistoricalReplayFactoryProjectionUnavailable ||
		inspection.FactoryProjection.Reason != factorysessions.HistoricalReplayFactoryProjectionReasonNotRecorded ||
		inspection.FactoryProjection.State != nil {
		t.Fatalf("portable Factory projection = %#v, want explicit unavailable state", inspection.FactoryProjection)
	}
}

func loadVersionPinnedRecordingFixture(t *testing.T, name string) recording.PortableRecording {
	t.Helper()
	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", name,
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	value, err := recording.DecodePortableRecording(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode fixture %q: %v", name, err)
	}
	return value
}

func buildLifecycleRecording(t *testing.T, status string, resumed bool) recording.PortableRecording {
	t.Helper()
	checkpointAt := time.Date(2026, 7, 12, 19, 0, 2, 0, time.UTC)
	events := []json.RawMessage{
		json.RawMessage(`{"id":"event-started","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-07-12T19:00:00Z"},"payload":{}}`),
		json.RawMessage(`{"id":"event-checkpoint","type":"JAVASCRIPT_CHECKPOINT_REF","context":{"sequence":1,"eventTime":"2026-07-12T19:00:02Z","checkpointId":"checkpoint-public-1"},"payload":{"artifactIds":["artifact-checkpoint"]}}`),
		json.RawMessage(`{"id":"event-paused","type":"SESSION_PAUSED","context":{"sequence":2,"eventTime":"2026-07-12T19:00:03Z"},"payload":{}}`),
	}
	result := &recording.PortableRecordingCanonicalResult{Status: "PARTIAL", Mode: "partial", PrimaryResult: json.RawMessage(`{"step":1}`), ArtifactIDs: []string{"artifact-checkpoint"}}
	if resumed {
		events = append(events,
			json.RawMessage(`{"id":"event-resumed","type":"SESSION_RESUMED","context":{"sequence":3,"eventTime":"2026-07-12T19:00:04Z"},"payload":{}}`),
			json.RawMessage(`{"id":"event-completed","type":"SESSION_COMPLETED","context":{"sequence":4,"eventTime":"2026-07-12T19:00:05Z"},"payload":{"artifactIds":["artifact-checkpoint"]}}`),
		)
		result = &recording.PortableRecordingCanonicalResult{Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"step":2}`), ArtifactIDs: []string{"artifact-checkpoint"}}
	}
	value, err := recording.BuildPortableRecording(recording.PortableRecordingCanonicalFacts{
		SessionID: "dur-sess-lifecycle-recording", Status: status, OrchestratorKind: "JAVASCRIPT",
		SourceRef: "workflow/lifecycle.js", SourceHash: recordingTestDigest('4'), PolicyHash: recordingTestDigest('5'),
		Artifacts: []recording.PortableRecordingCanonicalArtifact{{ID: "artifact-checkpoint", Kind: "CHECKPOINT", Visibility: "PUBLIC", ContentHash: recordingTestDigest('6'), SizeBytes: 12, CreatedAt: checkpointAt}},
		Events:    events, Result: result,
		Checkpoint: &recording.PortableRecordingCanonicalCheckpoint{ID: "checkpoint-public-1", Label: "Approval", Summary: "Waiting for operator input", Timestamp: checkpointAt, ArtifactID: "artifact-checkpoint"},
	})
	if err != nil {
		t.Fatalf("Build lifecycle recording: %v", err)
	}
	return value
}

func buildTerminalRecording(t *testing.T, status string, result *recording.PortableRecordingCanonicalResult) recording.PortableRecording {
	t.Helper()
	createdAt := time.Date(2026, 7, 12, 18, 0, 1, 0, time.UTC)
	value, err := recording.BuildPortableRecording(recording.PortableRecordingCanonicalFacts{
		SessionID: "dur-sess-recording-replay-terminal", Status: status, OrchestratorKind: "JAVASCRIPT",
		SourceRef: "workflow/terminal.js", SourceHash: recordingTestDigest('1'), PolicyHash: recordingTestDigest('2'),
		Artifacts: []recording.PortableRecordingCanonicalArtifact{{ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC", Label: "Result", ContentHash: recordingTestDigest('3'), SizeBytes: 21, CreatedAt: createdAt}},
		Events: []json.RawMessage{
			json.RawMessage(`{"id":"event-started","type":"SESSION_STARTED","context":{"sequence":0,"eventTime":"2026-07-12T18:00:00Z"},"payload":{}}`),
			json.RawMessage(`{"id":"event-terminal","type":"SESSION_COMPLETED","context":{"sequence":1,"eventTime":"2026-07-12T18:00:02Z"},"payload":{"artifactIds":["artifact-result"]}}`),
		},
		Result: result,
	})
	if err != nil {
		t.Fatalf("Build recording: %v", err)
	}
	return value
}

func assertRecordedInspectionParity(t *testing.T, value recording.PortableRecording, got RecordingReplayProjection) {
	t.Helper()
	if got.Artifacts.SessionID != value.Session.ID || len(got.Artifacts.Artifacts) != len(value.Artifacts) {
		t.Fatalf("artifact projection = %#v", got.Artifacts)
	}
	if got.Artifacts.Artifacts[0].ID != value.Artifacts[0].ID || got.Artifacts.Artifacts[0].ContentHash != value.Artifacts[0].ContentHash {
		t.Fatalf("artifact summary mismatch: got=%#v want=%#v", got.Artifacts.Artifacts[0], value.Artifacts[0])
	}
	if got.Events.SessionID != value.Session.ID || len(got.Events.Events) != len(value.Events) {
		t.Fatalf("event projection = %#v", got.Events)
	}
	for index, raw := range got.Events.Events {
		var event struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Context struct {
				Sequence int64 `json:"sequence"`
			} `json:"context"`
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("decode replay event: %v", err)
		}
		if event.ID != value.Events[index].ID || event.Type != value.Events[index].Type || event.Context.Sequence != value.Events[index].Sequence {
			t.Fatalf("event summary mismatch: got=%#v want=%#v", event, value.Events[index])
		}
	}
}

func recordingTestDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func TestReplayLegacyRecordingSelectsSessionIdentityFallbacks(t *testing.T) {
	t.Parallel()

	eventSessionID := "session-from-event"
	tests := []struct {
		name      string
		requested string
		state     recording.FactoryWorldState
		value     recording.ReplayArtifact
		want      string
	}{
		{
			name:      "session bracket identity wins",
			requested: "requested-session",
			state: recording.FactoryWorldState{
				FactoryState:   "RUNNING",
				SessionBracket: &factorydefinitions.FactoryWorldSessionBracketState{SessionID: "session-from-bracket"},
			},
			value: recording.ReplayArtifact{Events: []recording.FactoryEvent{
				legacyReplayTestEvent("event-1", recording.FactoryEventTypeSessionStarted, eventSessionID, time.Time{}, json.RawMessage(`{}`)),
			}},
			want: "session-from-bracket",
		},
		{
			name:      "event context identity is used when bracket is absent",
			requested: "requested-session",
			state:     recording.FactoryWorldState{FactoryState: "RUNNING"},
			value: recording.ReplayArtifact{Events: []recording.FactoryEvent{
				legacyReplayTestEvent("event-1", recording.FactoryEventTypeSessionStarted, eventSessionID, time.Time{}, json.RawMessage(`{}`)),
			}},
			want: "session-from-event",
		},
		{
			name:      "requested identity is used without recorded identity",
			requested: "requested-session",
			state:     recording.FactoryWorldState{FactoryState: "RUNNING"},
			value:     recording.ReplayArtifact{},
			want:      "requested-session",
		},
		{
			name:  "default identity is used when all sources are blank",
			state: recording.FactoryWorldState{FactoryState: "RUNNING"},
			value: recording.ReplayArtifact{},
			want:  "~default",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReplayLegacyRecording(test.value, test.requested, test.state)
			if err != nil {
				t.Fatalf("ReplayLegacyRecording: %v", err)
			}
			if got.Session.SessionID != test.want {
				t.Fatalf("session ID = %q, want %q", got.Session.SessionID, test.want)
			}
		})
	}
}

func TestReplayLegacyRecordingProjectsSnapshotLifecycleAndArtifacts(t *testing.T) {
	t.Parallel()

	sessionID := "session-from-context"
	startedAt := time.Date(2026, 9, 15, 8, 0, 0, 0, time.FixedZone("recorded-zone", 2*60*60))
	pausedAt := startedAt.Add(10 * time.Minute)
	resumedAt := startedAt.Add(20 * time.Minute)
	completedAt := startedAt.Add(30 * time.Minute)
	capturedAt := startedAt.Add(5 * time.Minute)
	startedPayload, err := json.Marshal(factorydefinitions.FactorySessionStartedEventPayload{
		StartedAt:  startedAt,
		SourceRef:  legacyReplayStringPointer("event/source.js"),
		SourceHash: legacyReplayStringPointer("event-source-hash"),
	})
	if err != nil {
		t.Fatalf("marshal start payload: %v", err)
	}
	snapshot := factorydefinitions.FactorySnapshot([]byte(`{"orchestrator":{"kind":"JAVASCRIPT","javascript":{"dialect":"typescript","sourceRef":"snapshot/source.js","sourceHash":"snapshot-source-hash"}}}`))
	value := recording.ReplayArtifact{
		SchemaVersion: "legacy",
		Factory:       &snapshot,
		Events: []recording.FactoryEvent{
			legacyReplayTestEvent("event-started", recording.FactoryEventTypeSessionStarted, sessionID, startedAt.Add(time.Hour), startedPayload),
			legacyReplayTestEvent("event-paused", recording.FactoryEventTypeSessionPaused, sessionID, pausedAt.Add(time.Hour), json.RawMessage(`{"pausedAt":"2026-09-15T08:10:00+02:00","status":"PAUSED"}`)),
			legacyReplayTestEvent("event-resumed", recording.FactoryEventTypeSessionResumed, sessionID, resumedAt.Add(time.Hour), json.RawMessage(`{"resumedAt":"2026-09-15T08:20:00+02:00","status":"RUNNING"}`)),
			legacyReplayTestEvent("event-completed", recording.FactoryEventTypeSessionCompleted, sessionID, completedAt.Add(time.Hour), json.RawMessage(`{"completedAt":"2026-09-15T08:30:00+02:00","finalStatus":"SUCCEEDED","resultStatus":"FINAL"}`)),
		},
	}
	state := recording.FactoryWorldState{
		Artifacts: []factorydefinitions.FactorySessionArtifactState{
			{
				ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC", Label: "Run result",
				Summary: "Recorded result", AuditMode: "strict", ContentHash: "sha256:result", SizeBytes: 17,
				CapturedAt: capturedAt, RedactionCounts: map[string]int{"paths": 1, "secrets": 2, "tokens": 3},
				CaptureMetadata: map[string]string{"sourceDispatchId": "dispatch-result"},
			},
			{ID: "artifact-checkpoint", Kind: "CHECKPOINT", Visibility: "INTERNAL"},
		},
	}

	got, err := ReplayLegacyRecording(value, "requested-session", state)
	if err != nil {
		t.Fatalf("ReplayLegacyRecording: %v", err)
	}
	if got.Session.SessionID != sessionID || got.Session.Status != fse.LifecycleStatusSucceeded ||
		got.Session.ResolvedSource.SourceRef != "snapshot/source.js" ||
		got.Session.ResolvedSource.SourceHash != "snapshot-source-hash" ||
		got.Session.OrchestratorKind != "JAVASCRIPT" || got.Session.Dialect != "typescript" {
		t.Fatalf("historical session projection = %#v, want snapshot and lifecycle facts", got.Session)
	}
	if got.Result.ResultStatus != fse.ResultStatusFinal || got.FactoryProjection == nil {
		t.Fatalf("result/projection = %#v / %#v, want final result and canonical Factory state", got.Result, got.FactoryProjection)
	}
	lifecycle := got.Session.Lifecycle
	if lifecycle == nil || lifecycle.StartedAt == nil || !lifecycle.StartedAt.Equal(startedAt.UTC()) ||
		lifecycle.PausedAt == nil || !lifecycle.PausedAt.Equal(pausedAt.UTC()) ||
		lifecycle.ResumedAt == nil || !lifecycle.ResumedAt.Equal(resumedAt.UTC()) ||
		lifecycle.FinishedAt == nil || !lifecycle.FinishedAt.Equal(completedAt.UTC()) {
		t.Fatalf("lifecycle timestamps = %#v, want UTC event payload timestamps", lifecycle)
	}
	if len(got.Artifacts.Artifacts) != 2 || got.Artifacts.Artifacts[0].ID != "artifact-result" ||
		got.Artifacts.Artifacts[0].CreatedAt == nil || !got.Artifacts.Artifacts[0].CreatedAt.Equal(capturedAt.UTC()) ||
		got.Artifacts.Artifacts[0].DispatchID != "dispatch-result" ||
		got.Artifacts.Artifacts[1].CreatedAt != nil || got.Artifacts.Artifacts[1].RedactionCounts != nil {
		t.Fatalf("artifact projection = %#v, want recorded metadata and absent optional fields", got.Artifacts)
	}
	if got.Redaction.SecretsRedacted != 2 ||
		got.WorkerHistory.Availability != recording.PortableRecordingWorkerHistoryUnavailable {
		t.Fatalf("legacy availability/redaction = %#v / %#v", got.WorkerHistory, got.Redaction)
	}
}

func TestReplayLegacyRecordingProjectsBracketResultAndArtifactReferences(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	pausedAt := startedAt.Add(time.Minute)
	resumedAt := startedAt.Add(2 * time.Minute)
	completedAt := startedAt.Add(3 * time.Minute)
	value := recording.ReplayArtifact{SchemaVersion: "legacy"}
	state := recording.FactoryWorldState{
		SessionBracket: &factorydefinitions.FactoryWorldSessionBracketState{
			SessionID: "bracket-session", SourceRef: "workflow/bracket.js", SourceHash: "bracket-hash",
			PolicyHash: "policy-hash", OrchestratorKind: "JAVASCRIPT", OrchestratorDialect: "typescript",
			ResultStatus:  string(factorydefinitions.FactorySessionResultStatusFailedWithPartial),
			ResultSummary: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "persisted partial result"}},
			ArtifactIDs:   []string{"artifact-present", "artifact-missing"}, Terminal: true,
			FinalStatus: string(factorydefinitions.FactorySessionLifecycleStatusFailed),
			StartedAt:   startedAt, PausedAt: pausedAt, ResumedAt: resumedAt, CompletedAt: completedAt,
			FailureDetail: &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "recorded session failure"},
		},
		Artifacts: []factorydefinitions.FactorySessionArtifactState{{
			ID: "artifact-present", Kind: "RESULT", Visibility: "PUBLIC", ContentHash: "sha256:result", SizeBytes: 11,
		}},
	}

	got, err := ReplayLegacyRecording(value, "requested-session", state)
	if err != nil {
		t.Fatalf("ReplayLegacyRecording: %v", err)
	}
	if got.Session.SessionID != "bracket-session" || got.Session.Status != fse.LifecycleStatusFailed ||
		got.Session.ResolvedSource.SourceRef != "workflow/bracket.js" || got.Result.ResultStatus != fse.ResultStatusFailedWithPartial {
		t.Fatalf("session/result projection = %#v / %#v, want bracket-owned facts", got.Session, got.Result)
	}
	if !strings.Contains(string(got.Result.PrimaryResult), "persisted partial result") ||
		!reflect.DeepEqual(got.Result.ArtifactIDs, []string{"artifact-present", "artifact-missing"}) ||
		len(got.Result.ArtifactRefs) != 1 || got.Result.ArtifactRefs[0].ID != "artifact-present" {
		t.Fatalf("recorded result/artifacts = %#v, want summary and only known artifact reference", got.Result)
	}
	if got.Result.Failure == nil || got.Result.Failure.Message != "recorded session failure" || !got.Result.Failure.PartialResultAvailable {
		t.Fatalf("recorded failure = %#v, want partial session failure", got.Result.Failure)
	}
	if got.Session.Lifecycle == nil || got.Session.Lifecycle.StartedAt == nil ||
		!got.Session.Lifecycle.StartedAt.Equal(startedAt) || got.Session.Lifecycle.PausedAt == nil ||
		!got.Session.Lifecycle.PausedAt.Equal(pausedAt) || got.Session.Lifecycle.ResumedAt == nil ||
		!got.Session.Lifecycle.ResumedAt.Equal(resumedAt) || got.Session.Lifecycle.FinishedAt == nil ||
		!got.Session.Lifecycle.FinishedAt.Equal(completedAt) {
		t.Fatalf("bracket lifecycle timestamps = %#v", got.Session.Lifecycle)
	}
}

func TestReplayLegacyRecordingDerivesLifecycleFromRecognizedEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event recording.FactoryEvent
		want  fse.LifecycleStatus
	}{
		{name: "run response", event: legacyReplayTestEvent("run", recording.FactoryEventTypeRunResponse, "", time.Time{}, json.RawMessage(`{"state":"FAILED"}`)), want: fse.LifecycleStatusFailed},
		{name: "run response without a state", event: legacyReplayTestEvent("run-empty", recording.FactoryEventTypeRunResponse, "", time.Time{}, json.RawMessage(`{"state":null}`)), want: fse.LifecycleStatusRunning},
		{name: "factory state response", event: legacyReplayTestEvent("factory", recording.FactoryEventTypeFactoryStateResponse, "", time.Time{}, json.RawMessage(`{"state":"PAUSED"}`)), want: fse.LifecycleStatusPaused},
		{name: "unknown factory state", event: legacyReplayTestEvent("factory-unknown", recording.FactoryEventTypeFactoryStateResponse, "", time.Time{}, json.RawMessage(`{"state":"UNKNOWN"}`)), want: fse.LifecycleStatusRunning},
		{name: "started", event: legacyReplayTestEvent("started", recording.FactoryEventTypeSessionStarted, "", time.Time{}, json.RawMessage(`{}`)), want: fse.LifecycleStatusRunning},
		{name: "paused", event: legacyReplayTestEvent("paused", recording.FactoryEventTypeSessionPaused, "", time.Time{}, json.RawMessage(`{}`)), want: fse.LifecycleStatusPaused},
		{name: "resumed", event: legacyReplayTestEvent("resumed", recording.FactoryEventTypeSessionResumed, "", time.Time{}, json.RawMessage(`{}`)), want: fse.LifecycleStatusRunning},
		{name: "completed", event: legacyReplayTestEvent("completed", recording.FactoryEventTypeSessionCompleted, "", time.Time{}, json.RawMessage(`{"finalStatus":"CANCELED"}`)), want: fse.LifecycleStatusCanceled},
		{name: "malformed completion status", event: legacyReplayTestEvent("completed-bad", recording.FactoryEventTypeSessionCompleted, "", time.Time{}, json.RawMessage(`{"finalStatus":3}`)), want: fse.LifecycleStatusRunning},
		{name: "unrelated event", event: legacyReplayTestEvent("other", recording.FactoryEventTypeWorkRequest, "", time.Time{}, json.RawMessage(`{}`)), want: fse.LifecycleStatusRunning},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReplayLegacyRecording(recording.ReplayArtifact{Events: []recording.FactoryEvent{test.event}}, "requested", recording.FactoryWorldState{})
			if err != nil {
				t.Fatalf("ReplayLegacyRecording: %v", err)
			}
			if got.Session.Status != test.want {
				t.Fatalf("session status = %q, want %q", got.Session.Status, test.want)
			}
		})
	}
}

func TestLegacyLifecycleStatusNormalizesRecordedNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  fse.LifecycleStatus
	}{
		{value: "COMPLETED", want: fse.LifecycleStatusSucceeded},
		{value: " succeeded ", want: fse.LifecycleStatusSucceeded},
		{value: "FAILED", want: fse.LifecycleStatusFailed},
		{value: "RUNNING", want: fse.LifecycleStatusRunning},
		{value: "PAUSED", want: fse.LifecycleStatusPaused},
		{value: "CANCELED", want: fse.LifecycleStatusCanceled},
		{value: "TIMED_OUT", want: fse.LifecycleStatusTimedOut},
		{value: "INTERRUPTED", want: fse.LifecycleStatusInterrupted},
		{value: "TERMINATED", want: fse.LifecycleStatusTerminated},
		{value: "unrecognized", want: ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()
			if got := normalizeLegacyLifecycleStatus(test.value); got != test.want {
				t.Fatalf("normalizeLegacyLifecycleStatus(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestReplayLegacyRecordingSelectsRecordedResultStatusAndFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		state  recording.FactoryWorldState
		events []recording.FactoryEvent
		want   fse.ResultStatus
	}{
		{
			name: "session bracket result is authoritative",
			state: recording.FactoryWorldState{SessionBracket: &factorydefinitions.FactoryWorldSessionBracketState{
				ResultStatus: "PARTIAL",
			}},
			want: fse.ResultStatus("PARTIAL"),
		},
		{
			name: "javascript runtime result precedes events",
			state: recording.FactoryWorldState{JavaScriptRuntime: &factorydefinitions.FactorySessionJavaScriptRuntimeState{
				ResultStatus: "PARTIAL",
			}},
			events: []recording.FactoryEvent{legacyReplayTestEvent("updated", recording.FactoryEventTypeSessionResultUpdated, "", time.Time{}, json.RawMessage(`{"resultStatus":"FINAL"}`))},
			want:   fse.ResultStatus("PARTIAL"),
		},
		{
			name:   "result update event",
			events: []recording.FactoryEvent{legacyReplayTestEvent("updated", recording.FactoryEventTypeSessionResultUpdated, "", time.Time{}, json.RawMessage(`{"resultStatus":"PARTIAL"}`))},
			want:   fse.ResultStatus("PARTIAL"),
		},
		{
			name:   "completion event",
			events: []recording.FactoryEvent{legacyReplayTestEvent("completed", recording.FactoryEventTypeSessionCompleted, "", time.Time{}, json.RawMessage(`{"resultStatus":"FINAL"}`))},
			want:   fse.ResultStatusFinal,
		},
		{
			name:   "malformed result event falls back",
			state:  recording.FactoryWorldState{FactoryState: "SUCCEEDED"},
			events: []recording.FactoryEvent{legacyReplayTestEvent("updated-bad", recording.FactoryEventTypeSessionResultUpdated, "", time.Time{}, json.RawMessage(`{"resultStatus":7}`))},
			want:   fse.ResultStatusFinal,
		},
		{
			name:   "completion without result falls back",
			events: []recording.FactoryEvent{legacyReplayTestEvent("completed-empty", recording.FactoryEventTypeSessionCompleted, "", time.Time{}, json.RawMessage(`{"finalStatus":"FAILED","resultStatus":null}`))},
			want:   fse.ResultStatusUnavailable,
		},
		{name: "successful lifecycle fallback", state: recording.FactoryWorldState{FactoryState: "SUCCEEDED"}, want: fse.ResultStatusFinal},
		{name: "failed lifecycle fallback", state: recording.FactoryWorldState{FactoryState: "FAILED"}, want: fse.ResultStatusUnavailable},
		{name: "nonterminal lifecycle fallback", state: recording.FactoryWorldState{FactoryState: "RUNNING"}, want: fse.ResultStatusNotReady},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReplayLegacyRecording(recording.ReplayArtifact{Events: test.events}, "requested", test.state)
			if err != nil {
				t.Fatalf("ReplayLegacyRecording: %v", err)
			}
			if got.Result.ResultStatus != test.want {
				t.Fatalf("result status = %q, want %q", got.Result.ResultStatus, test.want)
			}
		})
	}
}

func TestReplayLegacyRecordingEnrichesFailuresFromCompletedDispatches(t *testing.T) {
	t.Parallel()

	workItems := map[string]work.FactoryWorkItem{
		"work-existing": {ID: "work-existing", WorkTypeID: "task", State: "failed"},
	}
	message := "  preserved   worker failure\n" + strings.Repeat("detail ", 100)
	completion := factorydefinitions.FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-failed", TransitionID: "transition-failed",
		WorkItemIDs:     []string{"work-output", "work-existing", " ", "missing-work"},
		OutputWorkItems: []work.FactoryWorkItem{{ID: "work-output", WorkTypeID: "task", State: "failed"}},
		InputWorkItems:  []work.FactoryWorkItem{{ID: "work-input", WorkTypeID: "task", State: "failed"}},
		Result: factorydefinitions.WorkstationResult{
			Outcome: string(workers.OutcomeFailed), Error: message,
			FailureMetadata: &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeUnknown},
		},
	}
	state := recording.FactoryWorldState{
		FactoryState: "FAILED", WorkItemsByID: workItems,
		CompletedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{
			{DispatchID: "dispatch-succeeded", Result: factorydefinitions.WorkstationResult{Outcome: "SUCCEEDED"}, WorkItemIDs: []string{"work-succeeded"}},
			completion,
		},
	}

	got, err := ReplayLegacyRecording(recording.ReplayArtifact{}, "requested", state)
	if err != nil {
		t.Fatalf("ReplayLegacyRecording: %v", err)
	}
	expectedMessage := strings.Join(strings.Fields(message), " ")
	if len(expectedMessage) > 512 {
		expectedMessage = expectedMessage[:512]
	}
	if got.Result.Failure == nil || got.Result.Failure.Reason != string(workers.WorkFailureTypeUnknown) ||
		got.Result.Failure.Message != expectedMessage || got.Result.SessionStatus != fse.LifecycleStatusFailed {
		t.Fatalf("result failure = %#v, want safe recorded failure", got.Result)
	}
	if got.FactoryProjection == nil || len(got.FactoryProjection.FailureDetailsByWorkID) != 3 {
		t.Fatalf("Factory failure projection = %#v, want existing, output, and input Work facts", got.FactoryProjection)
	}
	for _, id := range []string{"work-existing", "work-output", "work-input"} {
		detail, ok := got.FactoryProjection.FailureDetailsByWorkID[id]
		if !ok || detail.DispatchID != "dispatch-failed" || detail.TransitionID != "transition-failed" ||
			detail.WorkItem.ID != id || detail.FailureDetail == nil || detail.FailureDetail.Message != expectedMessage {
			t.Errorf("failure detail for %q = %#v, want preserved dispatch, Work, and safe message", id, detail)
		}
	}
	if _, ok := got.FactoryProjection.FailureDetailsByWorkID["work-succeeded"]; ok {
		t.Fatal("successful completion was converted into a recorded failure")
	}
}

func TestLegacyFailureDetailPrefersRecordedDetailAndUsesSafeFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result factorydefinitions.WorkstationResult
		want   workers.FailureDetail
	}{
		{
			name: "typed recorded detail",
			result: factorydefinitions.WorkstationResult{
				FailureDetail: &workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "typed detail"},
				Error:         "unused error",
			},
			want: workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "typed detail"},
		},
		{
			name: "metadata and error message",
			result: factorydefinitions.WorkstationResult{
				FailureMetadata: &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeUnknown}, Error: "  recorded  error  ", Feedback: "unused feedback",
			},
			want: workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "recorded error"},
		},
		{
			name:   "feedback fallback",
			result: factorydefinitions.WorkstationResult{Feedback: "  recorded\nfeedback  "},
			want:   workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "recorded feedback"},
		},
		{
			name:   "empty fallback",
			result: factorydefinitions.WorkstationResult{},
			want:   workers.FailureDetail{Reason: workers.WorkFailureTypeUnknown, Message: "recorded worker failure"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := legacyFailureDetail(test.result)
			if got == nil || *got != test.want {
				t.Fatalf("legacy failure detail = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestReplayLegacyRecordingUsesRecordingAndWallClockLifecycleFallbacks(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Hour)
	tests := []struct {
		name       string
		recordedAt time.Time
		wallClock  *recording.ReplayWallClockMetadata
		wantStart  time.Time
		wantFinish time.Time
	}{
		{
			name:       "recorded time starts before wall clock metadata",
			recordedAt: startedAt,
			wallClock:  &recording.ReplayWallClockMetadata{StartedAt: startedAt.Add(30 * time.Minute), FinishedAt: finishedAt},
			wantStart:  startedAt, wantFinish: finishedAt,
		},
		{
			name:      "wall clock supplies both missing bounds",
			wallClock: &recording.ReplayWallClockMetadata{StartedAt: startedAt, FinishedAt: finishedAt},
			wantStart: startedAt, wantFinish: finishedAt,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := recording.ReplayArtifact{RecordedAt: test.recordedAt, WallClock: test.wallClock}
			got, err := ReplayLegacyRecording(value, "requested", recording.FactoryWorldState{FactoryState: "RUNNING"})
			if err != nil {
				t.Fatalf("ReplayLegacyRecording: %v", err)
			}
			lifecycle := got.Session.Lifecycle
			if lifecycle == nil || lifecycle.StartedAt == nil || !lifecycle.StartedAt.Equal(test.wantStart) ||
				lifecycle.FinishedAt == nil || !lifecycle.FinishedAt.Equal(test.wantFinish) {
				t.Fatalf("fallback lifecycle = %#v, want start=%v finish=%v", lifecycle, test.wantStart, test.wantFinish)
			}
		})
	}
}

func legacyReplayTestEvent(
	id string,
	eventType recording.FactoryEventType,
	sessionID string,
	eventTime time.Time,
	payload json.RawMessage,
) recording.FactoryEvent {
	context := recording.FactoryEventContext{EventTime: eventTime}
	if sessionID != "" {
		context.SessionID = &sessionID
	}
	return recording.FactoryEvent{
		SchemaVersion: recording.FactoryEventSchemaVersionV1,
		Id:            id, Type: eventType, Context: context, Payload: payload,
	}
}

func legacyReplayStringPointer(value string) *string {
	return &value
}
