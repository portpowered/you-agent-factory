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
	// DeadLetterRelativePath is kept below the loaded runtime base so terminal
	// delivery failures remain owned by the Factory Session runtime.
	DeadLetterRelativePath = ".you-agent-factory/webhooks/dead-letter.jsonl"
)

// SecretResolver resolves a declaration's reference without exposing secret
// material in Factory definitions or canonical events.
type SecretResolver func(
	context.Context,
	factorydefinitions.LoadedFactorySource,
	string,
) (string, error)

// DeadLetterAppender appends one already-encoded JSON Lines record to the
// session-owned runtime path. The appender owns directory creation and file
// permissions; the Webhooks service owns record redaction and line framing.
type DeadLetterAppender func(string, []byte) error

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
	DeadLetterPath   string
}
