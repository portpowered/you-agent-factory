package recordingmcp

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// QueryHistoryInput is the MCP request shape for
// you.recording.query_history. The identity fields are explicit so a caller
// cannot accidentally read a recording outside its Factory Session scope.
type QueryHistoryInput struct {
	RecordingID      string `json:"recordingId"`
	Artifact         string `json:"artifact"`
	FactorySessionID string `json:"factorySessionId"`
}

// QueryHistory returns ordered canonical history and detached projections
// through the you.recording.query_history MCP tool.
func QueryHistory(
	ctx context.Context,
	service recordings.Service,
	input QueryHistoryInput,
) ToolResponse[recordings.HistoricalRecordingQueryResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.HistoricalRecordingQueryResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}

	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID(strings.TrimSpace(input.RecordingID)),
			Artifact:    recordings.RecordingArtifactReference(strings.TrimSpace(input.Artifact)),
			Scope: recordings.CanonicalEventScope{
				FactorySessionID: strings.TrimSpace(input.FactorySessionID),
			},
		},
	})
	if err != nil {
		envelope := historicalQueryErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.HistoricalRecordingQueryResult]{Error: &envelope}
	}
	return ToolResponse[recordings.HistoricalRecordingQueryResult]{Result: &result}
}

func historicalQueryErrorEnvelope(recordingID string, err error) ToolErrorEnvelope {
	var typed *recordings.HistoricalRecordingQueryError
	if errors.As(err, &typed) {
		code := "recording.history.internal"
		message := "historical recording query failed"
		retryable := false
		switch typed.Kind {
		case recordings.HistoricalRecordingQueryErrorInvalidRequest:
			code = "recording.history.invalid"
			message = "invalid historical recording query"
		case recordings.HistoricalRecordingQueryErrorMissingHistory:
			code = "recording.history.not_found"
			message = "historical recording history not found"
		case recordings.HistoricalRecordingQueryErrorCorruptHistory:
			code = "recording.history.corrupt"
			message = "historical recording history is corrupt"
		case recordings.HistoricalRecordingQueryErrorUnavailable:
			code = "recording.history.unavailable"
			message = "historical recording history is unavailable"
			retryable = true
		}
		return ToolErrorEnvelope{
			Code:        code,
			Message:     message,
			Retryable:   retryable,
			RecordingID: strings.TrimSpace(recordingID),
			Details: map[string]any{
				"reason": string(typed.Kind),
			},
		}
	}
	return executionErrorEnvelope(recordingID, err)
}
