package service

import (
	"encoding/json"
	"fmt"
	"time"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
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
			return fmt.Errorf("encode recording snapshot: %w", err)
		}
		if err := write(target, data); err != nil {
			return fmt.Errorf("write recording snapshot %q: %w", target, err)
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
