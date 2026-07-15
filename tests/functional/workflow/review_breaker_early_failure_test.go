package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	executorLoopBreakerMaxVisits = 50
	reviewLoopBreakerMaxVisits   = 10
	reviewContinueReworkCycles   = 5

	testReviewLoopBreakerMaxVisits   = 3
	testExecutorLoopBreakerMaxVisits = 5
)

func TestGuardedLoopBreaker_DoesNotRouteBelowThresholdAfterReviewContinue(t *testing.T) {
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		reviewOutcomeFormat: goal.DecisionEnvelopeOutcomeFormat,
		reviewDualInput:     true,
	})

	responses := make([]workerexecution.InferenceResponse, 0, (reviewContinueReworkCycles+1)*2)
	for round := 1; round <= reviewContinueReworkCycles; round++ {
		responses = append(responses,
			workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
			workerexecution.InferenceResponse{Content: reviewContinueEnvelope(round)},
		)
	}
	// Let the harness settle after below-threshold rework instead of looping on the
	// mock provider default response until visit thresholds are hit.
	responses = append(responses,
		workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
		workerexecution.InferenceResponse{Content: `{"decision":"ACCEPTED","feedback":"settle below breaker threshold"}`},
	)

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": responses,
	})

	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.SubmitFull(context.Background(), []work.SubmitRequest{{
		WorkTypeID: "task",
		WorkID:     "work-task-below-threshold",
		TraceID:    "trace-review-breaker-below-threshold",
		Name:       "review-breaker-below-threshold",
		Payload:    []byte("prove guarded loop breaker honors visit thresholds"),
	}})

	if err := harness.RunUntilCompleteError(10 * time.Second); err != nil {
		t.Logf("factory run ended before terminal settle: %v", err)
	}

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	executorBreakerDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "executor-loop-breaker")
	reviewBreakerDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review-loop-breaker")

	if len(processDispatches) >= executorLoopBreakerMaxVisits {
		t.Fatalf(
			"test setup error: process dispatches = %d, want fewer than executor maxVisits %d",
			len(processDispatches),
			executorLoopBreakerMaxVisits,
		)
	}
	if len(reviewDispatches) >= reviewLoopBreakerMaxVisits {
		t.Fatalf(
			"test setup error: review dispatches = %d, want fewer than review maxVisits %d",
			len(reviewDispatches),
			reviewLoopBreakerMaxVisits,
		)
	}

	if len(executorBreakerDispatches) > 0 {
		t.Fatalf(
			"executor-loop-breaker dispatched after only %d process visits (maxVisits=%d, token process visits=%d): %#v",
			len(processDispatches),
			executorLoopBreakerMaxVisits,
			consumedTokenVisitCount(executorBreakerDispatches[0], "process"),
			executorBreakerDispatches,
		)
	}
	if len(reviewBreakerDispatches) > 0 {
		t.Fatalf(
			"review-loop-breaker dispatched after only %d review visits (maxVisits=%d, token review visits=%d): %#v",
			len(reviewDispatches),
			reviewLoopBreakerMaxVisits,
			consumedTokenVisitCount(reviewBreakerDispatches[0], "review"),
			reviewBreakerDispatches,
		)
	}

	harness.Assert().
		HasNoTokenInPlace("task:failed").
		HasTokenInPlace("task:complete")
}

func reviewContinueEnvelope(round int) string {
	return fmt.Sprintf(
		`{"decision":"CONTINUE","feedback":"round %d still needs executor work below breaker threshold"}`,
		round,
	)
}

func TestGuardedLoopBreaker_RoutesToFailedWhenReviewVisitsReachThreshold(t *testing.T) {
	threshold := testReviewLoopBreakerMaxVisits
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		reviewLoopBreakerMaxVisits: threshold,
	})

	responses := make([]workerexecution.InferenceResponse, 0, threshold*2+1)
	for round := 1; round <= threshold; round++ {
		responses = append(responses,
			workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
			workerexecution.InferenceResponse{Content: fmt.Sprintf("review rejection %d", round)},
		)
	}
	responses = append(responses, workerexecution.InferenceResponse{Content: "<COMPLETE>\n"})

	harness := runGuardedLoopBreakerHarness(t, dir, responses, "work-task-review-at-threshold")
	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	if len(reviewDispatches) != threshold {
		t.Fatalf("review dispatch count = %d, want %d at threshold", len(reviewDispatches), threshold)
	}
	for i, dispatch := range reviewDispatches {
		if dispatch.Outcome != workerexecution.OutcomeRejected {
			t.Fatalf("review dispatch %d outcome = %s, want %s", i, dispatch.Outcome, workerexecution.OutcomeRejected)
		}
	}

	reviewBreakerDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review-loop-breaker")
	if len(reviewBreakerDispatches) != 1 {
		t.Fatalf("review-loop-breaker dispatch count = %d, want 1", len(reviewBreakerDispatches))
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "review-loop-breaker", "task:failed")
}

func TestGuardedLoopBreaker_SurvivesReviewOneVisitBelowThreshold(t *testing.T) {
	threshold := testReviewLoopBreakerMaxVisits
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		reviewLoopBreakerMaxVisits: threshold,
	})

	responses := make([]workerexecution.InferenceResponse, 0, (threshold-1)*2+2)
	for round := 1; round < threshold; round++ {
		responses = append(responses,
			workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
			workerexecution.InferenceResponse{Content: fmt.Sprintf("review rejection %d", round)},
		)
	}
	responses = append(responses,
		workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
		workerexecution.InferenceResponse{Content: "<COMPLETE>\n"},
	)

	harness := runGuardedLoopBreakerHarness(t, dir, responses, "work-task-review-below-threshold")
	if err := harness.RunUntilCompleteError(10 * time.Second); err != nil {
		t.Logf("factory run ended before terminal settle: %v", err)
	}

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	reviewDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "review")
	rejectedReviewDispatches := 0
	for _, dispatch := range reviewDispatches {
		if dispatch.Outcome == workerexecution.OutcomeRejected {
			rejectedReviewDispatches++
		}
	}
	if rejectedReviewDispatches != threshold-1 {
		t.Fatalf("rejected review dispatch count = %d, want %d one below threshold", rejectedReviewDispatches, threshold-1)
	}
	if len(dispatchesForWorkstation(snapshot.DispatchHistory, "review-loop-breaker")) > 0 {
		t.Fatalf("review-loop-breaker dispatched with only %d rejected review visits (maxVisits=%d)", rejectedReviewDispatches, threshold)
	}

	harness.Assert().
		HasNoTokenInPlace("task:failed").
		HasTokenInPlace("task:complete")
}

func TestGuardedLoopBreaker_RoutesToFailedWhenExecutorVisitsReachThreshold(t *testing.T) {
	threshold := testExecutorLoopBreakerMaxVisits
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		executorLoopBreakerMaxVisits: threshold,
	})

	responses := make([]workerexecution.InferenceResponse, 0, threshold)
	for round := 1; round <= threshold; round++ {
		responses = append(responses, workerexecution.InferenceResponse{
			Content: fmt.Sprintf("<CONTINUE>\nround %d still looping at process", round),
		})
	}

	harness := runGuardedLoopBreakerHarness(t, dir, responses, "work-task-executor-at-threshold")
	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		HasTokenInPlace("task:failed").
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:in-review").
		HasNoTokenInPlace("task:complete")

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != threshold {
		t.Fatalf("process dispatch count = %d, want %d at threshold", len(processDispatches), threshold)
	}
	for i, dispatch := range processDispatches {
		if dispatch.Outcome != workerexecution.OutcomeContinue {
			t.Fatalf("process dispatch %d outcome = %s, want %s", i, dispatch.Outcome, workerexecution.OutcomeContinue)
		}
	}

	executorBreakerDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "executor-loop-breaker")
	if len(executorBreakerDispatches) != 1 {
		t.Fatalf("executor-loop-breaker dispatch count = %d, want 1", len(executorBreakerDispatches))
	}
	assertDispatchHistoryContainsWorkstationRoute(t, snapshot.DispatchHistory, "executor-loop-breaker", "task:failed")
}

func TestGuardedLoopBreaker_SurvivesExecutorOneVisitBelowThreshold(t *testing.T) {
	threshold := testExecutorLoopBreakerMaxVisits
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		executorLoopBreakerMaxVisits: threshold,
	})

	responses := make([]workerexecution.InferenceResponse, 0, threshold-1)
	for round := 1; round < threshold; round++ {
		responses = append(responses, workerexecution.InferenceResponse{
			Content: fmt.Sprintf("<CONTINUE>\nround %d still looping at process", round),
		})
	}

	provider := newStallAfterResponsesProvider(responses)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)
	harness.SubmitFull(context.Background(), []work.SubmitRequest{{
		WorkTypeID: "task",
		WorkID:     "work-task-executor-below-threshold",
		TraceID:    "trace-work-task-executor-below-threshold",
		Name:       "work-task-executor-below-threshold",
		Payload:    []byte("guarded loop breaker threshold coverage"),
	}})

	runHarnessUntilWatchedDispatchCount(t, harness, "process", threshold-1, 5*time.Second)

	snapshot, err := harness.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	processDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "process")
	if len(processDispatches) != threshold-1 {
		t.Fatalf("process dispatch count = %d, want %d one below threshold", len(processDispatches), threshold-1)
	}
	if len(dispatchesForWorkstation(snapshot.DispatchHistory, "executor-loop-breaker")) > 0 {
		t.Fatalf("executor-loop-breaker dispatched with only %d process visits (maxVisits=%d)", len(processDispatches), threshold)
	}

	harness.Assert().HasNoTokenInPlace("task:failed")
}

func runGuardedLoopBreakerHarness(
	t *testing.T,
	dir string,
	responses []workerexecution.InferenceResponse,
	workID string,
) *testutil.ServiceTestHarness {
	t.Helper()

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"processor": responses,
	})

	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.SubmitFull(context.Background(), []work.SubmitRequest{{
		WorkTypeID: "task",
		WorkID:     workID,
		TraceID:    "trace-" + workID,
		Name:       workID,
		Payload:    []byte("guarded loop breaker threshold coverage"),
	}})

	return harness
}

type stallAfterResponsesProvider struct {
	mu        sync.Mutex
	responses []workerexecution.InferenceResponse
	index     int
	stall     chan struct{}
}

func newStallAfterResponsesProvider(responses []workerexecution.InferenceResponse) *stallAfterResponsesProvider {
	return &stallAfterResponsesProvider{
		responses: responses,
		stall:     make(chan struct{}),
	}
}

func (p *stallAfterResponsesProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.index < len(p.responses) {
		resp := p.responses[p.index]
		p.index++
		return resp, nil
	}

	<-p.stall
	return workerexecution.InferenceResponse{}, errors.New("stallAfterResponsesProvider: unreachable")
}

var _ workers.Provider = (*stallAfterResponsesProvider)(nil)

func loopBreakerForWatchedWorkstation(workstation string) string {
	switch workstation {
	case "process":
		return "executor-loop-breaker"
	case "review":
		return "review-loop-breaker"
	default:
		return ""
	}
}

func runHarnessUntilWatchedDispatchCount(
	t *testing.T,
	harness *testutil.ServiceTestHarness,
	workstation string,
	want int,
	timeout time.Duration,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errCh := harness.RunInBackground(ctx)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("factory run error while waiting for %d %q dispatches: %v", want, workstation, err)
			}
			t.Fatalf("factory completed before %d %q dispatches", want, workstation)
		default:
		}

		snapshot, err := harness.GetEngineStateSnapshot()
		if err != nil {
			continue
		}
		dispatches := dispatchesForWorkstation(snapshot.DispatchHistory, workstation)
		if len(dispatches) != want {
			continue
		}
		if breakerWorkstation := loopBreakerForWatchedWorkstation(workstation); breakerWorkstation != "" {
			if len(dispatchesForWorkstation(snapshot.DispatchHistory, breakerWorkstation)) > 0 {
				continue
			}
		}
		cancel()
		<-errCh
		return
	}

	t.Fatalf("timed out waiting for %d %q dispatches", want, workstation)
}

func consumedTokenVisitCount(dispatch interfaces.CompletedDispatch, workstation string) int {
	for _, token := range dispatch.ConsumedTokens {
		if token.History.TotalVisits == nil {
			continue
		}
		if visits, ok := token.History.TotalVisits[workstation]; ok {
			return visits
		}
	}
	return 0
}

type processReviewLoopBreakerFactoryOptions struct {
	reviewOutcomeFormat          string
	reviewDualInput              bool
	reviewLoopBreakerMaxVisits   int
	executorLoopBreakerMaxVisits int
}

func copyProcessReviewLoopBreakerFactory(t *testing.T, opts processReviewLoopBreakerFactoryOptions) string {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, testutil.MustRepoPath(t, "tests/adhoc/factory"))
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse factory.json: %v", err)
	}

	if opts.reviewDualInput {
		cfg["workTypes"] = append(workTypesFromConfig(cfg), map[string]any{
			"name": "review",
			"states": []map[string]any{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "fin", "type": "FAILED"},
			},
		})

		workstations := workstationsFromConfig(cfg)
		for i, ws := range workstations {
			name, _ := ws["name"].(string)
			switch name {
			case "process":
				ws["outputs"] = []map[string]any{
					{"workType": "task", "state": "in-review"},
					{"workType": "review", "state": "init"},
				}
				workstations[i] = ws
			case "review":
				if opts.reviewOutcomeFormat != "" {
					ws["outcomeFormat"] = opts.reviewOutcomeFormat
				}
				ws["inputs"] = []map[string]any{
					{"workType": "task", "state": "in-review"},
					{"workType": "review", "state": "init"},
				}
				ws["outputs"] = []map[string]any{
					{"workType": "task", "state": "complete"},
					{"workType": "review", "state": "complete"},
				}
				ws["onFailure"] = []map[string]any{
					{"workType": "task", "state": "failed"},
					{"workType": "review", "state": "fin"},
				}
				ws["worker"] = "processor"
				workstations[i] = ws
			}
		}
		cfg["workstations"] = workstations
	} else if opts.reviewOutcomeFormat != "" {
		workstations := workstationsFromConfig(cfg)
		for i, ws := range workstations {
			if ws["name"] == "review" {
				ws["outcomeFormat"] = opts.reviewOutcomeFormat
				workstations[i] = ws
				break
			}
		}
		cfg["workstations"] = workstations
	}

	if opts.reviewLoopBreakerMaxVisits > 0 || opts.executorLoopBreakerMaxVisits > 0 {
		workstations := workstationsFromConfig(cfg)
		for i, ws := range workstations {
			name, _ := ws["name"].(string)
			switch name {
			case "review-loop-breaker":
				if opts.reviewLoopBreakerMaxVisits > 0 {
					patchVisitCountGuardMaxVisits(ws, opts.reviewLoopBreakerMaxVisits)
				}
			case "executor-loop-breaker":
				if opts.executorLoopBreakerMaxVisits > 0 {
					patchVisitCountGuardMaxVisits(ws, opts.executorLoopBreakerMaxVisits)
				}
			}
			workstations[i] = ws
		}
		cfg["workstations"] = workstations
	}

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, append(updated, '\n'), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	return dir
}

func workTypesFromConfig(cfg map[string]any) []map[string]any {
	raw, _ := cfg["workTypes"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		typed, ok := item.(map[string]any)
		if ok {
			out = append(out, typed)
		}
	}
	return out
}

func workstationsFromConfig(cfg map[string]any) []map[string]any {
	raw, _ := cfg["workstations"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		typed, ok := item.(map[string]any)
		if ok {
			out = append(out, typed)
		}
	}
	return out
}

func patchVisitCountGuardMaxVisits(workstation map[string]any, maxVisits int) {
	rawGuards, _ := workstation["guards"].([]any)
	for i, item := range rawGuards {
		guard, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if guard["type"] != "VISIT_COUNT" {
			continue
		}
		guard["maxVisits"] = maxVisits
		rawGuards[i] = guard
	}
	workstation["guards"] = rawGuards
}
