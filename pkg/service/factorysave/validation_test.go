package factorysave

import (
	"context"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIPrePersist(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	apiResult, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfilePrePersist,
	})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}

	saveErr := validateEditableFactoryTopology(factory, nil)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("validateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}

	apiSignatures := factoryvalidation.CanonicalTargetSignatures(apiResult.Targets)
	saveSignatures := factoryvalidation.CanonicalAPITargetSignatures(topologyErr.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(apiSignatures, saveSignatures) {
		t.Fatalf("ValidateFactoryAPI signatures = %#v, save signatures = %#v",
			apiSignatures, saveSignatures)
	}
	validationassert.HasTargetCode(t, topologyErr.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestValidateEditableFactoryTopology_ValidFactory_NoError(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}

	if err := validateEditableFactoryTopology(factory, nil); err != nil {
		t.Fatalf("validateEditableFactoryTopology: %v", err)
	}
}

func TestValidateUpsertNamedFactoryRequest_RejectsInvalidFactoryName(t *testing.T) {
	t.Parallel()

	factory := factoryapi.Factory{Name: ".."}

	err := validateUpsertNamedFactoryRequest(factory, nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactoryName", err)
	}
}
