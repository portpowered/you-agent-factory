package workersessionmcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
)

type toolError struct {
	Code            string         `json:"code"`
	Message         string         `json:"message"`
	Retryable       bool           `json:"retryable"`
	WorkerSessionID string         `json:"workerSessionId,omitempty"`
	Details         map[string]any `json:"details,omitempty"`
}

func invalidRequest(err error) *toolError {
	return &toolError{Code: "worker_session.invalid_request", Message: err.Error()}
}

func responseFailure(response *http.Response, err error, id string) *toolError {
	if err != nil || response == nil {
		return &toolError{Code: "worker_session.host_unavailable", Message: "Selected Worker Session host is unavailable", Retryable: true, WorkerSessionID: id}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	failure := &toolError{WorkerSessionID: id, Details: map[string]any{"status": response.StatusCode}}
	switch response.StatusCode {
	case 400:
		failure.Code = "worker_session.invalid_request"
	case 401, 403:
		failure.Code = "worker_session.permission_denied"
	case 404:
		failure.Code = "worker_session.not_found"
	case 409:
		failure.Code = "worker_session.conflict"
	case 503:
		failure.Code = "worker_session.unavailable"
		failure.Retryable = true
	default:
		failure.Code = "worker_session.internal_error"
	}
	failure.Message = "Selected host rejected the Worker Session request"
	readSafeFailureDetails(response, failure, id)
	return failure
}

// The request owner closes the body after decoding, including transport failures.
func readSafeFailureDetails(response *http.Response, failure *toolError, id string) {
	if response.Body != nil {
		var upstream struct {
			Code                     string `json:"code"`
			Phase                    string `json:"phase"`
			SourceWorkerSessionID    string `json:"sourceWorkerSessionId"`
			SuccessorWorkerSessionID string `json:"successorWorkerSessionId"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&upstream) == nil {
			// Preserve known public codes and phases, never arbitrary error text.
			if slices.Contains(safeUpstreamCodes, upstream.Code) {
				failure.Details["upstreamCode"] = upstream.Code
			}
			if slices.Contains([]string{"VALIDATION", "SOURCE_CANCELLATION", "SUCCESSOR_ADMISSION", "TRANSPORT", "RESPONSE", "WAIT"}, upstream.Phase) {
				failure.Details["phase"] = upstream.Phase
				if upstream.SourceWorkerSessionID == id {
					failure.Details["sourceWorkerSessionId"] = id
				}
				// Successor identity is supplied by the caller, not upstream diagnostics.
			}
		}
	}
}

var safeUpstreamCodes = []string{
	"WORKER_SESSION_NOT_FOUND", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE", "WORKER_SESSION_TRANSCRIPT_PROJECTION_UNAVAILABLE",
	"WORKER_SESSION_INTERRUPT_REQUEST_ID_CONFLICT", "WORKER_SESSION_INTERRUPT_SOURCE_CANCELLATION_FAILED", "WORKER_SESSION_INTERRUPT_SUCCESSOR_ADMISSION_FAILED",
}

func decodeResponse(response *http.Response, err error, id string) (any, *toolError) {
	if failure := responseFailure(response, err, id); failure != nil {
		return nil, failure
	}
	if response.Body == nil {
		return nil, malformedResponse(id)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<20))
	var value json.RawMessage
	if decoder.Decode(&value) != nil || len(value) == 0 || value[0] != '{' {
		return nil, malformedResponse(id)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, malformedResponse(id)
	}
	return value, nil
}

func malformedResponse(id string) *toolError {
	return &toolError{Code: "worker_session.internal_error", Message: "Selected host returned an invalid Worker Session response", WorkerSessionID: id}
}

type eventPage struct {
	Events    []json.RawMessage `json:"events"`
	Truncated bool              `json:"truncated"`
}

func decodeReplay(response *http.Response, err error, id string, limit int) (any, *toolError) {
	if failure := responseFailure(response, err, id); failure != nil {
		return nil, failure
	}
	if response.Body == nil {
		return nil, malformedResponse(id)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	page := eventPage{Events: make([]json.RawMessage, 0, limit)}
	var data []string
	appendFrame := func() error {
		if len(data) == 0 {
			return nil
		}
		frame := json.RawMessage(strings.Join(data, "\n"))
		data = nil
		if !json.Valid(frame) || len(frame) == 0 || frame[0] != '{' {
			return fmt.Errorf("invalid event frame")
		}
		if len(page.Events) == limit {
			page.Truncated = true
		} else {
			page.Events = append(page.Events, frame)
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if appendFrame() != nil {
				return nil, malformedResponse(id)
			}
			if page.Truncated {
				return page, nil
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if scanner.Err() != nil || appendFrame() != nil {
		return nil, malformedResponse(id)
	}
	return page, nil
}
