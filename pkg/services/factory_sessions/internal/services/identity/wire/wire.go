// Package wire constructs the Factory Sessions identity subservice from exact
// injected effect ports.
package wire

import (
	"fmt"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	identityservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity/internal/service"
)

func NewService(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
) (identity.Service, error) {
	if resolveSymlinks == nil {
		return nil, fmt.Errorf("construct Factory Session identity: symlink resolver is required")
	}
	if resolveHome == nil {
		return nil, fmt.Errorf("construct Factory Session identity: home resolver is required")
	}
	service := identityservice.New(resolveSymlinks, resolveHome)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Session identity: implementation rejected its dependencies")
	}
	return service, nil
}
