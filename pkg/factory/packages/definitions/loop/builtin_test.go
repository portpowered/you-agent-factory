package builtinloop_test

import (
	"reflect"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/operatordefaultsruntime"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/loop"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	invocations "github.com/portpowered/infinite-you/pkg/work/invocation"
)

func TestBuiltInLoopFactoryJSON_DeclaresStartupAndSessionRecurringTopology(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	assertLoopStructure(t, cfg)
}

func assertLoopStructure(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if findings := factoryconfig.CanonicalStructuralFindings(cfg); len(findings) != 0 {
		t.Fatalf("structural findings = %#v", findings)
	}
	assertLoopInvocationSignature(t, cfg)
	assertLoopSchedule(t, workstation(t, cfg, "schedule-loop-iteration"))
	if run := workstation(t, cfg, "run-loop-iteration"); run.Worktree != "${worktree}" {
		t.Fatalf("run worktree = %q, want invocation worktree interpolation", run.Worktree)
	}
}

func assertLoopInvocationSignature(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if cfg.Name != "@you/loop" || cfg.Project != "builtin-loop" {
		t.Fatalf("identity = %q/%q, want @you/loop/builtin-loop", cfg.Name, cfg.Project)
	}
	if cfg.InvocationSignature == nil || len(cfg.InvocationSignature.Parameters) != 3 {
		t.Fatalf("invocation signature = %#v, want request, period, and worktree parameters", cfg.InvocationSignature)
	}
	request := cfg.InvocationSignature.Parameters[0]
	if request.Name != "request" || !request.Required {
		t.Fatalf("request parameter = %#v, want required request", request)
	}
}

func assertLoopSchedule(t *testing.T, schedule interfaces.FactoryWorkstationConfig) {
	t.Helper()
	if schedule.Kind != interfaces.WorkstationKindCron || schedule.Cron == nil || schedule.Cron.Schedule != "0 0 31 12 *" || !schedule.Cron.TriggerAtStart {
		t.Fatalf("schedule workstation = %#v, want one-time trigger-at-start cron", schedule)
	}
	if schedule.WorkPropagation == nil || schedule.WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("schedule propagation = %#v, want preserved request input", schedule.WorkPropagation)
	}
	if len(schedule.Outputs) != 2 || schedule.Outputs[0].WorkTypeName != "loop-control" || schedule.Outputs[1].WorkTypeName != "iteration" {
		t.Fatalf("schedule outputs = %#v, want retained control and new iteration", schedule.Outputs)
	}
}

func TestBuiltInLoopFactory_NormalizesHourlyPeriodAndWorktree(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	arguments, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Check the release dashboard"},
		NamedArgs:      []invocations.NamedArgumentInput{{Key: "period", Values: []string{"1h"}}, {Key: "worktree", Values: []string{"release-dashboard"}}},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	if got := arguments.Arguments["period"].Values; !reflect.DeepEqual(got, []string{"1h"}) {
		t.Fatalf("period = %#v, want hourly period", got)
	}
	dailyArguments, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Check the release dashboard"},
		NamedArgs:      []invocations.NamedArgumentInput{{Key: "period", Values: []string{"24h"}}},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments daily period: %v", err)
	}
	if got := dailyArguments.Arguments["period"].Values; !reflect.DeepEqual(got, []string{"24h"}) {
		t.Fatalf("daily period = %#v, want 24h", got)
	}
	if got := arguments.Arguments["worktree"].Values; !reflect.DeepEqual(got, []string{"release-dashboard"}) {
		t.Fatalf("worktree = %#v, want configured worktree", got)
	}
	run, err := invocations.InterpolateWorkstationConfig(
		workstation(t, cfg, "run-loop-iteration"),
		invocations.RuntimeInvocationArguments(cfg.InvocationSignature, &arguments),
		nil,
	)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig: %v", err)
	}
	if run.Worktree != "release-dashboard" {
		t.Fatalf("interpolated run worktree = %q, want configured worktree", run.Worktree)
	}
	defaultArguments, err := invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Check the release dashboard"},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments default period: %v", err)
	}
	if got := defaultArguments.Arguments["period"].Values; !reflect.DeepEqual(got, []string{"1h"}) {
		t.Fatalf("default period = %#v, want hourly period", got)
	}

	_, err = invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Check the release dashboard"},
		NamedArgs:      []invocations.NamedArgumentInput{{Key: "period", Values: []string{"5m"}}},
	})
	if err == nil {
		t.Fatal("NormalizeArguments error = nil, want unsupported period failure")
	}
	if !strings.Contains(err.Error(), `parameter "period" value "5m" is not one of the declared choices`) {
		t.Fatalf("NormalizeArguments unsupported period error = %v", err)
	}

	_, err = invocations.NormalizeArguments(invocations.NormalizeArgumentsInput{
		Signature:      cfg.InvocationSignature,
		PositionalArgs: []string{"Check the release dashboard"},
		NamedArgs:      []invocations.NamedArgumentInput{{Key: "worktree", Values: []string{" "}}},
	})
	if err == nil {
		t.Fatal("NormalizeArguments error = nil, want empty worktree failure")
	}
	if !strings.Contains(err.Error(), `parameter "worktree" path value must not be empty`) {
		t.Fatalf("NormalizeArguments empty worktree error = %v", err)
	}
}

func TestBuiltInLoopFactory_AppliesOperatorModelOverridesWithoutChangingLoopConfiguration(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), cfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CURSOR",
		WorkerModel:         "loop-cursor-model",
	}); err != nil {
		t.Fatalf("ApplyToLoadedConfig: %v", err)
	}

	worker, ok := loaded.Worker("loop-worker")
	if !ok {
		t.Fatal("expected loop worker")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderCursor) {
		t.Fatalf("loop worker model provider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderCursor)
	}
	if worker.Model != "loop-cursor-model" {
		t.Fatalf("loop worker model = %q, want loop-cursor-model", worker.Model)
	}
	if schedule := workstation(t, loaded.FactoryConfig(), "schedule-loop-iteration"); schedule.Cron == nil || schedule.Cron.Schedule != "0 0 31 12 *" {
		t.Fatalf("schedule = %#v, want unchanged startup cron", schedule.Cron)
	}
	if run := workstation(t, loaded.FactoryConfig(), "run-loop-iteration"); run.Worktree != "${worktree}" {
		t.Fatalf("run worktree = %q, want unchanged invocation interpolation", run.Worktree)
	}
}

func TestBuiltInLoopFactory_RejectsUnsupportedOperatorModelProviderBeforeRuntimeDispatch(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), cfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	err = operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "not-a-provider",
	})
	if err == nil {
		t.Fatal("ApplyToLoadedConfig error = nil, want unsupported provider rejection")
	}
	if !strings.Contains(err.Error(), `unsupported worker model provider "not-a-provider"`) {
		t.Fatalf("ApplyToLoadedConfig error = %v, want actionable provider rejection", err)
	}
}

func workstation(t *testing.T, cfg *interfaces.FactoryConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	for _, workstation := range cfg.Workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("missing workstation %q", name)
	return interfaces.FactoryWorkstationConfig{}
}
