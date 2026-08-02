// Package wire constructs the private Definitions invocation resolver.
package wire

import (
	"fmt"

	invocationpolicy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/internal/service"
)

// NewService returns the one private invocation resolver. No policy bundle or
// operational dependency bag crosses this construction boundary.
func NewService() (invocationpolicy.Service, error) {
	service := invocationpolicyservice.New()
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions invocation resolver")
	}
	return service, nil
}
