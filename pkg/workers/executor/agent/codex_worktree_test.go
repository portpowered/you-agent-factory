package agent_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func withAgentRunnerID(runnerID string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.RunnerID = runnerID
	}
}

func withAgentWorktree(worktree string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.Worktree = worktree
	}
}

func withAgentWorkingDirectory(workingDirectory string) func(*interfaces.WorkstationExecutionRequest) {
	return func(req *interfaces.WorkstationExecutionRequest) {
		req.WorkingDirectory = workingDirectory
	}
}

func TestAgentExecutor_CodexPreparedWorktree_OmitsWorktreeCapability(t *testing.T) {
	provider := &agentMockProvider{response: interfaces.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {Model: "gpt-5-codex", ModelProvider: string(interfaces.ModelProviderCodex)},
		},
	}, provider)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentRunnerID(interfaces.RunnerIDCodex),
		withAgentWorktree("feature-a"),
		withAgentWorkingDirectory("/tmp/factory/.worktrees/feature-a"),
		withAgentPrompts("", "run"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, capability := range provider.lastReq.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityWorktree {
			t.Fatalf("capabilities = %#v, want worktree omitted for prepared Codex dispatch", provider.lastReq.RequiredOptionalCapabilities)
		}
	}
	if provider.lastReq.Worktree != "feature-a" || provider.lastReq.WorkingDirectory != "/tmp/factory/.worktrees/feature-a" {
		t.Fatalf("request metadata = worktree %q working_directory %q", provider.lastReq.Worktree, provider.lastReq.WorkingDirectory)
	}
}
