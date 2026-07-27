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
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const runWiringWorkTypeName = "prompt-task"

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
