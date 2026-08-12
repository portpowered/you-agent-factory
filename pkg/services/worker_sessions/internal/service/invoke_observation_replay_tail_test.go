package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestReplayObservationSubscriptionCancellationCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	subscription, err := newReplayObservationSubscription(context.Background(), &observationEventReaderFake{readResults: []events.ReadResult{initial}}, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(canceled) error = %v", err)
	}
	if got := subscription.Next(canceled); got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled delivery = %#v, want CANCELED", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after cancellation = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionCloseDuringReadCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	var racing *replayObservationSubscription
	reads := 0
	racingReader := &observationEventReaderFake{}
	racingReader.readFunc = func(context.Context, events.ReadRequest) (events.ReadResult, error) {
		reads++
		if reads == 1 {
			return initial, nil
		}
		racing.Close()
		return replayProgressResult(topic, 2, 2), nil
	}
	var err error
	racing, err = newReplayObservationSubscription(context.Background(), racingReader, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(racing) error = %v", err)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("racing initial delivery = %#v, want RECORD", got)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("racing delivery = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionRejectsSnapshotInvariantViolations(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	subscription := &replayObservationSubscription{
		topic:        topic,
		snapshotHead: 1,
		next:         events.Cursor{Topic: topic, Position: 1},
	}
	result := replayProgressResult(topic, 2, 2)
	if err := subscription.appendPage(result); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("appendPage(after snapshot) error = %v, want source unavailable", err)
	}
}

func replayProgressResult(topic events.Topic, head uint64, positions ...uint64) events.ReadResult {
	records := make([]events.Record, 0, len(positions))
	for _, position := range positions {
		records = append(records, replayObservationRecord(topic, position, fmt.Sprintf("event-%d", position)))
	}
	return events.ReadResult{
		Outcome:  events.ReadOutcomeProgress,
		Records:  records,
		Next:     events.Cursor{Topic: topic, Position: events.AggregateSequence(positions[len(positions)-1])},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: events.AggregateSequence(head)},
	}
}

func replayObservationRecord(topic events.Topic, position uint64, eventID string) events.Record {
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: events.AggregateSequence(position)},
		SourceType:     "worker_observation",
		SourceID:       "provider-session-1",
		SourceSequence: events.SourceSequence(position),
		SourceEventID:  events.SourceEventID(eventID),
		SchemaID:       "worker_session.observation",
		Payload:        []byte(`{"position":1}`),
	}
}

func timePointer(value time.Time) *time.Time { return &value }
