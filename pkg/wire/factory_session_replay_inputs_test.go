package wire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// TestProvideFactorySessionReplayInputsClassifiesPortableRecording proves the
// Wire-composed ReplayInputLoader capability -- built from the existing replay
// artifact loader and the Factory Session replay recording reader -- reads a
// real portable JavaScript Factory Session recording from disk and decodes
// it, without the caller assembling the raw reader and decoder itself.
func TestProvideFactorySessionReplayInputsClassifiesPortableRecording(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	loadReplay := provideReplayArtifactLoader(platformreplay.Local{})
	replayFiles := provideFactorySessionReplayRecordingReader(serviceedges.Edges{})
	capability := provideFactorySessionReplayInputs(loadReplay, replayFiles, platformfilesystem.Local{}.Open, logging.NoopLogger{})

	result, err := capability.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Legacy != nil {
		t.Fatal("Legacy = non-nil, want nil for a portable recording")
	}
	if result.Portable == nil {
		t.Fatal("Portable = nil, want decoded portable recording")
	}
	if got := result.Portable.Session.ID; got != "session-js-001" {
		t.Fatalf("Portable.Session.ID = %q, want session-js-001", got)
	}
}

// TestProvideFactorySessionReplayInputsDelegatesLegacyArtifact proves the
// same Wire-composed capability falls back to the existing legacy replay
// artifact loader for a file that is not a portable recording.
func TestProvideFactorySessionReplayInputsDelegatesLegacyArtifact(t *testing.T) {
	t.Parallel()

	overrideCalled := false
	edges := serviceedges.Edges{
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			overrideCalled = true
			return os.ReadFile(path)
		},
	}
	loadReplay := recordings.ReplayArtifactLoader(func(path string) (*recordings.ReplayArtifact, error) {
		return &recordings.ReplayArtifact{SchemaVersion: "legacy"}, nil
	})
	replayFiles := provideFactorySessionReplayRecordingReader(edges)
	capability := provideFactorySessionReplayInputs(loadReplay, replayFiles, platformfilesystem.Local{}.Open, logging.NoopLogger{})

	tempFile := filepath.Join(t.TempDir(), "legacy-replay.json")
	if err := os.WriteFile(tempFile, []byte(`{"schemaVersion":"legacy"}`), 0o600); err != nil {
		t.Fatalf("write legacy replay fixture: %v", err)
	}

	result, err := capability.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if !overrideCalled {
		t.Fatal("Factory Session replay recording reader edge override was not used")
	}
	if result.Portable != nil {
		t.Fatal("Portable = non-nil, want nil for a legacy artifact")
	}
	if result.Legacy == nil || result.Legacy.SchemaVersion != "legacy" {
		t.Fatalf("Legacy = %#v, want decoded legacy artifact", result.Legacy)
	}
}
