package smoke

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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
	initializeCLISystemConfig(t, binaryPath, homeDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", publicGoal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", publicGoal.PackagedFactoryName, err, stdout.String(), stderr.String())
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
	initializeCLISystemConfig(t, binaryPath, homeDir)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", publicGoal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("you run --named %s: %v", publicGoal.PackagedFactoryName, err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for batch invocation to exit: %v", ctx.Err())
	}
}

func initializeCLISystemConfig(t *testing.T, binaryPath, homeDir string) {
	t.Helper()
	command := exec.Command(binaryPath, "config", "init")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("you config init: %v\noutput:\n%s", err, output)
	}
}
