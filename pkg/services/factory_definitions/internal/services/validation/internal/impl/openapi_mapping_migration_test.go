package impl

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryConfigFromOpenAPIJSON_RejectsNonClassifierWithoutOutputsDuringValidation(
	t *testing.T,
) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		OnFailure: []factorydefinitions.IOConfig{{
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []factorydefinitions.IOConfig{{
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
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "process-task",
		Type:           factorydefinitions.WorkstationTypeModel,
		WorkerTypeName: "executor",
		Inputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
		Outputs: []factorydefinitions.IOConfig{{
			WorkTypeName: "task",
			StateName:    "done",
		}},
		ClassificationRoutes: []factorydefinitions.ClassificationRouteConfig{{
			Label: "approved",
			Outputs: []factorydefinitions.IOConfig{{
				WorkTypeName: "task",
				StateName:    "done",
			}},
		}},
		OnFailure: []factorydefinitions.IOConfig{{
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
	cfg.Workers = []factorydefinitions.FactoryWorkerConfig{{
		Name:     "linear-poller",
		Type:     factorydefinitions.WorkerTypeHosted,
		Provider: factorydefinitions.HostedWorkerProviderLinear,
		Auth:     &factorydefinitions.HostedWorkerAuthConfig{SecretRef: "secrets/linear-api-key"},
		Linear:   &factorydefinitions.HostedLinearWorkerConfig{},
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
