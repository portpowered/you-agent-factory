//go:build windows

package cancel_test

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
