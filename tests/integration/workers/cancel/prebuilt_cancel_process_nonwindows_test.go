//go:build !windows

package cancel_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func retainJoinedCancelTree(t *testing.T, tree workerProcessTree) func() {
	t.Helper()
	registerFailedTreeCleanup(t, tree)
	return func() { assertJoinedCancelTreeGone(t, tree) }
}

// These two executable witnesses consume the upstream CLI artifact. Functional
// tests own the broader Work assertions with controlled ScriptCommandRunner effects.
func TestPrebuiltPackagedScriptRuntime(t *testing.T) {
	t.Parallel()
	binary := resolveCancelArtifact(t)
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprintf("nonzero=%t", failure), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second) //nolint:testsleep // Failure ceiling for the real prebuilt daemon and shell, not synchronization.
			defer cancel()
			fixture := writePackagedScriptFixture(t, failure)
			hash, err := fileSHA256(binary)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("artifact=%s sha256=%s fixture=packaged-relative-shebang-v1", binary, hash)
			daemon := startCancelDaemon(t, ctx, binary, fixture)
			session := waitForCancelFactorySession(t, ctx, fixture.serverURL, daemon)
			workID := submitCancelWork(t, ctx, fixture.serverURL, session, "script-input")
			waitForFactoryForceResponse(t, ctx, fixture, session, workID)
			events, err := readFactoryEvents(ctx, fixture.serverURL, session)
			if err != nil {
				t.Fatal(err)
			}
			assertRealPackagedScriptResponse(t, events, failure)
			work := readCancelWork(t, ctx, fixture.serverURL, session, workID)
			wantState := "complete"
			if failure {
				wantState = "failed"
			}
			if work.State == nil || work.State.Name != wantState {
				t.Fatalf("script Work state = %+v, want %s", work.State, wantState)
			}
			stopCancelDaemon(t, binary, fixture, daemon)
		})
	}
}

func writePackagedScriptFixture(t *testing.T, failure bool) cancelFixture {
	t.Helper()
	fixture, err := writeCancelFixture(t)
	if err != nil {
		t.Fatal(err)
	}
	definition := `{"name":"packaged-script-runtime","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"runner","type":"SCRIPT_WORKER","command":"scripts/runtime-fixture.sh"}],"workstations":[{"name":"run-script","worker":"runner","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"definition":{"type":"SCRIPT_RUN","worker":"runner","body":"Run the packaged script."}}]}`
	if err := os.WriteFile(filepath.Join(fixture.factoryDir, "factory.json"), []byte(definition), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf 'packaged runtime success\\n'\n"
	if failure {
		script = "#!/bin/sh\nprintf 'packaged runtime failure\\n' >&2\nexit 23\n"
	}
	if err := os.WriteFile(filepath.Join(fixture.factoryDir, "scripts", "runtime-fixture.sh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertRealPackagedScriptResponse(t *testing.T, events []factoryapi.FactoryEvent, failure bool) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeScriptResponse {
			continue
		}
		response, err := event.Payload.AsScriptResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		wantCode, wantStdout, wantStderr := 0, "packaged runtime success\n", ""
		wantOutcome := factoryapi.ScriptExecutionOutcomeSucceeded
		if failure {
			wantCode, wantStdout, wantStderr = 23, "", "packaged runtime failure\n"
			wantOutcome = factoryapi.ScriptExecutionOutcomeFailedExitCode
		}
		if response.Outcome != wantOutcome || response.ExitCode == nil || *response.ExitCode != wantCode ||
			response.Stdout != wantStdout || response.Stderr != wantStderr || response.FailureType != nil {
			data, _ := json.Marshal(response)
			t.Fatalf("real shell response = %s, want %s/%d stdout=%q stderr=%q", data, wantOutcome, wantCode, wantStdout, wantStderr)
		}
		return
	}
	t.Fatal("real shell execution emitted no public script response")
}

func writeNativeForceFixture(t *testing.T, adapter string) cancelFixture {
	t.Helper()
	fixture, err := writeCancelFixture(t)
	if err != nil {
		t.Fatal(err)
	}
	provider := filepath.Join(fixture.factoryDir, "scripts")
	output := `{"type":"item.completed","item":{"id":"progress","type":"agent_message","text":"force fixture ready"}}`
	input := "cat >/dev/null\n"
	if adapter == "claude" {
		output = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"force fixture ready"}]}}`
		input = ""
	}
	script := "#!/bin/sh\nset -eu\nstate=\"$FACTORY_RELIABILITY_CANCEL_STATE\"\nname=source\n" +
		"if [ -d \"$state/source\" ]; then name=sibling; fi\n" +
		"printf '%s\\n' \"$name\" >>\"$state/native-launches\"\n" + input +
		"printf '%s\\n' '" + output + "'\n" +
		"exec sh \"$(dirname \"$0\")/cancel-worker.sh\" \"$name\" \"$state\"\n"
	if err := os.WriteFile(filepath.Join(provider, adapter), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.environment = setCancelEnvironment(fixture.environment, "PATH", provider+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fixture
}

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
	processes, snapshotErr := cancelProcessSnapshot()
	if snapshotErr != nil {
		return false, snapshotErr
	}
	if info, exists := processes[pid]; exists && strings.HasPrefix(info.state, "Z") {
		return false, nil // Exited; only its parent's deferred reap remains.
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
