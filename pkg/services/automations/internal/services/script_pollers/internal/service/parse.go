package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

type scriptPollerStdout struct {
	request          work.WorkRequest
	hasRequest       bool
	advancedCursor   string
	checkpoint       string
	advancesPosition bool
}

func parseScriptPollerStdout(stdout []byte) (scriptPollerStdout, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return scriptPollerStdout{}, nil
	}

	var envelope scriptPollerOutputEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return scriptPollerStdout{hasRequest: true}, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if len(envelope.Events) > 0 {
		return scriptPollerStdout{hasRequest: true}, fmt.Errorf("script poller emitted unsupported raw factory events")
	}
	if len(envelope.Request) > 0 && len(envelope.Submissions) > 0 {
		return scriptPollerStdout{hasRequest: true}, fmt.Errorf("script poller stdout must contain either request or submissions, not both")
	}

	recovery := scriptPollerRecoveryFromEnvelope(envelope)
	if len(envelope.Request) > 0 {
		request, err := work.ParseCanonicalWorkRequestJSON(envelope.Request)
		if err != nil {
			return scriptPollerStdout{hasRequest: true}, fmt.Errorf("script poller emitted malformed stdout: %w", err)
		}
		if err := validateScriptPollerWorkRequest(request); err != nil {
			return scriptPollerStdout{hasRequest: true}, err
		}
		return mergeScriptPollerStdout(scriptPollerStdout{
			request:    request,
			hasRequest: true,
		}, recovery), nil
	}
	if len(envelope.Submissions) > 0 {
		request, err := scriptPollerWorkRequestFromSubmissions(envelope.Submissions)
		if err != nil {
			return scriptPollerStdout{hasRequest: true}, err
		}
		return mergeScriptPollerStdout(scriptPollerStdout{
			request:    request,
			hasRequest: true,
		}, recovery), nil
	}

	request, err := work.ParseCanonicalWorkRequestJSON(trimmed)
	if err != nil {
		return scriptPollerStdout{hasRequest: true}, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if err := validateScriptPollerWorkRequest(request); err != nil {
		return scriptPollerStdout{hasRequest: true}, err
	}
	return mergeScriptPollerStdout(scriptPollerStdout{
		request:    request,
		hasRequest: true,
	}, recovery), nil
}

type scriptPollerOutputEnvelope struct {
	Request     json.RawMessage `json:"request"`
	Submissions json.RawMessage `json:"submissions"`
	Events      json.RawMessage `json:"events"`
	Cursor      string          `json:"cursor"`
	Checkpoint  string          `json:"checkpoint"`
}

func scriptPollerRecoveryFromEnvelope(envelope scriptPollerOutputEnvelope) scriptPollerStdout {
	cursor := strings.TrimSpace(envelope.Cursor)
	checkpoint := strings.TrimSpace(envelope.Checkpoint)
	return scriptPollerStdout{
		advancedCursor:   cursor,
		checkpoint:       checkpoint,
		advancesPosition: cursor != "" || checkpoint != "",
	}
}

func mergeScriptPollerStdout(parsed, recovery scriptPollerStdout) scriptPollerStdout {
	parsed.advancedCursor = recovery.advancedCursor
	parsed.checkpoint = recovery.checkpoint
	parsed.advancesPosition = recovery.advancesPosition
	return parsed
}

func scriptPollerWorkRequestFromSubmissions(data []byte) (work.WorkRequest, error) {
	var submissions []work.SubmitRequest
	if err := json.Unmarshal(data, &submissions); err != nil {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: decode submissions: %w", err)
	}
	if len(submissions) == 0 {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must contain at least one item")
	}

	request := work.WorkRequestFromSubmitRequests(submissions)
	if strings.TrimSpace(request.RequestID) == "" {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must share a non-empty requestId")
	}
	return request, nil
}

func validateScriptPollerWorkRequest(request work.WorkRequest) error {
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
		return fmt.Errorf("script poller emitted malformed stdout: unsupported work request type %q", request.Type)
	}
	if strings.TrimSpace(request.RequestID) == "" {
		return fmt.Errorf("script poller emitted malformed stdout: work request must set requestId")
	}
	return nil
}
