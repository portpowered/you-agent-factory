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
	"sync"
	"testing"
	"time"

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

func TestStreamByWorkerSessionIDUsesTopLevelIdentityRoute(t *testing.T) {
	var gotPath string
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"delivery\":\"REPLAY_SUMMARY\",\"workerSessionId\":\"direct-1\",\"providerSession\":null,\"workIds\":[],\"event\":null,\"errorCode\":null,\"errorMessage\":null,\"replaySummary\":{\"kind\":\"replay-summary\",\"complete\":true,\"reason\":\"terminal\",\"eventsEmitted\":0}}\n\n")
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "direct-1", ReplayOnly: true,
		OutputFormat: "json", Output: &output,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotPath != "/worker-sessions/direct-1/events" || len(gotQuery) != 1 || len(gotQuery["replayOnly"]) != 1 || gotQuery["replayOnly"][0] != "true" {
		t.Fatalf("request = path=%q query=%#v, want identity-only SSE route", gotPath, gotQuery)
	}
	if !strings.Contains(output.String(), `"kind":"replay-summary"`) {
		t.Fatalf("output = %q, want replay summary", output.String())
	}
}

func TestStreamReplayOnlyWritesEventFramesAndFinalSummary(t *testing.T) {
	var gotReplayOnly string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReplayOnly = r.URL.Query().Get("replayOnly")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker.output\",\"payload\":{\"text\":\"partial\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
			"data: {\"delivery\":\"REPLAY_SUMMARY\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":null,\"errorCode\":null,\"errorMessage\":null,\"replaySummary\":{\"kind\":\"replay-summary\",\"complete\":false,\"reason\":\"session-active\",\"eventsEmitted\":1}}\n\n",
		)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		Provider: "codex", Kind: "session_id", ID: "provider-session-1", OutputFormat: "json", ReplayOnly: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if gotReplayOnly != "true" {
		t.Fatalf("replayOnly query = %q, want true", gotReplayOnly)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON replay lines = %d, output=%q, want event and summary", len(lines), output.String())
	}
	var event map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("decode event line: %v", err)
	}
	if string(event["delivery"]) != `"RECORD"` {
		t.Fatalf("event line = %s, want RECORD", lines[0])
	}
	var summary streamReplaySummary
	if err := json.Unmarshal([]byte(lines[1]), &summary); err != nil {
		t.Fatalf("decode summary line: %v", err)
	}
	if summary.Kind != "replay-summary" || summary.Complete || summary.Reason != "session-active" || summary.EventsEmitted != 1 {
		t.Fatalf("summary = %#v, want active count-one replay summary", summary)
	}
}

func TestStreamRejectsReplayOnlyFollowBeforeRequestOrOutput(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "session-1",
		OutputFormat: "json", Follow: true, ReplayOnly: true, Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != StreamModeConflictCode {
		t.Fatalf("error = %v, want %s", err, StreamModeConflictCode)
	}
	if requests != 0 {
		t.Fatalf("conflicting stream made %d HTTP requests, want 0", requests)
	}
	if output.Len() != 0 {
		t.Fatalf("conflicting stream wrote stdout %q, want empty", output.String())
	}
}

func TestStreamReplayOnlyWritesTerminalReplayBeforeCompleteSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":1,\"sourceType\":\"worker\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":1,\"sourceEventId\":\"event-1\",\"schemaId\":\"worker.output\",\"payload\":{\"text\":\"complete\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
			"data: {\"delivery\":\"TERMINAL_REPLAY\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":{\"position\":2,\"sourceType\":\"worker_session_lifecycle\",\"sourceId\":\"worker-session-1\",\"sourceSequence\":2,\"sourceEventId\":\"terminal\",\"schemaId\":\"worker_session.lifecycle\",\"payload\":{\"status\":\"COMPLETED\"}},\"errorCode\":null,\"errorMessage\":null}\n\n",
			"data: {\"delivery\":\"REPLAY_SUMMARY\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":null,\"errorCode\":null,\"errorMessage\":null,\"replaySummary\":{\"kind\":\"replay-summary\",\"complete\":true,\"reason\":\"session-completed\",\"eventsEmitted\":2}}\n\n",
		)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, SessionID: "session-1",
		Provider: "codex", Kind: "session_id", ID: "provider-session-1", OutputFormat: "json", ReplayOnly: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("Stream() error = %v, want successful terminal replay completion", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("JSON terminal replay lines = %d, output=%q, want event, terminal, and summary", len(lines), output.String())
	}
	var terminal map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[1]), &terminal); err != nil {
		t.Fatalf("decode terminal line: %v", err)
	}
	if string(terminal["delivery"]) != `"TERMINAL_REPLAY"` {
		t.Fatalf("terminal line = %s, want TERMINAL_REPLAY", lines[1])
	}
	var summary streamReplaySummary
	if err := json.Unmarshal([]byte(lines[2]), &summary); err != nil {
		t.Fatalf("decode summary line: %v", err)
	}
	if summary.Kind != "replay-summary" || !summary.Complete || summary.Reason != "session-completed" || summary.EventsEmitted != 2 {
		t.Fatalf("summary = %#v, want complete completed-session count-two replay summary", summary)
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

func TestStreamReturnsMalformedFrameAsTypedFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {not-json}\n\n")
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "session-1", Output: &output,
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_FAILED" {
		t.Fatalf("error = %v, want typed WORKER_SESSION_STREAM_FAILED", err)
	}
	if output.Len() != 0 {
		t.Fatalf("malformed stream output = %q, want no frame", output.String())
	}
}

func TestStreamReturnsOutputWriteFailureAsTypedStreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"event\":null}\n\n")
	}))
	defer server.Close()

	writeErr := errors.New("broken stdout pipe")
	err := NewStream(testHTTPProtocol(t))(StreamConfig{
		Context: context.Background(), Server: server.URL, Provider: "codex", Kind: "session_id", ID: "session-1", Output: failingStreamWriter{err: writeErr},
	})
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_FAILED" {
		t.Fatalf("error = %v, want typed WORKER_SESSION_STREAM_FAILED", err)
	}
	if typed.Message != "failed to write Worker Session event stream" {
		t.Fatalf("typed write failure message = %q, want safe stream-write message", typed.Message)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("error = %v, want to preserve the output write cause", err)
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

func TestStreamMapsPostOpenContextCancellationToInterruptedAndClosesBody(t *testing.T) {
	body := &postOpenStreamBody{
		first:  []byte("data: {\"delivery\":\"RECORD\",\"workerSessionId\":\"worker-session-1\",\"providerSession\":null,\"workIds\":[],\"event\":null,\"errorCode\":null,\"errorMessage\":null}\n\n"),
		closed: make(chan struct{}),
	}
	protocol, err := clihttp.NewProtocol(postOpenStreamDoer{body: body}, testClock{})
	if err != nil {
		t.Fatalf("build post-open cancellation protocol: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := &streamSignalWriter{frame: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- NewStream(protocol)(StreamConfig{
			Context: ctx, Server: "http://factory.test:7437", Provider: "codex", Kind: "session_id", ID: "session-1", Output: output,
		})
	}()

	select {
	case <-output.frame:
	case <-time.After(time.Second):
		t.Fatal("stream did not render the complete pre-cancellation frame")
	}
	cancel()

	select {
	case <-body.closed:
	case <-time.After(time.Second):
		t.Fatal("canceling the request did not close the open response body")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stream error = %v, want context.Canceled", err)
		}
		var typed *CLIError
		if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_STREAM_INTERRUPTED" {
			t.Fatalf("stream error = %v, want WORKER_SESSION_STREAM_INTERRUPTED", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceling the request did not unblock the response read")
	}
}

type streamCancelDoer struct{}

func (streamCancelDoer) Do(*http.Request) (*http.Response, error) { return nil, context.Canceled }

type postOpenStreamDoer struct {
	body *postOpenStreamBody
}

func (d postOpenStreamDoer) Do(request *http.Request) (*http.Response, error) {
	go func() {
		<-request.Context().Done()
		_ = d.body.Close()
	}()
	return &http.Response{StatusCode: http.StatusOK, Body: d.body}, nil
}

type postOpenStreamBody struct {
	mu     sync.Mutex
	first  []byte
	closed chan struct{}
	once   sync.Once
}

func (b *postOpenStreamBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	if len(b.first) > 0 {
		n := copy(p, b.first)
		b.first = b.first[n:]
		b.mu.Unlock()
		return n, nil
	}
	b.mu.Unlock()
	<-b.closed
	return 0, errors.New("stream body closed")
}

func (b *postOpenStreamBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type streamSignalWriter struct {
	bytes.Buffer
	frame chan struct{}
	once  sync.Once
}

func (w *streamSignalWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(string(p), "delivery=RECORD") {
		w.once.Do(func() { close(w.frame) })
	}
	return n, err
}

type failingStreamWriter struct {
	err error
}

func (writer failingStreamWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
