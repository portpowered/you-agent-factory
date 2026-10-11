package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// The handler is the unit. All authorization, store transitions and Events
// behavior remain controlled at its injected public Service port.
type messageService struct {
	send   func(context.Context, agentmessages.SendRequest, *workersessions.CallerIdentity, string) (agentmessages.Message, error)
	get    func(context.Context, agentmessages.GetRequest) (agentmessages.Message, error)
	list   func(context.Context, agentmessages.ListRequest) (agentmessages.Page, error)
	follow func(context.Context, agentmessages.ListRequest, func(agentmessages.Observation) error) error
}

func (s messageService) Send(ctx context.Context, r agentmessages.SendRequest, c *workersessions.CallerIdentity, scope string) (agentmessages.Message, error) {
	return s.send(ctx, r, c, scope)
}
func (s messageService) Get(ctx context.Context, r agentmessages.GetRequest) (agentmessages.Message, error) {
	return s.get(ctx, r)
}
func (s messageService) List(ctx context.Context, r agentmessages.ListRequest) (agentmessages.Page, error) {
	return s.list(ctx, r)
}
func (s messageService) Follow(ctx context.Context, r agentmessages.ListRequest, observe func(agentmessages.Observation) error) error {
	return s.follow(ctx, r, observe)
}

func workerRequest(method, url, body string) *http.Request {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	r.Header.Set("X-You-Worker-Session-Id", "worker")
	r.Header.Set("Authorization", "Bearer "+base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	return r
}

func TestSendDecodesOnlyAuthoredBodyAndSeparateCallerAndScope(t *testing.T) {
	t.Parallel()
	var captured agentmessages.SendRequest
	var caller *workersessions.CallerIdentity
	var scope string
	service := messageService{send: func(_ context.Context, request agentmessages.SendRequest, principal *workersessions.CallerIdentity, factory string) (agentmessages.Message, error) {
		captured, caller, scope = request, principal, factory
		return agentmessages.Message{MessageID: "accepted", Status: agentmessages.Queued, Body: "[REDACTED]", BodyRedactionCount: 1}, nil
	}}
	h := NewHandler(service, logging.NoopLogger{})
	r := workerRequest("POST", "/messages?factorySessionId=host", `{"requestId":"r","body":"secret","bodySecret":true,"inReplyTo":"parent"}`)
	w := httptest.NewRecorder()
	h.Send(w, r)
	if w.Code != 202 || scope != "host" || caller.WorkerSessionID != "worker" || caller.Token == "" || captured.InReplyTo != "parent" || captured.To != nil || !captured.BodySecret || captured.ExpiresInSeconds != nil {
		t.Fatal("HTTP decoding changed sender authority or service-owned defaults")
	}
	if strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), caller.Token) || !strings.Contains(w.Body.String(), `"body":"[REDACTED]"`) {
		t.Fatal("response included request content or credentials")
	}
}

func TestSendRejectsMalformedEnvelopesBeforeServiceInvocation(t *testing.T) {
	t.Parallel()
	h := NewHandler(messageService{send: func(context.Context, agentmessages.SendRequest, *workersessions.CallerIdentity, string) (agentmessages.Message, error) {
		t.Fatal("malformed request reached message admission")
		return agentmessages.Message{}, nil
	}}, logging.NoopLogger{})
	for _, body := range []string{
		`null`, `{`, `{"body":"planted-secret","sender":"forged"}`, `{"body":"x","to":{"workerSessionId":"w","operator":true}}`,
		`{"body":"x"} {}`, `{"body":7}`, "{\"body\":\"\xff\"}",
		`{"body":"` + strings.Repeat("x", maxSendEnvelopeBytes) + `"}`,
	} {
		w := httptest.NewRecorder()
		h.Send(w, workerRequest("POST", "/messages", body))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "MESSAGE_INVALID_REQUEST") || strings.Contains(w.Body.String(), "planted-secret") {
			t.Fatalf("malformed payload response: %d %s", w.Code, w.Body)
		}
	}
	r := workerRequest("POST", "/messages?factorySessionId=a&factorySessionId=b", `{"body":"x"}`)
	w := httptest.NewRecorder()
	h.Send(w, r)
	if w.Code != 400 {
		t.Fatal("repeated recipient scope accepted")
	}
}

func TestMalformedCredentialsNeverDowngradeToOperatorRead(t *testing.T) {
	t.Parallel()
	h := NewHandler(messageService{}, logging.NoopLogger{})
	for _, mutate := range []func(*http.Request){
		func(r *http.Request) { r.Header.Del("Authorization") },
		func(r *http.Request) { r.Header.Del("X-You-Worker-Session-Id") },
		func(r *http.Request) { r.Header.Add("Authorization", "Bearer forged") },
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer planted-secret") },
	} {
		for _, operation := range []func(http.ResponseWriter, *http.Request){h.Send, h.List, func(w http.ResponseWriter, r *http.Request) { h.Get(w, r, "m") }} {
			r := workerRequest("GET", "/messages", `{}`)
			mutate(r)
			w := httptest.NewRecorder()
			operation(w, r)
			if w.Code != 403 || !strings.Contains(w.Body.String(), "WORKER_SESSION_CALLER_INVALID") || strings.Contains(w.Body.String(), "planted-secret") {
				t.Fatalf("credential response: %d %s", w.Code, w.Body)
			}
		}
	}
}

func TestReadAndInboxForwardCallerAndAllFilters(t *testing.T) {
	t.Parallel()
	var got agentmessages.ListRequest
	service := messageService{
		list: func(_ context.Context, request agentmessages.ListRequest) (agentmessages.Page, error) {
			got = request
			return agentmessages.Page{}, nil
		},
		get: func(_ context.Context, request agentmessages.GetRequest) (agentmessages.Message, error) {
			if request.Caller != nil || request.MessageID != "m" {
				t.Fatal("operator observation gained authority")
			}
			return agentmessages.Message{MessageID: "m", Status: agentmessages.Replied, RepliedByMessageID: "reply"}, nil
		},
	}
	h := NewHandler(service, logging.NoopLogger{})
	w := httptest.NewRecorder()
	h.List(w, workerRequest("GET", "/messages?toMe=true&markRead=true&toWorkerSessionId=to&fromWorkerSessionId=from&factorySessionId=host&threadId=thread&correlationWorkId=work&correlationFactorySessionId=correlation&status=QUEUED&status=READ&maxResults=7&nextToken=cursor", ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"messages":[]`) || got.Caller.WorkerSessionID != "worker" {
		t.Fatal("inbox representation or authority changed")
	}
	got.Caller = nil
	want := agentmessages.ListRequest{ToMe: true, MarkRead: true, ToWorkerSessionID: "to", FromWorkerSessionID: "from", FactorySessionID: "host", ThreadID: "thread", Correlation: agentmessages.Correlation{WorkID: "work", FactorySessionID: "correlation"}, Statuses: []agentmessages.Status{agentmessages.Queued, agentmessages.Read}, MaxResults: 7, NextToken: "cursor"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lost AND filters: %#v", got)
	}
	w = httptest.NewRecorder()
	h.Get(w, httptest.NewRequest("GET", "/messages/m", nil), "m")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"repliedByMessageId":"reply"`) {
		t.Fatal("get lost reply relationship")
	}
}

func TestListRejectsMalformedQueriesBeforeReadOrFollow(t *testing.T) {
	t.Parallel()
	h := NewHandler(messageService{}, logging.NoopLogger{})
	for _, query := range []string{"toMe=1", "markRead=", "follow=yes", "maxResults=0", "maxResults=501", "maxResults=no", "status=unknown", "status=", "threadId=a&threadId=b", "nextToken="} {
		w := httptest.NewRecorder()
		h.List(w, workerRequest("GET", "/messages?"+query, ""))
		if w.Code != 400 {
			t.Fatalf("query %s accepted", query)
		}
	}
}

func TestSendFailurePublishesSafeTypedResponseAndRetryAfter(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{agentmessages.ErrDisabled, agentmessages.ErrStoreCorrupt, agentmessages.ErrNotPermitted, &agentmessages.LimitError{Dimension: "sender", RetryAfterSeconds: 13}, errors.New("planted-secret")} {
		h := NewHandler(messageService{send: func(context.Context, agentmessages.SendRequest, *workersessions.CallerIdentity, string) (agentmessages.Message, error) {
			return agentmessages.Message{}, fmt.Errorf("planted-secret: %w", failure)
		}}, logging.NoopLogger{})
		w := httptest.NewRecorder()
		h.Send(w, workerRequest("POST", "/messages", `{"body":"planted-secret","requestId":"r","to":{"workerSessionId":"lead"}}`))
		if w.Code < 400 || strings.Contains(w.Body.String(), "planted-secret") {
			t.Fatal("failure leaked payload or collaborator diagnostic")
		}
		if errors.Is(failure, agentmessages.ErrLimitExceeded) && (w.Code != 429 || w.Header().Get("Retry-After") != "13") {
			t.Fatal("missing rate limit retry")
		}
	}
}

type failedResponse struct{ header http.Header }

func (w *failedResponse) Header() http.Header { return w.header }
func (*failedResponse) WriteHeader(int)       {}
func (*failedResponse) Write([]byte) (int, error) {
	return 0, errors.New("planted-secret writer error")
}

type diagnosticLogger struct {
	logging.NoopLogger
	lines []string
}

func (l *diagnosticLogger) Warn(message string, fields ...any) {
	l.lines = append(l.lines, fmt.Sprint(message, fields))
}

func TestResponseWriteFailureHasContentFreeDiagnostic(t *testing.T) {
	t.Parallel()
	logger := &diagnosticLogger{}
	h := NewHandler(messageService{get: func(context.Context, agentmessages.GetRequest) (agentmessages.Message, error) {
		return agentmessages.Message{Body: "planted-secret"}, nil
	}}, logger)
	h.Get(&failedResponse{header: make(http.Header)}, httptest.NewRequest("GET", "/messages/m", nil), "m")
	if len(logger.lines) != 1 || strings.Contains(logger.lines[0], "planted-secret") {
		t.Fatal("encoding diagnostic exposed payload or collaborator error")
	}
}

var _ io.Writer = (*failedResponse)(nil)

func TestMalformedBodyNeverLeaksToJSONErrorResponse(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	h := NewHandler(messageService{}, logging.NoopLogger{})
	h.Send(w, workerRequest("POST", "/messages", `{"body":"planted-secret","from":"forged"}`))
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response["code"] != "MESSAGE_INVALID_REQUEST" || response["family"] != "BAD_REQUEST" {
		t.Fatal("malformed envelope lacks structured error", err)
	}
}
