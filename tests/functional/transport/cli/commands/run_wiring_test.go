package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
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
		processHarness := newRunWiringRootProcessHarness(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		unrelatedWorkingDir := t.TempDir()
		cmd := processHarness.CommandContext(ctx,
			"run",
			"--named", runWiringNamedFactoryName,
			"--with-mock-workers=" + mockWorkersPath,
			"--no-record",
			"--server", baseURL,
			"--quiet",
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
		processHarness := newRunWiringRootProcessHarness(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		unrelatedWorkingDir := t.TempDir()
		cmd := processHarness.CommandContext(ctx,
			"run",
			"--named", interfaces.PackagedGoalFactoryName,
			"--with-mock-workers=" + mockWorkersPath,
			"--no-record",
			"--server", baseURL,
			"--quiet",
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

	processHarness := newRunWiringRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := processHarness.CommandContext(ctx,
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
	processHarness := newRunWiringRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := processHarness.CommandContext(ctx,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--server", baseURL,
		"--quiet",
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

// TestCLIRunFactoryWritesPrimaryResultFromStdin proves you run accepts stdin-only
// prompt input and writes the invocation primary result to stdout on success.
func TestCLIRunFactoryWritesPrimaryResultFromStdin(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run stdin wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-run-wiring-stdin-%d", time.Now().UnixNano())

	port, err := reserveRunWiringLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	processHarness := newRunWiringRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := processHarness.CommandContext(ctx,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--server", baseURL,
		"--quiet",
	)
	cmd.Dir = factoryDir
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory via stdin: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != prompt {
		t.Fatalf("stdout = %q, want stdin primary result %q", got, prompt)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful stdin invocation", stderr.String())
	}
}

// TestCLIRunRejectsConflictingPositionalAndStdinInput proves you run rejects
// simultaneous positional prompt and stdin input with a stable conflict code and
// writes no success primary-result payload to stdout.
func TestCLIRunRejectsConflictingPositionalAndStdinInput(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run input conflict wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	port, err := reserveRunWiringLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	processHarness := newRunWiringRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := processHarness.CommandContext(ctx,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--server", baseURL,
		"--quiet",
		"from positional",
	)
	cmd.Dir = factoryDir
	cmd.Stdin = strings.NewReader("from stdin")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected conflicting positional and stdin invocation inputs to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on conflict failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("stderr = %q, want stable conflict code", stderr.String())
	}
}

// TestCLIRunFailureWritesNoSuccessPayloadToStdout proves you run writes no
// success primary-result payload to stdout when invocation primary-result
// resolution fails.
func TestCLIRunFailureWritesNoSuccessPayloadToStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run unresolved primary-result wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfigWithUnresolvedInvocationReturn())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)

	port, err := reserveRunWiringLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	processHarness := newRunWiringRootProcessHarness(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := processHarness.CommandContext(ctx,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--server", baseURL,
		"--quiet",
		"trigger unresolved result",
	)
	cmd.Dir = factoryDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected unresolved invocation primary result to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invocation failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("stderr = %q, want stable unresolved-result code", stderr.String())
	}
}

// TestCLIRunCleanInvocationStdoutRemainsPipeable proves quiet you run invocations
// write only the primary result to stdout without operator lifecycle chatter so
// repeated runs remain pipeable.
func TestCLIRunCleanInvocationStdoutRemainsPipeable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run clean stdout wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	processHarness := newRunWiringRootProcessHarness(t)

	for _, prompt := range []string{
		"functional-clean-stdout-first",
		"functional-clean-stdout-second",
	} {
		stdout, stderr, err := runRunWiringFactoryCLI(
			t,
			factoryDir,
			processHarness,
			mockWorkersPath,
			nil,
			factoryPath,
			prompt,
		)
		if err != nil {
			t.Fatalf("run clean invocation for prompt %q: %v\nstdout:\n%s\nstderr:\n%s", prompt, err, stdout, stderr)
		}
		assertRunWiringCleanInvocationStdout(t, stdout, prompt)
	}

	stdout, stderr, err := runRunWiringFactoryCLI(
		t,
		factoryDir,
		processHarness,
		mockWorkersPath,
		strings.NewReader("functional-clean-stdin-only\n"),
		factoryPath,
	)
	if err != nil {
		t.Fatalf("run stdin-only clean invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	assertRunWiringCleanInvocationStdout(t, stdout, "functional-clean-stdin-only")
}

// TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup proves you run
// rejects ambiguous positional prompt and stdin input before runtime startup with
// a stable conflict diagnostic and no success stdout payload.
func TestCLIRunAmbiguousPromptAndStdinFailsBeforeRuntimeStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI run ambiguous input wiring")
	}

	factoryDir := support.ScaffoldFactory(t, runWiringFactoryConfig())
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeRunWiringMockWorkersConfig(t)
	processHarness := newRunWiringRootProcessHarness(t)

	stdout, stderr, err := runRunWiringFactoryCLI(
		t,
		factoryDir,
		processHarness,
		mockWorkersPath,
		strings.NewReader("functional-clean-stdin-conflict\n"),
		factoryPath,
		"functional-clean-positional-conflict",
	)
	if err == nil {
		t.Fatalf("expected ambiguous stdin and prompt invocation to fail\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on ambiguous input failure", stdout)
	}
	for _, want := range []string{
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"positional_text",
		"stdin_text",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want %q", stderr, want)
		}
	}
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

func runWiringFactoryConfigWithUnresolvedInvocationReturn() map[string]any {
	cfg := runWiringFactoryConfig()
	cfg["invocationReturn"] = map[string]any{
		"policy":        "EXPLICIT",
		"workTypeName":  "summary",
		"terminalState": "complete",
	}
	cfg["workTypes"] = append(cfg["workTypes"].([]map[string]any), map[string]any{
		"name": "summary",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	})
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
				WorkstationName: interfaces.PackagedGoalPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: interfaces.PackagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: interfaces.PackagedGoalCheckWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: interfaces.PackagedGoalReviewWorkstationName,
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

func newRunWiringRootProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	return builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
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

func runRunWiringFactoryCLI(
	t *testing.T,
	dir string,
	processHarness *builtcliacceptance.Harness,
	mockWorkersPath string,
	stdin *strings.Reader,
	factoryPath string,
	promptArgs ...string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	args := []string{
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--quiet",
	}
	args = append(args, promptArgs...)

	cmd := processHarness.CommandContext(ctx, args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr = cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

func assertRunWiringCleanInvocationStdout(t *testing.T, got string, want string) {
	t.Helper()

	got = strings.TrimSuffix(got, "\n")
	if got != want {
		t.Fatalf("stdout = %q, want exact primary clean invocation output", got)
	}
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Recording saved to",
		"Factory:",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stdout = %q, should not contain operator chatter %q", got, forbidden)
		}
	}
}
