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
