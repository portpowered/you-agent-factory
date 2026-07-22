package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// ResponseStream owns process-scoped response-event identity generation. All
// runtime state is allocated only when the outer service binds an explicit
// runtime clock.
type ResponseStream struct {
	eventIDs responseeventstore.ResponseEventIDGenerator
}

var _ responsestreamservice.Service = (*ResponseStream)(nil)

func New(eventIDs responseeventstore.ResponseEventIDGenerator) (*ResponseStream, error) {
	if eventIDs == nil {
		return nil, errors.New("construct Factory Session response streams: event ID generator is required")
	}
	return &ResponseStream{eventIDs: eventIDs}, nil
}

func (s *ResponseStream) NewEventStore(sessionID string, clock factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	if s == nil || s.eventIDs == nil {
		return nil, errors.New("Factory Session response-stream service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("Factory Session response-stream session ID is required")
	}
	store, err := responseeventstore.NewSessionResponseEventStoreWithClockAndLimits(
		strings.TrimSpace(sessionID), clock, responseeventstore.DefaultRetentionLimits(), s.eventIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("create Factory Session response-event store: %w", err)
	}
	return store, nil
}

func (s *ResponseStream) NewStreamRegistry(clock factoryruntime.Clock) (*responsestream.Registry, error) {
	if clock == nil {
		return nil, errors.New("Factory Session response-stream clock is required")
	}
	newStream := func() *responsestream.SessionResponseStream {
		return responsestream.NewSessionResponseStream(clock)
	}
	registry := responsestream.NewRegistry(newStream, clock)
	if registry == nil {
		return nil, errors.New("create Factory Session response-stream registry")
	}
	return registry, nil
}

func (s *ResponseStream) Subscribe(ctx context.Context, store *responseeventstore.SessionResponseEventStore, request responsestreamservice.SubscriptionRequest) (*responsestreamservice.Cursor, error) {
	if ctx == nil {
		return nil, errors.New("Factory Session response-event context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.AfterSequence < 0 {
		return nil, responsestreamservice.ErrInvalidCursor
	}
	if store == nil {
		return nil, errors.New("Factory Session response-event store is required")
	}
	selected := make(map[responseevents.Kind]struct{}, len(request.Kinds))
	for _, kind := range request.Kinds {
		if !validKind(kind) {
			return nil, responsestreamservice.ErrInvalidFilter
		}
		selected[kind] = struct{}{}
	}
	options := make([]responseeventstore.SubscribeOption, 0, 1)
	if dispatchID := strings.TrimSpace(request.DispatchID); dispatchID != "" {
		options = append(options, responseeventstore.WithDispatchFilter(dispatchID))
	}
	cursor, err := store.Subscribe(request.AfterSequence, options...)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return cursorValue(cursor), nil
	}
	filtered := &filteredCursor{cursor: cursor, selected: selected}
	return cursorValue(filtered), nil
}

func (s *ResponseStream) Complete(store *responseeventstore.SessionResponseEventStore) {
	if store != nil {
		store.Complete()
	}
}

func (s *ResponseStream) Publish(store *responseeventstore.SessionResponseEventStore, event responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	if store == nil {
		return responseevents.FactoryResponseEvent{}, errors.New("Factory Session response-event store is required")
	}
	return store.Publish(event)
}

func (s *ResponseStream) NewPublisher(stream *responsestream.SessionResponseStream, observer responsestream.DiagnosticsObserver) *responsestreamservice.Publisher {
	publisher := responsestream.NewPublisher(stream, observer)
	return &responsestreamservice.Publisher{
		PublishEvent:     publisher.Publish,
		ReportCompaction: publisher.ReportCompaction,
		ReadDiagnostics:  publisher.Diagnostics,
	}
}

func (s *ResponseStream) NewCursorTracker(store cursors.Store, identity cursors.StorageIdentity) (*responsestreamservice.Tracker, error) {
	tracker, err := cursors.NewTracker(store, identity)
	if err != nil {
		return nil, err
	}
	return &responsestreamservice.Tracker{
		RestoreCursor: tracker.Restore,
		AdvanceCursor: tracker.Advance,
		CurrentCursor: tracker.Current,
	}, nil
}

func (s *ResponseStream) Close(store *responseeventstore.SessionResponseEventStore) {
	if store != nil {
		store.Close()
	}
}

func validKind(kind responseevents.Kind) bool {
	switch kind {
	case responseevents.KindSession, responseevents.KindRun, responseevents.KindTurn,
		responseevents.KindMessage, responseevents.KindReasoning, responseevents.KindTool,
		responseevents.KindFileChange, responseevents.KindPlan, responseevents.KindProgress,
		responseevents.KindUsage, responseevents.KindError, responseevents.KindStreamGap:
		return true
	default:
		return false
	}
}

type filteredCursor struct {
	cursor   cursorOperations
	selected map[responseevents.Kind]struct{}
}

type cursorOperations interface {
	Next(context.Context) ([]responseevents.FactoryResponseEvent, error)
	Drain() ([]responseevents.FactoryResponseEvent, error)
	Detach()
}

func cursorValue(cursor cursorOperations) *responsestreamservice.Cursor {
	return &responsestreamservice.Cursor{
		NextEvents:   cursor.Next,
		DrainEvents:  cursor.Drain,
		DetachCursor: cursor.Detach,
	}
}

func (c *filteredCursor) Next(ctx context.Context) ([]responseevents.FactoryResponseEvent, error) {
	for {
		events, err := c.cursor.Next(ctx)
		if err != nil {
			return nil, err
		}
		if filtered := c.filter(events); len(filtered) > 0 {
			return filtered, nil
		}
	}
}

func (c *filteredCursor) Drain() ([]responseevents.FactoryResponseEvent, error) {
	events, err := c.cursor.Drain()
	if err != nil {
		return nil, err
	}
	return c.filter(events), nil
}

func (c *filteredCursor) Detach() { c.cursor.Detach() }

func (c *filteredCursor) filter(events []responseevents.FactoryResponseEvent) []responseevents.FactoryResponseEvent {
	filtered := make([]responseevents.FactoryResponseEvent, 0, len(events))
	for _, event := range events {
		if event.Kind == responseevents.KindStreamGap {
			filtered = append(filtered, event)
			continue
		}
		if _, ok := c.selected[event.Kind]; ok {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
