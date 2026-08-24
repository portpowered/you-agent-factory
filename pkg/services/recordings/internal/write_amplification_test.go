package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

const (
	measurementRecordedAt = "2026-08-23T17:00:00Z"
	measurementSessionID  = "measurement-session"
	measurementGeneration = "measurement-generation"
	measurementEventBody  = `{"fixed":"payload-with-a-deterministic-size"}`
)

type writeAmplificationMeasurement struct {
	eventCount    int
	v1Bytes       int64
	v2Bytes       int64
	v1Growth      float64
	v2Growth      float64
	improvement   float64
	v1FinalBytes  int
	v2FinalBytes  int
	v1WriteCalls  int
	v2AppendCalls int
}

type successfulWriteCapture struct {
	data                []byte
	bytes               int64
	writeCalls          int
	appendCalls         int
	forbidReplaceWrites bool
}

func TestReplayWriteAmplificationMeasurement(t *testing.T) {
	t.Parallel()

	counts := []int{10, 100, 1000}
	measurements := make([]writeAmplificationMeasurement, 0, len(counts))
	for _, eventCount := range counts {
		v1 := measureReplaySnapshotWrites(t, eventCount, false)
		v2 := measureReplaySnapshotWrites(t, eventCount, true)
		if v2.writeCalls != 0 {
			t.Fatalf("N=%d v2 replacement write calls = %d, want 0", eventCount, v2.writeCalls)
		}
		if v2.appendCalls != eventCount+2 {
			t.Fatalf("N=%d v2 append calls = %d, want header, %d events, and terminal", eventCount, v2.appendCalls, eventCount)
		}
		if v2.bytes != int64(len(v2.data)) {
			t.Fatalf("N=%d v2 submitted bytes = %d, final artifact bytes = %d; append-only writes must equal final size", eventCount, v2.bytes, len(v2.data))
		}
		if v1.bytes <= int64(len(v1.data)) {
			t.Fatalf("N=%d v1 submitted bytes = %d, final artifact bytes = %d; v1 must rewrite accumulated history", eventCount, v1.bytes, len(v1.data))
		}

		measurement := writeAmplificationMeasurement{
			eventCount:    eventCount,
			v1Bytes:       v1.bytes,
			v2Bytes:       v2.bytes,
			v1FinalBytes:  len(v1.data),
			v2FinalBytes:  len(v2.data),
			v1WriteCalls:  v1.writeCalls,
			v2AppendCalls: v2.appendCalls,
			improvement:   float64(v1.bytes) / float64(v2.bytes),
		}
		if len(measurements) > 0 {
			previous := measurements[len(measurements)-1]
			measurement.v1Growth = float64(v1.bytes) / float64(previous.v1Bytes)
			measurement.v2Growth = float64(v2.bytes) / float64(previous.v2Bytes)
		}
		measurements = append(measurements, measurement)

		assertMeasuredReplayArtifacts(t, eventCount, v1.data, v2.data)
	}

	// Growth is measured against the preceding tenfold corpus size. A v1
	// whole-file flush therefore grows approximately 100x, while v2 grows
	// approximately 10x plus its constant framing overhead.
	if measurements[1].v1Growth <= 25 || measurements[2].v1Growth <= 25 {
		t.Fatalf("v1 growth ratios = %.1fx, %.1fx, want quadratic amplification", measurements[1].v1Growth, measurements[2].v1Growth)
	}
	if measurements[1].v2Growth >= 20 || measurements[2].v2Growth >= 20 {
		t.Fatalf("v2 growth ratios = %.1fx, %.1fx, want linear growth", measurements[1].v2Growth, measurements[2].v2Growth)
	}
	for _, measurement := range measurements {
		if measurement.improvement <= 1 {
			t.Fatalf("N=%d improvement ratio = %.1fx, want v2 below v1", measurement.eventCount, measurement.improvement)
		}
	}

	t.Log("flush schedule: one snapshot flush after every event; final flush adds one terminal record")
	t.Log("growth columns: total submitted bytes divided by the preceding tenfold corpus total; first row is baseline")
	t.Log("N | v1 bytes written | v2 bytes written | v1 growth | v2 growth | improvement ratio")
	for index, measurement := range measurements {
		v1Growth := "baseline"
		v2Growth := "baseline"
		if index > 0 {
			v1Growth = fmt.Sprintf("%.1fx", measurement.v1Growth)
			v2Growth = fmt.Sprintf("%.1fx", measurement.v2Growth)
		}
		t.Logf("%d | %d | %d | %s | %s | %.1fx", measurement.eventCount, measurement.v1Bytes, measurement.v2Bytes, v1Growth, v2Growth, measurement.improvement)
	}
}

func measureReplaySnapshotWrites(t *testing.T, eventCount int, v2 bool) successfulWriteCapture {
	t.Helper()

	capture := successfulWriteCapture{forbidReplaceWrites: v2}
	writer := NewReplayRecordingSnapshotWriter(
		func(_ string, data []byte) error {
			capture.writeCalls++
			if capture.forbidReplaceWrites {
				return errors.New("v2 measurement forbids replacement writes")
			}
			capture.bytes += int64(len(data))
			capture.data = append([]byte(nil), data...)
			return nil
		},
		func(_ string, data []byte) error {
			capture.appendCalls++
			capture.bytes += int64(len(data))
			capture.data = append(capture.data, data...)
			return nil
		},
	)
	path := fmt.Sprintf("measurement-%d", eventCount)
	if v2 {
		path += ".jsonl"
	} else {
		path += ".json"
	}
	for index := 1; index <= eventCount; index++ {
		finalized := index == eventCount
		if err := writer(path, measurementSnapshot(t, index, finalized)); err != nil {
			t.Fatalf("measure N=%d v2=%t flush %d: %v", eventCount, v2, index, err)
		}
	}
	if capture.bytes == 0 || len(capture.data) == 0 {
		t.Fatalf("measure N=%d v2=%t produced no successful recording bytes", eventCount, v2)
	}
	return capture
}

func measurementSnapshot(t *testing.T, eventCount int, finalized bool) recordings.RecordingSnapshot {
	t.Helper()

	recordedAt, err := time.Parse(time.RFC3339, measurementRecordedAt)
	if err != nil {
		t.Fatalf("parse measurement time: %v", err)
	}
	factory, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name":         "measurement-factory",
		"workTypes":    []map[string]any{{"name": "task"}},
		"workers":      []map[string]any{{"name": "worker"}},
		"workstations": []map[string]any{{"name": "process", "worker": "worker"}},
	})
	if err != nil {
		t.Fatalf("build measurement Factory snapshot: %v", err)
	}
	artifact, err := replayimpl.NewEventLogArtifact(
		recordedAt,
		factory,
		&recordings.ReplayWallClockMetadata{StartedAt: recordedAt},
		factorydefinitions.ReplayDiagnostics{},
	)
	if err != nil {
		t.Fatalf("build measurement replay shell: %v", err)
	}

	events := make([]recordings.CanonicalEvent, eventCount)
	sessionID := measurementSessionID
	for index := 0; index < eventCount; index++ {
		event := artifact.Events[0]
		if index > 0 {
			event = factorydefinitions.FactoryEvent{
				Id:            fmt.Sprintf("measurement-event-%04d", index),
				SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
				Type:          factorydefinitions.FactoryEventTypeWorkRequest,
				Context: factorydefinitions.FactoryEventContext{
					EventTime: recordedAt,
					Sequence:  index,
					SessionID: &sessionID,
					Tick:      1,
				},
				Payload: []byte(measurementEventBody),
			}
		} else {
			event.Context.Sequence = 0
			event.Context.SessionID = &sessionID
		}
		events[index] = canonical.CanonicalEventFromFactory(event, measurementGeneration)
	}

	status := recordings.RecordingStatusFacts{
		RecordingID:    recordings.RecordingID("measurement-recording"),
		Scope:          recordings.CanonicalEventScope{FactorySessionID: sessionID},
		State:          recordings.RecordingActive,
		AcceptedEvents: eventCount,
	}
	if finalized {
		finishedAt := recordedAt.Add(time.Minute)
		status.State = recordings.RecordingFinalized
		status.FinalizedAt = &finishedAt
	}
	return recordings.RecordingSnapshot{Status: status, Events: events}
}

func assertMeasuredReplayArtifacts(t *testing.T, eventCount int, v1Data, v2Data []byte) {
	t.Helper()

	var v1Artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(v1Data, &v1Artifact); err != nil {
		t.Fatalf("decode measured v1 artifact N=%d: %v", eventCount, err)
	}
	if len(v1Artifact.Events) != eventCount {
		t.Fatalf("measured v1 event count N=%d = %d, want %d", eventCount, len(v1Artifact.Events), eventCount)
	}
	v2Stream, err := replayimpl.ParseReplayV2(v2Data)
	if err != nil {
		t.Fatalf("decode measured v2 artifact N=%d: %v", eventCount, err)
	}
	if len(v2Stream.Events) != eventCount || v2Stream.Terminal == nil || v2Stream.TruncatedTail {
		t.Fatalf("measured v2 stream N=%d = events=%d terminal=%t truncated=%t", eventCount, len(v2Stream.Events), v2Stream.Terminal != nil, v2Stream.TruncatedTail)
	}
}
