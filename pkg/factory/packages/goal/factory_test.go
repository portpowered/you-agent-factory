package goal

import (
	"context"
	"slices"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

var packagedGoalLifecycleStates = []string{
	"init",
	"plan",
	"execute",
	"check",
	"review",
	"structured-review",
	"complete",
	"blocked",
	"needs-human",
	"interrupted",
	"failed",
}

var packagedGoalProgressionWorkstations = []struct {
	name        string
	inputState  string
	outputState string
	publicType  string
	workerName  string
}{
	{name: PackagedPlanWorkstationName, inputState: "init", outputState: "plan", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-planner"},
	{name: PackagedExecuteWorkstationName, inputState: "plan", outputState: "execute", publicType: interfaces.WorkstationTypeAgent, workerName: "goal-executor"},
	{name: PackagedCheckWorkstationName, inputState: "execute", outputState: "", publicType: interfaces.WorkstationTypeClassify, workerName: "goal-checker"},
}

var packagedGoalWorkerPublicTypes = map[string]string{
	"goal-planner":  interfaces.WorkerTypeAgent,
	"goal-executor": interfaces.WorkerTypeAgent,
	"goal-checker":  interfaces.WorkerTypeScript,
	"goal-reviewer": interfaces.WorkerTypeAgent,
}

var packagedGoalWorkstationPublicTypes = map[string]string{
	PackagedPlanWorkstationName:                  interfaces.WorkstationTypeAgent,
	PackagedExecuteWorkstationName:               interfaces.WorkstationTypeAgent,
	PackagedCheckWorkstationName:                 interfaces.WorkstationTypeClassify,
	PackagedReviewWorkstationName:                interfaces.WorkstationTypeClassify,
	PackagedStructuredReviewWorkstationName:      interfaces.WorkstationTypeAgent,
	PackagedLoopBreakerWorkstationName:           interfaces.WorkstationTypeLogical,
	PackagedStructuredLoopBreakerWorkstationName: interfaces.WorkstationTypeLogical,
}

func TestBuiltInFactoryJSON_LoadsRunnablePackagedGoalFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
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
	assertGoalCheckerContract(t, cfg.Workers)
	assertGoalCheckReviewModeRoutes(t, cfg.Workstations)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalStructuredReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestBuiltInGoalFactory_ExecuteStateSchedulesCheckReviewModeClassifier(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	mapper := &factoryconfig.ConfigMapper{}
	net, err := mapper.Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}

	now := time.Date(2026, time.June, 20, 16, 0, 0, 0, time.UTC)
	executeToken := &interfaces.Token{
		ID:        "tok-execute",
		PlaceID:   "goal:execute",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Minute),
		Color: interfaces.TokenColor{
			WorkID:     "work-goal-1",
			WorkTypeID: PackagedGoalWorkTypeName,
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{},
			ConsecutiveFailures: map[string]int{},
			PlaceVisits:         map[string]int{},
		},
	}
	marking := petri.MarkingSnapshot{
		Tokens:      map[string]*interfaces.Token{"tok-execute": executeToken},
		PlaceTokens: map[string][]string{"goal:execute": {"tok-execute"}},
	}
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking:  marking,
		Topology: net,
	}

	runtimeLookup := goalRuntimeLookup{
		workstations: indexWorkstationsByName(cfg.Workstations),
		workers:      indexWorkerConfigsByName(cfg.Workers),
	}
	evaluator := scheduler.NewEnablementEvaluator(
		nil,
		scheduler.WithEnablementRuntimeConfig(runtimeLookup),
	)
	enabled := evaluator.FindEnabledTransitionsWithSnapshot(context.Background(), net, snapshot)

	var checkTransitionIDs []string
	for _, et := range enabled {
		transition := net.Transitions[et.TransitionID]
		if transition == nil {
			continue
		}
		if transition.Name == PackagedCheckWorkstationName {
			checkTransitionIDs = append(checkTransitionIDs, transition.Name)
		}
	}
	if len(checkTransitionIDs) != 1 {
		t.Fatalf("enabled check classifiers = %#v, want only %q", checkTransitionIDs, PackagedCheckWorkstationName)
	}

	sched := scheduler.NewWorkInQueueScheduler(1)
	decisions := sched.Select(enabled, snapshot)
	if len(decisions) != 1 {
		t.Fatalf("scheduler decisions = %#v, want one check classifier from goal:execute", decisions)
	}
	selected := net.Transitions[decisions[0].TransitionID]
	if selected == nil || selected.Name != PackagedCheckWorkstationName {
		t.Fatalf("selected transition = %#v, want %q", selected, PackagedCheckWorkstationName)
	}
}

func TestBuiltInGoalFactoryJSON_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCanonicalWorkTypeAndLifecycleStates(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
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
	assertGoalCheckerContract(t, cfg.Workers)
	assertGoalCheckReviewModeRoutes(t, cfg.Workstations)
	assertGoalReviewRoutes(t, cfg.Workstations)
	assertGoalStructuredReviewRoutes(t, cfg.Workstations)
	assertGoalLoopBreaker(t, cfg.Workstations)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertGoalPublicPrimitiveVocabulary(t, loaded.FactoryConfig().Workers, loaded.FactoryConfig().Workstations)
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
		if want.outputState != "" {
			assertSingleGoalRoute(t, want.name, workstation.Outputs, want.outputState)
		}
	}
}

func assertGoalCheckerContract(t *testing.T, workers []interfaces.WorkerConfig) {
	t.Helper()

	checker, ok := indexWorkerConfigsByName(workers)["goal-checker"]
	if !ok {
		t.Fatal("missing goal-checker worker")
	}
	if checker.Command != "sh" {
		t.Fatalf("goal-checker command = %q, want %q", checker.Command, "sh")
	}
	if len(checker.Args) != 2 {
		t.Fatalf("goal-checker args = %#v, want shell wrapper", checker.Args)
	}
	if checker.Args[0] != "-c" {
		t.Fatalf("goal-checker args[0] = %q, want %q", checker.Args[0], "-c")
	}
	wantScript := "make test >/dev/null && printf '%s' \"${" + PackagedCheckReviewModeEnvVar + ":-plain}\""
	if checker.Args[1] != wantScript {
		t.Fatalf("goal-checker shell wrapper = %q, want %q", checker.Args[1], wantScript)
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

func assertGoalStructuredReviewRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	structuredReview, ok := byName[PackagedStructuredReviewWorkstationName]
	if !ok {
		t.Fatal("missing structured review workstation")
	}
	if structuredReview.Type != interfaces.WorkstationTypeAgent && structuredReview.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("structured review workstation type = %q, want agent runtime type", structuredReview.Type)
	}
	if structuredReview.OutcomeFormat != DecisionEnvelopeOutcomeFormat {
		t.Fatalf("structured review outcomeFormat = %q, want %q", structuredReview.OutcomeFormat, DecisionEnvelopeOutcomeFormat)
	}
	if !UsesGoalRoutingDecisionEnvelope(&structuredReview) {
		t.Fatal("structured review workstation should use goal routing decision envelope")
	}
	assertSingleGoalRoute(t, structuredReview.Name, structuredReview.Inputs, PackagedStructuredReviewStateName)

	wantRoutes := map[string]string{
		"accepted":      "complete",
		"needs_changes": "plan",
		"tests_failed":  "plan",
		"needs_human":   "needs-human",
		"blocked":       "blocked",
		"interrupted":   "interrupted",
		"failed":        "failed",
	}
	gotRoutes := make(map[string]string, len(structuredReview.ClassificationRoutes))
	for _, route := range structuredReview.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("structured review route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("structured review route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("structured review route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	if _, ok := byName[PackagedAdvanceStructuredReviewWorkstationName]; ok {
		t.Fatalf("built-in factory should not declare %q; check-goal routes review modes directly", PackagedAdvanceStructuredReviewWorkstationName)
	}
}

func assertGoalCheckReviewModeRoutes(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	check, ok := indexWorkstationsByName(workstations)[PackagedCheckWorkstationName]
	if !ok {
		t.Fatal("missing check workstation")
	}
	if check.Type != interfaces.WorkstationTypeClassify {
		t.Fatalf("check workstation type = %q, want %q", check.Type, interfaces.WorkstationTypeClassify)
	}
	assertSingleGoalRoute(t, check.Name, check.Inputs, "execute")

	wantRoutes := map[string]string{
		PackagedReviewModePlainLabel:      "review",
		PackagedReviewModeStructuredLabel: PackagedStructuredReviewStateName,
	}
	gotRoutes := make(map[string]string, len(check.ClassificationRoutes))
	for _, route := range check.ClassificationRoutes {
		if len(route.Outputs) != 1 {
			t.Fatalf("check route %q outputs = %#v, want one goal output", route.Label, route.Outputs)
		}
		gotRoutes[route.Label] = route.Outputs[0].StateName
	}
	for label, wantState := range wantRoutes {
		gotState, ok := gotRoutes[label]
		if !ok {
			t.Fatalf("check route missing label %q", label)
		}
		if gotState != wantState {
			t.Fatalf("check route %q state = %q, want %q", label, gotState, wantState)
		}
	}

	workersByName := indexWorkstationsByName(workstations)
	if got := workersByName[PackagedCheckWorkstationName].Env[PackagedCheckReviewModeEnvVar]; got != "" {
		t.Fatalf("check workstation env %q = %q, want empty default so checker falls back to plain", PackagedCheckReviewModeEnvVar, got)
	}
}

func assertGoalLoopBreaker(t *testing.T, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	byName := indexWorkstationsByName(workstations)
	assertGoalLoopBreakerWorkstation(t, byName, PackagedLoopBreakerWorkstationName, PackagedReviewWorkstationName)
	assertGoalLoopBreakerWorkstation(t, byName, PackagedStructuredLoopBreakerWorkstationName, PackagedStructuredReviewWorkstationName)
}

func assertGoalLoopBreakerWorkstation(
	t *testing.T,
	workstations map[string]interfaces.FactoryWorkstationConfig,
	workstationName string,
	watchedWorkstation string,
) {
	t.Helper()

	loopBreaker, ok := workstations[workstationName]
	if !ok {
		t.Fatalf("missing goal loop breaker workstation %q", workstationName)
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker %q type = %q, want %q", workstationName, loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("loop breaker %q guards = %#v, want one visit_count guard", workstationName, loopBreaker.Guards)
	}
	guard := loopBreaker.Guards[0]
	if guard.Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("loop breaker %q guard type = %q, want %q", workstationName, guard.Type, interfaces.GuardTypeVisitCount)
	}
	if guard.Workstation != watchedWorkstation {
		t.Fatalf("loop breaker %q guard workstation = %q, want %q", workstationName, guard.Workstation, watchedWorkstation)
	}
	if guard.MaxVisits != 5 {
		t.Fatalf("loop breaker %q guard maxVisits = %d, want 5", workstationName, guard.MaxVisits)
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

func indexWorkerConfigsByName(workers []interfaces.WorkerConfig) map[string]interfaces.WorkerConfig {
	byName := make(map[string]interfaces.WorkerConfig, len(workers))
	for _, worker := range workers {
		byName[worker.Name] = worker
	}
	return byName
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

type goalRuntimeLookup struct {
	workstations map[string]interfaces.FactoryWorkstationConfig
	workers      map[string]interfaces.WorkerConfig
}

func (l goalRuntimeLookup) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := l.workstations[name]
	if !ok {
		return nil, false
	}
	return &workstation, true
}

func (l goalRuntimeLookup) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := l.workers[name]
	if !ok {
		return nil, false
	}
	return &worker, true
}
