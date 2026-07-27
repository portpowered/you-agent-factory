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

const (
	codexAuthFailureStderr = `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`
	codexThrottleFailureStderr = "ERROR: selected model is at capacity"
	codexTimeoutFailureStderr = "request timed out after waiting for provider response"
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

// TestProviderAuthRateLimitAndTimeoutRemainDistinct proves authentication,
// rate-limit, and timeout provider failures normalize to publicly distinct
// failure classes through the customer process boundary, including throttle
// exhaustion after default retry limits.
func TestProviderAuthRateLimitAndTimeoutRemainDistinct(t *testing.T) {
	const defaultThrottleRetryCalls = 3 * 3

	tests := []struct {
		name       string
		results    []platformprocess.CommandResult
		wantReason factoryapi.WorkFailureType
		wantCalls  int
	}{
		{
			name: "authentication failure",
			results: []platformprocess.CommandResult{{
				ExitCode: 1,
				Stderr:   []byte(codexAuthFailureStderr),
			}},
			wantReason: factoryapi.WorkFailureTypeAuthFailure,
			wantCalls:  1,
		},
		{
			name:       "rate-limit exhaustion",
			results:    repeatedCodexThrottleCommandResults(12),
			wantReason: factoryapi.WorkFailureTypeThrottled,
			wantCalls:  defaultThrottleRetryCalls,
		},
		{
			name: "timeout failure",
			results: []platformprocess.CommandResult{{
				ExitCode: 1,
				Stderr:   []byte(codexTimeoutFailureStderr),
			}},
			wantReason: factoryapi.WorkFailureTypeTimeout,
			wantCalls:  2,
		},
	}

	seenReasons := make(map[factoryapi.WorkFailureType]struct{}, len(tests))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
				modelprovider.ProviderCodex,
				"gpt-5-codex",
			))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"`+tc.name+`"}`))

			runner := testutil.NewProviderCommandRunner(tc.results...)
			session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
				t,
				dir,
				serviceedges.Edges{ProviderCommandRunner: runner},
				20*time.Second,
			)

			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
			}
			if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
				t.Fatalf("done place tokens = %d, want 0 after %s", got, tc.name)
			}
			if session.Runtime.Progress.Categories.Failed != 1 {
				t.Fatalf(
					"session progress categories = %+v, want one failed work item",
					session.Runtime.Progress.Categories,
				)
			}
			if runner.CallCount() != tc.wantCalls {
				t.Fatalf(
					"provider command runner calls = %d, want %d for %s",
					runner.CallCount(),
					tc.wantCalls,
					tc.name,
				)
			}

			reason := terminalInferenceFailureReason(t, events)
			if reason != tc.wantReason {
				t.Fatalf("terminal inference failure reason = %q, want %q", reason, tc.wantReason)
			}
			seenReasons[reason] = struct{}{}
		})
	}

	if len(seenReasons) != len(tests) {
		t.Fatalf("distinct failure reasons = %d, want %d publicly distinguishable classes", len(seenReasons), len(tests))
	}
}

func repeatedCodexThrottleCommandResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = platformprocess.CommandResult{
			ExitCode: 1,
			Stderr:   []byte(codexThrottleFailureStderr),
		}
	}
	return results
}

func terminalInferenceFailureReason(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.WorkFailureType {
	t.Helper()

	failure := terminalInferenceFailureObservation(t, events)
	if failure.FailureDetail == nil || failure.FailureDetail.Reason == "" {
		t.Fatalf("terminal inference failure detail = %#v, want public failure reason", failure.FailureDetail)
	}
	return failure.FailureDetail.Reason
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
