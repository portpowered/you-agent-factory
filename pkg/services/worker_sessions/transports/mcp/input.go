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
	ExpectedAttemptID        *string `json:"expectedAttemptId"`
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
	if err := validateControlFields(raw); err != nil {
		return input, err
	}
	_, err := decodeInput(raw, &input)
	if err != nil {
		return input, err
	}
	if strings.TrimSpace(input.WorkerSessionID) == "" {
		return input, fmt.Errorf("workerSessionId is required")
	}
	if !slices.Contains([]string{"CANCEL", "TERMINATE", "INTERRUPT", "KILL"}, input.Operation) {
		return input, fmt.Errorf("operation must be CANCEL, TERMINATE, INTERRUPT or KILL")
	}
	if input.Operation == "KILL" {
		return input, validateKillInput(input)
	}
	if input.ExpectedAttemptID != nil {
		return input, fmt.Errorf("expectedAttemptId is accepted only with KILL")
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

// Control fields must have one canonical spelling and one value. Go's struct
// decoder otherwise accepts case aliases and lets later members change the
// operation or target before admission.
func validateControlFields(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return fmt.Errorf("arguments must be one JSON object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid control arguments")
		}
		key, ok := token.(string)
		if !ok || seen[key] || !slices.Contains([]string{"workerSessionId", "operation", "requestId", "expectedAttemptId", "successorWorkerSessionId", "replacementMessage"}, key) {
			return fmt.Errorf("control arguments contain duplicate or unknown properties")
		}
		seen[key] = true
		if err := decoder.Decode(new(json.RawMessage)); err != nil {
			return fmt.Errorf("invalid control arguments")
		}
	}
	if closing, err := decoder.Token(); err != nil || closing != json.Delim('}') {
		return fmt.Errorf("arguments must be one JSON object")
	}
	return nil
}

func validateKillInput(input controlInput) error {
	if input.SuccessorWorkerSessionID != nil || input.ReplacementMessage != nil {
		return fmt.Errorf("replacement fields are accepted only with INTERRUPT")
	}
	if input.RequestID == nil || strings.TrimSpace(*input.RequestID) == "" || input.ExpectedAttemptID == nil || strings.TrimSpace(*input.ExpectedAttemptID) == "" {
		return fmt.Errorf("KILL requires requestId and expectedAttemptId")
	}
	return nil
}
