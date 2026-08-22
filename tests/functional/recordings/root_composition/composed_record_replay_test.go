package root_composition_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestComposedRecordReplayUsesRootBuildProcessAndExecute proves the ordinary
// record/replay flow through the customer process boundary. BuildProcess stays
// inert until Process.Execute opens the lifecycle, then the public API exposes
// the recorded terminal Work state and the replayed process reconstructs that
// state from the artifact without changing the source bytes.
func TestComposedRecordReplayUsesRootBuildProcessAndExecute(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldFactory(t, recordingsLifecycleActivationFactoryConfig())
	testutil.WriteSeedFile(t, factoryDir, "task", []byte(`{"title":"composed Recordings record/replay"}`))
	artifactPath := filepath.Join(t.TempDir(), "composed-recordings.replay.json")
	effects := newComposedRecordingEffects()

	recordAPI := support.NewProcessAPIServer()
	recordProcess, err := root.BuildProcess(t.Context(), effects.edges(
		recordAPI,
		support.NewStaticSuccessCommandRunner("composed recording provider COMPLETE"),
	))
	if err != nil {
		t.Fatalf("root.BuildProcess(record) error = %v", err)
	}
	support.CleanupProcess(t, recordProcess)
	if got := effects.totalCalls(); got != 0 {
		t.Fatalf("Recordings edge calls during inert BuildProcess = %d, want 0", got)
	}
	if _, err := os.Stat(artifactPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recording artifact after inert BuildProcess: stat error = %v, want not-exist", err)
	}

	recordInputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--record", artifactPath,
	})
	recordInputs.Input.Env = isolatedRecordingEnvironment(t)
	recordInputs.Input.WorkingDirectory = factoryDir
	recordCommand := support.StartProcessCommand(t, recordProcess, recordInputs.Input)
	recordURL := recordAPI.WaitForURL(t)
	support.WaitForTerminalStatus(t, recordURL, 15*time.Second)

	recordedWork := support.ListDefaultSessionWork(t, recordURL)
	if got := support.CountWorkAtCustomerState(recordedWork, "task:complete"); got != 1 {
		t.Fatalf("recorded Work at task:complete = %d, want 1; listed=%#v", got, recordedWork.Results)
	}
	recordedEvents := support.GetFactoryEventsAt(t, recordURL)
	if recordingsActivationLiveEventCount(recordedEvents, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("recorded public Factory Events missing dispatch response")
	}
	recordCommand.Stop(t)

	recordedPayload, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read finalized recording artifact: %v", err)
	}
	if len(recordedPayload) == 0 {
		t.Fatal("finalized recording artifact is empty")
	}
	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest) == 0 ||
		recordingsActivationEventCount(artifact, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatalf("recording artifact events = %#v, want dispatch request and response", artifact.Events)
	}
	writeCallsAfterRecord := effects.writeCalls.Load()

	replayDir := t.TempDir()
	replayAPI := support.NewProcessAPIServer()
	replayRunner := &composedReplayCommandRunner{}
	replayProcess, err := root.BuildProcess(t.Context(), effects.edges(replayAPI, replayRunner))
	if err != nil {
		t.Fatalf("root.BuildProcess(replay) error = %v", err)
	}
	support.CleanupProcess(t, replayProcess)

	replayInputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", replayDir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--replay", artifactPath,
		"--no-record",
	})
	replayInputs.Input.Env = isolatedRecordingEnvironment(t)
	replayInputs.Input.WorkingDirectory = replayDir
	replayCommand := support.StartProcessCommand(t, replayProcess, replayInputs.Input)
	replayURL := replayAPI.WaitForURL(t)
	support.WaitForTerminalStatus(t, replayURL, 15*time.Second)

	replayedWork := support.ListDefaultSessionWork(t, replayURL)
	if got := support.CountWorkAtCustomerState(replayedWork, "task:complete"); got != 1 {
		t.Fatalf("replayed Work at task:complete = %d, want 1; listed=%#v", got, replayedWork.Results)
	}
	replayedEvents := support.GetFactoryEventsAt(t, replayURL)
	if recordingsActivationLiveEventCount(replayedEvents, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("replayed public Factory Events missing dispatch response")
	}
	replayCommand.Stop(t)

	if replayRunner.calls.Load() != 0 {
		t.Fatalf("provider command calls during historical replay = %d, want 0", replayRunner.calls.Load())
	}
	if got, err := os.ReadFile(artifactPath); err != nil {
		t.Fatalf("read recording artifact after replay: %v", err)
	} else if !bytes.Equal(got, recordedPayload) {
		t.Fatal("historical replay mutated the source recording artifact")
	}
	if got := effects.writeCalls.Load(); got != writeCallsAfterRecord {
		t.Fatalf("RecordingWriteFile calls after replay = %d, want unchanged at %d", got, writeCallsAfterRecord)
	}
}

func isolatedRecordingEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
}

type composedRecordingEffects struct {
	filesystemCalls atomic.Int32
	readCalls       atomic.Int32
	writeCalls      atomic.Int32
}

func newComposedRecordingEffects() *composedRecordingEffects {
	return &composedRecordingEffects{}
}

func (effects *composedRecordingEffects) totalCalls() int32 {
	return effects.filesystemCalls.Load() + effects.readCalls.Load() + effects.writeCalls.Load()
}

func (effects *composedRecordingEffects) edges(
	api *support.ProcessAPIServer,
	providerRunner platformprocess.CommandRunner,
) serviceedges.Edges {
	return serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
		RecordingWriteFile: func(path string, payload []byte) error {
			effects.writeCalls.Add(1)
			return os.WriteFile(path, payload, 0o600)
		},
		RecordingMakeDirectories: func(path string, mode fs.FileMode) error {
			effects.filesystemCalls.Add(1)
			return os.MkdirAll(path, mode)
		},
		RecordingCreateTempFile: func(dir, pattern string) (recordings.RecordingTemporaryFile, error) {
			effects.filesystemCalls.Add(1)
			return os.CreateTemp(dir, pattern)
		},
		RecordingRemovePath: func(path string) error {
			effects.filesystemCalls.Add(1)
			return os.Remove(path)
		},
		RecordingRenamePath: func(oldPath, newPath string) error {
			effects.filesystemCalls.Add(1)
			return os.Rename(oldPath, newPath)
		},
		RecordingReadFile: func(path string) ([]byte, error) {
			effects.readCalls.Add(1)
			return os.ReadFile(path)
		},
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			effects.readCalls.Add(1)
			return os.ReadFile(path)
		},
		APIServerStarter: api.Start,
	}
}

type composedReplayCommandRunner struct {
	calls atomic.Int32
}

func (runner *composedReplayCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, errors.New("historical replay must not execute provider commands")
}

var _ platformprocess.CommandRunner = (*composedReplayCommandRunner)(nil)
