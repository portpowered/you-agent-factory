package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestDaemonRestartProbePreservesBoard(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	emptyDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	corruptDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	config := seededReplayResumeFactoryConfig()
	types := config["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
	boardDir := support.ScaffoldFactory(t, config)
	support.WriteAgentConfig(t, boardDir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	support.WriteWorkstationConfig(t, boardDir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	api := support.NewProcessAPIServer()
	boardAPIs := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	files := &restartProbeFiles{corruptRoot: corruptDir}
	runner := &restartProbeUnexpectedRunner{requests: make(chan platformprocess.CommandRequest, 4)}
	var starts atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		FactorySessionRuntimePersistenceFileSystem: files,
		ProviderCommandRunner:                      runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			starts.Add(1)
			if request.Port == 23101 || request.Port == 23102 {
				return boardAPIs[request.Port-23101].Start(ctx, request)
			}
			return api.Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	// A board's stop/reopen is ordered; independent empty/corrupt projects
	// below remain parallel on the same reusable process graph.
	t.Run("F1 F2 graceful DAG restart", func(t *testing.T) {
		testRestartProbeDAG(t, process, boardDir, boardAPIs, runner)
	})

	t.Run("F3 fresh board opens empty", func(t *testing.T) {
		t.Parallel()
		testRestartProbeFreshBoard(t, process, emptyDir, api, runner)
	})
	t.Run("F4 corrupt snapshot rejects selected board safely", func(t *testing.T) {
		t.Parallel()
		testRestartProbeCorruptBoard(t, process, corruptDir, files, runner)
	})
	t.Cleanup(func() {
		if starts.Load() != 3 || files.corruptReads.Load() != 1 {
			t.Errorf("startup attempts=%d corrupt probe reads=%d; want three and one", starts.Load(), files.corruptReads.Load())
		}
	})
}

func testRestartProbeFreshBoard(t *testing.T, process support.Process, dir string, api *support.ProcessAPIServer, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	inputs := restartProbeInputs(t, dir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	session := support.GetDefaultSession(t, baseURL)
	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, session.Id, "/work"))
	if len(works.Results) != 0 || runner.calls.Load() != 3 {
		t.Fatal("fresh opening recovered Work or dispatched a worker")
	}
	command.Stop(t)
	if strings.Contains(inputs.Stderr(), "CORRUPT") || strings.Contains(inputs.Stderr(), "recovery") {
		t.Fatalf("fresh opening reported recovery failure: %s", inputs.Stderr())
	}
}

func testRestartProbeCorruptBoard(t *testing.T, process support.Process, dir string, files *restartProbeFiles, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	artifact := seededReplayResumeArtifactPayload(t, true)
	path := filepath.Join(dir, "current-board.json")
	if err := os.WriteFile(path, artifact, 0600); err != nil {
		t.Fatal(err)
	}
	inputs := restartProbeInputs(t, dir)
	err := process.Execute(inputs.Input)
	var resumeErr *factorysessions.ResumeError
	if !errors.As(err, &resumeErr) || resumeErr.Outcome != factorysessions.ResumeOutcomeCorruptedPersistence {
		t.Fatalf("corrupt snapshot rejection = %v; stderr=%s", err, inputs.Stderr())
	}
	output := inputs.Stdout() + inputs.Stderr() + err.Error()
	if strings.Contains(output, restartProbeSecret) || strings.Contains(output, "Factory initiated:") || runner.calls.Load() != 3 {
		t.Fatalf("failed opening exposed payload, published readiness or dispatched: %s", output)
	}
	if got := mustReadSeededReplayArtifact(t, path); !bytes.Equal(got, artifact) || files.corruptWrites.Load() != 0 {
		t.Fatal("failed opening mutated selected source or durable snapshot")
	}
}

func restartProbeInputs(t *testing.T, dir string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet",
		"--provider", "CODEX", "--model", "gpt-5-codex", "--record", filepath.Join(dir, "current-board.json"),
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = dir
	return inputs
}

const restartProbeSecret = "private-snapshot-secret-marker"

type restartProbeFiles struct {
	corruptRoot   string
	corruptReads  atomic.Int32
	corruptWrites atomic.Int32
}

func (files *restartProbeFiles) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (files *restartProbeFiles) ReadFile(path string) ([]byte, error) {
	if strings.HasPrefix(filepath.Clean(path), files.corruptRoot+string(filepath.Separator)) {
		files.corruptReads.Add(1)
		return []byte(`{"Session":` + restartProbeSecret), nil
	}
	return os.ReadFile(path)
}

func (files *restartProbeFiles) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if strings.HasPrefix(filepath.Clean(path), files.corruptRoot+string(filepath.Separator)) {
		files.corruptWrites.Add(1)
		return errors.New("unexpected write during rejected opening")
	}
	return os.WriteFile(path, data, mode)
}

type restartProbeUnexpectedRunner struct {
	calls    atomic.Int32
	requests chan platformprocess.CommandRequest
}

func (runner *restartProbeUnexpectedRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.requests <- request
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("restart COMPLETE")}, nil
}
