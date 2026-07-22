// Package testkit provides a reusable black-box conformance suite for
// customer-implemented provider integrations.
package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationFactory constructs a fresh deterministic integration for an
// opaque identity. Conformance scenarios never branch on the identity value.
type IntegrationFactory func(contract.Identity) contract.Integration

// Fixture supplies immutable provider-neutral requests and expected responses.
type Fixture struct {
	FinalOnlyRequest contract.InvocationRequest
	StreamingRequest contract.InvocationRequest
	ToolRequest      contract.InvocationRequest
	ExpectedResponse contract.Response
}

// Suite supplies the success-mode integrations exercised by Run.
type Suite struct {
	Identities []contract.Identity
	FinalOnly  IntegrationFactory
	Streaming  IntegrationFactory
	Tool       IntegrationFactory
	Fixture    Fixture
}

// Run verifies discovery, negotiation, final-only completion, streamed message
// progress, correlated tool progress, result agreement, and exactly-once close.
func Run(t *testing.T, suite Suite) {
	t.Helper()
	requireSuite(t, suite)
	for _, identity := range suite.Identities {
		identity := identity
		t.Run(string(identity), func(t *testing.T) {
			runScenario(t, identity, scenario{
				name: "final-only", factory: suite.FinalOnly,
				request: suite.Fixture.FinalOnlyRequest, expected: suite.Fixture.ExpectedResponse,
				assertMaximum: assertFinalOnlyMaximum, assertEvents: assertFinalOnlyEvents,
			})
			runScenario(t, identity, scenario{
				name: "streaming", factory: suite.Streaming,
				request: suite.Fixture.StreamingRequest, expected: suite.Fixture.ExpectedResponse,
				assertMaximum: assertStreamingMaximum, assertEvents: assertStreamingEvents,
			})
			runScenario(t, identity, scenario{
				name: "tool-lifecycle", factory: suite.Tool,
				request: suite.Fixture.ToolRequest, expected: suite.Fixture.ExpectedResponse,
				assertMaximum: assertToolMaximum, assertEvents: assertToolEvents,
			})
		})
	}
}

type scenario struct {
	name          string
	factory       IntegrationFactory
	request       contract.InvocationRequest
	expected      contract.Response
	assertMaximum func(*testing.T, contract.CapabilitySet)
	assertEvents  func(*testing.T, []contract.EventDraft)
}

func runScenario(t *testing.T, identity contract.Identity, test scenario) {
	t.Helper()
	t.Run(test.name, func(t *testing.T) {
		integration := test.factory(identity)
		if integration == nil {
			t.Fatal("integration factory returned nil")
		}
		maximum := assertIntegrationContract(t, integration, identity, test.request)
		test.assertMaximum(t, maximum)
		destination := &recordingWriter{}
		if err := contract.ExecuteInvocation(context.Background(), integration, test.request, destination); err != nil {
			t.Fatalf("ExecuteInvocation() error = %v", err)
		}
		test.assertEvents(t, destination.events)
		assertCompletion(t, destination, test.expected)
	})
}

func assertIntegrationContract(t *testing.T, integration contract.Integration, identity contract.Identity, request contract.InvocationRequest) contract.CapabilitySet {
	t.Helper()
	if integration.Identity() != identity {
		t.Fatalf("Identity() = %q, want %q", integration.Identity(), identity)
	}
	if err := contract.ValidateIdentity(integration.Identity()); err != nil {
		t.Fatalf("Identity() is invalid: %v", err)
	}
	maximum := integration.MaximumCapabilities()
	if err := contract.ValidateMaximumCapabilities(maximum); err != nil {
		t.Fatalf("MaximumCapabilities() is invalid: %v", err)
	}
	discovery, err := integration.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if err := contract.ValidateDiscovery(discovery); err != nil {
		t.Fatalf("Discover() returned invalid readiness: %v", err)
	}
	if discovery.Readiness() != contract.ReadinessReady {
		t.Fatalf("Discover() readiness = %q, want %q", discovery.Readiness(), contract.ReadinessReady)
	}
	negotiated, err := integration.Capabilities(context.Background(), request)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if err := contract.ValidateNegotiatedCapabilities(maximum, negotiated); err != nil {
		t.Fatalf("Capabilities() returned invalid negotiation: %v", err)
	}
	for _, required := range request.RequiredCapabilities().Values() {
		if !negotiated.Has(required) {
			t.Fatalf("Capabilities() omitted required capability %q", required)
		}
	}
	return maximum
}

type recordingWriter struct {
	events     []contract.EventDraft
	completion *contract.Completion
	closes     int
	order      []string
}

func (w *recordingWriter) WriteEvent(_ context.Context, event contract.EventDraft) error {
	w.events = append(w.events, event)
	draft := event.Draft()
	w.order = append(w.order, string(draft.Kind)+":"+string(draft.Phase))
	return nil
}

func (w *recordingWriter) Close(_ context.Context, completion contract.Completion) error {
	w.closes++
	w.completion = &completion
	w.order = append(w.order, "CLOSE")
	return nil
}

func assertFinalOnlyEvents(t *testing.T, events []contract.EventDraft) {
	t.Helper()
	if len(events) != 0 {
		t.Fatalf("final-only integration emitted streaming events: %#v", drafts(events))
	}
}

func assertFinalOnlyMaximum(t *testing.T, maximum contract.CapabilitySet) {
	t.Helper()
	if maximum.Has(contract.CapabilityNativeStreaming) || maximum.Has(contract.CapabilityMessageDeltas) {
		t.Fatalf("final-only maximum capabilities = %#v, must not advertise streaming or deltas", maximum.Values())
	}
}

func assertStreamingMaximum(t *testing.T, maximum contract.CapabilitySet) {
	t.Helper()
	assertCapabilities(t, maximum, contract.CapabilityNativeStreaming, contract.CapabilityMessageDeltas, contract.CapabilityMessageSnapshots)
}

func assertToolMaximum(t *testing.T, maximum contract.CapabilitySet) {
	t.Helper()
	assertCapabilities(t, maximum, contract.CapabilityToolLifecycle, contract.CapabilityToolOutputDeltas)
}

func assertCapabilities(t *testing.T, capabilities contract.CapabilitySet, required ...contract.Capability) {
	t.Helper()
	for _, capability := range required {
		if !capabilities.Has(capability) {
			t.Fatalf("maximum capabilities = %#v, want %q", capabilities.Values(), capability)
		}
	}
}

func assertStreamingEvents(t *testing.T, events []contract.EventDraft) {
	t.Helper()
	want := []eventPhase{
		{workers.KindRun, workers.PhaseStarted},
		{workers.KindMessage, workers.PhaseStarted},
		{workers.KindMessage, workers.PhaseDelta},
		{workers.KindMessage, workers.PhaseCompleted},
		{workers.KindRun, workers.PhaseCompleted},
	}
	assertEventPhases(t, events, want)
}

func assertToolEvents(t *testing.T, events []contract.EventDraft) {
	t.Helper()
	want := []eventPhase{
		{workers.KindRun, workers.PhaseStarted},
		{workers.KindTool, workers.PhaseStarted},
		{workers.KindTool, workers.PhaseDelta},
		{workers.KindTool, workers.PhaseCompleted},
		{workers.KindMessage, workers.PhaseCompleted},
		{workers.KindRun, workers.PhaseCompleted},
	}
	assertEventPhases(t, events, want)
	assertStableToolCorrelation(t, events[1:4])
}

type eventPhase struct {
	kind  workers.Kind
	phase workers.Phase
}

func assertEventPhases(t *testing.T, events []contract.EventDraft, want []eventPhase) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("events = %#v, want %d ordered events", drafts(events), len(want))
	}
	for index, event := range events {
		draft := event.Draft()
		if draft.Kind != want[index].kind || draft.Phase != want[index].phase {
			t.Fatalf("event[%d] = %s/%s, want %s/%s", index, draft.Kind, draft.Phase, want[index].kind, want[index].phase)
		}
	}
}

func assertStableToolCorrelation(t *testing.T, events []contract.EventDraft) {
	t.Helper()
	var toolCallID, toolName string
	for index, event := range events {
		draft := event.Draft()
		if draft.Phase == workers.PhaseDelta {
			var payload workers.ToolDeltaPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatalf("tool event[%d] payload: %v", index, err)
			}
			if payload.ToolCallID != toolCallID || payload.OutputDelta == "" {
				t.Fatalf("tool delta correlation = %#v, want toolCallID %q and bounded output", payload, toolCallID)
			}
			continue
		}
		var payload workers.ToolPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			t.Fatalf("tool event[%d] payload: %v", index, err)
		}
		if index == 0 {
			toolCallID, toolName = payload.ToolCallID, payload.ToolName
		}
		if payload.ToolCallID != toolCallID || payload.ToolName != toolName {
			t.Fatalf("tool event[%d] correlation = %q/%q, want %q/%q", index, payload.ToolCallID, payload.ToolName, toolCallID, toolName)
		}
	}
}

func assertCompletion(t *testing.T, destination *recordingWriter, expected contract.Response) {
	t.Helper()
	if destination.closes != 1 || destination.completion == nil {
		t.Fatalf("destination closes = %d, completion = %#v; want exactly one close", destination.closes, destination.completion)
	}
	if destination.order[len(destination.order)-1] != "CLOSE" {
		t.Fatalf("destination order = %#v, want completion after terminal events", destination.order)
	}
	response := destination.completion.Response()
	if response == nil || destination.completion.Failure() != nil {
		t.Fatalf("completion = %#v, want one successful response", destination.completion)
	}
	if response.Content() != expected.Content() || !maps.Equal(response.Metadata(), expected.Metadata()) ||
		!sameProviderSession(response.ProviderSession(), expected.ProviderSession()) {
		t.Fatalf("response = %#v/%#v/%#v, want %q/%#v/%#v", response.Content(), response.Metadata(), response.ProviderSession(), expected.Content(), expected.Metadata(), expected.ProviderSession())
	}
	assertDetachedResponse(t, *response)
}

func assertDetachedResponse(t *testing.T, response contract.Response) {
	t.Helper()
	metadata := response.Metadata()
	metadata["testkit-mutation"] = "must-not-stick"
	if _, exists := response.Metadata()["testkit-mutation"]; exists {
		t.Fatal("response metadata aliases caller-owned mutation")
	}
	session := response.ProviderSession()
	if session == nil {
		return
	}
	sessionMetadata := session.Metadata()
	sessionMetadata["testkit-mutation"] = "must-not-stick"
	if _, exists := response.ProviderSession().Metadata()["testkit-mutation"]; exists {
		t.Fatal("Provider Session metadata aliases caller-owned mutation")
	}
}

func sameProviderSession(got, want *contract.ProviderSession) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return got.Provider() == want.Provider() && got.Kind() == want.Kind() && got.ID() == want.ID() && maps.Equal(got.Metadata(), want.Metadata())
}

func requireSuite(t *testing.T, suite Suite) {
	t.Helper()
	if err := validateSuite(suite); err != nil {
		t.Fatalf("invalid provider conformance suite: %v", err)
	}
}

func validateSuite(suite Suite) error {
	if suite.FinalOnly == nil || suite.Streaming == nil || suite.Tool == nil {
		return fmt.Errorf("final-only, streaming, and tool integration factories are required")
	}
	if len(suite.Identities) < 2 {
		return fmt.Errorf("at least two opaque provider identities are required")
	}
	seen := make(map[contract.Identity]struct{}, len(suite.Identities))
	for _, identity := range suite.Identities {
		if err := contract.ValidateIdentity(identity); err != nil {
			return err
		}
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("provider identity %q is duplicated", identity)
		}
		seen[identity] = struct{}{}
	}
	requests := []contract.InvocationRequest{suite.Fixture.FinalOnlyRequest, suite.Fixture.StreamingRequest, suite.Fixture.ToolRequest}
	for index, request := range requests {
		if request.InvocationID() == "" || request.Model() == "" || request.UserMessage() == "" {
			return fmt.Errorf("request[%d] requires invocation ID, model, and user message", index)
		}
	}
	if suite.Fixture.ExpectedResponse.Content() == "" {
		return fmt.Errorf("expected response content is required")
	}
	if !slices.Contains(suite.Fixture.StreamingRequest.RequiredCapabilities().Values(), contract.CapabilityNativeStreaming) ||
		!slices.Contains(suite.Fixture.ToolRequest.RequiredCapabilities().Values(), contract.CapabilityToolLifecycle) {
		return fmt.Errorf("streaming and tool requests require their corresponding capabilities")
	}
	return nil
}

func drafts(events []contract.EventDraft) []workers.Draft {
	result := make([]workers.Draft, 0, len(events))
	for _, event := range events {
		result = append(result, event.Draft())
	}
	return result
}
