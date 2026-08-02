package editable_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotscontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/contracts"
	snapshotsportabilityeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
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
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
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
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	canonicalLoads := 0
	loadCanonical := func(payload []byte, loader snapshotscontracts.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		canonicalLoads++
		return factorydefinitioncomposition.LoadCanonicalJSON(payload, loader)
	}
	err = snapshotsportabilityeditable.ValidateSnapshot(
		t.Context(),
		snapshot,
		nil,
		func(snapshot *factorydefinitions.FactorySnapshot, loader snapshotscontracts.WorkstationLoader) (factorydefinitions.DefinitionValidationRequest, error) {
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
	snapshot *factorydefinitions.FactorySnapshot,
	workstationLoader snapshotscontracts.WorkstationLoader,
) error {
	return snapshotsportabilityeditable.ValidateSnapshot(
		context.Background(),
		snapshot,
		workstationLoader,
		func(
			snapshot *factorydefinitions.FactorySnapshot,
			loader snapshotscontracts.WorkstationLoader,
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
