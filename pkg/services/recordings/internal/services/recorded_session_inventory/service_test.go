package recordedsessioninventory_test

import (
	"errors"
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
	inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, logging.NoopLogger{})

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
}

func TestListRecordedSessionsReturnsEmptyForAbsentOrEmptyRoot(t *testing.T) {
	tests := map[string]string{
		"absent": filepath.Join(t.TempDir(), "missing-recordings"),
		"empty":  t.TempDir(),
	}
	for name, root := range tests {
		t.Run(name, func(t *testing.T) {
			inventory := recordingswire.NewRecordedSessionInventory(
				os.ReadDir,
				&recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{}},
				logging.NoopLogger{},
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
}

func TestListRecordedSessionsPropagatesMalformedCandidateFromReplayInputLoader(t *testing.T) {
	root := t.TempDir()
	path := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "bad.json"), "unchanged")
	wantErr := errors.New("unsupported replay compatibility version")
	loader := &recordedInputLoader{
		inputs: map[string]recordings.LoadReplayInputResult{path: {}},
		errors: map[string]error{path: wantErr},
	}
	inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, logging.NoopLogger{})

	_, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestListRecordedSessionsDoesNotMutateArtifactsAndUsesLoaderBoundary(t *testing.T) {
	root := t.TempDir()
	path := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "00000000-0000-4000-8000-000000000004.jsonl"), "original-v2-bytes")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(before): %v", err)
	}
	loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
		path: {Legacy: &recordings.ReplayArtifact{}},
	}}
	inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, logging.NoopLogger{})

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
}

func TestListRecordedSessionsIgnoresNonDatedAndUnsupportedFiles(t *testing.T) {
	root := t.TempDir()
	valid := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "valid.json"), "valid")
	writeRecordingFile(t, root, "not-dated.json", "ignored")
	writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "ignored.txt"), "ignored")
	loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
		valid: {Portable: portableInput("valid-session")},
	}}
	inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, logging.NoopLogger{})

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
}

func TestListRecordedSessionsRejectsConflictingLegacySessionIdentities(t *testing.T) {
	root := t.TempDir()
	path := writeRecordingFile(t, root, filepath.Join("2026", "08", "24", "conflicting.json"), "conflicting")
	first, second := "session-first", "session-second"
	loader := &recordedInputLoader{inputs: map[string]recordings.LoadReplayInputResult{
		path: {Legacy: &recordings.ReplayArtifact{Events: []recordings.FactoryEvent{
			{Context: recordings.FactoryEventContext{SessionID: &first}},
			{Context: recordings.FactoryEventContext{SessionID: &second}},
		}}},
	}}
	inventory := recordingswire.NewRecordedSessionInventory(os.ReadDir, loader, logging.NoopLogger{})

	_, err := inventory.ListRecordedSessions(recordings.RecordedSessionInventoryRequest{RecordingRoot: root})
	if err == nil || !strings.Contains(err.Error(), "multiple Factory Session UUIDs") {
		t.Fatalf("error = %v, want conflicting identity error", err)
	}
}

type recordedInputLoader struct {
	inputs map[string]recordings.LoadReplayInputResult
	errors map[string]error
	calls  []string
}

func (loader *recordedInputLoader) LoadReplayInput(request recordings.LoadReplayInputRequest) (recordings.LoadReplayInputResult, error) {
	loader.calls = append(loader.calls, request.Path)
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
