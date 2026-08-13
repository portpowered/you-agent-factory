package wire

import (
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestProvideRecordingsRootConstructsThroughRecordingsWire(t *testing.T) {
	t.Parallel()

	root, err := provideRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("provideRecordingsRoot() returned nil root")
	}
	var published recordings.Service = root
	if _, err := published.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-factory-root",
	}); err == nil {
		t.Fatal("LoadReplayRecording() error = nil, want missing recording failure")
	}
}
