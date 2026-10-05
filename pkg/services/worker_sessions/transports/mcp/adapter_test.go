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
		ToolList:                     {`null`, `[]`, `"text"`, `{`, `{} {}`, `{"unknown":1}`, `{"history":null}`, `{"history":""}`, `{"history":"recent"}`, `{"history":1}`, `{"scope":null}`, `{"scope":"archive"}`, `{"state":["unknown"]}`, `{"state":"RUNNING"}`, `{"limit":0}`, `{"limit":1.5}`, `{"limit":"1"}`, `{"nextToken":" "}`},
		ToolRead:                     {`{}`, `{"workerSessionId":" "}`, `{"workerSessionId":"w","view":"logs"}`, `{"workerSessionId":"w","limit":1}`, `{"workerSessionId":"w","view":"transcript","limit":1}`, `{"workerSessionId":"w","view":"events","limit":0}`, `{"workerSessionId":"w","view":"events","limit":1001}`, `{"workerSessionId":"w","view":null}`},
		ToolControl:                  {`{}`, `{"workerSessionId":"w","operation":"KILL"}`, `{"workerSessionId":"w","operation":"CANCEL","requestId":"r"}`, `{"workerSessionId":"w","operation":"TERMINATE","successorWorkerSessionId":"s"}`, `{"workerSessionId":"w","operation":"CANCEL","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","successorWorkerSessionId":"s"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":"r","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT","successorWorkerSessionId":"s","replacementMessage":"m"}`, `{"workerSessionId":"w","operation":"INTERRUPT","requestId":" ","successorWorkerSessionId":"s","replacementMessage":"m"}`},
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
		{"list", ToolList, `{"scope":"factory","state":["RUNNING","PAUSED"],"limit":2,"nextToken":"opaque"}`, "GET", "/worker-sessions", `{"workerSessions":[],"nextToken":null}`},
		{"summary", ToolRead, `{"workerSessionId":"target"}`, "GET", "/worker-sessions/target", `{"workerSessionId":"target","state":"RUNNING"}`},
		{"cancel", ToolControl, `{"workerSessionId":"target","operation":"CANCEL"}`, "POST", "/worker-sessions/target/cancel", `{"workerSessionId":"target","outcome":"APPLIED"}`},
		{"terminate", ToolControl, `{"workerSessionId":"target","operation":"TERMINATE"}`, "POST", "/worker-sessions/target/terminate", `{"workerSessionId":"target","outcome":"NOOP"}`},
		{"interrupt", ToolControl, `{"workerSessionId":"target","operation":"INTERRUPT","requestId":"request","successorWorkerSessionId":"successor","replacementMessage":"replacement"}`, "POST", "/worker-sessions/target/interrupt", `{"sourceWorkerSessionId":"target","successorWorkerSessionId":"successor","outcome":"APPLIED"}`},
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
			value := workerCall(t, adapter, ToolRead, `{"workerSessionId":"target","view":"events","limit":2}`)
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
			value := workerCall(t, adapter, ToolControl, `{"workerSessionId":"target","operation":"INTERRUPT","requestId":"request","successorWorkerSessionId":"successor","replacementMessage":"replacement"}`)
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

func TestWorkerSessionCancellationReachesSelectedHost(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := workerAdapter(t, func(request *http.Request) (*http.Response, error) {
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	raw, err := adapter.Call(ctx, ToolList, json.RawMessage(`{}`))
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
			result := workerCall(t, adapter, ToolList, input)
			if result["error"] != nil {
				t.Fatalf("history result = %v", result)
			}
		})
	}
}
