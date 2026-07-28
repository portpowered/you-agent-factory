package validation_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPollerRunWorkstationKindTargets_RejectsConflictingBehavior(t *testing.T) {
	t.Parallel()

	cfg := taxonomyCompatibilityBaseConfig()
	cfg.Workers = []workerconfig.Config{{
		Name:    "script-poller",
		Type:    factorydefinitions.WorkerTypeScript,
		Command: "factory/scripts/poll.sh",
	}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "poll-tasks",
		Type:           factorydefinitions.WorkstationTypePoller,
		Kind:           factorydefinitions.WorkstationKindCron,
		WorkerTypeName: "script-poller",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
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
	cfg.Workers = []workerconfig.Config{
		{Name: "infer", Type: factorydefinitions.WorkerTypeInference, Operations: taxonomyInferenceOperationFixture()},
		{Name: "agent", Type: factorydefinitions.WorkerTypeAgent},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{
			Name:           "agent-with-infer",
			Type:           factorydefinitions.WorkstationTypeAgent,
			WorkerTypeName: "infer",
			Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
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
	cfg.Workers = []workerconfig.Config{{Name: "infer", Type: factorydefinitions.WorkerTypeInference, Operations: taxonomyInferenceOperationFixture()}}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "agent-with-infer",
		Type:           factorydefinitions.WorkstationTypeAgent,
		WorkerTypeName: "infer",
		Inputs:         []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []factorydefinitions.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationBehaviorCompatibility)
}

func taxonomyCompatibilityBaseConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
				{Name: "failed", Type: factorydefinitions.StateTypeFailed},
			},
		}},
	}
}

func taxonomyInferenceOperationFixture() []workerconfig.ModelOperation {
	return []workerconfig.ModelOperation{{
		Name: "TTS",
		Inputs: []workerconfig.ModelOperationSlot{{
			Name:         "text",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeText},
		}},
		Outputs: []workerconfig.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
		}},
	}}
}
