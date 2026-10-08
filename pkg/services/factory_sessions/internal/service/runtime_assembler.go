package service

import (
	"context"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"strings"
	"sync"

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

// successorBoardStartup selects a resumed writer only after its initial
// publication succeeds. Failed publication leaves the previous board selected.
type successorBoardStartup struct {
	roles.LifecycleRuntime
	publish func(context.Context) error
	once    sync.Once
	err     error
}

func (startup *successorBoardStartup) CompleteStartup(ctx context.Context) error {
	startup.once.Do(func() {
		startup.err = startup.LifecycleRuntime.CompleteStartup(ctx)
		if startup.err == nil {
			if err := startup.publish(ctx); err != nil {
				startup.err = startup.FailStartup(err)
			}
		}
	})
	return startup.err
}
