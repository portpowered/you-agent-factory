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
	"github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-session-1"},
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
			ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "codex", Kind: "session_id", Id: "provider-1"},
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
			WorkerSessionId: "worker-session-1", ProviderSession: &generated.WorkerSessionProviderSessionRef{Provider: "cursor", Kind: "session_id", Id: "cursor-session-1"},
			WorkIds: []string{"work-1"}, AttemptId: "attempt-1", State: "FAILED",
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
		"Worker Session ID:\tworker-session-1", "Entries:\t2", "Entry 1:\ttype=tool_output order=1 role=tool", "output=tool result", "Entry 2:\ttype=reasoning order=2 role=reasoning-summary", "encrypted=true encryptedContent=ciphertext",
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
