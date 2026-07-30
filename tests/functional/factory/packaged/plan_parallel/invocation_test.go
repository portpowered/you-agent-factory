package plan_parallel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parallelDAG = `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"task-a","workTypeName":"planned-task"},{"name":"task-b","workTypeName":"planned-task"},{"name":"task-c","workTypeName":"planned-task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"task-c","targetWorkName":"task-a"},{"type":"DEPENDS_ON","sourceWorkName":"task-c","targetWorkName":"task-b"}]}}`

func TestPackagedPlanParallelExecutesReadyDAGConcurrentlyAndMerges(t *testing.T) {
	runner := newPlanParallelRunner(parallelDAG)
	server := startPlanParallelServer(t, runner)
	response := invokePlanParallel(t, server, map[string]any{"request": "implement three dependent tasks"})

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := planParallelPrimaryText(t, response); !strings.Contains(got, "merged parallel result") {
		t.Fatalf("primary result = %q, want merger output", got)
	}
	if runner.maxConcurrentExecutors() < 2 {
		t.Fatalf("maximum concurrent task executors = %d, want at least 2 dependency-ready tasks", runner.maxConcurrentExecutors())
	}
	if runner.executionCount() != 3 || runner.mergeCount() != 1 {
		t.Fatalf("executor calls = %d, merge calls = %d; want 3 and 1", runner.executionCount(), runner.mergeCount())
	}
	for index, request := range runner.requestsSnapshot() {
		if request.Command != "codex" || !planParallelHasArgPair(request.Args, "--model", "operator-model") ||
			!planParallelHasArg(request.Args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("provider request[%d] = command %q args %#v, want operator model and packaged skip-permissions default", index, request.Command, request.Args)
		}
	}

	events := server.GetFactoryEvents(t)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 5 || dispatches[0].Request.TransitionId != "plan-parallel-work" ||
		dispatches[len(dispatches)-1].Request.TransitionId != "merge-plan-results" {
		t.Fatalf("dispatches = %#v, want planner, three tasks, and terminal merger", dispatches)
	}
	assertPlanParallelGeneratedDAGEvent(t, events)
	assertPlanParallelRetainedReplay(t, server, events)
}

func assertPlanParallelGeneratedDAGEvent(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil || len(*payload.Works) != 3 || payload.Relations == nil {
			continue
		}
		names := make([]string, 0, len(*payload.Works))
		for _, item := range *payload.Works {
			names = append(names, item.Name)
		}
		if strings.Join(names, ",") != "task-a,task-b,task-c" || len(*payload.Relations) != 5 {
			t.Fatalf("replayed generated DAG = works %v relations %#v", names, *payload.Relations)
		}
		dependencies := 0
		parentChildren := 0
		for _, relation := range *payload.Relations {
			switch relation.Type {
			case factoryapi.RelationTypeDependsOn:
				if relation.SourceWorkName != "task-c" ||
					(relation.TargetWorkName != "task-a" && relation.TargetWorkName != "task-b") {
					t.Fatalf("replayed generated dependency = %#v", relation)
				}
				dependencies++
			case factoryapi.RelationTypeParentChild:
				parentChildren++
			}
		}
		if dependencies != 2 || parentChildren != 3 {
			t.Fatalf("replayed generated relation counts = dependencies %d parent-child %d", dependencies, parentChildren)
		}
		return
	}
	t.Fatal("retained event history did not reconstruct the generated Work DAG")
}

func assertPlanParallelRetainedReplay(
	t *testing.T,
	server *support.FunctionalAPIServer,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("events = %d, want retained history", len(events))
	}
	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := server.GetFactoryEventsAfter(t, support.FactoryEventReadCursor{
		AfterEventID:  events[0].Id,
		AfterSequence: &sequence,
	})
	if len(replayed) != len(events)-1 {
		t.Fatalf("retained replay events = %d, want %d", len(replayed), len(events)-1)
	}
	for index := range replayed {
		if replayed[index].Id != events[index+1].Id {
			t.Fatalf("retained replay event %d = %q, want %q", index, replayed[index].Id, events[index+1].Id)
		}
	}
}

func TestPackagedPlanParallelRejectsPlannerBatchAboveCeilingAtomically(t *testing.T) {
	works := make([]string, 13)
	for index := range works {
		works[index] = fmt.Sprintf(`{"name":"task-%02d","workTypeName":"planned-task"}`, index+1)
	}
	runner := newPlanParallelRunner(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[` + strings.Join(works, ",") + `]}}`)
	server := startPlanParallelServer(t, runner)
	response := invokePlanParallel(t, server, map[string]any{"request": "produce too many tasks"})

	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	if runner.executionCount() != 0 || runner.mergeCount() != 0 {
		t.Fatalf("executor calls = %d, merge calls = %d; want atomic rejection", runner.executionCount(), runner.mergeCount())
	}
}

func TestPackagedPlanParallelChildFailureFansInWithoutMerge(t *testing.T) {
	runner := newPlanParallelRunner(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"failing-task","workTypeName":"planned-task"}]}}`)
	runner.failExecutors = true
	server := startPlanParallelServer(t, runner)
	response := invokePlanParallel(t, server, map[string]any{"request": "surface a child failure"})

	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
		t.Fatalf("response = %#v, want failed invocation without primary result", response)
	}
	if runner.executionCount() != 3 || runner.mergeCount() != 0 {
		t.Fatalf("executor calls = %d, merge calls = %d; want bounded provider retries and no merge", runner.executionCount(), runner.mergeCount())
	}
}

type planParallelRunner struct {
	mu             sync.Mutex
	plannerOutput  string
	active         int
	maxActive      int
	executions     int
	merges         int
	failExecutors  bool
	readyExecutors chan struct{}
	requests       []platformprocess.CommandRequest
}

func newPlanParallelRunner(plannerOutput string) *planParallelRunner {
	return &planParallelRunner{plannerOutput: plannerOutput, readyExecutors: make(chan struct{})}
}

func (runner *planParallelRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()
	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "Plan an executable Work DAG"):
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(runner.plannerOutput)}, nil
	case strings.Contains(prompt, "Treat the original request and all completed generated Work inputs"):
		runner.mu.Lock()
		runner.merges++
		runner.mu.Unlock()
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("merged parallel result")}, nil
	default:
		runner.mu.Lock()
		runner.executions++
		if runner.failExecutors {
			runner.mu.Unlock()
			return platformprocess.CommandResult{}, errors.New("planned task provider failure")
		}
		runner.active++
		if runner.active > runner.maxActive {
			runner.maxActive = runner.active
		}
		if runner.active >= 2 {
			select {
			case <-runner.readyExecutors:
			default:
				close(runner.readyExecutors)
			}
		}
		runner.mu.Unlock()

		select {
		case <-runner.readyExecutors:
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("planned task completed")}, nil
	}
}

func (runner *planParallelRunner) requestsSnapshot() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), runner.requests...)
}

func planParallelHasArgPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func planParallelHasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func (runner *planParallelRunner) maxConcurrentExecutors() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.maxActive
}

func (runner *planParallelRunner) executionCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.executions
}

func (runner *planParallelRunner) mergeCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.merges
}

func startPlanParallelServer(t *testing.T, runner platformprocess.CommandRunner) *support.FunctionalAPIServer {
	t.Helper()
	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedPlanParallelFactoryName)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Args:  []string{"--provider", "CODEX", "--model", "operator-model"},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
}

func invokePlanParallel(t *testing.T, server *support.FunctionalAPIServer, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	requestID := fmt.Sprintf("plan-parallel-%d", time.Now().UnixNano())
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST invocation status = %d", response.StatusCode)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation: %v", err)
	}
	return decoded
}

func planParallelPrimaryText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary text: %v", err)
	}
	return part.Text
}
