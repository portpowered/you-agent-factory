package service

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestQueryHistoricalWorkerAssociationsReturnsSortedIDsAndIncompleteDispatches(t *testing.T) {
	t.Parallel()

	workID := "work-recorded-1"
	events := []factorydefinitions.FactoryEvent{
		historicalAssociationEvent(t, 0, "dispatch-open", workID, factorydefinitions.FactoryEventTypeDispatchRequest,
			factorydefinitions.DispatchRequestEventPayload{
				TransitionID: "build",
				Inputs:       []factorydefinitions.DispatchConsumedWorkRef{{WorkID: workID}},
			}),
		historicalAssociationEvent(t, 1, "dispatch-open", workID, factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
			factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-z"}),
		historicalAssociationEvent(t, 2, "dispatch-complete", workID, factorydefinitions.FactoryEventTypeDispatchRequest,
			factorydefinitions.DispatchRequestEventPayload{
				TransitionID: "review",
				Inputs:       []factorydefinitions.DispatchConsumedWorkRef{{WorkID: workID}},
			}),
		historicalAssociationEvent(t, 3, "dispatch-complete", workID, factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc,
			factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-a"}),
		historicalAssociationEvent(t, 4, "dispatch-complete", workID, factorydefinitions.FactoryEventTypeDispatchResponse,
			workers.DispatchResponseEventPayload{Outcome: workers.OutcomeAccepted, TransitionID: "review"}),
	}
	payload := historicalReplayV1(t, events)
	query := New(func(string) ([]byte, error) { return payload, nil }, recordingsinternal.NewProjectionService(), logging.NoopLogger{}, testHistoricalQueryClock())
	result, err := query.QueryHistoricalWorkerAssociations(recordings.HistoricalWorkerAssociationsRequest{
		Recording: recordings.HistoricalRecordingIdentity{RecordingID: "recording-1", Artifact: "history.jsonl"},
		WorkID:    workID,
	})
	if err != nil {
		t.Fatalf("QueryHistoricalWorkerAssociations: %v", err)
	}
	if result.FactorySessionID != "recording-1" || result.WorkID != workID || result.State != recordings.HistoricalWorkerAssociationsAvailable {
		t.Fatalf("result identity/state = %#v, want recording-1/%s/AVAILABLE", result, workID)
	}
	if result.Count != 2 || !reflect.DeepEqual(result.WorkerSessionIDs, []string{"worker-session-a", "worker-session-z"}) {
		t.Fatalf("associations = %#v, want sorted worker-session IDs", result)
	}
	if !reflect.DeepEqual(result.IncompleteDispatchIDs, []string{"dispatch-open"}) {
		t.Fatalf("incomplete dispatches = %#v, want dispatch-open", result.IncompleteDispatchIDs)
	}
}

func TestQueryHistoricalWorkerAssociationsClassifiesUnavailableAndUnknownWork(t *testing.T) {
	t.Parallel()

	workID := "work-recorded-2"
	valid := historicalReplayV1(t, []factorydefinitions.FactoryEvent{
		historicalAssociationEvent(t, 0, "dispatch-1", workID, factorydefinitions.FactoryEventTypeDispatchRequest,
			factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"}),
	})
	tests := []struct {
		name   string
		read   func(string) ([]byte, error)
		workID string
		state  recordings.HistoricalWorkerAssociationsState
		code   string
	}{
		{name: "empty artifact", read: func(string) ([]byte, error) { return historicalReplayV1(t, nil), nil }, workID: workID,
			state: recordings.HistoricalWorkerAssociationsUnavailable, code: "RECORDED_WORKER_HISTORY_UNAVAILABLE"},
		{name: "missing file", read: func(string) ([]byte, error) { return nil, os.ErrNotExist }, workID: workID,
			state: recordings.HistoricalWorkerAssociationsUnavailable, code: "RECORDED_WORKER_HISTORY_UNAVAILABLE"},
		{name: "unreadable file", read: func(string) ([]byte, error) { return nil, os.ErrPermission }, workID: workID,
			state: recordings.HistoricalWorkerAssociationsUnavailable, code: "RECORDED_WORKER_HISTORY_UNAVAILABLE"},
		{name: "corrupt artifact", read: func(string) ([]byte, error) { return []byte("not json"), nil }, workID: workID,
			state: recordings.HistoricalWorkerAssociationsUnavailable, code: "RECORDED_WORKER_HISTORY_UNAVAILABLE"},
		{name: "unknown Work", read: func(string) ([]byte, error) { return valid, nil }, workID: "work-unknown",
			state: recordings.HistoricalWorkerAssociationsWorkNotFound, code: "WORK_NOT_FOUND"},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			query := New(testCase.read, recordingsinternal.NewProjectionService(), logging.NoopLogger{}, testHistoricalQueryClock())
			result, err := query.QueryHistoricalWorkerAssociations(recordings.HistoricalWorkerAssociationsRequest{
				Recording: recordings.HistoricalRecordingIdentity{RecordingID: "recording-2", Artifact: "history.jsonl"},
				WorkID:    testCase.workID,
			})
			if err != nil {
				t.Fatalf("QueryHistoricalWorkerAssociations: %v", err)
			}
			if result.State != testCase.state || result.ErrorCode != testCase.code {
				t.Fatalf("state/errorCode = %s/%q, want %s/%q", result.State, result.ErrorCode, testCase.state, testCase.code)
			}
			if result.Count != 0 || result.WorkerSessionIDs != nil || result.IncompleteDispatchIDs != nil {
				t.Fatalf("untrusted associations were returned: %#v", result)
			}
		})
	}
}

func TestQueryHistoricalWorkerAssociationsLogsOutcomeWithoutArtifactPath(t *testing.T) {
	t.Parallel()

	logger := &historicalQueryTestLogger{}
	artifactPath := `C:\private\recordings\history.jsonl`
	query := New(func(string) ([]byte, error) { return nil, os.ErrPermission }, recordingsinternal.NewProjectionService(), logger, &advancingHistoricalQueryClock{
		at:   time.Unix(1_700_000_000, 0).UTC(),
		step: 125 * time.Millisecond,
	})
	result, err := query.QueryHistoricalWorkerAssociations(recordings.HistoricalWorkerAssociationsRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: "recording-safe-id",
			Artifact:    recordings.RecordingArtifactReference(artifactPath),
		},
		WorkID: "work-safe-id",
	})
	if err != nil {
		t.Fatalf("QueryHistoricalWorkerAssociations: %v", err)
	}
	if result.State != recordings.HistoricalWorkerAssociationsUnavailable {
		t.Fatalf("history state = %s, want UNAVAILABLE", result.State)
	}
	if len(logger.entries) != 2 {
		t.Fatalf("operation log entries = %d, want start and terminal entries: %#v", len(logger.entries), logger.entries)
	}
	start := historicalQueryLogFields(t, logger.entries[0])
	if logger.entries[0].message != "recordings historical worker associations query started" ||
		start["recordingID"] != "recording-safe-id" || start["workID"] != "work-safe-id" {
		t.Fatalf("operation start log = %#v, want safe recording/work identifiers", logger.entries[0])
	}
	finished := historicalQueryLogFields(t, logger.entries[1])
	if logger.entries[1].message != "recordings historical worker associations query finished" ||
		finished["outcome"] != "unavailable" || finished["failureClass"] != "artifact_read_failed" {
		t.Fatalf("terminal operation log = %#v, want unavailable artifact_read_failed", logger.entries[1])
	}
	if duration, ok := finished["duration"].(time.Duration); !ok || duration != 125*time.Millisecond {
		t.Fatalf("terminal operation duration = %#v, want 125ms from the injected clock", finished["duration"])
	}
	for _, entry := range logger.entries {
		for _, value := range entry.fields {
			if strings.Contains(fmt.Sprint(value), artifactPath) {
				t.Fatalf("operation log leaked local artifact path %q: %#v", artifactPath, entry)
			}
		}
	}
}

type advancingHistoricalQueryClock struct {
	at    time.Time
	step  time.Duration
	calls int
}

func (clock *advancingHistoricalQueryClock) Now() time.Time {
	at := clock.at.Add(time.Duration(clock.calls) * clock.step)
	clock.calls++
	return at
}

func testHistoricalQueryClock() recordings.RecordingClock {
	return &advancingHistoricalQueryClock{at: time.Unix(1_700_000_000, 0).UTC()}
}

type historicalQueryLogEntry struct {
	message string
	fields  []any
}

type historicalQueryTestLogger struct {
	entries []historicalQueryLogEntry
}

func (logger *historicalQueryTestLogger) Debug(string, ...any)   {}
func (logger *historicalQueryTestLogger) Warn(string, ...any)    {}
func (logger *historicalQueryTestLogger) Error(string, ...any)   {}
func (logger *historicalQueryTestLogger) Verbose(string, ...any) {}
func (logger *historicalQueryTestLogger) Info(message string, fields ...any) {
	logger.entries = append(logger.entries, historicalQueryLogEntry{message: message, fields: append([]any(nil), fields...)})
}

func historicalQueryLogFields(t *testing.T, entry historicalQueryLogEntry) map[string]any {
	t.Helper()
	fields := make(map[string]any, len(entry.fields)/2)
	for index := 0; index+1 < len(entry.fields); index += 2 {
		key, ok := entry.fields[index].(string)
		if !ok {
			t.Fatalf("operation log field key[%d] = %#v, want string", index, entry.fields[index])
		}
		fields[key] = entry.fields[index+1]
	}
	return fields
}

func TestHistoricalRecordingHasGapUsesRecordedFailureCode(t *testing.T) {
	t.Parallel()

	if !historicalRecordingHasGap(recordings.RecordingStatusFacts{Failures: []recordings.RecordingFailure{{Code: "RETENTION_GAP"}}}) {
		t.Fatal("RETENTION_GAP failure was not classified as a history gap")
	}
	if historicalRecordingHasGap(recordings.RecordingStatusFacts{Failures: []recordings.RecordingFailure{{Code: "FLUSH_FAILED"}}}) {
		t.Fatal("ordinary flush failure was classified as a history gap")
	}
}

func TestHistoricalArtifactScopeRequiresReplayV2HeaderSessionMatch(t *testing.T) {
	t.Parallel()

	recordingID := recordings.RecordingID("a6925b33-db77-4e62-ab6f-2a8d06783e5e")
	header := replayimpl.ReplayV2Header{
		RecordType:    "header",
		SchemaVersion: replayimpl.ReplayV2SchemaVersion,
		RecordedAt:    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		SessionID:     string(recordingID),
		FactoryIdentity: replayimpl.ReplayV2FactoryIdentity{
			ID: "factory-1", Name: "Factory", FactoryDirectory: ".", SourceDirectory: ".",
		},
		Hashes: map[string]string{
			"factory_hash": "sha256:factory", "workers_hash": "sha256:workers",
			"workstations_hash": "sha256:workstations", "runtime_config_hash": "sha256:runtime",
		},
	}
	headerPayload, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal replay v2 header: %v", err)
	}
	headerPayload = append(headerPayload, '\n')
	eventPayload, err := replayimpl.MarshalReplayV2Event(historicalAssociationEvent(
		t, 0, "dispatch-1", "work-1", factorydefinitions.FactoryEventTypeDispatchRequest,
		factorydefinitions.DispatchRequestEventPayload{TransitionID: "build"},
	))
	if err != nil {
		t.Fatalf("marshal replay v2 event: %v", err)
	}
	artifact := append(headerPayload, eventPayload...)

	if _, found, err := historicalArtifactScope(artifact, recordings.RecordingID("other-session")); err != nil || found {
		t.Fatalf("mismatched header scope = (found %t, error %v), want no scope", found, err)
	}
	scope, found, err := historicalArtifactScope(artifact, recordingID)
	if err != nil || !found || scope.FactorySessionID != "~default" {
		t.Fatalf("matched header scope = (%#v, found %t, error %v), want ~default", scope, found, err)
	}
}

func historicalAssociationEvent(
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
		t.Fatalf("marshal %s payload: %v", eventType, err)
	}
	scope := "~default"
	workIDs := []string{workID}
	eventTime := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	return factorydefinitions.FactoryEvent{
		Type: eventType,
		Id:   "history-event-" + dispatchID + "-" + string(eventType),
		Context: factorydefinitions.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  eventTime,
			Sequence:   sequence,
			SessionID:  &scope,
			Tick:       1,
			WorkIDs:    &workIDs,
		},
		Payload:       encoded,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
	}
}

func historicalReplayV1(t *testing.T, events []factorydefinitions.FactoryEvent) []byte {
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
