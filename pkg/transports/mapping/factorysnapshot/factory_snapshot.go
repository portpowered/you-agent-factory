// Package factorysnapshot maps Factory-owned canonical snapshots to public
// generated transport contracts.
package factorysnapshot

import (
	"encoding/json"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// ObjectFromFactoryConfig maps an authored Factory Definition through the
// generated public contract at the transport boundary.
func ObjectFromFactoryConfig(
	factoryConfig *interfaces.FactoryConfig,
) (map[string]any, error) {
	payload, err := json.Marshal(factorymapping.FactoryConfigToOpenAPI(factoryConfig))
	if err != nil {
		return nil, fmt.Errorf("encode Factory snapshot boundary: %w", err)
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, fmt.Errorf("decode Factory snapshot boundary: %w", err)
	}
	return object, nil
}

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
