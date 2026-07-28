package recordingmcp

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// ReadPortableArtifactInput is the MCP request shape for
// you.recording.read_portable_artifact.
type ReadPortableArtifactInput struct {
	RecordingID string `json:"recordingId"`
	Reference   string `json:"reference"`
}

// ReadPortableArtifact reads and validates one published portable artifact
// through the you.recording.read_portable_artifact MCP tool.
func ReadPortableArtifact(
	ctx context.Context,
	service recordings.Service,
	input ReadPortableArtifactInput,
) ToolResponse[recordings.ReadPortableArtifactResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(input.RecordingID, errMissingRequestContext)
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[recordings.ReadPortableArtifactResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}

	result, err := service.ReadPortableArtifact(ctx, recordings.ReadPortableArtifactRequest{
		RecordingID: recordings.RecordingID(input.RecordingID),
		Reference:   recordings.RecordingArtifactReference(input.Reference),
	})
	if err != nil {
		envelope := readPortableArtifactErrorEnvelope(input.RecordingID, err)
		return ToolResponse[recordings.ReadPortableArtifactResult]{Error: &envelope}
	}
	return ToolResponse[recordings.ReadPortableArtifactResult]{Result: &result}
}
