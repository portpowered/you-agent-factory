// Package wire constructs the Webhooks root from exact application effects.
package wire

import (
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	internalservice "github.com/portpowered/infinite-you/pkg/services/webhooks/internal/service"
)

// HTTPClient is the selected outbound delivery effect.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Clock supplies delivery timestamps and cancellable retry scheduling.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// NewService constructs an inert root; Factory Sessions owns each activation.
func NewService(events recordings.Service, client HTTPClient,
	secrets webhooks.SecretResolver, clock Clock,
	deadLetters webhooks.DeadLetterAppender, logger logging.Logger) webhooks.Service {
	return internalservice.New(events, client, secrets, clock, deadLetters, logger)
}
