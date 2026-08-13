// Package wire constructs the private runtime snapshot subservice.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runtimesnapshot "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot"
	runtimesnapshotimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/runtime_snapshot/internal"
)

// NewService constructs the runtime snapshot resolver from the exact source
// loaders supplied by Factory Definitions composition.
func NewService(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	workstationLoader func() factorydefinitions.WorkstationLoader,
) (runtimesnapshot.Service, error) {
	service, err := runtimesnapshotimpl.New(loadCanonical, loadFactory, workstationLoader)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Definitions runtime snapshot resolver: %w", err)
	}
	return service, nil
}
