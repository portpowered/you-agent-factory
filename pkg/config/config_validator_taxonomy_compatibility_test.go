package config

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestRuleWorkerWorkstationBehaviorCompatibility_AcceptsCompatiblePairings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{
		{Name: "infer", Type: interfaces.WorkerTypeInference, Operations: inferenceOperationFixture()},
		{Name: "legacy-infer", Type: interfaces.WorkerTypeModel, Operations: inferenceOperationFixture()},
		{Name: "agent", Type: interfaces.WorkerTypeAgent},
		{Name: "legacy-agent", Type: interfaces.WorkerTypeModel},
		{Name: "script", Type: interfaces.WorkerTypeScript},
		{Name: "poller", Type: interfaces.WorkerTypePoller, Provider: interfaces.HostedWorkerProviderLinear},
	}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{
			Name:           "infer-run",
			Type:           interfaces.WorkstationTypeInference,
			Operation:      "TTS",
			WorkerTypeName: "infer",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "legacy-infer-run",
			Type:           interfaces.WorkstationTypeInvoke,
			Operation:      "TTS",
			WorkerTypeName: "legacy-infer",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "agent-run",
			Type:           interfaces.WorkstationTypeAgent,
			WorkerTypeName: "agent",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "legacy-agent-run",
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "legacy-agent",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "script-run",
			Type:           interfaces.WorkstationTypeScript,
			WorkerTypeName: "script",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "poller-run",
			Type:           interfaces.WorkstationTypePoller,
			WorkerTypeName: "poller",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
	}

	findings := ruleWorkerWorkstationBehaviorCompatibility(cfg)
	for _, finding := range findings {
		if finding.Rule == "workstation-worker-behavior-compatibility" {
			t.Fatalf("unexpected compatibility finding: %+v", finding)
		}
	}
}

func TestRuleWorkerWorkstationBehaviorCompatibility_RejectsIncompatiblePairings(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{
		{Name: "infer", Type: interfaces.WorkerTypeInference, Operations: inferenceOperationFixture()},
		{Name: "agent", Type: interfaces.WorkerTypeAgent},
		{Name: "script", Type: interfaces.WorkerTypeScript},
	}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{
		{
			Name:           "agent-with-infer",
			Type:           interfaces.WorkstationTypeAgent,
			WorkerTypeName: "infer",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "infer-with-agent",
			Type:           interfaces.WorkstationTypeInference,
			Operation:      "TTS",
			WorkerTypeName: "agent",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
		{
			Name:           "poller-with-agent",
			Type:           interfaces.WorkstationTypePoller,
			WorkerTypeName: "agent",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		},
	}

	findings := ruleWorkerWorkstationBehaviorCompatibility(cfg)
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[0](agent-with-infer).worker", "agent-run")
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[1](infer-with-agent).worker", "inference-run")
	assertFindingMatch(t, findings, "workstation-worker-behavior-compatibility", "workstations[2](poller-with-agent).worker", "poller-run")
}

func TestConfigValidator_LegacyModelWorkstationPairingRemainsValid(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.WorkerConfig{{
		Name:            "planner",
		Type:            interfaces.WorkerTypeModel,
		ModelProvider:   "CLAUDE",
		ExecutorProvider: "SCRIPT_WRAP",
	}}
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "plan-task",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "planner",
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
	}}

	result := NewConfigValidator().Validate(cfg)
	for _, finding := range result.Findings {
		if finding.Rule == "workstation-worker-behavior-compatibility" {
			t.Fatalf("legacy MODEL_WORKSTATION + MODEL_WORKER should remain valid, got %+v", finding)
		}
	}
}

func inferenceOperationFixture() []interfaces.ModelOperation {
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
