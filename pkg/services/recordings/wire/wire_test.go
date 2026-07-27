package wire_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type stubLedger struct{}

func (stubLedger) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }

func (stubLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{}, nil
}

func (stubLedger) StreamGenerationID() string { return "wire-test-generation" }

func (stubLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (stubLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (stubLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewService(
		stubLedger{},
		nil,
		func(string, []byte) error { return nil },
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root recordings.Service = service
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-root",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording() = %v, want ErrReplayRecordingNotFound", err)
	}
}
