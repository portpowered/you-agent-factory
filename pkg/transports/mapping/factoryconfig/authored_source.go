package factoryconfig

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// DecodeAuthoredFactoryAPI decodes supplied authored factory.json bytes
// without flattening or applying usage-aware taxonomy projection.
func DecodeAuthoredFactoryAPI(data []byte) (factoryapi.Factory, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal(data, &factory); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory config: %w", err)
	}
	return factory, nil
}
