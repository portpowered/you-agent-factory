package runtime_api

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	// Public authored vocabulary remains CODEX; runtime command identity is the
	// canonical internal provider command (codex) after registry resolution.
	codex := string(modelprovider.ProviderCodex)
	runner.assertProviderModel(t, "run-quorum-branch-a", codex, "gpt-5")
	runner.assertProviderModel(t, "run-quorum-branch-b", codex, "gpt-5")
	runner.assertProviderModel(t, "merge-quorum", codex, "gpt-5")
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
	codex := string(modelprovider.ProviderCodex)
	runner.assertProviderModel(t, "run-quorum-branch-a", codex, "gpt-5.1")
	runner.assertProviderModel(t, "run-quorum-branch-b", codex, "gpt-5.1")
	runner.assertProviderModel(t, "merge-quorum", codex, "gpt-5.2")
}

func startPackagedQuorumInvocationServer(t *testing.T, runner platformprocess.CommandRunner) *functionalAPIServer {
	t.Helper()
	dir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedQuorumFactoryName)
	return startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(runner, nil))
}

type quorumGatedCommandRunner struct {
	mu             sync.Mutex
	requests       map[string]platformprocess.CommandRequest
	callCounts     map[string]int
	mergePrompt    string
	startedA       chan struct{}
	startedB       chan struct{}
	startedAOnce   sync.Once
	startedBOnce   sync.Once
	releaseBranchB chan struct{}
}

func newQuorumGatedCommandRunner() *quorumGatedCommandRunner {
	return &quorumGatedCommandRunner{
		requests:       make(map[string]platformprocess.CommandRequest),
		callCounts:     make(map[string]int),
		startedA:       make(chan struct{}),
		startedB:       make(chan struct{}),
		releaseBranchB: make(chan struct{}),
	}
}

func (r *quorumGatedCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	lane := quorumRequestLane(request)
	r.mu.Lock()
	r.requests[lane] = request
	r.callCounts[lane]++
	r.mu.Unlock()
	switch lane {
	case "run-quorum-branch-a":
		r.startedAOnce.Do(func() { close(r.startedA) })
		return quorumCommandResult(request, "branch A COMPLETE"), nil
	case "run-quorum-branch-b":
		r.startedBOnce.Do(func() { close(r.startedB) })
		select {
		case <-r.releaseBranchB:
			return quorumCommandResult(request, "branch B COMPLETE"), nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	case "merge-quorum":
		prompt := commandPrompt(request)
		r.mu.Lock()
		r.mergePrompt = prompt
		r.mu.Unlock()
		return quorumCommandResult(request, "merged quorum response:\n"+prompt+"\nCOMPLETE"), nil
	default:
		return platformprocess.CommandResult{}, nil
	}
}

func quorumCommandResult(_ platformprocess.CommandRequest, result string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: []byte(result)}
}

func (r *quorumGatedCommandRunner) assertMergePrompt(t *testing.T, originalRequest string) {
	t.Helper()
	r.mu.Lock()
	prompt := r.mergePrompt
	r.mu.Unlock()
	assertPromptIncludes(t, prompt, "Original request:\n", originalRequest, "Branch A output:\n", "branch A", "Branch B output:\n", "branch B")
}

func commandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	if len(request.Args) > 0 {
		return request.Args[len(request.Args)-1]
	}
	return ""
}

func quorumRequestLane(request platformprocess.CommandRequest) string {
	prompt := commandPrompt(request)
	switch {
	case strings.Contains(prompt, "Produce branch A's independent assessment"):
		return "run-quorum-branch-a"
	case strings.Contains(prompt, "Produce branch B's independent assessment"):
		return "run-quorum-branch-b"
	case strings.Contains(prompt, "Synthesize the two quorum assessments"):
		return "merge-quorum"
	default:
		return "unknown"
	}
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
	return r.callCounts[workstation]
}
