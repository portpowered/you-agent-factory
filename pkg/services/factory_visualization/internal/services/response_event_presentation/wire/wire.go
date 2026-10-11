// Package wire constructs the Factory Visualization response/event presentation
// subservice.
package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
	presentationservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/internal/service"
)

// NewService constructs the inert parent-private response/event presentation
// owner. No presentation worker goroutines start until an output or stream is
// explicitly opened.
func NewService(scheduler platformclock.TimerSource) responseeventpresentation.Service {
	return presentationservice.New(scheduler)
}
