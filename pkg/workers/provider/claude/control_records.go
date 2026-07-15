package claude

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const (
	maximumRetryAttempt = 1_000_000
	maximumRetryDelayMS = int64((24 * time.Hour) / time.Millisecond)
)

type claudeRetryError struct {
	failure adapter.FailureFacts
}

func (*claudeRetryError) Error() string {
	return "Claude reported a retryable provider API failure"
}

func (d *decoder) retryObservation(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	payload := responseevents.ErrorPayload{
		Code: "claude_api_retry", Message: "Claude reported a provider API retry.", Retryable: true,
		RetryAttempt: boundedRetryAttempt(envelope.Attempt),
	}
	if delay := boundedRetryDelay(envelope.RetryDelayMS); delay != nil {
		seconds := int64((*delay + time.Second - 1) / time.Second)
		payload.RetryAfterSeconds = &seconds
	}
	encoded, err := marshalPayload(payload)
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return d.withRunStart(adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind: responseevents.KindError, Phase: responseevents.PhaseUpdated, ProviderSessionRef: d.sessionRef,
		Provenance: provenance("api_retry", responseevents.RepresentationNotification), Payload: encoded,
	}}}), nil
}

func (d *decoder) compactionObservation() (adapter.DecodeResult, error) {
	payload, err := marshalPayload(responseevents.ProgressPayload{
		Label: "context_compaction", Message: "Claude compacted the conversation context.",
	})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return d.withRunStart(adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind: responseevents.KindProgress, Phase: responseevents.PhaseUpdated, ProviderSessionRef: d.sessionRef,
		Provenance: provenance("compact_boundary", responseevents.RepresentationNotification), Payload: payload,
	}}}), nil
}

func claudeRetryFailure(stdout []byte) *adapter.FailureFacts {
	var latest *nativeEnvelope
	for _, line := range bytes.Split(bytes.ReplaceAll(stdout, []byte("\r\n"), []byte("\n")), []byte("\n")) {
		var envelope nativeEnvelope
		if json.Unmarshal(bytes.TrimSpace(line), &envelope) == nil && envelope.Type == "system" && envelope.Subtype == "api_retry" {
			copy := envelope
			latest = &copy
		}
	}
	if latest == nil {
		return nil
	}
	family, failureType := workerexecution.WorkFailureFamilyRetryable, workerexecution.WorkFailureTypeInternalServerError
	if status, ok := boundedStatus(latest.ErrorStatus); ok && status == 429 {
		family, failureType = workerexecution.WorkFailureFamilyThrottle, workerexecution.WorkFailureTypeThrottled
	}
	return &adapter.FailureFacts{
		Family: family, Type: failureType, Message: "Claude reported a retryable provider API failure.",
		Retry:           adapter.RetryGuidance{Retryable: true, RetryAfter: boundedRetryDelay(latest.RetryDelayMS)},
		ProviderSession: providerSession(latest.SessionID),
	}
}

func boundedRetryAttempt(value *int) *int {
	if value == nil || *value < 0 || *value > maximumRetryAttempt {
		return nil
	}
	result := *value
	return &result
}

func boundedRetryDelay(value *int64) *time.Duration {
	if value == nil || *value < 0 || *value > maximumRetryDelayMS {
		return nil
	}
	delay := time.Duration(*value) * time.Millisecond
	return &delay
}

func boundedStatus(raw json.RawMessage) (int, bool) {
	var number int
	if json.Unmarshal(raw, &number) == nil && number >= 100 && number <= 599 {
		return number, true
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.Atoi(strings.TrimSpace(text))
	return number, err == nil && number >= 100 && number <= 599
}

func optionalString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
