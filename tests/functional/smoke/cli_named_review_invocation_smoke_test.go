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
	"strconv"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/review"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNamedReviewInvocationVariants_RealCLIRequireApprovalAfterRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/review invocation smoke")
	}

	for _, tc := range []struct {
		name         string
		configure    func(t *testing.T, factoryDir string)
		operatorArgs []string
	}{
		{name: "defaults"},
		{
			name: "materialized_configuration",
			configure: func(t *testing.T, factoryDir string) {
				setReviewWorkerModel(t, factoryDir, "configured-codex-model")
			},
		},
		{
			name: "model_provider_flags",
			operatorArgs: []string{
				"--default-worker-model-provider", "CODEX",
				"--default-worker-model", "flag-codex-model",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, reviewCalls, err := runNamedReviewInvocationCLIJSON(t, tc.configure, tc.operatorArgs)
			if err != nil {
				t.Fatalf("you run --named %s: %v", review.PackagedFactoryName, err)
			}
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("status = %q, want COMPLETED", response.Status)
			}
			if got := invocationPrimaryResultText(t, response); got != "approved revised candidate" {
				t.Fatalf("primaryResult = %q, want approved revised candidate", got)
			}
			if reviewCalls != 2 {
				t.Fatalf("review invocation count = %d, want rejection followed by approval", reviewCalls)
			}
		})
	}
}

func runNamedReviewInvocationCLIJSON(
	t *testing.T,
	configure func(*testing.T, string),
	operatorArgs []string,
) (factoryapi.InvocationResponse, int, error) {
	t.Helper()

	homeDir := t.TempDir()
	factoryDir, err := factoryconfig.PersistNamedFactory(
		filepath.Join(homeDir, ".you-agent-factory", "you-agent-factories"),
		review.PackagedFactoryName,
		review.BuiltInFactoryJSON,
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", review.PackagedFactoryName, err)
	}
	if configure != nil {
		configure(t, factoryDir)
	}

	mockWorkersPath, reviewCallsPath := writePackagedReviewMockWorkers(t)
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
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

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

	count := reviewInvocationCount(t, reviewCallsPath)
	return response, count, runErr
}

func writePackagedReviewMockWorkers(t *testing.T) (string, string) {
	t.Helper()

	scriptDir := t.TempDir()
	counterPath := filepath.Join(scriptDir, "reviewer.count")
	reviewerCommand, reviewerArgs := packagedReviewSequenceCommand(t, scriptDir, counterPath)
	executorCommand, executorArgs := mockWorkerEchoCommand("candidate work")
	cfg := factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName: "review-work-executor", WorkstationName: review.PackagedExecuteWorkstationName,
				RunType:      factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: executorCommand, Args: executorArgs},
			},
			{
				WorkerName: "review-work-reviewer", WorkstationName: review.PackagedReviewWorkstationName,
				RunType:      factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: reviewerCommand, Args: reviewerArgs},
			},
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-review.json"), counterPath
}

func packagedReviewSequenceCommand(t *testing.T, scriptDir, counterPath string) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(scriptDir, "reviewer.ps1")
		script := "$count = 0\n" +
			"if (Test-Path -LiteralPath '" + counterPath + "') { $count = [int](Get-Content -Raw -LiteralPath '" + counterPath + "') }\n" +
			"if ($count -eq 0) { [Console]::Out.Write('{\"decision\":\"REJECTED\",\"feedback\":\"add the release date\"}') } else { [Console]::Out.Write('{\"decision\":\"ACCEPTED\",\"output\":\"approved revised candidate\"}') }\n" +
			"[IO.File]::WriteAllText('" + counterPath + "', [string]($count + 1))\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
			t.Fatalf("write review sequence script: %v", err)
		}
		return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath}
	}

	scriptPath := filepath.Join(scriptDir, "reviewer.sh")
	script := "#!/bin/sh\ncount=0\nif [ -f \"" + counterPath + "\" ]; then count=$(cat \"" + counterPath + "\"); fi\n" +
		"if [ \"$count\" -eq 0 ]; then printf '%s' '{\"decision\":\"REJECTED\",\"feedback\":\"add the release date\"}'; else printf '%s' '{\"decision\":\"ACCEPTED\",\"output\":\"approved revised candidate\"}'; fi\n" +
		"printf '%s' $((count + 1)) > \"" + counterPath + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write review sequence script: %v", err)
	}
	return scriptPath, nil
}

func reviewInvocationCount(t *testing.T, path string) int {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read review invocation count: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		t.Fatalf("parse review invocation count %q: %v", payload, err)
	}
	return count
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
