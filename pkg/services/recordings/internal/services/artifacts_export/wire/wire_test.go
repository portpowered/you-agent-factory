package wire_test

import (
	"testing"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordinglifecycle "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recording_lifecycle"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
)

type snapshotSourceStub struct{}

func (snapshotSourceStub) Snapshot(recordings.RecordingID) (recordinglifecycle.Snapshot, error) {
	return recordinglifecycle.Snapshot{}, recordings.ErrMissingRecordingTarget
}

func TestNewServiceConstructsArtifactsExportCapability(t *testing.T) {
	t.Parallel()

	if service := artifactsexportwire.NewService(snapshotSourceStub{}); service == nil {
		t.Fatal("NewService() = nil")
	}
}
