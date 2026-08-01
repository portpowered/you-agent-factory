// Package http adapts Automations HTTP operations through the accepted
// Automations root contract. Request decoding, representation mapping, service
// invocation, error mapping, and response encoding for owned Automations HTTP
// operations remain here with the owning service.
package http

import (
	"context"
	"errors"
	"reflect"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// Adapter maps Automations HTTP operations through the accepted root contract
// without importing Automations internals or owning canonical automation state.
type Adapter struct {
	root automations.Root
}

// NewAdapter constructs the Automations HTTP adapter bound to the accepted root.
func NewAdapter(root automations.Root) *Adapter {
	if !rootAvailable(root) {
		return nil
	}
	return &Adapter{root: root}
}

// Root returns the accepted Automations root consumed by adapter-owned operations.
func (a *Adapter) Root() automations.Root {
	if a == nil || !rootAvailable(a.root) {
		return automations.Root{}
	}
	return a.root
}

func rootAvailable(root automations.Root) bool {
	if root.Operations == nil {
		return false
	}

	value := reflect.ValueOf(root.Operations)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func (a *Adapter) invokeSourceStatus(
	ctx context.Context,
	request automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	if a == nil || !rootAvailable(a.root) {
		return automations.SourceStatusResult{}, errors.New("automations root is required")
	}
	return a.root.SourceStatus(ctx, request)
}
