package wire_test

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

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

type recordingLedger struct {
	subscribeCalls int
}

func (ledger *recordingLedger) CanonicalEvents() []factorydefinitions.FactoryEvent {
	return nil
}

func (ledger *recordingLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	ledger.subscribeCalls++
	panic("ledger subscription started during inert construction")
}

func (ledger *recordingLedger) StreamGenerationID() string { return "wire-test-generation" }

func (ledger *recordingLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {
	panic("ledger event recorder registered during inert construction")
}

func (ledger *recordingLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {
	panic("ledger event-type recorder registered during inert construction")
}

func (ledger *recordingLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {
	panic("ledger append during inert construction")
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	ledger := &recordingLedger{}
	writeCalls := 0
	writeFile := func(string, []byte) error {
		writeCalls++
		panic("snapshot write during inert construction")
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	makeDirectories, createTemporaryFile, removePath, renamePath, readFile := testPublicationEffects()
	service, err := recordingswire.NewService(
		ledger,
		nil,
		writeFile,
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	if ledger.subscribeCalls != 0 {
		t.Fatalf("construction started ledger subscriptions %d times, want inert construction", ledger.subscribeCalls)
	}
	if writeCalls != 0 {
		t.Fatalf("construction wrote snapshots %d times, want inert construction", writeCalls)
	}

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	if leaked := runtime.NumGoroutine() - baseline; leaked > 4 {
		t.Fatalf(
			"goroutine leak after construction: baseline=%d current=%d delta=%d, want no flush ticker goroutines",
			baseline, runtime.NumGoroutine(), leaked,
		)
	}

	var root recordings.Service = service
	if _, err := root.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-after-inert-construction",
	}); !errors.Is(err, recordings.ErrReplayRecordingNotFound) {
		t.Fatalf("LoadReplayRecording() = %v, want ErrReplayRecordingNotFound after inert construction", err)
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	validLedger := stubLedger{}
	validWriteFile := func(string, []byte) error { return nil }
	tests := []struct {
		name      string
		ledger    recordings.Ledger
		writeFile func(string, []byte) error
		wantErr   string
	}{
		{
			name:      "ledger",
			ledger:    nil,
			writeFile: validWriteFile,
			wantErr:   "construct Recordings: ledger is required",
		},
		{
			name:      "snapshot write function",
			ledger:    validLedger,
			writeFile: nil,
			wantErr:   "construct Recordings: snapshot write function is required",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			makeDirectories, createTemporaryFile, removePath, renamePath, readFile := testPublicationEffects()
			service, err := recordingswire.NewService(
				test.ledger,
				nil,
				test.writeFile,
				makeDirectories,
				createTemporaryFile,
				removePath,
				renamePath,
				readFile,
			)
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if err.Error() != test.wantErr {
				t.Fatalf("NewService() error = %q, want %q", err.Error(), test.wantErr)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewService(
		stubLedger{},
		nil,
		func(string, []byte) error { return nil },
		func(string, os.FileMode) error { return nil },
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
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

func TestNewServiceRejectsMissingArtifactPublicationEffects(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewService(
		stubLedger{},
		nil,
		func(string, []byte) error { return nil },
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("NewService() error = nil, want missing artifact publication effects")
	}
	if err.Error() != "construct Recordings publication: portable artifact publication operations are required" {
		t.Fatalf("NewService() error = %q, want missing publication effects", err.Error())
	}
	if service != nil {
		t.Fatalf("NewService() = %#v, want nil service", service)
	}
}

func testPublicationEffects() (
	recordings.RecordingMakeDirectories,
	recordings.RecordingCreateTemporaryFile,
	recordings.RecordingRemovePath,
	recordings.RecordingRenamePath,
	recordings.RecordingReadFile,
) {
	return os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile
}
