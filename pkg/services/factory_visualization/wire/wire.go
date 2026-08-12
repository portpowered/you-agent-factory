// Package wire is the Factory Visualization service composition boundary.
//
// Wire performs construction only, returns the singular factoryvisualization.Service
// interface, and starts no lifecycle components. Parent-private activation_lifecycle,
// live_view_projection, and response_event_presentation capabilities remain behind
// this boundary; peers depend on Root rather than owner internals or construction
// ports.
package wire

import (
	"fmt"
	"strconv"
	"sync"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	responseeventpresentationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// NewRuntimeSinkOwner constructs the typed Factory Visualization owner used
// to retain transport-selected sinks. The owner is inert: registration only
// retains a value and does not start visualization or touch the filesystem.
func NewRuntimeSinkOwner() factoryvisualization.RuntimeSinkOwner {
	return &runtimeSinkOwner{sinks: make(map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink)}
}

type runtimeSinkOwner struct {
	mu     sync.RWMutex
	nextID uint64
	sinks  map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink
}

func (owner *runtimeSinkOwner) RegisterRuntimeSink(sink factoryvisualization.Sink) (factoryvisualization.RuntimeSinkID, error) {
	if owner == nil {
		return "", fmt.Errorf("register Factory Visualization sink: owner is required")
	}
	if sink == nil {
		return "", fmt.Errorf("register Factory Visualization sink: sink is required")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.nextID++
	id := factoryvisualization.RuntimeSinkID("visualization-sink-" + strconv.FormatUint(owner.nextID, 10))
	owner.sinks[id] = sink
	return id, nil
}

func (owner *runtimeSinkOwner) RuntimeSink(id factoryvisualization.RuntimeSinkID) (factoryvisualization.Sink, bool) {
	if owner == nil || id == "" {
		return nil, false
	}
	owner.mu.RLock()
	sink, ok := owner.sinks[id]
	owner.mu.RUnlock()
	return sink, ok
}

func (owner *runtimeSinkOwner) CloseRuntimeSink(id factoryvisualization.RuntimeSinkID) {
	if owner == nil || id == "" {
		return
	}
	owner.mu.Lock()
	delete(owner.sinks, id)
	owner.mu.Unlock()
}

// NewRoot constructs an inert Factory Visualization root from construction and
// process-edge ports. It composes the accepted root through parent-private
// activation_lifecycle, live_view_projection, and response_event_presentation
// owner construction without publishing owner types on the returned peer surface.
// Missing required construction ports fail with a deterministic construction error
// and a nil root.
func NewRoot(
	source factoryvisualization.Source,
	peer recordings.ProjectionService,
	clock factoryvisualization.Clock,
	sink factoryvisualization.Sink,
	reportError factoryvisualization.ErrorReporter,
) (factoryvisualization.Service, error) {
	switch {
	case source == nil:
		return nil, fmt.Errorf("construct Factory Visualization: event source is required")
	case peer == nil:
		return nil, fmt.Errorf("construct Factory Visualization: projection service is required")
	case clock == nil:
		return nil, fmt.Errorf("construct Factory Visualization: clock is required")
	case sink == nil:
		return nil, fmt.Errorf("construct Factory Visualization: presentation sink is required")
	}
	recordingsPeer, err := recordingsPeerFromProjectionService(peer)
	if err != nil {
		return nil, err
	}
	activation, err := activationlifecyclewire.NewService(
		internalservice.ActivationEventSourceAdapter{Source: source},
		recordingsPeer,
		clock,
		internalservice.ActivationViewSinkAdapter{Sink: sink},
		activationlifecycle.ErrorReporter(reportError),
	)
	if err != nil {
		return nil, err
	}
	projection, err := liveviewprojectionwire.NewService(
		source,
		recordingsPeer,
		clock,
		sink,
		reportError,
	)
	if err != nil {
		return nil, err
	}
	presentation := responseeventpresentationwire.NewService()
	root, err := internalservice.New(
		activation,
		projection,
		presentation,
		source,
		recordingsPeer,
		clock,
		sink,
		reportError,
	)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("construct Factory Visualization: implementation rejected its dependencies")
	}
	return root, nil
}

// NewService is the canonical Factory Visualization root constructor.
func NewService(
	source factoryvisualization.Source,
	peer recordings.ProjectionService,
	clock factoryvisualization.Clock,
	sink factoryvisualization.Sink,
	reportError factoryvisualization.ErrorReporter,
) (factoryvisualization.Service, error) {
	return NewRoot(source, peer, clock, sink, reportError)
}

// NewCurrentRuntimeSource adapts a selected Factory Session runtime reader to
// the Visualization source contract.
func NewCurrentRuntimeSource(reader factoryvisualization.RuntimeReader) factoryvisualization.Source {
	return internalservice.NewCurrentRuntimeSource(reader)
}

// NewResponsePresentation constructs the inert response/event presentation
// capability used by transport composition.
func NewResponsePresentation() factoryvisualization.ResponsePresentation {
	return responseeventpresentationwire.NewService()
}
