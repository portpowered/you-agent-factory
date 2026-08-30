package runtimeopening

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// newOrderlyRecordingFlush adapts the already-composed Recordings root to the
// initializer lifecycle boundary. A missing record path means that this
// runtime has no live recording to flush, so the orderly-stop phase remains a
// no-op.
func newOrderlyRecordingFlush(
	service recordings.Service,
	recordingID string,
	recordPath string,
) lifecycle.OrderlyStopOperation {
	if service == nil || strings.TrimSpace(recordingID) == "" || strings.TrimSpace(recordPath) == "" {
		return nil
	}
	return func(ctx context.Context) error {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if _, err := service.FlushRecording(recordings.FlushRecordingRequest{
			RecordingID: recordings.RecordingID(recordingID),
		}); err != nil {
			return fmt.Errorf("flush live recording during orderly shutdown: %w", err)
		}
		return nil
	}
}
