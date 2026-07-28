package recordingmcp

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// QueryStatusInput is the MCP request shape for you.recording.query_status.
type QueryStatusInput struct {
	RecordingID string `json:"recordingId"`
}

// QueryStatus returns detached recording lifecycle status through the
// you.recording.query_status MCP tool.
func QueryStatus(
	ctx context.Context,
	service recordings.Service,
	input QueryStatusInput,
) ToolResponse[recordings.RecordingStatusResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.RecordingStatusResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}

	recordingID := recordings.RecordingID(input.RecordingID)
	result, err := service.QueryRecordingStatus(recordings.RecordingStatusRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		envelope := statusQueryErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.RecordingStatusResult]{Error: &envelope}
	}
	return ToolResponse[recordings.RecordingStatusResult]{Result: &result}
}
