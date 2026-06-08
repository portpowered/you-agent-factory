package validation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

func TestValidate_RejectsDuplicateExplicitEntityIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		cfg                 *interfaces.FactoryConfig
		wantSubjectType     factoryvalidation.SubjectType
		wantSubjectID       string
		wantMessageContains string
		wantPaths           []string
	}{
		{
			name: "resource_ids",
			cfg: stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
				cfg.Resources = append(cfg.Resources, interfaces.ResourceConfig{
					ID:       "resource-agent-slot",
					Name:     "review-agent-slot",
					Capacity: 1,
				})
			}),
			wantSubjectType:     factoryvalidation.SubjectTypeResource,
			wantSubjectID:       "resource-agent-slot",
			wantMessageContains: `duplicate resource id "resource-agent-slot"`,
			wantPaths:           []string{"factory.resources[0].id", "factory.resources[1].id"},
		},
		{
			name: "worker_ids",
			cfg: stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
				cfg.Workers = append(cfg.Workers, interfaces.WorkerConfig{
					ID:   "worker-executor",
					Name: "reviewer",
				})
			}),
			wantSubjectType:     factoryvalidation.SubjectTypeWorker,
			wantSubjectID:       "worker-executor",
			wantMessageContains: `duplicate worker id "worker-executor"`,
			wantPaths:           []string{"factory.workers[0].id", "factory.workers[1].id"},
		},
		{
			name: "work_type_ids",
			cfg: stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
				cfg.WorkTypes = append(cfg.WorkTypes, interfaces.WorkTypeConfig{
					ID:   "work-type-story",
					Name: "bug",
					States: []interfaces.StateConfig{
						{Name: "init", Type: interfaces.StateTypeInitial},
						{Name: "complete", Type: interfaces.StateTypeTerminal},
						{Name: "failed", Type: interfaces.StateTypeFailed},
					},
				})
			}),
			wantSubjectType:     factoryvalidation.SubjectTypeWorkType,
			wantSubjectID:       "work-type-story",
			wantMessageContains: `duplicate work type id "work-type-story"`,
			wantPaths:           []string{"factory.workTypes[0].id", "factory.workTypes[1].id"},
		},
		{
			name: "workstation_ids",
			cfg: stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
				cfg.Workstations = append(cfg.Workstations, interfaces.FactoryWorkstationConfig{
					ID:             "workstation-execute-story",
					Name:           "review-story",
					WorkerTypeName: "executor",
					Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
					Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
					OnFailure:      []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
				})
			}),
			wantSubjectType:     factoryvalidation.SubjectTypeWorkstation,
			wantSubjectID:       "workstation-execute-story",
			wantMessageContains: `duplicate workstation id "workstation-execute-story"`,
			wantPaths:           []string{"factory.workstations[0].id", "factory.workstations[1].id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := factoryvalidation.Validate(tt.cfg)
			assertDuplicateExplicitIDTargets(
				t,
				result.Targets,
				tt.wantSubjectType,
				tt.wantSubjectID,
				tt.wantMessageContains,
				tt.wantPaths,
			)
		})
	}
}

func TestValidate_RejectsDuplicateWorkStateIDsWithinWorkType(t *testing.T) {
	t.Parallel()

	cfg := stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.WorkTypes[0].States[1].ID = "state-ready"
	})

	result := factoryvalidation.Validate(cfg)
	assertDuplicateExplicitIDTargets(
		t,
		result.Targets,
		factoryvalidation.SubjectTypeWorkState,
		"work-type-story:state-ready",
		`duplicate work state id "state-ready" on work type "story"`,
		[]string{"factory.workTypes[0].states[0].id", "factory.workTypes[0].states[1].id"},
	)
}

func TestValidate_AllowsSameWorkStateIDAcrossDifferentWorkTypes(t *testing.T) {
	t.Parallel()

	cfg := stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.WorkTypes = append(cfg.WorkTypes, interfaces.WorkTypeConfig{
			ID:   "work-type-bug",
			Name: "bug",
			States: []interfaces.StateConfig{
				{ID: "state-ready", Name: "ready", Type: interfaces.StateTypeInitial},
				{ID: "state-done", Name: "done", Type: interfaces.StateTypeTerminal},
				{ID: "state-failed", Name: "failed", Type: interfaces.StateTypeFailed},
			},
		})
	})

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if target.Code == factoryvalidation.CodeDuplicateIdentifier &&
			target.Subject.Type == factoryvalidation.SubjectTypeWorkState {
			t.Fatalf("unexpected duplicate work-state id target %#v", target)
		}
	}
}

func TestValidateBlockingLoad_AllowsLegacyExampleFactoryWithoutEntityIDs(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "..", "examples", "basic", "factory", interfaces.FactoryConfigFile)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	loaded, err := config.LoadFromCanonicalJSON(payload, nil)
	if err != nil {
		t.Fatalf("LoadFromCanonicalJSON(%s): %v", path, err)
	}

	result := factoryvalidation.ValidateBlockingLoad(loaded.FactoryConfig())
	if len(result.Targets) != 0 {
		t.Fatalf("blocking load targets = %#v, want none for legacy name-keyed example", result.Targets)
	}
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

func TestValidate_UsesExplicitSubjectIDsForGraphValidationTargets(t *testing.T) {
	t.Parallel()

	cfg := stableIDValidationConfig(func(cfg *interfaces.FactoryConfig) {
		cfg.WorkTypes[0].States = cfg.WorkTypes[0].States[:1]
		cfg.Workstations[0].OnFailure = nil
	})

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkstation,
		ID:       "workstation-execute-story",
		Location: factoryvalidation.SubjectLocationOnFailure,
	})
	validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{
		Type:     factoryvalidation.SubjectTypeWorkType,
		ID:       "work-type-story",
		Location: factoryvalidation.SubjectLocationStates,
	})
}

func stableIDValidationConfig(mutate func(*interfaces.FactoryConfig)) *interfaces.FactoryConfig {
	cfg := &interfaces.FactoryConfig{
		Name: "stable-id-factory",
		WorkTypes: []interfaces.WorkTypeConfig{{
			ID:   "work-type-story",
			Name: "story",
			States: []interfaces.StateConfig{
				{ID: "state-ready", Name: "ready", Type: interfaces.StateTypeInitial},
				{ID: "state-done", Name: "done", Type: interfaces.StateTypeTerminal},
				{ID: "state-failed", Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Resources: []interfaces.ResourceConfig{{
			ID:       "resource-agent-slot",
			Name:     "agent-slot",
			Capacity: 1,
		}},
		Workers: []interfaces.WorkerConfig{{
			ID:   "worker-executor",
			Name: "executor",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "workstation-execute-story",
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "ready"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "done"}},
			OnFailure:      []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}},
		}},
	}
	if mutate != nil {
		mutate(cfg)
	}
	return cfg
}

func assertDuplicateExplicitIDTargets(
	t *testing.T,
	targets []factoryvalidation.Target,
	subjectType factoryvalidation.SubjectType,
	subjectID string,
	messageContains string,
	paths []string,
) {
	t.Helper()

	for _, path := range paths {
		found := false
		for _, target := range targets {
			if target.Code != factoryvalidation.CodeDuplicateIdentifier ||
				target.Subject.Type != subjectType ||
				target.Subject.ID != subjectID ||
				target.Path != path ||
				!strings.Contains(target.Message, messageContains) {
				continue
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("missing duplicate id target type=%s id=%q path=%q message containing %q in %#v", subjectType, subjectID, path, messageContains, targets)
		}
	}
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

func TestValidate_InvocationReturnExplicitValid(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			WorkTypeName:  "story",
			TerminalState: "complete",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "factory.invocationReturn.") {
			t.Fatalf("targets = %#v, want no invocationReturn findings", result.Targets)
		}
	}
}

func TestValidate_InvocationReturnExplicitMissingWorkType(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			TerminalState: "complete",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationReturnMissingWorkTypeName)
}

func TestValidate_InvocationReturnExplicitInvalidTerminalState(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		InvocationReturn: &interfaces.InvocationReturnConfig{
			Policy:        string(factoryapi.InvocationReturnPolicyExplicit),
			WorkTypeName:  "story",
			TerminalState: "review",
		},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "review", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeInvocationReturnInvalidTerminalState)
}

func TestValidate_InvocationReturnOmitted(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
	}

	result := factoryvalidation.Validate(cfg)
	for _, target := range result.Targets {
		if strings.HasPrefix(target.Code, "factory.invocationReturn.") {
			t.Fatalf("targets = %#v, want omitted invocationReturn to stay valid", result.Targets)
		}
	}
}

type missingOutputRoutesCase struct {
	name                  string
	workTypes             []interfaces.WorkTypeConfig
	workstation           interfaces.FactoryWorkstationConfig
	wantCode              string
	wantLocation          factoryvalidation.SubjectLocation
	wantPathSuffix        string
	wantMessageContains   string
	forbiddenCode         string
	forbiddenCodeLocation factoryvalidation.SubjectLocation
}

func missingOutputRoutesVsFailureRouteCases() []missingOutputRoutesCase {
	return []missingOutputRoutesCase{
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
			wantMessageContains:   "output routes",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingFailureRoute,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOnFailure,
		},
		{
			name: "routeless_cron_without_worker",
			workstation: interfaces.FactoryWorkstationConfig{
				Name: "trigger-monkey",
				Type: interfaces.WorkstationTypeLogical,
				Kind: interfaces.WorkstationKindCron,
				Cron: &interfaces.CronConfig{Schedule: "0 * * * *"},
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingOutputRoutes,
			wantLocation:          factoryvalidation.SubjectLocationOutputs,
			wantPathSuffix:        "factory.workstations[0].outputs",
			wantMessageContains:   "output routes",
			forbiddenCode:         factoryvalidation.CodeWorkstationMissingFailureRoute,
			forbiddenCodeLocation: factoryvalidation.SubjectLocationOnFailure,
		},
		{
			name: "routeless_logical_move_empty_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name: "router",
				Type: interfaces.WorkstationTypeLogical,
			},
			wantCode:              factoryvalidation.CodeWorkstationMissingOutputRoutes,
			wantLocation:          factoryvalidation.SubjectLocationOutputs,
			wantPathSuffix:        "factory.workstations[0].outputs",
			wantMessageContains:   "output routes",
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

	for _, tt := range missingOutputRoutesVsFailureRouteCases() {
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
			if tt.wantMessageContains != "" {
				assertWorkstationTargetMessageContains(
					t,
					result.Targets,
					tt.workstation.Name,
					tt.wantCode,
					tt.wantLocation,
					tt.wantMessageContains,
				)
			}
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

func assertWorkstationTargetMessageContains(
	t *testing.T,
	targets []factoryvalidation.Target,
	workstationID string,
	code string,
	location factoryvalidation.SubjectLocation,
	substring string,
) {
	t.Helper()
	for _, target := range targets {
		if target.Code != code || target.Subject.ID != workstationID || target.Subject.Location != location {
			continue
		}
		if !strings.Contains(target.Message, substring) {
			t.Fatalf("target message = %q, want substring %q (target %#v)", target.Message, substring, target)
		}
		return
	}
	t.Fatalf("targets = %#v, want %q for workstation %q at %q", targets, code, workstationID, location)
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

var workstationRouteRequirementCodes = []string{
	factoryvalidation.CodeWorkstationMissingOutputRoutes,
	factoryvalidation.CodeWorkstationMissingFailureRoute,
	factoryvalidation.CodeWorkstationMissingRejectionRoute,
}

func assertNoWorkstationRouteRequirementTargets(t *testing.T, targets []factoryvalidation.Target, workstationID string) {
	t.Helper()
	for _, target := range targets {
		if target.Subject.Type != factoryvalidation.SubjectTypeWorkstation || target.Subject.ID != workstationID {
			continue
		}
		for _, code := range workstationRouteRequirementCodes {
			if target.Code == code {
				t.Fatalf("workstation %q must not receive route requirement %q at %q, got %#v", workstationID, code, target.Subject.Location, target)
			}
		}
	}
}

func TestValidate_WorkerBackedKindsPreserveMissingFailureRouteWhenOutputsExist(t *testing.T) {
	t.Parallel()

	workTypesWithoutDefaultableFailure := []interfaces.WorkTypeConfig{{
		Name: "task",
		States: []interfaces.StateConfig{
			{Name: "in-review", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
		},
	}}
	outputRoute := []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}}
	workers := []interfaces.WorkerConfig{{Name: "worker-a"}}

	cases := []struct {
		name        string
		workstation interfaces.FactoryWorkstationConfig
	}{
		{
			name: "standard_workstation_with_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "process",
				Kind:           interfaces.WorkstationKindStandard,
				WorkerTypeName: "worker-a",
				Outputs:        outputRoute,
			},
		},
		{
			name: "repeater_with_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "repeater",
				Kind:           interfaces.WorkstationKindRepeater,
				WorkerTypeName: "worker-a",
				Outputs:        outputRoute,
			},
		},
		{
			name: "classifier_with_classification_route_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "classifier",
				Type:           interfaces.WorkstationTypeClassify,
				WorkerTypeName: "worker-a",
				ClassificationRoutes: []interfaces.ClassificationRouteConfig{
					{Label: "approved", Outputs: outputRoute},
				},
			},
		},
		{
			name: "poller_with_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "ingress",
				Kind:           interfaces.WorkstationKindPoller,
				WorkerTypeName: "worker-a",
				Outputs:        outputRoute,
			},
		},
		{
			name: "cron_with_worker_and_outputs",
			workstation: interfaces.FactoryWorkstationConfig{
				Name:           "scheduled-worker",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "worker-a",
				Cron:           &interfaces.CronConfig{Schedule: "0 * * * *"},
				Outputs:        outputRoute,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &interfaces.FactoryConfig{
				WorkTypes:    workTypesWithoutDefaultableFailure,
				Workers:      workers,
				Workstations: []interfaces.FactoryWorkstationConfig{tt.workstation},
			}
			result := factoryvalidation.Validate(cfg)
			assertWorkstationTarget(
				t,
				result.Targets,
				tt.workstation.Name,
				factoryvalidation.CodeWorkstationMissingFailureRoute,
				factoryvalidation.SubjectLocationOnFailure,
				"factory.workstations[0].onFailure",
			)
			assertWorkstationTargetAbsent(
				t,
				result.Targets,
				tt.workstation.Name,
				factoryvalidation.CodeWorkstationMissingOutputRoutes,
				factoryvalidation.SubjectLocationOutputs,
			)
		})
	}
}

func TestValidate_LogicalMoveOutcomeRouteExemption(t *testing.T) {
	t.Parallel()

	workTypesWithoutFailedState := []interfaces.WorkTypeConfig{{
		Name: "task",
		States: []interfaces.StateConfig{
			{Name: "init", Type: interfaces.StateTypeInitial},
			{Name: "in-review", Type: interfaces.StateTypeProcessing},
			{Name: "complete", Type: interfaces.StateTypeTerminal},
		},
	}}
	outputRoute := []interfaces.IOConfig{{WorkTypeName: "task", StateName: "in-review"}}

	cases := []struct {
		name        string
		workTypes   []interfaces.WorkTypeConfig
		workstation interfaces.FactoryWorkstationConfig
	}{
		{
			name:      "logical_move_with_outputs_no_outcome_routes",
			workTypes: workTypesWithoutFailedState,
			workstation: interfaces.FactoryWorkstationConfig{
				Name:    "router",
				Type:    interfaces.WorkstationTypeLogical,
				Outputs: outputRoute,
			},
		},
		{
			name:      "logical_move_cron_with_outputs_no_outcome_routes",
			workTypes: workTypesWithoutFailedState,
			workstation: interfaces.FactoryWorkstationConfig{
				Name:    "scheduled-router",
				Type:    interfaces.WorkstationTypeLogical,
				Kind:    interfaces.WorkstationKindCron,
				Cron:    &interfaces.CronConfig{Schedule: "0 * * * *"},
				Outputs: outputRoute,
			},
		},
		{
			name: "logical_move_repeater_with_outputs_no_outcome_routes",
			workTypes: []interfaces.WorkTypeConfig{{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "in-review", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			}},
			workstation: interfaces.FactoryWorkstationConfig{
				Name:    "loop-breaker",
				Type:    interfaces.WorkstationTypeLogical,
				Kind:    interfaces.WorkstationKindRepeater,
				Outputs: outputRoute,
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &interfaces.FactoryConfig{
				WorkTypes:    tt.workTypes,
				Workstations: []interfaces.FactoryWorkstationConfig{tt.workstation},
			}
			result := factoryvalidation.Validate(cfg)
			assertNoWorkstationRouteRequirementTargets(t, result.Targets, tt.workstation.Name)
		})
	}
}
