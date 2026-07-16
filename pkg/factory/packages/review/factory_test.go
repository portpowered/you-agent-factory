package review

import (
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuiltInFactoryJSON_LoadsReviewGatedTopology(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("identity = %q/%q", cfg.Name, cfg.Project)
	}
	if cfg.InvocationReturn == nil || cfg.InvocationReturn.WorkTypeName != PackagedWorkTypeName || cfg.InvocationReturn.TerminalState != PackagedInvocationReturnTerminalState {
		t.Fatalf("invocationReturn = %#v", cfg.InvocationReturn)
	}
	if len(cfg.Workstations) != 2 || len(cfg.Workers) != 2 {
		t.Fatalf("topology = %#v", cfg)
	}
	execute, executeOK := workstationByName(cfg.Workstations, PackagedExecuteWorkstationName)
	review, reviewOK := workstationByName(cfg.Workstations, PackagedReviewWorkstationName)
	if !executeOK || !reviewOK {
		t.Fatal("missing execute or review workstation")
	}
	if execute.WorkPropagation == nil || execute.WorkPropagation.Mode != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("execute work propagation = %#v, want PRESERVE_INPUT", execute.WorkPropagation)
	}
	if execute.Outputs[0].StateName != "in-review" || review.Outputs[0].StateName != "approved" || review.OnRejection[0].StateName != "init" || review.OnFailure[0].StateName != "failed" {
		t.Fatalf("review routes = %#v", review)
	}
	if review.OutcomeFormat != interfaces.WorkstationOutcomeFormatDecisionEnvelope {
		t.Fatalf("outcomeFormat = %q", review.OutcomeFormat)
	}
	for _, target := range factoryvalidation.Validate(cfg).Targets {
		if target.Severity == factoryvalidation.SeverityError {
			t.Fatalf("validation target = %#v", target)
		}
	}
}

func TestMaterializedFactory_RetainsReviewGate(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	review, ok := workstationByName(loaded.FactoryConfig().Workstations, PackagedReviewWorkstationName)
	if !ok || len(review.OnRejection) != 1 || review.OnRejection[0].StateName != "init" {
		t.Fatalf("materialized review = %#v", review)
	}
}

func workstationByName(workstations []interfaces.FactoryWorkstationConfig, name string) (interfaces.FactoryWorkstationConfig, bool) {
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation, true
		}
	}
	return interfaces.FactoryWorkstationConfig{}, false
}
