package validationentry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestValidateFactoryAPI_RoutelessCronAndLogicalMove_MissingOutputRoutesAtOutputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		decode      func() (factoryapi.Factory, error)
		workstation string
		profile     factoryvalidation.ValidationProfile
	}{
		{
			name:        "topology_routeless_cron",
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ValidationProfileTopology,
		},
		{
			name:        "pre_persist_routeless_cron",
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ValidationProfilePrePersist,
		},
		{
			name:        "topology_routeless_logical_move",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveFactory,
			workstation: "router",
			profile:     factoryvalidation.ValidationProfileTopology,
		},
		{
			name:        "pre_persist_routeless_logical_move_cron",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveCronFactory,
			workstation: "trigger-monkey",
			profile:     factoryvalidation.ValidationProfilePrePersist,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode factory: %v", err)
			}

			finding := factoryvalidation.ValidationResult{Targets: []factoryvalidation.ValidationTarget{{
				Code:     "factory.workstation.missingOutputRoutes",
				Severity: factoryvalidation.ValidationSeverityError,
				Message:  tc.workstation + " has no output routes",
				Subject: factoryvalidation.ValidationSubject{
					Type: factoryvalidation.ValidationSubjectTypeWorkstation, ID: tc.workstation,
					Location: factoryvalidation.ValidationSubjectLocationOutputs,
				},
			}}}
			validator := testFactoryDefinitionValidator(finding)
			if tc.profile == factoryvalidation.ValidationProfilePrePersist {
				validator = testFactoryDefinitionValidator(factoryvalidation.ValidationResult{}, finding)
			}
			result := invokeDefinitionValidationRole(t, factory, tc.profile, validator)
			if !result.HasTargets() {
				t.Fatal("expected validation targets for routeless workstation")
			}

			apiResult := apisurface.FactoryValidationResultToAPI(result)
			validationassert.HasTarget(
				t,
				apiResult.Targets,
				"factory.workstation.missingOutputRoutes",
				factoryapi.FactoryValidationSubjectTypeWorkstation,
				tc.workstation,
				factoryapi.FactoryValidationSubjectLocationOutputs,
				tc.workstation+" OUTPUTS missingOutputRoutes target",
			)
			for _, target := range apiResult.Targets {
				if target.Code == "factory.workstation.missingFailureRoute" &&
					target.Subject.Type == factoryapi.FactoryValidationSubjectTypeWorkstation &&
					target.Subject.Id == tc.workstation &&
					target.Subject.Location == factoryapi.FactoryValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", apiResult.Targets)
				}
			}
		})
	}
}
