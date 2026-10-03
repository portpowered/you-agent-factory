package impl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestWorkstationOutputSchemaTargetsRejectInvalidJSONSchema(t *testing.T) {
	cfg := testBaseConfig()
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{{
		Name:           "review",
		WorkerTypeName: "w1",
		OutputSchema:   `{"type":`,
	}}

	targets := workstationOutputSchemaTargets(cfg)
	if len(targets) != 1 {
		t.Fatalf("output schema targets = %#v, want one invalid-schema target", targets)
	}
	if targets[0].Code != CodeWorkstationInvalidOutputSchema || targets[0].Severity != SeverityError {
		t.Fatalf("invalid output schema target = %#v, want typed error", targets[0])
	}
	if !strings.Contains(targets[0].Path, "workstations[0](review).outputSchema") {
		t.Fatalf("invalid output schema path = %q, want workstation outputSchema path", targets[0].Path)
	}
}

func TestWorkstationOutputSchemaTargetsAllowValidSchemasAndDeferredReferences(t *testing.T) {
	cfg := testBaseConfig()
	cfg.InvocationSignature = &factorydefinitions.InvocationSignatureConfig{
		Parameters: []factorydefinitions.InvocationParameterConfig{{Name: "schema"}},
	}
	cfg.Workstations = []factorydefinitions.FactoryWorkstationConfig{
		{Name: "valid", OutputSchema: `{"type":"array","items":{"type":"string"}}`},
		{Name: "deferred", OutputSchema: `${schema}`},
		{Name: "legacy", OutputSchema: "schema.json"},
	}

	if targets := workstationOutputSchemaTargets(cfg); len(targets) != 0 {
		t.Fatalf("valid/deferred output schema targets = %#v, want none", targets)
	}
}

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

// The canonical loader belongs to the completed validation owner. Request data
// must not select a different effect, including after cancellation or failure.
func TestDefinitionValidationUsesInjectedCanonicalLoader(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"owned"}`)
	cfg := testBaseConfig()
	calls := 0
	workstationLoader := &validationWorkstationLoader{}
	validator := New(nil, func(got []byte, loader factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		calls++
		if !bytes.Equal(got, payload) || loader != workstationLoader {
			t.Fatalf("canonical load = %q, %v; want supplied payload and workstation loader", got, loader)
		}
		return nil, nil
	})
	if calls != 0 {
		t.Fatal("construction loaded canonical data")
	}
	request := factorydefinitions.DefinitionValidationRequest{
		Profile: factorydefinitions.ValidationProfilePrePersist,
		Config:  cfg, CanonicalPayload: payload, WorkstationLoader: workstationLoader,
		CanonicalFactoryLoader: func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			t.Fatal("request substituted the injected loader")
			return nil, nil
		},
	}
	result, err := validator.ValidateDefinition(context.Background(), request)
	if err != nil || result.HasTargets() || calls != 1 {
		t.Fatalf("ValidateDefinition = %+v, %v; canonical calls = %d", result, err, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = validator.ValidateDefinition(ctx, request)
	if !errors.Is(err, context.Canceled) || len(result.Targets) != 0 || calls != 1 {
		t.Fatalf("canceled validation = %+v, %v; canonical calls = %d", result, err, calls)
	}
	request.CanonicalPayload = nil
	result, err = validator.ValidateDefinition(context.Background(), request)
	if err == nil || err.Error() != "canonical Factory payload is required for pre-persist validation" || len(result.Targets) != 0 || calls != 1 {
		t.Fatalf("missing payload validation = %+v, %v; canonical calls = %d", result, err, calls)
	}
}

func TestDefinitionValidationPreservesInjectedLoaderFailure(t *testing.T) {
	t.Parallel()
	cause := errors.New("canonical read failed")
	validator := New(nil, func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, cause
	})
	result, err := validator.ValidateDefinition(context.Background(), factorydefinitions.DefinitionValidationRequest{
		Profile: factorydefinitions.ValidationProfilePrePersist,
		Config:  testBaseConfig(), CanonicalPayload: []byte(`{}`),
	})
	if !errors.Is(err, cause) || len(result.Targets) != 0 {
		t.Fatalf("loader failure = %+v, %v; want original cause and no partial findings", result, err)
	}
}

func TestDefinitionValidationPreservesBlockingFindingsAfterInvalidLoad(t *testing.T) {
	t.Parallel()
	cfg := testBaseConfig()
	cfg.Orchestrator = &factorydefinitions.FactoryOrchestratorConfig{Kind: "LEGACY"}
	validator := New(nil, func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, factorydefinitions.ErrInvalidNamedFactory
	})
	result, err := validator.ValidateDefinition(context.Background(), factorydefinitions.DefinitionValidationRequest{
		Profile: factorydefinitions.ValidationProfilePrePersist,
		Config:  cfg, CanonicalPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("invalid load = %v, want typed blocking findings", err)
	}
	for _, target := range result.Targets {
		if target.Code == CodeOrchestratorUnsupportedKind && target.Severity == factorydefinitions.ValidationSeverityError && target.Subject.Type == factorydefinitions.ValidationSubjectTypeFactory {
			return
		}
	}
	t.Fatalf("blocking findings = %+v; want typed unsupported orchestrator diagnostic", result)
}

type validationWorkstationLoader struct{}

func (*validationWorkstationLoader) Load(string) (*factorydefinitions.FactoryWorkstationConfig, error) {
	return nil, nil
}
