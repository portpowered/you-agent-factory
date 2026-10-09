// Package wire constructs the Factory Visualization live_view_projection subservice.
package wire

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	projectionservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewService constructs the private live_view_projection capability.
func NewService(
	retainedEvents func() []factorydefinitions.FactoryEvent,
	source liveviewprojection.Source,
	recordingsPeer recordings.Service,
	clock liveviewprojection.Clock,
	sink liveviewprojection.Sink,
	reportError liveviewprojection.ErrorReporter,
) (liveviewprojection.Service, error) {
	svc, err := projectionservice.New(retainedEvents, source, recordingsPeer, clock, sink, reportError)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

// Owner is the private behavior owner retained by Visualization composition.
type Owner = projectionservice.Owner

// NewOwner constructs the fixed behavior once, before any runtime opening.
func NewOwner(peer recordings.Service) *Owner {
	return projectionservice.NewOwner(peer)
}
