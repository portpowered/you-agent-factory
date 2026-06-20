package goal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var packagedGoalLifecycleStates = []string{
	"init",
	"plan",
	"execute",
	"check",
	"review",
	"summarize",
	"complete",
	"blocked",
	"needs-human",
	"interrupted",
	"failed",
}

var packagedGoalProgressionWorkstations = []struct {
	name            string
	inputState      string
	outputState     string
	publicType      string
	workerName      string
}{
	{name: PackagedPlanWorkstationName, inputState: "init", outputState: "plan", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-planner"},
	{name: PackagedExecuteWorkstationName, inputState: "plan", outputState: "execute", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-executor"},
	{name: PackagedCheckWorkstationName, inputState: "execute", outputState: "check", publicType: interfaces.WorkstationTypeScript, workerName: "goal-checker"},
	{name: "advance-goal-review", inputState: "check", outputState: "review", publicType: interfaces.WorkstationTypeLogical, workerName: ""},
	{name: PackagedSummarizeWorkstationName, inputState: "summarize", outputState: "complete", publicType: interfaces.WorkstationTypeAgent, workerName: PackagedSummarizerWorkerName},
}

var packagedGoalWorkerPublicTypes = map[string]string{
	"goal-planner":  interfaces.WorkerTypeAgent,
	"goal-executor": interfaces.WorkerTypeAgent,
	"goal-checker":  interfaces.WorkerTypeScript,
	"goal-reviewer":   interfaces.WorkerTypeAgent,
	PackagedSummarizerWorkerName: interfaces.WorkerTypeAgent,
}

var packagedGoalWorkstationPublicTypes = map[string]string{
	PackagedPlanWorkstationName:          interfaces.WorkstationTypeAgent,
	PackagedExecuteWorkstationName:       interfaces.WorkstationTypeAgent,
	PackagedCheckWorkstationName:         interfaces.WorkstationTypeScript,
	"advance-goal-review":                interfaces.WorkstationTypeLogical,
	PackagedReviewWorkstationName:      interfaces.WorkstationTypeClassify,
	PackagedSummarizeWorkstationName:   interfaces.WorkstationTypeAgent,
	PackagedLoopBreakerWorkstationName: interfaces.WorkstationTypeLogical,
}

func TestBuiltInFactoryJSON_LoadsRunnablePackagedGoalFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	if cfg.Name != PackagedFactoryName {
		t.Fatalf("factory name = %q, want %s", cfg.Name, PackagedFactoryName)
	}
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 {
		t.Fatalf("workTypes = %#v, want one goal work type", cfg.WorkTypes)
	}
	workType := cfg.WorkTypes[0]
	if workType.Name != PackagedGoalWorkTypeName {
		t.Fatalf("work type name = %q, want %s", workType.Name, PackagedGoalWorkTypeName)
	}
	if len(workType.HandlingBehavior) != 1 || workType.HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("handlingBehavior = %#v, want [DEFAULT]", workType.HandlingBehavior)
	}
	assertGoalLifecycleStates(t, workType.States)
	assertGoalProgressionTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestBuiltInGoalFactoryJSON_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCanonicalWorkTypeAndLifecycleStates(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	cfg := loaded.FactoryConfig()
	if cfg.Project != PackagedFactoryProject {
		t.Fatalf("materialized project = %q, want %s", cfg.Project, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 {
		t.Fatalf("materialized workTypes = %#v, want one goal work type", cfg.WorkTypes)
	}
	workType := cfg.WorkTypes[0]
	if workType.Name != PackagedGoalWorkTypeName {
		t.Fatalf("materialized work type name = %q, want %s", workType.Name, PackagedGoalWorkTypeName)
	}
	if len(workType.HandlingBehavior) != 1 || workType.HandlingBehavior[0] != interfaces.WorkTypeHandlingBehaviorDefault {
		t.Fatalf("materialized handlingBehavior = %#v, want [DEFAULT]", workType.HandlingBehavior)
	}
	assertGoalLifecycleStates(t, workType.States)
	assertGoalProgressionTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertGoalPublicPrimitiveVocabulary(t, loaded.FactoryConfig().Workers, loaded.FactoryConfig().Workstations)
}

var packagedGoalBoundedWorkerRolePromptExpectations = []struct {
	role           string
	mustContain    []string
	mustNotContain []string
}{
	{
		role: "planner",
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded plan",
			"## Goal",
			"## Plan",
			"## Acceptance checks",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "executor",
		mustContain: []string{
			"AGENT_RUN",
			"AGENT_WORKER",
			"bounded execution result",
			"## Completed work",
			"## Blockers",
			"## Follow-up for review",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "checker",
		mustContain: []string{
			"SCRIPT_RUN",
			"reviewable verification findings",
			"## Checks run",
			"## Findings",
			"## Recommendation",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
	{
		role: "reviewer",
		mustContain: []string{
			"AGENT_WORKER",
			"reviewable disposition",
			"## Disposition",
			"accepted",
			"needs_changes",
			"## Findings",
			"open-ended discussion",
		},
		mustNotContain: []string{"MODEL_WORKER", "MODEL_RUN"},
	},
}

func TestMaterializedPackagedGoalFactory_AuthorBoundedSummarizerPrompt(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	source, ok := packagedGoalRolePromptSourceByRole("summarizer")
	if !ok {
		t.Fatal("missing packaged role prompt source for summarizer")
	}

	workstation, ok := loaded.Workstation(source.WorkstationName)
	if !ok {
		t.Fatalf("missing workstation %q for summarizer role", source.WorkstationName)
	}
	prompt := strings.TrimSpace(workstation.PromptTemplate)
	if prompt == "" {
		t.Fatal("summarizer prompt is empty")
	}

	mustContain := []string{
		"AGENT_RUN",
		"AGENT_WORKER",
		"SCRIPT_RUN",
		"bounded final summary",
		"## Outcome",
		"## What was done",
		"## Verification",
		"## Follow-up",
		"open-ended discussion",
	}
	for _, marker := range mustContain {
		if !strings.Contains(prompt, marker) {
			t.Fatalf("summarizer prompt missing %q:\n%s", marker, prompt)
		}
	}
	for _, marker := range []string{"MODEL_WORKER", "MODEL_RUN"} {
		if strings.Contains(prompt, marker) {
			t.Fatalf("summarizer prompt must not contain legacy marker %q:\n%s", marker, prompt)
		}
	}
}

func TestMaterializedPackagedGoalFactory_AuthorBoundedWorkerRolePrompts(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	for _, expectation := range packagedGoalBoundedWorkerRolePromptExpectations {
		source, ok := packagedGoalRolePromptSourceByRole(expectation.role)
		if !ok {
			t.Fatalf("missing packaged role prompt source for role %q", expectation.role)
		}

		workstation, ok := loaded.Workstation(source.WorkstationName)
		if !ok {
			t.Fatalf("missing workstation %q for role %q", source.WorkstationName, expectation.role)
		}
		prompt := strings.TrimSpace(workstation.PromptTemplate)
		if prompt == "" {
			t.Fatalf("role %q prompt is empty", expectation.role)
		}
		for _, marker := range expectation.mustContain {
			if !strings.Contains(prompt, marker) {
				t.Fatalf("role %q prompt missing %q:\n%s", expectation.role, marker, prompt)
			}
		}
		for _, marker := range expectation.mustNotContain {
			if strings.Contains(prompt, marker) {
				t.Fatalf("role %q prompt must not contain legacy marker %q:\n%s", expectation.role, marker, prompt)
			}
		}
	}
}

func TestMaterializedPackagedGoalFactory_ResolvesSplitRolePromptFiles(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	for _, source := range PackagedGoalRolePromptSources {
		promptPath := filepath.Join(factoryDir, interfaces.WorkstationsDir, source.WorkstationName, source.PromptFile)
		promptBytes, err := os.ReadFile(promptPath)
		if err != nil {
			t.Fatalf("role %q prompt file %s: %v", source.Role, promptPath, err)
		}
		promptOnDisk := strings.TrimSpace(string(promptBytes))
		if promptOnDisk == "" {
			t.Fatalf("role %q prompt file %s is empty", source.Role, promptPath)
		}

		workstation, ok := loaded.Workstation(source.WorkstationName)
		if !ok {
			t.Fatalf("missing workstation %q for role %q", source.WorkstationName, source.Role)
		}
		if workstation.PromptFile != source.PromptFile {
			t.Fatalf("workstation %q promptFile = %q, want %q", source.WorkstationName, workstation.PromptFile, source.PromptFile)
		}
		if strings.TrimSpace(workstation.PromptTemplate) != promptOnDisk {
			t.Fatalf("workstation %q prompt template = %q, want loaded split file %q", source.WorkstationName, workstation.PromptTemplate, promptOnDisk)
		}
	}
}

func assertGoalPublicPrimitiveVocabulary(t *testing.T, workers []interfaces.WorkerConfig, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	workerTypes := indexWorkerTypesByName(workers)
	for name, wantPublic := range packagedGoalWorkerPublicTypes {
		rawType, ok := workerTypes[name]
		if !ok {
			t.Fatalf("missing worker %q", name)
		}
		if rawType == interfaces.WorkerTypeModel {
			t.Fatalf("worker %q uses legacy alias %q", name, interfaces.WorkerTypeModel)
		}
		publicType := interfaces.PublicWorkerTypeFromInternalRuntime(rawType)
		if publicType != wantPublic {
			t.Fatalf("worker %q public type = %q, want %q", name, publicType, wantPublic)
		}
	}

	byName := indexWorkstationsByName(workstations)
	for name, wantPublic := range packagedGoalWorkstationPublicTypes {
		workstation, ok := byName[name]
		if !ok {
			t.Fatalf("missing workstation %q", name)
		}
		workerType := workerTypes[workstation.WorkerTypeName]
		publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
		if publicType == interfaces.WorkstationTypeModel {
			t.Fatalf("workstation %q projects to legacy alias %q", name, interfaces.WorkstationTypeModel)
		}
		if publicType != wantPublic {
			t.Fatalf("workstation %q public type = %q, want %q", name, publicType, wantPublic)
		}
	}
}

func assertGoalLifecycleStates(t *testing.T, states []interfaces.StateConfig) {
	t.Helper()

	got := make([]string, 0, len(states))
	for _, state := range states {
		got = append(got, state.Name)
	}
	slices.Sort(got)
	want := append([]string(nil), packagedGoalLifecycleStates...)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("goal lifecycle states = %#v, want %#v", got, want)
	}
}

func assertGoalProgressionTopology(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, workers []interfaces.WorkerConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	workerTypes := indexWorkerTypesByName(workers)
	for _, want := range packagedGoalProgressionWorkstations {
		workstation, ok := byName[want.name]
		if !ok {
			t.Fatalf("missing progression workstation %q", want.name)
		}
		workerType := workerTypes[workstation.WorkerTypeName]
		publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
		if publicType != want.publicType {
			t.Fatalf("workstation %q public type = %q, want %q", want.name, publicType, want.publicType)
		}
		assertSingleGoalRoute(t, want.name, workstation.Inputs, want.inputState)
		assertSingleGoalRoute(t, want.name, workstation.Outputs, want.outputState)
	}
}

func assertGoalReviewRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	review, ok := indexWorkstationsByName(workstations)[PackagedReviewWorkstationName]
	if !ok {
		t.Fatal("missing review workstation")
	}
	if review.Type != interfaces.WorkstationTypeClassify {
		t.Fatalf("review workstation type = %q, want %q", review.Type, interfaces.WorkstationTypeClassify)
	}

	wantRoutes := map[string]string{
		"accepted":      "summarize",
		"needs_changes": "plan",
		"tests_failed":  "plan",
		"needs_human":   "needs-human",
		"blocked":       "blocked",
		"interrupted":   "interrupted",
		"failed":        "failed",
	}
	gotRoutes := make(map[string]string, len(review.ClassificationRoutes))
	for _, route := range review.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("review route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("review route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("review route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	execute, ok := indexWorkstationsByName(workstations)[PackagedExecuteWorkstationName]
	if !ok {
		t.Fatal("missing execute workstation for retry path")
	}
	assertSingleGoalRoute(t, execute.Name, execute.Inputs, "plan")
}

func assertGoalLoopBreaker(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	loopBreaker, ok := indexWorkstationsByName(workstations)[PackagedLoopBreakerWorkstationName]
	if !ok {
		t.Fatal("missing goal loop breaker workstation")
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("loop breaker guards = %#v, want one visit_count guard", loopBreaker.Guards)
	}
	guard := loopBreaker.Guards[0]
	if guard.Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("loop breaker guard type = %q, want %q", guard.Type, interfaces.GuardTypeVisitCount)
	}
	if guard.Workstation != PackagedReviewWorkstationName {
		t.Fatalf("loop breaker guard workstation = %q, want %q", guard.Workstation, PackagedReviewWorkstationName)
	}
	if guard.MaxVisits != 5 {
		t.Fatalf("loop breaker guard maxVisits = %d, want 5", guard.MaxVisits)
	}
	assertSingleGoalRoute(t, loopBreaker.Name, loopBreaker.Inputs, "plan")
	assertSingleGoalRoute(t, loopBreaker.Name, loopBreaker.Outputs, "failed")
}

func assertSingleGoalRoute(t *testing.T, workstationName string, routes []interfaces.IOConfig, wantState string) {
	t.Helper()

	if len(routes) != 1 {
		t.Fatalf("workstation %q routes = %#v, want one goal route", workstationName, routes)
	}
	if routes[0].WorkTypeName != PackagedGoalWorkTypeName {
		t.Fatalf("workstation %q work type = %q, want %s", workstationName, routes[0].WorkTypeName, PackagedGoalWorkTypeName)
	}
	if routes[0].StateName != wantState {
		t.Fatalf("workstation %q state = %q, want %q", workstationName, routes[0].StateName, wantState)
	}
}

func indexWorkerTypesByName(workers []interfaces.WorkerConfig) map[string]string {
	byName := make(map[string]string, len(workers))
	for _, worker := range workers {
		byName[worker.Name] = worker.Type
	}
	return byName
}

func indexWorkstationsByName(workstations []interfaces.FactoryWorkstationConfig) map[string]interfaces.FactoryWorkstationConfig {
	byName := make(map[string]interfaces.FactoryWorkstationConfig, len(workstations))
	for _, workstation := range workstations {
		byName[workstation.Name] = workstation
	}
	return byName
}

func packagedGoalRolePromptSourceByRole(role string) (PackagedGoalRolePromptSource, bool) {
	for _, source := range PackagedGoalRolePromptSources {
		if source.Role == role {
			return source, true
		}
	}
	return PackagedGoalRolePromptSource{}, false
}
