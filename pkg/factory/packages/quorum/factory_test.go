package quorum

import (
	"context"
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestBuiltInFactoryJSON_LoadsRunnablePackagedQuorumFactory(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
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

	split := workstations["split-quorum"]
	if !sameRoutes(split.Inputs, []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}}) ||
		!sameRoutes(split.Outputs, []interfaces.IOConfig{{WorkTypeName: "quorum-branch-a", StateName: "init"}, {WorkTypeName: "quorum-branch-b", StateName: "init"}}) {
		t.Fatalf("split routes = %#v, want one request input and two independent branch outputs", split)
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
	wantMergeInputs := []interfaces.IOConfig{{WorkTypeName: "quorum-branch-a", StateName: "complete"}, {WorkTypeName: "quorum-branch-b", StateName: "complete"}}
	if !sameRoutes(merge.Inputs, wantMergeInputs) || !sameRoutes(merge.Outputs, []interfaces.IOConfig{{WorkTypeName: "quorum-merge", StateName: "complete"}}) {
		t.Fatalf("merge routes = %#v, want ordered branch fan-in and one final Work", merge)
	}

	net, err := (&factoryconfig.ConfigMapper{}).Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Map quorum topology: %v", err)
	}
	splitTransition := net.Transitions["split-quorum"]
	if len(splitTransition.OutputArcs) != 2 {
		t.Fatalf("split output arcs = %#v, want two derived Work outputs", splitTransition.OutputArcs)
	}
	transformer := token_transformer.New(net.Places, net.WorkTypes)
	input := []interfaces.TokenColor{{WorkID: "request-work", WorkTypeID: "task", DataType: interfaces.DataTypeWork, RequestID: "request-1"}}
	for index := range splitTransition.OutputArcs {
		output, err := transformer.OutputToken(token_transformer.OutputTokenInput{ArcIndex: index, Arcs: splitTransition.OutputArcs, InputColors: input})
		if err != nil {
			t.Fatalf("derive branch %d: %v", index, err)
		}
		if output.Color.WorkID == "request-work" || output.Color.ParentID != "request-work" {
			t.Fatalf("branch %d lineage = %#v, want a distinct child of request-work", index, output.Color)
		}
		ApplyWorkRelations(output, &split, input)
		if !sameRelations(output.Color.Relations, []interfaces.Relation{{Type: interfaces.RelationParentChild, TargetWorkID: "request-work"}}) {
			t.Fatalf("branch %d relations = %#v, want public parent relation", index, output.Color.Relations)
		}
	}
	mergeTransition := net.Transitions["merge-quorum"]
	if len(mergeTransition.InputArcs) != 2 {
		t.Fatalf("merge input arcs = %#v, want both completed branch Work items before dispatch", mergeTransition.InputArcs)
	}
	mergeOutput := &interfaces.Token{Color: interfaces.TokenColor{WorkTypeID: "quorum-merge"}}
	ApplyWorkRelations(mergeOutput, &merge, []interfaces.TokenColor{{WorkID: "branch-a-work", WorkTypeID: "quorum-branch-a"}, {WorkID: "branch-b-work", WorkTypeID: "quorum-branch-b"}})
	wantDependencies := []interfaces.Relation{{Type: interfaces.RelationDependsOn, TargetWorkID: "branch-a-work", RequiredState: "complete"}, {Type: interfaces.RelationDependsOn, TargetWorkID: "branch-b-work", RequiredState: "complete"}}
	if !sameRelations(mergeOutput.Color.Relations, wantDependencies) {
		t.Fatalf("merge relations = %#v, want dependencies on both branch results", mergeOutput.Color.Relations)
	}
}

func loadQuorumConfig(t *testing.T) *interfaces.FactoryConfig {
	t.Helper()
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
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

func sameRelations(got, want []interfaces.Relation) bool {
	return reflect.DeepEqual(got, want)
}

func TestBuiltInQuorumFactory_DefaultNamedInvocationAcceptsInput(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	got, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
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
	normalized, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
		NamedArgs: []invocations.NamedArgumentInput{
			{Key: "branch-provider", Values: []string{"CLAUDE"}},
			{Key: "branch-model", Values: []string{"claude-sonnet-4-20250514"}},
			{Key: "merge-provider", Values: []string{"CODEX"}},
			{Key: "merge-model", Values: []string{"gpt-5"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	args := invocations.RuntimeInvocationArguments(cfg.InvocationSignature, &normalized)
	for _, workerName := range []string{"quorum-branch-a", "quorum-branch-b"} {
		worker, ok := workerByName(cfg.Workers, workerName)
		if !ok {
			t.Fatalf("worker %q missing", workerName)
		}
		effective, err := invocations.InterpolateWorkerConfig(*worker, args, nil)
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
	effectiveMerge, err := invocations.InterpolateWorkerConfig(*merge, args, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkerConfig(merge): %v", err)
	}
	if effectiveMerge.ModelProvider != "CODEX" || effectiveMerge.Model != "gpt-5" {
		t.Fatalf("effective merge worker = %#v, want configured merge provider/model", effectiveMerge)
	}
	diagnostic := invocations.InvocationDiagnostic(cfg.InvocationSignature, args)
	if diagnostic == nil || len(diagnostic.Parameters) != 5 {
		t.Fatalf("invocation diagnostic = %#v, want observable effective invocation parameters", diagnostic)
	}
}

func TestBuiltInQuorumFactory_RejectsUnsupportedRoleProvider(t *testing.T) {
	cfg := loadQuorumConfig(t)
	_, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Compare two release plans."},
		NamedArgs:      []invocations.NamedArgumentInput{{Key: "branch-provider", Values: []string{"unsupported"}}},
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

func assertArgumentValues(t *testing.T, arguments map[string]invocations.NormalizedArgument, name string, want []string) {
	t.Helper()
	got, ok := arguments[name]
	if !ok || !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("argument %q = %#v, want values %#v", name, got, want)
	}
}

func workerByName(workers []interfaces.WorkerConfig, name string) (*interfaces.WorkerConfig, bool) {
	for index := range workers {
		if workers[index].Name == name {
			return &workers[index], true
		}
	}
	return nil, false
}
