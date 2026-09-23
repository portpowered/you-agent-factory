package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

type historicalWorkerAssociationsReaderFunc func(recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error)

func (reader historicalWorkerAssociationsReaderFunc) QueryHistoricalWorkerAssociations(
	request recordings.HistoricalWorkerAssociationsRequest,
) (recordings.HistoricalWorkerAssociationsResult, error) {
	return reader(request)
}

func TestHistoryWritesAvailableAssociationsAsJSON(t *testing.T) {
	t.Parallel()

	var request recordings.HistoricalWorkerAssociationsRequest
	var output bytes.Buffer
	operation := NewHistory(historicalWorkerAssociationsReaderFunc(func(got recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error) {
		request = got
		return recordings.HistoricalWorkerAssociationsResult{
			FactorySessionID:      "recording-session-1",
			WorkID:                "work-1",
			State:                 recordings.HistoricalWorkerAssociationsAvailable,
			Count:                 2,
			WorkerSessionIDs:      []string{"worker-a", "worker-z"},
			IncompleteDispatchIDs: []string{"dispatch-open"},
		}, nil
	}))
	path := filepath.Join(t.TempDir(), "recording.jsonl")
	if err := operation(HistoryConfig{
		Context: context.Background(), Recording: path, SessionID: "recording-session-1",
		WorkID: "work-1", JSON: true, Output: &output,
	}); err != nil {
		t.Fatalf("HistoryOperation: %v", err)
	}
	if request.Recording.RecordingID != "recording-session-1" || request.Recording.Artifact != recordings.RecordingArtifactReference(path) || request.Recording.Scope.FactorySessionID != "" || request.WorkID != "work-1" {
		t.Fatalf("Recordings request = %#v, want artifact/session/work without a fabricated scope", request)
	}
	var result struct {
		FactorySessionID      string   `json:"factorySessionId"`
		WorkID                string   `json:"workId"`
		Status                string   `json:"status"`
		Count                 *int     `json:"count"`
		WorkerSessionIDs      []string `json:"workerSessionIds"`
		IncompleteDispatchIDs []string `json:"incompleteDispatchIds"`
		ErrorCode             *string  `json:"errorCode"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("history JSON %q: %v", output.String(), err)
	}
	if result.FactorySessionID != "recording-session-1" || result.WorkID != "work-1" || result.Status != "AVAILABLE" || result.Count == nil || *result.Count != 2 {
		t.Fatalf("JSON identity/status/count = %#v, want available count 2", result)
	}
	if !reflect.DeepEqual(result.WorkerSessionIDs, []string{"worker-a", "worker-z"}) || !reflect.DeepEqual(result.IncompleteDispatchIDs, []string{"dispatch-open"}) || result.ErrorCode != nil {
		t.Fatalf("JSON association details = %#v", result)
	}
}

func TestHistoryOmitsUntrustedCountsAndReportsDistinctStatus(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	operation := NewHistory(historicalWorkerAssociationsReaderFunc(func(recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error) {
		return recordings.HistoricalWorkerAssociationsResult{
			FactorySessionID: "recording-session-2",
			WorkID:           "work-2",
			State:            recordings.HistoricalWorkerAssociationsGap,
			Count:            7,
			WorkerSessionIDs: []string{"untrusted"},
			ErrorCode:        "RECORDED_WORKER_HISTORY_GAP",
		}, nil
	}))
	err := operation(HistoryConfig{
		Context: context.Background(), Recording: filepath.Join(t.TempDir(), "recording.jsonl"),
		SessionID: "recording-session-2", WorkID: "work-2", JSON: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("HistoryOperation: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatalf("history JSON %q: %v", output.String(), err)
	}
	for _, field := range []string{"count", "workerSessionIds", "incompleteDispatchIds"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("untrusted field %q was emitted in %s", field, output.String())
		}
	}
	var result struct {
		Status    string  `json:"status"`
		ErrorCode *string `json:"errorCode"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "GAP" || result.ErrorCode == nil || *result.ErrorCode != "RECORDED_WORKER_HISTORY_GAP" {
		t.Fatalf("JSON status/errorCode = %#v, want GAP with typed code", result)
	}
}

func TestHistoryRejectsRelativePathBeforeCallingReader(t *testing.T) {
	t.Parallel()

	called := false
	var output bytes.Buffer
	operation := NewHistory(historicalWorkerAssociationsReaderFunc(func(recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error) {
		called = true
		return recordings.HistoricalWorkerAssociationsResult{}, nil
	}))
	err := operation(HistoryConfig{
		Context: context.Background(), Recording: "relative.jsonl", SessionID: "recording-session-3",
		WorkID: "work-3", JSON: true, Output: &output,
	})
	if err == nil || called || !strings.Contains(err.Error(), "absolute local file path") {
		t.Fatalf("relative-path invocation = (%v, called %t), want path error before reader", err, called)
	}
	if !strings.Contains(output.String(), "WORKER_SESSION_HISTORY_INVALID") {
		t.Fatalf("JSON usage diagnostic = %q, want WORKER_SESSION_HISTORY_INVALID", output.String())
	}
}
