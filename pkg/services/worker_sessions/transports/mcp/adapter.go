// Package workersessionmcp maps Worker Session tools to one selected HTTP host.
package workersessionmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	client "github.com/portpowered/infinite-you/pkg/transports/http/client"
)

const (
	ToolList    = "you.worker_session.list"
	ToolRead    = "you.worker_session.read"
	ToolControl = "you.worker_session.control"
)

// HostClient is the exact generated-client boundary consumed by the adapter.
type HostClient interface {
	ListWorkerSessions(context.Context, *client.ListWorkerSessionsParams, ...client.RequestEditorFn) (*http.Response, error)
	GetWorkerSessionObservationByWorkerSessionId(context.Context, client.WorkerSessionID, ...client.RequestEditorFn) (*http.Response, error)
	ReadWorkerSessionTranscriptByWorkerSessionId(context.Context, client.WorkerSessionID, ...client.RequestEditorFn) (*http.Response, error)
	ReadWorkerSessionLogs(context.Context, client.WorkerSessionID, *client.ReadWorkerSessionLogsParams, ...client.RequestEditorFn) (*http.Response, error)
	StreamWorkerSessionEventsByTopLevelWorkerSessionId(context.Context, client.WorkerSessionID, *client.StreamWorkerSessionEventsByTopLevelWorkerSessionIdParams, ...client.RequestEditorFn) (*http.Response, error)
	CancelWorkerSession(context.Context, client.WorkerSessionID, ...client.RequestEditorFn) (*http.Response, error)
	TerminateWorkerSession(context.Context, client.WorkerSessionID, ...client.RequestEditorFn) (*http.Response, error)
	InterruptWorkerSession(context.Context, client.WorkerSessionID, client.InterruptWorkerSessionJSONRequestBody, ...client.RequestEditorFn) (*http.Response, error)
}

// Adapter retains one immutable host client; it never resolves local sessions.
type Adapter struct{ host HostClient }

func New(host HostClient) (*Adapter, error) {
	if host == nil {
		return nil, fmt.Errorf("worker session MCP host client is required")
	}
	return &Adapter{host: host}, nil
}

// Call validates the complete input before sending any request to the host.
func (a *Adapter) Call(ctx context.Context, name string, raw json.RawMessage) (json.RawMessage, error) {
	var result any
	var failure *toolError
	switch name {
	case ToolList:
		input, err := decodeList(raw)
		if err != nil {
			failure = invalidRequest(err)
		} else {
			result, failure = a.list(ctx, input)
		}
	case ToolRead:
		input, err := decodeRead(raw)
		if err != nil {
			failure = invalidRequest(err)
		} else {
			result, failure = a.read(ctx, input)
		}
	case ToolControl:
		input, err := decodeControl(raw)
		if err != nil {
			failure = invalidRequest(err)
		} else {
			result, failure = a.control(ctx, input)
		}
	default:
		failure = invalidRequest(fmt.Errorf("unknown Worker Session tool"))
	}
	if failure != nil {
		return json.Marshal(struct {
			Error *toolError `json:"error"`
		}{failure})
	}
	return json.Marshal(struct {
		Result any `json:"result"`
	}{result})
}

func (a *Adapter) list(ctx context.Context, input listInput) (any, *toolError) {
	history := client.ListWorkerSessionsParamsHistory(*input.History)
	params := client.ListWorkerSessionsParams{Limit: input.Limit, NextToken: input.NextToken, History: &history}
	if input.Scope != nil {
		scope := client.ListWorkerSessionsParamsScope(*input.Scope)
		params.Scope = &scope
	}
	if input.State != nil {
		states := make([]client.ListWorkerSessionsParamsState, len(*input.State))
		for i, state := range *input.State {
			states[i] = client.ListWorkerSessionsParamsState(state)
		}
		params.State = &states
	}
	response, err := a.host.ListWorkerSessions(ctx, &params)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	return decodeResponse(response, err, "")
}

func (a *Adapter) read(ctx context.Context, input readInput) (any, *toolError) {
	response, err := a.host.GetWorkerSessionObservationByWorkerSessionId(ctx, input.WorkerSessionID)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	session, failure := decodeResponse(response, err, input.WorkerSessionID)
	if failure != nil {
		return nil, failure
	}
	result := map[string]any{"session": session}
	switch input.View {
	case "logs":
		response, err := a.host.ReadWorkerSessionLogs(ctx, input.WorkerSessionID, &client.ReadWorkerSessionLogsParams{Limit: &input.Limit, NextToken: input.NextToken})
		if response != nil && response.Body != nil {
			defer func() { _ = response.Body.Close() }()
		}
		logs, failure := decodeResponse(response, err, input.WorkerSessionID)
		if failure != nil {
			return nil, failure
		}
		result["logs"] = logs
	case "transcript":
		response, err := a.host.ReadWorkerSessionTranscriptByWorkerSessionId(ctx, input.WorkerSessionID)
		if response != nil && response.Body != nil {
			defer func() { _ = response.Body.Close() }()
		}
		transcript, failure := decodeResponse(response, err, input.WorkerSessionID)
		if failure != nil {
			return nil, failure
		}
		result["transcript"] = transcript
	case "events":
		replayOnly := true
		replayCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		response, err := a.host.StreamWorkerSessionEventsByTopLevelWorkerSessionId(replayCtx, input.WorkerSessionID, &client.StreamWorkerSessionEventsByTopLevelWorkerSessionIdParams{ReplayOnly: &replayOnly})
		if response != nil && response.Body != nil {
			defer func() { _ = response.Body.Close() }()
		}
		events, failure := decodeReplay(response, err, input.WorkerSessionID, input.Limit)
		if failure != nil {
			return nil, failure
		}
		result["events"] = events
	}
	return result, nil
}

func (a *Adapter) control(ctx context.Context, input controlInput) (any, *toolError) {
	var response *http.Response
	var err error
	switch input.Operation {
	case "CANCEL":
		response, err = a.host.CancelWorkerSession(ctx, input.WorkerSessionID)
	case "TERMINATE":
		response, err = a.host.TerminateWorkerSession(ctx, input.WorkerSessionID)
	case "INTERRUPT":
		response, err = a.host.InterruptWorkerSession(ctx, input.WorkerSessionID, client.InterruptWorkerSessionJSONRequestBody{
			RequestId: *input.RequestID, SuccessorWorkerSessionId: *input.SuccessorWorkerSessionID, ReplacementMessage: *input.ReplacementMessage,
		})
	}
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	result, failure := decodeResponse(response, err, input.WorkerSessionID)
	if failure != nil && input.SuccessorWorkerSessionID != nil && failure.Details != nil {
		if _, partial := failure.Details["phase"]; partial {
			failure.Details["successorWorkerSessionId"] = *input.SuccessorWorkerSessionID
		}
	}
	return result, failure
}
