package agy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One reusable process executes invocation-owned sessions and an isolated home.
// Public primary output and the controlled command edge prove that overrides
// affect this invocation without becoming state on the injected collaborator.
func TestAgyRequestOverridesRemainScopedThroughPublicRun(t *testing.T) {
	// One-shot activation owns the process-wide ~default session. Run this
	// local phase before parallel explicit-session tests start their hosted
	// process, which also owns ~default for the lifetime of its listener.
	fixture := agySharedProcess(t)
	route := fixture.routes["one-shot-overrides"]
	scopes := make(map[string]bool)
	for _, test := range []struct {
		name    string
		flags   []string
		model   string
		failure string
	}{
		{name: "environment default", model: agyFunctionalModel},
		{name: "explicit model", flags: []string{"--model", "gemini-3.6-flash-low"}, model: "gemini-3.6-flash-low"},
		{name: "invalid effort", flags: []string{"--worker-reasoning-effort", "turbo"}, failure: `invalid --worker-reasoning-effort "turbo"`},
		{name: "default after override and rejection", model: agyFunctionalModel},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"you", "run", "--factory", route.workDir, "--quiet", "--no-record"}
			args = append(args, test.flags...)
			args = append(args, "prove scoped AGY overrides")
			inputs := support.FakeInputs(context.Background(), args)
			inputs.Input.Env = append(agySharedEnvironment(route.homeDir),
				"YOU_DEFAULT_WORKER_MODEL_PROVIDER=ANTIGRAVITY", "YOU_DEFAULT_WORKER_MODEL="+agyFunctionalModel)
			inputs.Input.WorkingDirectory = route.workDir
			before := route.callCount()
			err := fixture.process.Execute(inputs.Input)
			if test.failure != "" {
				if err == nil || !strings.Contains(err.Error(), test.failure) || route.callCount() != before {
					t.Fatalf("rejection = %v, calls = %d, want %s without dispatch", err, route.callCount()-before, test.failure)
				}
				return
			}
			assertAgyScopedOverride(t, inputs, err, route, before, scopes, test.model)
		})
	}
}

const agyFunctionalModel = agyGoldenModel

// TestAgyConductorSuccessThroughRootBuildProcess proves successful Agy
// print-mode execution through the customer process boundary and
// Providers-backed command adapter.
func TestAgyConductorSuccessThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, responseEvents, route, callStart := fixture.runDirect(t, "direct-conductor-success")

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("agy command runner calls = %d, want 1 through Providers path", got)
	}
	request := route.lastRequest()
	if !containsArgPair(request.Args, "--model", agyFunctionalModel) {
		t.Fatalf("argv = %#v, want --model %s", request.Args, agyFunctionalModel)
	}
	if !containsArg(request.Args, "-p") || !containsArgPair(request.Args, "--output-format", "stream-json") {
		t.Fatalf("argv = %#v, want shell-free -p print mode with stream-json output", request.Args)
	}
	assertAgyFinalOnlyCompletion(t, events, responseEvents, "agy functional answer COMPLETE")
}

// TestAgyNativeFailureThroughRootBuildProcessIsSafe proves native Agy failures
// remain safe and observable through the customer process boundary.
func TestAgyNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, _, route, callStart := fixture.runDirect(t, "direct-native-failure")
	const leaked = "/tmp/secret-key"

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after native failure", got)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("agy command runner calls = %d, want 1", got)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) || strings.Contains(payload, "secret-key") {
		t.Fatalf("Factory events leaked unsafe Agy failure detail: %s", payload)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyTimeoutFailureThroughRootBuildProcess proves timeout normalization
// through the customer process boundary without leaking partial output.
func TestAgyTimeoutFailureThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, _, route, callStart := fixture.runDirect(t, "direct-timeout")

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := route.callCount() - callStart; got < 1 {
		t.Fatalf("agy command runner calls = %d, want at least one retryable timeout attempt", got)
	}
	reason := terminalFailureReason(t, events)
	if reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want %q", reason, factoryapi.WorkFailureTypeTimeout)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyCommandCancellationThroughRootBuildProcessIsCanonical proves
// cancellation returns the canonical outcome through the Providers command
// adapter.
func TestAgyCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, _, route, callStart := fixture.runDirect(t, "direct-cancellation")

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("agy command runner calls = %d, want 1", got)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("Factory events missing canonical cancellation outcome: %s", payload)
	}
}

func assertAgyFinalOnlyCompletion(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	wantOutput string,
) {
	t.Helper()

	var completedMessages int
	for _, event := range responseEvents {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only Agy replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only Agy replay fabricated lifecycle: %#v", event)
		}
	}
	if completedMessages != 1 {
		t.Fatalf("completed message events = %d, want exactly one terminal result", completedMessages)
	}

	dispatchOutput := ""
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && *payload.Output != "" {
			dispatchOutput = *payload.Output
		}
	}
	if dispatchOutput != wantOutput {
		t.Fatalf("dispatch output = %q, want %q", dispatchOutput, wantOutput)
	}
}

func assertAgyProviderSession(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.InferenceOutcome,
	wantProvider string,
) {
	t.Helper()

	var found bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != wantOutcome {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil {
			t.Fatal("inference response missing provider session metadata")
		}
		if got := support.StringPointerValue(payload.ProviderSession.Provider); got != wantProvider {
			t.Fatalf("provider session provider = %q, want %q", got, wantProvider)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("missing inference response with outcome %q", wantOutcome)
	}
}

func terminalFailureReason(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.WorkFailureType {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeFailed && payload.FailureDetail != nil {
			return payload.FailureDetail.Reason
		}
	}
	t.Fatal("missing failed inference response with failure detail")
	return ""
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func assertAgyScopedOverride(t *testing.T, inputs *support.CapturedInputs, err error, route *agySharedCommandRoute, before int, scopes map[string]bool, model string) {
	t.Helper()
	if err != nil || strings.TrimSpace(inputs.Stdout()) != "override answer COMPLETE" || inputs.Stderr() != "" {
		t.Fatalf("invocation = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if route.callCount()-before != 1 {
		t.Fatalf("provider calls = %d, want 1", route.callCount()-before)
	}
	request := route.lastRequest()
	if request.WorkDir != route.workDir || request.ExecutionScopeID == "" || scopes[request.ExecutionScopeID] {
		t.Fatalf("provider workdir/scope = %q/%q, want this workspace and a fresh invocation scope", request.WorkDir, request.ExecutionScopeID)
	}
	scopes[request.ExecutionScopeID] = true
	if !containsArgPair(request.Args, "--model", model) ||
		!containsArgPair(request.Args, "--print-timeout", "2m") ||
		containsArg(request.Args, "--dangerously-skip-permissions") {
		t.Fatalf("provider argv = %#v, want model %s and authored timeout/permission policy", request.Args, model)
	}
}
