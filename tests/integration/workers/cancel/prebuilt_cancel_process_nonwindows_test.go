//go:build !windows

package cancel_test

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type cancelProcessInfo struct {
	parent int
	state  string
}

func cancelProcessSnapshot() (map[int]cancelProcessInfo, error) {
	output, err := exec.Command("ps", "-axo", "pid=,ppid=,stat=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes with ps: %w", err)
	}
	processes := make(map[int]cancelProcessInfo)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		parentPID, parentErr := strconv.Atoi(fields[1])
		if pidErr != nil || parentErr != nil {
			continue
		}
		processes[pid] = cancelProcessInfo{parent: parentPID, state: fields[2]}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return processes, nil
}

func processTreePIDs(rootPID int) ([]int, error) {
	processes, err := cancelProcessSnapshot()
	if err != nil {
		return nil, err
	}
	if _, exists := processes[rootPID]; !exists {
		return nil, nil
	}
	children := make(map[int][]int, len(processes))
	for childPID, info := range processes {
		if info.parent > 0 {
			children[info.parent] = append(children[info.parent], childPID)
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
	processes, err := cancelProcessSnapshot()
	if err != nil {
		return 0, err
	}
	info, exists := processes[pid]
	if !exists {
		return 0, fmt.Errorf("process %d is absent from the ps snapshot", pid)
	}
	return info.parent, nil
}

func processPIDPresent(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid process ID %d", pid)
	}
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, fmt.Errorf("check process %d: %w", pid, err)
}

func cleanupCancelProcessTree(tree workerProcessTree) {
	if tree.RootPID <= 0 {
		return
	}
	err := syscall.Kill(-tree.RootPID, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return
	}
	for _, pid := range tree.PIDs {
		if pid > 0 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
