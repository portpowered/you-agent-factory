package factorydefinition_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// crossPathPrePersistFixtures are shared JSON fixtures used to prove validate and
// editable-save pre-checks agree when both use ProfilePrePersist.
var crossPathPrePersistFixtures = []struct {
	name     string
	decode   func() (factoryapi.Factory, error)
	wantFail bool
}{
	{
		name:     "cross_path_invalid",
		decode:   factoryfixtures.DecodeCrossPathInvalidFactory,
		wantFail: true,
	},
	{
		name:     "cross_path_valid_alpha",
		decode:   factoryfixtures.DecodeCrossPathValidAlphaFactory,
		wantFail: false,
	},
}

func TestCrossPathFixtures_ValidateFactoryAPIPrePersistMatchesEditableSavePreCheck(t *testing.T) {
	t.Parallel()

	for _, tc := range crossPathPrePersistFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			factory, err := tc.decode()
			if err != nil {
				t.Fatalf("decode fixture: %v", err)
			}

			apiResult, err := validateFactoryAPIPrePersistForTest(context.Background(), factory)
			if err != nil {
				t.Fatalf("ValidateFactoryAPI: %v", err)
			}

			saveErr := validateEditableFactoryTopology(factory, nil)
			apiFailed := apiResult.HasBlockingTargets()
			var topologyErr *interfaces.ValidationTopologyError
			saveFailed := errors.As(saveErr, &topologyErr)

			if apiFailed != tc.wantFail {
				t.Fatalf("ValidateFactoryAPI failed = %v, want %v (targets = %#v)", apiFailed, tc.wantFail, apiResult.Targets)
			}
			if saveFailed != tc.wantFail {
				t.Fatalf("validateEditableFactoryTopology failed = %v, want %v (err = %v)", saveFailed, tc.wantFail, saveErr)
			}
			if apiFailed != saveFailed {
				t.Fatalf("ValidateFactoryAPI failed = %v, save pre-check failed = %v, want identical failure vs success",
					apiFailed, saveFailed)
			}
			if !tc.wantFail {
				return
			}

			apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.BlockingTargets())
			saveSignatures := factoryvalidation.CanonicalTargetSignatures(topologyErr.Targets)
			if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
				t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
					apiSignatures, saveSignatures)
			}
		})
	}
}
