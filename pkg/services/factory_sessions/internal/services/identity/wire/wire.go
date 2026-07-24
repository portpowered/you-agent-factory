// Package wire constructs the Factory Sessions identity subservice from exact
// injected effect ports.
package wire

import (
	"fmt"

	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identityservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/internal/service"
)

func NewService(dependencies identity.Dependencies) (identity.Service, error) {
	if dependencies.ResolveSymlinks == nil {
		return nil, fmt.Errorf("construct Factory Session identity: symlink resolver is required")
	}
	if dependencies.ResolveHome == nil {
		return nil, fmt.Errorf("construct Factory Session identity: home resolver is required")
	}
	if dependencies.Directories == nil {
		return nil, fmt.Errorf("construct Factory Session identity: directory inspection is required")
	}
	service := identityservice.New(dependencies)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Session identity: implementation rejected its dependencies")
	}
	return service, nil
}
