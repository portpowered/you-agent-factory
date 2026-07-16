package fix

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	"github.com/portpowered/infinite-you/pkg/workers/worktree"
	"go.uber.org/zap"
)

func TestBuiltInFactoryJSON_ModelsIsolatedPlanImplementReviewLoop(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("factory identity = %q/%q, want %q/%q", cfg.Name, cfg.Project, PackagedFactoryName, PackagedFactoryProject)
	}
	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != PackagedWorkTypeName {
		t.Fatalf("workTypes = %#v, want one %q work type", cfg.WorkTypes, PackagedWorkTypeName)
	}
	if cfg.InvocationReturn == nil || cfg.InvocationReturn.WorkTypeName != PackagedWorkTypeName || cfg.InvocationReturn.TerminalState != "complete" {
		t.Fatalf("invocationReturn = %#v, want explicit fix:complete", cfg.InvocationReturn)
	}

	workstations := map[string]interfaces.FactoryWorkstationConfig{}
	for _, workstation := range cfg.Workstations {
		workstations[workstation.Name] = workstation
	}
	for _, name := range []string{PackagedPlanWorkstationName, PackagedImplementWorkstationName, PackagedReviewWorkstationName} {
		workstation, ok := workstations[name]
		if !ok {
			t.Fatalf("missing workstation %q", name)
		}
		if workstation.Worktree != "${worktree}-{{ (index .Inputs 0).TraceID }}" {
			t.Fatalf("%s worktree = %q, want invocation-selected isolated worktree", name, workstation.Worktree)
		}
	}
	assertFixWorktreeInvocation(t, cfg, workstations)
	for _, worker := range cfg.Workers {
		if worker.ModelProvider != "" || worker.Model != "" {
			t.Fatalf("worker %q has fixed model selection %#v, want operator-configurable fields", worker.Name, worker)
		}
	}
	if workstations[PackagedImplementWorkstationName].Kind != interfaces.WorkstationKindRepeater {
		t.Fatalf("implement workstation kind = %q, want repeater", workstations[PackagedImplementWorkstationName].Kind)
	}
	assertFixRoute(t, workstations[PackagedReviewWorkstationName].OnRejection, "implement")
	assertFixRoute(t, workstations[PackagedReviewWorkstationName].Outputs, "complete")

	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v", target)
		}
	}
}

func TestBuiltInFixFactory_AppliesOperatorModelSelectionToEveryStage(t *testing.T) {
	globalRoot := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(globalRoot, PackagedFactoryName, BuiltInFactoryJSON); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	resolution, err := factoryconfig.ResolveNamedFactoryAcrossRoots(t.TempDir(), globalRoot, PackagedFactoryName)
	if err != nil {
		t.Fatalf("ResolveNamedFactoryAcrossRoots: %v", err)
	}
	application, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{})
	if err != nil {
		t.Fatalf("workerapplication.New: %v", err)
	}
	builder, err := runtimebuild.New(runtimebuild.Config{
		ApplyOperatorDefaults: true,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CLAUDE",
			WorkerModel:         "claude-sonnet-4-20250514",
		},
		WorkerApplication: application,
	}, factory.EnsureClock(nil), zap.NewNop(), func(context.Context, runtimebuild.SessionBuildSpec) (any, error) {
		return struct{}{}, nil
	})
	if err != nil {
		t.Fatalf("runtimebuild.New: %v", err)
	}
	spec, err := builder.BuildSpec(context.Background(), runtimebuild.SessionSpecInput{
		Dir: resolution.FactoryDir, FolderPath: resolution.FactoryDir, SessionID: "~default", ExecutionBaseDir: resolution.FactoryDir,
	})
	if err != nil {
		t.Fatalf("BuildSpec: %v", err)
	}
	for _, name := range []string{"fix-planner", "fix-implementer", "fix-reviewer"} {
		worker, ok := spec.LoadedFactoryCfg.Worker(name)
		if !ok {
			t.Fatalf("worker %q missing", name)
		}
		if worker.ModelProvider != "claude" || worker.Model != "claude-sonnet-4-20250514" {
			t.Fatalf("worker %q selection = %q/%q, want claude/claude-sonnet-4-20250514", name, worker.ModelProvider, worker.Model)
		}
	}
}

func assertFixWorktreeInvocation(t *testing.T, cfg *interfaces.FactoryConfig, workstations map[string]interfaces.FactoryWorkstationConfig) {
	t.Helper()
	var worktreeParameter *interfaces.InvocationParameterConfig
	for i := range cfg.InvocationSignature.Parameters {
		parameter := &cfg.InvocationSignature.Parameters[i]
		if parameter.Name == "worktree" {
			worktreeParameter = parameter
			break
		}
	}
	if worktreeParameter == nil || worktreeParameter.DefaultValue != "fix" {
		t.Fatalf("worktree invocation parameter = %#v, want fix prefix default", worktreeParameter)
	}
	normalized, err := workinvocation.NormalizeArguments(workinvocation.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"repair the login retry regression"},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments(default worktree): %v", err)
	}
	if got := normalized.Arguments["worktree"].Values; len(got) != 1 || got[0] != "fix" {
		t.Fatalf("default worktree argument = %#v, want [fix]", got)
	}

	for _, name := range []string{PackagedPlanWorkstationName, PackagedImplementWorkstationName, PackagedReviewWorkstationName} {
		resolved, err := workinvocation.InterpolateWorkstationConfig(workstations[name], &interfaces.InvocationArguments{
			Arguments: map[string]interfaces.InvocationArgument{"worktree": {Values: []string{"customer-fix"}}},
		}, nil)
		if err != nil {
			t.Fatalf("InterpolateWorkstationConfig(%s): %v", name, err)
		}
		if resolved.Worktree != "customer-fix-{{ (index .Inputs 0).TraceID }}" {
			t.Fatalf("%s resolved worktree = %q, want customer prefix plus trace", name, resolved.Worktree)
		}
	}

	if _, err := worktree.ResolveFactoryWorktreeCheckoutPath(t.TempDir(), "../escape"); err == nil {
		t.Fatal("invalid requested worktree name was accepted")
	}
}

func assertFixRoute(t *testing.T, routes []interfaces.IOConfig, state string) {
	t.Helper()
	for _, route := range routes {
		if route.WorkTypeName == PackagedWorkTypeName && route.StateName == state {
			return
		}
	}
	t.Fatalf("routes = %#v, want %s:%s", routes, PackagedWorkTypeName, state)
}
