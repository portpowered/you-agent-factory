package validation

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryConfigFromOpenAPIJSON_RejectsNonClassifierWithoutOutputsDuringValidation(
	t *testing.T,
) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		OnFailure: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "failed",
		}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "workstation-outputs")
}

func TestFactoryConfigFromOpenAPIJSON_AllowsMissingOnFailureWhenSuccessRoutingIsExplicit(
	t *testing.T,
) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "done",
		}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected validator to allow omitted onFailure when success routing is explicit, got %#v", findings)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsNonClassifierClassificationRoutesDuringValidation(
	t *testing.T,
) {
	cfg := testBaseConfig()
	cfg.Workstations = []interfaces.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "done",
		}},
		ClassificationRoutes: []interfaces.ClassificationRouteConfig{{
			Label: "approved",
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "task",
				StateName:    "done",
			}},
		}},
		OnFailure: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "failed",
		}},
	}}

	findings := ruleClassifierWorkstations(cfg)
	assertFindingExists(t, findings, "workstation-classification-routes")
}

func TestFactoryConfigFromOpenAPIJSON_RejectsHostedLinearWorkerMissingMappingWithoutPanic(
	t *testing.T,
) {
	cfg := testBaseConfig()
	cfg.Workers = []interfaces.FactoryWorkerConfig{{
		Name:     "linear-poller",
		Type:     interfaces.WorkerTypeHosted,
		Provider: interfaces.HostedWorkerProviderLinear,
		Auth:     &interfaces.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear:   &interfaces.HostedLinearWorkerConfig{},
	}}

	findings := ruleHostedWorkers(cfg)
	assertFindingMatch(
		t,
		findings,
		"hosted-worker-linear-mapping-work-type",
		"workers[0](linear-poller).linear.mapping.workType",
		"mapping.workType",
	)
	assertFindingMatch(
		t,
		findings,
		"hosted-worker-linear-mapping-state",
		"workers[0](linear-poller).linear.mapping.state",
		"mapping.state",
	)
}
