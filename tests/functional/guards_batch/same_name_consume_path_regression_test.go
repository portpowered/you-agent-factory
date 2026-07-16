package guards_batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSameNameConsumePathRegression_ScaffoldBuiltInOrderMapsSameNameGuard(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	factoryCfg := loaded.FactoryConfig()

	var consumeWS *interfaces.FactoryWorkstationConfig
	for i := range factoryCfg.Workstations {
		if factoryCfg.Workstations[i].Name == "consume" {
			consumeWS = &factoryCfg.Workstations[i]
			break
		}
	}
	if consumeWS == nil {
		t.Fatal("missing consume workstation")
	}
	if len(consumeWS.Inputs) != 2 {
		t.Fatalf("consume inputs = %d, want 2", len(consumeWS.Inputs))
	}
	if consumeWS.Inputs[0].Guard == nil || consumeWS.Inputs[0].Guard.Type != interfaces.GuardTypeSameName {
		t.Fatalf("first consume input guard = %#v, want same_name on idea", consumeWS.Inputs[0].Guard)
	}

	mapper := config.ConfigMapper{}
	net, err := mapper.Map(context.Background(), factoryCfg)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	transition := net.Transitions["consume"]
	if transition == nil {
		t.Fatal("missing consume transition")
	}
	var ideaArc *petri.Arc
	var taskArc *petri.Arc
	for i := range transition.InputArcs {
		switch transition.InputArcs[i].PlaceID {
		case "idea:to-complete":
			ideaArc = &transition.InputArcs[i]
		case "task:to-complete":
			taskArc = &transition.InputArcs[i]
		}
	}
	if ideaArc == nil || taskArc == nil {
		t.Fatalf("consume arcs = %#v", transition.InputArcs)
	}
	guard, ok := ideaArc.Guard.(*petri.SameNameGuard)
	if !ok {
		t.Fatalf("idea arc guard = %T, want *petri.SameNameGuard", ideaArc.Guard)
	}
	if guard.MatchBinding != taskArc.Name {
		t.Fatalf("guard binding = %q, want %q", guard.MatchBinding, taskArc.Name)
	}
}

// scaffoldConsumePathFactoryBuiltInOrder mirrors the checked-in factory consume
// workstation input order: guarded idea:to-complete first, unguarded task second.
func scaffoldConsumePathFactoryBuiltInOrder(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "same_name_consume_path_builtin_order",
		"workTypes": []map[string]any{
			{
				"name": "idea",
				"states": []map[string]any{
					{"name": "to-complete", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]any{
					{"name": "to-complete", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workstations": []map[string]any{
			{
				"name":   "consume",
				"type":   "LOGICAL_MOVE",
				"worker": "",
				"inputs": []map[string]any{
					{
						"workType": "idea",
						"state":    "to-complete",
						"guards": []map[string]any{
							{"type": "SAME_NAME", "matchInput": "task"},
						},
					},
					{
						"workType": "task",
						"state":    "to-complete",
					},
				},
				"outputs": []map[string]any{
					{"workType": "idea", "state": "complete"},
					{"workType": "task", "state": "complete"},
				},
			},
		},
	})
	support.WriteWorkstationConfig(t, dir, "consume", "---\ntype: LOGICAL_MOVE\n---\nConsume reviewed same-name pairs.\n")
	return dir
}

func TestSameNameConsumePathRegression_ReviewedPairCompletesWithoutStrandedTask(t *testing.T) {
	cellNames := []string{
		"dynamic-workflows-cell-cli-validate-list",
		"dynamic-workflows-cell-cli-run-status-result",
		"dynamic-workflows-cell-mcp-tools",
	}

	for _, cellName := range cellNames {
		t.Run(cellName, func(t *testing.T) {
			dir := scaffoldConsumePathFactoryBuiltInOrder(t)
			h := support.NewGuardsBatchHarness(t, dir)

			submitConsumePathPair(t, h, cellName)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			errCh := support.RunGuardsBatchHarness(t, h, ctx)

			support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
			support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)

			h.Assert().
				HasNoTokenInPlace("idea:to-complete").
				HasNoTokenInPlace("task:to-complete")

			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("factory run error: %v", err)
			}
		})
	}
}

func TestSameNameConsumePathRegression_MismatchedNamesStayAtToComplete(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := support.NewGuardsBatchHarness(t, dir)

	for _, req := range []work.SubmitRequest{
		{Name: "idea-alpha", WorkTypeID: "idea", TargetState: "to-complete", TraceID: "trace-idea-alpha"},
		{Name: "task-beta", WorkTypeID: "task", TargetState: "to-complete", TraceID: "trace-task-beta"},
	} {
		h.SubmitFull(context.Background(), []work.SubmitRequest{req})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:to-complete", 1, 3*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:to-complete", 1, 3*time.Second)
	time.Sleep(150 * time.Millisecond)

	h.Assert().
		PlaceTokenCount("idea:to-complete", 1).
		PlaceTokenCount("task:to-complete", 1).
		HasNoTokenInPlace("idea:complete").
		HasNoTokenInPlace("task:complete")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathRegression_ConcurrentPairsCompleteIndependently(t *testing.T) {
	dir := scaffoldConsumePathFactoryBuiltInOrder(t)
	h := support.NewGuardsBatchHarness(t, dir)

	pairs := [][2]string{
		{"dynamic-workflows-cell-cli-validate-list", "dynamic-workflows-cell-cli-run-status-result"},
		{"dynamic-workflows-cell-mcp-tools", "unrelated-cell-name"},
	}
	for _, pair := range pairs {
		submitConsumePathPair(t, h, pair[0])
		submitConsumePathPair(t, h, pair[1])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 4, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 4, time.Second)

	h.Assert().
		HasNoTokenInPlace("idea:to-complete").
		HasNoTokenInPlace("task:to-complete")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestSameNameConsumePathRegression_StaggeredArrivalCompletesWithoutStranding(t *testing.T) {
	cases := []struct {
		name  string
		order []work.SubmitRequest
	}{
		{
			name: "task_before_idea",
			order: []work.SubmitRequest{
				{Name: "dynamic-workflows-cell-cli-validate-list", WorkTypeID: "task", TargetState: "to-complete", TraceID: "trace-task-first"},
				{Name: "dynamic-workflows-cell-cli-validate-list", WorkTypeID: "idea", TargetState: "to-complete", TraceID: "trace-idea-second"},
			},
		},
		{
			name: "idea_before_task",
			order: []work.SubmitRequest{
				{Name: "dynamic-workflows-cell-cli-run-status-result", WorkTypeID: "idea", TargetState: "to-complete", TraceID: "trace-idea-first"},
				{Name: "dynamic-workflows-cell-cli-run-status-result", WorkTypeID: "task", TargetState: "to-complete", TraceID: "trace-task-second"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := scaffoldConsumePathFactoryBuiltInOrder(t)
			h := support.NewGuardsBatchHarness(t, dir)

			for _, req := range tc.order {
				h.SubmitFull(context.Background(), []work.SubmitRequest{req})
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			errCh := h.RunInBackground(ctx)

			support.WaitForHarnessPlaceTokenCount(t, h, "idea:complete", 1, 3*time.Second)
			support.WaitForHarnessPlaceTokenCount(t, h, "task:complete", 1, 3*time.Second)

			h.Assert().
				HasNoTokenInPlace("idea:to-complete").
				HasNoTokenInPlace("task:to-complete")

			cancel()
			if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("factory run error: %v", err)
			}
		})
	}
}
