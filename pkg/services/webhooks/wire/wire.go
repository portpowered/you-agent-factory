// Package wire constructs the Webhooks root from exact application effects.
package wire

import (
	"net/http"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	internalservice "github.com/portpowered/infinite-you/pkg/services/webhooks/internal/service"
)

// NewService constructs an inert Webhooks root. It starts no subscribers;
// Factory Sessions owns activation and shutdown for each runtime.
func NewService(
	httpClient interface {
		Do(*http.Request) (*http.Response, error)
	},
	secretResolver webhooks.SecretResolver,
	clockSource interface {
		Now() time.Time
		After(time.Duration) <-chan time.Time
	},
	deadLetterAppender webhooks.DeadLetterAppender,
	logger logging.Logger,
) webhooks.Service {
	if clockSource == nil {
		clockSource = clock.Real{}
	}
	return internalservice.NewWithDeadLetterAppender(
		httpClient,
		secretResolver,
		clockSource,
		deadLetterAppender,
		logger,
	)
}
