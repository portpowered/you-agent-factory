package factorysession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mapping "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// HostTransport is the selected host's HTTP execution edge.
type HostTransport interface {
	Do(*http.Request) (*http.Response, error)
}

// hostSessions adapts RUN's live lifecycle to one fixed HTTP host. It never falls back
// to the MCP process's local Factory Sessions service.
type hostSessions struct {
	server string
	http   HostTransport
}

// NewHostOperation binds the existing public live routes to the selected host.
func NewHostOperation(server string, transport HostTransport, workingRoot string, generateID factorysessions.SessionIDGenerator, resolveProvider ProviderIdentityResolver) (func(context.Context, SubagentInput) ToolResponse[SubagentResult], error) {
	endpoint, err := url.Parse(server)
	if err != nil || endpoint == nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || transport == nil {
		return nil, errors.New("subagent host and HTTP transport are required")
	}
	host := &hostSessions{server: strings.TrimRight(server, "/"), http: transport}
	return func(ctx context.Context, input SubagentInput) ToolResponse[SubagentResult] {
		return Subagent(ctx, host, workingRoot, generateID, resolveProvider, input)
	}, nil
}

func (host *hostSessions) Start(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	// RUN's bounded start retains its admission slot and closes late activation.
	// Keep HTTP response delivery alive for the existing close budget after
	// caller cancellation, so a successful late open still returns its identity.
	startCtx, stopStart := hostStartContext(ctx)
	defer stopStart()
	caller := request.Caller.Clone()
	factoryID := factorydefinitions.PackagedSubagentFactoryName
	requestID := request.Correlation.RequestID
	body := factoryapi.OpenFactorySessionRequest{FolderPath: request.FolderPath, FactoryId: &factoryID, RequestId: &requestID}
	var opened factoryapi.OpenFactorySessionResponse
	if err := host.exchange(startCtx, http.MethodPost, "/factory-sessions", body, caller, &opened); err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if opened.Session == nil || opened.Session.IsDefault || !exactChildID(opened.Session.Id, caller) {
		return factorysessions.SessionStartResult{}, errors.New("subagent host returned no exact child Factory Session")
	}
	return factorysessions.SessionStartResult{SessionID: opened.Session.Id, Mode: factorysessions.SessionOperationModeLive}, nil
}

func hostStartContext(ctx context.Context) (context.Context, func()) {
	detached, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopParent := context.AfterFunc(ctx, func() {
		timer := time.AfterFunc(subagentCloseTimeout, cancel)
		context.AfterFunc(detached, func() { timer.Stop() })
	})
	return detached, func() { stopParent(); cancel() }
}

func (host *hostSessions) Invoke(ctx context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	if !exactChildID(request.SessionID, request.Caller) {
		return factorysessions.InvocationResult{}, errors.New("subagent child Factory Session identity is required")
	}
	requestID, timeout := request.Correlation.RequestID, request.Wait.TimeoutMillis
	body := factoryapi.InvocationRequest{Args: &request.Args, RequestId: &requestID, TimeoutMillis: &timeout}
	var response factoryapi.InvocationResponse
	err := host.exchange(ctx, http.MethodPost, childPath(request.SessionID)+"/invocations", body, request.Caller.Clone(), &response)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	if response.SessionId != nil && *response.SessionId != request.SessionID {
		return factorysessions.InvocationResult{}, errors.New("subagent invocation returned a different Factory Session")
	}
	result := mapping.FactoryInvocationResultFromResponse(response)
	// sessionId is optional in InvocationResponse. The exact scoped request
	// supplies identity when the owner omits this compatibility field.
	result.SessionID = request.SessionID
	return factorysessions.InvocationResult{
		SessionID: result.SessionID, RequestID: result.RequestID, TraceID: result.TraceID,
		Status: factorysessions.InvocationTerminalStatus(result.Status), PrimaryResult: result.PrimaryResult,
		ErrorCode: result.ErrorCode, Message: result.Message, FailureReason: result.FailureReason,
		WorkID: result.WorkID, WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}

func exactChildID(id string, caller *workersessions.CallerIdentity) bool {
	return id != "" && strings.TrimSpace(id) == id && id != factorysessions.DefaultSessionID && (caller == nil || id != caller.WorkerSessionID)
}

func childPath(id string) string { return "/factory-sessions/" + url.PathEscape(id) }

// exchange omits credentials from JSON and never returns host diagnostic text.
// Typed caller refusal and context cancellation remain distinguishable.
func (host *hostSessions) exchange(ctx context.Context, method, path string, body any, caller *workersessions.CallerIdentity, result any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return errors.New("subagent host request could not be encoded")
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, host.server+path, bytes.NewReader(encoded))
	if err != nil {
		return errors.New("subagent host request is invalid")
	}
	request.Header.Set("Content-Type", "application/json")
	if caller != nil {
		if caller.WorkerSessionID == "" || caller.Token == "" {
			return workersessions.ErrCallerInvalid
		}
		request.Header.Set("X-You-Worker-Session-Id", caller.WorkerSessionID)
		request.Header.Set("Authorization", "Bearer "+caller.Token)
	}
	response, err := host.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("subagent host request failed")
	}
	if response == nil || response.Body == nil {
		return errors.New("subagent host response is unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return hostResponseError(response)
	}
	if result != nil && json.NewDecoder(response.Body).Decode(result) != nil {
		return errors.New("subagent host response is invalid")
	}
	return nil
}

type hostError struct {
	status    int
	code      string
	terminal  bool
	sessionID string
}

func (err *hostError) Error() string {
	return fmt.Sprintf("subagent host refused request (%d)", err.status)
}

func hostResponseError(response *http.Response) error {
	var body struct {
		Code      string `json:"code"`
		Outcome   string `json:"outcome"`
		SessionID string `json:"sessionId"`
	}
	_ = json.NewDecoder(response.Body).Decode(&body)
	if body.Code == "WORKER_SESSION_CALLER_INVALID" {
		return workersessions.ErrCallerInvalid
	}
	return &hostError{status: response.StatusCode, code: body.Code, terminal: body.Outcome == "TERMINAL_SESSION", sessionID: body.SessionID}
}
