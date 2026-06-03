package validation_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestValidate_EquivalentTargetsForInvalidFactoryThroughConfigAndPackageValidation(t *testing.T) {
	t.Parallel()

	apiFactory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	cfg, err := config.FactoryConfigFromOpenAPI(apiFactory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	configFindings := config.CanonicalStructuralFindings(&cfg)

	if len(explicit.Targets) == 0 || len(configFindings) == 0 {
		t.Fatalf("explicit targets = %d, config findings = %d, want both non-empty", len(explicit.Targets), len(configFindings))
	}
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, explicit targets = %d, want equivalent coverage", len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("finding[%d].Rule = %q, want explicit target code %q", index, configFindings[index].Rule, target.Code)
		}
	}

	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDuplicateIdentifier)
	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDanglingWorkerReference)
	validationassert.HasDomainTargetCode(t, explicit.Targets, factoryvalidation.CodeDanglingPlaceReference)
	validationassert.HasDomainTargetSubject(t, explicit.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "process",
		Location: factoryvalidation.SubjectLocationReference,
	})
}

func TestValidate_MissingOutputRoutesVsMissingFailureRoute(t *testing.T) {
	t.Parallel()

	baseWorkTypes := []interfaces.WorkTypeConfig{{
		Name: "task",
		States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "in-review", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
			{Name: "failed", Type: interfaces.StateTypeFailed},
		},
	}}
	baseWorkers := []interfaces.WorkerConfig{{Name: "worker-a"}}

	tests := []struct {
		name                  string
		workTypes             []interfaces.WorkTypeConfig
		workstation           interfaces.FactoryWorkstationConfig
		wantCode              string
		wantLocation          factoryvalidation.SubjectLocation
		wantPathSuffix        string
		forbiddenCode         string
		forbiddenCodeLocation factoryvalidation.SubjectLocation
	}{
		{
			name: "routeless_cron_empty_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "cron",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "worker-a",
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingOutputRoutes,
			wantLocation:          factoryvalidation.SubjectLocationOutputs,
			wantPathSuffix:        "factory.workstations[0].outputs",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingFailureRoute,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOnFailure,
		},
		{
			name: "classification_routes_without_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "classifier",
				Type:           interfaces.WorkstationTypeClassify,
				WorkerTypeName: "worker-a",
				ClassificationRoutes: []interfaces.ClassificationRouteConfig{
					{Label: "approved", Outputs: []interfaces.IOConfig{}},
					{Label: "rejected"},
				},
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingOutputRoutes,
			wantLocation:          factoryvalidation.SubjectLocationOutputs,
			wantPathSuffix:        "factory.workstations[0].outputs",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingFailureRoute,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOnFailure,
		},
		{
			name: "on_continue_only_not_effective_output",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "repeater",
				Kind:           interfaces.WorkstationKindRepeater,
				WorkerTypeName: "worker-a",
				OnContinue:     []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingOutputRoutes,
			wantLocation:          factoryvalidation.SubjectLocationOutputs,
			wantPathSuffix:        "factory.workstations[0].outputs",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingFailureRoute,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOnFailure,
		},
		{
			name: "outputs_without_defaultable_failure_route",
			workTypes: []interfaces.WorkTypeConfig{{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "in-review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			}},
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "repeater",
				Kind:           interfaces.WorkstationKindRepeater,
				WorkerTypeName: "worker-a",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingFailureRoute,
			wantLocation:          factoryvalidation.SubjectLocationOnFailure,
			wantPathSuffix:        "factory.workstations[0].onFailure",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingOutputRoutes,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOutputs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			workTypes := tt.workTypes
			if len(workTypes) == 0 {
				workTypes = baseWorkTypes
			}
			cfg := &interfaces.FactoryConfig{
				WorkTypes:    workTypes,
				Workers:      baseWorkers,
				Workstations: []interfaces.FactoryWorkstationConfig{tt.workstation},
			}

			result := factoryvalidation.Validate(cfg)
			validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
				Type:     factoryvalidation.SubjectTypeWorkstation,
				ID:       tt.workstation.Name,
				Location: tt.wantLocation,
			})
			assertWorkstationTarget(t, result.Targets, tt.workstation.Name, tt.wantCode, tt.wantLocation, tt.wantPathSuffix)
			assertWorkstationTargetAbsent(
				t,
				result.Targets,
				tt.workstation.Name,
				tt.forbiddenCode,
				tt.forbiddenCodeLocation,
			)
		})
	}
}

func assertWorkstationTarget(
	t *testing.T,
	targets []factoryvalidation.Target,
	workstationID string,
	code string,
	location factoryvalidation.SubjectLocation,
	pathSuffix string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code || target.Subject.ID != workstationID || target.Subject.Location != location {
			continue
		}
		if target.Path != pathSuffix {
			t.Fatalf("target path = %q, want %q (target %#v)", target.Path, pathSuffix, target)
		}
		return
	}
	t.Fatalf("targets = %#v, want %q for workstation %q at %q with path %q", targets, code, workstationID, location, pathSuffix)
}

func assertWorkstationTargetAbsent(
	t *testing.T,
	targets []factoryvalidation.Target,
	workstationID string,
	code string,
	location factoryvalidation.SubjectLocation,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code &&
			target.Subject.ID == workstationID &&
			target.Subject.Location == location {
			t.Fatalf("workstation %q must not receive %q at %q, got %#v", workstationID, code, location, target)
		}
	}
}

func TestValidate_MissingOutcomeRoutesUseCanonicalWorkstationLocations(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "in-review", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "repeater",
			Kind:           interfaces.WorkstationKindRepeater,
			WorkerTypeName: "worker-a",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "repeater",
		Location: factoryvalidation.SubjectLocationOnFailure,
	})
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "repeater",
		Location: factoryvalidation.SubjectLocationOnRejection,
	})
}

func TestValidate_RepresentativeCanonicalSubjects(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			Kind:           interfaces.WorkstationKindRepeater,
			WorkerTypeName: "worker-a",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "process",
		Location: factoryvalidation.SubjectLocationOnRejection,
	})
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkType,
		ID:       "task",
		Location: factoryvalidation.SubjectLocationStates,
	})
}

func TestValidate_MissingWorkTypeOutcomeStates(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "queued", Type: interfaces.StateTypeInitial},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "worker-a"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process",
			WorkerTypeName: "worker-a",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeMissingCompletionState)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeMissingFailureState)
}

func TestWorkTypeHandlingBehaviorTargets_RejectsMultipleDefaultWorkTypes(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		},
	}

	targets := factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{})
	validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
}

func TestWorkTypeHandlingBehaviorTargets_AllowsSingleDefaultWorkType(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task"},
		},
	}

	targets := factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{})
	if len(targets) != 0 {
		t.Fatalf("targets = %#v, want no handlingBehavior findings", targets)
	}
}

func TestValidate_RejectsDuplicateDefaultWorkTypesOnSavePath(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
			{Name: "task", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}},
		},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
}
