// Package wire constructs the Factory Visualization live_view_projection subservice.
package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	visualizationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService constructs the private live_view_projection capability.
func NewService(
	source visualizationcontracts.LiveViewSource,
	recordingsPeer recordings.Service,
	clock visualizationcontracts.LiveViewClock,
	sink visualizationcontracts.LiveViewSink,
	reportError liveviewprojection.ErrorReporter,
) (liveviewprojection.Service, error) {
	svc, err := projectionservice.New(source, recordingsPeer, clock, sink, reportError)
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
