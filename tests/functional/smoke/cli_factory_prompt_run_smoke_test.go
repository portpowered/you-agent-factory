package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryPromptRun_RealCLICompletesDefaultWorkType(t *testing.T) {
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
		"--continuously",
		"--quiet",
		mockWorkersPath,
		prompt,
	)
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start you run --factory: %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	item, err := waitForFactoryPromptWorkComplete(ctx, baseURL, defaultPromptRunWorkTypeName, prompt, 20*time.Second)
	if err != nil {
		if waitErr := <-waitCh; waitErr != nil {
			t.Fatalf("you run --factory: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
		}
		t.Fatalf("wait for completed default-handling work: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if stringPointerValue(item.WorkTypeName) != defaultPromptRunWorkTypeName {
		t.Fatalf("work type = %q, want %q", stringPointerValue(item.WorkTypeName), defaultPromptRunWorkTypeName)
	}
	if !strings.HasPrefix(item.Name, "factory-prompt-") {
		t.Fatalf("work name = %q, want factory-prompt-* prefix", item.Name)
	}
	if !factoryPromptRunWorkContentIncludes(item, prompt) {
		t.Fatalf("work content = %#v, want prompt text %q", item.Content, prompt)
	}

	cancel()
	_ = <-waitCh
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
				"name":               defaultPromptRunWorkTypeName,
				"handlingBehavior":   []string{"DEFAULT"},
				"states":             promptRunWorkTypeStates(),
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

func waitForFactoryPromptWorkComplete(
	ctx context.Context,
	baseURL string,
	workTypeName string,
	wantPrompt string,
	timeout time.Duration,
) (factoryapi.Work, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return factoryapi.Work{}, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/work", nil)
		if err != nil {
			return factoryapi.Work{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return factoryapi.Work{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}

		var work factoryapi.ListWorkResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&work)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return factoryapi.Work{}, decodeErr
		}
		if resp.StatusCode != http.StatusOK {
			return factoryapi.Work{}, fmt.Errorf("GET /work status = %d", resp.StatusCode)
		}

		for _, item := range work.Results {
			if stringPointerValue(item.WorkTypeName) != workTypeName {
				continue
			}
			if factoryPromptRunWorkStateName(item.State) != "complete" {
				continue
			}
			if factoryPromptRunWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
				continue
			}
			if !factoryPromptRunWorkContentIncludes(item, wantPrompt) {
				continue
			}
			return item, nil
		}

		select {
		case <-ctx.Done():
			return factoryapi.Work{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	return factoryapi.Work{}, fmt.Errorf("timed out waiting for completed %q work with prompt %q", workTypeName, wantPrompt)
}

func factoryPromptRunWorkContentIncludes(item factoryapi.Work, wantPrompt string) bool {
	if item.Content == nil {
		return false
	}
	for _, part := range *item.Content {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if textPart.Text == wantPrompt {
			return true
		}
	}
	return false
}

func factoryPromptRunWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func factoryPromptRunWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
