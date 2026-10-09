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

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/service"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
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

// NewRuntimeSourceOpening constructs one addressed runtime observation owner.
// Its returned operation allocates only a session identity handle.
func NewRuntimeSourceOpening(reader factoryvisualization.RuntimeReader) func(string) factoryvisualization.Source {
	return internalservice.NewRuntimeSourceOpening(reader)
}

// NewRuntimeOpening constructs the fixed addressed resource-selection owner.
func NewRuntimeOpening(reader factoryvisualization.RuntimeReader,
	openSource func(string) factoryvisualization.Source, openScope ScopeOpening,
	sinks factoryvisualization.RuntimeSinkOwner, override factoryvisualization.Sink,
	observe factoryvisualization.RootObserver,
) factoryvisualization.RuntimeOpening {
	return internalservice.NewRuntimeOpeningOwner(reader, openSource, openScope, sinks, override, observe).Open
}

// NewResponsePresentation constructs the inert response/event presentation
// capability used by transport composition.
func NewResponsePresentation() factoryvisualization.ResponsePresentation {
	return responseeventpresentationwire.NewService()
}

// ActivationOpening allocates subscription state over the fixed activation owner.
type ActivationOpening = func(activationlifecycle.EventSource, activationlifecycle.Clock, activationlifecycle.ViewSink, activationlifecycle.ErrorReporter) activationlifecycle.Service

// ProjectionOpening allocates observation state with its scoped retained history.
type ProjectionOpening = func(func() []factorydefinitions.FactoryEvent, liveviewprojection.Source, liveviewprojection.Clock, liveviewprojection.Sink, liveviewprojection.ErrorReporter) liveviewprojection.Service

// ScopeOpening allocates a lifecycle handle over prebuilt Visualization owners.
type ScopeOpening func(factoryvisualization.Source, factoryvisualization.Clock, factoryvisualization.Sink, factoryvisualization.ErrorReporter) (factoryvisualization.Service, error)

// NewActivationOpening captures only the fixed activation behavior.
func NewActivationOpening(peer recordings.Service) ActivationOpening {
	return activationlifecyclewire.NewOwner(peer).Open
}

// NewProjectionOpening captures only the fixed projection behavior.
func NewProjectionOpening(peer recordings.Service) ProjectionOpening {
	return liveviewprojectionwire.NewOwner(peer).Open
}

// NewScopeOpening receives each already constructed owner directly. Opening
// allocates scoped state without constructing another dependency graph.
func NewScopeOpening(activation ActivationOpening, projection ProjectionOpening, presentation factoryvisualization.ResponsePresentation, peer recordings.Service) ScopeOpening {
	owner := internalservice.NewScopeOwner(activation, projection, presentation, peer)
	return func(source factoryvisualization.Source, clock factoryvisualization.Clock, sink factoryvisualization.Sink, reportError factoryvisualization.ErrorReporter) (factoryvisualization.Service, error) {
		root, err := owner.Open(source, clock, sink, reportError)
		if err != nil {
			return nil, err
		}
		return root, nil
	}
}
