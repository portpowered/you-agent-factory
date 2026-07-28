package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	projectionimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/projections"
)

var (
	BuildFactoryWorldView                         = projectionimpl.BuildFactoryWorldView
	BuildFactoryWorldViewWithActiveThrottlePauses = projectionimpl.BuildFactoryWorldViewWithActiveThrottlePauses
	BuildSimpleDashboardProjection                = projectionimpl.BuildSimpleDashboardProjection
	ProjectActiveThrottlePauses                   = projectionimpl.ProjectActiveThrottlePauses
	ReconstructCanonicalFactoryWorldState         = projectionimpl.ReconstructCanonicalFactoryWorldState
)

type (
	SimpleDashboardProjection                = projectionimpl.SimpleDashboardProjection
	SimpleDashboardRuntimeProjection         = projectionimpl.SimpleDashboardRuntimeProjection
	SimpleDashboardWorkstationNodeProjection = projectionimpl.SimpleDashboardWorkstationNodeProjection
)

// Preserve factory_definitions import for vocabulary boundary tests that assert
// the shim still depends on the published Factory contract.
var _ = interfaces.FactoryEventTypeRunRequest
