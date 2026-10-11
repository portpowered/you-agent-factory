package workersessionmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	client "github.com/portpowered/infinite-you/pkg/transports/http/client"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// Caller identity is injected by the invocation, never accepted as tool data.
// This validates only credential syntax; the selected host verifies ownership.
func messageCaller(caller *workersessions.CallerIdentity, required bool) (client.RequestEditorFn, *toolError) {
	headers := make(http.Header)
	if caller != nil {
		headers.Set("X-You-Worker-Session-Id", caller.WorkerSessionID)
		headers.Set("Authorization", "Bearer "+caller.Token)
	}
	identity, err := apisurface.WorkerSessionCallerFromHeaders(headers)
	if err != nil || (required && identity == nil) {
		return nil, &toolError{Code: "worker_session.permission_denied", Message: "Current Worker Session execution credentials are required", Details: map[string]any{"upstreamCode": "WORKER_SESSION_CALLER_INVALID"}}
	}
	return func(_ context.Context, request *http.Request) error {
		for key, values := range headers {
			request.Header[key] = append([]string(nil), values...)
		}
		return nil
	}, nil
}

func (a *Adapter) messageAction(ctx context.Context, action string, raw json.RawMessage, caller *workersessions.CallerIdentity) (any, *toolError) {
	// Validate each closed action before touching even the HTTP client.
	switch action {
	case ActionMessage:
		input, err := decodeMessage(raw)
		if err != nil {
			return nil, invalidRequest(err)
		}
		return a.sendMessage(ctx, input, caller)
	case ActionRevive:
		input, err := decodeRevive(raw)
		if err != nil {
			return nil, invalidRequest(err)
		}
		editor, failure := messageCaller(caller, false)
		if failure != nil {
			return nil, failure
		}
		if credentialInArguments(raw, caller) {
			return nil, invalidRequest(fmt.Errorf("credentials are not tool arguments"))
		}
		resolve := true
		response, err := a.host.ContinueWorkerSession(ctx, input.WorkerSessionID, client.ContinueWorkerSessionJSONRequestBody{
			RequestId: input.RequestID, SuccessorWorkerSessionId: input.SuccessorWorkerSessionID,
			FollowUpInput: input.Message, ResolveHead: &resolve, FactorySessionId: input.FactorySessionID,
		}, editor)
		if response != nil && response.Body != nil {
			defer func() { _ = response.Body.Close() }()
		}
		return decodeMessageResponse(response, err, input.WorkerSessionID, caller, input.Message)
	default:
		input, err := decodeMessageRead(raw, action)
		if err != nil {
			return nil, invalidRequest(err)
		}
		return a.readMessages(ctx, action, input, raw, caller)
	}
}

func credentialInArguments(raw json.RawMessage, caller *workersessions.CallerIdentity) bool {
	return caller != nil && caller.Token != "" && strings.Contains(string(raw), caller.Token)
}

func (a *Adapter) sendMessage(ctx context.Context, input messageInput, caller *workersessions.CallerIdentity) (any, *toolError) {
	editor, failure := messageCaller(caller, true)
	if failure != nil {
		return nil, failure
	}
	body := client.SendAgentMessageJSONRequestBody{Body: input.Message, RequestId: input.RequestID,
		BodySecret: input.BodySecret, InReplyTo: input.InReplyTo, ExpiresInSeconds: input.ExpiresInSeconds}
	if input.WorkerSessionID != nil {
		body.To = &client.AgentMessageAddress{WorkerSessionId: *input.WorkerSessionID}
	}
	if input.Delivery != nil {
		value := client.AgentMessageSendRequestDelivery(*input.Delivery)
		body.Delivery = &value
	}
	if input.IfEnded != nil {
		value := client.AgentMessageSendRequestIfEnded(*input.IfEnded)
		body.IfEnded = &value
	}
	if input.ReplyIfEnded != nil {
		value := client.AgentMessageSendRequestReplyIfEnded(*input.ReplyIfEnded)
		body.ReplyIfEnded = &value
	}
	if input.Correlation != nil {
		body.Correlation = &struct {
			FactorySessionId *string `json:"factorySessionId,omitempty"`
			WorkId           *string `json:"workId,omitempty"`
		}{input.Correlation.FactorySessionID, input.Correlation.WorkID}
	}
	// Body credentials are redacted by the host. Identity/correlation credentials
	// cannot be persisted or reflected in a typed target diagnostic.
	identities, _ := json.Marshal(struct {
		Target      *string
		Reply       *string
		Request     string
		Scope       *string
		Correlation any
	}{input.WorkerSessionID, input.InReplyTo, input.RequestID, input.FactorySessionID, input.Correlation})
	if credentialInArguments(identities, caller) {
		return nil, invalidRequest(fmt.Errorf("credentials are not tool arguments"))
	}
	response, err := a.host.SendAgentMessage(ctx, &client.SendAgentMessageParams{FactorySessionId: input.FactorySessionID}, body, editor)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	return decodeMessageResponse(response, err, "", caller, input.Message)
}

func (a *Adapter) readMessages(ctx context.Context, action string, input messageReadInput, raw json.RawMessage, caller *workersessions.CallerIdentity) (any, *toolError) {
	editor, failure := messageCaller(caller, action == ActionList)
	if failure != nil {
		return nil, failure
	}
	if credentialInArguments(raw, caller) {
		return nil, invalidRequest(fmt.Errorf("credentials are not tool arguments"))
	}
	params := client.ListAgentMessagesParams{FactorySessionId: input.FactorySessionID, ThreadId: input.ThreadID,
		MaxResults: input.MaxResults, NextToken: input.NextToken, MarkRead: input.MarkRead,
		FromWorkerSessionId: input.FromWorkerSessionID, CorrelationWorkId: input.CorrelationWorkID, CorrelationFactorySessionId: input.CorrelationFactorySessionID}
	if action == ActionList {
		own := true
		params.ToMe = &own
	} else {
		params.ToWorkerSessionId = input.WorkerSessionID
	}
	if input.Status != nil {
		statuses := make([]client.AgentMessageDeliveryStatus, len(*input.Status))
		for i, status := range *input.Status {
			statuses[i] = client.AgentMessageDeliveryStatus(status)
		}
		params.Status = &statuses
	}
	response, err := a.host.ListAgentMessages(ctx, &params, editor)
	if response != nil && response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}
	return decodeMessageResponse(response, err, "", caller, "")
}

var safeMessageCodes = []string{
	"MESSAGE_INVALID_REQUEST", "MESSAGE_CURSOR_INVALID", "MESSAGE_INTERRUPT_UNSUPPORTED", "MESSAGE_NOT_PERMITTED",
	"MESSAGE_RECIPIENT_NOT_FOUND", "MESSAGE_NOT_FOUND", "MESSAGE_REQUEST_CONFLICT", "MESSAGE_LIMIT_EXCEEDED",
	"MESSAGING_DISABLED", "MESSAGE_STORE_UNAVAILABLE", "MESSAGE_STORE_CORRUPT", "WORKER_SESSION_AMBIGUOUS",
	"BAD_REQUEST", "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT", "NOT_FOUND", "WORKER_SESSION_CALLER_INVALID", "WORKER_SESSION_NOT_FOUND", "WORKER_SESSION_CONTINUATION_CONFLICT",
	"WORKER_SESSION_PROVIDER_CONTINUATION_INVALID", "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED",
}

// Only typed public host facts cross this error boundary. Never forward remote
// error text, transport causes, or arbitrary details that can echo content.
func decodeMessageResponse(response *http.Response, err error, id string, caller *workersessions.CallerIdentity, body string) (any, *toolError) {
	if err != nil || response == nil || (response.StatusCode >= 200 && response.StatusCode < 300) {
		return decodeResponse(response, err, id)
	}
	failure := responseFailure(&http.Response{StatusCode: response.StatusCode}, nil, id)
	if response.Body == nil {
		return nil, failure
	}
	raw := safeFailureBody(response.Body)
	var upstream struct {
		Code    string `json:"code"`
		Details struct {
			Candidates        []client.WorkerSessionAddressCandidate `json:"candidates"`
			Dimension         string                                 `json:"dimension"`
			RetryAfterSeconds int                                    `json:"retryAfterSeconds"`
		} `json:"details"`
	}
	if json.Unmarshal(raw, &upstream) != nil || !slices.Contains(safeMessageCodes, upstream.Code) {
		return nil, failure
	}
	failure.Details["upstreamCode"] = upstream.Code
	if upstream.Code == "MESSAGE_LIMIT_EXCEEDED" {
		failure.Code = "worker_session.limit_exceeded"
		failure.Retryable = upstream.Details.RetryAfterSeconds > 0
	}
	if upstream.Code == "WORKER_SESSION_AMBIGUOUS" {
		failure.Details["candidates"] = safeMessageCandidates(upstream.Details.Candidates, caller, body)
	}

	if upstream.Code == "MESSAGE_LIMIT_EXCEEDED" && slices.Contains([]string{"hop", "thread", "sender"}, upstream.Details.Dimension) {
		failure.Details["dimension"] = upstream.Details.Dimension
		if upstream.Details.RetryAfterSeconds > 0 {
			failure.Details["retryAfterSeconds"] = upstream.Details.RetryAfterSeconds
		}
	}
	return nil, failure
}

func safeMessageCandidates(candidates []client.WorkerSessionAddressCandidate, caller *workersessions.CallerIdentity, body string) []client.WorkerSessionAddressCandidate {
	safe := make([]client.WorkerSessionAddressCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !slices.Contains([]string{"RESERVED", "STARTING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELED", "TERMINATED"}, string(candidate.State)) {
			continue
		}
		values := []string{candidate.WorkerSessionId, candidate.FactorySessionId}
		if candidate.WorkId != nil {
			values = append(values, *candidate.WorkId)
		}
		private := false
		for _, value := range values {
			if (caller != nil && caller.Token != "" && strings.Contains(value, caller.Token)) || (body != "" && strings.Contains(value, body)) {
				private = true
			}
		}
		if !private {
			safe = append(safe, candidate)
		}
	}
	return safe
}
