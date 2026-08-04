package wire_test

import (
	"errors"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
		zap.NewNop(),
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

func TestRecordingReplayArtifactsRuntimeConstructionIsInert(t *testing.T) {
	t.Parallel()

	built := false
	factory := recordingswire.NewRecordingReplayArtifactsFactory(
		func(string) ([]byte, error) {
			t.Fatal("replay input reader called during construction")
			return nil, nil
		},
		func(string) (*recordings.ReplayArtifact, error) {
			t.Fatal("legacy replay loader called during construction")
			return nil, nil
		},
		zap.NewNop(),
		func(
			recordings.Ledger,
			recordings.ProjectionService,
		) (recordings.RecordingReplayArtifacts, recordings.RecordingLifecycle, error) {
			built = true
			return nil, nil, nil
		},
	)
	capability := factory()
	if capability == nil {
		t.Fatal("factory() = nil, want phase-aware capability")
	}
	if built {
		t.Fatal("runtime artifact builder called during construction")
	}
	_, err := capability.LoadReplay(recordings.LoadReplayRequest{RecordingID: "not-bound"})
	var typed *recordings.ReplayArtifactError
	if !errors.As(err, &typed) || typed.Kind != recordings.ReplayArtifactErrorUnavailable {
		t.Fatalf("LoadReplay() error = %v, want unavailable typed error before binding", err)
	}
	if built {
		t.Fatal("runtime artifact builder called by an unbound artifact operation")
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
		zap.NewNop(),
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

	loader := recordingswire.NewReplayInputLoader(nil, nil, zap.NewNop())
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
		zap.NewNop(),
	)
	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if !errors.Is(err, want) {
		t.Fatalf("LoadReplayInput() error = %v, want %v", err, want)
	}
}

func TestReplayInputLoaderRejectsMissingLegacyLoader(t *testing.T) {
	t.Parallel()

	tempFile := writeTempReplayInputFile(t, `{"schemaVersion":"legacy"}`)
	loader := recordingswire.NewReplayInputLoader(recordings.RecordingReadFile(os.ReadFile), nil, zap.NewNop())
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
		zap.NewNop(),
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: tempFile})
	if err == nil {
		t.Fatal("malformed portable recording error = nil")
	}
	if result.Portable != nil || result.Legacy != nil {
		t.Fatalf("result = %+v, want zero-value result on failure", result)
	}
	var typed *recordings.ReplayInputError
	if !errors.As(err, &typed) {
		t.Fatalf("LoadReplayInput() error = %T, want *recordings.ReplayInputError", err)
	}
	if typed.Kind != recordings.ReplayInputErrorPortable {
		t.Fatalf("ReplayInputError.Kind = %q, want %q", typed.Kind, recordings.ReplayInputErrorPortable)
	}
	if typed.Diagnostic == nil {
		t.Fatal("ReplayInputError.Diagnostic = nil, want unsupported-version diagnostic")
	}
	if typed.Diagnostic.Code != recordings.ReplayInputDiagnosticUnsupportedVersion {
		t.Fatalf(
			"ReplayInputError.Diagnostic.Code = %q, want %q",
			typed.Diagnostic.Code,
			recordings.ReplayInputDiagnosticUnsupportedVersion,
		)
	}
	if typed.Diagnostic.Area == "" || typed.Diagnostic.Message == "" || len(typed.Diagnostic.SupportedVersions) == 0 {
		t.Fatalf("ReplayInputError.Diagnostic = %#v, want structured supported-version facts", typed.Diagnostic)
	}
}

func TestReplayInputLoaderLogsIntentAndFailureWithoutInputPathOrPayload(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zap.InfoLevel)
	loader := recordingswire.NewReplayInputLoader(nil, nil, zap.New(core))
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "private/replay.json"}); err == nil {
		t.Fatal("LoadReplayInput() error = nil, want reader configuration error")
	}

	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("log entry count = %d, want 2", len(entries))
	}
	if entries[0].Message != "loading replay input" || entries[1].Message != "replay input reader is not configured" {
		t.Fatalf("log messages = %q, %q", entries[0].Message, entries[1].Message)
	}
	for _, entry := range entries {
		for _, field := range entry.Context {
			if field.Key == "path" || field.Key == "payload" || field.String == "private/replay.json" {
				t.Fatalf("log field = %#v, must not expose replay input path or payload", field)
			}
		}
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
