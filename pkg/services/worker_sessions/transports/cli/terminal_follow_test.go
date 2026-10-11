package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

type terminalProtocol struct {
	clihttp.Protocol
	execute func(*http.Request) (clihttp.Response, error)
}

func (p terminalProtocol) Execute(r *http.Request) (clihttp.Response, error) { return p.execute(r) }

func terminalSSE(id, delivery, kind, phase string, payload any) string {
	raw, _ := json.Marshal(payload)
	draft, _ := json.Marshal(map[string]any{"kind": kind, "phase": phase, "payload": json.RawMessage(raw)})
	frame, _ := json.Marshal(map[string]any{"workerSessionId": id, "delivery": delivery, "event": map[string]any{"payload": json.RawMessage(draft)}})
	return "data: " + string(frame) + "\n\n"
}
func terminalSessionSSE(id, delivery, state string) string {
	phase := state
	if state == "TERMINATED" {
		phase = "CANCELED"
	}
	return terminalSSE(id, delivery, "SESSION", phase, map[string]string{"status": state, "failureDetail": "safe control detail"})
}
func terminalMessageSSE(id, role, text string, partial bool) string {
	return terminalSSE(id, "EVENT", "MESSAGE", "COMPLETED", map[string]any{"role": role, "partial": partial, "contentBlocks": []any{map[string]string{"kind": "TEXT", "text": text}}})
}

func TestTerminalFollowPublishedSummaryAndStates(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"COMPLETED", "FAILED", "CANCELED", "TERMINATED"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			input := terminalMessageSSE("a", "user", "USER_PRIVATE", false) + terminalMessageSSE("a", "assistant", "Published\r\nresult", false) + terminalMessageSSE("a", "assistant", "PARTIAL_PRIVATE", true)
			input += terminalSSE("a", "EVENT", "REASONING", "COMPLETED", map[string]string{"summary": "REASONING_PRIVATE"})
			input += terminalSessionSSE("a", "TERMINAL_REPLAY", state)
			line, err := consumeTerminal(StreamConfig{Context: t.Context()}, "a", strings.NewReader(input))
			if err != nil || line.State != state {
				t.Fatalf("line=%+v err=%v", line, err)
			}
			expected := "safe control detail"
			if state == "COMPLETED" {
				expected = "Published\r\nresult"
			}
			if line.Result != expected {
				t.Fatalf("result=%q", line.Result)
			}
			for _, jsonOutput := range []bool{false, true} {
				var output bytes.Buffer
				if err := writeTerminalLine(StreamConfig{Output: &output, JSON: jsonOutput}, line); err != nil {
					t.Fatal(err)
				}
				if strings.Count(output.String(), "\n") != 1 || strings.Contains(output.String(), "PRIVATE") {
					t.Fatalf("physical output=%q", output.String())
				}
				if jsonOutput {
					var fields map[string]string
					if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil || len(fields) != 3 {
						t.Fatalf("NDJSON=%s %v", output.Bytes(), err)
					}
				}
			}
		})
	}
	if got := []rune(shortTerminalSummary(strings.Repeat("é", 300))); len(got) != terminalSummaryRunes+1 {
		t.Fatalf("bounded UTF-8=%d", len(got))
	}
}

func TestTerminalFollowRejectsFalseTerminalAndSourceFailure(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"eof":     terminalMessageSSE("a", "assistant", "partial", false),
		"run":     terminalSSE("a", "TERMINAL", "RUN", "COMPLETED", map[string]string{"status": "COMPLETED"}),
		"active":  terminalSSE("a", "TERMINAL", "SESSION", "STARTED", map[string]string{"status": "RUNNING"}),
		"phase":   terminalSSE("a", "TERMINAL", "SESSION", "COMPLETED", map[string]string{"status": "FAILED"}),
		"foreign": terminalSessionSSE("b", "TERMINAL", "COMPLETED"),
		"gap":     terminalSSE("a", "EVENT", "STREAM_GAP", "UPDATED", map[string]string{}),
		"source":  "data: {\"workerSessionId\":\"a\",\"delivery\":\"SOURCE_FAILURE\",\"errorCode\":\"CUSTOM_FAILURE\"}\n\n",
		"invalid": "data: {broken}\n\n",
		"missing": "data: {\"workerSessionId\":\"a\",\"delivery\":\"TERMINAL\"}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			line, err := consumeTerminal(StreamConfig{Context: t.Context()}, "a", strings.NewReader(input))
			var diagnostic *CLIError
			if !errors.As(err, &diagnostic) || line.State != "" {
				t.Fatalf("false result=%+v err=%v", line, err)
			}
		})
	}
}

type terminalLineWriter struct{ lines chan string }

func (w terminalLineWriter) Write(data []byte) (int, error) {
	w.lines <- string(data)
	return len(data), nil
}

//nolint:contextcheck // StreamConfig explicitly carries the derived observer context.
func TestTerminalFollowDeduplicatesFlushesAndJoinsObservers(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := make(chan string, 3)
	closed := make(chan string, 3)
	gates := map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{}), "c": make(chan struct{})}
	protocol := terminalProtocol{execute: func(r *http.Request) (clihttp.Response, error) {
		id := strings.Split(r.URL.Path, "/")[2]
		reader, writer := io.Pipe()
		go func() {
			opened <- id
			select {
			case <-gates[id]:
				_, _ = io.WriteString(writer, terminalSessionSSE(id, "TERMINAL", "COMPLETED"))
			case <-r.Context().Done():
			}
			_ = writer.Close()
			closed <- id
		}()
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}}, nil
	}}
	lines := make(chan string, 3)
	done := make(chan error, 1)
	go func() {
		done <- NewStream(protocol)(StreamConfig{Context: ctx, Server: "http://selected", Terminal: true, TerminalIDs: []string{" a ", "b", "a", "c", "b"}, Output: terminalLineWriter{lines}})
	}()
	for range 3 {
		<-opened
	}
	for _, id := range []string{"b", "a", "c"} {
		close(gates[id])
		select {
		case line := <-lines:
			if !strings.Contains(line, fmt.Sprintf("workerSessionId=%q", id)) {
				t.Fatal(line)
			}
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		}
		if id != "c" {
			select {
			case err := <-done:
				t.Fatalf("early exit %v", err)
			default:
			}
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for range 3 {
		<-closed
	}
	select {
	case id := <-opened:
		t.Fatalf("duplicate stream %s", id)
	default:
	}
}

//nolint:contextcheck // StreamConfig explicitly carries the derived observer context.
func TestTerminalFollowCancellationAndDiagnostics(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	opened := make(chan struct{})
	var closeOnce sync.Once
	protocol := terminalProtocol{execute: func(r *http.Request) (clihttp.Response, error) {
		reader, writer := io.Pipe()
		go func() {
			closeOnce.Do(func() { close(opened) })
			<-r.Context().Done()
			_ = writer.CloseWithError(r.Context().Err())
		}()
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}}, nil
	}}
	done := make(chan error, 1)
	go func() {
		done <- NewStream(protocol)(StreamConfig{Context: ctx, Server: "http://selected", Terminal: true, TerminalIDs: []string{"active"}, Output: io.Discard})
	}()
	<-opened
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	for _, ids := range [][]string{nil, {}, {" "}} {
		err := NewStream(protocol)(StreamConfig{Context: t.Context(), Server: "http://selected", Terminal: true, TerminalIDs: ids, Output: io.Discard})
		if cliErrorCode(err) != "WORKER_SESSION_FOLLOW_INVALID" {
			t.Fatalf("invalid=%v", err)
		}
	}
	missing := terminalProtocol{execute: func(*http.Request) (clihttp.Response, error) {
		return clihttp.Response{HTTP: &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}}, nil
	}}
	err = NewStream(missing)(StreamConfig{Context: t.Context(), Server: "http://selected", Terminal: true, TerminalIDs: []string{"missing"}, Output: io.Discard})
	if cliErrorCode(err) != "WORKER_SESSION_NOT_FOUND" || !strings.Contains(cliErrorMessage(err), "missing") || !strings.Contains(cliErrorMessage(err), "http://selected") {
		t.Fatalf("diagnostic=%v", err)
	}
}

type terminalBrokenWriter struct{}

func (terminalBrokenWriter) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestTerminalFollowFlushAndOutputFailure(t *testing.T) {
	t.Parallel()
	protocol := terminalProtocol{execute: func(*http.Request) (clihttp.Response, error) {
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(terminalSessionSSE("a", "TERMINAL", "COMPLETED")))}}, nil
	}}
	var underlying bytes.Buffer
	buffered := bufio.NewWriter(&underlying)
	config := StreamConfig{Context: t.Context(), Server: "http://selected", Terminal: true, TerminalIDs: []string{"a"}, Output: buffered}
	if err := NewStream(protocol)(config); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(underlying.String(), "workerSessionId=\"a\"") {
		t.Fatalf("unflushed result=%q", underlying.String())
	}
	for _, output := range []io.Writer{terminalBrokenWriter{}, bufio.NewWriter(terminalBrokenWriter{})} {
		config.Output = output
		err := NewStream(protocol)(config)
		if cliErrorCode(err) != "WORKER_SESSION_FOLLOW_OUTPUT_FAILED" || !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "http://selected") {
			t.Fatalf("output diagnostic=%v", err)
		}
	}
}
