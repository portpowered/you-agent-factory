package builtinloop_test

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/definitions/loop"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInLoopFactoryJSON_DeclaresHourlyRecurringTopology(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinloop.BuiltInLoopFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if findings := factoryconfig.CanonicalStructuralFindings(cfg); len(findings) != 0 {
		t.Fatalf("structural findings = %#v", findings)
	}
	if cfg.Name != "@you/loop" || cfg.Project != "builtin-loop" {
		t.Fatalf("identity = %q/%q, want @you/loop/builtin-loop", cfg.Name, cfg.Project)
	}
	if cfg.InvocationSignature == nil || len(cfg.InvocationSignature.Parameters) != 1 {
		t.Fatalf("invocation signature = %#v, want one request parameter", cfg.InvocationSignature)
	}
	request := cfg.InvocationSignature.Parameters[0]
	if request.Name != "request" || !request.Required {
		t.Fatalf("request parameter = %#v, want required request", request)
	}
	schedule := workstation(t, cfg, "schedule-loop-iteration")
	if schedule.Kind != interfaces.WorkstationKindCron || schedule.Cron == nil || schedule.Cron.Schedule != "0 * * * *" || !schedule.Cron.TriggerAtStart {
		t.Fatalf("schedule workstation = %#v, want hourly trigger-at-start cron", schedule)
	}
	if schedule.WorkPropagation == nil || schedule.WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("schedule propagation = %#v, want preserved request input", schedule.WorkPropagation)
	}
	if len(schedule.Outputs) != 2 || schedule.Outputs[0].WorkTypeName != "loop-control" || schedule.Outputs[1].WorkTypeName != "iteration" {
		t.Fatalf("schedule outputs = %#v, want retained control and new iteration", schedule.Outputs)
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
