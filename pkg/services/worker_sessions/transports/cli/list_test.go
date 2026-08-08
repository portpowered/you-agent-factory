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
				ProviderSessionAvailable: true,
				ProviderSession:          &factoryapi.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
				WorkIds:                  []string{"work-1"}, State: factoryapi.WorkerSessionObservationState("COMPLETED"),
				DurationBasis:  factoryapi.WorkerSessionObservationDurationBasis("RECORDED_TIMESTAMPS"),
				Transcript:     factoryapi.WorkerSessionObservationTranscript("AVAILABLE"),
				DurationMillis: int64Ptr(2500),
				TokenUsage:     &factoryapi.ProviderSessionTokenUsage{TotalTokens: intPtr(17)},
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
	for _, key := range []string{"providerSession", "turnId", "startedAt", "endedAt", "durationMillis", "tokenUsage", "failure"} {
		if got, ok := document.Sessions[1][key]; !ok || string(got) != "null" {
			t.Fatalf("session 2 %s = %s, want explicit null", key, got)
		}
	}
	if got := string(document.Sessions[0]["durationMillis"]); got != "2500" {
		t.Fatalf("session 1 durationMillis = %s, want 2500", got)
	}
	if got := string(document.Sessions[0]["tokenUsage"]); !strings.Contains(got, `"totalTokens":17`) {
		t.Fatalf("session 1 tokenUsage = %s, want totalTokens 17", got)
	}
}

func TestListHumanRendersLabelsTokensDurationAndFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{
			{
				WorkerSessionId: "worker-session-1", AttemptId: "attempt-1",
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
		"PROVIDER\tKIND\tSESSION ID\tWORK ID\tATTEMPT\tSTATE\tTOKENS\tDURATION\tFAILURE",
		"codex\tsession_id\tprovider-session-1\twork-1\tattempt-1\tFAILED\t17\t2.5s\tWORKERS_EXECUTION_FAILURE",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q missing %q", output.String(), want)
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
