package inference_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const providerExitNormalizationSessionID = "provider-exit-normalization-session"

// TestProviderNonZeroExitMapsToPublicFailure proves a provider process that exits
// non-zero is normalized into a public failed Work outcome with matching Factory
// Event and Provider Session diagnostics, without hanging or reporting success.
func TestProviderNonZeroExitMapsToPublicFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCursor,
		"cursor-test-model",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider non-zero exit"}`))

	const exitCode = 42
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: exitCode,
		Stdout: []byte(
			`{"type":"system","subtype":"init","session_id":"` + providerExitNormalizationSessionID + `"}` + "\n",
		),
		Stderr: []byte("provider process crashed unexpectedly"),
	})

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 after provider non-zero exit", got)
	}
	if runner.CallCount() < 1 {
		t.Fatalf("provider command runner calls = %d, want at least 1", runner.CallCount())
	}
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one failed work item",
			session.Runtime.Progress.Categories,
		)
	}

	failure := terminalInferenceFailureObservation(t, events)
	if failure.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("terminal inference outcome = %q, want failed", failure.Outcome)
	}
	if failure.FailureDetail == nil || failure.FailureDetail.Reason == "" {
		t.Fatalf("terminal inference failure detail = %#v, want public failure reason", failure.FailureDetail)
	}
	if failure.ProviderSession == nil || failure.ProviderSession.Id == nil ||
		*failure.ProviderSession.Id != providerExitNormalizationSessionID {
		t.Fatalf(
			"terminal inference provider session = %#v, want public Provider Session %q",
			failure.ProviderSession,
			providerExitNormalizationSessionID,
		)
	}
}

func terminalInferenceFailureObservation(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var terminal factoryapi.InferenceResponseEventPayload
	found := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		terminal = payload
		found = true
	}
	if !found {
		t.Fatalf("factory events missing terminal INFERENCE_RESPONSE failure")
	}
	return terminal
}
