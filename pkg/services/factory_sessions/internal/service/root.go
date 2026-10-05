package service

import (
	"fmt"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
)

var _ factorysessions.Service = (*Root)(nil)
var _ roles.RuntimeAssembly = (*Root)(nil)

// NewRootFromAssembly binds the already-composed assembly to the one
// process-scoped Factory Sessions root.
func NewRootFromAssembly(
	assembly roles.RuntimeAssembly,
	processRoot *Root,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*Root, error) {
	if assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: runtime assembly is required")
	}
	if liveChangeCoordinator == nil {
		return nil, fmt.Errorf("construct Factory Sessions: live-change coordinator is required")
	}
	concrete, ok := assembly.(*legacyservice.Assembly)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("construct Factory Sessions: runtime assembly implementation rejected")
	}
	root := processRoot
	if root == nil {
		return nil, fmt.Errorf("construct Factory Sessions: process root is required")
	}
	if root.Assembly != nil && root.Assembly != concrete {
		return nil, fmt.Errorf("construct Factory Sessions: process root is already bound to another assembly")
	}
	root.Assembly = concrete
	root.liveChangeCoordinator = liveChangeCoordinator
	return root, nil
}
