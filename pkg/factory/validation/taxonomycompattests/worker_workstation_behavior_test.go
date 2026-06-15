package validation_test

import (
	"strings"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestPollerRunWorkstationKindTargets_RejectsConflictingBehavior(t *testing.T) {
	t.Parallel()

	cfg := taxonomyCompatibilityBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:    "script-poller",
		Type:    interfaces.WorkerTypeScript,
		Command: "factory/scripts/poll.sh",
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "poll-tasks",
		Type:           interfaces.WorkstationTypePoller,
		Kind:           interfaces.WorkstationKindCron,
		WorkerTypeName: "script-poller",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	targets := factoryvalidation.PollerRunWorkstationKindTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodePollerRunWorkstationKindMismatch)
	if !strings.Contains(targets[0].Message, "POLLER_RUN") || !strings.Contains(targets[0].Message, "poller") {
		t.Fatalf("target message = %q, want POLLER_RUN and poller terminology", targets[0].Message)
	}
}

func TestWorkerWorkstationBehaviorCompatibilityTargets_RejectsIncompatiblePairings(t *testing.T) {
	t.Parallel()

	cfg := taxonomyCompatibilityBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{
		{Name: "infer", Type: interfaces.WorkerTypeInference, Operations: taxonomyInferenceOperationFixture()},
		{Name: "agent", Type: interfaces.WorkerTypeAgent},
	}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{
			Name:           "agent-with-infer",
			Type:           interfaces.WorkstationTypeAgent,
			WorkerTypeName: "infer",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
	}

	targets := factoryvalidation.WorkerWorkstationBehaviorCompatibilityTargets(cfg)
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
	if !strings.Contains(targets[0].Message, "agent-run") || !strings.Contains(targets[0].Message, "INFERENCE_WORKER") {
		t.Fatalf("target message = %q, want agent-run and INFERENCE_WORKER terminology", targets[0].Message)
	}
}

func TestValidate_IncludesWorkerWorkstationBehaviorCompatibilityTargets(t *testing.T) {
	t.Parallel()

	cfg := taxonomyCompatibilityBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{Name: "infer", Type: interfaces.WorkerTypeInference, Operations: taxonomyInferenceOperationFixture()}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "agent-with-infer",
		Type:           interfaces.WorkstationTypeAgent,
		WorkerTypeName: "infer",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
}

func taxonomyCompatibilityBaseConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
	}
}

func taxonomyInferenceOperationFixture() []interfaces.ModelOperation {
	return []interfaces.ModelOperation{{
		Name: "TTS",
		Inputs: []interfaces.ModelOperationSlot{{
			Name:         "text",
			ContentTypes: []string{interfaces.ModelOperationContentTypeText},
		}},
		Outputs: []interfaces.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
		}},
	}}
}
