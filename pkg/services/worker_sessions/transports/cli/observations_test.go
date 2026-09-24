package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	if err == nil || cliErrorCode(err) != "RECORDED_WORKER_HISTORY_GAP" {
		t.Fatalf("HistoryOperation error = %v, want typed GAP failure", err)
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

func TestHistoryFailureStatusesReturnActionableErrors(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		state recordings.HistoricalWorkerAssociationsState
		code  string
	}{
		{state: recordings.HistoricalWorkerAssociationsGap, code: "RECORDED_WORKER_HISTORY_GAP"},
		{state: recordings.HistoricalWorkerAssociationsUnavailable, code: "RECORDED_WORKER_HISTORY_UNAVAILABLE"},
		{state: recordings.HistoricalWorkerAssociationsWorkNotFound, code: "WORK_NOT_FOUND"},
	} {
		testCase := testCase
		t.Run(testCase.code, func(t *testing.T) {
			t.Parallel()
			operation := NewHistory(historicalWorkerAssociationsReaderFunc(func(recordings.HistoricalWorkerAssociationsRequest) (recordings.HistoricalWorkerAssociationsResult, error) {
				return recordings.HistoricalWorkerAssociationsResult{
					FactorySessionID: "recording-session-3", WorkID: "work-3", State: testCase.state, ErrorCode: testCase.code,
				}, nil
			}))
			var output bytes.Buffer
			err := operation(HistoryConfig{
				Context: context.Background(), Recording: filepath.Join(t.TempDir(), "recording.jsonl"),
				SessionID: "recording-session-3", WorkID: "work-3", JSON: true, Output: &output,
			})
			if err == nil || cliErrorCode(err) != testCase.code {
				t.Fatalf("HistoryOperation error = %v, want code %q", err, testCase.code)
			}
			var result struct {
				Status    string  `json:"status"`
				ErrorCode *string `json:"errorCode"`
			}
			if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
				t.Fatalf("decode retained result JSON %q: %v", output.String(), decodeErr)
			}
			if result.Status != string(testCase.state) || result.ErrorCode == nil || *result.ErrorCode != testCase.code {
				t.Fatalf("status/errorCode JSON = %#v, want %s/%s", result, testCase.state, testCase.code)
			}
		})
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

func TestReadJSONUsesTranscriptRouteAndPreservesNormalizedEntries(t *testing.T) {
	var gotPath string
	var gotQuery map[string]string
	timestamp := time.Date(2026, 8, 8, 20, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = map[string]string{"provider": r.URL.Query().Get("provider"), "kind": r.URL.Query().Get("kind"), "id": r.URL.Query().Get("id")}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionTranscriptResponse{
			WorkerSessionId: "worker-session-1",
			ProviderSession: generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
			WorkIds:         []string{"work-1"}, TurnId: stringPtrForTest("turn-1"), AttemptId: "attempt-1", State: "COMPLETED",
			Entries: []generated.ProviderSessionTranscriptEntry{
				{Order: 1, Type: generated.ProviderSessionTranscriptEntryType("user_message"), Text: stringPtrForTest("operator request")},
				{Order: 2, Type: generated.ProviderSessionTranscriptEntryType("tool_call"), Name: stringPtrForTest("lookup"), Arguments: stringPtrForTest(`{"key":"value"}`)},
				{Order: 3, Type: generated.ProviderSessionTranscriptEntryType("reasoning"), Encrypted: boolPtrForTest(true), EncryptedContent: stringPtrForTest("encrypted")},
				{Order: 4, Type: generated.ProviderSessionTranscriptEntryType("assistant_message"), Text: stringPtrForTest("done"), Timestamp: &timestamp},
			},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		Provider: "codex", Kind: "session_id", ID: "provider-session-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	assertTranscriptReadRequest(t, gotPath, gotQuery)
	assertTranscriptReadJSON(t, output.Bytes())
}

func TestReadByWorkerSessionIDUsesTopLevelTranscriptRoute(t *testing.T) {
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionTranscriptResponse{
			WorkerSessionId: "direct-1", AttemptId: "attempt-1", State: "COMPLETED",
			ProviderSession: generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-1"},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "direct-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if gotPath != "/worker-sessions/direct-1/transcript" || gotQuery != "" {
		t.Fatalf("request = path=%q query=%q, want top-level identity transcript route", gotPath, gotQuery)
	}
	if !strings.Contains(output.String(), `"workerSessionId":"direct-1"`) {
		t.Fatalf("output = %q, want transcript identity", output.String())
	}
}

func TestReadByWorkerSessionIDUsesFactorySessionScopedTranscriptRoute(t *testing.T) {
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionTranscriptResponse{
			WorkerSessionId: "worker-1", FactorySessionId: stringPtrForTest("session-1"),
			ProviderSession: generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-1"},
			WorkIds:         []string{"work-1"}, AttemptId: "attempt-1", State: "COMPLETED",
			Entries: []generated.ProviderSessionTranscriptEntry{{Order: 1, Type: generated.ProviderSessionTranscriptEntryType("assistant_message"), Text: stringPtrForTest("done")}},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		WorkerSessionID: "worker-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if gotPath != "/factory-sessions/session-1/worker-sessions/worker-1/transcript" || gotQuery != "" {
		t.Fatalf("request = path=%q query=%q, want Factory Session scoped identity transcript route", gotPath, gotQuery)
	}
	var response generated.WorkerSessionTranscriptResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode scoped transcript response: %v; output=%q", err, output.String())
	}
	if response.FactorySessionId == nil || *response.FactorySessionId != "session-1" ||
		response.WorkerSessionId != "worker-1" || len(response.WorkIds) != 1 || response.WorkIds[0] != "work-1" {
		t.Fatalf("scoped transcript response = %#v, want exact Factory Session and Work identity", response)
	}
}

func assertTranscriptReadRequest(t *testing.T, path string, query map[string]string) {
	t.Helper()
	if path != "/factory-sessions/session-1/worker-sessions/transcript" || query["provider"] != "codex" || query["kind"] != "session_id" || query["id"] != "provider-session-1" {
		t.Fatalf("request = path=%s query=%#v, want exact transcript route and identity", path, query)
	}
}

func assertTranscriptReadJSON(t *testing.T, payload []byte) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, string(payload))
	}
	if string(document["workerSessionId"]) != `"worker-session-1"` || string(document["attemptId"]) != `"attempt-1"` || string(document["turnId"]) != `"turn-1"` {
		t.Fatalf("envelope = %s/%s/%s, want session/attempt/turn", document["workerSessionId"], document["attemptId"], document["turnId"])
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(document["entries"], &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 4 || string(entries[1]["type"]) != `"tool_call"` || string(entries[2]["encrypted"]) != "true" || string(entries[3]["timestamp"]) != `"2026-08-08T20:00:00Z"` {
		t.Fatalf("entries = %#v, want ordered tool/encrypted/timestamp fields", entries)
	}
	if string(entries[0]["output"]) != "null" || string(entries[0]["callId"]) != "null" {
		t.Fatalf("optional entry fields = output=%s callId=%s, want explicit nulls", entries[0]["output"], entries[0]["callId"])
	}
}

func TestReadHumanLabelsTranscriptRolesAndEncryptedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionTranscriptResponse{
			WorkerSessionId: "worker-session-1", FactorySessionId: stringPtrForTest("factory-session-1"),
			ProviderSession: generated.WorkerSessionProviderSessionRef{Provider: "cursor", Kind: "session_id", Id: "cursor-session-1"},
			WorkIds:         []string{"work-1"}, AttemptId: "attempt-1", State: "FAILED",
			Entries: []generated.ProviderSessionTranscriptEntry{
				{Order: 1, Type: generated.ProviderSessionTranscriptEntryType("tool_output"), Output: stringPtrForTest("tool result")},
				{Order: 2, Type: generated.ProviderSessionTranscriptEntryType("reasoning"), Encrypted: boolPtrForTest(true), EncryptedContent: stringPtrForTest("ciphertext")},
			},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: context.Background(), Server: server.URL, Provider: "cursor", Kind: "session_id", ID: "cursor-session-1", Output: &output})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	for _, want := range []string{
		"Worker Session ID:\tworker-session-1", "Factory Session ID:\tfactory-session-1", "Entries:\t2",
		"Entry 1:\ttype=tool_output order=1 role=tool", "output=tool result",
		"Entry 2:\ttype=reasoning order=2 role=reasoning-summary", "encrypted=true encryptedContent=ciphertext",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output %q missing %q", output.String(), want)
		}
	}
}

func TestReadMapsMissingActiveUnavailableAndProjectionFailuresToStableErrors(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		status int
		code   string
	}{
		{name: "missing", status: http.StatusNotFound, code: "WORKER_SESSION_NOT_FOUND"},
		{name: "active", status: http.StatusConflict, code: "WORKER_SESSION_TRANSCRIPT_ACTIVE"},
		{name: "unavailable", status: http.StatusInternalServerError, code: "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"},
		{name: "projection", status: http.StatusInternalServerError, code: "WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.status)
				_ = json.NewEncoder(w).Encode(generated.ErrorResponse{Message: testCase.name, Code: generated.ErrorResponseCode(testCase.code), Family: generated.ErrorFamilyInternalServerError})
			}))
			defer server.Close()
			var output bytes.Buffer
			err := NewRead(testHTTPProtocol(t))(ReadConfig{Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "session-1", OutputFormat: "json", Output: &output})
			if err == nil {
				t.Fatal("Read() error = nil, want typed failure")
			}
			var typed *CLIError
			if !errors.As(err, &typed) || typed.Code != testCase.code {
				t.Fatalf("error = %v, want %s", err, testCase.code)
			}
			var payload map[string]string
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode error JSON: %v; output=%q", err, output.String())
			}
			if payload["code"] != testCase.code {
				t.Fatalf("error payload = %#v, want code %s", payload, testCase.code)
			}
		})
	}
}

func TestReadCancellationReturnsStableInterruptedError(t *testing.T) {
	protocol, err := clihttp.NewProtocol(readCancelDoer{}, testClock{})
	if err != nil {
		t.Fatalf("build canceled read protocol: %v", err)
	}
	var output bytes.Buffer
	err = NewRead(protocol)(ReadConfig{Context: context.Background(), Server: "http://factory.test:7437", Provider: "codex", Kind: "session_id", ID: "session-1", Output: &output})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_TRANSCRIPT_INTERRUPTED" || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want interrupted context cancellation", err)
	}
}

type readCancelDoer struct{}

func (readCancelDoer) Do(*http.Request) (*http.Response, error) { return nil, context.Canceled }

func boolPtrForTest(value bool) *bool { return &value }

func TestShowJSONUsesObservationDocumentAndExactIdentity(t *testing.T) {
	var gotPath string
	var gotQuery map[string]string
	started := time.Date(2026, 8, 8, 19, 0, 0, 0, time.UTC)
	ended := started.Add(2500 * time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = map[string]string{"provider": r.URL.Query().Get("provider"), "kind": r.URL.Query().Get("kind"), "id": r.URL.Query().Get("id")}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionObservation{
			WorkerSessionId: "worker-session-1", ProviderSessionAvailable: true,
			Model: stringPtrForTest("gpt-5.6-luna"), ReasoningEffort: stringPtrForTest("high"),
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
			WorkIds:         []string{"work-1"}, TurnId: stringPtrForTest("turn-1"), AttemptId: "attempt-2",
			State:     generated.WorkerSessionObservationStateCompleted,
			StartedAt: &started, EndedAt: &ended, DurationMillis: int64Ptr(2500),
			DurationBasis: generated.WorkerSessionObservationDurationBasisRECORDEDTIMESTAMPS,
			Transcript:    generated.WorkerSessionObservationTranscriptAVAILABLE,
			TokenUsage:    &generated.ProviderSessionTokenUsage{InputTokens: intPtr(4), OutputTokens: intPtr(8), TotalTokens: intPtr(12)},
			TurnUsage:     &generated.WorkerSessionTurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 450},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		Provider: "codex", Kind: "session_id", ID: "provider-session-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if gotPath != "/factory-sessions/session-1/worker-sessions/detail" || gotQuery["provider"] != "codex" || gotQuery["kind"] != "session_id" || gotQuery["id"] != "provider-session-1" {
		t.Fatalf("request = %s %#v, want exact show route and identity", gotPath, gotQuery)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if _, ok := document["sessions"]; ok {
		t.Fatalf("show JSON = %q, want one observation rather than list wrapper", output.String())
	}
	if string(document["workerSessionId"]) != `"worker-session-1"` || string(document["attemptId"]) != `"attempt-2"` {
		t.Fatalf("identity fields = %s/%s, want observation identity", document["workerSessionId"], document["attemptId"])
	}
	assertShowExecutionFacts(t, document)
	if !strings.Contains(string(document["tokenUsage"]), `"totalTokens":12`) || string(document["durationMillis"]) != "2500" {
		t.Fatalf("usage/duration = %s/%s, want token and timing projection", document["tokenUsage"], document["durationMillis"])
	}
	if got := string(document["turnUsage"]); got != `{"turnCount":3,"finalContextTokens":450,"peakContextTokens":450}` {
		t.Fatalf("turnUsage = %s, want derived values", got)
	}
	for _, key := range []string{"failure", "parse", "turnId", "startedAt", "endedAt"} {
		if _, ok := document[key]; !ok {
			t.Fatalf("show JSON missing stable field %q: %s", key, output.String())
		}
	}
}

func TestWorkerSessionConfirmationStateDefaultsUnknownValues(t *testing.T) {
	session := generated.WorkerSessionObservation{ConfirmationState: generated.ConfirmationState("LEGACY")}
	if got := workerSessionConfirmationState(session); got != generated.UNCONFIRMED {
		t.Fatalf("confirmationState = %q, want UNCONFIRMED", got)
	}
}

func TestWorkerSessionConfirmationStatePreservesConfirmed(t *testing.T) {
	session := generated.WorkerSessionObservation{ConfirmationState: generated.CONFIRMED}
	if got := workerSessionConfirmationState(session); got != generated.CONFIRMED {
		t.Fatalf("confirmationState = %q, want CONFIRMED", got)
	}
}

func assertShowExecutionFacts(t *testing.T, document map[string]json.RawMessage) {
	t.Helper()
	if string(document["model"]) != `"gpt-5.6-luna"` || string(document["reasoningEffort"]) != `"high"` {
		t.Fatalf("execution facts = %s/%s, want gpt-5.6-luna/high", document["model"], document["reasoningEffort"])
	}
}

func TestShowByWorkerSessionIDUsesTopLevelIdentityRoute(t *testing.T) {
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionObservation{
			WorkerSessionId: "direct-1", Direct: true, ProviderSessionAvailable: false,
			AttemptId: "attempt-1", State: generated.WorkerSessionObservationStateRunning,
			DurationBasis: generated.WorkerSessionObservationDurationBasisACTIVECLOCK,
			Transcript:    generated.WorkerSessionObservationTranscriptUNAVAILABLE,
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "direct-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if gotPath != "/worker-sessions/direct-1" || gotQuery != "" {
		t.Fatalf("request = path=%q query=%q, want top-level identity path without provider tuple", gotPath, gotQuery)
	}
	if !strings.Contains(output.String(), `"direct":true`) {
		t.Fatalf("output = %q, want direct origin", output.String())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	for _, key := range []string{"model", "reasoningEffort"} {
		if got, ok := document[key]; !ok || string(got) != "null" {
			t.Fatalf("legacy show %s = %s, want explicit null", key, got)
		}
	}
	if _, ok := document["turnUsage"]; ok {
		t.Fatalf("show turnUsage = %s, want field omitted without supported evidence", document["turnUsage"])
	}
}

func TestShowRejectsMixedIdentityModes(t *testing.T) {
	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context: context.Background(), Server: "http://127.0.0.1:1", WorkerSessionID: "direct-1",
		Provider: "codex", Kind: "session_id", ID: "provider-1", OutputFormat: "json", Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_MODE_CONFLICT" {
		t.Fatalf("error = %v, want identity mode conflict", err)
	}
}

func TestShowHumanRendersFailureAndParseDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionObservation{
			WorkerSessionId: "worker-session-1", ProviderSessionAvailable: true,
			Model: stringPtrForTest("gpt-5.6-luna"), ReasoningEffort: stringPtrForTest("high"),
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
			WorkIds:         []string{"work-1"}, AttemptId: "attempt-1", State: generated.WorkerSessionObservationStateFailed,
			DurationMillis: int64Ptr(2500), DurationBasis: generated.WorkerSessionObservationDurationBasisRECORDEDTIMESTAMPS,
			Transcript: generated.WorkerSessionObservationTranscriptAVAILABLE,
			TokenUsage: &generated.ProviderSessionTokenUsage{TotalTokens: intPtr(17)},
			TurnUsage:  &generated.WorkerSessionTurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 450},
			Failure:    &generated.WorkerSessionFailure{Kind: "WORKERS_EXECUTION_FAILURE", Detail: "safe failure detail", ProviderFailureKind: stringPtrForTest("dependency"), AgentRunFailureClass: stringPtrForTest("agent_run_provider_failure")},
			Parse:      generated.WorkerSessionParseDiagnostics{EventCount: 3, Errors: []generated.WorkerSessionParseDiagnostic{{Code: "provider_session_parse_error", LineNumber: 2, Message: "malformed event"}}},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "provider-session-1", Output: &output})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	for _, want := range []string{
		"Worker Session ID:\tworker-session-1", "Model:\tgpt-5.6-luna", "Reasoning Effort:\thigh", "Work IDs:\twork-1", "State:\tFAILED", "Duration:\t2.5s",
		"Token usage:\tinput=- cached-input=- cache-write=- output=- reasoning=- total=17",
		"Turn usage:\tcount=3 final-context=450 peak-context=450",
		"Failure:\tkind=WORKERS_EXECUTION_FAILURE detail=safe failure detail provider-kind=dependency", "agent-run-class=agent_run_provider_failure",
		"Parse diagnostics:\tevents=3 malformed=0 unknown=0 errors=1",
		"Parse error 1:\tcode=provider_session_parse_error line=2 message=malformed event",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output %q missing %q", output.String(), want)
		}
	}
}

func TestShowHumanRendersAbsentExecutionFactsAsDashes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionObservation{
			WorkerSessionId: "legacy-session", AttemptId: "attempt-legacy",
			State: generated.WorkerSessionObservationStateCompleted,
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "legacy-session", Output: &output,
	}); err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	for _, want := range []string{"Model:\t-", "Reasoning Effort:\t-"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q missing %q", output.String(), want)
		}
	}
}

// TestShowHumanNamesTheOperatorCancelTerminalReason is the operator-visible
// half of the diagnosis path: before the named reason existed this line read
// "Failure:\tunavailable" for a cancellation, which is indistinguishable from
// a failure whose cause was never recorded.
func TestShowHumanNamesTheOperatorCancelTerminalReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.WorkerSessionObservation{
			WorkerSessionId: "worker-session-canceled", WorkIds: []string{"work-1"}, AttemptId: "attempt-1",
			State:         generated.WorkerSessionObservationStateCanceled,
			DurationBasis: generated.WorkerSessionObservationDurationBasisRECORDEDTIMESTAMPS,
			Transcript:    generated.WorkerSessionObservationTranscriptUNAVAILABLE,
			Failure: &generated.WorkerSessionFailure{
				Kind:   "OPERATOR_CANCELED",
				Detail: "an operator cancel control ended the Worker Session",
			},
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "worker-session-canceled", Output: &output,
	})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	for _, want := range []string{
		"State:\tCANCELED",
		"Failure:\tkind=OPERATOR_CANCELED detail=an operator cancel control ended the Worker Session",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output %q missing %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "Failure:\tunavailable") {
		t.Fatalf("human output %q still reports an unnamed terminal reason", output.String())
	}
}

func TestShowMapsMissingSessionToStableJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(generated.ErrorResponse{Message: "worker session observation not found", Code: generated.ErrorResponseCodeNOTFOUND, Family: generated.ErrorFamilyNotFound})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "missing", OutputFormat: "json", Output: &output})
	if err == nil {
		t.Fatal("Show() error = nil, want missing-session error")
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_NOT_FOUND" {
		t.Fatalf("error = %v, want WORKER_SESSION_NOT_FOUND", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("decode error JSON: %v; output=%q", err, output.String())
	}
	if payload["code"] != "WORKER_SESSION_NOT_FOUND" || payload["message"] == "" {
		t.Fatalf("error payload = %#v, want stable missing-session code", payload)
	}
}

func TestShowRejectsUnsupportedIdentityBeforeHTTP(t *testing.T) {
	var output bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(), Server: "http://127.0.0.1:1", Provider: "other", Kind: "session_id", ID: "session-1", OutputFormat: "json", Output: &output})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "PROVIDER_UNSUPPORTED" {
		t.Fatalf("error = %v, want PROVIDER_UNSUPPORTED", err)
	}
	var payload map[string]string
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode error JSON: %v; output=%q", decodeErr, output.String())
	}
	if payload["code"] != "PROVIDER_UNSUPPORTED" {
		t.Fatalf("error payload = %#v, want unsupported provider code", payload)
	}
}

func stringPtrForTest(value string) *string { return &value }
