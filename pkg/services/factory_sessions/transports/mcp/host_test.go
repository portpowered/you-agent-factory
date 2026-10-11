package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type runHostTransport func(*http.Request) (*http.Response, error)

func (transport runHostTransport) Do(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func runHostResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func newRunHost(t *testing.T, transport runHostTransport) *hostSessions {
	t.Helper()
	return &hostSessions{server: "http://selected.invalid", http: transport}
}

func TestSubagentHostCallerAndOwnerResult(t *testing.T) {
	t.Parallel()
	const token = "planted-private-token"
	var paths []string
	host := newRunHost(t, func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path)
		assertRunHostCallerRequest(t, request, token)
		body, _ := io.ReadAll(request.Body)
		if strings.Contains(string(body), token) || strings.Contains(string(body), "caller") {
			t.Fatal("execution credentials entered body")
		}
		var document map[string]any
		if json.Unmarshal(body, &document) != nil {
			t.Fatal("invalid request JSON")
		}
		if request.URL.Path == "/factory-sessions" {
			if document["factoryId"] != "@you/subagent" || document["requestId"] != "request" || document["folderPath"] != "/root" {
				t.Fatal("live selection changed")
			}
			return runHostResponse(200, `{"session":{"id":"child","isDefault":false}}`), nil
		}
		return runHostResponse(200, `{"sessionId":"child","requestId":"request","traceId":"trace","status":"FAILED","failureReason":"auth_failure","message":""}`), nil
	})
	caller := &workersessions.CallerIdentity{WorkerSessionID: "caller", Token: token}
	started, err := host.Start(t.Context(), factorysessions.SessionStartRequest{Caller: caller, FolderPath: "/root", Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"}})
	if err != nil || started.SessionID != "child" {
		t.Fatalf("start failed: %v", err)
	}
	result, err := host.Invoke(t.Context(), factorysessions.SessionInvokeRequest{Caller: caller, SessionID: "child", Correlation: factorysessions.SessionOperationCorrelation{RequestID: "request"}, Args: map[string]any{"input": "prompt"}, Wait: factorysessions.SessionOperationWait{TimeoutMillis: 1000}})
	assertRunHostOwnerResult(t, result, err)
	if !reflect.DeepEqual(paths, []string{"/factory-sessions", "/factory-sessions/child/invocations"}) {
		t.Fatal("live routes changed")
	}
}

func assertRunHostOwnerResult(t *testing.T, result factorysessions.InvocationResult, err error) {
	t.Helper()
	if err != nil || result.FailureReason != "auth_failure" || result.Message != "" || result.TraceID != "trace" || result.Status != factorysessions.InvocationTerminalStatusFailed {
		t.Fatalf("owner result changed: %#v %v", result, err)
	}
}

func assertRunHostCallerRequest(t *testing.T, request *http.Request, token string) {
	t.Helper()
	if request.URL.Host != "selected.invalid" || request.Header.Get("X-You-Worker-Session-Id") != "caller" || request.Header.Get("Authorization") != "Bearer "+token {
		t.Fatal("request changed selected host or caller credentials")
	}
}

func TestSubagentHostCallerRefusalPrivacy(t *testing.T) {
	t.Parallel()
	for _, transportFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "typed", true: "transport"}[transportFailure], func(t *testing.T) {
			t.Parallel()
			const token = "planted-secret"
			host := newRunHost(t, func(*http.Request) (*http.Response, error) {
				if transportFailure {
					return nil, errors.New(token)
				}
				return runHostResponse(403, `{"code":"WORKER_SESSION_CALLER_INVALID","message":"`+token+`"}`), nil
			})
			_, err := host.Start(t.Context(), factorysessions.SessionStartRequest{Caller: &workersessions.CallerIdentity{WorkerSessionID: "caller", Token: token}})
			if err == nil || strings.Contains(err.Error(), token) || (!transportFailure && !errors.Is(err, workersessions.ErrCallerInvalid)) {
				t.Fatal("refusal lost privacy or type")
			}
		})
	}
}

func TestSubagentHostCloseFinalizesExactChild(t *testing.T) {
	t.Parallel()
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "terminal"}[terminal], func(t *testing.T) {
			t.Parallel()
			var calls []string
			host := newRunHost(t, func(request *http.Request) (*http.Response, error) {
				if request.Context().Err() != nil || request.URL.Host != "selected.invalid" || request.Header.Get("Authorization") != "" {
					t.Fatal("cleanup lost context, host or privacy")
				}
				calls = append(calls, request.Method+" "+request.URL.Path)
				switch request.Method {
				case http.MethodPost:
					if terminal {
						return runHostResponse(409, `{"sessionId":"child","outcome":"TERMINAL_SESSION"}`), nil
					}
					return runHostResponse(200, `{"sessionId":"child","outcome":"ACCEPTED"}`), nil
				case http.MethodDelete:
					return runHostResponse(204, ""), nil
				default:
					return runHostResponse(200, `{"id":"child","isDefault":false,"runtime":{"status":"FINISHED"}}`), nil
				}
			})
			result, err := host.Control(t.Context(), runHostCloseRequest("child"))
			if err != nil || !result.Closed {
				t.Fatalf("close failed: %v", err)
			}
			if !reflect.DeepEqual(calls, []string{"GET /factory-sessions/child", "POST /factory-sessions/child/terminate", "GET /factory-sessions/child", "DELETE /factory-sessions/child"}) {
				t.Fatalf("cleanup route/order changed: %v", calls)
			}
		})
	}
}

func runHostCloseRequest(id string) factorysessions.SessionControlRequest {
	return factorysessions.SessionControlRequest{SessionID: id, Mode: factorysessions.SessionOperationModeLive, Operation: factorysessions.SessionControlClose}
}

func TestSubagentHostCloseRefusalsAndMissing(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"default", "foreign", "missing", "delete-failure", "terminal-foreign", "deadline"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			deletes, terminates := 0, 0
			host := newRunHost(t, func(request *http.Request) (*http.Response, error) {
				return runHostCloseRefusalResponse(variant, request, &deletes, &terminates, cancel)
			})
			result, err := host.Control(ctx, runHostCloseRequest("child"))
			assertRunHostCloseRefusal(t, variant, result, err, deletes, terminates)
		})
	}
}

func TestSubagentHostResponsePrefix(t *testing.T) {
	t.Parallel()
	host := newRunHost(t, func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "selected.invalid" || request.URL.Path != "/factory-sessions/child/response-events" {
			t.Fatal("snapshot escaped exact child")
		}
		response := runHostResponse(200, "data: {\"factorySessionId\":\"child\",\"kind\":\"ERROR\",\"phase\":\"FAILED\"}\n\n")
		response.Header.Set(factorysessions.ResponseEventStreamRetainedCountHeader, "1")
		return response, nil
	})
	cursor, err := host.SubscribeFactoryResponseEvents(t.Context(), factorysessions.ResponseEventSubscriptionRequest{SessionID: "child"})
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Detach()
	events, err := cursor.Drain()
	if err != nil || len(events) != 1 || events[0].Kind != factorysessions.ResponseEventKindError {
		t.Fatal("retained activity lost")
	}
}

func runHostCloseRefusalResponse(variant string, request *http.Request, deletes, terminates *int, cancel context.CancelFunc) (*http.Response, error) {
	if variant == "missing" {
		return runHostResponse(404, `{"code":"NOT_FOUND"}`), nil
	}
	if request.Method == http.MethodDelete {
		*deletes++
		return runHostResponse(500, `{"message":"planted-private-token"}`), nil
	}
	if request.Method == http.MethodPost {
		*terminates++
		if variant == "terminal-foreign" {
			return runHostResponse(409, `{"sessionId":"peer","outcome":"TERMINAL_SESSION"}`), nil
		}
		if variant == "deadline" {
			cancel()
		}
		return runHostResponse(200, `{"sessionId":"child"}`), nil
	}
	if variant == "default" {
		return runHostResponse(200, `{"id":"child","isDefault":true}`), nil
	}
	if variant == "foreign" {
		return runHostResponse(200, `{"id":"peer"}`), nil
	}
	status := "FINISHED"
	if variant == "deadline" {
		status = "ACTIVE"
	}
	return runHostResponse(200, `{"id":"child","runtime":{"status":"`+status+`"}}`), nil
}

func assertRunHostCloseRefusal(t *testing.T, variant string, result factorysessions.SessionControlResult, err error, deletes, terminates int) {
	t.Helper()
	if variant == "missing" {
		assertRunHostMissingChild(t, result, err, deletes, terminates)
		return
	}
	if err == nil || result.Closed || strings.Contains(err.Error(), "planted-private-token") {
		t.Fatal("cleanup failure invented successful removal or leaked diagnostics")
	}
	if variant != "delete-failure" && deletes != 0 {
		t.Fatal("unsafe delete attempted")
	}
	if (variant == "default" || variant == "foreign") && terminates != 0 {
		t.Fatal("unrelated session terminated")
	}
	if variant == "deadline" && !errors.Is(err, context.Canceled) {
		t.Fatal("cleanup lost context failure")
	}
}

func assertRunHostMissingChild(t *testing.T, result factorysessions.SessionControlResult, err error, deletes, terminates int) {
	t.Helper()
	if err != nil || !result.Closed || terminates != 0 || deletes != 0 {
		t.Fatal("missing child was not reconciled")
	}
}

func TestSubagentHostLateActivationCleanup(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, finishOpen, deleted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	run, err := NewHostOperation("http://selected.invalid", newRunHostLateTransport(t, started, finishOpen, deleted), "/root", func() string { return "late-request" }, func(_ context.Context, identity string) (string, error) { return identity, nil })
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan ToolResponse[SubagentResult], 1)
	go func() { returned <- run(ctx, SubagentInput{Prompt: "late-start"}) }()
	select {
	case <-started:
	case <-t.Context().Done():
		t.Fatal("start did not reach HTTP edge")
	}
	cancel()
	select {
	case result := <-returned:
		if result.Error == nil {
			t.Error("canceled caller reported successful execution")
		}
	case <-t.Context().Done():
		t.Fatal("canceled RUN did not return")
	}
	close(finishOpen)
	select {
	case <-deleted:
	case <-t.Context().Done():
		t.Fatal("late activation was not retired")
	}
}

func newRunHostLateTransport(t *testing.T, started chan<- struct{}, finishOpen <-chan struct{}, deleted chan<- struct{}) runHostTransport {
	return func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodPost && request.URL.Path == "/factory-sessions" {
			close(started)
			<-finishOpen
			if request.Context().Err() != nil {
				t.Error("caller cancellation lost late activation response")
			}
			return runHostResponse(200, `{"session":{"id":"late-child"}}`), nil
		}
		if request.Context().Err() != nil {
			t.Error("late cleanup used canceled caller context")
		}
		switch request.Method {
		case http.MethodGet:
			return runHostResponse(200, `{"id":"late-child","runtime":{"status":"FINISHED"}}`), nil
		case http.MethodPost:
			return runHostResponse(200, `{"sessionId":"late-child","outcome":"ACCEPTED"}`), nil
		case http.MethodDelete:
			close(deleted)
			return runHostResponse(204, ""), nil
		default:
			return nil, errors.New("unexpected late child operation")
		}
	}
}
