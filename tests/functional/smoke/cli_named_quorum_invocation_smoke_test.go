package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/factory/packages/quorum"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNamedQuorumRun_RealCLIAcceptsRoleFlagsAndReturnsOneMergeResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/quorum invocation smoke")
	}
	homeDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(defaultpaths.NamedFactoriesRoot(homeDir), quorum.PackagedFactoryName, quorum.BuiltInFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory(@you/quorum): %v", err)
	}
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, buildYouCLIBinary(t), "--json", "run", "--named", quorum.PackagedFactoryName, "--with-mock-workers", "--no-record", "--server", fmt.Sprintf("http://127.0.0.1:%d", port), "--quiet", writeQuorumMockWorkersConfig(t), "--branch-provider", "CLAUDE", "--branch-model", "claude-sonnet-4-20250514", "--merge-provider", "CODEX", "--merge-model", "gpt-5", "compare the two plans")
	cmd.Dir = t.TempDir()
	cmd.Env = namedFactorySmokeEnvironment(homeDir)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstderr:\n%s", quorum.PackagedFactoryName, err, stderr.String())
	}
	var response factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &response); err != nil {
		t.Fatalf("decode quorum CLI response: %v\nstdout:\n%s", err, stdout.String())
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != "quorum merge result" {
		t.Fatalf("primary result = %q, want one final quorum merge result", got)
	}
}

func writeQuorumMockWorkersConfig(t *testing.T) string {
	t.Helper()
	branchACommand, branchAArgs := mockWorkerEchoCommand("branch A result")
	branchBCommand, branchBArgs := mockWorkerEchoCommand("branch B result")
	mergeCommand, mergeArgs := mockWorkerEchoCommand("quorum merge result")
	cfg := factoryconfig.MockWorkersConfig{MockWorkers: []factoryconfig.MockWorkerConfig{
		{WorkerName: "quorum-branch-a", WorkstationName: "run-quorum-branch-a", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: branchACommand, Args: branchAArgs}},
		{WorkerName: "quorum-branch-b", WorkstationName: "run-quorum-branch-b", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: branchBCommand, Args: branchBArgs}},
		{WorkerName: "quorum-merge", WorkstationName: "merge-quorum", RunType: factoryconfig.MockWorkerRunTypeScript, ScriptConfig: &factoryconfig.MockWorkerScriptConfig{Command: mergeCommand, Args: mergeArgs}},
	}}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-quorum.json")
}
