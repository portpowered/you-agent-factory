package wire

import (
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	projectionquerywire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/wire"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
)

func TestNewServiceConstructsReplayCapability(t *testing.T) {
	t.Parallel()

	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, nil
		},
	)
	lifecycle := recordinglifecyclewire.NewService(planner, nil, nil)
	projection := projectionquerywire.NewService()
	if service := NewService(lifecycle, projection); service == nil {
		t.Fatal("NewService() = nil")
	}
}
