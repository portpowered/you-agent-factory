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

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRunModeCompat_RealCLIOperatorContinuousRunReportsStartupOutputWithoutQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run-mode compatibility smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	wantDashboardURL := fmt.Sprintf("http://127.0.0.1:%d/dashboard/ui", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--continuously",
		mockWorkersPath,
	)
	runCmd.Dir = dir

	var stdout, stderr strings.Builder
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	if err := runCmd.Start(); err != nil {
		t.Fatalf("start operator-oriented you run: %v", err)
	}

	runWait := make(chan error, 1)
	go func() {
		runWait <- runCmd.Wait()
	}()

	if err := waitForSmokeServerReady(ctx, baseURL, 20*time.Second); err != nil {
		if waitErr := <-runWait; waitErr != nil {
			t.Fatalf("you run: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
		}
		t.Fatalf("wait for factory API: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	output := stdout.String()
	for _, want := range []string{
		"Factory initiated: " + dir,
		"Dashboard URL: " + wantDashboardURL,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want operator startup output %q", output, want)
		}
	}

	cancel()
	_ = <-runWait
}

func TestRunModeCompat_RealCLIFactoryTextInvocationSuppressesOperatorChatter(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run-mode compatibility smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	prompt := fmt.Sprintf("functional-run-mode-compat-factory-%d", time.Now().UnixNano())

	stdout, stderr, err := runFactoryPromptCLI(t, dir, binaryPath, mockWorkersPath, nil, factoryPath, prompt)
	if err != nil {
		t.Fatalf("run factory text invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	assertFactoryPromptCleanInvocationStdout(t, stdout, prompt)
}

func TestRunModeCompat_RealCLINamedGoalBatchStdoutDoesNotIncludeOperatorChatter(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run-mode compatibility smoke")
	}

	goalText := fmt.Sprintf("functional-run-mode-compat-goal-%d", time.Now().UnixNano())

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
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", goal.PackagedFactoryName, err, stdout.String(), stderr.String())
	}
	assertFactoryPromptCleanInvocationStdout(t, stdout.String(), packagedGoalMockWorkerAcceptedSummary)
}
