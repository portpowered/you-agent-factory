package cli_rest_journeys_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var validatePersistCLIProcess support.ApplicationProcess

// Validation never admits a runtime. Observations travel with each command's
// context so parallel scenarios cannot count another scenario's external effects.
type validationObservation struct {
	providerCalls atomic.Int64
	apiStarts     atomic.Int64
}

type validationObservationKey struct{}

func validationContext(t *testing.T, observation *validationObservation) context.Context {
	t.Helper()
	return context.WithValue(t.Context(), validationObservationKey{}, observation)
}

type validationCommandRunner struct{}

func (validationCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if observation, ok := ctx.Value(validationObservationKey{}).(*validationObservation); ok {
		observation.providerCalls.Add(1)
	}
	return platformprocess.CommandResult{}, fmt.Errorf("validation must not dispatch a provider")
}

func rejectValidationHostStart(ctx context.Context, _ platformhttpserver.StartRequest) error {
	if observation, ok := ctx.Value(validationObservationKey{}).(*validationObservation); ok {
		observation.apiStarts.Add(1)
	}
	return fmt.Errorf("validation must not start a runtime host")
}
