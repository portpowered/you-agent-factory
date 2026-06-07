package smoke

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

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryPromptRun_RealCLIWritesPrimaryResultFromPositionalText(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-smoke-factory-prompt-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
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
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != prompt {
		t.Fatalf("stdout = %q, want only primary result %q", got, prompt)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful invocation", stderr.String())
	}
}

func TestFactoryPromptRun_RealCLIWritesPrimaryResultFromStdin(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-smoke-stdin-factory-prompt-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
	)
	cmd.Dir = dir
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

func TestFactoryPromptRun_RealCLIRejectsConflictingPositionalAndStdinInput(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		"from positional",
	)
	cmd.Dir = dir
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

func TestFactoryPromptRun_RealCLIFailureWritesNoSuccessPayloadToStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfigWithUnresolvedInvocationReturn())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		"trigger unresolved result",
	)
	cmd.Dir = dir

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

func TestFactoryPromptRun_RealCLIRejectsFactoryWithoutDefaultHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfigWithoutDefault())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"missing-default-handling",
	)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure without DEFAULT handling work type, output:\n%s", output)
	}
	combined := string(output)
	if !strings.Contains(combined, "handlingBehavior DEFAULT") {
		t.Fatalf("error output = %q, want handlingBehavior DEFAULT guidance", combined)
	}
}

const defaultPromptRunWorkTypeName = "prompt-task"

func factoryPromptRunSmokeConfig() map[string]any {
	return map[string]any{
		"name": "factory-prompt-run-smoke",
		"workTypes": []map[string]any{
			{
				"name":             defaultPromptRunWorkTypeName,
				"handlingBehavior": []string{"DEFAULT"},
				"states":           promptRunWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "init"}},
				"outputs":   []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "failed"}},
			},
		},
	}
}

func factoryPromptRunSmokeConfigWithoutDefault() map[string]any {
	cfg := factoryPromptRunSmokeConfig()
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

func factoryPromptRunSmokeConfigWithUnresolvedInvocationReturn() map[string]any {
	cfg := factoryPromptRunSmokeConfig()
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

func promptRunWorkTypeStates() []map[string]string {
	return []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
}

func writeDefaultMockWorkersConfig(t *testing.T) string {
	t.Helper()

	data, err := json.MarshalIndent(factoryconfig.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal default mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func buildYouCLIBinary(t *testing.T) string {
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

func reserveLocalTCPPort() (int, error) {
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
