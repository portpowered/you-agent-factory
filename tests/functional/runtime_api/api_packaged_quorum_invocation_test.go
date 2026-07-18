package runtime_api

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/quorum"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_PackagedQuorumGatesMergeUntilBothBranchesComplete(t *testing.T) {
	runner := newQuorumGatedCommandRunner()
	host, stream := startPackagedQuorumInvocationHost(t, runner)
	args := map[string]interface{}{"input": "quorum request"}
	request := factoryapi.InvocationRequest{Args: &args}

	responseCh := make(chan factoryapi.InvocationResponse, 1)
	go func() {
		responseCh <- postInvocation(t, host.Endpoint(), request)
	}()

	runner.waitForBranchStarts(t)
	assertNoCompletedQuorumMerge(t, host.Endpoint())
	close(runner.releaseBranchB)

	select {
	case response := <-responseCh:
		assertMergedQuorumResult(t, primaryResultText(t, response), "quorum request")
		assertPackagedQuorumDispatchOrder(t, stream, response.TraceId)
		assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for gated quorum invocation to complete")
	}
}

func TestSessionInvocationAPI_PackagedQuorumAppliesRoleArguments(t *testing.T) {
	runner := newQuorumGatedCommandRunner()
	close(runner.releaseBranchB)
	host, stream := startPackagedQuorumInvocationHost(t, runner)
	args := map[string]interface{}{
		"input":          "configured quorum request",
		"branchProvider": "CODEX",
		"branchModel":    "gpt-5.1",
		"mergeProvider":  "CODEX",
		"mergeModel":     "gpt-5.2",
	}
	request := factoryapi.InvocationRequest{Args: &args}

	response := postInvocation(t, host.Endpoint(), request)
	assertMergedQuorumResult(t, primaryResultText(t, response), "configured quorum request")
	assertPackagedQuorumModels(t, stream, response.TraceId, map[string]string{
		"quorum-branch-a": "gpt-5.1",
		"quorum-branch-b": "gpt-5.1",
		"quorum-merge":    "gpt-5.2",
	})
	assertInvocationTraceWorkState(t, host.Endpoint(), response.TraceId, "complete", factoryapi.WorkStateTypeTERMINAL)
}

func startPackagedQuorumInvocationHost(t *testing.T, runner workers.CommandRunner) (*support.RootRunFunctionalHost, *factoryEventHTTPStream) {
	t.Helper()
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), quorum.PackagedFactoryName, quorum.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/quorum): %v", err)
	}
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot:        dir,
		SystemRoot:         t.TempDir(),
		DisableMockWorkers: true,
		FunctionalEdges: wire.FunctionalEdges{
			ProviderCommandRunner: runner,
		},
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, shutdownErr := host.Shutdown(shutdownCtx); shutdownErr != nil {
			t.Errorf("Shutdown() error = %v", shutdownErr)
		}
	})

	stream := openRootRunFactoryEventHTTPStream(t, host)
	requireFunctionalEventStreamPrelude(t, stream)
	return host, stream
}

type quorumGatedCommandRunner struct {
	startedA       chan struct{}
	startedB       chan struct{}
	startedAOnce   sync.Once
	startedBOnce   sync.Once
	releaseBranchB chan struct{}
}

func newQuorumGatedCommandRunner() *quorumGatedCommandRunner {
	return &quorumGatedCommandRunner{
		startedA:       make(chan struct{}),
		startedB:       make(chan struct{}),
		releaseBranchB: make(chan struct{}),
	}
}

func (r *quorumGatedCommandRunner) Run(ctx context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
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
		return quorumCommandResult(request, "merged quorum response:\n"+prompt+"\nCOMPLETE"), nil
	default:
		return workers.CommandResult{}, nil
	}
}

func quorumCommandResult(_ workers.CommandRequest, result string) workers.CommandResult {
	return workers.CommandResult{Stdout: []byte(result)}
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

func assertNoCompletedQuorumMerge(t *testing.T, endpoint string) {
	t.Helper()
	works := getGeneratedJSON[factoryapi.ListWorkResponse](t, support.DefaultSessionWorkURL(endpoint, "/work"))
	for _, candidate := range works.Results {
		if stringPointerValue(candidate.WorkTypeName) == "quorum-merge" && generatedWorkStateType(candidate.State) == factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("GET /work exposed completed quorum merge before both branches completed: %#v", candidate)
		}
	}
}

func assertPackagedQuorumDispatchOrder(t *testing.T, stream *factoryEventHTTPStream, traceID string) {
	t.Helper()
	completedBranches := make(map[string]bool, 2)
	for {
		event := stream.next(10 * time.Second)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || !packagedGoalEventHasTrace(event, traceID) {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode packaged quorum DISPATCH_RESPONSE: %v", err)
		}
		switch payload.TransitionId {
		case "run-quorum-branch-a", "run-quorum-branch-b":
			if payload.Outcome != factoryapi.WorkOutcomeAccepted {
				t.Fatalf("packaged quorum branch %q outcome = %q, want ACCEPTED", payload.TransitionId, payload.Outcome)
			}
			completedBranches[payload.TransitionId] = true
		case quorum.PackagedMergeWorkstationName:
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || len(completedBranches) != 2 {
				t.Fatalf("packaged quorum merge outcome = %q after branches %v, want ACCEPTED after both branches", payload.Outcome, completedBranches)
			}
			return
		}
	}
}

func assertPackagedQuorumModels(t *testing.T, stream *factoryEventHTTPStream, traceID string, want map[string]string) {
	t.Helper()
	remaining := make(map[string]string, len(want))
	for worker, model := range want {
		remaining[worker] = model
	}
	for len(remaining) > 0 {
		event := stream.next(10 * time.Second)
		if event.Type != factoryapi.FactoryEventTypeModelRequest || !packagedGoalEventHasTrace(event, traceID) {
			continue
		}
		payload, err := event.Payload.AsModelRequestEventPayload()
		if err != nil {
			t.Fatalf("decode packaged quorum MODEL_REQUEST: %v", err)
		}
		model, ok := remaining[payload.Worker]
		if !ok {
			continue
		}
		if payload.Model != model {
			t.Fatalf("packaged quorum worker %q model = %q, want %q", payload.Worker, payload.Model, model)
		}
		delete(remaining, payload.Worker)
	}
}
