package recordingmcp

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// LoadReplayInput is the MCP request shape for you.recording.load_replay.
type LoadReplayInput struct {
	RecordingID string `json:"recordingId"`
}

// LoadReplay loads finalized canonical replay facts through the
// you.recording.load_replay MCP tool.
func LoadReplay(
	ctx context.Context,
	service recordings.Service,
	input LoadReplayInput,
) ToolResponse[recordings.LoadReplayRecordingResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.LoadReplayRecordingResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}

	recordingID := recordings.RecordingID(input.RecordingID)
	result, err := service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: recordingID,
	})
	if err != nil {
		envelope := loadReplayErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.LoadReplayRecordingResult]{Error: &envelope}
	}
	return ToolResponse[recordings.LoadReplayRecordingResult]{Result: &result}
}
