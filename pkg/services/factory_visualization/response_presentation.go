package factory_visualization

import (
	"errors"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	responseeventpresentationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/wire"
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

// ResponsePresentation is the service-root operation used by transports.
type ResponsePresentation interface {
	OpenBestEffortOutput(io.Writer) Output
	OpenLosslessOutput(io.Writer) Output
	OpenBestEffortFactoryEventStream(io.Writer, FactoryEventEncoder) interface {
		PresentFactoryEvents([]factorydefinitions.FactoryEvent)
		Finalize(FinalResponseWriter) (bool, error)
		CloseAndDrain() error
	}
	OpenLosslessFactoryEventStream(io.Writer, FactoryEventEncoder) interface {
		PresentFactoryEvents([]factorydefinitions.FactoryEvent)
		Finalize(FinalResponseWriter) (bool, error)
		CloseAndDrain() error
	}
}

type responsePresentation struct {
	owner responsePresentationOwner
}

type responsePresentationOwner interface {
	OpenBestEffortOutput(io.Writer) Output
	OpenLosslessOutput(io.Writer) Output
	OpenBestEffortFactoryEventStream(io.Writer, FactoryEventEncoder) contracts.FactoryEventStream
	OpenLosslessFactoryEventStream(io.Writer, FactoryEventEncoder) contracts.FactoryEventStream
}

// NewResponsePresentation constructs the inert response-presentation service.
// Wire is the application-level caller.
func NewResponsePresentation() ResponsePresentation {
	return responsePresentation{owner: responseeventpresentationwire.NewService()}
}

func (p responsePresentation) OpenBestEffortOutput(writer io.Writer) Output {
	return p.owner.OpenBestEffortOutput(writer)
}

func (p responsePresentation) OpenLosslessOutput(writer io.Writer) Output {
	return p.owner.OpenLosslessOutput(writer)
}

func (p responsePresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return p.owner.OpenBestEffortFactoryEventStream(writer, encode)
}

func (p responsePresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return p.owner.OpenLosslessFactoryEventStream(writer, encode)
}

var defaultPresentationOwner = responseeventpresentationwire.NewService()

func defaultResponsePresentationOwner() responsePresentationOwner {
	return defaultPresentationOwner
}

func openBestEffortOutput(writer io.Writer) Output {
	return defaultResponsePresentationOwner().OpenBestEffortOutput(writer)
}

func openLosslessOutput(writer io.Writer) Output {
	return defaultResponsePresentationOwner().OpenLosslessOutput(writer)
}

func appendPresentationLine(payload []byte) []byte {
	return contracts.AppendLine(payload)
}

func isPresentationClosedErr(err error) bool {
	return errors.Is(err, contracts.ErrOutputClosed)
}

func isPresentationBackpressureErr(err error) bool {
	return errors.Is(err, contracts.ErrBacklogFull)
}
