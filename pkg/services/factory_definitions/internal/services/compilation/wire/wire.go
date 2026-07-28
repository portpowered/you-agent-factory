// Package wire constructs the Factory Definitions compilation subservice from
// exact injected load and encode ports.
package wire

import (
	"fmt"

	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/service"
)

// NewService constructs the private compilation subservice from exact injected
// canonical/directory load and encode ports. Callers must supply Dependencies;
// this constructor does not select Runtime/Petri implementations or take
// Wire/root construction ownership.
func NewService(deps compilationservice.Dependencies) (compilationservice.Service, error) {
	if deps.LoadCanonical == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: canonical Factory loader is required")
	}
	if deps.LoadFromFactoryDir == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: authored Factory directory loader is required")
	}
	if deps.EncodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: canonical Factory encoder is required")
	}
	service := compilationserviceimpl.New(
		deps.LoadCanonical,
		deps.LoadFromFactoryDir,
		deps.EncodeFactory,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions compilation: implementation rejected its dependencies")
	}
	return service, nil
}
