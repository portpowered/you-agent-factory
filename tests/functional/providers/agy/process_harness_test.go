package agy

import (
	"encoding/json"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
