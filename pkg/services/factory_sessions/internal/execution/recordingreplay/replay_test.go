package recordingreplay

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	recording "github.com/portpowered/infinite-you/pkg/services/recordings"
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
