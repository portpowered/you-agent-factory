package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestNamedQuorumRun_RealCLIAcceptsRoleFlagsAndReturnsOneMergeResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/quorum invocation smoke")
	}
	homeDir := t.TempDir()
	binaryPath := buildYouCLIBinary(t)
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "--json", "run", "--named", factorydefinitions.PackagedQuorumFactoryName, "--with-mock-workers", "--no-record", "--server", fmt.Sprintf("http://127.0.0.1:%d", port), writeQuorumMockWorkersConfig(t), "--branch-provider", "CLAUDE", "--branch-model", "claude-sonnet-4-20250514", "--merge-provider", "CODEX", "--merge-model", "gpt-5", "compare the two plans")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --named %s: %v\nstdout:\n%s\nstderr:\n%s", factorydefinitions.PackagedQuorumFactoryName, err, stdout.String(), stderr.String())
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
	branchACommand, branchAArgs := mockWorkerEchoCommand(
		`{"type":"result","subtype":"success","is_error":false,"result":"branch A result","session_id":"quorum-branch-a-session"}`,
	)
	branchBCommand, branchBArgs := mockWorkerEchoCommand(
		`{"type":"result","subtype":"success","is_error":false,"result":"branch B result","session_id":"quorum-branch-b-session"}`,
	)
	mergeCommand, mergeArgs := mockWorkerEchoCommand("quorum merge result")
	cfg := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{
		{WorkerName: "quorum-branch-a", WorkstationName: "run-quorum-branch-a", RunType: workers.MockWorkerRunTypeScript, ScriptConfig: &workers.MockWorkerScriptConfig{Command: branchACommand, Args: branchAArgs}},
		{WorkerName: "quorum-branch-b", WorkstationName: "run-quorum-branch-b", RunType: workers.MockWorkerRunTypeScript, ScriptConfig: &workers.MockWorkerScriptConfig{Command: branchBCommand, Args: branchBArgs}},
		{WorkerName: "quorum-merge", WorkstationName: "merge-quorum", RunType: workers.MockWorkerRunTypeScript, ScriptConfig: &workers.MockWorkerScriptConfig{Command: mergeCommand, Args: mergeArgs}},
	}}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-quorum.json")
}
