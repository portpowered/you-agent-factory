package apisurface

import (
	"context"
	"errors"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ErrFactoryResponseEventStreamExpired reports that the completed session's
// ephemeral response-event retention window elapsed before subscription.
var ErrFactoryResponseEventStreamExpired = errors.New("factory response event stream expired")

// FactoryResponseEventRecord is one transport-neutral serialized observation
// returned by a session-owned ephemeral response-event subscription.
type FactoryResponseEventRecord struct {
	Sequence int64
	Kind     string
	Data     []byte
}

// FactoryResponseEventSubscription is the transport-independent cursor exposed
// by one session-owned ephemeral response-event store. The HTTP transport owns
// detachment; canceling that observer must not cancel the Factory Session run.
type FactoryResponseEventSubscription interface {
	Next(ctx context.Context) ([]FactoryResponseEventRecord, error)
	Detach()
}

// DurableSessionAPI groups the durable application capabilities exposed to
// transport mapping and retained compatibility facades.
type DurableSessionAPI interface {
	DurableSessionExecutionAPI
	DurableSessionListingAPI
	DurableSessionProjectionAPI
	DurableSessionLifecycleAPI
}

// ValidateWritableNamedFactoryName enforces the public named-factory contract
// for create/import paths. The reserved default-current identifier is valid for
// readback only and must never be persisted as a customer-named factory.
func ValidateWritableNamedFactoryName(name factoryapi.FactoryName) error {
	if err := factoryconfig.ValidateNamedFactoryName(string(name)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNamedFactoryName, err)
	}
	if name == DefaultCurrentFactoryName {
		return fmt.Errorf("%w: %q is reserved for current-factory readback", ErrInvalidNamedFactoryName, name)
	}
	return nil
}
