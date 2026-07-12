package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

const packagedGoalMockWorkerAcceptedSummary = "mock worker accepted"

func TestNamedGoalRun_RealCLICompletesBatchInvocationWithPrimaryResultOnStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal batch run smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-named-goal-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	homeDir := t.TempDir()
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want only primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout.String(), goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful batch invocation", stderr.String())
	}
}

func TestNamedGoalRun_RealCLIExitsAfterBatchCompletionWithoutContinuousMode(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal batch exit smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-named-goal-exit-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	homeDir := t.TempDir()
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("you run --named %s: %v", goal.PackagedFactoryName, err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for batch invocation to exit: %v", ctx.Err())
	}
}

func TestNamedGoalRun_RealCLIUpgradesLegacyMaterializedBuiltinBeforeBatchInvocation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal legacy materialized upgrade smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-named-goal-legacy-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	homeDir := t.TempDir()
	globalRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	if _, err := factoryconfig.PersistNamedFactory(globalRoot, goal.PackagedFactoryName, legacyBuiltInGoalFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory(legacy goal): %v", err)
	}

	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s with legacy materialized builtin: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want only primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(stdout.String(), goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
}

func TestNamedGoalRun_RealCLIMaterializesFreshFactoryAndPreservesCustomerEditsOnRerun(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal fresh materialization smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-named-goal-fresh-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	homeDir := t.TempDir()
	materializedDir := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories", "@you%2Fgoal")
	if _, err := os.Stat(materializedDir); !os.IsNotExist(err) {
		t.Fatalf("fresh home should not already contain materialized @you/goal factory: stat %v", err)
	}

	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	unrelatedWorkingDir := t.TempDir()

	runNamedGoalSmokeCLI := func(goalText string) (stdout string, stderr string, runErr error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(
			ctx,
			binaryPath,
			"run",
			"--named", goal.PackagedFactoryName,
			"--with-mock-workers",
			"--no-record",
			"--server", baseURL,
			"--quiet",
			mockWorkersPath,
			goalText,
		)
		cmd.Dir = unrelatedWorkingDir
		cmd.Env = append(os.Environ(), "HOME="+homeDir)

		var outBuf, errBuf strings.Builder
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		runErr = cmd.Run()
		return outBuf.String(), errBuf.String(), runErr
	}

	stdout, stderr, err := runNamedGoalSmokeCLI(goalText)
	if err != nil {
		t.Fatalf("first you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout, stderr)
	}
	if stdout != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("first stdout = %q, want only primary result %q", stdout, packagedGoalMockWorkerAcceptedSummary)
	}
	assertNamedGoalMaterializedSplitLayout(t, materializedDir)

	workerPath := filepath.Join(materializedDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName)
	editedWorkerBody := "customer edited goal executor after first materialization\n"
	if err := os.WriteFile(workerPath, []byte(editedWorkerBody), 0o644); err != nil {
		t.Fatalf("WriteFile(customer-edited worker): %v", err)
	}

	secondGoalText := fmt.Sprintf("functional-smoke-named-goal-fresh-rerun-%d", time.Now().UnixNano())
	stdout, stderr, err = runNamedGoalSmokeCLI(secondGoalText)
	if err != nil {
		t.Fatalf("second you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout, stderr)
	}
	if stdout != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("second stdout = %q, want only primary result %q", stdout, packagedGoalMockWorkerAcceptedSummary)
	}

	workerBody, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("ReadFile(customer-edited worker): %v", err)
	}
	if strings.TrimSpace(string(workerBody)) != strings.TrimSpace(editedWorkerBody) {
		t.Fatalf("customer-edited worker body = %q, want preserved %q", string(workerBody), editedWorkerBody)
	}
}

func assertNamedGoalMaterializedSplitLayout(t *testing.T, factoryDir string) {
	t.Helper()

	for _, dirName := range []string{interfaces.WorkersDir, interfaces.WorkstationsDir} {
		info, err := os.Stat(filepath.Join(factoryDir, dirName))
		if err != nil {
			t.Fatalf("stat materialized %s: %v", dirName, err)
		}
		if !info.IsDir() {
			t.Fatalf("materialized %s is not a directory", dirName)
		}
	}
	for _, path := range []string{
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		filepath.Join(factoryDir, interfaces.WorkersDir, "goal-executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, goal.PackagedExecuteWorkstationName, interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized path %s: %v", path, err)
		}
	}
}

func TestNamedGoalRun_RealCLIPreservesCustomerEditsWhileUpgradingLegacyMaterializedBuiltin(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal legacy materialized preservation smoke")
	}

	goalText := fmt.Sprintf("functional-smoke-named-goal-preserve-edit-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	homeDir := t.TempDir()
	globalRoot := filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories")
	legacyDir, err := factoryconfig.PersistNamedFactory(globalRoot, goal.PackagedFactoryName, legacyBuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(legacy goal): %v", err)
	}
	workerPath := filepath.Join(legacyDir, "workers", "goal-executor", "AGENTS.md")
	editedWorkerBody := "customer edited worker body\n"
	if err := os.WriteFile(workerPath, []byte(editedWorkerBody), 0o644); err != nil {
		t.Fatalf("WriteFile(customer-edited worker): %v", err)
	}

	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s with customer-edited legacy materialized builtin: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want only primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}

	workerBody, err := os.ReadFile(workerPath)
	if err != nil {
		t.Fatalf("ReadFile(customer-edited worker): %v", err)
	}
	if strings.TrimSpace(string(workerBody)) != strings.TrimSpace(editedWorkerBody) {
		t.Fatalf("customer-edited worker body = %q, want preserved %q", string(workerBody), editedWorkerBody)
	}
}

var legacyBuiltInGoalFactoryJSON = []byte(`{
  "name": "@you/goal",
  "id": "builtin-goal",
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "workers": [
    {
      "name": "goal-executor",
      "type": "MODEL_WORKER",
      "body": "You are the @you/goal built-in factory worker."
    }
  ],
  "workstations": [
    {
      "name": "execute-goal",
      "type": "MODEL_WORKSTATION",
      "worker": "goal-executor",
      "inputs": [
        {"workType": "task", "state": "init"}
      ],
      "outputs": [
        {"workType": "task", "state": "complete"}
      ],
      "onFailure": [
        {"workType": "task", "state": "failed"}
      ],
      "body": "Execute the requested goal work for {{ .WorkID }}."
    }
  ]
}`)
