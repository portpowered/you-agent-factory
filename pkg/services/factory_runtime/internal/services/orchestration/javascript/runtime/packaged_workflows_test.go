package workflowruntime_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/runtime"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

func TestPackagedDeepResearchWorkflowDefaultsEveryChildToSkipPermissions(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{}
	outcome := runPackagedWorkflow(t, "deep-research", "deep-research.workflow.js", map[string]any{
		"topic": "compare the current runtime architecture and its operational tradeoffs",
	}, executor)
	if !outcome.OK {
		t.Fatalf("deep-research failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	if got := executor.labels(); len(got) != 3 || got[2] != "lead-research-synthesis" {
		t.Fatalf("child labels = %#v, want two specialists and lead synthesis", got)
	}
}

func TestPackagedDeepResearchWorkflowRetriesOneFailedSpecialistAndSynthesizes(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{failLabel: "research-specialist-technical"}
	outcome := runPackagedWorkflow(t, "deep-research", "deep-research.workflow.js", map[string]any{
		"topic": "compare the current runtime architecture and its operational tradeoffs",
	}, executor)
	if !outcome.OK {
		t.Fatalf("deep-research failure = %#v; calls = %#v", outcome.Failure, executor.labels())
	}
	labels := executor.labels()
	if len(labels) != 4 || labels[2] != "research-specialist-technical-retry" || labels[3] != "lead-research-synthesis" {
		t.Fatalf("child labels = %#v, want two specialists, bounded technical retry, and lead synthesis", labels)
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
	projected := projectPrimaryText(t, "packaged-tournament", outcome.Value)
	if !strings.Contains(projected, "result for tournament-competitor-4") ||
		strings.Count(projected, "candidate B wins") != 2 {
		t.Fatalf("champion result = %q, want entrant 4 and both winning rationales", projected)
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
	projected := projectPrimaryText(t, "packaged-tournament-max", outcome.Value)
	if !strings.Contains(projected, "result for tournament-competitor-8") ||
		strings.Count(projected, "candidate B wins") != 3 {
		t.Fatalf("maximum bracket champion = %q, want entrant 8 and three winning rationales", projected)
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
	projected := projectPrimaryText(t, "packaged-tournament-fenced", outcome.Value)
	if !strings.Contains(projected, "result for tournament-competitor-1") || !strings.Contains(projected, "clearer") {
		t.Fatalf("champion = %q, want candidate A and fenced judgment rationale", projected)
	}
}

func TestPackagedTournamentWorkflowRejectsEmptyJudgeRationaleWithProvenance(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{judgeText: `{"winner":"A","rationale":""}`}
	outcome := runPackagedWorkflow(t, "tournament", "tournament.workflow.js", map[string]any{
		"request": "choose a strategy", "rounds": 1,
	}, executor)
	if outcome.OK || !strings.Contains(outcome.Failure.Message, "provide a rationale at round 1 match 1") {
		t.Fatalf("tournament outcome = %#v, want empty-rationale provenance", outcome)
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

func TestPackagedSpawnWorkflowAcceptsStructuredPlannerObject(t *testing.T) {
	executor := &packagedWorkflowChildExecutor{plannerTasks: []string{"task one", "task two"}}
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

func TestRun_AgentRunSchemaReachesExecutorAsDetachedValidatedObject(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `const schema = { type: "object", properties: { answer: { type: "string" } }, required: ["answer"] };
const child = agent.run({ prompt: "review", schema });
schema.properties.answer.type = "number";
return child;`,
		SourceRef: "inline", SessionID: "schema-detached",
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
		return stub
	}})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() = outcome %#v, error %v", outcome, err)
	}
	requests := stub.executionRequests()
	if len(requests) != 1 {
		t.Fatalf("executor requests = %#v, want one request", requests)
	}
	want := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
		"required": []any{"answer"},
	}
	if !reflect.DeepEqual(requests[0].OutputSchema, want) {
		t.Fatalf("executor schema = %#v, want %#v", requests[0].OutputSchema, want)
	}
	if workflowruntime.SchemaDigest(requests[0].OutputSchema) == "" {
		t.Fatal("executor schema digest = empty, want deterministic digest input")
	}
}

func TestRun_ParallelSchemasAreValidatedAndDetachedPerChild(t *testing.T) {
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `const schema = { type: "object", properties: { answer: { type: "string" } } };
return parallel([
  { prompt: "first", label: "first", schema },
  { prompt: "second", label: "second", schema },
]);`,
		SourceRef: "inline", SessionID: "parallel-schema-detached",
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
		return stub
	}})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() = outcome %#v, error %v", outcome, err)
	}
	requests := stub.executionRequests()
	if len(requests) != 2 {
		t.Fatalf("executor requests = %#v, want two requests", requests)
	}
	for _, request := range requests {
		properties, ok := request.OutputSchema["properties"].(map[string]any)
		if !ok || properties["answer"].(map[string]any)["type"] != "string" {
			t.Fatalf("executor request %q schema = %#v, want validated object schema", request.Label, request.OutputSchema)
		}
	}
	requests[0].OutputSchema["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "number"
	if requests[1].OutputSchema["properties"].(map[string]any)["answer"].(map[string]any)["type"] != "string" {
		t.Fatal("parallel child schemas share mutable nested state")
	}
}

func TestRun_StructuredChildResultsStayNativeAndMetadataCollisionSafe(t *testing.T) {
	executor := &structuredChildExecutor{}
	outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
		Source: `const schema = { type: "object", properties: { answer: { type: "string" } }, required: ["answer"] };
return (async function () {
const single = await agent.run({ prompt: "single", label: "single", schema });
const many = await parallel([
  { prompt: "first", label: "first", schema },
  { prompt: "second", label: "second", schema },
]);
return { single, many };
})();`,
		SourceRef: "inline",
		SessionID: "structured-native-results",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}, factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return executor
		},
	})
	if err != nil || !outcome.OK {
		t.Fatalf("Run() = outcome %#v, error %v", outcome, err)
	}
	projected := projectPrimaryJSON(t, "structured-native-results", outcome.Value)
	assertStructuredProjectedChild(t, projected["single"], "single")
	many, ok := projected["many"].([]any)
	if !ok || len(many) != 2 {
		t.Fatalf("projected parallel children = %#v, want two children", projected["many"])
	}
	assertStructuredProjectedChild(t, many[0], "first")
	assertStructuredProjectedChild(t, many[1], "second")
	if executor.requestCount() != 3 {
		t.Fatalf("structured child requests = %d, want one direct and two parallel requests", executor.requestCount())
	}
}

func assertStructuredProjectedChild(t *testing.T, value any, wantLabel string) {
	t.Helper()
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("structured child = %#v, want object", value)
	}
	if child["label"] != wantLabel || child["schemaValidated"] != true {
		t.Fatalf("structured child metadata = %#v, want label %q and schemaValidated true", child, wantLabel)
	}
	if child["schemaDigest"] == "" {
		t.Fatalf("structured child schemaDigest = %#v, want non-empty digest", child["schemaDigest"])
	}
	output, ok := child["output"].(map[string]any)
	if !ok {
		t.Fatalf("structured child output = %#v, want native object", child["output"])
	}
	if output["answer"] != "native:"+wantLabel {
		t.Fatalf("structured child answer = %#v, want native answer", output["answer"])
	}
	if output["schemaValidated"] != "customer-output" {
		t.Fatalf("customer schemaValidated field = %#v, want preserved customer value", output["schemaValidated"])
	}
}

func TestRun_AgentRunRejectsInvalidSchemaBeforeChildDispatch(t *testing.T) {
	cases := []struct {
		name   string
		source string
	}{
		{
			name:   "non-object",
			source: `agent.run({ prompt: "prompt-secret", schema: "schema-secret" });`,
		},
		{
			name:   "invalid-json-schema",
			source: `agent.run({ prompt: "prompt-secret", schema: { type: "not-a-schema-type" } });`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			stub := &stubChildExecutor{mode: stubChildExecutionMode}
			outcome, err := runtimeWorkflows.Run(context.Background(), factory.JavaScriptRuntimeRequest{
				Source: test.source, SourceRef: "inline", SessionID: "invalid-child-schema-" + test.name,
				Policy: workflowpolicy.DefaultEffectivePolicy(),
			}, factory.JavaScriptRuntimeHooks{NewChildExecutor: func(string, factory.JavaScriptChildRecordSink, workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
				return stub
			}})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if outcome.OK || !strings.Contains(outcome.Failure.Message, `"schema"`) {
				t.Fatalf("Run() outcome = %#v, want schema failure", outcome)
			}
			for _, secret := range []string{"prompt-secret", "schema-secret"} {
				if strings.Contains(outcome.Failure.Message, secret) {
					t.Fatalf("failure message = %q, must not expose %q", outcome.Failure.Message, secret)
				}
			}
			if len(stub.executionRequests()) != 0 {
				t.Fatalf("executor requests = %#v, want none", stub.executionRequests())
			}
			assertNoChildDispatchRecords(t, outcome.Records)
		})
	}
}

func TestSchemaDigest_IsDeterministicForEquivalentSchemas(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"required":   []any{"answer"},
		"properties": map[string]any{"answer": map[string]any{"type": "string"}},
		"type":       "object",
	}
	right := map[string]any{
		"type":       "object",
		"properties": map[string]any{"answer": map[string]any{"type": "string"}},
		"required":   []any{"answer"},
	}
	if got, want := workflowruntime.SchemaDigest(left), workflowruntime.SchemaDigest(right); got == "" || got != want {
		t.Fatalf("SchemaDigest(left) = %q, right = %q, want equal non-empty digests", got, want)
	}
}

func TestRun_ChildExecutionBoundary_RoutesAgentRunParallelAndPipelineThroughHooks(t *testing.T) {
	source := readFixture(t, "child-execution-boundary.workflow.js")
	stub := &stubChildExecutor{mode: stubChildExecutionMode}
	req := factory.JavaScriptRuntimeRequest{
		Source:    source,
		SourceRef: "child-execution-boundary.workflow.js",
		SessionID: "session-child-execution-boundary",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata: map[string]string{
			"name": "child-execution-boundary",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}
	hooks := factory.JavaScriptRuntimeHooks{
		NewChildExecutor: func(_ string, _ factory.JavaScriptChildRecordSink, _ workflowpolicy.EffectivePolicy) factory.JavaScriptChildExecutor {
			return stub
		},
	}

	outcome, err := runtimeWorkflows.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}

	assertStubChildExecutorLabelOrder(t, stub.labelOrder())
	assertChildExecutionBoundaryProjection(t, projectPrimaryJSON(t, req.SessionID, outcome.Value))
}

func assertStubChildExecutorLabelOrder(t *testing.T, gotLabels []string) {
	t.Helper()
	if len(gotLabels) != 5 {
		t.Fatalf("child executor call count = %d, want 5", len(gotLabels))
	}
	if gotLabels[0] != "agent-run-boundary" {
		t.Fatalf("first child label = %q, want agent-run-boundary", gotLabels[0])
	}
	parallelLabels := append([]string(nil), gotLabels[1], gotLabels[2])
	sort.Strings(parallelLabels)
	if fmt.Sprint(parallelLabels) != fmt.Sprint([]string{"parallel-boundary-a", "parallel-boundary-b"}) {
		t.Fatalf("parallel child labels = %#v, want [parallel-boundary-a parallel-boundary-b]", parallelLabels)
	}
	if gotLabels[3] != "pipeline-edit-boundary" || gotLabels[4] != "pipeline-review-boundary" {
		t.Fatalf("pipeline child labels = %#v, want [pipeline-edit-boundary pipeline-review-boundary]", gotLabels[3:])
	}
}

func assertChildExecutionBoundaryProjection(t *testing.T, projected map[string]any) {
	t.Helper()
	if projected["label"] != "child-execution-boundary" {
		t.Fatalf("projected label = %#v", projected["label"])
	}

	single, ok := projected["single"].(map[string]any)
	if !ok {
		t.Fatalf("projected single = %#v, want object", projected["single"])
	}
	assertStubChildResult(t, single, "agent-run-boundary", "stub-dispatch-1")

	parallel, ok := projected["parallel"].([]any)
	if !ok || len(parallel) != 2 {
		t.Fatalf("projected parallel = %#v, want 2 entries", projected["parallel"])
	}
	assertStubChildResultByLabel(t, parallel[0], "parallel-boundary-a")
	assertStubChildResultByLabel(t, parallel[1], "parallel-boundary-b")

	pipeline, ok := projected["pipeline"].([]any)
	if !ok || len(pipeline) != 1 {
		t.Fatalf("projected pipeline = %#v, want 1 item", projected["pipeline"])
	}
	item, ok := pipeline[0].(map[string]any)
	if !ok || item["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline item = %#v, want completed status", pipeline[0])
	}
	stages, ok := item["stages"].([]any)
	if !ok || len(stages) != 2 {
		t.Fatalf("pipeline stages = %#v, want 2 stages", item["stages"])
	}
	editStage, ok := stages[0].(map[string]any)
	if !ok || editStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline edit stage = %#v", stages[0])
	}
	assertStubChildResult(t, editStage["result"], "pipeline-edit-boundary", "stub-dispatch-4")

	reviewStage, ok := stages[1].(map[string]any)
	if !ok || reviewStage["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("pipeline review stage = %#v", stages[1])
	}
	assertStubChildResult(t, reviewStage["result"], "pipeline-review-boundary", "stub-dispatch-5")
}

func assertStubChildResultByLabel(t *testing.T, value any, wantLabel string) {
	t.Helper()
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("child result = %#v, want object", value)
	}
	if child["label"] != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	assertStubChildResult(t, value, wantLabel, "")
}

func assertStubChildResult(t *testing.T, value any, wantLabel, wantDispatchID string) {
	t.Helper()
	child, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("child result = %#v, want object", value)
	}
	if child["status"] != factory.JavaScriptChildDispatchStatusCompleted {
		t.Fatalf("child status = %#v, want %q", child["status"], factory.JavaScriptChildDispatchStatusCompleted)
	}
	if child["executionMode"] != stubChildExecutionMode {
		t.Fatalf("child executionMode = %#v, want %q", child["executionMode"], stubChildExecutionMode)
	}
	if child["label"] != wantLabel {
		t.Fatalf("child label = %#v, want %q", child["label"], wantLabel)
	}
	if wantDispatchID != "" && child["dispatchId"] != wantDispatchID {
		t.Fatalf("child dispatchId = %#v, want %q", child["dispatchId"], wantDispatchID)
	}
	output, ok := child["output"].(map[string]any)
	if !ok {
		t.Fatalf("child output = %#v, want object", child["output"])
	}
	wantText := fmt.Sprintf("stub:%s", wantLabel)
	if output["text"] != wantText {
		t.Fatalf("child output text = %#v, want %q", output["text"], wantText)
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
	failLabel    string
	judgeText    string
}

func (e *packagedWorkflowChildExecutor) Execute(_ context.Context, request factory.JavaScriptChildExecutionRequest) (factory.JavaScriptChildExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !request.SkipPermissions {
		return factory.JavaScriptChildExecutionResult{}, fmt.Errorf("packaged child %q did not default skipPermissions", request.Label)
	}
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
	var output map[string]any
	switch request.Label {
	case "spawn-planner":
		tasks := e.plannerTasks
		if tasks == nil {
			tasks = []string{"task one", "task two"}
		}
		output = map[string]any{"tasks": tasks}
	case "spawn-merger":
		output = map[string]any{"answer": "merged spawn result"}
	default:
		if strings.HasPrefix(request.Label, "tournament-judge-") {
			text = e.judgeText
			if text == "" {
				text = `{"winner":"B","rationale":"candidate B wins"}`
			}
		} else if strings.HasPrefix(request.Label, "research-specialist-") {
			output = map[string]any{"evidence": text}
		} else if strings.HasPrefix(request.Label, "spawn-task-") {
			output = map[string]any{"result": text}
		} else if request.Label == "lead-research-synthesis" {
			output = map[string]any{"answer": text}
		}
	}
	if output == nil {
		output = map[string]any{"text": text}
	}
	return factory.JavaScriptChildExecutionResult{
		DispatchID:      fmt.Sprintf("dispatch-%d", e.calls),
		ChildIndex:      e.calls - 1,
		Status:          factory.JavaScriptChildDispatchStatusCompleted,
		ExecutionMode:   factory.JavaScriptChildExecutionModeFake,
		Output:          output,
		SchemaValidated: len(request.OutputSchema) > 0,
	}, nil
}

func (e *packagedWorkflowChildExecutor) labels() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.called...)
}

func projectPrimaryText(t *testing.T, sessionID string, value factory.TypedValue) string {
	t.Helper()
	parts, projection := factory.ProjectPrimaryResult(sessionID, value, nil)
	if projection.HasIssues() {
		t.Fatalf("project primary result: %#v", projection.Issues)
	}
	if len(parts) != 1 || parts[0].Type != work.WorkContentPartTypeText {
		t.Fatalf("primary result parts = %#v, want one text part", parts)
	}
	return parts[0].Text
}
