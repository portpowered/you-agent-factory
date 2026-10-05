package workersessionmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

type listInput struct {
	History   *string   `json:"history"`
	Scope     *string   `json:"scope"`
	State     *[]string `json:"state"`
	Limit     *int      `json:"limit"`
	NextToken *string   `json:"nextToken"`
}
type readInput struct {
	WorkerSessionID string  `json:"workerSessionId"`
	View            string  `json:"view"`
	Limit           int     `json:"limit"`
	NextToken       *string `json:"nextToken"`
}
type controlInput struct {
	WorkerSessionID          string  `json:"workerSessionId"`
	Operation                string  `json:"operation"`
	RequestID                *string `json:"requestId"`
	SuccessorWorkerSessionID *string `json:"successorWorkerSessionId"`
	ReplacementMessage       *string `json:"replacementMessage"`
}

func decodeInput(raw json.RawMessage, output any) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("arguments must be one JSON object")
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("arguments cannot contain null values")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return nil, fmt.Errorf("arguments contain an unknown property or invalid value")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("arguments must be one JSON object")
	}
	return fields, nil
}

func decodeList(raw json.RawMessage) (listInput, error) {
	var input listInput
	_, err := decodeInput(raw, &input)
	if err != nil {
		return input, err
	}
	if input.History == nil {
		history := "all"
		input.History = &history
	}
	if !slices.Contains([]string{"active", "all", "archived"}, *input.History) {
		return input, fmt.Errorf("history must be active, all or archived")
	}
	if input.Scope != nil && !slices.Contains([]string{"all", "direct", "factory"}, *input.Scope) {
		return input, fmt.Errorf("scope must be all, direct or factory")
	}
	if input.State != nil {
		for _, state := range *input.State {
			if !slices.Contains([]string{"RESERVED", "STARTING", "RUNNING", "PAUSED", "COMPLETED", "FAILED", "CANCELED", "TERMINATED"}, state) {
				return input, fmt.Errorf("state must be a Worker Session lifecycle state")
			}
		}
	}
	if input.Limit != nil && *input.Limit < 1 {
		return input, fmt.Errorf("limit must be positive")
	}
	if input.NextToken != nil && strings.TrimSpace(*input.NextToken) == "" {
		return input, fmt.Errorf("nextToken must be nonempty")
	}
	return input, nil
}

func decodeRead(raw json.RawMessage) (readInput, error) {
	input := readInput{View: "summary", Limit: 100}
	fields, err := decodeInput(raw, &input)
	if err != nil {
		return input, err
	}
	if strings.TrimSpace(input.WorkerSessionID) == "" {
		return input, fmt.Errorf("workerSessionId is required")
	}
	if !slices.Contains([]string{"summary", "transcript", "events", "logs"}, input.View) {
		return input, fmt.Errorf("view must be summary, transcript, events or logs")
	}
	if _, supplied := fields["limit"]; supplied && input.View != "events" && input.View != "logs" {
		return input, fmt.Errorf("limit is accepted only with events or logs")
	}
	if input.NextToken != nil && (input.View != "logs" || strings.TrimSpace(*input.NextToken) == "") {
		return input, fmt.Errorf("nextToken must be nonempty and is accepted only with logs")
	}
	if input.Limit < 1 || input.Limit > 1000 {
		return input, fmt.Errorf("limit must be between 1 and 1000")
	}
	return input, nil
}

func decodeControl(raw json.RawMessage) (controlInput, error) {
	var input controlInput
	_, err := decodeInput(raw, &input)
	if err != nil {
		return input, err
	}
	if strings.TrimSpace(input.WorkerSessionID) == "" {
		return input, fmt.Errorf("workerSessionId is required")
	}
	if !slices.Contains([]string{"CANCEL", "TERMINATE", "INTERRUPT"}, input.Operation) {
		return input, fmt.Errorf("operation must be CANCEL, TERMINATE or INTERRUPT")
	}
	if input.Operation != "INTERRUPT" {
		if input.RequestID != nil || input.SuccessorWorkerSessionID != nil || input.ReplacementMessage != nil {
			return input, fmt.Errorf("replacement fields are accepted only with INTERRUPT")
		}
		return input, nil
	}
	if input.RequestID == nil || strings.TrimSpace(*input.RequestID) == "" || input.SuccessorWorkerSessionID == nil || strings.TrimSpace(*input.SuccessorWorkerSessionID) == "" || input.ReplacementMessage == nil || strings.TrimSpace(*input.ReplacementMessage) == "" {
		return input, fmt.Errorf("INTERRUPT requires requestId, successorWorkerSessionId and replacementMessage")
	}
	return input, nil
}
