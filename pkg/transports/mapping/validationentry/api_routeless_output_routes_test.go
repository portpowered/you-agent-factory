package validationentry_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
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
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ProfileTopology,
		},
		{
			name:        "pre_persist_routeless_cron",
			decode:      factoryfixtures.DecodeRoutelessCronFactory,
			workstation: "cron",
			profile:     factoryvalidation.ProfilePrePersist,
		},
		{
			name:        "topology_routeless_logical_move",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveFactory,
			workstation: "router",
			profile:     factoryvalidation.ProfileTopology,
		},
		{
			name:        "pre_persist_routeless_logical_move_cron",
			decode:      factoryfixtures.DecodeRoutelessLogicalMoveCronFactory,
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

			apiResult := apisurface.FactoryValidationResultToAPI(result)
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
