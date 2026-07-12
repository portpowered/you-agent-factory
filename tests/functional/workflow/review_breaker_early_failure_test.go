package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

const (
	executorLoopBreakerMaxVisits = 50
	reviewLoopBreakerMaxVisits   = 10
	reviewContinueReworkCycles   = 5
)

func TestGuardedLoopBreaker_DoesNotRouteBelowThresholdAfterReviewContinue(t *testing.T) {
	dir := copyProcessReviewLoopBreakerFactory(t, processReviewLoopBreakerFactoryOptions{
		reviewOutcomeFormat: goal.DecisionEnvelopeOutcomeFormat,
		reviewDualInput:     true,
	})

	responses := make([]interfaces.InferenceResponse, 0, reviewContinueReworkCycles*2)
	for round := 1; round <= reviewContinueReworkCycles; round++ {
		responses = append(responses,
			interfaces.InferenceResponse{Content: "<COMPLETE>\n"},
			interfaces.InferenceResponse{Content: reviewContinueEnvelope(round)},
		)
	}

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"processor": responses,
	})

	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	harness.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
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
		HasTokenInPlace("task:init")
}

func reviewContinueEnvelope(round int) string {
	return fmt.Sprintf(
		`{"decision":"CONTINUE","feedback":"round %d still needs executor work below breaker threshold"}`,
		round,
	)
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
	reviewOutcomeFormat string
	reviewDualInput     bool
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
