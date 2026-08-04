package wire_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

type replayInputTestLedger struct{}

func (replayInputTestLedger) CanonicalEvents() []factorydefinitions.FactoryEvent { return nil }

func (replayInputTestLedger) Subscribe(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (factorydefinitions.FactoryEventStream, error) {
	return factorydefinitions.FactoryEventStream{}, nil
}

func (replayInputTestLedger) StreamGenerationID() string { return "replay-input-capability-test" }

func (replayInputTestLedger) AddEventRecorder(func(factorydefinitions.FactoryEvent)) {}

func (replayInputTestLedger) AddEventTypeRecorder(func(factorydefinitions.FactoryEventType)) {}

func (replayInputTestLedger) AppendRecordedEvent(factorydefinitions.FactoryEvent) {}

func TestReplayInputLoaderClassifiesPortableRecording(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	loader := recordingswire.NewReplayArtifactCapability(
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
	loader := recordingswire.NewReplayArtifactCapability(
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

	loader := recordingswire.NewReplayArtifactCapability(nil, nil)
	if _, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"}); err == nil {
		t.Fatal("missing reader error = nil")
	}
}

func TestReplayInputLoaderWrapsReadFailure(t *testing.T) {
	t.Parallel()

	want := errors.New("recording read unavailable")
	loader := recordingswire.NewReplayArtifactCapability(
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
	loader := recordingswire.NewReplayArtifactCapability(recordings.RecordingReadFile(os.ReadFile), nil)
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
	loader := recordingswire.NewReplayArtifactCapability(
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

func TestReplayArtifactCapabilityLedgerOperationsAreUnsupported(t *testing.T) {
	t.Parallel()

	loader := recordingswire.NewReplayArtifactCapability(recordings.RecordingReadFile(os.ReadFile), nil)
	assertUnsupportedContext := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("error = nil, want ReplayArtifactErrorUnsupportedContext")
		}
		if !errors.Is(err, recordings.ErrReplayArtifactUnsupportedContext) {
			t.Fatalf("errors.Is(err, ErrReplayArtifactUnsupportedContext) = false for err = %v", err)
		}
		var replayErr *recordings.ReplayArtifactError
		if !errors.As(err, &replayErr) {
			t.Fatalf("errors.As(err, *ReplayArtifactError) = false for err = %v", err)
		}
		if replayErr.Kind != recordings.ReplayArtifactErrorUnsupportedContext {
			t.Fatalf("Kind = %q, want %q", replayErr.Kind, recordings.ReplayArtifactErrorUnsupportedContext)
		}
	}

	t.Run("LoadReplay", func(t *testing.T) {
		t.Parallel()
		_, err := loader.LoadReplay(recordings.LoadReplayRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("BuildArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.BuildArtifact(recordings.BuildArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("ValidateArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.ValidateArtifact(recordings.ValidateArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("EncodeArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.EncodeArtifact(recordings.EncodeArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("DecodeArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.DecodeArtifact(recordings.DecodeArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("SummarizeArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.SummarizeArtifact(recordings.SummarizeArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("ExportArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.ExportArtifact(context.Background(), recordings.ExportArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
	t.Run("ReadArtifact", func(t *testing.T) {
		t.Parallel()
		_, err := loader.ReadArtifact(context.Background(), recordings.ReadArtifactRequest{})
		assertUnsupportedContext(t, err)
	})
}

func TestLedgerBackedReplayArtifactsLoadReplayInputIsUnsupported(t *testing.T) {
	t.Parallel()

	service, err := recordingswire.NewServiceWithProjectionAndEffects(
		replayInputTestLedger{},
		recordingswire.NewProjectionService(),
		nil,
		func(string, []byte) error { return nil },
		os.MkdirAll,
		func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
	if err != nil {
		t.Fatalf("construct Recordings service: %v", err)
	}
	replayArtifacts, ok := service.(recordings.RecordingReplayArtifacts)
	if !ok {
		t.Fatal("Recordings service does not implement RecordingReplayArtifacts")
	}
	_, err = replayArtifacts.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if !errors.Is(err, recordings.ErrReplayArtifactUnsupportedContext) {
		t.Fatalf("LoadReplayInput() error = %v, want ErrReplayArtifactUnsupportedContext", err)
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
