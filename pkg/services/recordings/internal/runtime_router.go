package internal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
)

// runtimeLedgerRouter keeps the public Recordings root stable while routing
// session-scoped event operations to the private runtime ledgers it owns.
// The fallback ledger preserves the root contract for callers that append
// factory-wide events before a runtime scope has been opened.
type runtimeLedgerRouter struct {
	mu            sync.RWMutex
	fallback      recordings.Ledger
	routes        map[string]recordings.RuntimeEventLedger
	recorders     []func(interfaces.FactoryEvent)
	typeRecorders []func(interfaces.FactoryEventType)
}

func newRuntimeLedgerRouter(now func() time.Time) *runtimeLedgerRouter {
	fallback := recordingevents.NewRuntimeLedger(nil, now, "recordings-root", nil)
	return &runtimeLedgerRouter{
		fallback: fallback,
		routes:   make(map[string]recordings.RuntimeEventLedger),
	}
}

func (router *runtimeLedgerRouter) register(
	scope string,
	ledger recordings.RuntimeEventLedger,
) error {
	if router == nil || ledger == nil {
		return recordings.ErrInvalidRecordingScope
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = ledger.StreamGenerationID()
	}
	router.mu.Lock()
	router.routes[scope] = ledger
	recorders := append([]func(interfaces.FactoryEvent){}, router.recorders...)
	typeRecorders := append([]func(interfaces.FactoryEventType){}, router.typeRecorders...)
	router.mu.Unlock()
	for _, recorder := range recorders {
		ledger.AddEventRecorder(recorder)
	}
	for _, recorder := range typeRecorders {
		ledger.AddEventTypeRecorder(recorder)
	}
	return nil
}

func (router *runtimeLedgerRouter) unregister(
	scope string,
	ledger recordings.RuntimeEventLedger,
) {
	if router == nil || ledger == nil {
		return
	}
	scope = strings.TrimSpace(scope)
	router.mu.Lock()
	current, ok := router.routes[scope]
	if ok && current != nil && current.StreamGenerationID() == ledger.StreamGenerationID() {
		delete(router.routes, scope)
	}
	router.mu.Unlock()
}

func (router *runtimeLedgerRouter) route(scope string) recordings.Ledger {
	if router == nil {
		return nil
	}
	scope = strings.TrimSpace(scope)
	router.mu.RLock()
	defer router.mu.RUnlock()
	if scope != "" {
		return router.routes[scope]
	}
	if len(router.routes) == 1 {
		for _, ledger := range router.routes {
			return ledger
		}
	}
	return router.fallback
}

func (router *runtimeLedgerRouter) CanonicalEvents() []interfaces.FactoryEvent {
	if router == nil {
		return nil
	}
	router.mu.RLock()
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.RUnlock()
	if len(ledgers) == 0 {
		if fallback == nil {
			return nil
		}
		return fallback.CanonicalEvents()
	}
	events := make([]interfaces.FactoryEvent, 0)
	for _, ledger := range ledgers {
		events = append(events, ledger.CanonicalEvents()...)
	}
	sort.SliceStable(events, func(left, right int) bool {
		return events[left].Context.EventTime.Before(events[right].Context.EventTime)
	})
	return events
}

func (router *runtimeLedgerRouter) Subscribe(
	ctx context.Context,
	reconnect *interfaces.FactoryEventReconnectCursor,
	scope interfaces.FactoryEventReconnectScope,
) (interfaces.FactoryEventStream, error) {
	ledger := router.route(scope.SessionID)
	if ledger == nil {
		return interfaces.FactoryEventStream{}, recordings.ErrReconnectCursorUnavailable
	}
	return ledger.Subscribe(ctx, reconnect, scope)
}

func (router *runtimeLedgerRouter) StreamGenerationID() string {
	if router == nil {
		return ""
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	if len(router.routes) == 1 {
		for _, ledger := range router.routes {
			return ledger.StreamGenerationID()
		}
	}
	if router.fallback == nil {
		return ""
	}
	return router.fallback.StreamGenerationID()
}

func (router *runtimeLedgerRouter) AddEventRecorder(
	recorder func(interfaces.FactoryEvent),
) {
	if router == nil || recorder == nil {
		return
	}
	router.mu.Lock()
	router.recorders = append(router.recorders, recorder)
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.Unlock()
	if fallback != nil {
		fallback.AddEventRecorder(recorder)
	}
	for _, ledger := range ledgers {
		ledger.AddEventRecorder(recorder)
	}
}

func (router *runtimeLedgerRouter) AddEventTypeRecorder(
	recorder func(interfaces.FactoryEventType),
) {
	if router == nil || recorder == nil {
		return
	}
	router.mu.Lock()
	router.typeRecorders = append(router.typeRecorders, recorder)
	ledgers := make([]recordings.RuntimeEventLedger, 0, len(router.routes))
	for _, ledger := range router.routes {
		ledgers = append(ledgers, ledger)
	}
	fallback := router.fallback
	router.mu.Unlock()
	if fallback != nil {
		fallback.AddEventTypeRecorder(recorder)
	}
	for _, ledger := range ledgers {
		ledger.AddEventTypeRecorder(recorder)
	}
}

func (router *runtimeLedgerRouter) AppendRecordedEvent(
	event interfaces.FactoryEvent,
) {
	if router == nil {
		return
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	if ledger != nil {
		ledger.AppendRecordedEvent(event)
	}
}

func (router *runtimeLedgerRouter) AppendRecordedEventWithValidation(
	event interfaces.FactoryEvent,
	validate func(interfaces.FactoryEvent) error,
) (interfaces.FactoryEvent, error) {
	if router == nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("recordings ledger router is unavailable")
	}
	scope := ""
	if event.Context.SessionID != nil {
		scope = strings.TrimSpace(*event.Context.SessionID)
	}
	ledger := router.route(scope)
	appender, ok := ledger.(interface {
		AppendRecordedEventWithValidation(
			interfaces.FactoryEvent,
			func(interfaces.FactoryEvent) error,
		) (interfaces.FactoryEvent, error)
	})
	if !ok {
		return interfaces.FactoryEvent{}, fmt.Errorf("recordings ledger does not support atomic append")
	}
	return appender.AppendRecordedEventWithValidation(event, validate)
}

var _ recordings.Ledger = (*runtimeLedgerRouter)(nil)
