package workersessionmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

// Subagent selects an action before executing RUN or contacting the selected host.
// RUN retains its Factory-owned decoding, defaults, errors and cleanup.
func (a *Adapter) Subagent(ctx context.Context, raw json.RawMessage, run func(context.Context, json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	action, input, err := decodeAction(raw)
	if err != nil {
		return json.Marshal(struct {
			Error *toolError `json:"error"`
		}{invalidRequest(err)})
	}
	if action == "RUN" {
		return run(ctx, input)
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
	action := "RUN"
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
			if err := json.Unmarshal(value, &action); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || !slices.Contains([]string{"RUN", ActionList, ActionRead, ActionControl}, action) {
				return "", nil, fmt.Errorf("action must be RUN, LIST, READ or CONTROL")
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
