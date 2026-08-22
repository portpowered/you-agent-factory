package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
)

// responseEventSchemaID identifies the source-native JSON shape Events
// receives for every mirrored Factory Session response event.
const responseEventSchemaID = events.SchemaID("factory-response-event.v1")

// responseEventSourceType names the Factory Sessions response-event
// producer family within the injected Events root's identity space.
const responseEventSourceType = events.SourceType("factory-session-response-event")

// ResponseStream owns process-scoped response-event identity generation and
// publishes every Factory Session response event through the injected
// Events root before its session-owned store ever retains it, so the two
// surfaces can never observe different records or aggregate ordering.
// NewEventStore additionally binds the store to that same injected Events
// root (responseeventstore.NewSessionResponseEventStoreWithEventsAuthority),
// so Subscribe's delivered content is read back from Events rather than
// trusted solely from the store's own retained copy: the store's tiered
// retention/gap bookkeeping (which Events has no equivalent for) still
// decides which sequences remain deliverable, but the bytes returned for a
// still-deliverable sequence come from the injected Events root. All runtime
// state is allocated only when the outer service binds an explicit runtime
// clock.
type ResponseStream struct {
	eventIDs        responseeventstore.ResponseEventIDGenerator
	retentionLimits *responseeventstore.RetentionLimits
	events          events.Service
	logger          logging.Logger
}

var _ responsestreamservice.Service = (*ResponseStream)(nil)

func New(
	eventIDs responseeventstore.ResponseEventIDGenerator,
	limits *factorysessions.ResponseEventRetentionLimits,
	eventsService events.Service,
	logger ...logging.Logger,
) (*ResponseStream, error) {
	if eventIDs == nil {
		return nil, errors.New("construct Factory Session response streams: event ID generator is required")
	}
	if eventsService == nil {
		return nil, errors.New("construct Factory Session response streams: Events root is required")
	}
	service := &ResponseStream{
		eventIDs: eventIDs,
		events:   eventsService,
		logger:   logging.EnsureLogger(firstLogger(logger)),
	}
	if limits != nil {
		service.retentionLimits = &responseeventstore.RetentionLimits{
			MaxEvents:                limits.MaxEvents,
			MaxBytes:                 limits.MaxBytes,
			CompletedRetentionWindow: limits.CompletedRetentionWindow,
		}
	}
	return service, nil
}

func firstLogger(loggers []logging.Logger) logging.Logger {
	if len(loggers) == 0 {
		return nil
	}
	return loggers[0]
}

func (s *ResponseStream) NewEventStore(sessionID string, clock factoryruntime.Clock) (*responseeventstore.SessionResponseEventStore, error) {
	if s == nil || s.eventIDs == nil {
		return nil, errors.New("Factory Session response-stream service is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("Factory Session response-stream session ID is required")
	}
	limits := responseeventstore.DefaultRetentionLimits()
	if s.retentionLimits != nil {
		limits = *s.retentionLimits
	}
	trimmedSessionID := strings.TrimSpace(sessionID)
	store, err := responseeventstore.NewSessionResponseEventStoreWithEventsAuthority(
		trimmedSessionID, clock, limits, s.eventIDs, s.events, responseEventTopic(trimmedSessionID),
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

// Publish submits event to the injected Events root and, only once Events
// has accepted it, retains the identical record in store: Events.Append is
// the sole authority that accepts or rejects the record and assigns its
// aggregate position, and store.PublishThroughAuthority holds the store's
// write lock across that exact Append call, so Events' decision and the
// store's retained state are one atomic step. The store's session-monotonic
// Sequence is taken directly from the aggregate position Events assigned
// (rather than predicted beforehand), so no external authority ever accepts
// an identity the store did not itself produce. If Events rejects the
// append, Publish fails and the store's retained state and subscribers are
// left completely untouched -- no partial, mirrored, or locally-only record
// is ever produced, and the store can never retain a record Events did not
// accept.
func (s *ResponseStream) Publish(store *responseeventstore.SessionResponseEventStore, event responseevents.FactoryResponseEvent) (responseevents.FactoryResponseEvent, error) {
	if store == nil {
		return responseevents.FactoryResponseEvent{}, errors.New("Factory Session response-event store is required")
	}
	eventID := strings.TrimSpace(s.eventIDs())
	if eventID == "" {
		return responseevents.FactoryResponseEvent{}, errors.New("response event ID generator returned an empty identity")
	}
	sessionID := store.FactorySessionID()

	published, err := store.PublishThroughAuthority(event, func(prepared responseevents.FactoryResponseEvent, sequenceHint int64) (int64, string, error) {
		prepared.Sequence = sequenceHint
		prepared.EventID = eventID
		payload, err := json.Marshal(prepared)
		if err != nil {
			return 0, "", fmt.Errorf("encode factory session response event for Events: %w", err)
		}
		result, err := s.events.Append(context.Background(), events.AppendRequest{
			Topic:          responseEventTopic(sessionID),
			SourceType:     responseEventSourceType,
			SourceID:       events.SourceID(sessionID),
			SourceSequence: events.SourceSequence(sequenceHint),
			SourceEventID:  events.SourceEventID(eventID),
			SchemaID:       responseEventSchemaID,
			Payload:        payload,
		})
		if err != nil {
			return 0, "", fmt.Errorf("factory session response event rejected by Events: %w", err)
		}
		return int64(result.Record.ID.Position), eventID, nil
	})
	if err != nil {
		return responseevents.FactoryResponseEvent{}, err
	}
	return published, nil
}

// responseEventTopic names the Events topic one Factory Session's response
// events mirror into: one topic per session, matching the documented Topic
// naming example in pkg/services/events/identity.go.
func responseEventTopic(factorySessionID string) events.Topic {
	return events.Topic(fmt.Sprintf("factory-session/%s/response-events", factorySessionID))
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
