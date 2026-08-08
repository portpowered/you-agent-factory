package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

func TestStreamJSONUsesSessionScopedSSEAndStopsAtTerminal(t *testing.T) {
	var gotPath string
	var gotQuery map[string]string
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = map[string]string{"provider": r.URL.Query().Get("provider"), "kind": r.URL.Query().Get("kind"), "id": r.URL.Query().Get("id")}
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":{\"provider\":\"codex\",\"kind\":\"session_id\",\"id\":\"provider-session-1\"},\"workIds\":[\"work-1\"],\"event\":{\"position\":1,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker_session.started\",\"payload\":{\"state\":\"RUNNING\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
			"data: {\"delivery\":\"TERMINAL\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":{\"provider\":\"codex\",\"kind\":\"session_id\",\"id\":\"provider-session-1\"},\"workIds\":[\"work-1\"],\"event\":{\"position\":2,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":2,\"sourceEventId\":\"event-2\",\"schemaId\":\"worker_session.completed\",\"payload\":{\"state\":\"COMPLETED\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
		)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		Provider: "codex", Kind: "session_id", ID: "provider-session-1", OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotPath != "/factory-sessions/session-1/worker-sessions/events" || gotQuery["provider"] != "codex" || gotQuery["kind"] != "session_id" || gotQuery["id"] != "provider-session-1" || gotAccept != "text/event-stream" {
		t.Fatalf("request = path=%s query=%#v accept=%q, want session-scoped SSE request", gotPath, gotQuery, gotAccept)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON stream lines = %d, output=%q, want retained and terminal frames", len(lines), output.String())
	}
	var first, terminal map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("decode first stream frame: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &terminal); err != nil {
		t.Fatalf("decode terminal stream frame: %v", err)
	}
	if string(first["delivery"]) != `"RECORD"` || string(terminal["delivery"]) != `"TERMINAL"` {
		t.Fatalf("deliveries = %s/%s, want RECORD/TERMINAL", first["delivery"], terminal["delivery"])
	}
	if !strings.Contains(string(first["event"]), `"position":1`) || !strings.Contains(string(first["event"]), `"state":"RUNNING"`) {
		t.Fatalf("first event = %s, want canonical position and payload", first["event"])
	}
}

func TestStreamHumanRendersExplicitSourceFailureAndReturnsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":{\"provider\":\"cursor\",\"kind\":\"session_id\",\"id\":\"cursor-session-1\"},\"workIds\":[],\"event\":{\"position\":4,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":4,\"sourceEventId\":\"event-4\",\"schemaId\":\"worker_session.output\",\"payload\":{\"text\":\"hello\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
			"data: {\"delivery\":\"SOURCE_FAILURE\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":{\"provider\":\"cursor\",\"kind\":\"session_id\",\"id\":\"cursor-session-1\"},\"workIds\":[],\"event\":null,\"errorCode\":\"WORKER_SESSION_STREAM_GAP\",\"errorMessage\":\"retained Worker Session event history is unavailable\"}\n\n",
		)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, Provider: "cursor", Kind: "session_id", ID: "cursor-session-1", Output: &output,
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want source failure")
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_GAP" {
		t.Fatalf("error = %v, want WORKER_SESSION_STREAM_GAP", err)
	}
	for _, want := range []string{"delivery=RECORD", "position=4", "payload={\"text\":\"hello\"}", "delivery=SOURCE_FAILURE", "errorCode=WORKER_SESSION_STREAM_GAP"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human output %q missing %q", output.String(), want)
		}
	}
}

func TestStreamReturnsClosedBeforeTerminalAsJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker_session\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker_session.started\",\"payload\":{}},\"errorCode\":null,\"errorMessage\":null}\n\n")
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "session-1", OutputFormat: "json", Output: &output,
	})
	if err == nil {
		t.Fatal("Stream() error = nil, want closed-before-terminal error")
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_CLOSED" {
		t.Fatalf("error = %v, want WORKER_SESSION_STREAM_CLOSED", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("output = %q, want only the complete event frame after an opened stream", output.String())
	}
	var frame map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &frame); err != nil || string(frame["delivery"]) != `"RECORD"` {
		t.Fatalf("frame = %#v, decode error=%v, want RECORD", frame, err)
	}
}

func TestStreamPropagatesContextCancellation(t *testing.T) {
	protocol, err := clihttp.NewProtocol(streamCancelDoer{}, testClock{})
	if err != nil {
		t.Fatalf("build canceled stream protocol: %v", err)
	}
	err = NewStream(protocol)(StreamConfig{
		Context: context.Background(), Server: "http://factory.test:7437", Provider: "codex", Kind: "session_id", ID: "session-1", Output: &bytes.Buffer{},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_INTERRUPTED" {
		t.Fatalf("error = %v, want interrupted stream code", err)
	}
}

type streamCancelDoer struct{}

func (streamCancelDoer) Do(*http.Request) (*http.Response, error) { return nil, context.Canceled }
