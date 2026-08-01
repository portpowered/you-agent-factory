// Package service implements the Factory Visualization root lifecycle.
package service

import (
	"context"
	"errors"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Service owns the retained event projection, reconnect cursor, and live
// subscription lifecycle for one Factory visualization.
type Service struct {
	activation activationlifecycle.Service
	projection liveviewprojection.Service

	presentationOwner responseeventpresentation.Service

	source      Source
	recordings  recordings.Service
	clock       Clock
	sink        Sink
	reportError ErrorReporter

	mu              sync.Mutex
	presentationSeq int
	presentations   map[PresentationSessionID]*rootPresentationSession
}

// New assembles an inert Factory Visualization root from already-constructed
// parent-private owners. The owning wire package composes those owners before
// calling this implementation constructor.
func New(
	activation activationlifecycle.Service,
	projection liveviewprojection.Service,
	presentation responseeventpresentation.Service,
	source Source,
	recordingsPeer recordings.Service,
	clock Clock,
	sink Sink,
	reportError ErrorReporter,
) (*Service, error) {
	return assembleRoot(
		activation,
		projection,
		presentation,
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

// ActivationEventSourceAdapter maps the public Visualization source port to
// the activation lifecycle owner's private event-source port. It is exported
// only within the parent-private internal package so the owning wire package
// can construct the adapter without exposing activation internals to peers.
type ActivationEventSourceAdapter struct {
	Source Source
}

func (a ActivationEventSourceAdapter) SubscribeFactoryEvents(
	ctx context.Context,
	reconnect *factorydefinitions.FactoryEventReconnectCursor,
	scope factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if a.Source == nil {
		return nil, errors.New("subscribe Factory visualization events: event source is required")
	}
	return a.Source.SubscribeFactoryEvents(ctx, reconnect, scope)
}

func (a ActivationEventSourceAdapter) GetEngineObservation(
	ctx context.Context,
) (*activationlifecycle.EngineObservation, error) {
	if a.Source == nil {
		return nil, errors.New("read Factory visualization engine observation: event source is required")
	}
	facts, err := a.Source.GetRuntimeSnapshotFacts(ctx)
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

// ActivationViewSinkAdapter maps activation lifecycle views to the public
// Visualization view sink without publishing activation-owned view types.
type ActivationViewSinkAdapter struct {
	Sink Sink
}

func (a ActivationViewSinkAdapter) PresentFactoryView(view activationlifecycle.View) {
	if a.Sink == nil {
		return
	}
	a.Sink.PresentFactoryView(View{
		Runtime: RuntimeObservation{
			TickCount: view.EngineObservation.TickCount,
		},
		RenderData: view.RenderData,
		ObservedAt: view.ObservedAt,
	})
}
