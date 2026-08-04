package wire_test

import (
	"errors"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func TestReplayInputLoaderClassifiesPortableRecording(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a portable recording")
			return nil, nil
		},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: path})
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

func TestReplayInputLoaderDelegatesLegacyArtifact(t *testing.T) {
	t.Parallel()

	requestedPath := ""
	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	want := &recordings.ReplayArtifact{SchemaVersion: "legacy"}
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(path string) (*recordings.ReplayArtifact, error) {
			requestedPath = path
			return want, nil
		},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Portable != nil {
		t.Fatal("Portable = non-nil, want nil for a legacy artifact")
	}
	if result.Legacy != want {
		t.Fatalf("Legacy = %v, want %v", result.Legacy, want)
	}
	if requestedPath != tempFile {
		t.Fatalf("legacy loader path = %q, want %q", requestedPath, tempFile)
	}
}

func TestReplayInputLoaderRejectsMissingReader(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayInputLoader(nil, nil)
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"}); err == nil {
		t.Fatal("missing reader error = nil")
	}
}

func TestReplayInputLoaderWrapsReadFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("recording read unavailable")
	loader := recordingswire.NewReplayInputLoader(
		func(path string) ([]byte, error) {
			if path != "recording.json" {
				t.Fatalf("path = %q, want recording.json", path)
			}
			return nil, want
		},
		nil,
	)
	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if !errors.Is(err, want) {
		t.Fatalf("LoadReplayInput() error = %v, want %v", err, want)
	}
}

func TestReplayInputLoaderRejectsMissingLegacyLoader(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	loader := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil)
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile}); err == nil {
		t.Fatal("missing legacy loader error = nil")
	}
}

func TestReplayInputLoaderPropagatesMalformedPortableRecording(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(
		t,
		`{"recordingKind":"`+recordings.KindJavaScriptFactorySession+`","schemaVersion":"not-a-real-version"}`,
	)
	loader := recordingswire.NewReplayInputLoader(
		recordings.RecordingReadFile(os.ReadFile),
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy loader must not be called for a portable recording payload")
			return nil, nil
		},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err == nil {
		t.Fatal("malformed portable recording error = nil")
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %+v, want zero-value result on failure", result)
	}
}

func writeTempReplayInputFile(t *testing.T, contents string) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "replay-input-*.json")
	if err != nil {
		t.Fatalf("create temp replay input file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(contents); err != nil {
		t.Fatalf("write temp replay input file: %v", err)
	}
	return file.Name()
}
