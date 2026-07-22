package globalconfiginventory_test

import (
	"bytes"
	"testing"

	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/globalconfiginventory"
	identityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory"
)

func TestInventoryLane_RepeatedSerializationIsByteIdenticalAcrossOwners(t *testing.T) {
	t.Parallel()

	topologyFirst, err := globalconfiginventory.MarshalCanonicalJSON(globalconfiginventory.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology first MarshalCanonicalJSON() error = %v", err)
	}
	topologySecond, err := globalconfiginventory.MarshalCanonicalJSON(globalconfiginventory.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(topologyFirst, topologySecond) {
		t.Fatalf("repeated global config topology inventory json differs")
	}

	operatorFirst, err := operator_settings.MarshalInputInventoryJSON(operator_settings.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator first MarshalInputInventoryJSON() error = %v", err)
	}
	operatorSecond, err := operator_settings.MarshalInputInventoryJSON(operator_settings.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(operatorFirst, operatorSecond) {
		t.Fatalf("repeated operator config input inventory json differs")
	}

	systemFirst, err := identityinventory.MarshalInputInventoryJSON(identityinventory.ProjectInputInventory())
	if err != nil {
		t.Fatalf("system first MarshalInputInventoryJSON() error = %v", err)
	}
	systemSecond, err := identityinventory.MarshalInputInventoryJSON(identityinventory.ProjectInputInventory())
	if err != nil {
		t.Fatalf("system second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(systemFirst, systemSecond) {
		t.Fatalf("repeated system config input inventory json differs")
	}
}
