package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
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
		for _, name := range []string{"foreign session reference", "foreign repository reference", "missing recording", "corrupt recording"} {
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
	// Selection failures concern a retained durable board. Without its
	// snapshot, a valid old reference intentionally starts a fresh board.
	writeRestartProbeFile(t, filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json"), []byte(`{"Session":{"SessionID":"~default"}}`))
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

func TestUnreadableMissingSnapshotStartsQuietEmpty(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	repo, home := t.TempDir(), t.TempDir()
	config := seededReplayResumeFactoryConfig()
	types := config["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
	station := config["workstations"].([]map[string]any)[0]
	station["type"] = "LOGICAL_MOVE"
	delete(station, "worker")
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: LOGICAL_MOVE\n---\n")
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &restartProbeUnexpectedRunner{requests: make(chan platformprocess.CommandRequest, 1)}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	// Relaunches of the same repository/default board intentionally run in
	// order. This journey owns its profile, files, streams and reusable graph.
	command, url := startPlainBoardInRepository(t, process, repo, home, apis[0])
	seedPlainBoardSiblingWork(t, url, repo)
	waitForPlainBoardWorkConfirmed(t, url)
	restartProbeShutdown(t, url, command)
	oldPath := plainBoardSelectedRecording(t, repo)
	oldHistory := mustReadSeededReplayArtifact(t, oldPath)
	snapshot := filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json")
	if err := os.Remove(snapshot); err != nil {
		t.Fatal(err)
	}
	command, url, inputs := startQuietEmptyPlainBoard(t, process, repo, home, apis[1])
	seedPlainBoardSiblingWork(t, url, repo+" fresh § —")
	want := waitForPlainBoardWorkConfirmed(t, url)
	restartProbeShutdown(t, url, command)
	assertQuietMissingSnapshot(t, repo, inputs)
	if freshPath := plainBoardSelectedRecording(t, repo); freshPath == oldPath {
		t.Fatal("missing snapshot reused its stale recording")
	}
	command, url = startPlainBoardInRepository(t, process, repo, home, apis[2])
	got := waitForPlainBoardWorkConfirmed(t, url)
	assertRestartProbeRecoveredWork(t, []factoryapi.Work{want}, []factoryapi.Work{got})
	restartProbeShutdown(t, url, command)
	if !bytes.Equal(oldHistory, mustReadSeededReplayArtifact(t, oldPath)) {
		t.Fatal("fresh startup or clean restart changed old history")
	}
	// F11: a valid stale reference whose recording is also absent is quiet.
	if err := os.Remove(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plainBoardSelectedRecording(t, repo)); err != nil {
		t.Fatal(err)
	}
	command, url, inputs = startQuietEmptyPlainBoard(t, process, repo, home, apis[3])
	restartProbeShutdown(t, url, command)
	assertQuietMissingSnapshot(t, repo, inputs)
	if runner.calls.Load() != 0 {
		t.Fatal("missing snapshot dispatched stale Work")
	}
}

func plainBoardSelectedRecording(t *testing.T, repo string) string {
	t.Helper()
	var reference struct{ ArtifactReference string }
	if err := json.Unmarshal(mustReadSeededReplayArtifact(t, filepath.Join(repo, ".you-agent-factory", "current-board.json")), &reference); err != nil {
		t.Fatal(err)
	}
	return reference.ArtifactReference
}

func TestUnreadableSnapshotRepeatedDamagePreservesBoardEvidence(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	repo, home := t.TempDir(), t.TempDir()
	scaffoldUnreadableBoard(t, repo)
	apis := []*support.ProcessAPIServer{support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer(), support.NewProcessAPIServer()}
	var starts atomic.Int32
	runner := &restartProbeUnexpectedRunner{requests: make(chan platformprocess.CommandRequest, 1)}
	files := &unreadableOpeningFiles{storage: platformreplay.NewLocal(runtime.GOOS)}
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner:                      runner,
		FactorySessionRuntimePersistenceFileSystem: files,
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			return apis[starts.Add(1)-1].Start(ctx, request)
		},
	})
	support.CleanupProcess(t, process)
	command, url := startPlainBoardInRepository(t, process, repo, home, apis[0])
	seedPlainBoardSiblingWork(t, url, repo)
	waitForPlainBoardWorkConfirmed(t, url)
	restartProbeShutdown(t, url, command)
	snapshot := filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json")
	reference := filepath.Join(repo, ".you-agent-factory", "current-board.json")
	// Repeated damage to the same board is intentionally ordered: the second
	// opening must preserve archives and fresh Work produced by the first.
	for index, damaged := range [][]byte{[]byte(`{"secret":"fixture-private-prompt",`), []byte(`{"Session":42,"secret":"fixture-private-prompt"}`)} {
		oldPath := plainBoardSelectedRecording(t, repo)
		oldHistory := mustReadSeededReplayArtifact(t, oldPath)
		oldReference := mustReadSeededReplayArtifact(t, reference)
		writeRestartProbeFile(t, snapshot, damaged)
		command, url, inputs := startEmptyPlainBoard(t, process, repo, home, apis[index+1])
		cause := []string{"INVALID_JSON", "INVALID_SCHEMA"}[index]
		assertUnreadableStatus(t, url, snapshot, cause)
		if runner.calls.Load() != 0 {
			t.Fatal("damaged startup dispatched old Work")
		}
		assertUnreadableArchive(t, snapshot, damaged, index+1)
		assertUnreadableArchive(t, reference, oldReference, index+1)
		seedPlainBoardSiblingWork(t, url, repo+" fresh § —")
		waitForPlainBoardWorkConfirmed(t, url)
		assertUnreadableStatus(t, url, snapshot, cause)
		if index == 1 {
			putPlainBoardBatch(t, url, "fresh-recovery", []byte(`{"requestId":"fresh-recovery","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"fresh-recovery","name":"fresh","workTypeName":"task","state":"fresh","payload":"fresh recovery § —"}]}`))
			support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
				return status.TotalTokens == 3 && status.Categories.Terminal == 2 && runner.calls.Load() == 1
			})
		}
		restartProbeShutdown(t, url, command)
		assertUnreadableStderr(t, snapshot, damaged, cause, inputs)
		assertUnreadableArchive(t, snapshot, damaged, index+1)
		if oldPath == plainBoardSelectedRecording(t, repo) || !bytes.Equal(oldHistory, mustReadSeededReplayArtifact(t, oldPath)) {
			t.Fatal("fallback reused or changed retained history")
		}
		if strings.Contains(inputs.Stdout()+inputs.Stderr(), "fixture-private-prompt") {
			t.Fatal("startup exposed damaged content")
		}
	}
	if runner.calls.Load() != 1 {
		t.Fatal("fresh Work did not dispatch exactly once")
	}
	// F6: clean reopening removes the prior session's diagnostic and retains
	// the new Work. All generations reuse the same process graph.
	command, url, inputs := startEmptyRecoverySuccessor(t, process, repo, home, apis[3])
	support.WaitForStatus(t, url, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.TotalTokens == 3 && status.Categories.Terminal == 2
	})
	if runner.calls.Load() != 1 {
		t.Fatal("clean restart redispatched completed Work")
	}
	restartProbeShutdown(t, url, command)
	if strings.Contains(inputs.Stderr(), "Started an empty board.") {
		t.Fatal("clean restart repeated the recovery warning")
	}
	testUnreadableOversizedBoard(t, process, repo, home, snapshot, apis[4])
	testUnreadableLocalArtifacts(t, process, repo, home, apis[5:], runner)
	testUnreadableOpeningFailures(t, process, repo, home, files, &starts, runner)
}

func testUnreadableOpeningFailures(t *testing.T, process support.Process, repo, home string, files *unreadableOpeningFiles, starts *atomic.Int32, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	// These attempts reopen the same default board after its joined shutdown.
	// Serial execution protects the customer-visible single-writer invariant.
	for _, cell := range []string{"F7 preservation denied", "F12 cancel before quarantine", "F12 cancel before publication"} {
		t.Run(cell, func(t *testing.T) {
			snapshot := filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json")
			reference := filepath.Join(repo, ".you-agent-factory", "current-board.json")
			damaged := []byte(`{"secret":"fixture-private-prompt",`)
			writeRestartProbeFile(t, snapshot, damaged)
			oldReference := mustReadSeededReplayArtifact(t, reference)
			history := plainBoardSelectedRecording(t, repo)
			oldHistory := mustReadSeededReplayArtifact(t, history)
			beforeStarts, beforeCalls := starts.Load(), runner.calls.Load()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			fault := &unreadableOpeningFault{path: snapshot, operation: "rename"}
			if strings.Contains(cell, "cancel") {
				fault.cancel = cancel
			}
			publication := strings.Contains(cell, "publication")
			if publication {
				fault.path, fault.operation = reference, "write"
			}
			files.fault.Store(fault)
			defer files.fault.Store(nil)
			inputs := support.FakeInputs(ctx, []string{"you", "run", "--continuously", "--with-server"})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = repo
			err := process.Execute(inputs.Input)
			if !fault.observed.Load() || err == nil || starts.Load() != beforeStarts || runner.calls.Load() != beforeCalls {
				t.Fatalf("fault observed=%v error=%v; failed opening became ready or dispatched", fault.observed.Load(), err)
			}
			if fault.cancel != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("startup lost cancellation: %v", err)
			}
			if fault.cancel == nil {
				var diagnostic interface{ CLIErrorCode() string }
				if !errors.As(err, &diagnostic) || diagnostic.CLIErrorCode() != "DURABLE_SESSION_PERSISTENCE_FAILED" {
					t.Fatalf("preservation failure lost its safe typed diagnostic: %v", err)
				}
			}
			if publication {
				assertUnreadableArchive(t, snapshot, damaged, 6)
				assertUnreadableArchive(t, reference, oldReference, 6)
			} else if !bytes.Equal(damaged, mustReadSeededReplayArtifact(t, snapshot)) || !bytes.Equal(oldReference, mustReadSeededReplayArtifact(t, reference)) {
				t.Fatal("failed preservation changed original evidence")
			}
			if !bytes.Equal(oldHistory, mustReadSeededReplayArtifact(t, history)) {
				t.Fatal("failed opening changed retained history")
			}
			if output := inputs.Stdout() + inputs.Stderr(); strings.Contains(output, "fixture-private-prompt") || strings.Contains(output, "Started an empty board.") {
				t.Fatal("failed opening leaked content or claimed recovered success")
			}
		})
	}
}

type unreadableOpeningFault struct {
	path, operation string
	cancel          context.CancelFunc
	observed        atomic.Bool
}

type unreadableOpeningFiles struct {
	restartProbeFiles
	storage platformreplay.Storage
	fault   atomic.Pointer[unreadableOpeningFault]
}

func (files *unreadableOpeningFiles) fail(path, operation string) error {
	if fault := files.fault.Load(); fault != nil && fault.path == path && fault.operation == operation {
		fault.observed.Store(true)
		if fault.cancel != nil {
			fault.cancel()
			return context.Canceled
		}
		return fs.ErrPermission
	}
	return nil
}

func (files *unreadableOpeningFiles) RenameNoReplace(source, destination string) error {
	if err := files.fail(source, "rename"); err != nil {
		return err
	}
	return files.Local.RenameNoReplace(source, destination)
}

func (files *unreadableOpeningFiles) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if err := files.fail(path, "write"); err != nil {
		return err
	}
	return files.storage.WriteFile(path, data)
}

func scaffoldUnreadableBoard(t *testing.T, repo string) {
	t.Helper()
	config := seededReplayResumeFactoryConfig()
	types := config["workTypes"].([]map[string]any)
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "waiting", "type": "PROCESSING"})
	station := config["workstations"].([]map[string]any)[0]
	station["type"] = "LOGICAL_MOVE"
	delete(station, "worker")
	types[0]["states"] = append(types[0]["states"].([]map[string]string), map[string]string{"name": "fresh", "type": "PROCESSING"})
	config["workstations"] = append(config["workstations"].([]map[string]any), map[string]any{
		"name": "fresh-process", "worker": "worker-a",
		"inputs":  []map[string]string{{"workType": "task", "state": "fresh"}},
		"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
	})
	dir := filepath.Join(repo, "factory")
	if err := os.Rename(support.ScaffoldFactory(t, config), dir); err != nil {
		t.Fatal(err)
	}
	support.WriteWorkstationConfig(t, dir, "process", "---\ntype: LOGICAL_MOVE\n---\n")
	support.WriteWorkstationConfig(t, dir, "fresh-process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\n---\n")
}

func testUnreadableOversizedBoard(t *testing.T, process support.Process, repo, home, snapshot string, api *support.ProcessAPIServer) {
	t.Helper()
	// F4 owns one real cap+1 file. Stream its checksum rather than decoding or
	// retaining another full copy of the oversized bytes in the witness.
	file, err := os.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("fixture-private-prompt"); err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate((64 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	wantHash := unreadableFileChecksum(t, snapshot)
	command, url, inputs := startEmptyPlainBoard(t, process, repo, home, api)
	assertUnreadableStatus(t, url, snapshot, "SIZE_LIMIT")
	status := support.GetJSON[factoryapi.StatusResponse](t, url+"/status")
	archive := status.StartupRecovery.QuarantinedFile
	if got := unreadableFileChecksum(t, archive); got != wantHash {
		t.Fatal("oversized archive changed bytes")
	}
	seedPlainBoardSiblingWork(t, url, repo+" oversized fresh")
	waitForPlainBoardWorkConfirmed(t, url)
	restartProbeShutdown(t, url, command)
	wantLine := fmt.Sprintf("Durable state %q quarantined as %q: SIZE_LIMIT. Started an empty board.\n", snapshot, archive)
	if strings.Count(inputs.Stderr(), wantLine) != 1 || strings.Contains(inputs.Stdout()+inputs.Stderr(), "fixture-private-prompt") {
		t.Fatal("oversized warning leaked content or was missing/repeated")
	}
}

func unreadableFileChecksum(t *testing.T, path string) [32]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func startEmptyRecoverySuccessor(t *testing.T, process support.Process, repo, home string, api *support.ProcessAPIServer) (*support.ProcessCommand, string, *support.CapturedInputs) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repo
	command := support.StartProcessCommand(t, process, inputs.Input)
	url := restartProbeReadyURL(t, api, command)
	status := support.GetJSON[factoryapi.StatusResponse](t, url+"/status")
	if status.StartupRecovery != nil {
		t.Fatal("clean successor retained a closed session's recovery condition")
	}
	return command, url, inputs
}

func assertUnreadableStatus(t *testing.T, url, snapshot, cause string) {
	t.Helper()
	for _, route := range []string{"/status", "/factory-sessions/~default/status"} {
		status := support.GetJSON[factoryapi.StatusResponse](t, url+route)
		recovery := status.StartupRecovery
		if recovery == nil || recovery.Code != "DURABLE_STATE_QUARANTINED" || recovery.Cause != factoryapi.StatusResponseStartupRecoveryCause(cause) || recovery.File != snapshot {
			t.Fatalf("%s recovery = %#v, want preserved startup condition", route, recovery)
		}
		if !strings.HasPrefix(recovery.QuarantinedFile, snapshot+".unreadable.") {
			t.Fatal("status omitted the quarantine path")
		}
		encoded, err := json.Marshal(status)
		if err != nil || bytes.Contains(encoded, []byte("fixture-private-prompt")) {
			t.Fatal("status exposed damaged content or could not be encoded")
		}
	}
}

func assertUnreadableStderr(t *testing.T, snapshot string, damaged []byte, cause string, inputs *support.CapturedInputs) {
	t.Helper()
	archives, err := filepath.Glob(snapshot + ".unreadable.*")
	if err != nil {
		t.Fatal(err)
	}
	for _, archive := range archives {
		if !bytes.Equal(damaged, mustReadSeededReplayArtifact(t, archive)) {
			continue
		}
		want := fmt.Sprintf("Durable state %q quarantined as %q: %s. Started an empty board.\n", snapshot, archive, cause)
		if got := inputs.Stderr(); strings.Count(got, want) != 1 || strings.Count(got, "Started an empty board.") != 1 {
			t.Fatalf("expected one safe recovery warning %q, got %q", want, got)
		}
		if strings.Contains(inputs.Stdout(), "Started an empty board.") {
			t.Fatal("recovery warning was written to stdout")
		}
		return
	}
	t.Fatal("warning has no byte-preserving archive")
}

func assertUnreadableArchive(t *testing.T, path string, want []byte, count int) {
	t.Helper()
	archives, err := filepath.Glob(path + ".unreadable.*")
	if err != nil || len(archives) != count {
		t.Fatalf("archive count=%d, want %d: %v", len(archives), count, err)
	}
	for _, archive := range archives {
		if bytes.Equal(want, mustReadSeededReplayArtifact(t, archive)) {
			return
		}
	}
	t.Fatal("quarantine did not preserve exact bytes")
}

func startQuietEmptyPlainBoard(t *testing.T, process support.Process, repo, home string, api *support.ProcessAPIServer) (*support.ProcessCommand, string, *support.CapturedInputs) {
	t.Helper()
	command, url, inputs := startEmptyPlainBoard(t, process, repo, home, api)
	raw := support.GetJSON[map[string]any](t, url+"/status")
	if _, present := raw["startupRecovery"]; present {
		t.Fatal("missing snapshot reported a recovery diagnostic")
	}
	return command, url, inputs
}

func startEmptyPlainBoard(t *testing.T, process support.Process, repo, home string, api *support.ProcessAPIServer) (*support.ProcessCommand, string, *support.CapturedInputs) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--continuously", "--with-server"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repo
	command := support.StartProcessCommand(t, process, inputs.Input)
	url := restartProbeReadyURL(t, api, command)
	status := support.GetJSON[factoryapi.StatusResponse](t, url+"/status")
	works := support.GetJSON[factoryapi.ListWorkResponse](t, url+"/factory-sessions/~default/work")
	if status.TotalTokens != 0 || len(works.Results) != 0 {
		t.Fatal("empty startup replayed stale Work")
	}
	return command, url, inputs
}

func assertQuietMissingSnapshot(t *testing.T, repo string, inputs *support.CapturedInputs) {
	t.Helper()
	if output := inputs.Stderr(); strings.Contains(output, "quarantined") || strings.Contains(output, "recovery") {
		t.Fatalf("missing snapshot emitted recovery warning: %s", output)
	}
	archives, err := filepath.Glob(filepath.Join(repo, ".you-agent-factory", "durable-sessions", "*.unreadable.*"))
	if err != nil || len(archives) != 0 {
		t.Fatalf("missing snapshot created quarantine archives: %v, %v", archives, err)
	}
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

func testUnreadableLocalArtifacts(t *testing.T, process support.Process, repo, home string, apis []*support.ProcessAPIServer, runner *restartProbeUnexpectedRunner) {
	t.Helper()
	// F9 cells reopen the same default board in order after joined shutdown.
	for index, cell := range []string{"local reference", "scoped recording"} {
		t.Run("F9 "+cell, func(t *testing.T) {
			snapshot := filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json")
			reference := filepath.Join(repo, ".you-agent-factory", "current-board.json")
			history := plainBoardSelectedRecording(t, repo)
			prior := map[string][]byte{snapshot: mustReadSeededReplayArtifact(t, snapshot), reference: mustReadSeededReplayArtifact(t, reference), history: mustReadSeededReplayArtifact(t, history)}
			source, cause := reference, "INVALID_JSON"
			if cell == "scoped recording" {
				source, cause = history, "INVALID_SCHEMA"
			}
			damaged := []byte(`{"private":"fixture-private-prompt",`)
			writeRestartProbeFile(t, source, damaged)
			prior[source] = damaged
			beforeCalls := runner.calls.Load()
			command, url, inputs := startEmptyPlainBoard(t, process, repo, home, apis[index])
			assertUnreadableStatus(t, url, source, cause)
			if runner.calls.Load() != beforeCalls {
				t.Fatal("artifact fallback dispatched prior Work")
			}
			seedPlainBoardSiblingWork(t, url, repo+" F9 § —")
			waitForPlainBoardWorkConfirmed(t, url)
			restartProbeShutdown(t, url, command)
			assertUnreadableStderr(t, source, damaged, cause, inputs)
			for path, data := range prior {
				if path == history && source != history {
					if !bytes.Equal(data, mustReadSeededReplayArtifact(t, path)) {
						t.Fatal("reference fallback changed prior recording")
					}
					continue
				}
				archives, err := filepath.Glob(path + ".unreadable.*")
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, archive := range archives {
					if bytes.Equal(data, mustReadSeededReplayArtifact(t, archive)) {
						found = true
					}
				}
				if !found {
					t.Fatalf("fallback lost retained evidence for %s", path)
				}
			}
			if history == plainBoardSelectedRecording(t, repo) {
				t.Fatal("fallback reused selected history")
			}
			if strings.Contains(inputs.Stdout()+inputs.Stderr(), "fixture-private-prompt") {
				t.Fatal("artifact fallback disclosed content")
			}
		})
	}
}
