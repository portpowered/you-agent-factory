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

	"github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
			WorkIds:         []string{"work-1"}, TurnId: stringPtrForTest("turn-1"), AttemptId: "attempt-2",
			State:     generated.WorkerSessionObservationStateCompleted,
			StartedAt: &started, EndedAt: &ended, DurationMillis: int64Ptr(2500),
			DurationBasis: generated.WorkerSessionObservationDurationBasisRECORDEDTIMESTAMPS,
			Transcript:    generated.WorkerSessionObservationTranscriptAVAILABLE,
			TokenUsage:    &generated.ProviderSessionTokenUsage{InputTokens: intPtr(4), OutputTokens: intPtr(8), TotalTokens: intPtr(12)},
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
	if !strings.Contains(string(document["tokenUsage"]), `"totalTokens":12`) || string(document["durationMillis"]) != "2500" {
		t.Fatalf("usage/duration = %s/%s, want token and timing projection", document["tokenUsage"], document["durationMillis"])
	}
	for _, key := range []string{"failure", "parse", "turnId", "startedAt", "endedAt"} {
		if _, ok := document[key]; !ok {
			t.Fatalf("show JSON missing stable field %q: %s", key, output.String())
		}
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
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
			WorkIds:         []string{"work-1"}, AttemptId: "attempt-1", State: generated.WorkerSessionObservationStateFailed,
			DurationMillis: int64Ptr(2500), DurationBasis: generated.WorkerSessionObservationDurationBasisRECORDEDTIMESTAMPS,
			Transcript: generated.WorkerSessionObservationTranscriptAVAILABLE,
			TokenUsage: &generated.ProviderSessionTokenUsage{TotalTokens: intPtr(17)},
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
		"Worker Session ID:\tworker-session-1", "Work IDs:\twork-1", "State:\tFAILED", "Duration:\t2.5s",
		"Token usage:\tinput=- cached-input=- cache-write=- output=- reasoning=- total=17",
		"Failure:\tkind=WORKERS_EXECUTION_FAILURE detail=safe failure detail provider-kind=dependency", "agent-run-class=agent_run_provider_failure",
		"Parse diagnostics:\tevents=3 malformed=0 unknown=0 errors=1",
		"Parse error 1:\tcode=provider_session_parse_error line=2 message=malformed event",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output %q missing %q", output.String(), want)
		}
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
