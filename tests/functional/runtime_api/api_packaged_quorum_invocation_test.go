package runtime_api

import (
	"context"
	"strings"
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
	args := map[string]interface{}{"input": "quorum request"}
	request := factoryapi.InvocationRequest{Args: &args}

	responseCh := make(chan string, 1)
	go func() {
		responseCh <- primaryResultText(t, postInvocation(t, server.URL(), request))
	}()

	runner.waitForBranchStarts(t)
	if runner.callCount("merge-quorum") != 0 {
		t.Fatalf("merge call count before branch B release = %d, want 0", runner.callCount("merge-quorum"))
	}
	close(runner.releaseBranchB)

	select {
	case got := <-responseCh:
		assertMergedQuorumResult(t, got, "quorum request")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated quorum invocation to complete")
	}
	if runner.callCount("merge-quorum") != 1 {
		t.Fatalf("merge call count = %d, want exactly 1", runner.callCount("merge-quorum"))
	}
	runner.assertMergePrompt(t, "quorum request")
	runner.assertProviderModel(t, "run-quorum-branch-a", "CODEX", "gpt-5")
	runner.assertProviderModel(t, "run-quorum-branch-b", "CODEX", "gpt-5")
	runner.assertProviderModel(t, "merge-quorum", "CODEX", "gpt-5")
}

func TestSessionInvocationAPI_PackagedQuorumAppliesRoleArguments(t *testing.T) {
	runner := newQuorumGatedCommandRunner()
	close(runner.releaseBranchB)
	server := startPackagedQuorumInvocationServer(t, runner)
	args := map[string]interface{}{
		"input":          "configured quorum request",
		"branchProvider": "CODEX",
		"branchModel":    "gpt-5.1",
		"mergeProvider":  "CODEX",
		"mergeModel":     "gpt-5.2",
	}
	request := factoryapi.InvocationRequest{Args: &args}

	response := postInvocation(t, server.URL(), request)
	for _, workstation := range []string{"run-quorum-branch-a", "run-quorum-branch-b", "merge-quorum"} {
		if runner.callCount(workstation) != 1 {
			t.Fatalf("%s call count = %d, want one configured quorum dispatch", workstation, runner.callCount(workstation))
		}
	}
	runner.assertMergePrompt(t, "configured quorum request")
	assertMergedQuorumResult(t, primaryResultText(t, response), "configured quorum request")
	runner.assertProviderModel(t, "run-quorum-branch-a", "CODEX", "gpt-5.1")
	runner.assertProviderModel(t, "run-quorum-branch-b", "CODEX", "gpt-5.1")
	runner.assertProviderModel(t, "merge-quorum", "CODEX", "gpt-5.2")
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
	mergePrompt    string
	startedA       chan struct{}
	startedB       chan struct{}
	startedAOnce   sync.Once
	startedBOnce   sync.Once
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
		r.startedAOnce.Do(func() { close(r.startedA) })
		return quorumCommandResult(request, "branch A COMPLETE"), nil
	case "run-quorum-branch-b":
		r.startedBOnce.Do(func() { close(r.startedB) })
		select {
		case <-r.releaseBranchB:
			return quorumCommandResult(request, "branch B COMPLETE"), nil
		case <-ctx.Done():
			return workers.CommandResult{}, ctx.Err()
		}
	case "merge-quorum":
		prompt := commandPrompt(request)
		r.mu.Lock()
		r.mergePrompt = prompt
		r.mu.Unlock()
		return quorumCommandResult(request, "merged quorum response:\n"+prompt+"\nCOMPLETE"), nil
	default:
		return workers.CommandResult{}, nil
	}
}

func quorumCommandResult(_ workers.CommandRequest, result string) workers.CommandResult {
	return workers.CommandResult{Stdout: []byte(result)}
}

func (r *quorumGatedCommandRunner) assertMergePrompt(t *testing.T, originalRequest string) {
	t.Helper()
	r.mu.Lock()
	prompt := r.mergePrompt
	r.mu.Unlock()
	assertPromptIncludes(t, prompt, "Original request:\n", originalRequest, "Branch A output:\n", "branch A", "Branch B output:\n", "branch B")
}

func commandPrompt(request workers.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	if len(request.Args) > 0 {
		return request.Args[len(request.Args)-1]
	}
	return ""
}

func (r *quorumGatedCommandRunner) assertProviderModel(t *testing.T, workstation, provider, model string) {
	t.Helper()
	r.mu.Lock()
	request, ok := r.requests[workstation]
	r.mu.Unlock()
	if !ok {
		t.Fatalf("no command request for %s", workstation)
	}
	if request.Command != provider || !containsArgumentPair(request.Args, "--model", model) {
		t.Fatalf("%s command = %q %#v, want %s provider with model %q", workstation, request.Command, request.Args, provider, model)
	}
}

func assertMergedQuorumResult(t *testing.T, result, originalRequest string) {
	t.Helper()
	assertPromptIncludes(t, result, "Original request:\n", originalRequest, "Branch A output:\n", "branch A", "Branch B output:\n", "branch B")
}

func assertPromptIncludes(t *testing.T, text string, values ...string) {
	t.Helper()
	lastIndex := 0
	for _, value := range values {
		nextIndex := strings.Index(text[lastIndex:], value)
		if nextIndex < 0 {
			t.Fatalf("prompt = %q, missing %q", text, value)
		}
		lastIndex += nextIndex + len(value)
	}
}

func containsArgumentPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
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
