package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const packagedGoalMockWorkerAcceptedSummary = "mock worker accepted"
const packagedGoalFactoryName = "@you/goal"

var goal = struct {
	PackagedFactoryName string
}{
	PackagedFactoryName: packagedGoalFactoryName,
}

func TestProviderPosture_RemovedDefaultProviderFlagIsRejected(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx,
		"run",
		"--default-worker-model-provider", "DEFAULT",
		"--no-record",
	)
	if err == nil {
		t.Fatalf("expected removed flag failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for absent provider posture")
	}

	combined := result.Stdout + result.Stderr
	for _, want := range []string{"unknown flag", "--default-worker-model-provider"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("output = %q, want documented absent-provider guidance %q", combined, want)
		}
	}
}

func TestProviderPosture_Configured_ExplicitHomeConfigEnablesNamedGoalSuccessPath(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("slow built-CLI configured provider named goal acceptance")
	}

	harness := builtcliacceptance.NewReusableHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initResult, initOutcome := initializeConfig(t, ctx, session, "configured-provider-config-init")
	configPath := initOutcome.ConfigPath
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if writeErr := os.WriteFile(configPath, configBody, 0o600); writeErr != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, writeErr)
	}

	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf("acceptance-configured-provider-%d", time.Now().UnixNano())

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		goalText,
	)

	result, err := session.Run(ctx, args...)
	session.RequireSuccess(t, "configured-provider-named-goal", result, err)

	if got := result.Stdout; got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(result.Stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful configured-provider run", result.Stderr)
	}
	if initOutcome.SystemConfigOutcome != "created" {
		t.Fatalf("init stdout = %q, want created system config outcome for %q", initResult.Stdout, configPath)
	}
}

func TestProviderPosture_Discovered_EnvDefaultResolvesWithoutFileProvider(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("slow built-CLI discovered provider named goal acceptance")
	}

	harness := builtcliacceptance.NewReusableHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	_, initOutcome := initializeConfig(t, ctx, session, "discovered-provider-config-init")
	configPath := initOutcome.ConfigPath
	configData, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%q): %v", configPath, readErr)
	}
	if strings.Contains(string(configData), "workerModelProvider") {
		t.Fatalf("operator config = %q, want no file-level workerModelProvider before env discovery", string(configData))
	}

	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf("acceptance-discovered-provider-%d", time.Now().UnixNano())

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--provider", "DEFAULT",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		goalText,
	)

	result, err := session.RunWithEnv(ctx, []string{
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=codex",
		"YOU_DEFAULT_WORKER_MODEL=gpt-5-codex",
	}, args...)
	session.RequireSuccess(t, "discovered-provider-named-goal", result, err)

	if got := result.Stdout; got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful discovered-provider run", result.Stderr)
	}
}

func writePackagedGoalMockWorkersConfig(t *testing.T) string {
	t.Helper()

	checkerCmd, checkerArgs := mockWorkerEchoCommand("plain")
	reviewerCmd, reviewerArgs := mockWorkerEchoCommand("accepted")

	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: "plan-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: "check-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: checkerCmd,
					Args:    checkerArgs,
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: "review-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: reviewerCmd,
					Args:    reviewerArgs,
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal-acceptance.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func mockWorkerEchoCommand(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(output, "'", "''")
		return "powershell.exe", []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"[Console]::Out.Write('" + literal + "')",
		}
	}
	return "/bin/echo", []string{output}
}
