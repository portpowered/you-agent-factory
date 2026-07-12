package recordingreplay

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/recording"
)

func TestReplayRecordingRestoresCompletedPublicReadModelsWithoutLiveExecution(t *testing.T) {
	t.Parallel()
	value := buildTerminalRecording(t, "SUCCEEDED", &recording.CanonicalResult{
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
	value := buildTerminalRecording(t, "FAILED", &recording.CanonicalResult{
		Status: "FAILED_WITH_PARTIAL", Mode: "partial", PrimaryResult: json.RawMessage(`{"partial":true}`),
		ArtifactIDs: []string{"artifact-result"},
		Failure:     &recording.FailureSummary{Reason: "WORKFLOW_FAILED", Message: "safe failure", PartialResultAvailable: true},
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
	value := buildTerminalRecording(t, "SUCCEEDED", &recording.CanonicalResult{
		Status: "FINAL", Mode: "final", PrimaryResult: json.RawMessage(`{"answer":"done"}`), ArtifactIDs: []string{"artifact-result"},
	})
	value.Result.PrimaryResult = json.RawMessage(`{"answer":"tampered"}`)

	got, err := ReplayRecording(value)
	var diagnostic *recording.Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Code != recording.CodeInvalidDigest || diagnostic.Path != "result.contentHash" {
		t.Fatalf("ReplayRecording error = %#v", err)
	}
	if !reflect.DeepEqual(got, RecordingReplayProjection{}) {
		t.Fatalf("untrusted partial projection = %#v", got)
	}
}

func buildTerminalRecording(t *testing.T, status string, result *recording.CanonicalResult) recording.Recording {
	t.Helper()
	createdAt := time.Date(2026, 7, 12, 18, 0, 1, 0, time.UTC)
	value, err := recording.Build(recording.CanonicalFacts{
		SessionID: "dur-sess-recording-replay-terminal", Status: status, OrchestratorKind: "JAVASCRIPT",
		SourceRef: "workflow/terminal.js", SourceHash: recordingTestDigest('1'), PolicyHash: recordingTestDigest('2'),
		Artifacts: []recording.CanonicalArtifact{{ID: "artifact-result", Kind: "RESULT", Visibility: "PUBLIC", Label: "Result", ContentHash: recordingTestDigest('3'), SizeBytes: 21, CreatedAt: createdAt}},
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

func assertRecordedInspectionParity(t *testing.T, value recording.Recording, got RecordingReplayProjection) {
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
