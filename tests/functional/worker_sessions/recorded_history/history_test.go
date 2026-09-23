package recordedhistory_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const historyRecordingSessionID = "a6925b33-db77-4e62-ab6f-2a8d06783e5e"

func TestWorkerSessionsHistoryCLIReportsTrustedRecordedAssociations(t *testing.T) {
	t.Parallel()

	runner := &historyCommandRunner{}
	var dispatchCalls atomic.Int32
	var submissionCalls atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		DispatchRecorder: func(recordings.FactoryDispatchRecord) {
			dispatchCalls.Add(1)
		},
		SubmissionRecorder: func(work.FactorySubmissionRecord) {
			submissionCalls.Add(1)
		},
		RecordingReadFile: func(path string) ([]byte, error) {
			if filepath.Base(path) == "unreadable.jsonl" {
				return nil, os.ErrPermission
			}
			return os.ReadFile(path)
		},
	})
	t.Cleanup(func() {
		if got := runner.calls.Load(); got != 0 {
			t.Errorf("controlled provider command runner calls = %d, want 0", got)
		}
		if got := dispatchCalls.Load(); got != 0 {
			t.Errorf("controlled dispatch recorder calls = %d, want 0", got)
		}
		if got := submissionCalls.Load(); got != 0 {
			t.Errorf("controlled submission recorder calls = %d, want 0", got)
		}
	})

	tests := []struct {
		name       string
		workID     string
		fileName   string
		artifact   []byte
		wantStatus string
		wantCode   string
		wantCount  *int
		wantIDs    []string
		wantOpen   []string
	}{
		{
			name:   "F1 recorded associations are sorted and incomplete dispatches are identified",
			workID: "work-history-1", fileName: "available.jsonl",
			artifact: replayV1Artifact(t, []factorydefinitions.FactoryEvent{
				historyEvent(t, 0, "dispatch-open", "work-history-1", factorydefinitions.FactoryEventTypeDispatchRequest,
					factorydefinitions.DispatchRequestEventPayload{TransitionID: "build", Inputs: []factorydefinitions.DispatchConsumedWorkRef{{WorkID: "work-history-1"}}}),
				historyEvent(t, 1, "dispatch-open", "work-history-1", factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
					factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-z"}),
				historyEvent(t, 2, "dispatch-complete", "work-history-1", factorydefinitions.FactoryEventTypeDispatchRequest,
					factorydefinitions.DispatchRequestEventPayload{TransitionID: "review", Inputs: []factorydefinitions.DispatchConsumedWorkRef{{WorkID: "work-history-1"}}}),
				historyEvent(t, 3, "dispatch-complete", "work-history-1", factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
					factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-a"}),
				historyEvent(t, 4, "dispatch-complete", "work-history-1", factorydefinitions.FactoryEventTypeDispatchResponse,
					workers.DispatchResponseEventPayload{Outcome: workers.OutcomeAccepted, TransitionID: "review"}),
			}),
			wantStatus: "AVAILABLE", wantCount: intPointer(2),
			wantIDs: []string{"worker-session-a", "worker-session-z"}, wantOpen: []string{"dispatch-open"},
		},
		{
			name:   "F2 known Work with no association returns a trusted zero count",
			workID: "work-history-empty", fileName: "empty-associations.jsonl",
			artifact: replayV1Artifact(t, []factorydefinitions.FactoryEvent{
				historyEvent(t, 0, "dispatch-unassociated", "work-history-empty", factorydefinitions.FactoryEventTypeDispatchRequest,
					factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"}),
			}),
			wantStatus: "AVAILABLE", wantCount: intPointer(0), wantIDs: []string{}, wantOpen: []string{},
		},
		{
			name:   "F3 unknown Work is distinct from empty association history",
			workID: "work-unknown", fileName: "unknown-work.jsonl",
			artifact: replayV1Artifact(t, []factorydefinitions.FactoryEvent{
				historyEvent(t, 0, "dispatch-known", "work-known", factorydefinitions.FactoryEventTypeDispatchRequest,
					factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"}),
			}),
			wantStatus: "WORK_NOT_FOUND", wantCode: "WORK_NOT_FOUND",
		},
		{
			name:   "F4 empty recording is unavailable",
			workID: "work-history-4", fileName: "empty-recording.jsonl",
			artifact: replayV1Artifact(t, nil), wantStatus: "UNAVAILABLE", wantCode: "RECORDED_WORKER_HISTORY_UNAVAILABLE",
		},
		{
			name:   "F5 missing recording is unavailable",
			workID: "work-history-5", fileName: "missing.jsonl", wantStatus: "UNAVAILABLE",
			wantCode: "RECORDED_WORKER_HISTORY_UNAVAILABLE",
		},
		{
			name:   "unreadable recording is unavailable",
			workID: "work-history-6", fileName: "unreadable.jsonl",
			artifact: replayV1Artifact(t, []factorydefinitions.FactoryEvent{
				historyEvent(t, 0, "dispatch-unreadable", "work-history-6", factorydefinitions.FactoryEventTypeDispatchRequest,
					factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"}),
			}),
			wantStatus: "UNAVAILABLE", wantCode: "RECORDED_WORKER_HISTORY_UNAVAILABLE",
		},
		{
			name:   "F6 a recorded retention gap prevents association counts",
			workID: "work-history-7", fileName: "gap.json",
			artifact:   portableHistoryGapArtifact(t, "work-history-7"),
			wantStatus: "GAP", wantCode: "RECORDED_WORKER_HISTORY_GAP",
		},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			workingDirectory := t.TempDir()
			recordingPath := filepath.Join(t.TempDir(), testCase.fileName)
			if testCase.artifact != nil {
				if err := os.WriteFile(recordingPath, testCase.artifact, 0o600); err != nil {
					t.Fatalf("write recording fixture: %v", err)
				}
			}
			inputs := support.FakeInputs(context.Background(), []string{
				"you", "worker-sessions", "history", "--recording", recordingPath,
				"--session", historyRecordingSessionID, "--work-id", testCase.workID, "--output", "json",
			})
			inputs.Input.Env = historyEnvironment(home)
			inputs.Input.WorkingDirectory = workingDirectory
			if err := process.Execute(inputs.Input); err != nil {
				t.Fatalf("Process.Execute: %v\nstdout: %s\nstderr: %s", err, inputs.Stdout(), inputs.Stderr())
			}
			var result historyCLIResult
			if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
				t.Fatalf("decode public history JSON %q: %v", inputs.Stdout(), err)
			}
			if result.FactorySessionID != historyRecordingSessionID || result.WorkID != testCase.workID || result.Status != testCase.wantStatus {
				t.Fatalf("public result identity/status = %#v, want session=%s work=%s status=%s", result, historyRecordingSessionID, testCase.workID, testCase.wantStatus)
			}
			if result.ErrorCode != testCase.wantCode {
				t.Fatalf("public errorCode = %q, want %q", result.ErrorCode, testCase.wantCode)
			}
			if testCase.wantCount == nil {
				if result.Count != nil || result.WorkerSessionIDs != nil || result.IncompleteDispatchIDs != nil {
					t.Fatalf("untrusted association result contains a count or IDs: %#v", result)
				}
			} else {
				if result.Count == nil || *result.Count != *testCase.wantCount {
					t.Fatalf("public count = %v, want %d", result.Count, *testCase.wantCount)
				}
				if !reflect.DeepEqual(result.WorkerSessionIDs, testCase.wantIDs) || !reflect.DeepEqual(result.IncompleteDispatchIDs, testCase.wantOpen) {
					t.Fatalf("public association sets = %#v/%#v, want %#v/%#v", result.WorkerSessionIDs, result.IncompleteDispatchIDs, testCase.wantIDs, testCase.wantOpen)
				}
			}
		})
	}
}

type historyCommandRunner struct {
	calls atomic.Int32
}

func (runner *historyCommandRunner) Run(context.Context, process.CommandRequest) (process.CommandResult, error) {
	runner.calls.Add(1)
	return process.CommandResult{ExitCode: 1}, errors.New("unexpected provider command")
}

type historyCLIResult struct {
	FactorySessionID      string   `json:"factorySessionId"`
	WorkID                string   `json:"workId"`
	Status                string   `json:"status"`
	Count                 *int     `json:"count"`
	WorkerSessionIDs      []string `json:"workerSessionIds"`
	IncompleteDispatchIDs []string `json:"incompleteDispatchIds"`
	ErrorCode             string   `json:"errorCode"`
}

func historyEvent(
	t *testing.T,
	sequence int,
	dispatchID string,
	workID string,
	eventType factorydefinitions.FactoryEventType,
	payload any,
) factorydefinitions.FactoryEvent {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}
	scope := "~default"
	workIDs := []string{workID}
	return factorydefinitions.FactoryEvent{
		Type: eventType,
		Id:   fmt.Sprintf("history/%s/%s", dispatchID, eventType),
		Context: factorydefinitions.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
			Sequence:   sequence,
			SessionID:  &scope,
			Tick:       1,
			WorkIDs:    &workIDs,
		},
		Payload:       encoded,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
	}
}

func replayV1Artifact(t *testing.T, events []factorydefinitions.FactoryEvent) []byte {
	t.Helper()
	payload, err := json.Marshal(struct {
		SchemaVersion string                            `json:"schemaVersion"`
		RecordedAt    time.Time                         `json:"recordedAt"`
		Events        []factorydefinitions.FactoryEvent `json:"events"`
	}{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		Events:        events,
	})
	if err != nil {
		t.Fatalf("marshal replay v1 fixture: %v", err)
	}
	return payload
}

func portableHistoryGapArtifact(t *testing.T, workID string) []byte {
	t.Helper()
	event := historyEvent(t, 0, "dispatch-gap", workID, factorydefinitions.FactoryEventTypeDispatchRequest,
		factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"})
	canonicalEvent := canonicalHistoryEvent(event, "generation-gap")
	artifact := recordings.PortableArtifact{
		SchemaVersion: recordings.PortableArtifactSchemaV1,
		Summary: recordings.PortableArtifactSummary{
			RecordingID: historyRecordingSessionID,
			Scope:       canonicalEvent.Scope,
			State:       recordings.RecordingFinalized,
			Available:   true,
			EventCount:  1,
			FirstCursor: &canonicalEvent.Cursor,
			LastCursor:  &canonicalEvent.Cursor,
			Failures:    []recordings.RecordingFailure{{Code: "RETENTION_GAP"}},
		},
		Events: []recordings.CanonicalEvent{canonicalEvent},
		Integrity: recordings.PortableArtifactIntegrity{
			Algorithm: recordings.PortableArtifactIntegritySHA256,
		},
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal portable gap artifact: %v", err)
	}
	artifact.Integrity.Digest = recordings.PortableArtifactIntegritySHA256 + ":" + digestPortableArtifact(t, payload)
	encoded, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal portable gap artifact with digest: %v", err)
	}
	return encoded
}

func digestPortableArtifact(t *testing.T, payload []byte) string {
	t.Helper()
	var artifact recordings.PortableArtifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("decode portable artifact: %v", err)
	}
	artifact.Integrity.Digest = ""
	unsigned, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal unsigned portable artifact: %v", err)
	}
	digest := sha256.Sum256(unsigned)
	return hex.EncodeToString(digest[:])
}

func canonicalHistoryEvent(event factorydefinitions.FactoryEvent, generationID string) recordings.CanonicalEvent {
	sourceContext, _ := json.Marshal(event.Context)
	scope := recordings.CanonicalEventScope{}
	if event.Context.SessionID != nil {
		scope.FactorySessionID = *event.Context.SessionID
	}
	sequence := recordings.CanonicalEventSequence(event.Context.Sequence)
	return recordings.CanonicalEvent{
		ID:          recordings.CanonicalEventID(event.Id),
		Sequence:    sequence,
		FactoryTick: event.Context.Tick,
		Scope:       scope,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: generationID,
			Sequence:           sequence,
		},
		RecordedAt:    event.Context.EventTime,
		Kind:          recordings.CanonicalEventKind(event.Type),
		Payload:       string(event.Payload),
		SourceContext: string(sourceContext),
	}
}

func historyEnvironment(home string) []string {
	volume := filepath.VolumeName(home)
	homePath := strings.TrimPrefix(home, volume)
	environment := make([]string, 0, len(os.Environ())+4)
	for _, item := range os.Environ() {
		key, _, found := strings.Cut(item, "=")
		if found && (strings.EqualFold(key, "HOME") || strings.EqualFold(key, "USERPROFILE") || strings.EqualFold(key, "HOMEDRIVE") || strings.EqualFold(key, "HOMEPATH")) {
			continue
		}
		environment = append(environment, item)
	}
	return append(environment, "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE="+volume, "HOMEPATH="+homePath)
}

func intPointer(value int) *int { return &value }
