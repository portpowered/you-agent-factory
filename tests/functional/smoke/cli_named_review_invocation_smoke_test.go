package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/factory/packages/review"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNamedReviewInvocationVariants_RealCLIRequireApprovalAfterRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/review invocation smoke")
	}

	for _, tc := range []struct {
		name             string
		configure        func(t *testing.T, factoryDir string)
		operatorArgs     []string
		expectedProvider string
		expectedModel    string
	}{
		{name: "defaults"},
		{
			name:             "materialized_configuration",
			expectedProvider: "codex",
			expectedModel:    "configured-codex-model",
			configure: func(t *testing.T, factoryDir string) {
				setReviewWorkerModel(t, factoryDir, "configured-codex-model")
			},
		},
		{
			name:             "model_provider_flags",
			expectedProvider: "gemini",
			expectedModel:    "flag-gemini-model",
			operatorArgs: []string{
				"--default-worker-model-provider", "GEMINI",
				"--default-worker-model", "flag-gemini-model",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, invocations, err := runNamedReviewInvocationCLIJSON(t, tc.configure, tc.operatorArgs)
			if err != nil {
				t.Fatalf("you run --named %s: %v", review.PackagedFactoryName, err)
			}
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED", response.Status)
			}
			if got := invocationPrimaryResultText(t, response); got != "approved revised candidate" {
				t.Fatalf("primaryResult = %q, want approved revised candidate", got)
			}
			if len(invocations) != 4 {
				t.Fatalf("provider invocation count = %d, want work/review followed by revised work/review", len(invocations))
			}
			assertReviewProviderInvocations(t, invocations, tc.expectedProvider, tc.expectedModel)
		})
	}
}

func runNamedReviewInvocationCLIJSON(
	t *testing.T,
	configure func(*testing.T, string),
	operatorArgs []string,
) (factoryapi.InvocationResponse, []reviewProviderInvocation, error) {
	t.Helper()

	homeDir := t.TempDir()
	factoryDir, err := factoryconfig.PersistNamedFactory(
		defaultpaths.NamedFactoriesRoot(homeDir),
		review.PackagedFactoryName,
		review.BuiltInFactoryJSON,
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", review.PackagedFactoryName, err)
	}
	if configure != nil {
		configure(t, factoryDir)
	}

	mockWorkersPath, invocationPath := writePackagedReviewMockWorkers(t)
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}

	args := []string{"--json", "run", "--named", review.PackagedFactoryName}
	args = append(args, operatorArgs...)
	args = append(args,
		"--with-mock-workers", "--no-record",
		"--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		"--quiet", mockWorkersPath,
		"write the release notes",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildYouCLIBinary(t), args...)
	cmd.Dir = t.TempDir()
	cmd.Env = namedFactorySmokeEnvironment(homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var response factoryapi.InvocationResponse
	if stdout.Len() > 0 {
		if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
			t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
		}
	}
	if runErr != nil && stderr.Len() > 0 {
		runErr = fmt.Errorf("%w\nstderr:\n%s", runErr, stderr.String())
	}
	if runErr != nil && stdout.Len() > 0 {
		runErr = fmt.Errorf("%w\nstdout:\n%s", runErr, stdout.String())
	}

	if runErr != nil {
		return response, nil, runErr
	}
	return response, reviewProviderInvocations(t, invocationPath), nil
}

type reviewProviderInvocation struct {
	Provider string          `json:"provider"`
	Args     json.RawMessage `json:"args"`
	Review   bool            `json:"review"`
}

func writePackagedReviewMockWorkers(t *testing.T) (string, string) {
	t.Helper()

	scriptDir := t.TempDir()
	invocationPath := filepath.Join(scriptDir, "provider-invocations.jsonl")
	if runtime.GOOS == "windows" {
		writePackagedReviewMockWorkerWindows(t, scriptDir, invocationPath)
	} else {
		writePackagedReviewMockWorkerPOSIX(t, scriptDir, invocationPath)
	}
	command, args := packagedReviewMockWorkerCommand(scriptDir)
	cfg := factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{
		{WorkerName: "review-work-executor", WorkstationName: review.PackagedExecuteWorkstationName, RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: command, Args: args}},
		{WorkerName: "review-work-reviewer", WorkstationName: review.PackagedReviewWorkstationName, RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: command, Args: args}},
	}}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-review.json"), invocationPath
}

func packagedReviewMockWorkerCommand(scriptDir string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(scriptDir, "mock-provider.ps1")}
	}
	return filepath.Join(scriptDir, "mock-provider.sh"), nil
}

func writePackagedReviewMockWorkerWindows(t *testing.T, scriptDir, invocationPath string) {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "mock-provider.ps1")
	script := "$originalArgs = $env:YOU_MOCK_WORKER_ARGS_JSON | ConvertFrom-Json\n" +
		"$review = $env:YOU_MOCK_WORKER_TYPE -eq 'review-work-reviewer'\n" +
		"$entry = @{provider=$env:YOU_MOCK_WORKER_COMMAND; args=$originalArgs; review=$review} | ConvertTo-Json -Compress\n" +
		"Add-Content -LiteralPath '" + invocationPath + "' -Value $entry\n" +
		"if ($review) { $count = @(Get-Content -LiteralPath '" + invocationPath + "' | ConvertFrom-Json | Where-Object { $_.review }).Count; if ($count -eq 1) { $output='{\"decision\":\"REJECTED\",\"feedback\":\"add the release date\"}' } else { $output='{\"decision\":\"ACCEPTED\",\"output\":\"approved revised candidate\"}' } } else { $output='candidate work' }\n" +
		"[Console]::Out.Write($output)\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write mock provider script: %v", err)
	}
}

func writePackagedReviewMockWorkerPOSIX(t *testing.T, scriptDir, invocationPath string) {
	t.Helper()
	script := "#!/bin/sh\nreview=false\n[ \"$YOU_MOCK_WORKER_TYPE\" = 'review-work-reviewer' ] && review=true\n" +
		"printf '{\"provider\":\"%s\",\"args\":%s,\"review\":%s}\\n' \"$YOU_MOCK_WORKER_COMMAND\" \"$YOU_MOCK_WORKER_ARGS_JSON\" \"$review\" >> \"" + invocationPath + "\"\n" +
		"if [ \"$review\" = true ]; then count=$(grep -c '\"review\":true' \"" + invocationPath + "\"); if [ \"$count\" -eq 1 ]; then printf '%s' '{\"decision\":\"REJECTED\",\"feedback\":\"add the release date\"}'; else printf '%s' '{\"decision\":\"ACCEPTED\",\"output\":\"approved revised candidate\"}'; fi; else printf '%s' 'candidate work'; fi\n"
	path := filepath.Join(scriptDir, "mock-provider.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write mock provider: %v", err)
	}
}

func reviewProviderInvocations(t *testing.T, path string) []reviewProviderInvocation {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read provider invocations: %v", err)
	}
	var invocations []reviewProviderInvocation
	for _, line := range strings.Split(strings.TrimSpace(string(payload)), "\n") {
		var invocation reviewProviderInvocation
		if err := json.Unmarshal([]byte(line), &invocation); err != nil {
			t.Fatalf("decode provider invocation %q: %v", line, err)
		}
		invocations = append(invocations, invocation)
	}
	return invocations
}

func assertReviewProviderInvocations(t *testing.T, invocations []reviewProviderInvocation, wantProvider, wantModel string) {
	t.Helper()
	for index, invocation := range invocations {
		if wantProvider != "" && invocation.Provider != wantProvider {
			t.Fatalf("provider invocation %d = %q, want %q", index, invocation.Provider, wantProvider)
		}
		if (index%2 == 1) != invocation.Review {
			t.Fatalf("provider invocation %d review = %t, want alternating work/review", index, invocation.Review)
		}
		args := providerInvocationArgs(t, invocation)
		if wantModel != "" && !containsAdjacentArgs(args, "--model", wantModel) {
			t.Fatalf("provider invocation %d args = %#v, want --model %q", index, args, wantModel)
		}
	}
}

func providerInvocationArgs(t *testing.T, invocation reviewProviderInvocation) []string {
	t.Helper()
	var args []string
	if err := json.Unmarshal(invocation.Args, &args); err == nil {
		return args
	}
	var wrapped struct {
		Value []string `json:"value"`
	}
	if err := json.Unmarshal(invocation.Args, &wrapped); err != nil || wrapped.Value == nil {
		t.Fatalf("decode provider args %s: %v", invocation.Args, err)
	}
	return wrapped.Value
}

func containsAdjacentArgs(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func setReviewWorkerModel(t *testing.T, factoryDir, model string) {
	t.Helper()
	path := filepath.Join(factoryDir, "factory.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode materialized factory: %v", err)
	}
	for _, worker := range factory["workers"].([]any) {
		definition := worker.(map[string]any)
		definition["modelProvider"] = "CODEX"
		definition["model"] = model
	}
	updated, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("encode materialized factory: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write materialized factory: %v", err)
	}
}
