package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestUnreadableSnapshotRepeatedDamagePreservesBoardEvidence(t *testing.T) {
	t.Parallel()
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
	for _, cell := range []string{"F7 preservation denied", "F7 stream read failure with preservation denied", "F12 cancel before quarantine", "F12 cancel before publication"} {
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
			fault.readError = strings.Contains(cell, "stream read failure")
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
			assertUnreadableOpeningFailureCause(t, fault, err)
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
	readError       bool
	readObserved    atomic.Bool
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

func (files *unreadableOpeningFiles) Open(path string) (io.ReadCloser, error) {
	file, err := files.Local.Open(path)
	if err != nil {
		return nil, err
	}
	if fault := files.fault.Load(); fault != nil && fault.path == path && fault.readError {
		return &unreadableOpeningStream{ReadCloser: file, fault: fault}, nil
	}
	return file, nil
}

type unreadableOpeningStream struct {
	io.ReadCloser
	fault *unreadableOpeningFault
}

func (stream *unreadableOpeningStream) Read([]byte) (int, error) {
	stream.fault.readObserved.Store(true)
	return 0, fs.ErrPermission
}

func (files *unreadableOpeningFiles) ReadFileBounded(path string, limit int64) ([]byte, error) {
	return platformfilesystem.NewRecovery(files, files.Local).ReadFileBounded(path, limit)
}

func (files *unreadableOpeningFiles) RenameNoReplace(source, destination string) error {
	if err := files.fail(source, "rename"); err != nil {
		return err
	}
	return platformfilesystem.NewRecovery(files.Local, files.Local).RenameNoReplace(source, destination)
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

func assertUnreadableOpeningFailureCause(t *testing.T, fault *unreadableOpeningFault, err error) {
	t.Helper()
	if fault.readError && !fault.readObserved.Load() {
		t.Fatal("failed opening did not encounter the snapshot stream error")
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
}
