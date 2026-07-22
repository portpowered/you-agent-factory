package goal

import (
	"slices"
	"strings"
	"testing"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

var packagedGoalLifecycleStates = []string{
	"init",
	"execute",
	"complete",
	"failed",
}

var packagedGoalWorkerPublicTypes = map[string]string{
	"goal-executor": interfaces.WorkerTypeAgent,
}

var packagedGoalWorkstationPublicTypes = map[string]string{
	PackagedExecuteWorkstationName: interfaces.WorkstationTypeAgent,
}

func TestBuiltInFactoryJSON_LoadsRunnablePackagedGoalFactory(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
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
	assertGoalRepeaterTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestBuiltInGoalFactoryJSON_PassesTopologyValidation(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	findings := factoryvalidation.New(nil).
		ValidateTopology(t.Context(), cfg, nil).
		Findings
	for _, finding := range findings {
		if finding.Severity == interfaces.ValidationSeverityError {
			t.Fatalf("structural finding = %#v, want no validation errors", finding)
		}
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v, want no validation errors", target)
		}
	}
}

func TestBuiltInGoalFactoryJSON_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("ParseFactoryConfig: %v", err)
	}
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCanonicalWorkTypeAndLifecycleStates(t *testing.T) {
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
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
	assertGoalRepeaterTopology(t, cfg.Workstations, cfg.Workers)
	assertGoalPublicPrimitiveVocabulary(t, cfg.Workers, cfg.Workstations)
}

func TestMaterializedPackagedGoalFactory_ExposesCurrentPublicPrimitiveVocabulary(t *testing.T) {
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertGoalPublicPrimitiveVocabulary(t, loaded.FactoryConfig().Workers, loaded.FactoryConfig().Workstations)
}

func assertGoalPublicPrimitiveVocabulary(t *testing.T, workers []workerconfig.Config, workstations []interfaces.FactoryWorkstationConfig) {
	t.Helper()

	if len(workstations) != 1 {
		t.Fatalf("workstations = %d, want exactly one minimal repeater", len(workstations))
	}

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

func assertGoalRepeaterTopology(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, workers []workerconfig.Config) {
	t.Helper()

	if len(workstations) != 1 {
		t.Fatalf("workstations = %#v, want exactly one repeater workstation", workstations)
	}
	if len(workers) != 1 {
		t.Fatalf("workers = %#v, want exactly one agent worker", workers)
	}

	execute, ok := indexWorkstationsByName(workstations)[PackagedExecuteWorkstationName]
	if !ok {
		t.Fatalf("missing repeater workstation %q", PackagedExecuteWorkstationName)
	}
	if execute.Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("execute workstation kind = %q, want %q", execute.Kind, interfaces.WorkstationKindRepeater)
	}
	if execute.Type != interfaces.WorkstationTypeAgent && execute.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("execute workstation type = %q, want agent runtime type", execute.Type)
	}
	if execute.WorkerTypeName != "goal-executor" {
		t.Fatalf("execute workstation worker = %q, want goal-executor", execute.WorkerTypeName)
	}

	assertSingleGoalRoute(t, execute.Name, execute.Inputs, "init")

	if len(execute.Outputs) != 1 {
		t.Fatalf("execute outputs = %#v, want one completion route", execute.Outputs)
	}
	for _, route := range execute.Outputs {
		if route.StateName != "complete" {
			t.Fatalf("execute output state = %q, want complete", route.StateName)
		}
	}
	assertSingleGoalRoute(t, execute.Name, execute.OnContinue, "init")
	assertSingleGoalRoute(t, execute.Name, execute.OnRejection, "init")
	assertSingleGoalRoute(t, execute.Name, execute.OnFailure, "failed")

	executor, ok := indexWorkerConfigsByName(workers)["goal-executor"]
	if !ok {
		t.Fatal("missing goal-executor worker")
	}
	if executor.StopToken != "<COMPLETE>" {
		t.Fatalf("goal-executor stopToken = %q, want <COMPLETE>", executor.StopToken)
	}
	if strings.TrimSpace(execute.Body) == "" {
		t.Fatal("execute workstation prompt body is empty")
	}
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

func indexWorkerConfigsByName(workers []workerconfig.Config) map[string]workerconfig.Config {
	byName := make(map[string]workerconfig.Config, len(workers))
	for _, worker := range workers {
		byName[worker.Name] = worker
	}
	return byName
}

func indexWorkerTypesByName(workers []workerconfig.Config) map[string]string {
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
