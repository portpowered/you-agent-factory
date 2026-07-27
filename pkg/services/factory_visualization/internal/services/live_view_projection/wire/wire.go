// Package wire constructs the Factory Visualization live_view_projection subservice.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
)

// NewService constructs the private live_view_projection capability.
func NewService(
	source liveviewprojection.Source,
	projections recordings.ProjectionService,
	clock liveviewprojection.Clock,
	sink liveviewprojection.Sink,
	reportError liveviewprojection.ErrorReporter,
) (liveviewprojection.Service, error) {
	svc, err := projectionservice.New(source, projections, clock, sink, reportError)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// BindRetainedEventsSupplier forwards activation-owned retained history into the
// private owner for root Observe calls that occur before projection Start.
func BindRetainedEventsSupplier(
	svc liveviewprojection.Service,
	supplier func() []factorydefinitions.FactoryEvent,
) {
	projectionservice.BindRetainedEventsSupplier(svc, supplier)
}
