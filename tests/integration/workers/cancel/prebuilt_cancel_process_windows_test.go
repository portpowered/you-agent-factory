//go:build windows

package cancel_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"golang.org/x/sys/windows"
)

// This Windows spine proves direct native force and archived recovery. Factory
// dispatch replay and the Linux witness remain distinct retained requirements.
func TestPrebuiltWorkerSessionForceDirectTreeAndArchive(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			proveNativeForceTreeAndArchive(t, provider)
		})
	}
}

func proveNativeForceTreeAndArchive(t *testing.T, provider string) {
	t.Helper()
	binary := resolveCancelArtifact(t)
	ctx, cancel := context.WithTimeout(t.Context(), 110*time.Second)
	defer cancel()
	fixture := writeNativeForceFixture(t, provider)
	hash, err := fileSHA256(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%s OS=%s/%s fixture=direct-force-tree-v1", binary, hash, runtime.GOOS, runtime.GOARCH)
	daemon := startCancelDaemon(t, ctx, binary, fixture)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("native force daemon stdout=%s stderr=%s", daemon.stdout.String(), daemon.stderr.String())
		}
	})
	session := waitForCancelFactorySession(t, ctx, fixture.serverURL, daemon)
	source := invokeNativeForceFixture(t, ctx, binary, fixture, session, provider, "source")
	sourceTree := waitForFixtureProcessTree(t, ctx, fixture.stateDir, "source")
	registerFailedTreeCleanup(t, sourceTree)
	assertObservedTreeAncestry(t, sourceTree)
	invokeNativeForceFixture(t, ctx, binary, fixture, session, provider, "sibling")
	siblingTree := waitForFixtureProcessTree(t, ctx, fixture.stateDir, "sibling")
	registerFailedTreeCleanup(t, siblingTree)
	assertObservedTreeAncestry(t, siblingTree)
	// The .cmd launcher is the actual command root; retain it in the census
	// along with the PowerShell root and its child/grandchild readiness facts.
	parents, err := processSnapshotParents()
	if err != nil || parents[sourceTree.RootPID] == 0 {
		t.Fatalf("resolve live provider launcher ancestry: %v", err)
	}
	sourceTree.PIDs = append(sourceTree.PIDs, parents[sourceTree.RootPID])
	assertNativeStaleForceRefused(t, ctx, binary, fixture, source)
	assertNativeTreesLive(t, sourceTree, siblingTree)
	result := forceNativeFixture(t, ctx, binary, fixture, source)
	sample, err := captureProcessSample(sourceTree, siblingTree, time.Now())
	if err != nil || len(sample.TargetPresent) != 0 || !reflect.DeepEqual(sample.UnrelatedPresent, siblingTree.PIDs) {
		t.Fatalf("APPLIED did not join exact tree with live sibling: sample=%+v error=%v", sample, err)
	}
	assertForcedNativeArchive(t, ctx, fixture, source)
	if retry := forceNativeFixture(t, ctx, binary, fixture, source); !reflect.DeepEqual(result, retry) {
		t.Fatalf("committed retry changed: original=%+v retry=%+v", result, retry)
	}
	assertNativeTerminalForceNoop(t, ctx, binary, fixture, source)
	assertNativeTreesLive(t, siblingTree)
	assertNativeLaunchCount(t, fixture, 2)
	stopCancelDaemon(t, binary, fixture, daemon)
	daemon = startCancelDaemon(t, ctx, binary, fixture)
	waitForCancelFactorySession(t, ctx, fixture.serverURL, daemon)
	assertForcedNativeArchive(t, ctx, fixture, source)
	if retry := forceNativeFixture(t, ctx, binary, fixture, source); !reflect.DeepEqual(result, retry) {
		t.Fatalf("restart retry changed: original=%+v retry=%+v", result, retry)
	}
	assertNativeLaunchCount(t, fixture, 2)
	stopCancelDaemon(t, binary, fixture, daemon)
	t.Log("PASS: direct force joined native launcher/root/child/grandchild before APPLIED; sibling survived; archive and committed retry survived joined host restart")
}

func writeNativeForceFixture(t *testing.T, adapter string) cancelFixture {
	t.Helper()
	fixture, err := writeCancelFixture(t)
	if err != nil {
		t.Fatal(err)
	}
	provider := filepath.Join(fixture.factoryDir, "scripts")
	files := map[string]string{
		"codex.cmd": "@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File \"%~dp0codex.ps1\"\r\nexit /b %errorlevel%\r\n",
		"codex.ps1": `$ErrorActionPreference = "Stop"
$state = $env:FACTORY_RELIABILITY_CANCEL_STATE
$name = "source"
if (Test-Path -LiteralPath (Join-Path $state "source")) { $name = "sibling" }
[System.IO.File]::AppendAllText((Join-Path $state "native-launches"), $name + [Environment]::NewLine)
$null = [Console]::In.ReadToEnd()
[Console]::WriteLine('{"type":"item.completed","item":{"id":"progress","type":"agent_message","text":"force fixture ready"}}')
[Console]::Out.Flush()
& (Join-Path $PSScriptRoot "cancel-worker.ps1") -WorkID $name -StateRoot $state
`,
	}
	if adapter == "claude" {
		files = map[string]string{
			"claude.cmd": "@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File \"%~dp0claude.ps1\"\r\nexit /b %errorlevel%\r\n",
			"claude.ps1": `$ErrorActionPreference = "Stop"
$state = $env:FACTORY_RELIABILITY_CANCEL_STATE
$name = "source"
if (Test-Path -LiteralPath (Join-Path $state "source")) { $name = "sibling" }
[System.IO.File]::AppendAllText((Join-Path $state "native-launches"), $name + [Environment]::NewLine)
[Console]::WriteLine('{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"force fixture ready"}]}}')
[Console]::Out.Flush()
& (Join-Path $PSScriptRoot "cancel-worker.ps1") -WorkID $name -StateRoot $state
`,
		}
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(provider, name), []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.environment = setCancelEnvironment(fixture.environment, "PATH", provider+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fixture
}

func assertNativeLaunchCount(t *testing.T, fixture cancelFixture, want int) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(fixture.stateDir, "native-launches"))
	if err != nil || len(strings.Fields(string(content))) != want {
		t.Fatalf("provider launches = %q, error=%v; want exactly %d with no force/restart retry", content, err, want)
	}
}

func invokeNativeForceFixture(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, session, provider, name string) factoryapi.WorkerSessionObservation {
	t.Helper()
	id := "force-" + name
	document := map[string]any{"execution": map[string]any{
		"factorySessionId": session, "workstationName": "__provider_invocation__",
		"dispatch":   map[string]any{"dispatchId": id + "-dispatch", "workstationName": "__provider_invocation__", "workerType": "process-worker"},
		"workerType": "process-worker", "runnerId": provider, "executorProvider": provider, "modelProvider": provider, "model": "force-fixture",
		"workingDirectory": fixture.factoryDir, "workingDirectoryAuthored": true, "userMessage": name,
	}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "invoke",
		"--request-id", id+"-invoke", "--worker-session-id", id, "--dispatch-id", id+"-dispatch", "--execution", string(encoded), "--retry-max-attempts", "1", "--async")
	if result.err != nil {
		t.Fatalf("invoke native fixture: %+v", result)
	}
	waitForFixtureProcessTree(t, ctx, fixture.stateDir, name)
	observation, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+id)
	if err != nil || string(observation.State) != "RUNNING" || observation.ProviderSession != nil {
		t.Fatalf("native fixture before force: observation=%+v error=%v", observation, err)
	}
	return observation
}

func assertNativeTreesLive(t *testing.T, trees ...workerProcessTree) {
	t.Helper()
	for _, tree := range trees {
		for _, pid := range tree.PIDs {
			present, err := processPIDPresent(pid)
			if err != nil || !present {
				t.Fatalf("refused control affected live %s process %d: present=%t error=%v", tree.WorkID, pid, present, err)
			}
		}
	}
}

func assertNativeStaleForceRefused(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-stale", "--expected-attempt-id", observation.AttemptId+"-stale")
	var response factoryapi.ErrorResponse
	if result.err == nil || json.Unmarshal([]byte(result.stderr), &response) != nil || string(response.Code) != "WORKER_SESSION_CONTROL_CONFLICT" {
		t.Fatalf("stale force must return typed conflict: result=%+v response=%+v", result, response)
	}
}

func assertNativeTerminalForceNoop(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-expired", "--expected-attempt-id", observation.AttemptId)
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != "NOOP" || string(response.State) != "TERMINATED" {
		t.Fatalf("terminal force must be a no-op: result=%+v response=%+v", result, response)
	}
	assertForcedNativeArchive(t, ctx, fixture, observation)
}

func forceNativeFixture(t *testing.T, ctx context.Context, binary string, fixture cancelFixture, observation factoryapi.WorkerSessionObservation) factoryapi.WorkerSessionControlResponse {
	t.Helper()
	result := runCancelCLI(ctx, binary, fixture, "--remote", "--server", fixture.serverURL, "--json", "worker-sessions", "terminate", observation.WorkerSessionId,
		"--force", "--request-id", observation.WorkerSessionId+"-kill", "--expected-attempt-id", observation.AttemptId)
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || string(response.Outcome) != "APPLIED" || string(response.State) != "TERMINATED" || response.Forced == nil || !*response.Forced {
		current, readErr := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+observation.WorkerSessionId)
		t.Logf("force failure observation=%+v error=%v", current, readErr)
		t.Fatalf("native force: result=%+v response=%+v", result, response)
	}
	return response
}

func assertForcedNativeArchive(t *testing.T, ctx context.Context, fixture cancelFixture, original factoryapi.WorkerSessionObservation) {
	t.Helper()
	observation, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, fixture.serverURL+"/worker-sessions/"+original.WorkerSessionId)
	if err != nil || string(observation.State) != "TERMINATED" || observation.TerminalCause == nil || string(*observation.TerminalCause) != "OPERATOR_KILL" || observation.AttemptId != original.AttemptId {
		t.Fatalf("force terminal archive: observation=%+v error=%v", observation, err)
	}
}

func processSnapshotParents() (map[int]int, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	parents := make(map[int]int)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return parents, nil
		}
		return nil, err
	}
	for {
		if entry.ProcessID != 0 {
			parents[int(entry.ProcessID)] = int(entry.ParentProcessID)
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, err
		}
	}
	return parents, nil
}

func processTreePIDs(rootPID int) ([]int, error) {
	parents, err := processSnapshotParents()
	if err != nil {
		return nil, err
	}
	if _, exists := parents[rootPID]; !exists {
		return nil, nil
	}
	children := make(map[int][]int, len(parents))
	for childPID, parentPID := range parents {
		if parentPID > 0 {
			children[parentPID] = append(children[parentPID], childPID)
		}
	}
	visited := map[int]struct{}{rootPID: {}}
	result := []int{rootPID}
	var visit func(int)
	visit = func(parentPID int) {
		for _, childPID := range children[parentPID] {
			if _, seen := visited[childPID]; seen {
				continue
			}
			visited[childPID] = struct{}{}
			visit(childPID)
			result = append(result, childPID)
		}
	}
	visit(rootPID)
	sort.Ints(result)
	return result, nil
}

func processParentPID(pid int) (int, error) {
	parents, err := processSnapshotParents()
	if err != nil {
		return 0, err
	}
	parent, exists := parents[pid]
	if !exists {
		return 0, fmt.Errorf("process %d is absent from the Windows process snapshot", pid)
	}
	return parent, nil
}

func processPIDPresent(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid process ID %d", pid)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, fmt.Errorf("check process %d: %w", pid, err)
	}
	if status == uint32(windows.WAIT_OBJECT_0) {
		return false, nil
	}
	if status != uint32(windows.WAIT_TIMEOUT) {
		return false, fmt.Errorf("check process %d returned wait status %d", pid, status)
	}
	return true, nil
}

func cleanupCancelProcessTree(tree workerProcessTree) {
	if tree.RootPID > 0 {
		_ = exec.Command("taskkill", "/PID", strconv.Itoa(tree.RootPID), "/T", "/F").Run()
	}
	for _, pid := range tree.PIDs {
		if pid <= 0 {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			continue
		}
		_ = windows.TerminateProcess(handle, 1)
		_ = windows.CloseHandle(handle)
	}
}
