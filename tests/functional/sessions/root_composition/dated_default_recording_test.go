package root_composition_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformruntimeartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix proves the
// shared platform reservation boundary still provides collision-numbered names
// for callers that explicitly opt into that policy. Recordings use the exact
// named variant instead, but both policies must retain the platform's UTC date
// layout and atomic file reservation behavior.
func TestNamedRuntimeArtifactCollisionUsesUTCDateAndExplicitSuffix(t *testing.T) {
	t.Parallel()

	reserver, err := platformruntimeartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)

	first, err := reserver.ReserveNamedWithCollision(root, at, "session", ".json")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision first: %v", err)
	}
	second, err := reserver.ReserveNamedWithCollision(root, at, "session", ".json")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision second: %v", err)
	}

	wantDir := filepath.Join(root, "2026", "08", "23")
	wantFirst := filepath.Join(wantDir, "session.json")
	wantSecond := filepath.Join(wantDir, "session-2.json")
	if first != wantFirst || second != wantSecond {
		t.Fatalf("named reservation paths = %q, %q; want %q, %q", first, second, wantFirst, wantSecond)
	}
	if got := platformruntimeartifact.RuntimeArtifactPathComponents("session id", "json"); got != "session_id-json" {
		t.Fatalf("RuntimeArtifactPathComponents() = %q, want %q", got, "session_id-json")
	}
}

// TestRecordingFormatsRemainObservableThroughReusableRootProcess proves the
// default whole-file and explicit JSONL recording contracts through one
// reusable root process. The subtests retain separate homes, factories, and
// artifacts, while the immutable process wiring is constructed once.
func TestRecordingFormatsRemainObservableThroughReusableRootProcess(t *testing.T) {
	acquireRootCompositionFixtureSlot(t)
	fixture := ensureRootCompositionFixture(t)
	fixture.stopSharedHostForDirectProcess(t)
	t.Run("default recording reserves distinct dated UUID artifacts and replays", func(t *testing.T) {
		homeDir := t.TempDir()
		factoryDir := support.ScaffoldSingleStepFactory(t, "rec-3-default-recording")
		fixture.withRootCompositionRoute(t, rootCompositionRouteSpec{
			label:      "default-recording",
			homeDir:    homeDir,
			workingDir: factoryDir,
			// The replay helper deliberately selects a fresh working directory;
			// keep that sibling inside this scenario's owned temporary root.
			extraPaths: []string{filepath.Dir(homeDir)},
		}, func() {
			testDefaultRecordingContract(t, fixture.process, homeDir, factoryDir)
		})
	})
	t.Run("explicit JSONL recording appends through root process", func(t *testing.T) {
		homeDir := t.TempDir()
		factoryDir := support.ScaffoldSingleStepFactory(t, "rec-3-explicit-jsonl-recording")
		recordingPath := filepath.Join(t.TempDir(), "explicit.replay.jsonl")
		fixture.withRootCompositionRoute(t, rootCompositionRouteSpec{
			label:      "explicit-jsonl-recording",
			homeDir:    homeDir,
			workingDir: factoryDir,
			extraPaths: []string{filepath.Dir(recordingPath)},
		}, func() {
			testExplicitJSONLRecordingContract(t, fixture.process, homeDir, factoryDir, recordingPath)
		})
	})
}

func testDefaultRecordingContract(t *testing.T, process support.Process, homeDir, factoryDir string) {
	t.Helper()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	wantDate := time.Now().UTC().Format("2006/01/02")

	firstReportedPath := executeDefaultRecordingRun(t, process, factoryDir, env)
	secondReportedPath := executeDefaultRecordingRun(t, process, factoryDir, env)

	recordingRoot := filepath.Join(homeDir, ".you-agent-factory", "recordings")
	paths := listRecordingArtifacts(t, recordingRoot)
	if len(paths) != 2 {
		t.Fatalf("default recording artifacts = %#v, want exactly two", paths)
	}

	for _, path := range paths {
		assertDatedUUIDRecordingPath(t, recordingRoot, path, wantDate)
		assertWholeFileReplayArtifact(t, path, "~default")
	}
	t.Logf("REC-3 default recording paths: %s and %s", paths[0], paths[1])
	if filepath.Dir(paths[0]) != filepath.Dir(paths[1]) {
		t.Fatalf("default recording directories = %q and %q, want one shared UTC date directory", filepath.Dir(paths[0]), filepath.Dir(paths[1]))
	}
	if paths[0] == paths[1] {
		t.Fatalf("default recording paths collided at %q", paths[0])
	}
	if firstReportedPath != paths[0] && firstReportedPath != paths[1] {
		t.Fatalf("first shutdown-reported path = %q, want one of actual artifacts %#v", firstReportedPath, paths)
	}
	if secondReportedPath != paths[0] && secondReportedPath != paths[1] {
		t.Fatalf("second shutdown-reported path = %q, want one of actual artifacts %#v", secondReportedPath, paths)
	}
	if firstReportedPath == secondReportedPath {
		t.Fatalf("shutdown-reported paths collided at %q", firstReportedPath)
	}

	firstReplayOutput, firstReplayError := replayRecordingThroughRoot(t, process, paths[0])
	secondReplayOutput, secondReplayError := replayRecordingThroughRoot(t, process, paths[1])
	if firstReplayError != nil || secondReplayError != nil {
		t.Fatalf("replay errors = %v, %v; outputs = %q, %q", firstReplayError, secondReplayError, firstReplayOutput, secondReplayOutput)
	}
	if firstReplayOutput != secondReplayOutput {
		t.Fatalf("replay outputs differ: first=%q second=%q", firstReplayOutput, secondReplayOutput)
	}
	pathsAfterReplay := listRecordingArtifacts(t, recordingRoot)
	if strings.Join(pathsAfterReplay, "\n") != strings.Join(paths, "\n") {
		t.Fatalf("replay changed live recording artifacts: before=%#v after=%#v", paths, pathsAfterReplay)
	}
}

func testExplicitJSONLRecordingContract(t *testing.T, process support.Process, homeDir, factoryDir, recordingPath string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", factoryDir, "--record", recordingPath})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(explicit JSONL recording) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	data, err := os.ReadFile(recordingPath)
	if err != nil {
		t.Fatalf("read explicit JSONL recording: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("explicit JSONL recording = %q, want trailing newline", data)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) < 2 {
		t.Fatalf("explicit JSONL recording has %d records, want multiple appended records", len(lines))
	}
	for index, line := range lines {
		if !json.Valid(line) {
			t.Fatalf("explicit JSONL record[%d] = %q, want valid JSON", index, line)
		}
	}
}

func executeDefaultRecordingRun(t *testing.T, process support.Process, factoryDir string, env []string) string {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", factoryDir})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(default recording run) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	for _, line := range strings.Split(inputs.Stdout(), "\n") {
		if path, ok := strings.CutPrefix(line, "Recording saved: "); ok && strings.TrimSpace(path) != "" {
			return filepath.Clean(strings.TrimSpace(path))
		}
	}
	t.Fatalf("default recording run output = %q, want shutdown-reported recording path", inputs.Stdout())
	return ""
}

func listRecordingArtifacts(t *testing.T, recordingRoot string) []string {
	t.Helper()

	var paths []string
	if err := filepath.WalkDir(recordingRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			paths = append(paths, filepath.Clean(path))
		}
		return nil
	}); err != nil {
		t.Fatalf("walk default recording root %q: %v", recordingRoot, err)
	}
	sort.Strings(paths)
	return paths
}

func assertDatedUUIDRecordingPath(t *testing.T, recordingRoot, path, wantDate string) {
	t.Helper()

	relative, err := filepath.Rel(recordingRoot, path)
	if err != nil {
		t.Fatalf("relative recording path %q from %q: %v", path, recordingRoot, err)
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	if len(parts) != 4 || filepath.Join(parts[0], parts[1], parts[2]) != filepath.FromSlash(wantDate) {
		t.Fatalf("recording path = %q, want %s/<uuid>.json under %q", path, wantDate, recordingRoot)
	}
	if filepath.Ext(parts[3]) != ".json" {
		t.Fatalf("recording filename = %q, want .json", parts[3])
	}
	stem := strings.TrimSuffix(parts[3], ".json")
	parsed, err := uuid.Parse(stem)
	if err != nil || parsed.String() != stem {
		t.Fatalf("recording filename stem = %q, want canonical UUID", stem)
	}
	if strings.Contains(path, "__factory_session_id__") || len(stem) != 36 {
		t.Fatalf("recording path = %q, contains a placeholder or collision suffix", path)
	}
}

func assertWholeFileReplayArtifact(t *testing.T, path, publicSessionID string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording %q: %v", path, err)
	}
	if !json.Valid(bytes.TrimSpace(data)) {
		t.Fatalf("recording %q is not one valid JSON document", path)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("recording %q lost the existing whole-file trailing newline", path)
	}
	artifact := testutil.LoadReplayArtifact(t, path)
	if artifact.SchemaVersion == "" || artifact.RecordedAt.IsZero() {
		t.Fatalf("recording %q missing whole-file envelope metadata: %#v", path, artifact)
	}
	if len(artifact.Events) == 0 {
		t.Fatalf("recording %q has no Factory Events", path)
	}
	seenSessionID := false
	for index, event := range artifact.Events {
		if event.Context.Sequence != index {
			t.Fatalf("recording %q event[%d] sequence = %d, want %d", path, index, event.Context.Sequence, index)
		}
		if event.Id == "" || event.Type == "" {
			t.Fatalf("recording %q event[%d] = %#v, want existing event identity and type", path, index, event)
		}
		if event.Context.SessionID != nil && *event.Context.SessionID != "" {
			seenSessionID = true
			if *event.Context.SessionID != publicSessionID {
				t.Fatalf("recording %q event[%d] session ID = %q, want public session ID %q", path, index, *event.Context.SessionID, publicSessionID)
			}
		}
	}
	if !seenSessionID {
		t.Fatalf("recording %q has no canonical Factory Session ID in its events", path)
	}
}

func replayRecordingThroughRoot(t *testing.T, process support.Process, path string) (string, error) {
	t.Helper()

	workingDirectory := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", workingDirectory, "--replay", path, "--no-record", "--quiet",
	})
	inputs.Input.Env = append(
		os.Environ(),
		"HOME="+t.TempDir(),
		"USERPROFILE="+t.TempDir(),
	)
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	return inputs.Stdout() + "\nSTDERR:\n" + inputs.Stderr(), err
}
