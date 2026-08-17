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

func TestWireUsesPrecomposedRecordingsRuntimeAndMCPRoles(t *testing.T) {
	t.Parallel()

	root, err := provideRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{}, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	opening, err := provideRecordingsRuntimeOpening(root)
	if err != nil || opening == nil {
		t.Fatalf("provideRecordingsRuntimeOpening(root) = %v, %v; want runtime opening", opening, err)
	}
	if _, err := provideRecordingsRuntimeOpening(nil); err == nil {
		t.Fatal("provideRecordingsRuntimeOpening(nil) error = nil, want capability validation")
	}

	buildServer := provideMCPServerBuilder()
	if buildServer == nil {
		t.Fatal("provideMCPServerBuilder() returned nil")
	}
	if server, err := buildServer(nil, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(nil roles) = %v, %v; want inert protocol server", server, err)
	}
	if server, err := buildServer(nil, root, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(recordings root) = %v, %v; want owner-backed protocol server", server, err)
	}

	if _, err := provideHTTPRuntimeBinding(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideHTTPRuntimeBinding(nil roles) error = nil, want required-owner validation")
	}
}
