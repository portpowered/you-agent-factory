package validation

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateDefinitionTopologyOwnsProfileWithoutCanonicalLoad(t *testing.T) {
	service := New(nil, func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("topology validation must not load canonical JSON")
		return nil, nil
	})

	result, err := service.ValidateDefinition(t.Context(), factorydefinitions.DefinitionValidationRequest{
		Profile: factorydefinitions.ValidationProfileTopology,
		Config:  &factorydefinitions.FactoryConfig{},
		SubmittedTaxonomy: factorydefinitions.SubmittedDefinitionTaxonomy{
			Workers: []factorydefinitions.SubmittedWorkerTaxonomy{{Name: "infer", Type: factorydefinitions.WorkerTypeInference}},
			Workstations: []factorydefinitions.SubmittedWorkstationTaxonomy{{
				Name: "agent", Type: factorydefinitions.WorkstationTypeAgent, Worker: "infer",
			}},
		},
	})
	if err != nil {
		t.Fatalf("ValidateDefinition: %v", err)
	}
	if len(result.Targets) != 1 || result.Targets[0].Code != factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility {
		t.Fatalf("targets = %#v, want owner-computed taxonomy compatibility target", result.Targets)
	}
}

func TestSubmittedTaxonomyCompatibilityTargetsPreservesPublicAliasPolicy(t *testing.T) {
	cases := []struct {
		name, workerType, workstationType string
	}{
		{name: "legacy model pair", workerType: factorydefinitions.WorkerTypeModel, workstationType: factorydefinitions.WorkstationTypeModel},
		{name: "legacy invoke", workerType: factorydefinitions.WorkerTypeInference, workstationType: factorydefinitions.WorkstationTypeInvoke},
		{name: "model worker with agent", workerType: factorydefinitions.WorkerTypeModel, workstationType: factorydefinitions.WorkstationTypeAgent},
		{name: "model worker with inference", workerType: factorydefinitions.WorkerTypeModel, workstationType: factorydefinitions.WorkstationTypeInference},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taxonomy := factorydefinitions.SubmittedDefinitionTaxonomy{
				Workers: []factorydefinitions.SubmittedWorkerTaxonomy{{Name: "worker-a", Type: tc.workerType}},
				Workstations: []factorydefinitions.SubmittedWorkstationTaxonomy{{
					Name: "process", Type: tc.workstationType, Worker: "worker-a",
				}},
			}
			if targets := SubmittedTaxonomyCompatibilityTargets(taxonomy); len(targets) != 0 {
				t.Fatalf("targets = %#v, want alias pairing accepted", targets)
			}
		})
	}
}

func TestSubmittedTaxonomyCompatibilityTargetsReportsPublicMismatch(t *testing.T) {
	taxonomy := factorydefinitions.SubmittedDefinitionTaxonomy{
		Workers: []factorydefinitions.SubmittedWorkerTaxonomy{{Name: "worker-a", Type: factorydefinitions.WorkerTypeAgent}},
		Workstations: []factorydefinitions.SubmittedWorkstationTaxonomy{{
			Name: "process", Type: factorydefinitions.WorkstationTypeInference, Worker: "worker-a", Index: 3,
		}},
	}
	targets := SubmittedTaxonomyCompatibilityTargets(taxonomy)
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one", targets)
	}
	target := targets[0]
	if target.Code != factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility ||
		target.Subject.ID != "process" || target.Path != "factory.workstations[3].worker" {
		t.Fatalf("target = %#v, want owner-created workstation reference", target)
	}
}

func TestValidateEffectiveDefinitionRequiresDefaultHandlingWorkType(t *testing.T) {
	service := New(nil)
	result, err := service.ValidateEffectiveDefinition(t.Context(), factorydefinitions.EffectiveDefinitionValidationRequest{
		Config: &factorydefinitions.FactoryConfig{WorkTypes: []factorydefinitions.WorkTypeConfig{{
			Name: "story",
			States: []factorydefinitions.StateConfig{
				{Name: "init", Type: factorydefinitions.StateTypeInitial},
				{Name: "done", Type: factorydefinitions.StateTypeTerminal},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("ValidateEffectiveDefinition: %v", err)
	}
	if !result.HasBlockingTargets() {
		t.Fatalf("targets = %#v, want required DEFAULT work type finding", result.Targets)
	}
}

func TestDefinitionValidationOperationsRejectMissingAndCanceledContext(t *testing.T) {
	service := New(nil)
	request := factorydefinitions.DefinitionValidationRequest{Config: &factorydefinitions.FactoryConfig{}}
	if _, err := service.ValidateDefinition(nil, request); err == nil {
		t.Fatal("ValidateDefinition(nil) error = nil")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.ValidateDefinition(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateDefinition(canceled) error = %v, want context.Canceled", err)
	}
}

func TestValidateDefinitionPrePersistLoadsThenStopsAtBlockingFindings(t *testing.T) {
	loadCalls := 0
	service := New(nil, func(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		loadCalls++
		if string(payload) != `{}` {
			t.Fatalf("canonical payload = %q, want %q", payload, `{}`)
		}
		return nil, nil
	})
	cfg := &factorydefinitions.FactoryConfig{Workstations: []factorydefinitions.FactoryWorkstationConfig{
		{ID: "duplicate", Name: "first"},
		{ID: "duplicate", Name: "second"},
	}}

	result, err := service.ValidateDefinition(t.Context(), factorydefinitions.DefinitionValidationRequest{
		Profile:          factorydefinitions.ValidationProfilePrePersist,
		Config:           cfg,
		CanonicalPayload: []byte(`{}`),
		SubmittedTaxonomy: factorydefinitions.SubmittedDefinitionTaxonomy{
			Workers: []factorydefinitions.SubmittedWorkerTaxonomy{{Name: "infer", Type: factorydefinitions.WorkerTypeInference}},
			Workstations: []factorydefinitions.SubmittedWorkstationTaxonomy{{
				Name: "agent", Type: factorydefinitions.WorkstationTypeAgent, Worker: "infer",
			}},
		},
	})
	if err != nil {
		t.Fatalf("ValidateDefinition: %v", err)
	}
	if loadCalls != 1 {
		t.Fatalf("canonical load calls = %d, want 1", loadCalls)
	}
	if !result.HasBlockingTargets() {
		t.Fatalf("targets = %#v, want blocking-load finding", result.Targets)
	}
	for _, target := range result.Targets {
		if target.Code == factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility {
			t.Fatalf("topology phase ran after blocking-load finding: %#v", result.Targets)
		}
	}
}

func TestValidateDefinitionPrePersistPreservesCanonicalLoadFailure(t *testing.T) {
	wantErr := errors.New("read canonical payload")
	service := New(nil, func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, wantErr
	})

	_, err := service.ValidateDefinition(t.Context(), factorydefinitions.DefinitionValidationRequest{
		Profile:          factorydefinitions.ValidationProfilePrePersist,
		Config:           &factorydefinitions.FactoryConfig{},
		CanonicalPayload: []byte(`{}`),
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ValidateDefinition error = %v, want %v", err, wantErr)
	}
}

func TestValidateDefinitionPrePersistTurnsInvalidCanonicalLoadIntoBlockingFindings(t *testing.T) {
	service := New(nil, func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, factorydefinitions.ErrInvalidNamedFactory
	})
	cfg := &factorydefinitions.FactoryConfig{Workstations: []factorydefinitions.FactoryWorkstationConfig{
		{ID: "duplicate", Name: "first"},
		{ID: "duplicate", Name: "second"},
	}}

	result, err := service.ValidateDefinition(t.Context(), factorydefinitions.DefinitionValidationRequest{
		Profile:          factorydefinitions.ValidationProfilePrePersist,
		Config:           cfg,
		CanonicalPayload: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("ValidateDefinition: %v", err)
	}
	if !result.HasBlockingTargets() {
		t.Fatalf("targets = %#v, want blocking-load findings", result.Targets)
	}
}

func configTestFactoryDefinitionValidator() factorydefinitions.Validator {
	return New(nil)
}

func ruleWorkerWorkstationBehaviorCompatibility(
	cfg *factorydefinitions.FactoryConfig,
) []Finding {
	return FactoryDefinitionFindings(
		configTestFactoryDefinitionValidator().
			WorkerWorkstationBehaviorCompatibility(context.Background(), cfg),
	)
}

func ruleWorkTypeHandlingBehavior(
	cfg *factorydefinitions.FactoryConfig,
	requireDefault bool,
) []Finding {
	return FactoryDefinitionFindings(
		configTestFactoryDefinitionValidator().
			WorkTypeHandlingBehavior(context.Background(), cfg, requireDefault),
	)
}

func ruleCanonicalFactoryDefinitionValidation(
	cfg *factorydefinitions.FactoryConfig,
) []Finding {
	return FactoryDefinitionFindings(
		configTestFactoryDefinitionValidator().
			Validate(context.Background(), cfg, nil).
			Targets,
	)
}
