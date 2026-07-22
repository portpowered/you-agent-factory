package quorum

import (
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/invocationinterpolation"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedQuorumFactory(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("packaged identity = %q/%q, want %q/%q", cfg.Name, cfg.Project, PackagedFactoryName, PackagedFactoryProject)
	}
	if cfg.InvocationSignature == nil || len(cfg.Workers) != 3 || len(cfg.Workstations) != 4 || len(cfg.WorkTypes) != 4 {
		t.Fatalf("quorum config is not runnable: %#v", cfg)
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if strings.HasPrefix(target.Code, "factory.invocationSignature.") {
			t.Fatalf("validation target = %#v, want valid quorum signature", target)
		}
	}
}

func TestBuiltInQuorumFactory_UsesIndependentBranchesAndGatedMerge(t *testing.T) {
	cfg := loadQuorumConfig(t)
	workstations := workstationsByName(cfg.Workstations)
	assertQuorumRoutes(t, workstations)
	assertQuorumLineageAndDependencies(t, workstations)
}

func assertQuorumRoutes(t *testing.T, workstations map[string]interfaces.FactoryWorkstationConfig) {
	t.Helper()

	split := workstations["split-quorum"]
	if !sameRoutes(split.Inputs, []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}) ||
		!sameRoutes(split.Outputs, []interfaces.IOConfig{{WorkTypeName: "task", StateName: "quorum-context"}, {WorkTypeName: "quorum-branch-a", StateName: "init"}, {WorkTypeName: "quorum-branch-b", StateName: "init"}}) ||
		split.WorkPropagation == nil || split.WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("split = %#v, want preserved request context and two independent branch outputs", split)
	}

	for _, branch := range []string{"a", "b"} {
		workType := "quorum-branch-" + branch
		station := workstations["run-quorum-branch-"+branch]
		if !sameRoutes(station.Inputs, []interfaces.IOConfig{{WorkTypeName: workType, StateName: "init"}}) ||
			!sameRoutes(station.Outputs, []interfaces.IOConfig{{WorkTypeName: workType, StateName: "complete"}}) {
			t.Fatalf("branch %s routes = %#v, want isolated %s Work", branch, station, workType)
		}
	}

	merge := workstations["merge-quorum"]
	wantMergeInputs := []interfaces.IOConfig{{WorkTypeName: "task", StateName: "quorum-context"}, {WorkTypeName: "quorum-branch-a", StateName: "complete"}, {WorkTypeName: "quorum-branch-b", StateName: "complete"}}
	if !sameRoutes(merge.Inputs, wantMergeInputs) || !sameRoutes(merge.Outputs, []interfaces.IOConfig{{WorkTypeName: "quorum-merge", StateName: "complete"}}) {
		t.Fatalf("merge routes = %#v, want ordered branch fan-in and one final Work", merge)
	}
}

func assertQuorumLineageAndDependencies(t *testing.T, workstations map[string]interfaces.FactoryWorkstationConfig) {
	t.Helper()
	split := workstations["split-quorum"]
	merge := workstations["merge-quorum"]
	if len(split.Outputs) != 3 {
		t.Fatalf("split outputs = %#v, want preserved request and two derived Work outputs", split.Outputs)
	}
	input := []factoryruntime.RuntimeTokenColor{{WorkID: "request-work", WorkTypeID: "task", DataType: factoryruntime.RuntimeTokenDataTypeWork, RequestID: "request-1"}}
	for index := 1; index < len(split.Outputs); index++ {
		output := &factoryruntime.RuntimeToken{Color: factoryruntime.RuntimeTokenColor{
			WorkID:     split.Outputs[index].WorkTypeName + "-work",
			WorkTypeID: split.Outputs[index].WorkTypeName,
			ParentID:   "request-work",
		}}
		ApplyWorkRelations(output, &split, input)
		if !sameRelations(output.Color.Relations, []work.Relation{{Type: work.RelationParentChild, TargetWorkID: "request-work"}}) {
			t.Fatalf("branch %d relations = %#v, want public parent relation", index, output.Color.Relations)
		}
	}
	if len(merge.Inputs) != 3 {
		t.Fatalf("merge inputs = %#v, want original request and both completed branch Work items before dispatch", merge.Inputs)
	}
	mergeOutput := &factoryruntime.RuntimeToken{Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "quorum-merge"}}
	ApplyWorkRelations(mergeOutput, &merge, []factoryruntime.RuntimeTokenColor{{WorkID: "request-work", WorkTypeID: "task"}, {WorkID: "branch-a-work", WorkTypeID: "quorum-branch-a"}, {WorkID: "branch-b-work", WorkTypeID: "quorum-branch-b"}})
	wantDependencies := []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "branch-a-work", RequiredState: "complete"}, {Type: work.RelationDependsOn, TargetWorkID: "branch-b-work", RequiredState: "complete"}}
	if !sameRelations(mergeOutput.Color.Relations, wantDependencies) {
		t.Fatalf("merge relations = %#v, want dependencies on both branch results", mergeOutput.Color.Relations)
	}
}

func loadQuorumConfig(t *testing.T) *interfaces.FactoryConfig {
	t.Helper()
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	return cfg
}

func workstationsByName(workstations []interfaces.FactoryWorkstationConfig) map[string]interfaces.FactoryWorkstationConfig {
	byName := make(map[string]interfaces.FactoryWorkstationConfig, len(workstations))
	for _, workstation := range workstations {
		byName[workstation.Name] = workstation
	}
	return byName
}

func sameRoutes(got, want []interfaces.IOConfig) bool {
	return reflect.DeepEqual(got, want)
}

func sameRelations(got, want []work.Relation) bool {
	return reflect.DeepEqual(got, want)
}

func TestBuiltInQuorumFactory_DefaultNamedInvocationAcceptsInput(t *testing.T) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	got, err := work.NormalizeArguments(work.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	input, ok := got.Arguments["input"]
	if !ok || !reflect.DeepEqual(input.Values, []string{"Compare two release plans."}) {
		t.Fatalf("input = %#v, want accepted default named invocation input", input)
	}
	assertArgumentValues(t, got.Arguments, "branchProvider", []string{"CODEX"})
	assertArgumentValues(t, got.Arguments, "branchModel", []string{"gpt-5"})
	assertArgumentValues(t, got.Arguments, "mergeProvider", []string{"CODEX"})
	assertArgumentValues(t, got.Arguments, "mergeModel", []string{"gpt-5"})
}

func TestBuiltInQuorumFactory_RoleSpecificInvocationOverridesSelectEffectiveWorkers(t *testing.T) {
	cfg := loadQuorumConfig(t)
	normalized, err := work.NormalizeArguments(work.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
		NamedArgs: []work.NamedArgumentInput{
			{Key: "branch-provider", Values: []string{"CLAUDE"}},
			{Key: "branch-model", Values: []string{"claude-sonnet-4-20250514"}},
			{Key: "merge-provider", Values: []string{"CODEX"}},
			{Key: "merge-model", Values: []string{"gpt-5"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	args := work.RuntimeInvocationArguments(cfg.InvocationSignature, &normalized)
	for _, workerName := range []string{"quorum-branch-a", "quorum-branch-b"} {
		worker, ok := workerByName(cfg.Workers, workerName)
		if !ok {
			t.Fatalf("worker %q missing", workerName)
		}
		effective, err := invocationinterpolation.InterpolateWorkerConfig(*worker, args, nil)
		if err != nil {
			t.Fatalf("InterpolateWorkerConfig(%q): %v", workerName, err)
		}
		if effective.ModelProvider != "CLAUDE" || effective.Model != "claude-sonnet-4-20250514" {
			t.Fatalf("effective %s worker = %#v, want configured branch provider/model", workerName, effective)
		}
	}
	merge, ok := workerByName(cfg.Workers, "quorum-merge")
	if !ok {
		t.Fatal("merge worker missing")
	}
	effectiveMerge, err := invocationinterpolation.InterpolateWorkerConfig(*merge, args, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkerConfig(merge): %v", err)
	}
	if effectiveMerge.ModelProvider != "CODEX" || effectiveMerge.Model != "gpt-5" {
		t.Fatalf("effective merge worker = %#v, want configured merge provider/model", effectiveMerge)
	}
	if len(args.Arguments) != 5 {
		t.Fatalf("runtime invocation arguments = %#v, want five effective parameters", args)
	}
}

func TestBuiltInQuorumFactory_RejectsUnsupportedRoleProvider(t *testing.T) {
	cfg := loadQuorumConfig(t)
	_, err := work.NormalizeArguments(work.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
		NamedArgs:      []work.NamedArgumentInput{{Key: "branch-provider", Values: []string{"unsupported"}}},
	})
	if err == nil || !strings.Contains(err.Error(), "branchProvider") {
		t.Fatalf("NormalizeArguments error = %v, want actionable branchProvider validation error", err)
	}
}

func TestIsPackagedFactory_MatchesBuiltInQuorumIdentity(t *testing.T) {
	if !IsPackagedFactory(&interfaces.FactoryConfig{Name: PackagedFactoryName}) || !IsPackagedFactory(&interfaces.FactoryConfig{Project: PackagedFactoryProject}) {
		t.Fatal("expected packaged quorum identity match")
	}
	if IsPackagedFactory(&interfaces.FactoryConfig{Name: "customer-quorum"}) || IsPackagedFactory(nil) {
		t.Fatal("unexpected packaged quorum identity match")
	}
}

func TestApplyWorkRelations_OnlyEmitsRelationsForEligibleQuorumWork(t *testing.T) {
	branchInputs := []factoryruntime.RuntimeTokenColor{
		{WorkID: "", WorkTypeID: "quorum-branch-a"},
		{WorkID: "request-work", WorkTypeID: "task"},
		{WorkID: "branch-a-work", WorkTypeID: "quorum-branch-a"},
		{WorkID: "branch-b-work", WorkTypeID: "quorum-branch-b"},
	}

	ApplyWorkRelations(nil, &interfaces.FactoryWorkstationConfig{Name: PackagedSplitWorkstationName}, nil)
	ApplyWorkRelations(&factoryruntime.RuntimeToken{}, nil, nil)

	for _, output := range []*factoryruntime.RuntimeToken{
		{Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "task", ParentID: "request-work"}},
		{Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "quorum-branch-a"}},
	} {
		ApplyWorkRelations(output, &interfaces.FactoryWorkstationConfig{Name: PackagedSplitWorkstationName}, nil)
		if len(output.Color.Relations) != 0 {
			t.Fatalf("split ineligible output relations = %#v, want none", output.Color.Relations)
		}
	}

	nonMerge := &factoryruntime.RuntimeToken{Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "task"}}
	ApplyWorkRelations(nonMerge, &interfaces.FactoryWorkstationConfig{Name: PackagedMergeWorkstationName}, branchInputs)
	if len(nonMerge.Color.Relations) != 0 {
		t.Fatalf("non-merge relations = %#v, want none", nonMerge.Color.Relations)
	}

	merge := &factoryruntime.RuntimeToken{Color: factoryruntime.RuntimeTokenColor{WorkTypeID: "quorum-merge"}}
	ApplyWorkRelations(merge, &interfaces.FactoryWorkstationConfig{Name: PackagedMergeWorkstationName}, branchInputs)
	want := []work.Relation{
		{Type: work.RelationDependsOn, TargetWorkID: "branch-a-work", RequiredState: "complete"},
		{Type: work.RelationDependsOn, TargetWorkID: "branch-b-work", RequiredState: "complete"},
	}
	if !sameRelations(merge.Color.Relations, want) {
		t.Fatalf("merge relations = %#v, want %#v", merge.Color.Relations, want)
	}
}

func assertArgumentValues(t *testing.T, arguments map[string]work.NormalizedArgument, name string, want []string) {
	t.Helper()
	got, ok := arguments[name]
	if !ok || !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("argument %q = %#v, want values %#v", name, got, want)
	}
}

func workerByName(workers []workerconfig.Config, name string) (*workerconfig.Config, bool) {
	for index := range workers {
		if workers[index].Name == name {
			return &workers[index], true
		}
	}
	return nil, false
}
