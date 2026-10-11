package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type protocolStub struct {
	clihttp.Protocol
	execute func(*http.Request) (clihttp.Response, error)
}

func (stub protocolStub) Execute(request *http.Request) (clihttp.Response, error) {
	return stub.execute(request)
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (body *trackedBody) Close() error { body.closed = true; return nil }

func environment(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) { value, exists := values[key]; return value, exists }
}

func workerConfig(output io.Writer) Config {
	return Config{Context: context.Background(), Server: "http://selected.test/prefix", Output: output, OutputFormat: "json",
		LookupEnv: environment(map[string]string{
			"YOU_WORKER_SESSION_ID": "exact/caller", "YOU_WORKER_SESSION_TOKEN": strings.Repeat("A", 43),
			"YOU_SERVER": "http://ignored.test", "YOU_WORK_ID": "own-work", "YOU_FACTORY_SESSION_ID": "own-factory",
		})}
}

const messageJSON = `{"messageId":"msg-1","threadId":"msg-1","from":{"principal":"WORKER","workerSessionId":"exact/caller"},"to":{"kind":"WORKER_SESSION","workerSessionId":"lead"},"body":"[REDACTED]","bodySha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bodyRedactionCount":1,"delivery":"QUEUE","ifEnded":"REVIVE","replyIfEnded":"HOLD","status":"QUEUED","hop":0,"sentAt":"2026-10-11T01:00:00Z","expiresAt":"2026-10-12T01:00:00Z"}`

func assertSendRequest(t *testing.T, request *http.Request, reply bool) {
	t.Helper()
	for name, pair := range map[string][2]string{
		"method": {request.Method, http.MethodPost}, "host": {request.URL.Host, "selected.test"}, "path": {request.URL.Path, "/prefix/messages"},
		"scope": {request.URL.Query().Get("factorySessionId"), "legacy-scope"}, "token": {request.Header.Get("Authorization"), "Bearer " + strings.Repeat("A", 43)},
		"caller": {request.Header.Get("X-You-Worker-Session-Id"), "exact/caller"}, "media": {request.Header.Get("Content-Type"), "application/json"},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("send mapping lost %s", name)
		}
	}
	encoded, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), strings.Repeat("A", 43)) || strings.Contains(string(encoded), "exact/caller") {
		t.Fatal("credential or caller appeared in payload")
	}
	var payload agentmessages.SendRequest
	if json.Unmarshal(encoded, &payload) != nil {
		t.Fatal("invalid request JSON")
	}
	seconds := 60
	want := agentmessages.SendRequest{RequestID: "generated-request", Body: "planted-secret", BodySecret: true, IfEnded: "HOLD", ReplyIfEnded: "HOLD",
		ExpiresInSeconds: &seconds, Correlation: agentmessages.Correlation{WorkID: "own-work", FactorySessionID: "explicit-correlation"}}
	if reply {
		want.InReplyTo = "msg-original"
	} else {
		want.To = &agentmessages.Address{WorkerSessionID: "lead"}
	}
	if !reflect.DeepEqual(payload, want) {
		t.Fatal("send mapping changed authored fields or reply address default")
	}
}

func TestSendRequestAndReplyRepresentation(t *testing.T) {
	t.Parallel()
	for _, reply := range []bool{false, true} {
		t.Run(map[bool]string{false: "send", true: "reply"}[reply], func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			body := &trackedBody{Reader: strings.NewReader(`{"message":` + messageJSON + `}`)}
			adapter := NewAdapter(protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
				assertSendRequest(t, request, reply)
				return clihttp.Response{HTTP: &http.Response{StatusCode: 202, Body: body}}, nil
			}}, nil, func() string { return "generated-request" })
			expires := time.Minute
			config := SendConfig{Config: workerConfig(&output), Stdin: strings.NewReader("planted-secret"), BodySecret: true, IfEnded: "hold", ReplyHold: true,
				Expires: &expires, Correlation: agentmessages.Correlation{FactorySessionID: "explicit-correlation"}}
			config.SessionID = "legacy-scope"
			if reply {
				config.InReplyTo = "msg-original"
			} else {
				config.WorkerSessionID = "lead"
			}
			if err := adapter.Send(config); err != nil {
				t.Fatal(err)
			}
			var result factoryapi.AgentMessageSendResponse
			if json.Unmarshal(output.Bytes(), &result) != nil || result.Message.MessageId != "msg-1" || result.Message.BodyRedactionCount != 1 ||
				strings.Contains(output.String(), "planted-secret") || !body.closed {
				t.Fatal("response representation, privacy or cleanup changed")
			}
		})
	}
}

func TestCallerValidationBeforeHTTPAndStdin(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		env  map[string]string
		want error
	}{
		{name: "absent", want: agentmessages.ErrNotPermitted},
		{name: "id only", env: map[string]string{"YOU_WORKER_SESSION_ID": "caller"}, want: workersessions.ErrCallerInvalid},
		{name: "token only", env: map[string]string{"YOU_WORKER_SESSION_TOKEN": strings.Repeat("A", 43)}, want: workersessions.ErrCallerInvalid},
		{name: "empty", env: map[string]string{"YOU_WORKER_SESSION_ID": "", "YOU_WORKER_SESSION_TOKEN": ""}, want: workersessions.ErrCallerInvalid},
		{name: "malformed", env: map[string]string{"YOU_WORKER_SESSION_ID": "caller", "YOU_WORKER_SESSION_TOKEN": "bad"}, want: workersessions.ErrCallerInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewAdapter(protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				t.Fatal("invalid authority reached HTTP")
				return clihttp.Response{}, nil
			}}, nil, nil)
			config := SendConfig{Config: Config{Context: context.Background(), Server: "http://selected.test", Output: io.Discard, LookupEnv: environment(test.env)},
				WorkerSessionID: "lead", Stdin: refusingReader{}}
			if err := adapter.Send(config); !errors.Is(err, test.want) {
				t.Fatalf("unexpected typed refusal: %v", err)
			}
			if errors.Is(test.want, workersessions.ErrCallerInvalid) {
				if err := adapter.List(ListConfig{Config: config.Config}); !errors.Is(err, test.want) {
					t.Fatal("partial credentials downgraded to observation")
				}
			}
		})
	}
}

type refusingReader struct{}

func (refusingReader) Read([]byte) (int, error) { return 0, errors.New("stdin must not be read") }

func TestBodySourcesAndLimits(t *testing.T) {
	t.Parallel()
	explicit := "exact\nbody"
	for _, test := range []struct {
		name   string
		config SendConfig
		want   string
		bad    bool
		limit  bool
	}{
		{name: "explicit", config: SendConfig{Body: &explicit}, want: explicit},
		{name: "args", config: SendConfig{BodyArgs: []string{"one", "two"}}, want: "one two"},
		{name: "stdin", config: SendConfig{Stdin: strings.NewReader("exact\n")}, want: "exact\n"},
		{name: "UTF8 boundary", config: SendConfig{Stdin: strings.NewReader(strings.Repeat("é", 4096))}, want: strings.Repeat("é", 4096)},
		{name: "oversized", config: SendConfig{Stdin: strings.NewReader(strings.Repeat("é", 4097))}, limit: true},
		{name: "invalid UTF8", config: SendConfig{BodyArgs: []string{string([]byte{0xff})}}, bad: true},
		{name: "whitespace", config: SendConfig{BodyArgs: []string{" \n\t"}}, bad: true},
		{name: "conflict", config: SendConfig{Body: &explicit, BodyArgs: []string{"other"}}, bad: true},
		{name: "TTY", config: SendConfig{Stdin: refusingReader{}, StdinIsTTY: true}, bad: true},
		{name: "read error", config: SendConfig{Stdin: refusingReader{}}, bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := (&Adapter{}).sendBody(test.config)
			if test.limit {
				if !errors.Is(err, agentmessages.ErrLimitExceeded) || got != "" {
					t.Fatal("oversized stdin lost typed limit refusal")
				}
			} else if test.bad {
				if !errors.Is(err, agentmessages.ErrBadRequest) || got != "" {
					t.Fatal("invalid body lost safe typed refusal")
				}
			} else if err != nil || got != test.want {
				t.Fatal("body bytes changed")
			}
		})
	}
}

func TestReadFiltersAndEmptyPage(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	maxResults := 7
	protocol := protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
		query := request.URL.Query()
		for key, want := range map[string]string{"toMe": "true", "markRead": "true", "toWorkerSessionId": "recipient", "fromWorkerSessionId": "sender",
			"threadId": "thread", "correlationWorkId": "work", "correlationFactorySessionId": "factory", "nextToken": "opaque+/=", "maxResults": "7", "factorySessionId": "scope"} {
			if query.Get(key) != want {
				t.Fatalf("missing AND filter %s", key)
			}
		}
		if len(query["status"]) != 2 || query["status"][0] != "QUEUED" || query["status"][1] != "READ" || request.Header.Get("Authorization") != "" {
			t.Fatal("repeated statuses or read-only operator request changed")
		}
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"messages":null,"nextToken":null}`))}}, nil
	}}
	config := ListConfig{Config: Config{Context: context.Background(), Server: "http://selected.test", SessionID: "scope", Output: &output, OutputFormat: "json"},
		ToMe: true, MarkRead: true, ToWorkerSessionID: "recipient", FromWorkerSessionID: "sender", ThreadID: "thread", Correlation: agentmessages.Correlation{WorkID: "work", FactorySessionID: "factory"},
		Statuses: []string{"QUEUED", "READ"}, NextToken: "opaque+/=", MaxResults: &maxResults}
	if err := NewAdapter(protocol, nil, nil).List(config); err != nil || strings.TrimSpace(output.String()) != `{"messages":[],"nextToken":null}` {
		t.Fatalf("empty inbox representation changed: %v", err)
	}
}

func TestGetEscapedIdentityAndHumanOutput(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	adapter := NewAdapter(protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
		if request.URL.EscapedPath() != "/prefix/messages/msg%2Fopaque%3F%23" || request.Method != http.MethodGet {
			t.Fatal("get identity changed route")
		}
		return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(messageJSON))}}, nil
	}}, nil, nil)
	config := GetConfig{Config: workerConfig(&output), MessageID: "msg/opaque?#"}
	config.OutputFormat = "human"
	if err := adapter.Get(config); err != nil || output.String() != "msg-1 QUEUED exact/caller -> lead\n[REDACTED]\n" {
		t.Fatalf("get output changed: %v", err)
	}
}

func TestRemoteErrorsPreserveDetailsAndOmitCredentials(t *testing.T) {
	t.Parallel()
	for _, code := range []string{"WORKER_SESSION_AMBIGUOUS", "MESSAGE_LIMIT_EXCEEDED", "MESSAGE_NOT_PERMITTED"} {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			token := strings.Repeat("A", 43)
			body := &trackedBody{Reader: strings.NewReader(`{"family":"validation","code":"` + code + `","message":"denied ` + token + `","details":{"retryAfterSeconds":9,"candidates":[{"workerSessionId":"legacy","factorySessionId":"scope","workId":"work","state":"RUNNING"}]}}`)}
			adapter := NewAdapter(protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				return clihttp.Response{HTTP: &http.Response{StatusCode: 409, Body: body}}, nil
			}}, nil, nil)
			err := adapter.List(ListConfig{Config: workerConfig(io.Discard)})
			var typed *clihttp.APIError
			if !errors.As(err, &typed) || string(typed.Response.Code) != code || typed.Response.Details == nil || typed.StatusCode != 409 || !body.closed {
				t.Fatal("typed response details or cleanup lost")
			}
			encoded, _ := json.Marshal(typed.Response)
			if strings.Contains(string(encoded)+err.Error(), token) || !strings.Contains(string(encoded), "scope") {
				t.Fatal("credential disclosure or ambiguity loss")
			}
		})
	}
}

func TestTransportAndDecodeFailuresAreSafe(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"transport", "nil", "decode", "trailing", "unknown error"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			body := &trackedBody{Reader: strings.NewReader("planted-secret " + strings.Repeat("A", 43))}
			adapter := NewAdapter(protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				switch mode {
				case "transport":
					return clihttp.Response{HTTP: &http.Response{Body: body}}, errors.New("planted-secret " + strings.Repeat("A", 43))
				case "nil":
					return clihttp.Response{}, nil
				case "trailing":
					body.Reader = strings.NewReader(`{"messages":[]} {}`)
				case "unknown error":
					return clihttp.Response{HTTP: &http.Response{StatusCode: 503, Body: body}}, nil
				}
				return clihttp.Response{HTTP: &http.Response{StatusCode: 200, Body: body}}, nil
			}}, nil, nil)
			err := adapter.List(ListConfig{Config: workerConfig(&output)})
			if !errors.Is(err, agentmessages.ErrStoreUnavailable) || output.Len() != 0 || (mode != "nil" && !body.closed) {
				t.Fatal("failure disclosed collaborator payload, wrote output or leaked response")
			}
		})
	}
}
