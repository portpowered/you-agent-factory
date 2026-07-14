package validationentry_test

import (
	"context"
	"testing"

	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestValidateFactoryAPI_RoutelessCronAndLogicalMove_MissingOutputRoutesAtOutputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		decode      func() (factoryapi.Factory, error)
		workstation string
		profile     factoryvalidation.Profile
	}{
		{
			name:        "topology_routeless_cron",
			decode:      factoryvalidation.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ProfileTopology,
		},
		{
			name:        "pre_persist_routeless_cron",
			decode:      factoryvalidation.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ProfilePrePersist,
		},
		{
			name:        "topology_routeless_logical_move",
			decode:      factoryvalidation.DecodeRoutelessLogicalMoveFactory,
			workstation: "router",
			profile:     factoryvalidation.ProfileTopology,
		},
		{
			name:        "pre_persist_routeless_logical_move_cron",
			decode:      factoryvalidation.DecodeRoutelessLogicalMoveCronFactory,
			workstation: "trigger-monkey",
			profile:     factoryvalidation.ProfilePrePersist,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode factory: %v", err)
			}

			result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
				Profile: tc.profile,
			})
			if err != nil {
				t.Fatalf("ValidateFactoryAPI: %v", err)
			}
			if !result.HasTargets() {
				t.Fatal("expected validation targets for routeless workstation")
			}

			apiResult := result.FactoryValidationResult()
			validationassert.HasTarget(
				t,
				apiResult.Targets,
				factoryvalidation.CodeWorkstationMissingOutputRoutes,
				factoryapi.FactoryValidationSubjectTypeWorkstation,
				tc.workstation,
				factoryapi.FactoryValidationSubjectLocationOutputs,
				tc.workstation+" OUTPUTS missingOutputRoutes target",
			)
			for _, target := range apiResult.Targets {
				if target.Code == factoryvalidation.CodeWorkstationMissingFailureRoute &&
					target.Subject.Type == factoryapi.FactoryValidationSubjectTypeWorkstation &&
					target.Subject.Id == tc.workstation &&
					target.Subject.Location == factoryapi.FactoryValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", apiResult.Targets)
				}
			}
		})
	}
}
