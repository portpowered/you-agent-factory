package workflowruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
)

func TestPackagedTournamentWorkflowRunsOneOnOneJudging(t *testing.T) {
	if _, err := workflowruntime.ResolveChildWorkerSettings(
		factory.JavaScriptChildExecutionRequest{Prompt: "candidate", Label: "candidate", ModelProvider: "CODEX", Model: "gpt-5"},
		nil,
		factory.JavaScriptWorkerSettings{DefaultModelProvider: "CODEX", DefaultModel: "gpt-5"},
	); err != nil {
		t.Fatalf("resolve representative tournament child: %v", err)
	}
	executor := &packagedWorkflowChildExecutor{}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request":       "choose a strategy",
		"rounds":        1,
		"modelProvider": "CODEX",
		"model":         "gpt-5",
	}, executor)
	if !outcome.OK {
		t.Fatalf("tournament failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	if got := executor.labels(); len(got) != 3 || !strings.HasPrefix(got[2], "tournament-judge-") {
		t.Fatalf("child labels = %#v, want two competitors then one judge", got)
	}
}

func TestPackagedTournamentWorkflowRunsBoundedMultiRoundBracket(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 2,
	}, executor)
	if !outcome.OK {
		t.Fatalf("tournament failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	labels := executor.labels()
	if len(labels) != 7 {
		t.Fatalf("child labels = %#v, want four competitors and three judges", labels)
	}
	firstRound := map[string]bool{labels[4]: true, labels[5]: true}
	if !firstRound["tournament-judge-r1-m1"] || !firstRound["tournament-judge-r1-m2"] || labels[6] != "tournament-judge-r2-m1" {
		t.Fatalf("judge labels = %#v, want deterministic two-round bracket", labels[4:])
	}
	projected := projectPrimaryJSON(t, "packaged-tournament", outcome.Value)
	champion, ok := projected["champion"].(map[string]any)
	if !ok || champion["entrant"] != float64(4) {
		t.Fatalf("champion = %#v, want deterministic entrant 4 after B wins each match", projected["champion"])
	}
}

func TestPackagedTournamentWorkflowRunsMaximumBracketWithinPolicy(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 3,
	}, executor)
	if !outcome.OK {
		t.Fatalf("tournament failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	if labels := executor.labels(); len(labels) != 15 {
		t.Fatalf("maximum bracket child labels = %#v, want eight competitors and seven judges", labels)
	}
	projected := projectPrimaryJSON(t, "packaged-tournament-max", outcome.Value)
	champion, ok := projected["champion"].(map[string]any)
	if !ok || champion["entrant"] != float64(8) {
		t.Fatalf("maximum bracket champion = %#v, want entrant 8", projected["champion"])
	}
}

func TestPackagedTournamentWorkflowRejectsUnaffordableBracketBeforeFirstChild(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 6
	policy.Concurrency = 4
	outcome := runPackagedWorkflowWithPolicy(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 2,
	}, policy, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "requires 7 agent calls") {
		t.Fatalf("tournament outcome = %#v, want derived-budget failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 0 {
		t.Fatalf("child labels = %#v, want no launch before budget rejection", labels)
	}
}

func TestPackagedTournamentWorkflowRejectsInvalidJudgeSelectionWithProvenance(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{judgeText: `{"winner":"C","rationale":"invalid"}`}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 1,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "select A or B at round 1 match 1") {
		t.Fatalf("tournament outcome = %#v, want invalid-judge provenance", outcome)
	}
}

func TestPackagedTournamentWorkflowAcceptsFencedJudgeJSON(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{judgeText: "```json\n{\"winner\":\"A\",\"rationale\":\"clearer\"}\n```"}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 1,
	}, executor)
	if !outcome.OK {
		t.Fatalf("tournament failure = %#v", outcome.Failure)
	}
	projected := projectPrimaryJSON(t, "packaged-tournament-fenced", outcome.Value)
	champion := projected["champion"].(map[string]any)
	if champion["entrant"] != float64(1) {
		t.Fatalf("champion = %#v, want candidate A from fenced judgment", champion)
	}
}

func TestPackagedTournamentWorkflowStopsWhenCompetitorFails(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "tournament-competitor-2"}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 1,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "competitor 2 failed") {
		t.Fatalf("tournament outcome = %#v, want competitor failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 2 {
		t.Fatalf("child labels = %#v, want competitor fan-out and no judge", labels)
	}
}

func TestPackagedTournamentWorkflowReportsJudgeFailureProvenance(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "tournament-judge-r1-m1"}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 1,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "judge failed at round 1 match 1") {
		t.Fatalf("tournament outcome = %#v, want judge failure provenance", outcome)
	}
}

func TestPackagedSpawnWorkflowRunsExactCountAndMerge(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request":       "research travel",
		"count":         2,
		"modelProvider": "CODEX",
		"model":         "gpt-5",
	}, executor)
	if !outcome.OK {
		t.Fatalf("spawn failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	if got := executor.labels(); len(got) != 4 || got[0] != "spawn-planner" || got[3] != "spawn-merger" {
		t.Fatalf("child labels = %#v, want planner, two tasks, merger", got)
	}
}

func TestPackagedSpawnWorkflowAcceptsFencedPlannerJSON(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{plannerText: "```json\n[\"task one\",\"task two\"]\n```"}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if !outcome.OK {
		t.Fatalf("spawn failure = %#v", outcome.Failure)
	}
	if labels := executor.labels(); len(labels) != 4 || labels[3] != "spawn-merger" {
		t.Fatalf("child labels = %#v, want fenced plan to reach merger", labels)
	}
}

func TestPackagedSpawnWorkflowHonorsMaximumExactCountWithinPolicy(t *testing.T) {
	tasks := make([]string, 14)
	for index := range tasks {
		tasks[index] = fmt.Sprintf("task %d", index+1)
	}
	executor := &packagedWorkflowChildExecutor{plannerTasks: tasks}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 14,
	}, executor)
	if !outcome.OK {
		t.Fatalf("spawn failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	labels := executor.labels()
	if len(labels) != 16 || labels[0] != "spawn-planner" || labels[15] != "spawn-merger" {
		t.Fatalf("child labels = %#v, want planner, exactly fourteen tasks, and merger", labels)
	}
}

func TestPackagedSpawnWorkflowRejectsUnaffordableCountBeforePlanner(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 3
	policy.Concurrency = 2
	outcome := runPackagedWorkflowWithPolicy(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, policy, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "requires 4 agent calls") {
		t.Fatalf("spawn outcome = %#v, want derived-budget failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 0 {
		t.Fatalf("child labels = %#v, want no planner launch before budget rejection", labels)
	}
}

func TestPackagedSpawnWorkflowRejectsWrongPlanCountBeforeFanout(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{plannerTasks: []string{"only one"}}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "exactly 2 tasks") {
		t.Fatalf("spawn outcome = %#v, want exact-count plan failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 1 || labels[0] != "spawn-planner" {
		t.Fatalf("child labels = %#v, want planner only", labels)
	}
}

func TestPackagedSpawnWorkflowStopsWhenMergerFails(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "spawn-merger"}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "spawn merger failed") {
		t.Fatalf("spawn outcome = %#v, want merger failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 4 || labels[3] != "spawn-merger" {
		t.Fatalf("child labels = %#v, want planner, two tasks, and failed merger", labels)
	}
}

func TestPackagedSpawnWorkflowStopsWhenPlannerFails(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "spawn-planner"}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "spawn planner failed") {
		t.Fatalf("spawn outcome = %#v, want planner failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 1 || labels[0] != "spawn-planner" {
		t.Fatalf("child labels = %#v, want only failed planner", labels)
	}
}

func TestPackagedSpawnWorkflowRejectsDuplicatePlanBeforeFanout(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{plannerTasks: []string{"same", "same"}}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "duplicate tasks") {
		t.Fatalf("spawn outcome = %#v, want duplicate-plan failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 1 || labels[0] != "spawn-planner" {
		t.Fatalf("child labels = %#v, want planner only before atomic fanout rejection", labels)
	}
}

func TestPackagedSpawnWorkflowStopsBeforeMergeWhenChildFails(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "spawn-task-2"}
	outcome := runPackagedWorkflow(t, "spawn", "spawn.workflow.js", map[string]any{
		"request": "research travel", "count": 2,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "spawn task 2 failed") {
		t.Fatalf("spawn outcome = %#v, want child failure", outcome)
	}
	if labels := executor.labels(); len(labels) != 3 || labels[0] != "spawn-planner" || labels[2] == "spawn-merger" {
		t.Fatalf("child labels = %#v, want planner and two tasks without merger", labels)
	}
}

func runPackagedWorkflow(t *testing.T, slug, script string, args map[string]any, executor factory.JavaScriptChildExecutor) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	policy := workflowpolicy.DefaultEffectivePolicy()
	policy.MaxAgents = 16
	policy.Concurrency = 4
	return runPackagedWorkflowWithPolicy(t, slug, script, args, policy, executor)
}

func runPackagedWorkflowWithPolicy(
	t *testing.T,
	slug, script string,
	args map[string]any,
	policy workflowpolicy.EffectivePolicy,
	executor factory.JavaScriptChildExecutor,
) factory.JavaScriptRuntimeOutcome {
	t.Helper()
	source, err := fs.ReadFile(packagedfactories.Source(), "factories/"+slug+"/factory.js")
	if err != nil {
		t.Fatalf("read packaged workflow: %v", err)
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	outcome, err := runtimeWorkflows.Run(t.Context(), factory.JavaScriptRuntimeRequest{
		Source:    string(source),
		SourceRef: script,
		SessionID: "packaged-" + slug,
		Args:      encodedArgs,
		Policy:    policy,
		WorkerSettings: factory.JavaScriptWorkerSettings{
			DefaultModelProvider: "CODEX",
			DefaultModel:         "gpt-5",
		},
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return executor
		},
	})
	if err != nil {
		t.Fatalf("run packaged workflow: %v", err)
	}
	return outcome
}

type packagedWorkflowChildExecutor struct {
	mu           sync.Mutex
	calls        int
	called       []string
	plannerTasks []string
	plannerText  string
	failLabel    string
	judgeText    string
}

func (e *packagedWorkflowChildExecutor) Execute(_ context.Context, request factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	e.called = append(e.called, request.Label)
	if request.Label == e.failLabel {
		return factory.JavaScriptChildExecutionResult{
			DispatchID: fmt.Sprintf("dispatch-%d", e.calls), ChildIndex: e.calls - 1,
			Status: factory.JavaScriptChildDispatchStatusFailed, ExecutionMode: factory.JavaScriptChildExecutionModeFake,
			Diagnostic: "injected packaged workflow failure",
		}, nil
	}
	text := "result for " + request.Label
	switch request.Label {
	case "spawn-planner":
		text = e.plannerText
		if text == "" {
			tasks := e.plannerTasks
			if tasks == nil {
				tasks = []string{"task one", "task two"}
			}
			encoded, _ := json.Marshal(tasks)
			text = string(encoded)
		}
	case "spawn-merger":
		text = "merged spawn result"
	default:
		if strings.HasPrefix(request.Label, "tournament-judge-") {
			text = e.judgeText
			if text == "" {
				text = `{"winner":"B","rationale":"candidate B wins"}`
			}
		}
	}
	return factory.JavaScriptChildExecutionResult{
		DispatchID:    fmt.Sprintf("dispatch-%d", e.calls),
		ChildIndex:    e.calls - 1,
		Status:        factory.JavaScriptChildDispatchStatusCompleted,
		ExecutionMode: factory.JavaScriptChildExecutionModeFake,
		Output:        map[string]any{"text": text},
	}, nil
}

func (e *packagedWorkflowChildExecutor) labels() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.called...)
}
