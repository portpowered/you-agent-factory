package wire

import (
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type recordingsFactoryLedger struct {
	recordings.Ledger
}

func TestProvideRecordingsFactoryConstructsThroughRecordingsWire(t *testing.T) {
	t.Parallel()

	factory := provideRecordingsFactory(
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{},
	)
	service := factory(
		&recordingsFactoryLedger{},
		recordingswire.NewProjectionService(),
	)
	if service == nil {
		t.Fatal("provideRecordingsFactory() returned nil service")
	}
	var published recordings.Service = service
	if _, err := published.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-factory-root",
	}); err == nil {
		t.Fatal("LoadReplayRecording() error = nil, want missing recording failure")
	}
}
