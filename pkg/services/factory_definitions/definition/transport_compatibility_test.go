package factorydefinition

import (
	"fmt"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func editableFactoryForCompatibilityTest(request factoryapi.Factory) (EditableFactory, error) {
	snapshot, err := factorydefinitions.NewFactorySnapshot(request)
	if err != nil {
		return EditableFactory{}, err
	}
	var version *factorydefinitions.FactoryVersion
	if request.Version != nil {
		version = &factorydefinitions.FactoryVersion{Logical: request.Version.Logical.Int64(), Physical: request.Version.Physical.UTC()}
	}
	return EditableFactory{Name: string(request.Name), Snapshot: snapshot, Version: version}, nil
}

func mustEditableFactoryForTest(t *testing.T, request factoryapi.Factory) EditableFactory {
	t.Helper()
	editable, err := editableFactoryForCompatibilityTest(request)
	if err != nil {
		t.Fatalf("capture editable Factory: %v", err)
	}
	return editable
}

func factorySnapshotForCompatibilityTest(snapshot *factorydefinitions.FactorySnapshot) (factoryapi.Factory, error) {
	mapped, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map Factory snapshot for compatibility test: %w", err)
	}
	if mapped == nil {
		return factoryapi.Factory{}, fmt.Errorf("Factory snapshot is required")
	}
	return *mapped, nil
}
