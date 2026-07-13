package validation_test

import (
	"os"
	"path/filepath"
	"strings"
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

	path := filepath.Join("..", "..", "..", "..", "examples", "basic", "factory", interfaces.FactoryConfigFile)
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
