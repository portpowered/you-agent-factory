package pi

import (
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

type piRetryError struct {
	failure adapter.FailureFacts
}

func (*piRetryError) Error() string {
	return "Pi reported a retryable provider API failure"
}

func (d *decoder) compactionObservation(nativeType, message string) (adapter.DecodeResult, error) {
	payload, err := marshalPayload(responseevents.ProgressPayload{
		Label: "context_compaction", Message: message,
	})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindProgress, Phase: responseevents.PhaseUpdated,
		ProviderSessionRef: d.sessionRef, Provenance: provenance(nativeType, responseevents.RepresentationNotification), Payload: payload,
	})
}

func (d *decoder) retryObservation(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	payload := responseevents.ErrorPayload{
		Code: "pi_api_retry", Message: "Pi reported a provider API retry.", Retryable: true,
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
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindError, Phase: responseevents.PhaseUpdated,
		ProviderSessionRef: d.sessionRef, Provenance: provenance("auto_retry_start", responseevents.RepresentationNotification), Payload: encoded,
	})
}

func piRetryFailure(stdout []byte) *adapter.FailureFacts {
	var latest *nativeEnvelope
	forEachRecord(stdout, func(raw []byte) {
		var envelope nativeEnvelope
		if json.Unmarshal(raw, &envelope) != nil || envelope.Type != "auto_retry_start" {
			return
		}
		copy := envelope
		latest = &copy
	})
	if latest == nil {
		return nil
	}
	family, failureType := workerexecution.WorkFailureFamilyRetryable, workerexecution.WorkFailureTypeInternalServerError
	if status, ok := boundedStatus(latest.ErrorStatus); ok && status == 429 {
		family, failureType = workerexecution.WorkFailureFamilyThrottle, workerexecution.WorkFailureTypeThrottled
	}
	return &adapter.FailureFacts{
		Family: family, Type: failureType, Message: "Pi reported a retryable provider API failure.",
		Retry:           adapter.RetryGuidance{Retryable: true, RetryAfter: boundedRetryDelay(latest.RetryDelayMS)},
		ProviderSession: providerSession(strings.TrimSpace(latest.ID)),
	}
}

func piRetryFailureFromStdout(stdout []byte) *adapter.FailureFacts {
	failure := piRetryFailure(stdout)
	if failure == nil {
		return nil
	}
	if failure.ProviderSession == nil {
		if sessionID := sessionIDFromRecord(stdout); sessionID != "" {
			failure.ProviderSession = providerSession(sessionID)
		}
	}
	return failure
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
	if len(raw) == 0 {
		return 0, false
	}
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

func sessionIDFromRecord(stdout []byte) string {
	var sessionID string
	forEachRecord(stdout, func(raw []byte) {
		if sessionID != "" {
			return
		}
		var record struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(raw, &record) == nil && record.Type == "session" {
			sessionID = strings.TrimSpace(record.ID)
		}
	})
	return sessionID
}
