// Package factorysnapshot maps Factory-owned canonical snapshots to public
// generated transport contracts.
package factorysnapshot

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ToAPI decodes a Factory-owned canonical snapshot into the generated public
// transport contract at the HTTP boundary.
func ToAPI(snapshot *interfaces.FactorySnapshot) (*factoryapi.Factory, error) {
	if snapshot == nil {
		return nil, nil
	}
	var factory factoryapi.Factory
	if err := snapshot.Decode(&factory); err != nil {
		return nil, fmt.Errorf("map factory snapshot to public contract: %w", err)
	}
	return &factory, nil
}
