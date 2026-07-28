package blockingloadtests

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateBlockingLoad_RejectsCrossPathInvalidWithoutOutcomeRouteOnlyFindings(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	result := factoryvalidation.ValidateBlockingLoad(&cfg)
	if len(result.Targets) == 0 {
		t.Fatal("expected blocking load targets for cross-path invalid fixture")
	}
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute ||
			target.Code == factoryvalidation.CodeWorkstationMissingRejectionRoute {
			t.Fatalf("unexpected deferred outcome-route target %#v", target)
		}
	}
}

func TestValidateBlockingLoad_DoesNotDeferMissingOutputRoutes(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "task",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "complete", Type: factorydefinitions.StateTypeTerminal},
			},
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "consume",
		}},
	}

	result := factoryvalidation.ValidateBlockingLoad(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkstationMissingOutputRoutes)
}

func TestValidateBlockingLoad_AllowsNamedFactoryFixture(t *testing.T) {
	t.Parallel()

	loaded, err := factorydefinitioncomposition.LoadCanonicalJSON([]byte(`{
		"name": "alpha",
		"id": "alpha",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","body":"You are the executor."}],
		"workstations": [{"name":"execute-alpha","worker":"executor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Implement."}]
	}`), nil)
	if err != nil {
		t.Fatalf("LoadFromCanonicalJSON: %v", err)
	}

	result := factoryvalidation.ValidateBlockingLoad(loaded.FactoryConfig())
	if len(result.Targets) != 0 {
		t.Fatalf("blocking load targets = %#v, want none for named factory fixture", result.Targets)
	}
}
