// Package wire constructs the Recordings recording-lifecycle subservice.
package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	lifecycleservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle/internal/service"
)

// NewTargetPlanner constructs the private lifecycle target planner selected by
// the Recordings composition layer without exposing its implementation package.
func NewTargetPlanner(
	clock recordings.RecordingClock,
	newID recordings.RecordingIdentityGenerator,
	join recordings.RecordingPathJoiner,
) recordings.LiveRecordingTargetPlanner {
	return lifecycleservice.NewTargetPlanner(clock, newID, join)
}

// NewService constructs the private lifecycle owner from the exact target
// planner selected by the application graph.
func NewService(
	targets recordings.LiveRecordingTargetPlanner,
	writer recordings.RecordingSnapshotWriter,
	tickers recordings.RecordingFlushTickerFactory,
	clocks ...recordings.RecordingClock,
) recordinglifecycle.Service {
	return lifecycleservice.New(targets, writer, tickers, clocks...)
}
