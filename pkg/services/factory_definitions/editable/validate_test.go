package editable_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/editable"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func TestValidateSnapshotRequiresFactory(t *testing.T) {
	err := validateSnapshot(nil, nil)
	if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
		t.Fatalf("ValidateSnapshot error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestValidateSnapshotReturnsDomainTopologyError(t *testing.T) {
	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	err = validateSnapshot(snapshot, nil)
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("ValidateSnapshot error = %v, want TopologyError", err)
	}
	if len(topologyErr.Targets) == 0 {
		t.Fatal("TopologyError.Targets is empty")
	}
}

func TestValidateSnapshotForcesPrePersistOwnerProfile(t *testing.T) {
	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	canonicalLoads := 0
	loadCanonical := func(payload []byte, loader interfaces.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		canonicalLoads++
		return factorydefinitioncomposition.LoadCanonicalJSON(payload, loader)
	}
	err = editable.ValidateSnapshot(
		t.Context(),
		snapshot,
		nil,
		func(snapshot *interfaces.FactorySnapshot, loader interfaces.WorkstationLoader) (factorydefinitions.DefinitionValidationRequest, error) {
			request, mapErr := validationentry.MapEditableFactorySnapshot(snapshot, loader, loadCanonical)
			request.Profile = factorydefinitions.ValidationProfileTopology
			return request, mapErr
		},
		factoryvalidation.New(nil, loadCanonical),
	)
	if err != nil {
		t.Fatalf("ValidateSnapshot: %v", err)
	}
	if canonicalLoads != 1 {
		t.Fatalf("canonical loads = %d, want 1 from forced pre-persist profile", canonicalLoads)
	}
}

func validateSnapshot(
	snapshot *interfaces.FactorySnapshot,
	workstationLoader interfaces.WorkstationLoader,
) error {
	return editable.ValidateSnapshot(
		context.Background(),
		snapshot,
		workstationLoader,
		func(
			snapshot *interfaces.FactorySnapshot,
			loader interfaces.WorkstationLoader,
		) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapEditableFactorySnapshot(
				snapshot,
				loader,
				factorydefinitioncomposition.LoadCanonicalJSON,
			)
		},
		factoryvalidation.New(nil, factorydefinitioncomposition.LoadCanonicalJSON),
	)
}
