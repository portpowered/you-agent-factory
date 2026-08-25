package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestReplayInputLoaderReturnsPortableAndLegacyDomainOutcomes(t *testing.T) {
	t.Parallel()

	portablePath := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	portablePayload, err := os.ReadFile(portablePath)
	if err != nil {
		t.Fatalf("read portable replay fixture: %v", err)
	}

	wantLegacy := &recordings.ReplayArtifact{SchemaVersion: "legacy-v1"}
	cases := []struct {
		name         string
		readFile     recordings.RecordingReadFile
		loadLegacy   recordings.ReplayArtifactLoader
		wantPortable bool
		wantLegacy   *recordings.ReplayArtifact
	}{
		{
			name: "portable recording",
			readFile: func(string) ([]byte, error) {
				return portablePayload, nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				t.Fatal("legacy loader called for portable recording")
				return nil, nil
			},
			wantPortable: true,
		},
		{
			name: "legacy artifact",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			loadLegacy: func(path string) (*recordings.ReplayArtifact, error) {
				if path != "recording.json" {
					t.Fatalf("legacy path = %q, want recording.json", path)
				}
				return wantLegacy, nil
			},
			wantLegacy: wantLegacy,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loader := NewReplayInputLoader(testCase.readFile, testCase.loadLegacy, logging.NoopLogger{})
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
			if err != nil {
				t.Fatalf("LoadReplayInput() error = %v", err)
			}
			if (result.Portable != nil) != testCase.wantPortable {
				t.Fatalf("Portable = %#v, want present=%t", result.Portable, testCase.wantPortable)
			}
			if !reflect.DeepEqual(result.Legacy, testCase.wantLegacy) {
				t.Fatalf("Legacy = %#v, want %#v", result.Legacy, testCase.wantLegacy)
			}
			if testCase.wantPortable && (result.Portable == nil || result.Portable.Session.ID != "session-js-001") {
				t.Fatalf("Portable session = %#v, want session-js-001", result.Portable)
			}
		})
	}
}

func TestReplayInputLoaderClassifiesDependencyAndValidationFailures(t *testing.T) {
	t.Parallel()

	legacyUnavailable := errors.New("legacy source unavailable")
	tests := []struct {
		name       string
		readFile   recordings.RecordingReadFile
		loadLegacy recordings.ReplayArtifactLoader
		wantCause  error
		wantFamily recordings.ReplayInputFamily
		wantCode   recordings.ReplayArtifactDiagnosticCode
	}{
		{
			name:       "reader unavailable",
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "reader canceled",
			readFile: func(string) ([]byte, error) {
				return nil, context.Canceled
			},
			wantCause:  context.Canceled,
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticCancelled,
		},
		{
			name: "legacy loader unavailable",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "legacy loader failure",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"schemaVersion":"legacy"}`), nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				return nil, legacyUnavailable
			},
			wantCause:  legacyUnavailable,
			wantFamily: recordings.ReplayInputFamilyLegacy,
			wantCode:   recordings.ReplayArtifactDiagnosticDependencyFailure,
		},
		{
			name: "malformed portable recording",
			readFile: func(string) ([]byte, error) {
				return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"not-supported"}`), nil
			},
			loadLegacy: func(string) (*recordings.ReplayArtifact, error) {
				t.Fatal("legacy loader called for malformed portable recording")
				return nil, nil
			},
			wantFamily: recordings.ReplayInputFamilyPortable,
			wantCode:   recordings.ReplayArtifactDiagnosticMalformed,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			loader := NewReplayInputLoader(testCase.readFile, testCase.loadLegacy, logging.NoopLogger{})
			result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
			if result.Portable != nil || result.Legacy != nil {
				t.Fatalf("LoadReplayInput() result = %#v, want zero result", result)
			}
			if err == nil {
				t.Fatal("LoadReplayInput() error = nil, want classified failure")
			}
			var inputErr *recordings.ReplayInputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v, want ReplayInputError", err)
			}
			if inputErr.Family != testCase.wantFamily || inputErr.Diagnostic.Code != testCase.wantCode {
				t.Fatalf("error = %#v, want family=%q code=%q", inputErr, testCase.wantFamily, testCase.wantCode)
			}
			if testCase.wantCause != nil && !errors.Is(err, testCase.wantCause) {
				t.Fatalf("error = %v, want cause %v", err, testCase.wantCause)
			}
		})
	}
}

func TestReplayInputLoaderPreservesDetachedPortableDiagnostics(t *testing.T) {
	t.Parallel()

	loader := NewReplayInputLoader(
		func(string) ([]byte, error) {
			return []byte(`{"recordingKind":"` + recordings.KindJavaScriptFactorySession + `","schemaVersion":"2","replayCompatibilityVersion":"99"}`), nil
		},
		nil,
		logging.NoopLogger{},
	)

	_, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: `C:\customer\private\recording.json`})
	var inputErr *recordings.ReplayInputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want ReplayInputError", err)
	}
	if inputErr.Diagnostic.Code != recordings.ReplayArtifactDiagnosticUnsupportedVersion ||
		inputErr.Diagnostic.Area != "compatibility" ||
		inputErr.Diagnostic.Path != "replayCompatibilityVersion" ||
		len(inputErr.Diagnostic.SupportedVersions) == 0 {
		t.Fatalf("diagnostic = %#v, want detached unsupported-version facts", inputErr.Diagnostic)
	}
	inputErr.Diagnostic.Path = "mutated"
	inputErr.Diagnostic.SupportedVersions[0] = "mutated"

	_, err = loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	var second *recordings.ReplayInputError
	if !errors.As(err, &second) {
		t.Fatalf("second error = %v, want ReplayInputError", err)
	}
	if second.Diagnostic.Path == "mutated" || second.Diagnostic.SupportedVersions[0] == "mutated" {
		t.Fatalf("later diagnostic observed caller mutation: %#v", second.Diagnostic)
	}
}

func TestReplayInputLoaderReportsIgnoredFutureFields(t *testing.T) {
	t.Parallel()

	path := testpath.MustRepoPathFromCaller(
		t,
		0,
		"pkg", "services", "recordings", "internal", "artifacts", "testdata", "valid-v2.json",
	)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read portable replay fixture: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode portable replay fixture: %v", err)
	}
	document["futureReplayField"] = json.RawMessage(`true`)
	payload, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("encode portable replay fixture: %v", err)
	}

	loader := NewReplayInputLoader(
		func(string) ([]byte, error) { return payload, nil },
		nil,
		logging.NoopLogger{},
	)
	result, err := loader.LoadReplayInput(recordings.LoadReplayInputRequest{Path: "recording.json"})
	if err != nil {
		t.Fatalf("LoadReplayInput() error = %v", err)
	}
	if result.Diagnostics == nil || !reflect.DeepEqual(result.Diagnostics.IgnoredJSONPaths, []string{"$.futureReplayField"}) {
		t.Fatalf("diagnostics = %#v, want ignored future field", result.Diagnostics)
	}
}
