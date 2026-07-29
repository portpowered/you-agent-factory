package impl

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestWorkerModelProviderTargetsRequiresACPIntegrationIdentity(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name:             "implementer",
		Type:             factorydefinitions.WorkerTypeModel,
		ExecutorProvider: "ACP",
	}}}

	targets := workerModelProviderTargets(cfg)
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one", targets)
	}
	if targets[0].Code != CodeWorkerACPModelProviderRequired || targets[0].Path != "factory.workers[0](implementer).modelProvider" {
		t.Fatalf("target = %#v, want ACP modelProvider diagnostic", targets[0])
	}
}

func TestWorkerModelProviderTargetsAcceptsACPIntegrationIdentity(t *testing.T) {
	cfg := &factorydefinitions.FactoryConfig{Workers: []factorydefinitions.FactoryWorkerConfig{{
		Name:             "implementer",
		Type:             factorydefinitions.WorkerTypeModel,
		ExecutorProvider: "ACP",
		ModelProvider:    "cursor-acp",
	}}}

	if targets := workerModelProviderTargets(cfg); len(targets) != 0 {
		t.Fatalf("targets = %#v, want none", targets)
	}
}
