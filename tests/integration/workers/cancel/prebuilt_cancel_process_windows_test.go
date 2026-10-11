//go:build windows

package cancel_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

// Retain the actual process objects before cancellation. A numeric PID opened
// after the join can already identify an unrelated process on a busy host.
func retainJoinedCancelTree(t *testing.T, tree workerProcessTree) func() {
	t.Helper()
	handles := make(map[int]windows.Handle, len(tree.PIDs))
	t.Cleanup(func() {
		for _, handle := range handles {
			if t.Failed() {
				// Failure cleanup targets only the retained fixture process object.
				_ = windows.TerminateProcess(handle, 1)
				status, err := windows.WaitForSingleObject(handle, 5000)
				if err != nil || status != uint32(windows.WAIT_OBJECT_0) {
					t.Errorf("failed fixture cleanup did not join retained process: status=%d error=%v", status, err)
				}
			}
			_ = windows.CloseHandle(handle)
		}
	})
	for _, pid := range tree.PIDs {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			t.Fatalf("retain matched process %d before cancellation: %v", pid, err)
		}
		handles[pid] = handle
		var created, exited, kernel, user windows.Filetime
		_ = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user)
		image := make([]uint16, 1024)
		size := uint32(len(image))
		_ = windows.QueryFullProcessImageName(handle, 0, &image[0], &size)
		t.Logf("retained tree root=%d child=%d grandchild=%d PID=%d created=%d image=%s", tree.RootPID, tree.ChildPID, tree.GrandchildPID, pid, created.Nanoseconds(), windows.UTF16ToString(image[:size]))
		status, err := windows.WaitForSingleObject(handle, 0)
		if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("matched process %d was not live before cancellation: status=%d error=%v", pid, status, err)
		}
	}
	return func() {
		t.Helper()
		for pid, handle := range handles {
			status, err := windows.WaitForSingleObject(handle, 0)
			if err == nil && status == uint32(windows.WAIT_TIMEOUT) {
				// Console hosts can finish asynchronous OS teardown after the
				// worker exits. Join the retained object, without killing it or
				// treating the caller's exit as proof that descendants exited.
				t.Logf("joining retained process %d after caller/Work completion", pid)
				status, err = windows.WaitForSingleObject(handle, 5000)
			}
			if err != nil || status != uint32(windows.WAIT_OBJECT_0) {
				t.Fatalf("joined cancellation left retained process %d alive: status=%d error=%v", pid, status, err)
			}
		}
		t.Logf("matched process objects exited: PIDs=%v", tree.PIDs)
	}
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
