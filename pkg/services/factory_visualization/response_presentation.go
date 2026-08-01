package factory_visualization

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
)

const (
	// DefaultProgressQueueCapacity is the bounded backlog for best-effort outputs.
	DefaultProgressQueueCapacity = contracts.DefaultProgressQueueCapacity
)

// Output serializes encoded presentation records onto one transport writer.
type Output = contracts.Output

// FactoryEventEncoder is the transport-owned representation edge for one
// canonical Factory Event.
type FactoryEventEncoder = contracts.FactoryEventEncoder

// FinalResponseWriter writes one terminal invocation representation after all
// accepted Factory Event records have drained.
type FinalResponseWriter = contracts.FinalResponseWriter

// ResponsePresentation is the presentation capability exposed at the root
// package without publishing its construction or implementation package.
type ResponsePresentation = responseeventpresentation.Service
