package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	implicitDir := support.ScaffoldFactory(t, config)
	implicitRepo := t.TempDir()
	implicitFactory := filepath.Join(implicitRepo, "factory")
	if err := os.Rename(implicitDir, implicitFactory); err != nil {
		t.Fatal(err)
	}
	implicitDir = implicitFactory
	support.WriteAgentConfig(t, implicitDir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	support.WriteWorkstationConfig(t, implicitDir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	support.WriteAgentConfig(t, boardDir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	support.WriteWorkstationConfig(t, boardDir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	api := support.NewProcessAPIServer()
	boardAPIs := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	files := &restartProbeFiles{corruptRoot: corruptDir}
	runner := &restartProbeUnexpectedRunner{requests: make(chan platformprocess.CommandRequest, 4)}
	var starts atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		FactorySessionRuntimePersistenceFileSystem: files,
		ProviderCommandRunner:                      runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			// The two ordered local journeys own the first five starts. Route
			// the exact-command journey without changing its default listener.
			if index := starts.Add(1); index <= int32(len(boardAPIs)) {
				return boardAPIs[index-1].Start(ctx, request)
			}
			return api.Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	// A board's stop/reopen is ordered; independent empty/corrupt projects
	// below remain parallel on the same reusable process graph.
	t.Run("F1 F2 graceful DAG restart", func(t *testing.T) {
		testRestartProbeDAG(t, process, boardDir, boardAPIs[:2], runner)
	})
	t.Run("PlainBoard graceful DAG restart without selector", func(t *testing.T) {
		// Both journeys exercise local ~default ownership, so run this smallest
		// cohort in order before the independent parallel projects below.
		runner.calls.Store(0)
		home := t.TempDir()
		invocations := 0
		var retainedReference []byte
		testRestartProbeDAGWithInputs(t, process, implicitDir, boardAPIs[2:], runner, func(t *testing.T, dir string) *support.CapturedInputs {
			if invocations > 0 {
				reference, err := os.ReadFile(filepath.Join(implicitRepo, ".you-agent-factory", "current-board.json"))
				if err != nil {
					t.Fatalf("implicit opening did not publish its repository reference: %v", err)
				}
				if retainedReference != nil && !bytes.Equal(retainedReference, reference) {
					t.Fatal("graceful restart replaced the selected recording reference")
				}
				retainedReference = reference
			}
			invocations++
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = implicitRepo
			return inputs
		}, 0)
	})
	t.Run("PlainBoard invalid selection rejects without activation", func(t *testing.T) {
		// These exact commands share Current Factory/~default ownership with
		// the graceful journeys. Keep this cohort ordered on the same graph.
		for _, name := range []string{"malformed reference", "foreign session reference", "foreign repository reference", "missing recording", "corrupt recording"} {
			t.Run(name, func(t *testing.T) {
				testPlainBoardRejectedSelection(t, process, name, &starts, runner)
			})
		}
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
		if starts.Load() != 6 || files.corruptReads.Load() != 1 {
			t.Errorf("startup attempts=%d corrupt probe reads=%d; want six and one", starts.Load(), files.corruptReads.Load())
		}
	})
}

func testPlainBoardRejectedSelection(t *testing.T, process support.Process, name string, starts *atomic.Int32, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, seededReplayResumeFactoryConfig()), dir); err != nil {
		t.Fatal(err)
	}
	recording := filepath.Join(home, "board.json")
	fields := map[string]string{
		"schemaVersion":    "factory-sessions.current-board.v1",
		"factoryDirectory": dir, "factorySessionId": "~default", "artifactReference": recording,
	}
	if name == "foreign repository reference" {
		fields["factoryDirectory"] = filepath.Join(repo, "sibling", "factory")
	}
	reference, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	switch name {
	case "malformed reference":
		reference = []byte(`{"private":"` + restartProbeSecret + `"`)
	case "foreign session reference":
		reference = bytes.Replace(reference, []byte(`"factorySessionId":"~default"`), []byte(`"factorySessionId":"foreign"`), 1)
	case "corrupt recording":
		writeRestartProbeFile(t, recording, []byte(`{"private":"`+restartProbeSecret+`"`))
	}
	refPath := filepath.Join(repo, ".you-agent-factory", "current-board.json")
	sentinel := filepath.Join(repo, "worktrees", "sentinel.txt")
	request := filepath.Join(repo, "request.json")
	writeRestartProbeFile(t, refPath, reference)
	writeRestartProbeFile(t, sentinel, []byte("worktree § —"))
	writeRestartProbeFile(t, request, []byte("request § —"))
	beforeStarts, beforeCalls := starts.Load(), runner.calls.Load()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repo
	err = process.Execute(inputs.Input)
	var diagnostic interface{ CLIErrorCode() string }
	if err == nil || !errors.As(err, &diagnostic) {
		t.Fatalf("invalid selection did not return a coded startup error: %v; stderr=%s", err, inputs.Stderr())
	}
	wantCode := "DURABLE_SESSION_PERSISTENCE_FAILED"
	if name == "missing recording" {
		wantCode = "CURRENT_BOARD_RECORDING_MISSING"
	} else if name == "corrupt recording" {
		wantCode = "CURRENT_BOARD_RECORDING_CORRUPT"
	}
	if diagnostic.CLIErrorCode() != wantCode {
		t.Fatalf("startup error code = %s, want %s", diagnostic.CLIErrorCode(), wantCode)
	}
	output := inputs.Stdout() + inputs.Stderr()
	if strings.Contains(output, restartProbeSecret) || strings.Contains(output, "Factory initiated:") ||
		starts.Load() != beforeStarts || runner.calls.Load() != beforeCalls {
		t.Fatalf("rejected selection exposed contents or activated runtime: %s", output)
	}
	for path, expected := range map[string][]byte{refPath: reference, sentinel: []byte("worktree § —"), request: []byte("request § —")} {
		if actual := mustReadSeededReplayArtifact(t, path); !bytes.Equal(actual, expected) {
			t.Fatalf("rejected opening changed retained file %s", path)
		}
	}
	if name == "corrupt recording" {
		if actual := mustReadSeededReplayArtifact(t, recording); !bytes.Equal(actual, []byte(`{"private":"`+restartProbeSecret+`"`)) {
			t.Fatal("rejected opening changed corrupt recording")
		}
	} else if _, err := os.Stat(recording); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("rejected opening created selected recording: %v", err)
	}
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
