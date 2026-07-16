package runtime_api

import (
	"context"
	"sync"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/quorum"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedQuorumGatesMergeUntilBothBranchesComplete(t *testing.T) {
	runner := newQuorumGatedCommandRunner()
	server := startPackagedQuorumInvocationServer(t, runner)

	responseCh := make(chan string, 1)
	go func() {
		responseCh <- primaryResultText(t, postInvocation(t, server.URL(), textInvocationRequest(t, "quorum request", nil)))
	}()

	runner.waitForBranchStarts(t)
	if runner.callCount("merge-quorum") != 0 {
		t.Fatalf("merge call count before branch B release = %d, want 0", runner.callCount("merge-quorum"))
	}
	close(runner.releaseBranchB)

	select {
	case got := <-responseCh:
		if got != "merged branch A and branch B" {
			t.Fatalf("primary result = %q, want merged quorum result", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated quorum invocation to complete")
	}
	if runner.callCount("merge-quorum") != 1 {
		t.Fatalf("merge call count = %d, want exactly 1", runner.callCount("merge-quorum"))
	}
}

func TestSessionInvocationAPI_PackagedQuorumAppliesRoleArguments(t *testing.T) {
	runner := newQuorumGatedCommandRunner()
	close(runner.releaseBranchB)
	server := startPackagedQuorumInvocationServer(t, runner)
	args := map[string]interface{}{
		"input":          "configured quorum request",
		"branchProvider": "CLAUDE",
		"branchModel":    "claude-sonnet-4-20250514",
		"mergeProvider":  "CODEX",
		"mergeModel":     "gpt-5",
	}
	request := factoryapi.InvocationRequest{Args: &args}

	response := postInvocation(t, server.URL(), request)
	if got := primaryResultText(t, response); got != "merged branch A and branch B" {
		t.Fatalf("primary result = %q, want merged quorum result", got)
	}
	for _, workstation := range []string{"run-quorum-branch-a", "run-quorum-branch-b", "merge-quorum"} {
		if runner.callCount(workstation) != 1 {
			t.Fatalf("%s call count = %d, want one configured quorum dispatch", workstation, runner.callCount(workstation))
		}
	}
}

func startPackagedQuorumInvocationServer(t *testing.T, runner workers.CommandRunner) *functionalAPIServer {
	t.Helper()
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), quorum.PackagedFactoryName, quorum.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/quorum): %v", err)
	}
	return startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, runner, nil)
	})
}

type quorumGatedCommandRunner struct {
	mu             sync.Mutex
	requests       map[string]workers.CommandRequest
	startedA       chan struct{}
	startedB       chan struct{}
	releaseBranchB chan struct{}
}

func newQuorumGatedCommandRunner() *quorumGatedCommandRunner {
	return &quorumGatedCommandRunner{
		requests:       make(map[string]workers.CommandRequest),
		startedA:       make(chan struct{}),
		startedB:       make(chan struct{}),
		releaseBranchB: make(chan struct{}),
	}
}

func (r *quorumGatedCommandRunner) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.requests[request.WorkstationName] = request
	r.mu.Unlock()
	switch request.WorkstationName {
	case "run-quorum-branch-a":
		close(r.startedA)
		return workers.CommandResult{Stdout: []byte("branch A")}, nil
	case "run-quorum-branch-b":
		close(r.startedB)
		select {
		case <-r.releaseBranchB:
			return workers.CommandResult{Stdout: []byte("branch B")}, nil
		case <-ctx.Done():
			return workers.CommandResult{}, ctx.Err()
		}
	case "merge-quorum":
		return workers.CommandResult{Stdout: []byte("merged branch A and branch B")}, nil
	default:
		return workers.CommandResult{}, nil
	}
}

func (r *quorumGatedCommandRunner) waitForBranchStarts(t *testing.T) {
	t.Helper()
	for _, started := range []<-chan struct{}{r.startedA, r.startedB} {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for both quorum branches to start")
		}
	}
}

func (r *quorumGatedCommandRunner) callCount(workstation string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.requests[workstation]; ok {
		return 1
	}
	return 0
}
