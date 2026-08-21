package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListJSONUsesStableSessionDocumentAndNullOptionals(t *testing.T) {
	var gotPath string
	var gotWorkID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotWorkID = r.URL.Query().Get("workId")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{
			{
				WorkerSessionId: "worker-session-1", AttemptId: "attempt-1",
				Model: stringPtrForTest("gpt-5.6-luna"), ReasoningEffort: stringPtrForTest("high"),
				ProviderSessionAvailable: true,
				ProviderSession:          &factoryapi.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
				WorkIds:                  []string{"work-1"}, State: factoryapi.WorkerSessionObservationState("FAILED"),
				DurationBasis:  factoryapi.WorkerSessionObservationDurationBasis("RECORDED_TIMESTAMPS"),
				Transcript:     factoryapi.WorkerSessionObservationTranscript("AVAILABLE"),
				DurationMillis: int64Ptr(2500),
				TokenUsage:     &factoryapi.ProviderSessionTokenUsage{TotalTokens: intPtr(17)},
				TurnUsage:      &factoryapi.WorkerSessionTurnUsage{TurnCount: 3, FinalContextTokens: 450, PeakContextTokens: 450},
				Failure:        &factoryapi.WorkerSessionFailure{Kind: "INCOMPLETE_OUTPUT", Detail: "safe incomplete-output detail"},
			},
			{
				WorkerSessionId: "worker-session-2", AttemptId: "attempt-2",
				ProviderSessionAvailable: false, WorkIds: []string{"work-1"},
				State:         factoryapi.WorkerSessionObservationState("RUNNING"),
				DurationBasis: factoryapi.WorkerSessionObservationDurationBasis("ACTIVE_CLOCK"),
				Transcript:    factoryapi.WorkerSessionObservationTranscript("UNAVAILABLE"),
			},
		}})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		WorkID: "work-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotPath != "/factory-sessions/session-1/worker-sessions" || gotWorkID != "work-1" {
		t.Fatalf("request = %s?workId=%s, want session route and work-1", gotPath, gotWorkID)
	}
	if strings.Contains(output.String(), "PROVIDER\tKIND") {
		t.Fatalf("JSON output contains human table: %q", output.String())
	}
	var document struct {
		Sessions []map[string]json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	if len(document.Sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(document.Sessions))
	}
	for _, key := range []string{"providerSession", "model", "reasoningEffort", "turnId", "startedAt", "endedAt", "durationMillis", "tokenUsage", "failure"} {
		if got, ok := document.Sessions[1][key]; !ok || string(got) != "null" {
			t.Fatalf("session 2 %s = %s, want explicit null", key, got)
		}
	}
	if _, ok := document.Sessions[1]["turnUsage"]; ok {
		t.Fatalf("session 2 turnUsage = %s, want field omitted without supported evidence", document.Sessions[1]["turnUsage"])
	}
	if got := string(document.Sessions[0]["model"]); got != `"gpt-5.6-luna"` {
		t.Fatalf("session 1 model = %s, want gpt-5.6-luna", got)
	}
	if got := string(document.Sessions[0]["reasoningEffort"]); got != `"high"` {
		t.Fatalf("session 1 reasoningEffort = %s, want high", got)
	}
	if got := string(document.Sessions[0]["durationMillis"]); got != "2500" {
		t.Fatalf("session 1 durationMillis = %s, want 2500", got)
	}
	if got := string(document.Sessions[0]["tokenUsage"]); !strings.Contains(got, `"totalTokens":17`) {
		t.Fatalf("session 1 tokenUsage = %s, want totalTokens 17", got)
	}
	if got := string(document.Sessions[0]["turnUsage"]); got != `{"turnCount":3,"finalContextTokens":450,"peakContextTokens":450}` {
		t.Fatalf("session 1 turnUsage = %s, want derived values", got)
	}
	if got := string(document.Sessions[0]["failure"]); !strings.Contains(got, `"kind":"INCOMPLETE_OUTPUT"`) {
		t.Fatalf("session 1 failure = %s, want INCOMPLETE_OUTPUT", got)
	}
}

func TestListHumanRendersLabelsTokensDurationAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{
			{
				WorkerSessionId: "worker-session-1", AttemptId: "attempt-1",
				Model: stringPtrForTest("gpt-5.6-luna"), ReasoningEffort: stringPtrForTest("high"),
				ProviderSessionAvailable: true,
				ProviderSession:          &factoryapi.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
				WorkIds:                  []string{"work-1"}, State: factoryapi.WorkerSessionObservationState("FAILED"),
				DurationMillis: int64Ptr(2500), TokenUsage: &factoryapi.ProviderSessionTokenUsage{TotalTokens: intPtr(17)},
				Failure: &factoryapi.WorkerSessionFailure{Kind: "WORKERS_EXECUTION_FAILURE", Detail: "safe detail"},
			},
		}})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "work-1", Output: &output,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for _, want := range []string{
		"DIRECT\tPROVIDER\tKIND\tSESSION ID\tWORK ID\tATTEMPT\tSTATE\tTOKENS\tDURATION\tFAILURE\tMODEL\tREASONING EFFORT",
		"false\tcodex\tsession_id\tprovider-session-1\twork-1\tattempt-1\tFAILED\t17\t2.5s\tWORKERS_EXECUTION_FAILURE\tgpt-5.6-luna\thigh",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q missing %q", output.String(), want)
		}
	}
}

func TestListHumanRendersAbsentExecutionFactsAsDashes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{{
			WorkerSessionId: "legacy-session", AttemptId: "attempt-legacy", WorkIds: []string{"work-1"},
			State: factoryapi.WorkerSessionObservationState("COMPLETED"),
		}}})
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "work-1", Output: &output,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !strings.Contains(output.String(), "COMPLETED\t-\t-") {
		t.Fatalf("output %q missing absent model and reasoning-effort columns", output.String())
	}
}

func TestListPreservesDistinctBoundedFailureKindsInHumanAndJSON(t *testing.T) {
	kinds := []string{"REJECTED", "INCOMPLETE_OUTPUT", "WORKERS_EXECUTION_FAILURE"}
	observations := make([]factoryapi.WorkerSessionObservation, 0, len(kinds))
	for _, kind := range kinds {
		observations = append(observations, factoryapi.WorkerSessionObservation{
			WorkerSessionId: "worker-session-" + kind,
			AttemptId:       "attempt-" + kind,
			WorkIds:         []string{"work-1"},
			State:           factoryapi.WorkerSessionObservationState("FAILED"),
			Failure:         &factoryapi.WorkerSessionFailure{Kind: kind, Detail: "safe bounded detail"},
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: observations})
	}))
	defer server.Close()

	var humanOutput bytes.Buffer
	if err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "work-1", Output: &humanOutput,
	}); err != nil {
		t.Fatalf("human List() error = %v", err)
	}
	for _, kind := range kinds {
		if !strings.Contains(humanOutput.String(), kind) {
			t.Fatalf("human output %q missing failure kind %q", humanOutput.String(), kind)
		}
	}

	var jsonOutput bytes.Buffer
	if err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "work-1", OutputFormat: "json", Output: &jsonOutput,
	}); err != nil {
		t.Fatalf("JSON List() error = %v", err)
	}
	var document factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(jsonOutput.Bytes(), &document); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, jsonOutput.String())
	}
	if len(document.Sessions) != len(kinds) {
		t.Fatalf("JSON session count = %d, want %d", len(document.Sessions), len(kinds))
	}
	for index, kind := range kinds {
		if document.Sessions[index].Failure == nil || document.Sessions[index].Failure.Kind != kind {
			t.Fatalf("JSON session %d failure = %#v, want %q", index, document.Sessions[index].Failure, kind)
		}
	}
}

func TestListHumanRendersKnownWorkWithoutSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{}})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{Context: context.Background(), Server: server.URL, WorkID: "work-1", Output: &output})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := output.String(); got != "No worker sessions found.\n" {
		t.Fatalf("output = %q, want empty-state message", got)
	}
}

func TestListJSONMissingWorkReturnsStableMachineReadableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "work not found", Family: factoryapi.ErrorFamilyNotFound,
			Code: factoryapi.ErrorResponseCode("NOT_FOUND"),
		})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "missing", OutputFormat: "json", Output: &output,
	})
	if err == nil {
		t.Fatal("List() error = nil, want missing Work error")
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORK_NOT_FOUND" {
		t.Fatalf("error = %v, want CLIError code WORK_NOT_FOUND", err)
	}
	var payload map[string]string
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode error JSON: %v; output=%q", decodeErr, output.String())
	}
	if payload["code"] != "WORK_NOT_FOUND" || payload["message"] != "work not found" {
		t.Fatalf("error payload = %#v, want stable code/message", payload)
	}
}

func TestListWithoutWorkIDUsesTopLevelIdentityCollection(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{}})
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("List() error = %v, want top-level success", err)
	}
	if gotPath != "/worker-sessions" {
		t.Fatalf("request path = %q, want top-level Worker Sessions collection", gotPath)
	}
}

func TestListRejectsUnsupportedOutputFormat(t *testing.T) {
	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: "http://127.0.0.1:1", WorkID: "work-1", OutputFormat: "yaml", Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "OUTPUT_UNSUPPORTED" {
		t.Fatalf("error = %v, want OUTPUT_UNSUPPORTED", err)
	}
}

type testClock struct{}

func (testClock) Now() time.Time { return time.Unix(1, 0) }

func testHTTPProtocol(t *testing.T) clihttp.Protocol {
	t.Helper()
	protocol, err := clihttp.NewProtocol(&http.Client{}, testClock{})
	if err != nil {
		t.Fatalf("build HTTP protocol: %v", err)
	}
	return protocol
}

func intPtr(value int) *int       { return &value }
func int64Ptr(value int64) *int64 { return &value }
