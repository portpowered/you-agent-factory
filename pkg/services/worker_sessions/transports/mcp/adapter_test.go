package workersessionmcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	client "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

type requestDoer func(*http.Request) (*http.Response, error)

func TestSubagentActionValidationBeforeEffects(t *testing.T) {
	t.Parallel()
	adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid action reached HTTP")
		return nil, nil
	})
	run := func(context.Context, json.RawMessage) (json.RawMessage, error) {
		t.Fatal("invalid action reached RUN/configuration")
		return nil, nil
	}
	for _, input := range []string{
		`null`, `[]`, `{}`, `{"action":"UNKNOWN"}`, `{"action":"run"}`, `{"action":""}`, `{"action":null}`, `{"action":1}`, `{"action":true}`,
		`{"Action":"LIST"}`, `{"action":"LIST","action":"RUN"}`, `{"action":"LIST","\u0061ction":"RUN"}`,
		`{"action":"LIST","scope":"all","Scope":"factory"}`, `{"action":"LIST","prompt":"cross-action"}`,
		`{"action":"READ"}`, `{"action":"READ","workerSessionId":null}`, `{"action":"CONTROL","workerSessionId":"w"}`,
		`{"action":"LIST"} {}`, `{"action":"LIST","scope":"direct","scope":"factory"}`,
	} {
		if input == `{}` { // Omitted RUN belongs to the existing Factory decoder.
			continue
		}
		response, err := adapter.Subagent(context.Background(), json.RawMessage(input), run)
		if err != nil || !strings.Contains(string(response), `"code":"worker_session.invalid_request"`) {
			t.Fatalf("input=%s response=%s err=%v", input, response, err)
		}
	}
}

func TestSubagentDefaultAndExplicitRunPreserveArguments(t *testing.T) {
	t.Parallel()
	adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("RUN reached selected host")
		return nil, nil
	})
	for _, input := range []string{`{"prompt":"hello"}`, `{"action":"RUN","prompt":"hello"}`} {
		response, err := adapter.Subagent(context.Background(), json.RawMessage(input), func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			if string(raw) != `{"prompt":"hello"}` {
				t.Fatalf("RUN arguments=%s", raw)
			}
			return json.RawMessage(`{"result":{"text":"answer"}}`), nil
		})
		if err != nil || string(response) != `{"result":{"text":"answer"}}` {
			t.Fatalf("RUN result=%s err=%v", response, err)
		}
	}
}

func (do requestDoer) Do(request *http.Request) (*http.Response, error) { return do(request) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (body *trackedBody) Close() error { body.closed = true; return nil }

func workerAdapter(t *testing.T, do requestDoer) *Adapter {
	t.Helper()
	host, err := client.NewClient("http://selected-host:7437", client.WithHTTPClient(do))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(host)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func workerCall(t *testing.T, adapter *Adapter, name, input string) map[string]any {
	t.Helper()
	raw, err := adapter.Call(context.Background(), name, json.RawMessage(input))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestWorkerSessionValidationRejectsBeforeHTTP(t *testing.T) {
	t.Parallel()
	adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid input reached the host")
		return nil, nil
	})
	cases := map[string][]string{
		ActionList:                   {`null`, `[]`, `"text"`, `{`, `{} {}`, `{"unknown":1}`, `{"history":null}`, `{"history":""}`, `{"history":"recent"}`, `{"history":1}`, `{"scope":null}`, `{"scope":"archive"}`, `{"state":["unknown"]}`, `{"state":"RUNNING"}`, `{"limit":0}`, `{"limit":1.5}`, `{"limit":"1"}`, `{"nextToken":" "}`},
		ActionRead:                   {`{}`, `{"workerSessionId":" "}`, `{"workerSessionId":"w","view":"unknown"}`, `{"workerSessionId":"w","limit":1}`, `{"workerSessionId":"w","view":"transcript","limit":1}`, `{"workerSessionId":"w","view":"events","limit":0}`, `{"workerSessionId":"w","view":"events","limit":1001}`, `{"workerSessionId":"w","view":null}`, `{"workerSessionId":"w","nextToken":"opaque"}`, `{"workerSessionId":"w","view":"events","nextToken":"opaque"}`, `{"workerSessionId":"w","view":"transcript","nextToken":"opaque"}`, `{"workerSessionId":"w","view":"logs","nextToken":" "}`, `{"workerSessionId":"w","view":"logs","nextToken":null}`, `{"workerSessionId":"w","view":"logs","limit":1001}`, `{"workerSessionId":"w","view":"logs","extra":true}`},
		ActionControl:                {`{}`, `{"workerSessionId":"w","operation":"KILL"}`, `{"workerSessionId":"w","operation":"CANCEL","requestId":"r"}`, `{"workerSessionId":"w","operation":"TERMINATE","successorWorkerSessionId":"s"}`, `{"workerSessionId":"w","operation":"CANCEL","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","successorWorkerSessionId":"s"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT","successorWorkerSessionId":"s","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":" ","successorWorkerSessionId":"s","replacementMessage":"m"}`},
		"you.worker_session.unknown": {`{}`},
	}
	for tool, inputs := range cases {
		for _, input := range inputs {
			t.Run(tool+"/"+input, func(t *testing.T) {
				value := workerCall(t, adapter, tool, input)
				failure := value["error"].(map[string]any)
				if failure["code"] != "worker_session.invalid_request" || failure["retryable"] != false || value["result"] != nil {
					t.Fatalf("invalid input result: %v", value)
				}
			})
		}
	}
}

func TestWorkerSessionGeneratedRequestsAndResults(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, tool, input, method, path, body string
	}{
		{"list", ActionList, `{"scope":"factory","state":["RUNNING","PAUSED"],"limit":2,"nextToken":"opaque"}`, "GET", "/worker-sessions", `{"workerSessions":[],"nextToken":null}`},
		{"summary", ActionRead, `{"workerSessionId":"target"}`, "GET", "/worker-sessions/target", `{"workerSessionId":"target","state":"RUNNING"}`},
		{"cancel", ActionControl, `{"workerSessionId":"target","operation":"CANCEL"}`, "POST", "/worker-sessions/target/cancel", `{"workerSessionId":"target","outcome":"APPLIED"}`},
		{"terminate", ActionControl, `{"workerSessionId":"target","operation":"TERMINATE"}`, "POST", "/worker-sessions/target/terminate", `{"workerSessionId":"target","outcome":"NOOP"}`},
		{"interrupt", ActionControl, `{"workerSessionId":"target","operation":"INTERRUPT","requestId":"request","successorWorkerSessionId":"successor","replacementMessage":"replacement"}`, "POST", "/worker-sessions/target/interrupt", `{"sourceWorkerSessionId":"target","successorWorkerSessionId":"successor","outcome":"APPLIED"}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			calls := 0
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Host != "selected-host:7437" || request.URL.Path != test.path || request.Method != test.method {
					t.Fatalf("request = %s %s", request.Method, request.URL)
				}
				assertWorkerRequestInput(t, request, test.name)
				return &http.Response{StatusCode: 200, Body: body}, nil
			})
			value := workerCall(t, adapter, test.tool, test.input)
			result := value["result"].(map[string]any)
			if test.name == "summary" {
				if len(result) != 1 {
					t.Fatalf("summary has extra members: %v", result)
				}
				result = result["session"].(map[string]any)
			}
			want := map[string]any{}
			if err := json.Unmarshal([]byte(test.body), &want); err != nil {
				t.Fatal(err)
			}
			actualJSON, _ := json.Marshal(result)
			wantJSON, _ := json.Marshal(want)
			if string(actualJSON) != string(wantJSON) || calls != 1 || !body.closed {
				t.Fatalf("result = %s, want %s; calls=%d closed=%v", actualJSON, wantJSON, calls, body.closed)
			}
		})
	}
}

func TestWorkerSessionLogsPageForwardingAndCleanup(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, input, limit, token string
		status                    int
		code                      string
	}{
		{"default", `{"workerSessionId":"target","view":"logs"}`, "100", "", 200, ""},
		{"cursor", `{"workerSessionId":"target","view":"logs","limit":2,"nextToken":"opaque+/="}`, "2", "opaque+/=", 200, ""},
		{"invalid cursor", `{"workerSessionId":"target","view":"logs","nextToken":"wrong"}`, "100", "wrong", 400, "invalid_request"},
		{"unavailable capture", `{"workerSessionId":"target","view":"logs"}`, "100", "", 503, "unavailable"},
		{"host outage", `{"workerSessionId":"target","view":"logs"}`, "100", "", 0, "host_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			const page = `{"workerSessionId":"target","events":[],"committedPosition":7,"health":"INCOMPLETE","nextToken":"stable"}`
			summary := &trackedBody{Reader: strings.NewReader(`{"workerSessionId":"target","state":"FAILED"}`)}
			logs := &trackedBody{Reader: strings.NewReader(page)}
			calls := 0
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: 200, Body: summary}, nil
				}
				if request.Method != "GET" || request.URL.Path != "/worker-sessions/target/logs" || request.URL.Query().Get("limit") != test.limit || request.URL.Query().Get("nextToken") != test.token {
					t.Fatalf("logs request: %s %s", request.Method, request.URL)
				}
				response := &http.Response{StatusCode: test.status, Body: logs}
				if test.status == 0 {
					return response, errors.New("private transport details")
				}
				return response, nil
			})
			value := workerCall(t, adapter, ActionRead, test.input)
			if test.code == "" {
				assertLogsViewResult(t, value, page)
			} else if value["error"].(map[string]any)["code"] != "worker_session."+test.code || value["result"] != nil {
				t.Fatalf("logs failure: %v", value)
			}
			if calls != 2 || !summary.closed || !logs.closed {
				t.Fatalf("request/body ownership: calls=%d summary=%v logs=%v", calls, summary.closed, logs.closed)
			}
		})
	}
}

func assertLogsViewResult(t *testing.T, value map[string]any, page string) {
	t.Helper()
	result := value["result"].(map[string]any)
	if len(result) != 2 || result["session"].(map[string]any)["state"] != "FAILED" {
		t.Fatalf("logs view members: %v", value)
	}
	got, _ := json.Marshal(result["logs"])
	var want map[string]any
	_ = json.Unmarshal([]byte(page), &want)
	expected, _ := json.Marshal(want)
	if string(got) != string(expected) {
		t.Fatalf("page altered: %s", got)
	}
}

func TestWorkerSessionReplayIsBoundedAndCloses(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			t.Parallel()
			var frames strings.Builder
			for i := 0; i < count; i++ {
				frame, _ := json.Marshal(map[string]any{"sequence": i, "type": "capture_health"})
				frames.WriteString(": keepalive\nevent: observation\ndata: " + string(frame) + "\n\n")
			}
			body := &trackedBody{Reader: strings.NewReader(frames.String())}
			calls := 0
			var replayContext context.Context
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if request.URL.Path != "/worker-sessions/target" {
						t.Fatal("observation must precede replay")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"workerSessionId":"target"}`))}, nil
				}
				if request.URL.Path != "/worker-sessions/target/events" || request.URL.Query().Get("replayOnly") != "true" {
					t.Fatalf("replay request: %s", request.URL)
				}
				replayContext = request.Context()
				return &http.Response{StatusCode: 200, Body: body}, nil
			})
			value := workerCall(t, adapter, ActionRead, `{"workerSessionId":"target","view":"events","limit":2}`)
			result := value["result"].(map[string]any)
			page := result["events"].(map[string]any)
			events := page["events"].([]any)
			if len(result) != 2 || len(events) != min(count, 2) || page["truncated"] != (count > 2) {
				t.Fatalf("replay result=%v calls=%d closed=%v context=%v", value, calls, body.closed, replayContext.Err())
			}
			assertReplayCleanup(t, calls, body.closed, replayContext)
			for i, event := range events {
				if event.(map[string]any)["sequence"] != float64(i) || event.(map[string]any)["type"] != "capture_health" {
					t.Fatalf("event order or payload changed: %v", events)
				}
			}
		})
	}
}

func TestWorkerSessionErrorsAreTypedAndSafe(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		status int
		code   string
		retry  bool
	}{
		{400, "invalid_request", false}, {401, "permission_denied", false}, {403, "permission_denied", false},
		{404, "not_found", false}, {409, "conflict", false}, {500, "internal_error", false}, {503, "unavailable", true},
		{0, "host_unavailable", true},
	} {
		t.Run(test.code, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader(`{"code":"WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED","message":"secret-token","phase":"SUCCESSOR_ADMISSION","sourceWorkerSessionId":"target","successorWorkerSessionId":"secret-token"}`)}
			adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
				if test.status == 0 {
					return nil, errors.New("secret-token")
				}
				return &http.Response{StatusCode: test.status, Body: body}, nil
			})
			value := workerCall(t, adapter, ActionControl, `{"workerSessionId":"target","operation":"INTERRUPT","requestId":"request","successorWorkerSessionId":"successor","replacementMessage":"replacement"}`)
			failure := value["error"].(map[string]any)
			if failure["code"] != "worker_session."+test.code || failure["retryable"] != test.retry || failure["workerSessionId"] != "target" {
				t.Fatalf("error = %v", value)
			}
			raw, _ := json.Marshal(value)
			if strings.Contains(string(raw), "secret-token") || (test.status != 0 && !body.closed) {
				t.Fatalf("unsafe error or open body: %s", raw)
			}
			if test.status != 0 {
				partial := failure["details"].(map[string]any)
				if partial["phase"] != "SUCCESSOR_ADMISSION" || partial["sourceWorkerSessionId"] != "target" || partial["successorWorkerSessionId"] != "successor" {
					t.Fatalf("missing partial-failure identity: %v", partial)
				}
			}
		})
	}
}

func TestWorkerSessionTranscriptUnavailableErrors(t *testing.T) {
	t.Parallel()
	const unavailable = `{"code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE","message":"secret-token","workerSessionId":"foreign-secret"}`
	for _, test := range []struct {
		name, body, code, upstream string
		status                     int
		retryable                  bool
	}{
		{"owner lost", unavailable, "unavailable", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 500, false},
		{"projection unavailable", `{"code":"WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE"}`, "unavailable", "WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE", 500, false},
		{"invalid", unavailable, "invalid_request", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 400, false},
		{"unauthorized", unavailable, "permission_denied", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 401, false},
		{"forbidden", unavailable, "permission_denied", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 403, false},
		{"unknown ID", unavailable, "not_found", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 404, false},
		{"conflict", unavailable, "conflict", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 409, false},
		{"transient", unavailable, "unavailable", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 503, true},
		{"other server status", unavailable, "internal_error", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", 502, false},
		{"unknown code", `{"code":"secret-token","message":"secret-token"}`, "internal_error", "", 500, false},
		{"other safe code", `{"code":"WORKER_SESSION_NOT_FOUND"}`, "internal_error", "WORKER_SESSION_NOT_FOUND", 500, false},
		{"truncated", `{"code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"`, "internal_error", "", 500, false},
		{"trailing object", unavailable + `{}`, "internal_error", "", 500, false},
		{"trailing garbage", unavailable + `secret-token`, "internal_error", "", 500, false},
		{"duplicate code", `{"code":"secret-token","code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"}`, "internal_error", "", 500, false},
		{"escaped duplicate", `{"code":"secret-token","\u0063ode":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"}`, "internal_error", "", 500, false},
		{"case alias", `{"Code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"}`, "internal_error", "", 500, false},
		{"invalid code type", `{"code":123}`, "internal_error", "", 500, false},
		{"null body", `null`, "internal_error", "", 500, false},
		{"array body", `[]`, "internal_error", "", 500, false},
		{"empty body", ``, "internal_error", "", 500, false},
		{"oversized prefix", unavailable + strings.Repeat(" ", 1<<20), "internal_error", "", 500, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			failure := responseFailure(&http.Response{StatusCode: test.status, Body: body}, nil, "requested-worker")
			if failure.Code != "worker_session."+test.code || failure.Retryable != test.retryable || failure.WorkerSessionID != "requested-worker" || failure.Details["status"] != test.status {
				t.Fatalf("failure=%+v", failure)
			}
			if got, _ := failure.Details["upstreamCode"].(string); got != test.upstream {
				t.Fatalf("upstream=%q want=%q", got, test.upstream)
			}
			raw, _ := json.Marshal(failure)
			if strings.Contains(string(raw), "secret") || failure.Message != "Selected host rejected the Worker Session request" {
				t.Fatalf("unsafe failure=%s", raw)
			}
		})
	}
}

func TestWorkerSessionTranscriptUnavailableClosesBodies(t *testing.T) {
	t.Parallel()
	summary := &trackedBody{Reader: strings.NewReader(`{"workerSessionId":"target","state":"FAILED"}`)}
	transcript := &trackedBody{Reader: strings.NewReader(`{"code":"WORKER_SESSION_TRANSCRIPT_UNAVAILABLE","message":"secret-token"}`)}
	calls := 0
	adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path == "/worker-sessions/target" {
			return &http.Response{StatusCode: 200, Body: summary}, nil
		}
		if request.URL.Path != "/worker-sessions/target/transcript" {
			t.Fatalf("unexpected request=%s", request.URL)
		}
		return &http.Response{StatusCode: 500, Body: transcript}, nil
	})
	value := workerCall(t, adapter, ActionRead, `{"workerSessionId":"target","view":"transcript"}`)
	failure := value["error"].(map[string]any)
	if value["result"] != nil || failure["code"] != "worker_session.unavailable" || failure["retryable"] != false || failure["workerSessionId"] != "target" || calls != 2 || !summary.closed || !transcript.closed {
		t.Fatalf("result=%v calls=%d summary closed=%v transcript closed=%v", value, calls, summary.closed, transcript.closed)
	}
}

func TestWorkerSessionCancellationReachesSelectedHost(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	raw, err := adapter.Call(ctx, ActionList, json.RawMessage(`{}`))
	if err != nil || !strings.Contains(string(raw), `"worker_session.host_unavailable"`) {
		t.Fatalf("canceled call = %s, %v", raw, err)
	}
}

func assertWorkerRequestInput(t *testing.T, request *http.Request, name string) {
	t.Helper()
	if name == "list" {
		query := request.URL.Query()
		if query.Get("scope") != "factory" || query.Get("limit") != "2" || query.Get("nextToken") != "opaque" || !slices.Equal(query["state"], []string{"RUNNING", "PAUSED"}) {
			t.Fatalf("list query = %v", query)
		}
	}
	if name == "interrupt" {
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if len(input) != 3 || input["requestId"] != "request" || input["successorWorkerSessionId"] != "successor" || input["replacementMessage"] != "replacement" {
			t.Fatalf("interrupt body = %v", input)
		}
	}
}

func assertReplayCleanup(t *testing.T, calls int, closed bool, ctx context.Context) {
	t.Helper()
	if calls != 2 || !closed || ctx.Err() != context.Canceled {
		t.Fatalf("replay cleanup: calls=%d closed=%v context=%v", calls, closed, ctx.Err())
	}
}

func TestWorkerSessionHistorySelection(t *testing.T) {
	t.Parallel()
	for _, history := range []string{"", "active", "all", "archived"} {
		t.Run(history, func(t *testing.T) {
			t.Parallel()
			want := history
			if want == "" {
				want = "all"
			}
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				if request.URL.Query().Get("history") != want {
					t.Fatalf("history query = %v, want %s", request.URL.Query(), want)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"sessions":[]}`)), Header: make(http.Header)}, nil
			})
			input := `{}`
			if history != "" {
				input = `{"history":"` + history + `"}`
			}
			result := workerCall(t, adapter, ActionList, input)
			if result["error"] != nil {
				t.Fatalf("history result = %v", result)
			}
		})
	}
}

func TestWorkerSessionKillRejectsInvalidTupleBeforeHTTP(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		`{"workerSessionId":"w","operation":"KILL","requestId":"r"}`,
		`{"workerSessionId":"w","operation":"KILL","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":" ","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":" "}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":null}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":1}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a","successorWorkerSessionId":"s"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a","replacementMessage":"m"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a","force":true}`,
		`{"workerSessionId":"w","operation":"KILL","operation":"TERMINATE"}`,
		`{"workerSessionId":"w","operation":"KILL","Operation":"TERMINATE"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","requestId":"other","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"stale","expectedAttemptId":"a"}`,
		`{"workerSessionId":"unrelated","workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a","\u006fperation":"TERMINATE"}`,
		`{"WorkerSessionId":"w","operation":"KILL","requestId":"r","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","Operation":"KILL","requestId":"r","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","RequestId":"r","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":"r","ExpectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"KILL","requestId":null,"requestId":"r","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"CANCEL","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"TERMINATE","expectedAttemptId":"a"}`,
		`{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","successorWorkerSessionId":"s","replacementMessage":"m","expectedAttemptId":"a"}`,
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid kill tuple reached selected host")
				return nil, nil
			})
			value := workerCall(t, adapter, ActionControl, input)
			if value["result"] != nil || value["error"].(map[string]any)["code"] != "worker_session.invalid_request" {
				t.Fatalf("invalid tuple = %v", value)
			}
		})
	}
}

func TestWorkerSessionKillSelectedHostResults(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		status     int
		body, code string
		retryable  bool
	}{
		{"applied", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"APPLIED","state":"TERMINATED","dispatchId":"logical-dispatch"}`, "", false},
		{"unsupported", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"UNSUPPORTED","state":"RUNNING","dispatchId":"logical-dispatch"}`, "", false},
		{"noop", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"NOOP","state":"COMPLETED","dispatchId":"logical-dispatch"}`, "", false},
		{"old host", 200, `{"workerSessionId":"target","action":"TERMINATE","outcome":"APPLIED","state":"TERMINATED","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"wrong target", 200, `{"workerSessionId":"other","action":"TERMINATE","forced":true,"outcome":"APPLIED","state":"TERMINATED","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"false force", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":false,"outcome":"APPLIED","state":"TERMINATED","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"missing result", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true}`, "internal_error", false},
		{"applied without terminal join", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"APPLIED","state":"RUNNING","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"applied natural failure", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"APPLIED","state":"FAILED","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"noop active", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"NOOP","state":"RUNNING","dispatchId":"logical-dispatch"}`, "internal_error", false},
		{"noop natural failure", 200, `{"workerSessionId":"target","action":"TERMINATE","forced":true,"outcome":"NOOP","state":"FAILED","dispatchId":"logical-dispatch"}`, "", false},
		{"stale attempt", 409, `{"code":"WORKER_SESSION_CONTROL_CONFLICT"}`, "conflict", false},
		{"unconfirmed join", 503, `{"code":"WORKER_SESSION_CONTROL_FAILED","message":"private runner details"}`, "unavailable", true},
		{"host unavailable", 0, `{"message":"private connection details"}`, "host_unavailable", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			body := &trackedBody{Reader: strings.NewReader(test.body)}
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				calls++
				assertKillRequest(t, request)
				response := &http.Response{StatusCode: test.status, Body: body}
				if test.status == 0 {
					return response, errors.New("private transport details")
				}
				return response, nil
			})
			value := workerCall(t, adapter, ActionControl, `{"workerSessionId":"target","operation":"KILL","requestId":"request","expectedAttemptId":"physical-attempt"}`)
			assertKillResult(t, value, test.body, test.code, test.retryable)
			if calls != 1 || !body.closed {
				t.Fatalf("kill calls=%d body closed=%v", calls, body.closed)
			}
		})
	}
}

func assertKillRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Method != "POST" || request.URL.Host != "selected-host:7437" || request.URL.Path != "/worker-sessions/target/terminate" {
		t.Fatalf("kill request = %s %s", request.Method, request.URL)
	}
	var tuple map[string]any
	if err := json.NewDecoder(request.Body).Decode(&tuple); err != nil {
		t.Fatal(err)
	}
	if len(tuple) != 3 || tuple["force"] != true || tuple["requestId"] != "request" || tuple["expectedAttemptId"] != "physical-attempt" {
		t.Fatalf("kill tuple = %v", tuple)
	}
}

func assertKillResult(t *testing.T, value map[string]any, body, code string, retryable bool) {
	t.Helper()
	if code == "" {
		got, _ := json.Marshal(value["result"])
		if string(got) == "null" || value["error"] != nil {
			t.Fatalf("kill result = %v", value)
		}
		var expected any
		_ = json.Unmarshal([]byte(body), &expected)
		want, _ := json.Marshal(expected)
		if string(got) != string(want) {
			t.Fatalf("kill response changed: %s, want %s", got, want)
		}
	} else {
		failure, ok := value["error"].(map[string]any)
		if !ok || value["result"] != nil || failure["code"] != "worker_session."+code || failure["retryable"] != retryable {
			t.Fatalf("kill failure = %v", value)
		}
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "private") {
			t.Fatalf("private host diagnostics leaked: %s", raw)
		}
	}
}

func TestWorkerSessionInterruptPreservesUnsupportedProviderPreflight(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"PROVIDER_UNSUPPORTED", "UNSUPPORTED"} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			assertWorkerSessionUnsupportedPreflight(t, code)
		})
	}
}

func assertWorkerSessionUnsupportedPreflight(t *testing.T, code string) {
	t.Helper()
	body := &trackedBody{Reader: strings.NewReader(`{"code":"` + code + `","message":"private-provider-detail","phase":"VALIDATION","sourceWorkerSessionId":"target"}`)}
	calls := 0
	adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Method != http.MethodPost || request.URL.Path != "/worker-sessions/target/interrupt" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusConflict, Body: body}, nil
	})
	value := workerCall(t, adapter, ActionControl, `{"workerSessionId":"target","operation":"INTERRUPT","requestId":"request","successorWorkerSessionId":"successor","replacementMessage":"replacement"}`)
	failure := value["error"].(map[string]any)
	details := failure["details"].(map[string]any)
	if failure["code"] != "worker_session.conflict" || failure["retryable"] != false || details["upstreamCode"] != code || details["phase"] != "VALIDATION" || calls != 1 || !body.closed {
		t.Fatalf("unsupported preflight: %v calls=%d closed=%v", value, calls, body.closed)
	}
	raw, _ := json.Marshal(value)
	if strings.Contains(string(raw), "private-provider-detail") {
		t.Fatalf("unsafe upstream text: %s", raw)
	}
}

func TestWorkerSessionValidationResumeModeBeforeHTTP(t *testing.T) {
	t.Parallel()
	adapter := workerAdapter(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid mode reached HTTP")
		return nil, nil
	})
	prefix := `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","successorWorkerSessionId":"s","replacementMessage":"m",`
	for _, field := range []string{`"resumeMode":""`, `"resumeMode":" "`, `"resumeMode":"invalid"`, `"resumeMode":"Provider"`, `"resumeMode":null`, `"resumeMode":1`, `"resumeMode":true`, `"resumeMode":[]`, `"resumeMode":"provider","resumeMode":"recorded"`, `"ResumeMode":"recorded"`} {
		failure := workerCall(t, adapter, ActionControl, prefix+field+"}")["error"].(map[string]any)
		if failure["code"] != "worker_session.invalid_request" || failure["retryable"] != false {
			t.Fatalf("invalid mode: %v", failure)
		}
	}
	for _, op := range []string{"CANCEL", "TERMINATE", "KILL"} {
		failure := workerCall(t, adapter, ActionControl, `{"workerSessionId":"w","operation":"`+op+`","resumeMode":"recorded"}`)["error"].(map[string]any)
		if failure["code"] != "worker_session.invalid_request" || failure["retryable"] != false {
			t.Fatalf("inapplicable mode: %v", failure)
		}
	}
}

func TestWorkerSessionInterruptForwardsResumeMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"", "provider", "recorded"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if (mode == "" && body["resumeMode"] != nil) || (mode != "" && body["resumeMode"] != mode) || body["replacementMessage"] != "m" {
					t.Fatalf("forwarded body: %v", body)
				}
				return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"accepted":true}`))}, nil
			})
			input := `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","successorWorkerSessionId":"s","replacementMessage":"m"`
			if mode != "" {
				input += `,"resumeMode":"` + mode + `"`
			}
			if value := workerCall(t, adapter, ActionControl, input+"}"); value["error"] != nil {
				t.Fatalf("forwarding: %v", value)
			}
		})
	}
}
