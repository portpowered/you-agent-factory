// Package wire constructs the Webhooks root from exact application effects.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	internalservice "github.com/portpowered/infinite-you/pkg/services/webhooks/internal/service"
)

// NewService constructs an inert root; Factory Sessions owns each activation.
func NewService(events recordings.Service, client webhooks.HTTPClient,
	secrets webhooks.SecretResolver, clock webhooks.Clock,
	deadLetters webhooks.DeadLetterAppender, logger logging.Logger) webhooks.Service {
	return internalservice.New(events, client, secrets, clock, deadLetters, logger)
}
