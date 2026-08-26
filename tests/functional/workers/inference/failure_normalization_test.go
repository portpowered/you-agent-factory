package inference_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	codexAuthFailureStderr     = `ERROR: unexpected status 401 Unauthorized {"type":"authentication_error","message":"invalid api key"}`
	codexThrottleFailureStderr = "ERROR: selected model is at capacity"
	codexTimeoutFailureStderr  = "request timed out after waiting for provider response"
)

const providerExitNormalizationSessionID = "provider-exit-normalization-session"

const (
	failureRedactionPromptNeedle     = "probe-prompt-redaction-9f3a7c"
	failureRedactionEnvKey           = "FACTORY_PROBE_SECRET"
	failureRedactionEnvNeedle        = "probe-env-redaction-8e2b1d"
	failureRedactionCredentialEnvKey = "OPENAI_API_KEY"
	failureRedactionCredentialNeedle = "sk-probe-credential-redaction-7d4c0b"
)

// TestProviderNonZeroExitMapsToPublicFailure proves a provider process that exits
// non-zero is normalized into a public failed Work outcome with matching Factory
// Event and Provider Session diagnostics, without hanging or reporting success.
func TestProviderNonZeroExitMapsToPublicFailure(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"cursor-test-model",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider non-zero exit"}`))

	const exitCode = 42
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: exitCode,
		Stdout: []byte(
			`{"type":"thread.started","thread_id":"` + providerExitNormalizationSessionID + `"}` + "\n",
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

// TestProviderMissingCompletionEvidenceMapsToPublicFailure proves an otherwise
// successful provider call cannot advance Work when it returns no authoritative
// completion evidence.
func TestProviderMissingCompletionEvidenceMapsToPublicFailure(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider missing completion evidence"}`))

	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{})
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 without completion evidence", got)
	}
	if provider.CallCount() != 1 {
		t.Fatalf("provider calls = %d, want 1 terminal missing-evidence attempt", provider.CallCount())
	}
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf("session progress categories = %+v, want one failed work item", session.Runtime.Progress.Categories)
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one terminal response", dispatches)
	}
	response := dispatches[0].Response
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want failed", response.Outcome)
	}
	if response.Error == nil || *response.Error != "provider completion evidence was missing" {
		t.Fatalf("dispatch error = %#v, want missing-completion diagnostic", response.Error)
	}
}

// TestProviderTaskCompletePartialOutputDoesNotAdvanceWork proves that a zero
// exit code, artifact signal, and task-complete lifecycle record do not stand
// in for an authoritative final provider response.
func TestProviderTaskCompletePartialOutputDoesNotAdvanceWork(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider contradictory completion"}`))

	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{
		Content: "partial output before provider completion",
		Diagnostics: &workerexecution.WorkDiagnostics{
			Command:  &workerexecution.CommandDiagnostic{ExitCode: 0},
			Metadata: map[string]string{"artifact_present": "true"},
			Provider: &workerexecution.ProviderDiagnostic{
				ResponseMetadata: map[string]string{
					workerexecution.ProviderResponseMetadataCompletionEvidence: "task_complete",
				},
			},
		},
	})
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 without authoritative final evidence", got)
	}
	if provider.CallCount() != 1 {
		t.Fatalf("provider calls = %d, want 1 terminal completion-validation attempt", provider.CallCount())
	}
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf("session progress categories = %+v, want one failed work item", session.Runtime.Progress.Categories)
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil {
		t.Fatalf("dispatch observations = %#v, want one terminal response", dispatches)
	}
	response := dispatches[0].Response
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want failed", response.Outcome)
	}
	if response.Error == nil || *response.Error != "provider completion evidence was contradictory" {
		t.Fatalf("dispatch error = %#v, want safe contradictory-completion diagnostic", response.Error)
	}
}

// TestProviderAuthRateLimitAndTimeoutRemainDistinct proves authentication,
// rate-limit, and timeout provider failures normalize to publicly distinct
// failure classes through the customer process boundary, including throttle
// exhaustion after default retry limits.
func TestProviderAuthRateLimitAndTimeoutRemainDistinct(t *testing.T) {
	t.Parallel()

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
			name:       "timeout failure",
			results:    nil,
			wantReason: factoryapi.WorkFailureTypeTimeout,
			wantCalls:  9,
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
			if tc.name == "timeout failure" {
				runner = testutil.NewProviderCommandRunner(platformprocess.CommandResult{
					ExitCode: 1,
					Stderr:   []byte(codexTimeoutFailureStderr),
				})
				runner.Queue(repeatedCodexTimeoutCommandResults(12)...)
			}
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

// TestProviderFailureRedactsPromptEnvironmentAndCredentials proves provider
// failure diagnostics on Work, Factory Events, and Provider Session surfaces
// omit prompt bodies, environment values, and credential material while still
// exposing a stable public failure signal.
func TestProviderFailureRedactsPromptEnvironmentAndCredentials(t *testing.T) {
	t.Setenv(failureRedactionCredentialEnvKey, failureRedactionCredentialNeedle)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  `+failureRedactionEnvKey+`: `+failureRedactionEnvNeedle+`
---
Test workstation with private prompt `+failureRedactionPromptNeedle+`.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider failure redaction"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(codexAuthFailureStderr),
	})
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one failed work item",
			session.Runtime.Progress.Categories,
		)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	request := runner.LastRequest()
	if !strings.Contains(string(request.Stdin), failureRedactionPromptNeedle) {
		t.Fatalf("provider stdin = %q, want private prompt needle %q", request.Stdin, failureRedactionPromptNeedle)
	}
	if !commandEnvContains(request.Env, failureRedactionEnvKey+"="+failureRedactionEnvNeedle) {
		t.Fatalf("provider command env missing workstation secret %q=%q", failureRedactionEnvKey, failureRedactionEnvNeedle)
	}
	if !commandEnvContains(request.Env, failureRedactionCredentialEnvKey+"="+failureRedactionCredentialNeedle) {
		t.Fatalf(
			"provider command env missing credential %q=%q",
			failureRedactionCredentialEnvKey,
			failureRedactionCredentialNeedle,
		)
	}

	reason := terminalInferenceFailureReason(t, events)
	if reason != factoryapi.WorkFailureTypeAuthFailure {
		t.Fatalf("terminal inference failure reason = %q, want %q", reason, factoryapi.WorkFailureTypeAuthFailure)
	}

	assertPublicProviderFailureSurfacesRedactSensitiveMaterial(
		t,
		session,
		listed,
		events,
		failureRedactionPromptNeedle,
		failureRedactionEnvNeedle,
		failureRedactionCredentialNeedle,
	)
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

func repeatedCodexTimeoutCommandResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = platformprocess.CommandResult{
			ExitCode: 1,
			Stderr:   []byte(codexTimeoutFailureStderr),
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
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse &&
			event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
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
		t.Fatalf("factory events missing terminal MODEL_RESPONSE failure")
	}
	return terminal
}

func commandEnvContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func assertPublicProviderFailureSurfacesRedactSensitiveMaterial(
	t *testing.T,
	session factoryapi.FactorySession,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	needles ...string,
) {
	t.Helper()

	failure := terminalInferenceFailureObservation(t, events)
	if failure.FailureDetail == nil || failure.FailureDetail.Message == "" {
		t.Fatalf("terminal inference failure detail = %#v, want stable public failure message", failure.FailureDetail)
	}

	failureEvents := make([]factoryapi.FactoryEvent, 0, len(events))
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse,
			factoryapi.FactoryEventTypeModelResponse:
			failureEvents = append(failureEvents, event)
		}
	}

	publicObservation, err := json.Marshal(struct {
		Session       factoryapi.FactorySession                `json:"session"`
		Work          factoryapi.ListWorkResponse              `json:"work"`
		FailureEvents []factoryapi.FactoryEvent                `json:"failureEvents"`
		Inference     factoryapi.InferenceResponseEventPayload `json:"inferenceFailure"`
	}{
		Session:       session,
		Work:          listed,
		FailureEvents: failureEvents,
		Inference:     failure,
	})
	if err != nil {
		t.Fatalf("marshal public provider failure surfaces: %v", err)
	}
	payload := string(publicObservation)
	for _, needle := range needles {
		if strings.Contains(payload, needle) {
			t.Fatalf("public provider failure surfaces leaked %q: %s", needle, payload)
		}
	}
	failureObservation, err := json.Marshal(struct {
		FailureEvents []factoryapi.FactoryEvent                `json:"failureEvents"`
		Inference     factoryapi.InferenceResponseEventPayload `json:"inferenceFailure"`
	}{
		FailureEvents: failureEvents,
		Inference:     failure,
	})
	if err != nil {
		t.Fatalf("marshal public provider failure events: %v", err)
	}
	if err := support.ValidateProviderSessionFixtureContent(
		"failure-normalization-redaction",
		"public-provider-failure-surfaces",
		failureObservation,
	); err != nil {
		t.Fatalf("public provider failure surfaces failed sanitization: %v", err)
	}
}
