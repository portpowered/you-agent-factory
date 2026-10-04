package inference_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type recordingProjectDirectory string

func (directory recordingProjectDirectory) Getwd() (string, error) { return string(directory), nil }

type tempCleanupProvider struct {
	started chan struct{}
	release chan struct{}
	result  platformprocess.CommandResult
}

func (runner *tempCleanupProvider) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	close(runner.started)
	select {
	case <-runner.release:
		return runner.result, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

// A separate process cohort is required because the existing inference cohort
// injects a recording writer. This cohort uses the production default writer;
// its second construction proves restart through the supported process reader.
func TestWorkerRecordingSurvivesTempCleanup(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	ownedTemp := filepath.Join(t.TempDir(), "invocation-temp")
	if err := os.Mkdir(ownedTemp, 0o755); err != nil {
		t.Fatal(err)
	}
	loaded := loadOpeningRecordFixture(t, "codex", "success")
	runner := &tempCleanupProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		result: platformprocess.CommandResult{Stdout: loaded.Stdout.Raw, Stderr: []byte(loaded.Stderr)},
	}
	edges := serviceedges.Edges{FactorySessionsWorkingDirectory: recordingProjectDirectory(project), ProviderCommandRunner: runner}
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	dir := wsrFT004Factory(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--dir", dir, "--session", uuid.NewString(), "--quiet", "--record", filepath.Join(project, "factory-recording.json")})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = sharedInferenceProcessEnvironment(t.TempDir())
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		inputs.Input.Env = setSharedInferenceEnvironment(inputs.Input.Env, key, ownedTemp)
	}
	var stdout, stderr wsrFT009Output
	inputs.Input.Stdout, inputs.Input.Stderr = &stdout, &stderr
	done := make(chan error, 1)
	go func() { done <- process.Execute(inputs.Input) }()
	select {
	case <-runner.started:
	case err := <-done:
		t.Fatalf("Execute ended before provider gate: %v, %s", err, stderr.snapshot())
	case <-ctx.Done():
		t.Fatal("provider gate timed out")
	}
	recordingID := tempCleanupRecordingIdentity(t, project)
	opening, err := process.WorkerRecordingReader().LoadWorkerRecording(t.Context(), recordingID)
	if err != nil || len(opening.Sessions) != 1 || len(opening.Sessions[0].Records) != 1 {
		t.Fatalf("durable opening = %#v, error %v", opening, err)
	}
	// Delete only the explicit invocation-owned child, never the host temp root.
	if err := os.RemoveAll(ownedTemp); err != nil {
		t.Fatal(err)
	}
	close(runner.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute after cleanup: %v, %s", err, stderr.snapshot())
		}
	case <-ctx.Done():
		t.Fatal("Execute timed out after cleanup")
	}
	assertTempCleanupWorkCompleted(t, filepath.Join(project, "factory-recording.json"))
	live, err := process.WorkerRecordingReader().LoadWorkerRecording(t.Context(), recordingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(live.Sessions) != 1 || live.Sessions[0].Status != recordings.WorkerRecordingStatusComplete || len(live.Sessions[0].Records) <= 2 {
		t.Fatalf("post-cleanup history = %#v, want output and completed terminal", live)
	}
	restarted, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, restarted)
	reloaded, err := restarted.WorkerRecordingReader().LoadWorkerRecording(t.Context(), recordingID)
	if err != nil || !reflect.DeepEqual(live, reloaded) {
		t.Fatalf("restart history differs: error %v, snapshot %#v", err, reloaded)
	}
}

func assertTempCleanupWorkCompleted(t *testing.T, path string) {
	t.Helper()
	artifact := testutil.LoadReplayArtifact(t, path)
	for _, event := range artifact.Events {
		if string(event.Type) != string(factoryapi.FactoryEventTypeDispatchResponse) {
			continue
		}
		var payload struct {
			Outcome    string `json:"outcome"`
			Output     string `json:"output"`
			OutputWork []struct {
				WorkTypeName string `json:"workTypeName"`
				State        struct {
					Name string `json:"name"`
				} `json:"state"`
			} `json:"outputWork"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Outcome == "ACCEPTED" && payload.Output == "Codex fixture answer COMPLETE" && len(payload.OutputWork) == 1 && payload.OutputWork[0].WorkTypeName == "task" && payload.OutputWork[0].State.Name == "done" {
			return
		}
	}
	t.Fatal("recorded public Work history omitted the expected accepted result after cleanup")
}
func tempCleanupRecordingIdentity(t *testing.T, project string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(project, ".you-agent-factory", "worker-recordings", "*.worker.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("durable journals = %v, error %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var header struct {
		RecordingID string `json:"recordingId"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.RecordingID == "" {
		t.Fatalf("opening journal identity: %v", err)
	}
	return header.RecordingID
}
