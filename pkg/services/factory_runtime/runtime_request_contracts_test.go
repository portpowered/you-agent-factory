package factory

import (
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestRuntimeActivationInputsCloneDetachesResultBody(t *testing.T) {
	t.Parallel()

	const declared = `{"decision":"ACCEPTED","output":"detached"}`
	body := []byte(declared)
	inputs := RuntimeActivationInputs{
		Workers: RuntimeActivationWorkerInputs{
			MockWorkers: &RuntimeActivationMockWorkersConfig{
				MockWorkers: []RuntimeActivationMockWorker{{RunType: "accept", ResultBody: body}},
			},
		},
	}
	cloned, err := inputs.Clone()
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	body[1] = 'x'
	if got := string(cloned.Workers.MockWorkers.MockWorkers[0].ResultBody); got != declared {
		t.Fatalf("cloned result body = %s, want %s", got, declared)
	}
	cloned.Workers.MockWorkers.MockWorkers[0].ResultBody[2] = 'y'
	if body[2] != declared[2] {
		t.Fatal("mutating cloned result body changed caller bytes")
	}
}

func TestWorkRestoreErrorBoundsEscapesAndPreservesCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("PRIVATE-PROMPT")
	err := &WorkRestoreError{Reason: WorkRestoreConflictingPlacement,
		WorkID:   "work\n\x1b\u202e" + strings.Repeat("x", 1000),
		PlaceIDs: []string{"task:ready", "task:done"}, Cause: cause}
	message := err.Error()
	for _, want := range []string{`\n`, `\x1b`, `\u202e`, "task:ready", "task:done", "..."} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q missing escaped context %q", message, want)
		}
	}
	if len(message) > 400 || strings.ContainsAny(message, "\n\x1b\u202e") || strings.Contains(message, "PRIVATE") {
		t.Fatalf("restore message is unbounded or unsafe: %q", message)
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause identity lost")
	}
	err.PlaceIDs = make([]string, 1000)
	for i := range err.PlaceIDs {
		err.PlaceIDs[i] = strings.Repeat("p", 1000)
	}
	if len(err.Error()) > 1500 {
		t.Fatal("place list is unbounded")
	}
	err.Reason = "PRIVATE-REASON"
	if strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("unknown reason leaked")
	}
	var absent *WorkRestoreError
	if absent.Error() != "" || absent.Unwrap() != nil {
		t.Fatal("nil restore error is not safe")
	}
}

func TestWorkRestoreErrorReportsBoundedSafeCanonicalSequence(t *testing.T) {
	t.Parallel()
	cause := errors.New("PRIVATE-PROMPT")
	restore := &WorkRestoreError{Reason: WorkRestoreInvalidHistory, WorkID: "synthetic-conflict",
		PlaceIDs: []string{"idea:to-complete"}, Cause: cause,
		Events: []WorkRestoreEvent{
			{Sequence: 41, Kind: factorydefinitions.FactoryEventTypeWorkStateChange},
			{Sequence: 42, Kind: factorydefinitions.FactoryEventTypeDispatchResponse},
		}}
	message := restore.Error()
	for _, want := range []string{"synthetic-conflict", "idea:to-complete", "event sequence: 41 WORK_STATE_CHANGE -> 42 DISPATCH_RESPONSE"} {
		if !strings.Contains(message, want) {
			t.Fatalf("diagnostic missing %q: %q", want, message)
		}
	}
	if strings.Contains(message, "PRIVATE") || !errors.Is(restore, cause) {
		t.Fatal("diagnostic exposed cause or lost cause identity")
	}
	restore.Events = make([]WorkRestoreEvent, 1000)
	for index := range restore.Events {
		restore.Events[index] = WorkRestoreEvent{Sequence: index, Kind: "PRIVATE\nKIND"}
	}
	message = restore.Error()
	if strings.Contains(message, "PRIVATE") || strings.Contains(message, "\n") || len(message) > 500 || !strings.Contains(message, "...") {
		t.Fatalf("unbounded or unsafe diagnostic: %q", message)
	}
}

func TestObservationScopeRequestPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request := ObservationScopeRequest(ObservationScopeProgress)
	if request.Scope != ObservationScopeProgress {
		t.Fatalf("scope = %q, want PROGRESS", request.Scope)
	}

	full := ObservationScopeRequest("")
	if full.Scope != "" {
		t.Fatalf("empty scope request = %#v, want zero scope for FULL default", full)
	}
}

func TestPauseControlRequestPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request := PauseControlRequest()
	if request != (PauseRequest{}) {
		t.Fatalf("pause request = %#v, want empty PauseRequest", request)
	}
}

func TestPlanDispatchRequestFromIntentPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	request, err := PlanDispatchRequestFromIntent(PlanDispatchIntent{
		DispatchID:      "dispatch-runtime-root",
		CorrelationID:   "corr-runtime-root",
		WorkIDs:         []string{"work-runtime-root"},
		WorkstationName: "review",
		WorkerType:      "inference",
		ReplayKey:       "review/trace-1/work-runtime-root",
	})
	if err != nil {
		t.Fatalf("PlanDispatchRequestFromIntent: %v", err)
	}
	if request.DispatchID != "dispatch-runtime-root" {
		t.Fatalf("dispatch id = %q, want dispatch-runtime-root", request.DispatchID)
	}
	if request.CorrelationID != "corr-runtime-root" {
		t.Fatalf("correlation id = %q, want corr-runtime-root", request.CorrelationID)
	}
	if len(request.WorkIDs) != 1 || request.WorkIDs[0] != "work-runtime-root" {
		t.Fatalf("work ids = %#v, want [work-runtime-root]", request.WorkIDs)
	}
	if request.WorkstationName != "review" {
		t.Fatalf("workstation name = %q, want review", request.WorkstationName)
	}
	if request.WorkerType != "inference" {
		t.Fatalf("worker type = %q, want inference", request.WorkerType)
	}
	if request.ReplayKey != "review/trace-1/work-runtime-root" {
		t.Fatalf("replay key = %q, want review/trace-1/work-runtime-root", request.ReplayKey)
	}
}

func TestPlanDispatchRequestFromIntentRejectsMissingFields(t *testing.T) {
	t.Parallel()

	valid := PlanDispatchIntent{
		DispatchID:      "dispatch-runtime-root",
		CorrelationID:   "corr-runtime-root",
		WorkIDs:         []string{"work-runtime-root"},
		WorkstationName: "review",
		WorkerType:      "inference",
		ReplayKey:       "review/trace-1/work-runtime-root",
	}

	cases := []struct {
		name   string
		intent PlanDispatchIntent
		want   string
	}{
		{
			name:   "missing dispatch id",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.DispatchID = "" }),
			want:   "dispatch id is required",
		},
		{
			name:   "missing correlation id",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.CorrelationID = "" }),
			want:   "correlation id is required",
		},
		{
			name:   "missing workstation name",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkstationName = "" }),
			want:   "workstation name is required",
		},
		{
			name:   "missing worker type",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkerType = "" }),
			want:   "worker type is required",
		},
		{
			name:   "missing replay key",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.ReplayKey = "" }),
			want:   "replay key is required",
		},
		{
			name:   "missing work ids",
			intent: withPlanDispatchIntent(valid, func(intent *PlanDispatchIntent) { intent.WorkIDs = nil }),
			want:   "work ids must contain at least one Work identifier",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := PlanDispatchRequestFromIntent(tc.intent)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func withPlanDispatchIntent(
	intent PlanDispatchIntent,
	mutate func(*PlanDispatchIntent),
) PlanDispatchIntent {
	clone := intent
	clone.WorkIDs = append([]string(nil), intent.WorkIDs...)
	mutate(&clone)
	return clone
}

func TestMapExecutionCatalogEntry_ReturnsDetachedWorkersPolicy(t *testing.T) {
	t.Parallel()

	worker := factorydefinitions.ResolvedWorkerDefinition{
		Name:            "worker",
		Type:            factorydefinitions.WorkerTypeModel,
		Provider:        "provider-a",
		Model:           "model-a",
		ModelProvider:   "provider-a",
		Args:            []string{"--mode=fast"},
		Body:            "worker prompt",
		SkipPermissions: true,
		AgentToolPolicy: "READ_ONLY",
	}
	workstation := factorydefinitions.ResolvedWorkstationDefinition{
		Name:                  "run",
		Type:                  factorydefinitions.WorkstationTypeModel,
		WorkerName:            "worker",
		Runner:                "codex",
		RunnerSelectionSource: "factory",
		Body:                  "workstation prompt",
		PromptTemplate:        "template",
		Environment:           map[string]string{"MODE": "fast"},
		OperationBindings: []factorydefinitions.ResolvedModelOperationBinding{{
			Slot: "prompt",
			Config: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "config",
			}},
		}},
	}

	mapper := ExecutionCatalogMapper{}
	first, err := mapper.MapExecutionCatalogEntry(worker, workstation)
	if err != nil {
		t.Fatalf("MapExecutionCatalogEntry: %v", err)
	}
	if first.RunnerID != "codex" || first.Model != "model-a" || first.Prompt != "workstation prompt" ||
		first.AgentToolPolicy != "READ_ONLY" || !first.SkipPermissions {
		t.Fatalf("mapped policy = %#v", first)
	}
	first.Args[0] = "mutated"
	first.Environment["MODE"] = "mutated"
	first.OperationBindings[0].Config[0].Text = "mutated"

	second, err := mapper.MapExecutionCatalogEntry(worker, workstation)
	if err != nil {
		t.Fatalf("MapExecutionCatalogEntry after mutation: %v", err)
	}
	if second.Args[0] != "--mode=fast" || second.Environment["MODE"] != "fast" ||
		second.OperationBindings[0].Config[0].Text != "config" {
		t.Fatalf("second mapping was affected by first-result mutation: %#v", second)
	}
}

func TestMapExecutionCatalogEntry_RejectsMismatchedDetachedPair(t *testing.T) {
	t.Parallel()

	_, err := (ExecutionCatalogMapper{}).MapExecutionCatalogEntry(
		factorydefinitions.ResolvedWorkerDefinition{Name: "worker-a"},
		factorydefinitions.ResolvedWorkstationDefinition{
			Name:       "run",
			Runner:     "codex",
			WorkerName: "worker-b",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "does not match selected worker") {
		t.Fatalf("error = %v, want detached pair mismatch", err)
	}
}
