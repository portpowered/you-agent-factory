package named_invocation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const preparationFailureSensitiveValue = "credential-that-must-not-leak"

// TestNamedInvocationSharedPreparationFailures keeps every preparation-only
// failure on one immutable root process. Each case owns its Factory files,
// HOME, working directory, and command output, so sharing the process cannot
// hide cross-scenario state.
func TestNamedInvocationSharedPreparationFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for shared named-invocation preparation failures")
	}

	fixture := newNamedInvocationFailureFixture(t)
	tests := []struct {
		name      string
		selection string
		signature map[string]any
		factory   string
		arguments []string
		wantCode  string
		packaged  bool
	}{
		{
			name:      "named/missing_required",
			selection: "named",
			signature: missingRequiredSignature(),
			wantCode:  string(work.ArgumentErrorCodeMissingRequiredInput),
			packaged:  true,
		},
		{
			name:      "file/missing_required",
			selection: "file",
			signature: missingRequiredSignature(),
			wantCode:  string(work.ArgumentErrorCodeMissingRequiredInput),
			packaged:  true,
		},
		{
			name:      "named/reserved_collision",
			selection: "named",
			signature: reservedCollisionSignature(),
			wantCode:  climanifest.CompositionCollisionLongName,
			packaged:  true,
		},
		{
			name:      "file/reserved_collision",
			selection: "file",
			signature: reservedCollisionSignature(),
			wantCode:  climanifest.CompositionCollisionLongName,
			packaged:  true,
		},
		{
			name: "explicit/static_collision",
			factory: `{
  "name": "collision",
  "invocationSignature": {
    "parameters": [
      {
        "name": "credential",
        "sensitive": true,
        "bindings": [{"kind": "POSITIONAL", "position": 1}]
      },
      {
        "name": "reserved",
        "externalName": "quiet",
        "bindings": [{"kind": "NAMED"}]
      }
    ]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  climanifest.CompositionCollisionLongName,
		},
		{
			name: "explicit/sensitive_normalization_failure",
			factory: `{
  "name": "sensitive",
  "invocationSignature": {
    "parameters": [{
      "name": "token",
      "sensitive": true,
      "required": true,
      "choices": ["allowed"],
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`,
			arguments: []string{preparationFailureSensitiveValue},
			wantCode:  string(work.ArgumentErrorCodeStringValidationMismatch),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runSharedPreparationFailureCase(t, fixture, test)
		})
	}
}

type sharedPreparationFailureCase struct {
	name      string
	selection string
	signature map[string]any
	factory   string
	arguments []string
	wantCode  string
	packaged  bool
}

type namedInvocationFailureFixture struct {
	process       support.ApplicationProcess
	provider      *testutil.ProviderCommandRunner
	listener      *listenerStartObservation
	observation   *preparationSideEffectObservation
	processBuilds atomic.Int32
}

func newNamedInvocationFailureFixture(t *testing.T) *namedInvocationFailureFixture {
	t.Helper()
	fixture := &namedInvocationFailureFixture{
		provider:    testutil.NewProviderCommandRunner(),
		listener:    &listenerStartObservation{},
		observation: &preparationSideEffectObservation{},
	}
	process, err := fixture.buildProcess(t.Context(), fixture.edges())
	if err != nil {
		t.Fatalf("build shared named-invocation failure process: %v", err)
	}
	fixture.process = process
	// Register this assertion before CleanupProcess so the reusable process is
	// closed before final route and effect counts are checked.
	t.Cleanup(func() {
		if got := fixture.processBuilds.Load(); got != 1 {
			t.Errorf("shared named-invocation failure process constructions = %d, want 1", got)
		}
		if got := fixture.listener.calls.Load(); got != 0 {
			t.Errorf("shared named-invocation failure HTTP listener starts = %d, want 0", got)
		}
		fixture.observation.assertZero(t, fixture.provider.CallCount(), fixture.listener.calls.Load())
	})
	support.CleanupProcess(t, process)
	return fixture
}

func (fixture *namedInvocationFailureFixture) buildProcess(
	ctx context.Context,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	fixture.processBuilds.Add(1)
	return support.BuildProcessWithContext(ctx, edges)
}

func (fixture *namedInvocationFailureFixture) edges() serviceedges.Edges {
	return preparationFailureEdges(fixture.observation, fixture.provider, fixture.listener)
}

func preparationFailureEdges(
	observation *preparationSideEffectObservation,
	provider *testutil.ProviderCommandRunner,
	listener *listenerStartObservation,
) serviceedges.Edges {
	return serviceedges.Edges{
		APIServerStarter:          listener.Start,
		ProviderCommandRunner:     provider,
		FactorySessionIDGenerator: observation.nextSessionID,
		RuntimeHostObserver:       observation.observeRuntimeHost,
		WorkRequestIDGenerator:    observation.nextWorkRequestID,
		SubmissionRecorder:        observation.recordSubmission,
		DispatchRecorder:          observation.recordDispatch,
		RecordingWriteFile:        observation.recordRecordingWrite,
		RecordingAppendFile:       observation.recordRecordingAppend,
		RecordingMakeDirectories:  observation.recordRecordingMakeDirectories,
		RecordingCreateTempFile:   observation.recordRecordingCreateTempFile,
		RecordingRemovePath:       observation.recordRecordingRemove,
		RecordingRenamePath:       observation.recordRecordingRename,
		RecordingReadFile:         observation.recordRecordingRead,
		RecordingOpenFile:         observation.recordRecordingOpen,
		RecordingReadDirectory:    observation.recordRecordingReadDirectory,
	}
}

func runSharedPreparationFailureCase(
	t *testing.T,
	fixture *namedInvocationFailureFixture,
	test sharedPreparationFailureCase,
) {
	t.Helper()
	scenario := newNamedInvocationFailureScenario(t)
	factoryPath := filepath.Join(scenario.workingDirectory, "factory.json")
	if test.packaged {
		factoryDir := initializePackagedFactory(
			t, fixture.process, scenario.environment, scenario.workingDirectory,
			scenario.homeDir, packagedGoalFactoryName,
		)
		factoryPath = filepath.Join(factoryDir, "factory.json")
		replaceInvocationSignatureFixture(t, factoryPath, test.signature)
		factoryDir = support.CopyFactoryAsNamed(t, factoryDir, scenario.homeDir, customizedNamedGoalFactoryName)
		factoryPath = filepath.Join(factoryDir, "factory.json")
	} else if err := os.WriteFile(factoryPath, []byte(test.factory), 0o600); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}

	before := fixture.observation.snapshot(fixture.provider.CallCount(), fixture.listener.calls.Load())
	stdout, stderr, err := executePreparationFailure(
		t, fixture.process, scenario.environment, scenario.workingDirectory,
		preparationFailureArguments(test.selection, factoryPath, test.arguments),
	)
	after := fixture.observation.snapshot(fixture.provider.CallCount(), fixture.listener.calls.Load())
	if err == nil || !strings.Contains(err.Error(), test.wantCode) {
		t.Fatalf("%s error = %v, want stable code %s", test.name, err, test.wantCode)
	}
	observable := errText(err) + stdout + stderr
	if strings.Contains(observable, preparationFailureSensitiveValue) {
		t.Fatalf("%s leaked sensitive input: %s", test.name, observable)
	}
	after.delta(before).assertZero(t)
}

type namedInvocationFailureScenario struct {
	rootDir          string
	homeDir          string
	workingDirectory string
	environment      []string
}

func newNamedInvocationFailureScenario(t *testing.T) namedInvocationFailureScenario {
	t.Helper()
	rootDir := t.TempDir()
	homeDir := filepath.Join(rootDir, "home")
	workingDirectory := filepath.Join(rootDir, "work")
	for label, path := range map[string]string{"home": homeDir, "working directory": workingDirectory} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create named-invocation failure %s: %v", label, err)
		}
	}
	return namedInvocationFailureScenario{
		rootDir:          rootDir,
		homeDir:          homeDir,
		workingDirectory: workingDirectory,
		environment:      namedInvocationEnvironment(homeDir),
	}
}

func preparationFailureArguments(selection, factoryPath string, arguments []string) []string {
	base := emptyInvocationArguments(selection, factoryPath, customizedNamedGoalFactoryName)
	return append(base, arguments...)
}

func emptyInvocationArguments(selection, factoryPath, factoryName string) []string {
	if selection == "named" {
		return []string{"you", "run", "--named", factoryName, "--no-record", "--quiet"}
	}
	return []string{"you", "run", "--factory", factoryPath, "--no-record", "--quiet"}
}

func executePreparationFailure(
	t *testing.T,
	process customerProcess,
	environment []string,
	workingDirectory string,
	args []string,
) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err := process.Execute(root.Input{
		Args:             args,
		Env:              environment,
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
		StdinIsTTY:       &stdinIsTTY,
		StdoutIsTTY:      &stdoutIsTTY,
	})
	return stdout.String(), stderr.String(), err
}

func missingRequiredSignature() map[string]any {
	return map[string]any{"parameters": []any{map[string]any{
		"name": "input", "required": true,
		"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
	}}}
}

func reservedCollisionSignature() map[string]any {
	return map[string]any{"parameters": []any{map[string]any{
		"name": "reserved", "externalName": "quiet",
		"bindings": []any{map[string]any{"kind": "NAMED"}},
	}}}
}

type preparationSideEffectObservation struct {
	sessionIDs               atomic.Int32
	runtimeHosts             atomic.Int32
	workIDs                  atomic.Int32
	submissions              atomic.Int32
	dispatches               atomic.Int32
	recordingWrites          atomic.Int32
	recordingAppends         atomic.Int32
	recordingDirectories     atomic.Int32
	recordingTempFiles       atomic.Int32
	recordingRemoves         atomic.Int32
	recordingRenames         atomic.Int32
	recordingReads           atomic.Int32
	recordingOpens           atomic.Int32
	recordingDirectoriesRead atomic.Int32
}

func (observation *preparationSideEffectObservation) nextSessionID() string {
	observation.sessionIDs.Add(1)
	return "unexpected-session"
}

func (observation *preparationSideEffectObservation) observeRuntimeHost(factorysessions.RuntimeHostBinding) {
	observation.runtimeHosts.Add(1)
}

func (observation *preparationSideEffectObservation) nextWorkRequestID() string {
	observation.workIDs.Add(1)
	return "unexpected-work"
}

func (observation *preparationSideEffectObservation) recordSubmission(work.FactorySubmissionRecord) {
	observation.submissions.Add(1)
}

func (observation *preparationSideEffectObservation) recordDispatch(recordings.FactoryDispatchRecord) {
	observation.dispatches.Add(1)
}

func (observation *preparationSideEffectObservation) recordRecordingWrite(path string, data []byte) error {
	observation.recordingWrites.Add(1)
	return os.WriteFile(path, data, 0o644)
}

func (observation *preparationSideEffectObservation) recordRecordingAppend(path string, data []byte) error {
	observation.recordingAppends.Add(1)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (observation *preparationSideEffectObservation) recordRecordingMakeDirectories(path string, mode fs.FileMode) error {
	observation.recordingDirectories.Add(1)
	return os.MkdirAll(path, mode)
}

func (observation *preparationSideEffectObservation) recordRecordingCreateTempFile(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
	observation.recordingTempFiles.Add(1)
	return os.CreateTemp(dir, pattern)
}

func (observation *preparationSideEffectObservation) recordRecordingRemove(path string) error {
	observation.recordingRemoves.Add(1)
	return os.Remove(path)
}

func (observation *preparationSideEffectObservation) recordRecordingRename(oldPath, newPath string) error {
	observation.recordingRenames.Add(1)
	return os.Rename(oldPath, newPath)
}

func (observation *preparationSideEffectObservation) recordRecordingRead(path string) ([]byte, error) {
	observation.recordingReads.Add(1)
	return os.ReadFile(path)
}

func (observation *preparationSideEffectObservation) recordRecordingOpen(path string) (io.ReadCloser, error) {
	observation.recordingOpens.Add(1)
	return os.Open(path)
}

func (observation *preparationSideEffectObservation) recordRecordingReadDirectory(path string) ([]fs.DirEntry, error) {
	observation.recordingDirectoriesRead.Add(1)
	return os.ReadDir(path)
}

type preparationEffectSnapshot struct {
	sessionIDs               int32
	runtimeHosts             int32
	workIDs                  int32
	submissions              int32
	dispatches               int32
	recordingWrites          int32
	recordingAppends         int32
	recordingDirectories     int32
	recordingTempFiles       int32
	recordingRemoves         int32
	recordingRenames         int32
	recordingReads           int32
	recordingOpens           int32
	recordingDirectoriesRead int32
	providerCalls            int32
	listenerStarts           int32
}

func (observation *preparationSideEffectObservation) snapshot(providerCalls int, listenerStarts int32) preparationEffectSnapshot {
	return preparationEffectSnapshot{
		sessionIDs:               observation.sessionIDs.Load(),
		runtimeHosts:             observation.runtimeHosts.Load(),
		workIDs:                  observation.workIDs.Load(),
		submissions:              observation.submissions.Load(),
		dispatches:               observation.dispatches.Load(),
		recordingWrites:          observation.recordingWrites.Load(),
		recordingAppends:         observation.recordingAppends.Load(),
		recordingDirectories:     observation.recordingDirectories.Load(),
		recordingTempFiles:       observation.recordingTempFiles.Load(),
		recordingRemoves:         observation.recordingRemoves.Load(),
		recordingRenames:         observation.recordingRenames.Load(),
		recordingReads:           observation.recordingReads.Load(),
		recordingOpens:           observation.recordingOpens.Load(),
		recordingDirectoriesRead: observation.recordingDirectoriesRead.Load(),
		providerCalls:            int32(providerCalls),
		listenerStarts:           listenerStarts,
	}
}

func (after preparationEffectSnapshot) delta(before preparationEffectSnapshot) preparationEffectSnapshot {
	return preparationEffectSnapshot{
		sessionIDs:               after.sessionIDs - before.sessionIDs,
		runtimeHosts:             after.runtimeHosts - before.runtimeHosts,
		workIDs:                  after.workIDs - before.workIDs,
		submissions:              after.submissions - before.submissions,
		dispatches:               after.dispatches - before.dispatches,
		recordingWrites:          after.recordingWrites - before.recordingWrites,
		recordingAppends:         after.recordingAppends - before.recordingAppends,
		recordingDirectories:     after.recordingDirectories - before.recordingDirectories,
		recordingTempFiles:       after.recordingTempFiles - before.recordingTempFiles,
		recordingRemoves:         after.recordingRemoves - before.recordingRemoves,
		recordingRenames:         after.recordingRenames - before.recordingRenames,
		recordingReads:           after.recordingReads - before.recordingReads,
		recordingOpens:           after.recordingOpens - before.recordingOpens,
		recordingDirectoriesRead: after.recordingDirectoriesRead - before.recordingDirectoriesRead,
		providerCalls:            after.providerCalls - before.providerCalls,
		listenerStarts:           after.listenerStarts - before.listenerStarts,
	}
}

func (delta preparationEffectSnapshot) assertZero(t *testing.T) {
	t.Helper()
	if delta != (preparationEffectSnapshot{}) {
		t.Fatalf("preparation execution-side-effect delta = %#v, want zero", delta)
	}
}

func (observation *preparationSideEffectObservation) assertZero(t *testing.T, providerCalls int, listenerStarts int32) {
	t.Helper()
	observation.snapshot(providerCalls, listenerStarts).assertZero(t)
}

var errCanceledFactoryRootLookup = errors.New("explicit Factory root lookup failed after cancellation")

// TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects
// retains the filesystem cancellation edge in its own process because the
// injected lookup cancels the invocation context and is not reusable.
func TestRun_EffectiveSchemaPreparationFailuresStopBeforeExecutionSideEffects(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test for explicit Factory root lookup cancellation")
	}

	t.Run("cancellation_during_explicit_file_root_lookup", func(t *testing.T) {
		runIsolatedFactoryRootLookupCancellation(t)
	})
}

func runIsolatedFactoryRootLookupCancellation(t *testing.T) {
	t.Helper()
	workingDirectory := t.TempDir()
	factoryPath := filepath.Join(workingDirectory, "factory.json")
	factory := `{
  "name": "canceled",
  "invocationSignature": {
    "parameters": [{
      "name": "input",
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  }
}`
	if err := os.WriteFile(factoryPath, []byte(factory), 0o600); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := testutil.NewProviderCommandRunner()
	listener := &listenerStartObservation{}
	observation := &preparationSideEffectObservation{}
	processBuilds := atomic.Int32{}
	edges := preparationFailureEdges(observation, provider, listener)
	edges.FactoryDefinitionAuthoredReaderFileSystem = cancelingRootLookupFileSystem{
		target: factoryPath,
		cancel: cancel,
	}
	processBuilds.Add(1)
	process, err := support.BuildProcessWithContext(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	before := observation.snapshot(provider.CallCount(), listener.calls.Load())
	homeDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdinIsTTY := true
	stdoutIsTTY := false
	err = process.Execute(root.Input{
		Args:  []string{"you", "run", "--factory", factoryPath, "--no-record", "--quiet", "draft"},
		Env:   namedInvocationEnvironment(homeDir),
		Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
		Context: ctx, WorkingDirectory: workingDirectory,
		StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
	})
	after := observation.snapshot(provider.CallCount(), listener.calls.Load())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute() error = %v, want context cancellation", err)
	}
	if errors.Is(err, errCanceledFactoryRootLookup) {
		t.Fatalf("Process.Execute() returned lookup failure instead of context cancellation: %v", err)
	}
	observable := errText(err) + stdout.String() + stderr.String()
	if strings.Contains(observable, preparationFailureSensitiveValue) {
		t.Fatalf("cancellation leaked sensitive input: %s", observable)
	}
	after.delta(before).assertZero(t)
	t.Cleanup(func() {
		if got := processBuilds.Load(); got != 1 {
			t.Errorf("isolated cancellation process constructions = %d, want 1", got)
		}
		if got := listener.calls.Load(); got != 0 {
			t.Errorf("isolated cancellation HTTP listener starts = %d, want 0", got)
		}
		observation.assertZero(t, provider.CallCount(), listener.calls.Load())
	})
	support.CleanupProcess(t, process)
}

type cancelingRootLookupFileSystem struct {
	target string
	cancel context.CancelFunc
}

func (filesystem cancelingRootLookupFileSystem) Stat(path string) (fs.FileInfo, error) {
	if filepath.Clean(path) == filepath.Clean(filesystem.target) {
		filesystem.cancel()
		return nil, errCanceledFactoryRootLookup
	}
	return os.Stat(path)
}

func (filesystem cancelingRootLookupFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
