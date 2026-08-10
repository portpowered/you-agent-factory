package wire

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// NewOpeningPresentationOwner constructs the process-scoped owner used by
// transport adapters to keep streams, callbacks, and live bindings out of
// opening operation requests.
func NewOpeningPresentationOwner() factorysessions.OpeningPresentationOwner {
	return &openingPresentationOwner{scopes: make(map[factorysessions.OpeningScopeID]openingScope)}
}

type openingScope struct {
	application      *factorysessions.ApplicationOpeningScope
	directJavaScript *factorysessions.DirectJavaScriptRunScope
	stdio            *factorysessions.StdioOpeningScope
}

type openingPresentationOwner struct {
	mu     sync.RWMutex
	nextID uint64
	scopes map[factorysessions.OpeningScopeID]openingScope
}

func (o *openingPresentationOwner) RegisterApplication(scope factorysessions.ApplicationOpeningScope) (factorysessions.OpeningScopeID, error) {
	if scope.RuntimeHostObserver == nil && scope.Completion == nil && scope.RuntimeHTTPServicesBound == nil && scope.HistoricalReplayBound == nil && scope.VisualizationSink == nil {
		return "", errors.New("application opening scope has no presentation owner")
	}
	scope = gateApplicationCompletion(scope)
	return o.register(openingScope{application: &scope})
}

func (o *openingPresentationOwner) Application(id factorysessions.OpeningScopeID) (factorysessions.ApplicationOpeningScope, bool) {
	o.mu.RLock()
	scope, ok := o.scopes[id]
	o.mu.RUnlock()
	if !ok || scope.application == nil {
		return factorysessions.ApplicationOpeningScope{}, false
	}
	return *scope.application, true
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

func (o *openingPresentationOwner) ObserveHost(id factorysessions.OpeningScopeID, binding factorysessions.RuntimeHostBinding) {
	if scope, ok := o.Application(id); ok && scope.RuntimeHostObserver != nil {
		scope.RuntimeHostObserver(binding)
	}
	if scope, ok := o.DirectJavaScript(id); ok && scope.RuntimeHostObserver != nil {
		scope.RuntimeHostObserver(binding)
	}
}

func (o *openingPresentationOwner) Close(id factorysessions.OpeningScopeID) {
	if id == "" {
		return
	}
	o.mu.Lock()
	delete(o.scopes, id)
	o.mu.Unlock()
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

func gateApplicationCompletion(scope factorysessions.ApplicationOpeningScope) factorysessions.ApplicationOpeningScope {
	if scope.Completion == nil {
		return scope
	}
	ready := make(chan struct{})
	var publish sync.Once
	observer := scope.RuntimeHostObserver
	scope.RuntimeHostObserver = func(binding factorysessions.RuntimeHostBinding) {
		publish.Do(func() {
			if observer != nil {
				observer(binding)
			}
			close(ready)
		})
	}
	completion := scope.Completion
	scope.Completion = func(ctx context.Context) error {
		select {
		case <-ready:
			return completion(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return scope
}
