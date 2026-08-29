package internal

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
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
		redacted, err := redactedRecordingSnapshot(snapshot)
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		data, err := json.Marshal(redacted)
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
	appendFiles ...func(string, []byte) error,
) recordings.RecordingSnapshotWriter {
	var appendFile func(string, []byte) error
	if len(appendFiles) > 0 && appendFiles[0] != nil {
		appendFile = appendFiles[0]
	}
	return newReplayRecordingSnapshotWriter(write, appendFile, nil)
}

func newReplayRecordingSnapshotWriter(
	write func(string, []byte) error,
	appendFile func(string, []byte) error,
	prepareAppend func(string) error,
) recordings.RecordingSnapshotWriter {
	if write == nil {
		return nil
	}
	var stateMu sync.Mutex
	v2States := make(map[string]*replayV2SnapshotState)
	return func(target string, snapshot recordings.RecordingSnapshot) error {
		redacted, err := redactedRecordingSnapshot(snapshot)
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(target)), ".jsonl") {
			stateMu.Lock()
			defer stateMu.Unlock()
			state := v2States[target]
			if state == nil {
				state = &replayV2SnapshotState{}
				v2States[target] = state
			}
			return writeReplayV2Snapshot(target, redacted, appendFile, prepareAppend, state)
		}

		return writeReplayV1Snapshot(target, redacted, write)
	}
}

type replayV2SnapshotState struct {
	targetPrepared  bool
	headerEmitted   bool
	persistedEvents int
	terminalEmitted bool
}

func writeReplayV1Snapshot(
	target string,
	snapshot recordings.RecordingSnapshot,
	write func(string, []byte) error,
) error {
	events := make([]factorydefinitions.FactoryEvent, len(snapshot.Events))
	for index, event := range snapshot.Events {
		events[index] = canonical.FactoryEventFromCanonical(event)
	}
	recordedAt := time.Time{}
	if len(events) > 0 {
		recordedAt = events[0].Context.EventTime
	}
	wallClock := &recordings.ReplayWallClockMetadata{
		StartedAt: recordedAt,
	}
	if snapshot.Status.FinalizedAt != nil {
		wallClock.FinishedAt = snapshot.Status.FinalizedAt.UTC()
	}
	data, err := replayimpl.MarshalArtifact(&recordings.ReplayArtifact{
		SchemaVersion: replayimpl.CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        events,
		WallClock:     wallClock,
	})
	if err != nil {
		return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
	}
	var persisted recordings.ReplayArtifact
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

func writeReplayV2Snapshot(
	target string,
	snapshot recordings.RecordingSnapshot,
	appendFile func(string, []byte) error,
	prepareAppend func(string) error,
	state *replayV2SnapshotState,
) error {
	if appendFile == nil {
		return fmt.Errorf("%w at %q: append effect is required", recordings.ErrRecordingSnapshotWrite, target)
	}
	events := make([]factorydefinitions.FactoryEvent, len(snapshot.Events))
	for index, event := range snapshot.Events {
		events[index] = canonical.FactoryEventFromCanonical(event)
	}
	recordedAt := time.Time{}
	if len(events) > 0 {
		recordedAt = events[0].Context.EventTime
	}
	if recordedAt.IsZero() && snapshot.Status.FinalizedAt != nil {
		recordedAt = snapshot.Status.FinalizedAt.UTC()
	}
	artifact := &recordings.ReplayArtifact{
		SchemaVersion: replayimpl.CurrentSchemaVersion,
		RecordedAt:    recordedAt,
		Events:        events,
		Factory:       replayFactorySnapshotFromEvents(events),
		WallClock: &recordings.ReplayWallClockMetadata{
			StartedAt: recordedAt,
		},
	}
	if snapshot.Status.FinalizedAt != nil {
		artifact.WallClock.FinishedAt = snapshot.Status.FinalizedAt.UTC()
	}
	if len(events) == 0 && snapshot.Status.FinalizedAt == nil {
		return nil
	}
	if !state.targetPrepared && prepareAppend != nil {
		if err := prepareAppend(target); err != nil {
			return fmt.Errorf("%w at %q: prepare replay v2 target: %w", recordings.ErrRecordingSnapshotWrite, target, err)
		}
		state.targetPrepared = true
	}

	if err := writeReplayV2Header(target, snapshot, artifact, appendFile, state); err != nil {
		return err
	}
	if err := writeReplayV2Events(target, events, appendFile, state); err != nil {
		return err
	}
	return writeReplayV2Terminal(target, snapshot, appendFile, state)
}

func writeReplayV2Header(
	target string,
	snapshot recordings.RecordingSnapshot,
	artifact *recordings.ReplayArtifact,
	appendFile func(string, []byte) error,
	state *replayV2SnapshotState,
) error {
	if state.headerEmitted {
		return nil
	}
	headerSessionID := strings.TrimSpace(snapshot.CanonicalSessionID)
	if headerSessionID == "" {
		headerSessionID = snapshot.Status.Scope.FactorySessionID
	}
	line, err := replayimpl.MarshalReplayV2Header(artifact, headerSessionID)
	if err != nil {
		return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
	}
	if err := appendReplayV2Line(target, line, appendFile); err != nil {
		return err
	}
	state.headerEmitted = true
	return nil
}

func writeReplayV2Events(
	target string,
	events []factorydefinitions.FactoryEvent,
	appendFile func(string, []byte) error,
	state *replayV2SnapshotState,
) error {
	if state.persistedEvents > len(events) {
		return fmt.Errorf(
			"%w at %q: persisted event boundary %d exceeds snapshot event count %d",
			recordings.ErrRecordingSnapshotWrite,
			target,
			state.persistedEvents,
			len(events),
		)
	}
	for state.persistedEvents < len(events) {
		line, err := replayimpl.MarshalReplayV2Event(events[state.persistedEvents])
		if err != nil {
			return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
		}
		if err := appendReplayV2Line(target, line, appendFile); err != nil {
			return err
		}
		state.persistedEvents++
	}
	return nil
}

func writeReplayV2Terminal(
	target string,
	snapshot recordings.RecordingSnapshot,
	appendFile func(string, []byte) error,
	state *replayV2SnapshotState,
) error {
	if snapshot.Status.FinalizedAt == nil || state.terminalEmitted {
		return nil
	}
	line, err := replayimpl.MarshalReplayV2Terminal(
		snapshot.Status.FinalizedAt.UTC(),
		string(snapshot.Status.State),
		replayV2FlushDiagnostics(snapshot.Status.Failures),
	)
	if err != nil {
		return fmt.Errorf("%w: %w", recordings.ErrRecordingSnapshotEncoding, err)
	}
	if err := appendReplayV2Line(target, line, appendFile); err != nil {
		return err
	}
	state.terminalEmitted = true
	return nil
}

func appendReplayV2Line(
	target string,
	line []byte,
	appendFile func(string, []byte) error,
) error {
	if err := appendFile(target, line); err != nil {
		return fmt.Errorf("%w at %q: append replay v2 record: %w", recordings.ErrRecordingSnapshotWrite, target, err)
	}
	return nil
}

func replayFactorySnapshotFromEvents(
	events []factorydefinitions.FactoryEvent,
) *factorydefinitions.FactorySnapshot {
	for _, event := range events {
		if event.Type != factorydefinitions.FactoryEventTypeRunRequest {
			continue
		}
		var payload struct {
			Factory *factorydefinitions.FactorySnapshot `json:"factory"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil
		}
		return payload.Factory.Clone()
	}
	return nil
}

func replayV2FlushDiagnostics(
	failures []recordings.RecordingFailure,
) replayimpl.ReplayV2FlushDiagnostics {
	diagnostics := replayimpl.ReplayV2FlushDiagnostics{FailureCount: len(failures)}
	for _, failure := range failures {
		diagnostics.FailureCodes = append(diagnostics.FailureCodes, failure.Code)
	}
	return diagnostics
}

func redactedRecordingSnapshot(
	snapshot recordings.RecordingSnapshot,
) (recordings.RecordingSnapshot, error) {
	events, _, err := recordings.RedactCanonicalEvents(snapshot.Events, snapshot.SecretProvenance)
	if err != nil {
		return recordings.RecordingSnapshot{}, err
	}
	return recordings.RecordingSnapshot{
		Status:             snapshot.Status,
		Events:             events,
		CanonicalSessionID: snapshot.CanonicalSessionID,
	}, nil
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
