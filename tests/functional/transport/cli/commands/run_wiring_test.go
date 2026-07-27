package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/goal"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	runWiringWorkTypeName              = "prompt-task"
	runWiringNamedFactoryName          = "alpha"
	runWiringPackagedGoalSummaryResult = "mock worker accepted"
)

// TestCLIRunNamedFactory proves you run resolves named and packaged Factory
// identities through the CLI and writes the expected primary-result outcome to stdout on success.
func TestCLIRunNamedFactory(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run named/packaged factory wiring")
	}

	t.Run("named_from_unrelated_working_directory", func(t *testing.T) {
		homeDir := t.TempDir()
		sourceDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
		support.CreateNamedFactory(
			t,
			homeDir,
			sourceDir,
			runWiringNamedFactoryName,
			filepath.Join(sourceDir, interfaces.FactoryConfigFile),
		)

		prompt := fmt.Sprintf("functional-run-wiring-named-%d", time.Now().UnixNano())

		port, err := reserveRunWiringLocalTCPPort()
		if err != nil {
			t.Fatalf("reserve port: %v", err)
		}
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

		mockWorkersPath := writeRunWiringMockWorkersConfig(t)
		binaryPath := buildRunWiringYouCLIBinary(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		unrelatedWorkingDir := t.TempDir()
		cmd := exec.CommandContext(
			ctx,
			binaryPath,
			"run",
			"--named", runWiringNamedFactoryName,
			"--with-mock-workers",
			"--no-record",
			"--server", baseURL,
			"--quiet",
			mockWorkersPath,
			prompt,
		)
		cmd.Dir = unrelatedWorkingDir
		cmd.Env = runWiringCustomerHomeEnvironment(homeDir)

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf(
				"you run --named %s: %v\nstdout:\n%s\nstderr:\n%s",
				runWiringNamedFactoryName,
				err,
				stdout.String(),
				stderr.String(),
			)
		}
		if got := stdout.String(); got != runWiringPackagedGoalSummaryResult {
			t.Fatalf(
				"stdout = %q, want summary primary result %q",
				got,
				runWiringPackagedGoalSummaryResult,
			)
		}
		if strings.Contains(stdout.String(), prompt) {
			t.Fatalf("stdout echoed submitted prompt text %q", prompt)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty stderr on successful named run", stderr.String())
		}
	})

	t.Run("packaged_goal_summary_primary_result", func(t *testing.T) {
		homeDir := t.TempDir()
		support.InstallPackagedFactory(t, homeDir, interfaces.PackagedGoalFactoryName)

		goalText := fmt.Sprintf("functional-run-wiring-packaged-%d", time.Now().UnixNano())
		port, err := reserveRunWiringLocalTCPPort()
		if err != nil {
			t.Fatalf("reserve port: %v", err)
		}
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

		mockWorkersPath := writeRunWiringPackagedGoalMockWorkersConfig(t)
		binaryPath := buildRunWiringYouCLIBinary(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		unrelatedWorkingDir := t.TempDir()
		cmd := exec.CommandContext(
			ctx,
			binaryPath,
			"run",
			"--named", interfaces.PackagedGoalFactoryName,
			"--with-mock-workers",
			"--no-record",
			"--server", baseURL,
			"--quiet",
			mockWorkersPath,
			goalText,
		)
		cmd.Dir = unrelatedWorkingDir
		cmd.Env = runWiringCustomerHomeEnvironment(homeDir)

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf(
				"you run --named %s: %v\nstdout:\n%s\nstderr:\n%s",
				interfaces.PackagedGoalFactoryName,
				err,
				stdout.String(),
				stderr.String(),
			)
		}
		if got := stdout.String(); got != runWiringPackagedGoalSummaryResult {
			t.Fatalf(
				"stdout = %q, want summary primary result %q",
				got,
				runWiringPackagedGoalSummaryResult,
			)
		}
		if strings.Contains(stdout.String(), goalText) {
			t.Fatalf("stdout echoed submitted goal text %q", goalText)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty stderr on successful packaged run", stderr.String())
		}
	})
}

// TestCLIRunInvalidFactoryReturnsValidationFailure proves you run fails with an
// actionable Factory load validation diagnostic when the selected Factory cannot
// be loaded, and writes no success primary-result payload to stdout.
func TestCLIRunInvalidFactoryReturnsValidationFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run invalid factory wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfigWithoutDefault())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	prompt := "missing-default-handling"

	binaryPath := buildRunWiringYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		prompt,
	)
	cmd.Dir = factoryDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected you run to fail for invalid Factory load")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you run error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("you run exit code = %d, want validation failure exit code 1", exitErr.ExitCode())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no success primary-result payload on load validation failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "handlingBehavior DEFAULT") {
		t.Fatalf(
			"stderr = %q, want actionable load/validation diagnostic for missing DEFAULT handling",
			stderr.String(),
		)
	}
}

// TestCLIRunFactoryByPath proves you run executes against an authored Factory
// filesystem path and writes the invocation primary result to stdout on success.
func TestCLIRunFactoryByPath(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-run-wiring-path-%d", time.Now().UnixNano())

	port, err := reserveRunWiringLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	binaryPath := buildRunWiringYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		prompt,
	)
	cmd.Dir = factoryDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != prompt {
		t.Fatalf("stdout = %q, want primary result %q", got, prompt)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful path run", stderr.String())
	}

	functionalevidence.Covers(t, "cli/you.run")
}

func runWiringFactoryConfigWithoutDefault() map[string]any {
	cfg := runWiringFactoryConfig()
	workTypes := cfg["workTypes"].([]map[string]any)
	withoutDefault := make([]map[string]any, len(workTypes))
	for i, workType := range workTypes {
		cloned := make(map[string]any, len(workType))
		for key, value := range workType {
			if key == "handlingBehavior" {
				continue
			}
			cloned[key] = value
		}
		withoutDefault[i] = cloned
	}
	cfg["workTypes"] = withoutDefault
	return cfg
}

func runWiringFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-run-wiring",
		"workTypes": []map[string]any{
			{
				"name":             runWiringWorkTypeName,
				"handlingBehavior": []string{"DEFAULT"},
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": runWiringWorkTypeName, "state": "init"}},
				"outputs":   []map[string]string{{"workType": runWiringWorkTypeName, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": runWiringWorkTypeName, "state": "failed"}},
			},
		},
	}
}

func runWiringCustomerHomeEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func writeRunWiringPackagedGoalMockWorkersConfig(t *testing.T) string {
	t.Helper()

	return support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: goal.PackagedPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: goal.PackagedExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: goal.PackagedCheckWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: goal.PackagedReviewWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"accepted"},
				},
			},
		},
	})
}

func writeRunWiringMockWorkersConfig(t *testing.T) string {
	t.Helper()

	data, err := json.MarshalIndent(workers.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal default mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func buildRunWiringYouCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func reserveRunWiringLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}
