package goal

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var packagedGoalLegacyPrimitiveAliases = []string{
	"MODEL_WORKER",
	"MODEL_WORKSTATION",
}

var packagedGoalRequiredPublicPrimitives = []string{
	interfaces.WorkerTypeAgent,
	interfaces.WorkerTypeScript,
	interfaces.WorkstationTypeAgent,
	interfaces.WorkstationTypeScript,
	interfaces.WorkstationTypeLogical,
	interfaces.WorkstationTypeClassify,
}

var packagedGoalLifecycleStates = []string{
	"init",
	"plan",
	"execute",
	"check",
	"review",
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
}

func TestBuiltInGoalFactoryJSON_UsesCurrentPublicPrimitiveNames(t *testing.T) {
	assertPackagedGoalAuthoredJSONUsesPublicPrimitives(t, "built-in goal factory JSON", factoryconfig.BuiltInGoalFactoryJSON)
}

func TestMaterializedPackagedGoalFactory_AuthoredFilesUseCurrentPublicPrimitiveNames(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", factoryJSONPath, err)
	}
	assertPackagedGoalAuthoredJSONUsesPublicPrimitives(t, interfaces.FactoryConfigFile, factoryJSON)

	err = filepath.WalkDir(factoryDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || path == factoryJSONPath {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".gitkeep") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(factoryDir, path)
		if err != nil {
			return err
		}
		assertPackagedGoalAuthoredContentUsesPublicPrimitives(t, relativePath, string(content))
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(materialized goal factory): %v", err)
	}
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
}

func assertPackagedGoalAuthoredContentUsesPublicPrimitives(t *testing.T, sourceLabel, content string) {
	t.Helper()

	for _, alias := range packagedGoalLegacyPrimitiveAliases {
		if strings.Contains(content, alias) {
			t.Fatalf("%s introduces legacy alias %q", sourceLabel, alias)
		}
	}
}

func assertPackagedGoalAuthoredJSONUsesPublicPrimitives(t *testing.T, sourceLabel string, content []byte) {
	t.Helper()

	authored := string(content)
	assertPackagedGoalAuthoredContentUsesPublicPrimitives(t, sourceLabel, authored)
	for _, primitive := range packagedGoalRequiredPublicPrimitives {
		if !strings.Contains(authored, primitive) {
			t.Fatalf("%s missing required public primitive %q", sourceLabel, primitive)
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
		"accepted":      "complete",
		"needs_changes": "execute",
		"tests_failed":  "execute",
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
	assertSingleGoalRoute(t, loopBreaker.Name, loopBreaker.Inputs, "review")
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
