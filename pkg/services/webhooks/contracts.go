// Package webhooks owns outbound Factory Event delivery.
package webhooks

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	EventIDHeader       = "X-Factory-Event-ID"
	TimestampHeader     = "X-Factory-Webhook-Timestamp"
	SignatureHeader     = "X-Factory-Webhook-Signature"
	SignatureVersionV1  = "v1"
	MaxResponseBodySize = 4 << 10
)

// SecretResolver resolves a declaration's reference without exposing secret
// material in Factory definitions or canonical events.
type SecretResolver func(
	context.Context,
	factorydefinitions.LoadedFactorySource,
	string,
) (string, error)

// Service starts session-scoped outbound subscriptions.
type Service interface {
	Start(context.Context, StartRequest) (Subscription, error)
}

// Subscription owns all endpoint subscribers started for one Factory Session.
type Subscription func(context.Context) error

// StartRequest binds one loaded Factory definition to its canonical session
// event stream. ActivationCursor is the last event retained at activation and
// prevents delivery of pre-activation history.
type StartRequest struct {
	Definitions      []factorydefinitions.FactoryWebhookConfig
	Events           recordings.Service
	Scope            recordings.CanonicalEventScope
	ActivationCursor *recordings.CanonicalEventCursor
	RuntimeSource    factorydefinitions.LoadedFactorySource
}
