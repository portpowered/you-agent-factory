package recordedsessioninventory_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
)

func TestListRecordedSessionsEnumeratesMixedDatedVersionsDeterministically(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			root := t.TempDir()
			paths := []string{
				writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "00000000-0000-4000-8000-000000000002.jsonl"), "v2-empty"),
				writeRecordingFile(t, root, filepath.Join("2026", "08", "23", "v1-second.json"), "v1-second"),
				writeRecordingFile(t, root, filepath.Join("2026-08", "2026-08-22", "v1-first.json"), "v1-first"),
				writeRecordingFile(t, root, filepath.Join("2026", "08", "21", "same-later.json"), "same-later"),
				writeRecordingFile(t, root, filepath.Join("2026", "08", "20", "same-earlier.json"), "same-earlier"),
			}
			loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
				paths[0]: {Legacy: &recordings.ReplayArtifact{}},
				paths[1]: {Portable: portableInput("session-2")},
				paths[2]: {Portable: portableInput("session-1")},
				paths[3]: {Portable: portableInput("session-same")},
				paths[4]: {Portable: portableInput("session-same")},
			}}
			inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, selectedLogger)

			result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
			if err != nil {
				t.Fatalf("ListRecordedSessions() error = %v", err)
			}
			want := []recordings.RecordedSessionSummary{
				{
					FactorySessionID:  "00000000-0000-4000-8000-000000000002",
					ArtifactReference: "2026/08/24/00000000-0000-4000-8000-000000000002.jsonl",
					Format:            recordings.RecordedSessionFormatV2JSONL,
				},
				{
					FactorySessionID:  "session-1",
					ArtifactReference: "2026-08/2026-08-22/v1-first.json",
					Format:            recordings.RecordedSessionFormatV1JSON,
				},
				{
					FactorySessionID:  "session-2",
					ArtifactReference: "2026/08/23/v1-second.json",
					Format:            recordings.RecordedSessionFormatV1JSON,
				},
				{
					FactorySessionID:  "session-same",
					ArtifactReference: "2026/08/20/same-earlier.json",
					Format:            recordings.RecordedSessionFormatV1JSON,
				},
				{
					FactorySessionID:  "session-same",
					ArtifactReference: "2026/08/21/same-later.json",
					Format:            recordings.RecordedSessionFormatV1JSON,
				},
			}
			if !reflect.DeepEqual(result.Sessions, want) {
				t.Fatalf("sessions = %#v, want %#v", result.Sessions, want)
			}
			if !reflect.DeepEqual(loader.calls, sortedCopy(paths)) {
				t.Fatalf("loader calls = %#v, want %#v", loader.calls, sortedCopy(paths))
			}
			if len(loader.metadataOnly) != len(paths) {
				t.Fatalf("metadata-only calls = %#v, want one per candidate", loader.metadataOnly)
			}
			for index, metadataOnly := range loader.metadataOnly {
				if !metadataOnly {
					t.Fatalf("loader call %d used full replay loading, want metadata-only", index)
				}
			}
		})
	}
}

func TestListRecordedSessionsReturnsEmptyForAbsentOrEmptyRoot(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			tests := map[string]string{
				"absent": filepath.Join(t.TempDir(), "missing-recordings"),
				"empty":  t.TempDir(),
			}
			for name, root := range tests {
				t.Run(name, func(t *testing.T) {
					inventory := recordingswire.NewRecordedSessionInventory(
						os.ReadDir,
						&recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{}},
						selectedLogger,
					)
					result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
					if err != nil {
						t.Fatalf("ListRecordedSessions() error = %v", err)
					}
					if len(result.Sessions) != 0 {
						t.Fatalf("sessions = %#v, want empty", result.Sessions)
					}
				})
			}
		})
	}
}

func TestListRecordedSessionsSkipsMalformedCandidateAndReportsIt(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			root := t.TempDir()
			bad := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "bad.json"), "unchanged")
			good := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "good.json"), "good")
			loader := &recordedInputLoader{
				inputs: map[string]recordings.LoadReplayInputResult{
					bad:  {},
					good: {Portable: portableInput("good-session")},
				},
				errors: map[string]error{bad: errors.New("credential=planted-secret absolute-home-path")},
			}
			inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, selectedLogger)

			result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
			if err != nil {
				t.Fatalf("ListRecordedSessions() error = %v, want the bad recording skipped", err)
			}
			if len(result.Sessions) != 1 || result.Sessions[0].FactorySessionID != "good-session" {
				t.Fatalf("sessions = %#v, want only the readable recording", result.Sessions)
			}
			wantWarnings := []recordings.RecordedSessionDiagnostic{{ArtifactReference: "2026/08/24/bad.json", Code: "UNREADABLE_RECORDING", Reason: "Recording could not be read or decoded, or has no valid session identity."}}
			if !reflect.DeepEqual(result.Warnings, wantWarnings) {
				t.Fatalf("warnings = %#v, want %#v", result.Warnings, wantWarnings)
			}
			if capture, ok := selectedLogger.(*inventoryLogCapture); ok {
				want := []inventoryLogEntry{
					inventoryEntry("recordings session inventory skipped unreadable recording", []any{"operation", "list_recorded_sessions", "artifact_reference", "2026/08/24/bad.json", "reason", wantWarnings[0].Reason}),
					inventoryEntry("recordings session inventory skipped recordings", []any{"operation", "list_recorded_sessions", "skipped_recording_count", 1}),
				}
				if !reflect.DeepEqual(capture.warnings, want) {
					t.Fatalf("warnings = %#v, want %#v", capture.warnings, want)
				}
				if capture.infos[1].fields["recorded_session_count"] != 1 {
					t.Fatalf("outcome = %#v", capture.infos[1])
				}
				if strings.Contains(fmt.Sprint(capture), root) {
					t.Fatal("inventory logs leaked absolute root")
				}
			}
			for path, want := range map[string]string{bad: "unchanged", good: "good"} {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != want {
					t.Fatalf("artifact mutated: %q %v", after, err)
				}
			}

		})
	}
}

func TestListRecordedSessionsDoesNotMutateArtifactsAndUsesLoaderBoundary(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			root := t.TempDir()
			path := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "00000000-0000-4000-8000-000000000004.jsonl"), "original-v2-bytes")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(before): %v", err)
			}
			loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
				path: {Legacy: &recordings.ReplayArtifact{}},
			}}
			inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, selectedLogger)

			if _, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root}); err != nil {
				t.Fatalf("ListRecordedSessions() error = %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(after): %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("artifact bytes changed from %q to %q", before, after)
			}
			if !reflect.DeepEqual(loader.calls, []string{path}) {
				t.Fatalf("loader calls = %#v, want %#v", loader.calls, []string{path})
			}
		})
	}
}

func TestListRecordedSessionsIgnoresNonDatedAndUnsupportedFiles(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			root := t.TempDir()
			valid := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "valid.json"), "valid")
			writeRecordingFile(t, root, "not-dated.json", "ignored")
			writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "ignored.txt"), "ignored")
			loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
				valid: {Portable: portableInput("valid-session")},
			}}
			inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, selectedLogger)

			result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
			if err != nil {
				t.Fatalf("ListRecordedSessions() error = %v", err)
			}
			if len(result.Sessions) != 1 || result.Sessions[0].FactorySessionID != "valid-session" {
				t.Fatalf("sessions = %#v, want one valid session", result.Sessions)
			}
			if len(loader.calls) != 1 || loader.calls[0] != valid {
				t.Fatalf("loader calls = %#v, want only %q", loader.calls, valid)
			}
		})
	}
}

func TestListRecordedSessionsSkipsConflictingLegacySessionIdentities(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		t.Run(variant, func(t *testing.T) {
			t.Parallel()
			selectedLogger := inventoryTestLogger(t, variant)
			root := t.TempDir()
			path := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "conflicting.json"), "conflicting")
			first, second := "session-first", "session-second"
			loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
				path: {Legacy: &recordings.ReplayArtifact{Events: []recordings.FactoryEvent{
					{Context: recordings.FactoryEventContext{SessionID: &first}},
					{Context: recordings.FactoryEventContext{SessionID: &second}},
				}}},
			}}
			inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, selectedLogger)

			result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
			if err != nil || len(result.Sessions) != 0 {
				t.Fatalf("sessions = %#v, error = %v, want the conflicting recording skipped", result.Sessions, err)
			}
		})
	}
}

type recordedInputLoader struct {
	inputs       map[string]recordings.LoadReplayInputResult
	errors       map[string]error
	calls        []string
	metadataOnly []bool
}

func (loader *recordedInputLoader) LoadReplayInput(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	loader.calls = append(loader.calls, request.Path)
	loader.metadataOnly = append(loader.metadataOnly, request.MetadataOnly)
	if err := loader.errors[request.Path]; err != nil {
		return recordings.LoadReplayInputResult{}, err
	}
	return loader.inputs[request.Path], nil
}

func portableInput(id string) *recordings.PortableRecording {
	return &recordings.PortableRecording{Session: recordings.PortableRecordingSessionSummary{ID: id}}
}

func writeRecordingFile(t *testing.T, root, relative, contents string) string {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
	return path
}

func sortedCopy(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

// Each scenario owns its observer; cleanup verifies the operation's safe records.
func inventoryTestLogger(t *testing.T, variant string) logging.Logger {
	t.Helper()
	if variant == "noop" {
		return logging.NoopLogger{}
	}
	capture := &inventoryLogCapture{}
	t.Cleanup(func() {
		if len(capture.infos) < 2 || len(capture.infos)%2 != 0 || capture.infos[0].message != "recordings session inventory accepted" || capture.infos[1].message != "recordings session inventory outcome" {
			t.Errorf("inventory info records = %#v, want accepted and terminal outcome", capture.infos)
		}
		for _, entry := range append(capture.infos, capture.warnings...) {
			if entry.fields["operation"] != "list_recorded_sessions" {
				t.Errorf("operation correlation = %#v", entry)
			}
			for _, private := range []string{"planted-secret", "absolute-home-path"} {
				if strings.Contains(fmt.Sprint(entry), private) {
					t.Errorf("diagnostic leaked private cause: %#v", entry)
				}
			}
		}
	})
	return capture
}

type inventoryLogEntry struct {
	message string
	fields  map[string]any
}
type inventoryLogCapture struct {
	logging.NoopLogger
	infos, warnings []inventoryLogEntry
}

func inventoryEntry(message string, fields []any) inventoryLogEntry {
	entry := inventoryLogEntry{message: message, fields: map[string]any{}}
	for index := 0; index+1 < len(fields); index += 2 {
		entry.fields[fields[index].(string)] = fields[index+1]
	}
	return entry
}
func (capture *inventoryLogCapture) Info(message string, fields ...any) {
	capture.infos = append(capture.infos, inventoryEntry(message, fields))
}
func (capture *inventoryLogCapture) Warn(message string, fields ...any) {
	capture.warnings = append(capture.warnings, inventoryEntry(message, fields))
}

func TestListRecordedSessionsPreservesDirectoryFailure(t *testing.T) {
	t.Parallel()
	for _, variant := range []string{"capture", "noop"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%t", variant, nested), func(t *testing.T) {
				t.Parallel()
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, "2026"), 0o700); err != nil {
					t.Fatal(err)
				}
				cause := errors.New("private directory failure")
				reader := func(path string) ([]fs.DirEntry, error) {
					if !nested || path != root {
						return nil, cause
					}
					return os.ReadDir(path)
				}
				capture := &inventoryLogCapture{}
				var logger logging.Logger = capture
				if variant == "noop" {
					logger = logging.NoopLogger{}
				}
				inventory := recordingswire.NewRecordedSessionInventory(reader, &recordedInputLoader{}, logger)
				result, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
				if !errors.Is(err, cause) || !strings.Contains(err.Error(), "read recording directory") || len(result.Sessions) != 0 || len(result.Warnings) != 0 {
					t.Fatalf("result = %#v, error = %v", result, err)
				}
				if variant == "capture" {
					want := []inventoryLogEntry{
						inventoryEntry("recordings session inventory accepted", []any{"operation", "list_recorded_sessions"}),
						inventoryEntry("recordings session inventory outcome", []any{"operation", "list_recorded_sessions", "outcome", "failure", "recorded_session_count", 0}),
					}
					if !reflect.DeepEqual(capture.infos, want) || len(capture.warnings) != 0 {
						t.Fatalf("diagnostics = %#v, want %#v", capture, want)
					}
				}
			})
		}
	}
}
