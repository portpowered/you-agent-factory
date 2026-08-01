// Package service implements the Factory Visualization root lifecycle.
package service

import (
	"context"
	"errors"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Service owns the retained event projection, reconnect cursor, and live
// subscription lifecycle for one Factory visualization.
type Service struct {
	activation activationlifecycle.Service
	projection liveviewprojection.Service

	presentationOwner responsePresentationOwner

	source      Source
	recordings  recordings.Service
	clock       Clock
	sink        Sink
	reportError ErrorReporter

	mu              sync.Mutex
	presentationSeq int
	presentations   map[PresentationSessionID]*rootPresentationSession
}

// New constructs an inert Factory visualization service.
func New(
	source Source,
	peer recordings.ProjectionService,
	clock Clock,
	sink Sink,
	reportError ErrorReporter,
) (*Service, error) {
	switch {
	case source == nil:
		return nil, errors.New("initialize Factory visualization: event source is required")
	case peer == nil:
		return nil, errors.New("initialize Factory visualization: projection service is required")
	case clock == nil:
		return nil, errors.New("initialize Factory visualization: clock is required")
	case sink == nil:
		return nil, errors.New("initialize Factory visualization: presentation sink is required")
	}
	recordingsPeer, err := recordingsPeerFromProjectionService(peer)
	if err != nil {
		return nil, err
	}
	activation, err := activationlifecyclewire.NewService(
		activationEventSource(source),
		recordingsPeer,
		clock,
		activationViewSink(sink),
		activationlifecycle.ErrorReporter(reportError),
	)
	if err != nil {
		return nil, err
	}
	projection, err := liveviewprojectionwire.NewService(
		source,
		recordingsPeer,
		clock,
		projectionSink(sink),
		reportError,
	)
	if err != nil {
		return nil, err
	}
	return assembleRoot(
		activation,
		projection,
		defaultResponsePresentationOwner(),
		source,
		recordingsPeer,
		clock,
		sink,
		reportError,
	)
}

// Start subscribes once to retained-then-live canonical Factory events.
func (s *Service) Start(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return errors.New("start Factory visualization: service is required")
	}
	return s.activation.Start(ctx)
}

// Stop cancels and joins the event subscription, then emits one final view
// while the Factory Runtime is still active.
func (s *Service) Stop(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return nil
	}
	return s.activation.Stop(ctx)
}

// Wait blocks until the live Factory event subscription exits.
func (s *Service) Wait(ctx context.Context) error {
	if s == nil || s.activation == nil {
		return nil
	}
	return s.activation.Wait(ctx)
}

func adaptSink(sink Sink) liveviewprojection.Sink {
	return sink
}

type activationSourceAdapter struct {
	source Source
}

func (a activationSourceAdapter) SubscribeFactoryEvents(
	ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if a.source == nil {
		return nil, errors.New("subscribe Factory visualization events: event source is required")
	}
	return a.source.SubscribeFactoryEvents(ctx, reconnect, scope)
}

func (a activationSourceAdapter) GetEngineObservation(
	ctx context.Context,
) (*activationlifecycle.EngineObservation, error) {
	if a.source == nil {
		return nil, errors.New("read Factory visualization engine observation: event source is required")
	}
	facts, err := a.source.GetRuntimeSnapshotFacts(ctx)
	if err != nil {
		return nil, err
	}
	if facts == nil {
		return nil, nil
	}
	return &activationlifecycle.EngineObservation{
		TickCount:            facts.RuntimeObservation.TickCount,
		ActiveThrottlePauses: facts.ActiveThrottlePauses,
	}, nil
}

type activationSinkAdapter struct {
	sink Sink
}

func (a activationSinkAdapter) PresentFactoryView(view activationlifecycle.View) {
	if a.sink == nil {
		return
	}
	a.sink.PresentFactoryView(View{
		Runtime: RuntimeObservation{
			TickCount: view.EngineObservation.TickCount,
		},
		RenderData: view.RenderData,
		ObservedAt: view.ObservedAt,
	})
}
