package wire_test

import (
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecyclewire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/wire"
)

func TestNewServiceConstructsLifecycleOwner(t *testing.T) {
	t.Parallel()

	planner := recordings.LiveRecordingTargetPlannerFunc(
		func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
			return recordings.LiveRecordingTarget{}, nil
		},
	)
	if service := recordinglifecyclewire.NewService(planner, nil, nil); service == nil {
		t.Fatal("NewService returned nil")
	}
}
