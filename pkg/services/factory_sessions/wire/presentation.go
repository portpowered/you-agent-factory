package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// NewOpeningPresentationOwner constructs the process-scoped owner used by
// transport adapters to keep direct-protocol streams and event consumers out
// of opening operation requests.
func NewOpeningPresentationOwner() factorysessions.OpeningPresentationOwner {
	return &openingPresentationOwner{scopes: make(map[factorysessions.OpeningScopeID]openingScope)}
}

type openingScope struct {
	directJavaScript *factorysessions.DirectJavaScriptRunScope
	stdio            *factorysessions.StdioOpeningScope
	invocationEvents *factorysessions.InvocationEventScope
}

type openingPresentationOwner struct {
	mu     sync.RWMutex
	nextID uint64
	scopes map[factorysessions.OpeningScopeID]openingScope
}

func (o *openingPresentationOwner) RegisterDirectJavaScript(scope factorysessions.DirectJavaScriptRunScope) (factorysessions.OpeningScopeID, error) {
	if scope.Output == nil && scope.RuntimeHostObserver == nil {
		return "", errors.New("direct JavaScript opening scope has no presentation owner")
	}
	return o.register(openingScope{directJavaScript: &scope})
}

func (o *openingPresentationOwner) DirectJavaScript(id factorysessions.OpeningScopeID) (factorysessions.DirectJavaScriptRunScope, bool) {
	o.mu.RLock()
	scope, ok := o.scopes[id]
	o.mu.RUnlock()
	if !ok || scope.directJavaScript == nil {
		return factorysessions.DirectJavaScriptRunScope{}, false
	}
	return *scope.directJavaScript, true
}

func (o *openingPresentationOwner) RegisterStdio(scope factorysessions.StdioOpeningScope) (factorysessions.OpeningScopeID, error) {
	if scope.Input == nil || scope.Output == nil {
		return "", errors.New("stdio opening scope requires input and output")
	}
	return o.register(openingScope{stdio: &scope})
}

func (o *openingPresentationOwner) Stdio(id factorysessions.OpeningScopeID) (factorysessions.StdioOpeningScope, bool) {
	o.mu.RLock()
	scope, ok := o.scopes[id]
	o.mu.RUnlock()
	if !ok || scope.stdio == nil {
		return factorysessions.StdioOpeningScope{}, false
	}
	return *scope.stdio, true
}

func (o *openingPresentationOwner) RegisterInvocationEvents(scope factorysessions.InvocationEventScope) (factorysessions.OpeningScopeID, error) {
	if scope.Consume == nil {
		return "", errors.New("invocation event scope requires a consumer")
	}
	return o.register(openingScope{invocationEvents: &scope})
}

func (o *openingPresentationOwner) InvocationEvents(id factorysessions.OpeningScopeID) (factorysessions.FactoryEventConsumer, bool) {
	scope, ok := o.invocationEventScope(id)
	if !ok {
		return nil, false
	}
	return scope.Consume, true
}

func (o *openingPresentationOwner) invocationEventScope(id factorysessions.OpeningScopeID) (factorysessions.InvocationEventScope, bool) {
	o.mu.RLock()
	scope, ok := o.scopes[id]
	o.mu.RUnlock()
	if !ok || scope.invocationEvents == nil || scope.invocationEvents.Consume == nil {
		return factorysessions.InvocationEventScope{}, false
	}
	return *scope.invocationEvents, true
}

func (o *openingPresentationOwner) StartFactoryEventBridge(
	ctx context.Context,
	reader roles.FactoryEventReader,
	id factorysessions.OpeningScopeID,
) (interface {
	Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error
}, error) {
	if ctx == nil {
		return nil, errors.New("Factory Event bridge context is required")
	}
	if reader == nil {
		return nil, errors.New("Factory Event bridge service is required")
	}
	scope, ok := o.invocationEventScope(id)
	if !ok {
		return nil, fmt.Errorf("Factory Event scope %q is not registered", id)
	}
	sessionID := strings.TrimSpace(scope.FactorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := reader.SubscribeFactoryEventsForSession(
		streamCtx, sessionID, nil,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe invocation Factory Events: %w", err)
	}
	if stream == nil || stream.Events == nil {
		cancel()
		return nil, errors.New("subscribe invocation Factory Events: stream is unavailable")
	}
	bridge := &factoryEventBridge{
		sessionID: sessionID,
		consume:   scope.Consume,
		cancel:    cancel,
		done:      make(chan struct{}),
		seen:      make(map[string]struct{}),
	}
	bridge.presentUnseen(stream.History)
	go func() {
		defer close(bridge.done)
		for event := range stream.Events {
			bridge.presentUnseen([]factorydefinitions.FactoryEvent{event})
		}
	}()
	return bridge, nil
}

func (o *openingPresentationOwner) Close(id factorysessions.OpeningScopeID) {
	if id == "" {
		return
	}
	o.mu.Lock()
	delete(o.scopes, id)
	o.mu.Unlock()
}

type factoryEventBridge struct {
	mu        sync.Mutex
	sessionID string
	consume   factorysessions.FactoryEventConsumer
	cancel    context.CancelFunc
	done      chan struct{}
	seen      map[string]struct{}
}

func (bridge *factoryEventBridge) Finish(
	ctx context.Context,
	reader roles.FactoryEventReader,
	outcome factorysessions.FactoryInvocationOutcome,
) error {
	if bridge == nil {
		return nil
	}
	if bridge.cancel != nil {
		bridge.cancel()
	}
	if bridge.done != nil {
		<-bridge.done
	}
	// Read the same session the bridge subscribed to. A canceled invocation may
	// not have produced a public result carrying a session ID yet, and using the
	// result here would silently fall back to ~default.
	events, err := readFactoryEventHistory(ctx, reader, bridge.sessionID)
	if err != nil {
		return err
	}
	bridge.presentUnseen(events)
	return nil
}

func (bridge *factoryEventBridge) presentUnseen(events []factorydefinitions.FactoryEvent) {
	if bridge == nil || bridge.consume == nil {
		return
	}
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	unseen := make([]factorydefinitions.FactoryEvent, 0, len(events))
	for _, event := range events {
		key := factoryEventPresentationKey(event)
		if _, ok := bridge.seen[key]; ok {
			continue
		}
		bridge.seen[key] = struct{}{}
		unseen = append(unseen, event.Clone())
	}
	if len(unseen) > 0 {
		bridge.consume(unseen)
	}
}

func factoryEventPresentationKey(event factorydefinitions.FactoryEvent) string {
	encoded, err := json.Marshal(event)
	if err == nil {
		return string(encoded)
	}
	return event.Id + "\x00" + string(event.Payload)
}

func readFactoryEventHistory(
	ctx context.Context,
	reader roles.FactoryEventReader,
	sessionID string,
) ([]factorydefinitions.FactoryEvent, error) {
	if reader == nil {
		return nil, errors.New("Factory Session event reader is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	var (
		stream *factorydefinitions.FactoryEventStream
		err    error
	)
	if sessionID != "" && sessionID != factorysessions.DefaultSessionID {
		stream, err = reader.ReadDurableFactorySessionEventStream(
			ctx, sessionID, factorysessions.EventReconnectRequest{},
		)
	} else {
		stream, err = reader.SubscribeFactoryEventsForSession(
			ctx, factorysessions.DefaultSessionID, nil,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("read invocation Factory Events: %w", err)
	}
	if stream == nil {
		return nil, errors.New("read invocation Factory Events: stream is unavailable")
	}
	events := make([]factorydefinitions.FactoryEvent, len(stream.History))
	for index := range stream.History {
		events[index] = stream.History[index].Clone()
	}
	return events, nil
}

func (o *openingPresentationOwner) register(scope openingScope) (factorysessions.OpeningScopeID, error) {
	if o == nil {
		return "", errors.New("opening presentation owner is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextID++
	id := factorysessions.OpeningScopeID("opening-" + strconv.FormatUint(o.nextID, 10))
	if _, exists := o.scopes[id]; exists {
		return "", fmt.Errorf("opening presentation scope %q already exists", id)
	}
	o.scopes[id] = scope
	return id, nil
}
