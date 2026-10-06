package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	boardAPIs := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	failureAPIs := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer()}
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
			} else if index <= int32(len(boardAPIs)+len(failureAPIs)) {
				return failureAPIs[index-int32(len(boardAPIs))-1].Start(ctx, request)
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
		testRestartProbeDAGWithInputs(t, process, implicitDir, boardAPIs[2:5], runner, func(t *testing.T, dir string) *support.CapturedInputs {
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
	if !t.Run("PlainBoard legacy canonical adoption", func(t *testing.T) {
		runner.calls.Store(0)
		repo, home := t.TempDir(), t.TempDir()
		dir := filepath.Join(repo, "factory")
		if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
			t.Fatal(err)
		}
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
		invocations := 0
		testRestartProbeDAGWithInputs(t, process, dir, boardAPIs[5:8], runner, func(t *testing.T, dir string) *support.CapturedInputs {
			if invocations == 1 {
				// Simulate a pre-reference installation without altering its
				// confirmed board or canonical recording. Future relaunches
				// must use the automatically adopted reference.
				if err := os.Remove(filepath.Join(repo, ".you-agent-factory", "current-board.json")); err != nil {
					t.Fatal(err)
				}
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = repo
			invocations++
			return inputs
		}, 0)
	}) {
		return
	}
	if !t.Run("PlainBoard same JSONL recording reopen", func(t *testing.T) {
		runner.calls.Store(0)
		dir := support.ScaffoldFactory(t, config)
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
		// Reuse the same process, public ~default identity, root and JSONL
		// recording across three joined shutdowns, including terminal recovery.
		testRestartProbeDAGWithInputs(t, process, dir, boardAPIs[8:11], runner, func(t *testing.T, dir string) *support.CapturedInputs {
			inputs := restartProbeInputs(t, dir)
			inputs.Input.Args[len(inputs.Input.Args)-1] = filepath.Join(dir, "current-board.jsonl")
			return inputs
		}, 23201)
	}) {
		return
	}
	t.Run("PlainBoard F8 recovered prerequisite failure", func(t *testing.T) {
		runner.calls.Store(0)
		t.Cleanup(func() { runner.fail.Store(false) })
		repo, home := t.TempDir(), t.TempDir()
		dir := filepath.Join(repo, "factory")
		if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
			t.Fatal(err)
		}
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
		support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
		invocations := 0
		testRestartProbeDAGWithInputs(t, process, dir, failureAPIs, runner, func(t *testing.T, _ string) *support.CapturedInputs {
			runner.fail.Store(invocations > 0)
			invocations++
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = repo
			return inputs
		}, 0)
		runner.fail.Store(false)
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
		if starts.Load() != 14 || files.corruptReads.Load() != 1 {
			t.Errorf("startup attempts=%d corrupt probe reads=%d; want fourteen and one", starts.Load(), files.corruptReads.Load())
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
	assertPlainBoardRejectedSelection(t, name, home, recording, refPath, sentinel, request, reference, inputs, diagnostic.CLIErrorCode(), starts.Load()-beforeStarts, runner.calls.Load()-beforeCalls)
}

func testRestartProbeFreshBoard(t *testing.T, process support.Process, dir string, api *support.ProcessAPIServer, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	beforeCalls := runner.calls.Load()
	inputs := restartProbeInputs(t, dir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)
	session := support.GetDefaultSession(t, baseURL)
	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(baseURL, session.Id, "/work"))
	if len(works.Results) != 0 || runner.calls.Load() != beforeCalls {
		t.Fatal("fresh opening recovered Work or dispatched a worker")
	}
	command.Stop(t)
	if strings.Contains(inputs.Stderr(), "CORRUPT") || strings.Contains(inputs.Stderr(), "recovery") {
		t.Fatalf("fresh opening reported recovery failure: %s", inputs.Stderr())
	}
}

func testRestartProbeCorruptBoard(t *testing.T, process support.Process, dir string, files *restartProbeFiles, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	beforeCalls := runner.calls.Load()
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
	if strings.Contains(output, restartProbeSecret) || strings.Contains(output, "Factory initiated:") || runner.calls.Load() != beforeCalls {
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

func TestPlainBoardSiblingRepositoriesShareProfile(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	home := t.TempDir()
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &restartProbeUnexpectedRunner{requests: make(chan platformprocess.CommandRequest, 2), output: "guard § —"}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	repos := []string{t.TempDir(), t.TempDir()}
	var before [2]factoryapi.Work
	var references [2][]byte
	var artifacts [2]string
	for i, repo := range repos {
		config := seededReplayResumeFactoryConfig()
		types := config["workTypes"].([]map[string]any)
		types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
		station := config["workstations"].([]map[string]any)[0]
		station["type"] = "LOGICAL_MOVE"
		delete(station, "worker")
		if err := os.Rename(support.ScaffoldFactory(t, config), filepath.Join(repo, "factory")); err != nil {
			t.Fatal(err)
		}
		support.WriteWorkstationConfig(t, filepath.Join(repo, "factory"), "process", "---\ntype: LOGICAL_MOVE\n---\n")
		writeRestartProbeFile(t, filepath.Join(repo, "worktrees", "sentinel.txt"), []byte("untouched § —"))
		command, url := startPlainBoardInRepository(t, process, repo, home, apis[i])
		seedPlainBoardSiblingWork(t, url, repo)
		before[i] = waitForPlainBoardWorkConfirmed(t, url)
		restartProbeShutdown(t, url, command)
		references[i] = mustReadSeededReplayArtifact(t, filepath.Join(repo, ".you-agent-factory", "current-board.json"))
		var reference struct{ ArtifactReference string }
		if err := json.Unmarshal(references[i], &reference); err != nil {
			t.Fatal(err)
		}
		artifacts[i] = reference.ArtifactReference
	}
	// The first repository is now legacy; the more recently used sibling
	// still shares its profile. Adoption must preserve the sibling's bytes.
	siblingRecording := mustReadSeededReplayArtifact(t, artifacts[1])
	if err := os.Remove(filepath.Join(repos[0], ".you-agent-factory", "current-board.json")); err != nil {
		t.Fatal(err)
	}
	assertPlainBoardSiblingRecovery(t, process, home, apis, repos, before, references, artifacts, siblingRecording)
	if runner.calls.Load() != 0 {
		t.Fatal("waiting sibling Work dispatched a provider or process mutation")
	}
	// These exact-command journeys all own local ~default routing. Extend the
	// smallest ordered cohort on its existing graph for the retained guard.
	t.Run("F9 retained visit threshold", func(t *testing.T) {
		testPlainBoardRetainedVisitThreshold(t, process, home, apis[4:], runner)
	})
}

func testPlainBoardRetainedVisitThreshold(t *testing.T, process support.Process, home string, apis []*support.ProcessAPIServer, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	repo := t.TempDir()
	config := seededReplayResumeFactoryConfig()
	types := config["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
	stations := config["workstations"].([]map[string]any)
	stations[0]["outputs"] = []map[string]string{{"workType": "task", "state": "waiting"}}
	config["workstations"] = append(stations, map[string]any{
		"name": "finish-after-two-visits", "type": "LOGICAL_MOVE",
		"inputs": []map[string]string{{"workType": "task", "state": "waiting"}}, "outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		"guards": []map[string]any{{"type": "VISIT_COUNT", "workstation": "process", "maxVisits": float64(2)}},
	})
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	support.WriteWorkstationConfig(t, dir, "finish-after-two-visits", "---\ntype: LOGICAL_MOVE\n---\n")
	command, url := startPlainBoardInRepository(t, process, repo, home, apis[0])
	batch := []byte(`{"requestId":"guard-board","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"same-id","name":"same-id","workTypeName":"task","state":"init","payload":"guard § —"}]}`)
	putPlainBoardBatch(t, url, "guard-board", batch)
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 1 && runner.calls.Load() == 1
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var before factoryapi.Work
	for ctx.Err() == nil {
		before = support.GetDefaultSessionWorkByID(t, url, "same-id")
		if before.State != nil && before.State.Name == "waiting" && before.ConfirmationState != nil && *before.ConfirmationState == factoryapi.CONFIRMED {
			break
		}
	}
	if ctx.Err() != nil {
		t.Fatal("first watched visit did not rest CONFIRMED at the unsatisfied guard")
	}
	events := support.GetFactoryEventsForSessionAt(t, url, "~default")
	restartProbeShutdown(t, url, command)
	command, url = startPlainBoardInRepository(t, process, repo, home, apis[1])
	after := support.GetDefaultSessionWorkByID(t, url, "same-id")
	if after.State == nil || after.State.Name != "waiting" || !reflect.DeepEqual(before.Content, after.Content) || !reflect.DeepEqual(before.WorkId, after.WorkId) || runner.calls.Load() != 1 {
		t.Fatal("restart lost the waiting Work or released the guard prematurely")
	}
	assertRestartProbeEventFacts(t, events, support.GetFactoryEventsForSessionAt(t, url, "~default"))
	restartProbePost(t, support.SessionWorkURL(url, "~default", "/work/same-id/move"), []byte(`{"stateName":"init"}`))
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 1 })
	assertPlainBoardGuardDispatches(t, runner)
	assertRestartProbeEventFacts(t, events, support.GetFactoryEventsForSessionAt(t, url, "~default"))
	restartProbeShutdown(t, url, command)
}

func seedPlainBoardSiblingWork(t *testing.T, url, repo string) {
	t.Helper()
	// Deliberately reuse the Work ID: repository scope, rather than ID or
	// profile recency, must disambiguate these two confirmed boards.
	batch, err := json.Marshal(map[string]any{"requestId": "sibling-board", "type": "FACTORY_REQUEST_BATCH", "works": []map[string]any{
		{"workId": "same-id", "name": "same-id", "workTypeName": "task", "state": "waiting", "payload": repo + " § —", "tags": map[string]string{"repo": repo}},
		{"workId": "anchor", "name": "anchor", "workTypeName": "task", "state": "init", "payload": "durable snapshot anchor"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	putPlainBoardBatch(t, url, "sibling-board", batch)
}

func putPlainBoardBatch(t *testing.T, url, requestID string, batch []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, url+"/factory-sessions/~default/work-requests/"+requestID, bytes.NewReader(batch))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("sibling batch admission returned %d", response.StatusCode)
	}
}

func startPlainBoardInRepository(t *testing.T, process support.Process, repo, home string, api *support.ProcessAPIServer) (*support.ProcessCommand, string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repo
	command := support.StartProcessCommand(t, process, inputs.Input)
	return command, restartProbeReadyURL(t, api, command)
}

func waitForPlainBoardWorkConfirmed(t *testing.T, url string) factoryapi.Work {
	t.Helper()
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 2 && status.Categories.Terminal == 1
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		work := support.GetDefaultSessionWorkByID(t, url, "same-id")
		if work.ConfirmationState != nil && *work.ConfirmationState == factoryapi.CONFIRMED {
			return work
		}
	}
	t.Fatal("sibling Work did not become CONFIRMED")
	return factoryapi.Work{}
}

type restartProbeFiles struct {
	platformfilesystem.Local
	corruptRoot   string
	corruptReads  atomic.Int32
	corruptWrites atomic.Int32
}

func (files *restartProbeFiles) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (files *restartProbeFiles) ReadFile(path string) ([]byte, error) {
	return files.ReadFileBounded(path, 64<<20)
}

func (files *restartProbeFiles) ReadFileBounded(path string, limit int64) ([]byte, error) {
	if strings.HasPrefix(filepath.Clean(path), files.corruptRoot+string(filepath.Separator)) {
		files.corruptReads.Add(1)
		return []byte(`{"Session":` + restartProbeSecret), nil
	}
	return files.Local.ReadFileBounded(path, limit)
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
	fail     atomic.Bool
	requests chan platformprocess.CommandRequest
	output   string
}

func (runner *restartProbeUnexpectedRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	runner.requests <- request
	if runner.fail.Load() {
		return platformprocess.CommandResult{Stderr: []byte("controlled provider unavailable"), ExitCode: 1}, nil
	}
	output := runner.output
	if output == "" {
		output = "restart COMPLETE"
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}, nil
}

func assertPlainBoardRejectedSelection(t *testing.T, name, home, recording, refPath, sentinel, request string, reference []byte, inputs *support.CapturedInputs, code string, newStarts, newCalls int32) {
	t.Helper()
	wantCode := "DURABLE_SESSION_PERSISTENCE_FAILED"
	if name == "missing recording" {
		wantCode = "CURRENT_BOARD_RECORDING_MISSING"
	} else if name == "corrupt recording" {
		wantCode = "CURRENT_BOARD_RECORDING_CORRUPT"
	}
	if code != wantCode {
		t.Fatalf("startup error code = %s, want %s", code, wantCode)
	}
	output := inputs.Stdout() + inputs.Stderr()
	if strings.Contains(output, restartProbeSecret) || strings.Contains(output, "Factory initiated:") ||
		newStarts != 0 || newCalls != 0 {
		t.Fatalf("rejected selection exposed contents or activated runtime: %s", output)
	}
	assertPlainBoardRejectedFiles(t, name, home, recording, refPath, sentinel, request, reference)
}

func assertPlainBoardRejectedFiles(t *testing.T, name, home, recording, refPath, sentinel, request string, reference []byte) {
	t.Helper()
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
	// This profile started with no dated recordings. Rejection must occur
	// before the automatic target's exclusive reservation creates an artifact.
	err := filepath.WalkDir(filepath.Join(home, ".you-agent-factory", "recordings"), func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			t.Errorf("rejected opening reserved a fresh recording: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertPlainBoardSiblingRecovery(t *testing.T, process support.Process, home string, apis []*support.ProcessAPIServer, repos []string, before [2]factoryapi.Work, references [2][]byte, artifacts [2]string, siblingRecording []byte) {
	t.Helper()
	for i, repo := range repos {
		command, url := startPlainBoardInRepository(t, process, repo, home, apis[i+2])
		works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(url, "~default", "/work"))
		if len(works.Results) != 2 {
			t.Fatalf("repository %d recovered %d Work, want two", i, len(works.Results))
		}
		after := support.GetDefaultSessionWorkByID(t, url, "same-id")
		if !reflect.DeepEqual(before[i].Content, after.Content) || !reflect.DeepEqual(before[i].Tags, after.Tags) || !reflect.DeepEqual(before[i].WorkId, after.WorkId) || after.State == nil || after.State.Name != "waiting" {
			t.Fatalf("repository recovered the wrong board: before=%#v after=%#v", before[i], after)
		}
		restartProbeShutdown(t, url, command)
		if got := mustReadSeededReplayArtifact(t, filepath.Join(repo, ".you-agent-factory", "current-board.json")); !bytes.Equal(got, references[i]) {
			t.Fatal("repository selected a different recording")
		}
		if got := mustReadSeededReplayArtifact(t, filepath.Join(repo, "worktrees", "sentinel.txt")); !bytes.Equal(got, []byte("untouched § —")) {
			t.Fatal("restart mutated a worktree sentinel")
		}
		if i == 0 && !bytes.Equal(mustReadSeededReplayArtifact(t, artifacts[1]), siblingRecording) {
			t.Fatal("legacy adoption mutated the sibling recording")
		}
	}
}

func assertPlainBoardGuardDispatches(t *testing.T, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	if runner.calls.Load() != 2 {
		t.Fatalf("guard dispatched %d watched visits, want two total", runner.calls.Load())
	}
	for range 2 {
		request := <-runner.requests
		if !strings.Contains(strings.Join(request.Args, " ")+string(request.Stdin), "guard § —") {
			t.Fatalf("watched dispatch lost UTF-8 payload: args=%q stdin=%q", request.Args, request.Stdin)
		}
	}
}
