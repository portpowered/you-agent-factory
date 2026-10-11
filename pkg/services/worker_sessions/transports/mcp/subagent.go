package workersessionmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	client "github.com/portpowered/infinite-you/pkg/transports/http/client"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// Subagent validates the selector before starting or contacting the selected host.
func (a *Adapter) Subagent(ctx context.Context, raw json.RawMessage, start func(context.Context, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	action, input, err := decodeAction(raw)
	if err != nil {
		return json.Marshal(struct {
			Error *toolError `json:"error"`
		}{invalidRequest(err)})
	}
	if action == "RUN" {
		return json.Marshal(struct {
			Error *toolError `json:"error"`
		}{&toolError{Code: "worker_session.run_removed", Message: "RUN is no longer supported; use START and READ the returned workerSessionId"}})
	}
	if action == "START" {
		return start(ctx, input)
	}
	return a.Call(ctx, action, input)
}

// Read object members individually so duplicate selectors and case aliases
// cannot change the chosen action or target before validation.
func decodeAction(raw json.RawMessage) (string, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return "", nil, fmt.Errorf("arguments must be one JSON object")
	}
	fields := map[string]json.RawMessage{}
	action := "START"
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", nil, fmt.Errorf("invalid subagent arguments")
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil || !slices.Contains([]string{"action", "prompt", "provider", "model", "reasoningEffort", "workingRoot", "timeoutMillis", "history", "scope", "state", "limit", "nextToken", "workerSessionId", "view", "operation", "requestId", "expectedAttemptId", "successorWorkerSessionId", "replacementMessage", "resumeMode"}, key) {
			return "", nil, fmt.Errorf("arguments contain duplicate or unknown properties")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return "", nil, fmt.Errorf("invalid subagent arguments")
		}
		fields[key] = value
		if key == "action" {
			action, err = decodeActionSelector(value)
			if err != nil {
				return "", nil, err
			}
		}
	}
	if closing, err := decoder.Token(); err != nil || closing != json.Delim('}') {
		return "", nil, fmt.Errorf("arguments must be one JSON object")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", nil, fmt.Errorf("arguments must be one JSON object")
	}
	delete(fields, "action")
	input, err := json.Marshal(fields)
	return action, input, err
}

func decodeActionSelector(value json.RawMessage) (string, error) {
	var action string
	if err := json.Unmarshal(value, &action); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || !slices.Contains([]string{"START", "RUN", ActionList, ActionRead, ActionControl}, action) {
		return "", fmt.Errorf("action must be START, RUN, LIST, READ or CONTROL")
	}
	return action, nil
}

// Start forwards the prompt form to the selected host. Route resolution,
// preflight and admission are authoritative on that host.
func (a *Adapter) Start(ctx context.Context, raw json.RawMessage, workingRoot string, generateID func() string) (json.RawMessage, error) {
	input, err := decodeStart(raw, workingRoot)
	var result any
	var failure *toolError
	if err != nil {
		failure = invalidRequest(err)
	} else {
		if input.RequestID == nil {
			value := generateID()
			input.RequestID = &value
		}
		if input.WorkerSessionID == nil {
			value := generateID()
			input.WorkerSessionID = &value
		}
		response, requestErr := a.host.StartWorkerSession(ctx, nil, client.StartWorkerSessionJSONRequestBody{
			RequestId: *input.RequestID, WorkerSessionId: *input.WorkerSessionID, Prompt: &input.Prompt,
			Provider: input.Provider, Model: input.Model, ReasoningEffort: input.ReasoningEffort,
			WorkingRoot: input.WorkingRoot, TimeoutMillis: input.TimeoutMillis,
		})
		if response != nil && response.Body != nil {
			defer func() { _ = response.Body.Close() }()
		}
		result, failure = decodeResponse(response, requestErr, *input.WorkerSessionID)
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

type startInput struct {
	RequestID       *string `json:"requestId"`
	WorkerSessionID *string `json:"workerSessionId"`
	Prompt          string  `json:"prompt"`
	Provider        *string `json:"provider"`
	Model           *string `json:"model"`
	ReasoningEffort *string `json:"reasoningEffort"`
	WorkingRoot     *string `json:"workingRoot"`
	TimeoutMillis   *int64  `json:"timeoutMillis"`
}

func decodeStart(raw json.RawMessage, workingRoot string) (startInput, error) {
	var input startInput
	_, err := decodeInput(raw, &input)
	if err == nil && strings.TrimSpace(input.Prompt) == "" {
		err = fmt.Errorf("prompt must be nonempty")
	}
	if err == nil {
		err = validateStartStrings(input)
	}

	if err == nil && input.TimeoutMillis != nil && (*input.TimeoutMillis < 1 || *input.TimeoutMillis > 9223372036854) {
		err = fmt.Errorf("timeoutMillis must be positive and fit an execution duration")
	}
	if input.WorkingRoot == nil && workingRoot != "" {
		input.WorkingRoot = &workingRoot
	}
	if err == nil && input.WorkingRoot != nil && !filepath.IsAbs(*input.WorkingRoot) {
		err = fmt.Errorf("workingRoot must be an absolute path")
	}
	return input, err
}

func validateStartStrings(input startInput) error {
	for _, value := range []*string{input.RequestID, input.WorkerSessionID, input.Provider, input.Model, input.ReasoningEffort, input.WorkingRoot} {
		if value != nil && strings.TrimSpace(*value) == "" {
			return fmt.Errorf("explicit identities, route and workingRoot must be nonempty")
		}
	}
	return nil
}
