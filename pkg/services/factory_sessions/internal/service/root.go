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

// requireRootAssembly preserves the public construction boundary's existing
// nil and implementation classification without binding a constructed root.
func requireRootAssembly(
	assembly roles.RuntimeAssembly,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*legacyservice.Assembly, error) {
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
	return concrete, nil
}
