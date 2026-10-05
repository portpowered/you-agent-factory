package cli_rest_journeys_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
func initializeFactorydefinitionstransportsclivalidatepersistFixture(t *testing.T) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		ProviderCommandRunner: validationCommandRunner{},
		APIServerStarter:      rejectValidationHostStart,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build Factory validation CLI process: %v\n", err)
		t.Fatal("customer fixture setup failed; see preceding diagnostic")
	}
	validatePersistCLIProcess = process
	t.Cleanup(func() {
		exitCode := 0
		//nolint:testsleep // This deadline bounds process teardown after all scenario-owned commands have joined.
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Close(closeContext); err != nil {
			fmt.Fprintf(os.Stderr, "close Factory validation CLI process: %v\n", err)
			exitCode = 1
		}
		if exitCode != 0 {
			t.Error("customer fixture cleanup failed; see preceding diagnostic")
		}
	})
}
func resetfactorydefinitionstransportsclivalidatepersist0State() {
	var freshValidatePersistCLIProcess support.ApplicationProcess
	validatePersistCLIProcess = freshValidatePersistCLIProcess
}
