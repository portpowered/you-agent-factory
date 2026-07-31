package providerparity

import (
	"context"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"testing"
)

func TestTransportParity_AgyFinalOnlyCLIAndAPIAgree(t *testing.T) {
	t.Parallel()
	assertTransportParityForFixture(t, FixtureAgyFinalOnly)
}

func assertTransportParityForFixture(t *testing.T, fixtureID string) {
	t.Helper()

	fixture, ok := FixtureByID(fixtureID)
	if !ok {
		t.Fatalf("unknown fixture %q", fixtureID)
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity(%q) error = %v", fixtureID, err)
	}
	if err := AssertTruthfulStreamingFidelity(fixture, outcome); err != nil {
		t.Fatalf("AssertTruthfulStreamingFidelity(%q) error = %v", fixtureID, err)
	}
	if err := AssertCLIAPITransportParity(outcome); err != nil {
		t.Fatalf("AssertCLIAPITransportParity(%q) error = %v", fixtureID, err)
	}
	if fixture.ToolLifecycle {
		if err := AssertObservableToolLifecycle(outcome.Events); err != nil {
			t.Fatalf("AssertObservableToolLifecycle(%q) error = %v", fixtureID, err)
		}
	}
	if outcome.Terminal.Response.Content != fixture.WantContent {
		t.Fatalf("terminal content = %q, want %q", outcome.Terminal.Response.Content, fixture.WantContent)
	}
	apiInvocation := apisurface.InvocationResponseFromResult(outcome.InvocationResult)
	if apiInvocation.Status == "" {
		t.Fatalf("API invocation projection missing status")
	}
}
