package agent_test

import (
	"context"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/work"
	executorpkg "github.com/portpowered/infinite-you/pkg/workers/executor"
)

func withAgentRunnerID(runnerID string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.RunnerID = runnerID
	}
}

func withAgentWorktree(worktree string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.Worktree = worktree
	}
}

func withAgentWorkingDirectory(workingDirectory string) func(*workerexecution.WorkstationExecutionRequest) {
	return func(req *workerexecution.WorkstationExecutionRequest) {
		req.WorkingDirectory = workingDirectory
	}
}

func TestAgentExecutor_CodexPreparedWorktree_OmitsWorktreeCapability(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "ok"}}
	executor := executorpkg.NewAgentExecutor(runtimefixtures.RuntimeConfigLookupFixture{
		Workers: map[string]*workerconfig.Config{
			"worker-a": {Model: "gpt-5-codex", ModelProvider: string(modelprovider.Codex)},
		},
	}, provider)

	_, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-1",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentRunnerID(workerexecution.RunnerIDCodex),
		withAgentWorktree("feature-a"),
		withAgentWorkingDirectory("/tmp/factory/.worktrees/feature-a"),
		withAgentPrompts("", "run"),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, capability := range provider.lastReq.RequiredOptionalCapabilities {
		if capability == workerexecution.RunnerOptionalCapabilityWorktree {
			t.Fatalf("capabilities = %#v, want worktree omitted for prepared Codex dispatch", provider.lastReq.RequiredOptionalCapabilities)
		}
	}
	if provider.lastReq.Worktree != "feature-a" || provider.lastReq.WorkingDirectory != "/tmp/factory/.worktrees/feature-a" {
		t.Fatalf("request metadata = worktree %q working_directory %q", provider.lastReq.Worktree, provider.lastReq.WorkingDirectory)
	}
}
