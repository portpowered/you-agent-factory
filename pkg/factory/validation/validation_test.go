package validation_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

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
		Workers: []workerconfig.Config{{Name: "worker-a"}},
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
		Workers: []workerconfig.Config{{Name: "worker-a"}},
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
		Workers: []workerconfig.Config{{Name: "worker-a"}},
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
	baseWorkers := []workerconfig.Config{{Name: "worker-a"}}

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
	workers := []workerconfig.Config{{Name: "worker-a"}}

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

func TestValidate_CanonicalFindingsAndStableIdentity(t *testing.T) {
	t.Run("canonical findings match config validation", func(t *testing.T) {
		apiFactory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
		if err != nil {
			t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
		}
		cfg, err := config.FactoryConfigFromOpenAPI(apiFactory)
		if err != nil {
			t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
		}
		explicit, findings := factoryvalidation.Validate(&cfg), config.CanonicalStructuralFindings(&cfg)
		if len(explicit.Targets) == 0 || len(explicit.Targets) != len(findings) {
			t.Fatalf("explicit targets = %d, config findings = %d, want equal non-zero counts", len(explicit.Targets), len(findings))
		}
		for i, target := range explicit.Targets {
			if findings[i].Rule != target.Code {
				t.Fatalf("finding[%d].Rule = %q, want %q", i, findings[i].Rule, target.Code)
			}
		}
		for _, code := range []string{factoryvalidation.CodeDuplicateIdentifier, factoryvalidation.CodeDanglingWorkerReference, factoryvalidation.CodeDanglingPlaceReference} {
			validationassert.HasDomainTargetCode(t, explicit.Targets, code)
		}
		validationassert.HasDomainTargetSubject(t, explicit.Targets, factoryvalidation.Subject{Type: factoryvalidation.SubjectTypeWorkstation, ID: "process", Location: factoryvalidation.SubjectLocationReference})
	})

	t.Run("duplicate explicit ids retain subjects messages and paths", func(t *testing.T) {
		cases := []struct {
			name, id, message string
			typeOf            factoryvalidation.SubjectType
			paths             []string
			mutate            func(*interfaces.FactoryConfig)
		}{
			{"resources", "resource-agent-slot", `duplicate resource id "resource-agent-slot"`, factoryvalidation.SubjectTypeResource, []string{"factory.resources[0].id", "factory.resources[1].id"}, func(c *interfaces.FactoryConfig) {
				c.Resources = append(c.Resources, factoryresource.Config{ID: "resource-agent-slot", Name: "review-slot", Capacity: 1})
			}},
			{"workers", "worker-executor", `duplicate worker id "worker-executor"`, factoryvalidation.SubjectTypeWorker, []string{"factory.workers[0].id", "factory.workers[1].id"}, func(c *interfaces.FactoryConfig) {
				c.Workers = append(c.Workers, workerconfig.Config{ID: "worker-executor", Name: "reviewer"})
			}},
			{"work types", "work-type-story", `duplicate work type id "work-type-story"`, factoryvalidation.SubjectTypeWorkType, []string{"factory.workTypes[0].id", "factory.workTypes[1].id"}, func(c *interfaces.FactoryConfig) {
				c.WorkTypes = append(c.WorkTypes, interfaces.WorkTypeConfig{ID: "work-type-story", Name: "bug"})
			}},
			{"workstations", "workstation-execute-story", `duplicate workstation id "workstation-execute-story"`, factoryvalidation.SubjectTypeWorkstation, []string{"factory.workstations[0].id", "factory.workstations[1].id"}, func(c *interfaces.FactoryConfig) {
				c.Workstations = append(c.Workstations, interfaces.FactoryWorkstationConfig{ID: "workstation-execute-story", Name: "review-story", WorkerTypeName: "executor", Inputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}}})
			}},
			{"work states", "work-type-story:state-ready", `duplicate work state id "state-ready" on work type "story"`, factoryvalidation.SubjectTypeWorkState, []string{"factory.workTypes[0].states[0].id", "factory.workTypes[0].states[1].id"}, func(c *interfaces.FactoryConfig) { c.WorkTypes[0].States[1].ID = "state-ready" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := stableIDConfig()
				tc.mutate(cfg)
				assertTargetDetails(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeDuplicateIdentifier, tc.typeOf, tc.id, "", tc.message, tc.paths...)
			})
		}
	})

	t.Run("same state id in different work types remains valid", func(t *testing.T) {
		cfg := stableIDConfig()
		cfg.WorkTypes = append(cfg.WorkTypes, interfaces.WorkTypeConfig{ID: "work-type-bug", Name: "bug", States: []interfaces.StateConfig{{ID: "state-ready", Name: "ready", Type: interfaces.StateTypeInitial}}})
		assertTargetAbsent(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeDuplicateIdentifier, factoryvalidation.SubjectTypeWorkState)
	})

	t.Run("explicit ids remain graph target subjects", func(t *testing.T) {
		cfg := stableIDConfig()
		cfg.WorkTypes[0].States = cfg.WorkTypes[0].States[:1]
		cfg.Workstations[0].Outputs[0].StateName = "missing"
		cfg.Workstations[0].OnFailure = nil
		result := factoryvalidation.Validate(cfg)
		validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeDanglingPlaceReference)
		validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{Type: factoryvalidation.SubjectTypeWorkstation, ID: "workstation-execute-story", Location: factoryvalidation.SubjectLocationOnFailure})
		validationassert.HasDomainTargetSubject(t, result.Targets, factoryvalidation.Subject{Type: factoryvalidation.SubjectTypeWorkType, ID: "work-type-story", Location: factoryvalidation.SubjectLocationStates})
	})

	t.Run("legacy example without ids remains valid", func(t *testing.T) {
		path := filepath.Join("..", "..", "..", "examples", "basic", "factory", interfaces.FactoryConfigFile)
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		loaded, err := config.LoadFromCanonicalJSON(payload, nil)
		if err != nil {
			t.Fatalf("LoadFromCanonicalJSON(%s): %v", path, err)
		}
		if got := factoryvalidation.ValidateBlockingLoad(loaded.FactoryConfig()).Targets; len(got) != 0 {
			t.Fatalf("blocking load targets = %#v, want none", got)
		}
	})

	t.Run("default work type uniqueness is consistent", func(t *testing.T) {
		cfg := &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{{Name: "story", HandlingBehavior: []string{interfaces.WorkTypeHandlingBehaviorDefault}}, {Name: "task"}}}
		if got := factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{}); len(got) != 0 {
			t.Fatalf("single default targets = %#v, want none", got)
		}
		cfg.WorkTypes[1].HandlingBehavior = []string{interfaces.WorkTypeHandlingBehaviorDefault}
		validationassert.HasDomainTargetCode(t, factoryvalidation.WorkTypeHandlingBehaviorTargets(cfg, factoryvalidation.WorkTypeHandlingBehaviorOptions{}), factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
		validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeWorkTypeHandlingBehaviorUniqueDefault)
	})
}

func TestValidate_InvocationContracts(t *testing.T) {
	t.Run("return policy accepts explicit valid and omitted configurations", func(t *testing.T) {
		cfg := invocationConfig()
		cfg.InvocationReturn = &interfaces.InvocationReturnConfig{Policy: string(factoryapi.InvocationReturnPolicyExplicit), WorkTypeName: "task", TerminalState: "done"}
		assertCodePrefixAbsent(t, factoryvalidation.Validate(cfg).Targets, "factory.invocationReturn.")
		cfg.InvocationReturn = nil
		assertCodePrefixAbsent(t, factoryvalidation.Validate(cfg).Targets, "factory.invocationReturn.")
	})

	t.Run("return policy rejects missing work type and non-terminal state", func(t *testing.T) {
		cfg := invocationConfig()
		cfg.InvocationReturn = &interfaces.InvocationReturnConfig{Policy: string(factoryapi.InvocationReturnPolicyExplicit), TerminalState: "done"}
		validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeInvocationReturnMissingWorkTypeName)
		cfg.InvocationReturn = &interfaces.InvocationReturnConfig{Policy: string(factoryapi.InvocationReturnPolicyExplicit), WorkTypeName: "task", TerminalState: "queued"}
		validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeInvocationReturnInvalidTerminalState)
	})

	t.Run("valid signature remains accepted", func(t *testing.T) {
		cfg := invocationConfig()
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{UnknownNamedArgumentPolicy: string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyReject), Parameters: []interfaces.InvocationParameterConfig{
			{Name: "input", ExternalName: "input", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1}}},
			{Name: "output", ExternalName: "output", TypeHint: string(factoryapi.FactoryInvocationParameterTypeHintPath), Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}}},
		}, OutputContract: &interfaces.InvocationOutputContractConfig{Mode: string(factoryapi.FactoryInvocationOutputContractModeFile), PathParameter: "output"}}
		cfg.Workers[0].Model, cfg.Workstations[0].Body, cfg.Workstations[0].WorkingDirectory = "${input}", "Render ${input}", "/tmp/${output}"
		assertCodePrefixAbsent(t, factoryvalidation.Validate(cfg).Targets, "factory.invocationSignature.")
	})

	t.Run("malformed bindings defaults and interpolation are rejected", func(t *testing.T) {
		cfg := invocationConfig()
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{UnknownNamedArgumentPolicy: string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect), Parameters: []interfaces.InvocationParameterConfig{
			{Name: "secret", ExternalName: "token", Sensitive: true, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 2}}},
			{Name: "secret", ExternalName: "token", ValueMode: string(factoryapi.FactoryInvocationParameterValueModeRepeated), DefaultValue: "one", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindStdin)}, {Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}}},
			{Name: "extras", ExternalName: "extras", ValueMode: string(factoryapi.FactoryInvocationParameterValueModeVariadic), Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 4}}},
		}}
		for _, code := range []string{factoryvalidation.CodeInvocationSignatureDuplicateParameterName, factoryvalidation.CodeInvocationSignatureDuplicateNamedKey, factoryvalidation.CodeInvocationSignatureSensitivePositional, factoryvalidation.CodeInvocationSignatureInvalidDefaultShape, factoryvalidation.CodeInvocationSignatureInvalidStdinRouting, factoryvalidation.CodeInvocationSignatureInvalidPositionalOrdering, factoryvalidation.CodeInvocationSignatureInvalidNamedRestShape} {
			validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(cfg).Targets, code)
		}
		cfg = invocationConfig()
		cfg.InvocationSignature = &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{{Name: "items", ExternalName: "item", ValueMode: string(factoryapi.FactoryInvocationParameterValueModeRepeated), Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}}}}, OutputContract: &interfaces.InvocationOutputContractConfig{Mode: string(factoryapi.FactoryInvocationOutputContractModeFile), PathParameter: "missing-output"}}
		cfg.Workers[0].Model, cfg.Workstations[0].Body = "${missing}", "Use ${items}"
		for _, code := range []string{factoryvalidation.CodeInvocationSignatureUnknownOutputPathParameter, factoryvalidation.CodeInvocationSignatureInvalidInterpolationReference, factoryvalidation.CodeInvocationSignatureIncompatibleInterpolationReference} {
			validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(cfg).Targets, code)
		}
	})
}

func TestValidate_WorkerModelProviderPreservesExistingOpenAIAlias(t *testing.T) {
	t.Parallel()

	cfg := invocationConfig()
	cfg.Workers[0].ModelProvider = "openai"

	assertTargetAbsent(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeWorkerUnsupportedModelProvider, factoryvalidation.SubjectTypeWorker)
}

func TestValidate_OrchestratorCompatibilityAndWorkPropagation(t *testing.T) {
	t.Run("legacy Petri factory without orchestrator remains valid", func(t *testing.T) {
		cfg := invocationConfig()
		if got := interfaces.EffectiveOrchestratorKind(cfg); got != interfaces.OrchestratorKindPetri {
			t.Fatalf("EffectiveOrchestratorKind = %q, want PETRI", got)
		}
		if targets := factoryvalidation.OrchestratorTargets(cfg); len(targets) != 0 || !factoryvalidation.IsPetriOrchestratorValidationScope(cfg) {
			t.Fatalf("orchestrator targets = %#v, Petri scope = %v", targets, factoryvalidation.IsPetriOrchestratorValidationScope(cfg))
		}
	})

	t.Run("JavaScript acceptance and rejection boundaries", func(t *testing.T) {
		valid := &interfaces.FactoryConfig{Name: "dynamic", Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: interfaces.OrchestratorKindJavaScript, JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "factory/workflows/review.js", Entrypoint: "main"}}}
		if result := factoryvalidation.Validate(valid); result.HasTargets() {
			t.Fatalf("valid JavaScript targets = %#v, want none", result.Targets)
		}
		cases := []struct {
			name, code string
			cfg        *interfaces.FactoryConfig
		}{
			{"missing source", factoryvalidation.CodeOrchestratorJavaScriptMissingSource, &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: interfaces.OrchestratorKindJavaScript, JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{}}}},
			{"empty agent preset", factoryvalidation.CodeOrchestratorJavaScriptInvalidAgent, &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: interfaces.OrchestratorKindJavaScript, JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{SourceRef: "workflow.js", Agents: map[string]interfaces.FactoryOrchestratorJavaScriptAgent{"reviewer": {Preset: "   "}}}}}},
			{"Petri fields", factoryvalidation.CodeOrchestratorIncompatiblePetriField, &interfaces.FactoryConfig{Orchestrator: valid.Orchestrator, WorkTypes: []interfaces.WorkTypeConfig{{Name: "task"}}}},
			{"unsupported kind", factoryvalidation.CodeOrchestratorUnsupportedKind, &interfaces.FactoryConfig{Orchestrator: &interfaces.FactoryOrchestratorConfig{Kind: "STREAM"}}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				validationassert.HasDomainTargetCode(t, factoryvalidation.Validate(tc.cfg).Targets, tc.code)
			})
		}
	})

	t.Run("generated work propagation modes and omission remain accepted", func(t *testing.T) {
		for _, generated := range []factoryapi.WorkPropagationMode{factoryapi.WorkPropagationModeOutputAsPayload, factoryapi.WorkPropagationModePreserveInput} {
			cfg := invocationConfig()
			cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{Mode: interfaces.WorkPropagationMode(generated)}
			assertTargetAbsent(t, factoryvalidation.Validate(cfg).Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode, factoryvalidation.SubjectTypeWorkstation)
		}
		assertTargetAbsent(t, factoryvalidation.Validate(invocationConfig()).Targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode, factoryvalidation.SubjectTypeWorkstation)
	})

	t.Run("empty and unsupported work propagation modes are rejected", func(t *testing.T) {
		for _, mode := range []interfaces.WorkPropagationMode{"", "MERGE_PAYLOAD", interfaces.WorkPropagationMode(factoryapi.WorkPropagationMode("PRESERVE_OUTPUT"))} {
			cfg := invocationConfig()
			cfg.Workstations[0].WorkPropagation = &interfaces.WorkPropagationConfig{Mode: mode}
			targets := factoryvalidation.Validate(cfg).Targets
			validationassert.HasDomainTargetCode(t, targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode)
			if mode == "MERGE_PAYLOAD" {
				assertTargetDetails(t, targets, factoryvalidation.CodeWorkstationUnsupportedWorkPropagationMode, factoryvalidation.SubjectTypeWorkstation, "process", factoryvalidation.SubjectLocation(""), `unsupported workPropagation.mode "MERGE_PAYLOAD"`, "factory.workstations[0](process).workPropagation.mode")
			}
		}
	})
}

func stableIDConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{Name: "stable-id-factory",
		WorkTypes: []interfaces.WorkTypeConfig{{ID: "work-type-story", Name: "story", States: []interfaces.StateConfig{{ID: "state-ready", Name: "ready", Type: interfaces.StateTypeInitial}, {ID: "state-done", Name: "done", Type: interfaces.StateTypeTerminal}, {ID: "state-failed", Name: "failed", Type: interfaces.StateTypeFailed}}}},
		Resources: []factoryresource.Config{{ID: "resource-agent-slot", Name: "agent-slot", Capacity: 1}}, Workers: []workerconfig.Config{{ID: "worker-executor", Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{ID: "workstation-execute-story", Name: "execute-story", WorkerTypeName: "executor", Inputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "ready"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "done"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "story", StateName: "failed"}}}}}
}

func invocationConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{Name: "invocation-validation", WorkTypes: []interfaces.WorkTypeConfig{{Name: "task", States: []interfaces.StateConfig{{Name: "queued", Type: interfaces.StateTypeInitial}, {Name: "done", Type: interfaces.StateTypeTerminal}, {Name: "failed", Type: interfaces.StateTypeFailed}}}}, Workers: []workerconfig.Config{{Name: "worker-a", Type: interfaces.WorkerTypeInference}}, Workstations: []interfaces.FactoryWorkstationConfig{{Name: "process", WorkerTypeName: "worker-a", Inputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "queued"}}, Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}}, OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}}}}
}

func assertTargetDetails(t *testing.T, targets []factoryvalidation.Target, code string, subjectType factoryvalidation.SubjectType, id string, location factoryvalidation.SubjectLocation, message string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		found := false
		for _, target := range targets {
			if target.Code == code && target.Subject.Type == subjectType && target.Subject.ID == id && (location == "" || target.Subject.Location == location) && target.Path == path && strings.Contains(target.Message, message) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing target code=%q type=%q id=%q location=%q path=%q message containing %q in %#v", code, subjectType, id, location, path, message, targets)
		}
	}
}

func assertTargetAbsent(t *testing.T, targets []factoryvalidation.Target, code string, subjectType factoryvalidation.SubjectType) {
	t.Helper()
	for _, target := range targets {
		if target.Code == code && target.Subject.Type == subjectType {
			t.Fatalf("unexpected target %#v", target)
		}
	}
}

func assertCodePrefixAbsent(t *testing.T, targets []factoryvalidation.Target, prefix string) {
	t.Helper()
	for _, target := range targets {
		if strings.HasPrefix(target.Code, prefix) {
			t.Fatalf("targets = %#v, want no code with prefix %q", targets, prefix)
		}
	}
}
