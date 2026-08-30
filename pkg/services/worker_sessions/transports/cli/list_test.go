package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
				WorkId:                   stringPtrForTest("work-1"), WorkName: stringPtrForTest("Build API"),
				WorkIds: []string{"work-1"}, State: factoryapi.WorkerSessionObservationState("FAILED"),
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
	assertStableSessionDocument(t, document.Sessions)
}

func assertStableSessionDocument(t *testing.T, sessions []map[string]json.RawMessage) {
	t.Helper()
	if len(sessions) != 2 {
		t.Fatalf("session count = %d, want 2", len(sessions))
	}
	for _, key := range []string{"providerSession", "model", "reasoningEffort", "turnId", "startedAt", "endedAt", "durationMillis", "tokenUsage", "failure", "factorySessionId", "recordingHealth", "recordingHealthReason", "workId", "workName"} {
		if got, ok := sessions[1][key]; !ok || string(got) != "null" {
			t.Fatalf("session 2 %s = %s, want explicit null", key, got)
		}
	}
	if got := string(sessions[1]["confirmationState"]); got != `"UNCONFIRMED"` {
		t.Fatalf("session 2 confirmationState = %s, want UNCONFIRMED", got)
	}
	if _, ok := sessions[1]["turnUsage"]; ok {
		t.Fatalf("session 2 turnUsage = %s, want field omitted without supported evidence", sessions[1]["turnUsage"])
	}
	if got := string(sessions[0]["model"]); got != `"gpt-5.6-luna"` {
		t.Fatalf("session 1 model = %s, want gpt-5.6-luna", got)
	}
	if got := string(sessions[0]["reasoningEffort"]); got != `"high"` {
		t.Fatalf("session 1 reasoningEffort = %s, want high", got)
	}
	if got := string(sessions[0]["durationMillis"]); got != "2500" {
		t.Fatalf("session 1 durationMillis = %s, want 2500", got)
	}
	if got := string(sessions[0]["tokenUsage"]); !strings.Contains(got, `"totalTokens":17`) {
		t.Fatalf("session 1 tokenUsage = %s, want totalTokens 17", got)
	}
	if got := string(sessions[0]["turnUsage"]); got != `{"turnCount":3,"finalContextTokens":450,"peakContextTokens":450}` {
		t.Fatalf("session 1 turnUsage = %s, want derived values", got)
	}
	if got := string(sessions[0]["failure"]); !strings.Contains(got, `"kind":"INCOMPLETE_OUTPUT"`) {
		t.Fatalf("session 1 failure = %s, want INCOMPLETE_OUTPUT", got)
	}
	if got := string(sessions[0]["workId"]); got != `"work-1"` {
		t.Fatalf("session 1 workId = %s, want work-1", got)
	}
	if got := string(sessions[0]["workName"]); got != `"Build API"` {
		t.Fatalf("session 1 workName = %s, want Build API", got)
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
				WorkId:                   stringPtrForTest("work-1"), WorkName: stringPtrForTest("Build API"),
				WorkIds: []string{"work-1"}, State: factoryapi.WorkerSessionObservationState("FAILED"),
				StartedAt:      timePtr(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
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
		"WORK NAME", "WORK ID", "WORKER SESSION ID", "PROVIDER", "KIND", "PROVIDER SESSION ID", "STATE", "STARTED", "DURATION", "EXIT/FAILURE KIND",
		"Build API", "work-1", "worker-session-1", "codex", "session_id", "provider-session-1", "FAILED", "2026-01-02T03:04:05Z", "2.5s", "WORKERS_EXECUTION_FAILURE",
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
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "legacy-session") || !strings.Contains(lines[1], "COMPLETED") || strings.Count(lines[1], "-") < 5 {
		t.Fatalf("output %q missing absent execution facts", output.String())
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

func TestListTopLevelSupportsRepeatedStatesAndPositiveLimit(t *testing.T) {
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{}})
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, Scope: "factory", States: []string{"RUNNING", "FAILED"},
		Limit: 2, LimitSet: true, MaxResults: 9, NextToken: "cursor-1", OutputFormat: "json", Output: &output,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := gotQuery["scope"]; len(got) != 1 || got[0] != "factory" {
		t.Fatalf("scope query = %#v, want factory", got)
	}
	if got := gotQuery["state"]; len(got) != 2 || got[0] != "RUNNING" || got[1] != "FAILED" {
		t.Fatalf("state query = %#v, want repeated RUNNING/FAILED", got)
	}
	if got := gotQuery["limit"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("limit query = %#v, want 2", got)
	}
	if _, ok := gotQuery["maxResults"]; ok {
		t.Fatalf("query includes legacy maxResults when limit is set: %#v", gotQuery)
	}
	if got := gotQuery["nextToken"]; len(got) != 1 || got[0] != "cursor-1" {
		t.Fatalf("nextToken query = %#v, want cursor-1", got)
	}
}

func TestListTopLevelIsBoundedAndResumesWithExplicitNextToken(t *testing.T) {
	server, requests := newBoundedWorkerSessionServer(t)
	defer server.Close()

	first := runBoundedWorkerSessionList(t, server.URL, "")
	assertBoundedWorkerSessionPage(t, first, "worker-session-1", "cursor-2")
	assertBoundedWorkerSessionRequest(t, (*requests)[0], "")

	second := runBoundedWorkerSessionList(t, server.URL, "cursor-2")
	assertBoundedWorkerSessionPage(t, second, "worker-session-2", "")
	assertBoundedWorkerSessionRequest(t, (*requests)[1], "cursor-2")
	if len(*requests) != 2 {
		t.Fatalf("explicit continuation made %d total requests, want two", len(*requests))
	}
}

func newBoundedWorkerSessionServer(t *testing.T) (*httptest.Server, *[]url.Values) {
	t.Helper()
	requests := []url.Values{}
	nextToken := "cursor-2"
	pages := map[string]factoryapi.ListWorkerSessionsResponse{
		"": {
			Sessions: []factoryapi.WorkerSessionObservation{{
				WorkerSessionId: "worker-session-1",
				AttemptId:       "attempt-1",
			}},
			PaginationContext: &factoryapi.PaginationContext{MaxResults: 1, NextToken: &nextToken},
		},
		nextToken: {
			Sessions: []factoryapi.WorkerSessionObservation{{
				WorkerSessionId: "worker-session-2",
				AttemptId:       "attempt-2",
			}},
			PaginationContext: &factoryapi.PaginationContext{MaxResults: 1},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		requests = append(requests, query)
		page, ok := pages[query.Get("nextToken")]
		if !ok {
			t.Fatalf("unexpected continuation token %q", query.Get("nextToken"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	return server, &requests
}

func runBoundedWorkerSessionList(t *testing.T, server string, nextToken string) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	var output bytes.Buffer
	if err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server,
		MaxResults: 1, MaxResultsSet: true, NextToken: nextToken, OutputFormat: "json", Output: &output,
	}); err != nil {
		t.Fatalf("bounded List() error = %v", err)
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON output: %v; output=%q", err, output.String())
	}
	return response
}

func assertBoundedWorkerSessionPage(t *testing.T, response factoryapi.ListWorkerSessionsResponse, wantID string, wantNextToken string) {
	t.Helper()
	if len(response.Sessions) != 1 || response.Sessions[0].WorkerSessionId != wantID {
		t.Fatalf("page = %#v, want %s", response.Sessions, wantID)
	}
	if response.PaginationContext == nil {
		t.Fatal("pagination context = nil")
	}
	if wantNextToken == "" {
		if response.PaginationContext.NextToken != nil {
			t.Fatalf("pagination context = %#v, want exhausted page", response.PaginationContext)
		}
		return
	}
	if response.PaginationContext.NextToken == nil || *response.PaginationContext.NextToken != wantNextToken {
		t.Fatalf("pagination context = %#v, want %s", response.PaginationContext, wantNextToken)
	}
}

func assertBoundedWorkerSessionRequest(t *testing.T, query url.Values, wantNextToken string) {
	t.Helper()
	if got := query.Get("maxResults"); got != "1" {
		t.Fatalf("maxResults = %q, want 1", got)
	}
	if got := query.Get("nextToken"); got != wantNextToken {
		t.Fatalf("nextToken = %q, want %q", got, wantNextToken)
	}
}

func TestListRejectsTopLevelFiltersForWorkScopedRoute(t *testing.T) {
	cases := []struct {
		name   string
		config ListConfig
	}{
		{name: "state", config: ListConfig{States: []string{"RUNNING"}}},
		{name: "limit", config: ListConfig{Limit: 2, LimitSet: true}},
		{name: "legacy max results", config: ListConfig{MaxResults: 2}},
		{name: "explicit zero legacy max results", config: ListConfig{MaxResultsSet: true}},
		{name: "next token", config: ListConfig{NextToken: "cursor-1"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			config := testCase.config
			config.Context = context.Background()
			config.Server = "http://127.0.0.1:1"
			config.WorkID = "work-1"
			config.OutputFormat = "json"
			config.Output = &output
			err := NewList(testHTTPProtocol(t))(config)
			var typed *CLIError
			if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_SCOPED_FILTER_UNSUPPORTED" {
				t.Fatalf("error = %v, want WORKER_SESSION_SCOPED_FILTER_UNSUPPORTED", err)
			}
			var payload map[string]string
			if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
				t.Fatalf("decode error JSON: %v; output=%q", decodeErr, output.String())
			}
			if payload["code"] != typed.Code {
				t.Fatalf("error payload = %#v, want code %q", payload, typed.Code)
			}
		})
	}
}

func TestListRejectsNonPositiveLimit(t *testing.T) {
	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: "http://127.0.0.1:1", Limit: 0, LimitSet: true, OutputFormat: "json", Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "LIMIT_INVALID" {
		t.Fatalf("error = %v, want LIMIT_INVALID", err)
	}
	var payload map[string]string
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode error JSON: %v; output=%q", decodeErr, output.String())
	}
	if payload["code"] != "LIMIT_INVALID" {
		t.Fatalf("error payload = %#v, want LIMIT_INVALID", payload)
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

func TestListFailureDiagnosticsIdentifyTransportBodyAndDecodeStages(t *testing.T) {
	secret := "secret-response-payload"
	tests := []struct {
		name      string
		doer      clihttp.Doer
		wantStage string
	}{
		{
			name: "transport",
			doer: listDoerFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New(secret)
			}),
			wantStage: "transport",
		},
		{
			name: "body",
			doer: listDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(listFailingReader{err: errors.New(secret)})}, nil
			}),
			wantStage: "body",
		},
		{
			name: "decode",
			doer: listDoerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{"))}, nil
			}),
			wantStage: "decode",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			protocol, err := clihttp.NewProtocol(testCase.doer, testClock{})
			if err != nil {
				t.Fatalf("NewProtocol() error = %v", err)
			}
			var output bytes.Buffer
			var diagnostics bytes.Buffer
			err = NewList(protocol)(ListConfig{
				Context: context.Background(), Server: "http://factory.test", WorkID: "work-1",
				OutputFormat: "json", Output: &output, Diagnostics: &diagnostics, Verbose: true,
			})
			var typed *CLIError
			if !errors.As(err, &typed) || typed.Code != "FACTORY_UNREACHABLE" {
				t.Fatalf("error = %v, want FACTORY_UNREACHABLE", err)
			}
			if !strings.Contains(diagnostics.String(), "errorStage="+testCase.wantStage) {
				t.Fatalf("diagnostics = %q, want errorStage=%s", diagnostics.String(), testCase.wantStage)
			}
			if strings.Contains(diagnostics.String(), secret) || strings.Contains(output.String(), secret) {
				t.Fatalf("failure exposed response payload: diagnostics=%q output=%q", diagnostics.String(), output.String())
			}
			if strings.Contains(output.String(), `"sessions"`) {
				t.Fatalf("failure output contains a success collection: %q", output.String())
			}
		})
	}
}

func TestListOutputWriterFailureIsTypedAndDoesNotReportSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(factoryapi.ListWorkerSessionsResponse{Sessions: []factoryapi.WorkerSessionObservation{{
			WorkerSessionId: "worker-session-1", AttemptId: "attempt-1", WorkIds: []string{"work-1"},
		}}})
	}))
	defer server.Close()

	err := NewList(testHTTPProtocol(t))(ListConfig{
		Context: context.Background(), Server: server.URL, WorkID: "work-1", OutputFormat: "json",
		Output: listFailingWriter{err: errors.New("output sink unavailable")},
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_OUTPUT_FAILED" {
		t.Fatalf("error = %v, want WORKER_SESSION_OUTPUT_FAILED", err)
	}
}

func TestListCanceledContextIsTypedAndDoesNotReturnCollection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	protocol, err := clihttp.NewProtocol(listDoerFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	}), testClock{})
	if err != nil {
		t.Fatalf("NewProtocol() error = %v", err)
	}
	var output bytes.Buffer
	err = NewList(protocol)(ListConfig{
		Context: ctx, Server: "http://factory.test", WorkID: "work-1", OutputFormat: "json", Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "FACTORY_UNREACHABLE" || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want FACTORY_UNREACHABLE wrapping context.Canceled", err)
	}
	if strings.Contains(output.String(), `"sessions"`) {
		t.Fatalf("canceled output contains a success collection: %q", output.String())
	}
}

type listDoerFunc func(*http.Request) (*http.Response, error)

func (f listDoerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

type listFailingReader struct{ err error }

func (reader listFailingReader) Read([]byte) (int, error) { return 0, reader.err }

type listFailingWriter struct{ err error }

func (writer listFailingWriter) Write([]byte) (int, error) { return 0, writer.err }

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

func intPtr(value int) *int              { return &value }
func int64Ptr(value int64) *int64        { return &value }
func timePtr(value time.Time) *time.Time { return &value }
