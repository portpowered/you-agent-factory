package dashboard

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	dashboardimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/projection_query/projections/dashboard"
)

var SimpleDashboardRenderDataFromWorldState = dashboardimpl.SimpleDashboardRenderDataFromWorldState

type (
	SimpleDashboardRenderData         = dashboardimpl.SimpleDashboardRenderData
	SimpleDashboardActiveExecution    = dashboardimpl.SimpleDashboardActiveExecution
	SimpleDashboardWorkstationActivity = dashboardimpl.SimpleDashboardWorkstationActivity
	SimpleDashboardSessionData        = dashboardimpl.SimpleDashboardSessionData
)

// Preserve published Recordings and Factory contract imports for boundary tests.
var (
	_ = recordings.SimpleDashboardRenderData{}
	_ = interfaces.FactoryWorldState{}
)
