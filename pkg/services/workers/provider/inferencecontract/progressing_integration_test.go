package inferencecontract_test

import (
	"context"
	"testing"

	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestProgressingExternalIntegrationPublishesOrderedProgressAndOneTerminal(t *testing.T) {
	t.Parallel()

	integration := contract.ProgressingExternalIntegration("customer.provider", "structured progress COMPLETE")
	destination := &orderedWriter{}
	request := contract.NewInvocationRequest(contract.InvocationInput{
		InvocationID: "invocation-1",
		Model:        "fixture",
		UserMessage:  "prove progress",
		Required: contract.NewCapabilitySet(
			contract.CapabilityPromptSubmission,
		),
	})

	if err := contract.ExecuteInvocation(context.Background(), integration, request, destination); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}

	stats := integration.Stats()
	if stats.InvokeCalls != 1 || stats.ProgressWrites != 3 || stats.TerminalCloses != 1 {
		t.Fatalf("stats = %#v, want one invoke, three progress writes, one terminal", stats)
	}
	if stats.DiscoverCalls != 0 || stats.CapabilityCalls != 0 {
		t.Fatalf("Discover/Capabilities calls = %#v, want zero on ExecuteInvocation leaf path", stats)
	}
	wantOrder := []string{"RUN:STARTED", "MESSAGE:COMPLETED", "RUN:COMPLETED", "CLOSE"}
	if len(destination.order) != len(wantOrder) {
		t.Fatalf("destination order = %#v, want %#v", destination.order, wantOrder)
	}
	for index, want := range wantOrder {
		if destination.order[index] != want {
			t.Fatalf("destination order[%d] = %q, want %q", index, destination.order[index], want)
		}
	}
	if destination.closes != 1 || destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("completion = %#v closes=%d, want one successful terminal", destination.completion, destination.closes)
	}
	if destination.completion.Response().Content() != "structured progress COMPLETE" {
		t.Fatalf(
			"terminal content = %q, want structured progress COMPLETE",
			destination.completion.Response().Content(),
		)
	}
}

func TestProgressingExternalIntegrationDiscoverAndCapabilitiesAreReady(t *testing.T) {
	t.Parallel()

	integration := contract.ProgressingExternalIntegration("customer.provider", "ready")
	if integration.Identity() != "customer.provider" {
		t.Fatalf("Identity() = %q, want customer.provider", integration.Identity())
	}
	if !integration.MaximumCapabilities().Has(contract.CapabilityPromptSubmission) {
		t.Fatal("MaximumCapabilities() missing prompt submission")
	}

	discovery, err := integration.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if discovery.Readiness() != contract.ReadinessReady {
		t.Fatalf("Discover readiness = %s, want ready", discovery.Readiness())
	}

	request := contract.NewInvocationRequest(contract.InvocationInput{
		InvocationID: "invocation-ready",
		Model:        "fixture",
		UserMessage:  "capabilities",
		Required: contract.NewCapabilitySet(
			contract.CapabilityPromptSubmission,
		),
	})
	capabilities, err := integration.Capabilities(context.Background(), request)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !capabilities.Has(contract.CapabilityPromptSubmission) {
		t.Fatal("Capabilities() missing prompt submission")
	}

	stats := integration.Stats()
	if stats.DiscoverCalls != 1 || stats.CapabilityCalls != 1 || stats.DiscoverBeforeInvoke != 1 ||
		stats.CapabilitiesBeforeInvoke != 1 {
		t.Fatalf("pre-invoke stats = %#v, want discover/capabilities counted before invoke", stats)
	}
}
