package script_pollers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func parseScriptPollerOutput(stdout []byte) (work.WorkRequest, bool, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return work.WorkRequest{}, false, nil
	}

	var envelope scriptPollerOutputEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if len(envelope.Events) > 0 {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted unsupported raw factory events")
	}
	if len(envelope.Request) > 0 && len(envelope.Submissions) > 0 {
		return work.WorkRequest{}, true, fmt.Errorf("script poller stdout must contain either request or submissions, not both")
	}
	if len(envelope.Request) > 0 {
		request, err := work.ParseCanonicalWorkRequestJSON(envelope.Request)
		if err != nil {
			return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
		}
		if err := validateScriptPollerWorkRequest(request); err != nil {
			return work.WorkRequest{}, true, err
		}
		return request, true, nil
	}
	if len(envelope.Submissions) > 0 {
		request, err := scriptPollerWorkRequestFromSubmissions(envelope.Submissions)
		if err != nil {
			return work.WorkRequest{}, true, err
		}
		return request, true, nil
	}

	request, err := work.ParseCanonicalWorkRequestJSON(trimmed)
	if err != nil {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if err := validateScriptPollerWorkRequest(request); err != nil {
		return work.WorkRequest{}, true, err
	}
	return request, true, nil
}

type scriptPollerOutputEnvelope struct {
	Request     json.RawMessage `json:"request"`
	Submissions json.RawMessage `json:"submissions"`
	Events      json.RawMessage `json:"events"`
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
