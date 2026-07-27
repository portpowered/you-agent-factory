// Package wire constructs the Recordings recording-lifecycle subservice.
package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/internal/service"
)

// NewService constructs the private lifecycle owner from the exact target
// planner selected by the application graph.
func NewService(targets recordings.LiveRecordingTargetPlanner) recordinglifecycle.Service {
	return lifecycleservice.New(targets)
}
