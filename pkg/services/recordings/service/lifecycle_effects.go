package service

import (
	"encoding/json"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay"
)

// NewRecordingSnapshotWriter adapts policy-free byte persistence to the
// Recordings-owned lifecycle snapshot format.
func NewRecordingSnapshotWriter(
	write func(string, []byte) error,
) recordings.RecordingSnapshotWriter {
	if write == nil {
		return nil
	}
	return func(target string, snapshot recordings.RecordingSnapshot) error {
		data, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		if err := write(target, data); err != nil {
			return fmt.Errorf(
				"%w at %q: %w",
				recordings.ErrRecordingSnapshotWrite,
				target,
				err,
			)
		}
		return nil
	}
}

// NewReplayRecordingSnapshotWriter persists lifecycle snapshots in the
// replay-compatible artifact format consumed by existing recording readers.
func NewReplayRecordingSnapshotWriter(
	write func(string, []byte) error,
) recordings.RecordingSnapshotWriter {
	if write == nil {
		return nil
	}
	return func(target string, snapshot recordings.RecordingSnapshot) error {
		events := make([]factorydefinitions.FactoryEvent, len(snapshot.Events))
		for index, event := range snapshot.Events {
			events[index] = canonical.FactoryEventFromCanonical(event)
		}
		recordedAt := time.Time{}
		if len(events) > 0 {
			recordedAt = events[0].Context.EventTime
		}
		wallClock := &factorydefinitions.ReplayWallClockMetadata{
			StartedAt: recordedAt,
		}
		if snapshot.Status.FinalizedAt != nil {
			wallClock.FinishedAt = snapshot.Status.FinalizedAt.UTC()
		}
		data, err := replay.MarshalArtifact(&factorydefinitions.ReplayArtifact{
			SchemaVersion: replay.CurrentSchemaVersion,
			RecordedAt:    recordedAt,
			Events:        events,
			WallClock:     wallClock,
		})
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		var persisted factorydefinitions.ReplayArtifact
		if err := json.Unmarshal(data, &persisted); err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		persisted.Events = events
		data, err = json.MarshalIndent(&persisted, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		data = append(data, '\n')
		if err := write(target, data); err != nil {
			return fmt.Errorf(
				"%w at %q: %w",
				recordings.ErrRecordingSnapshotWrite,
				target,
				err,
			)
		}
		return nil
	}
}

// NewRecordingFlushTickerFactory binds the real scheduling edge selected by
// Wire without embedding wall-clock scheduling in lifecycle policy.
func NewRecordingFlushTickerFactory() recordings.RecordingFlushTickerFactory {
	return func(interval time.Duration) recordings.RecordingFlushTicker {
		ticker := time.NewTicker(interval)
		return recordings.RecordingFlushTicker{
			Ticks: ticker.C,
			Stop:  ticker.Stop,
		}
	}
}
