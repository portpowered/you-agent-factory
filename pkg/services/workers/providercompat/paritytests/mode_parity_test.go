package providerparity

import (
	"context"
	"testing"
)

func TestModeParity_AgyFinalOnlyPrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, FixtureAgyFinalOnly)
}

func assertModeParityForFixture(t *testing.T, fixtureID string) {
	t.Helper()

	fixture, ok := FixtureByID(fixtureID)
	if !ok {
		t.Fatalf("unknown fixture %q", fixtureID)
	}
	outcome, err := RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity(%q) error = %v", fixtureID, err)
	}
	if err := AssertPrimaryStreamModeParity(outcome); err != nil {
		t.Fatalf("AssertPrimaryStreamModeParity(%q) error = %v", fixtureID, err)
	}
	modeOutcome, err := RunModeParity(outcome)
	if err != nil {
		t.Fatalf("RunModeParity(%q) error = %v", fixtureID, err)
	}
	if modeOutcome.PrimaryOnlyInvocation.Status == "" {
		t.Fatalf("primary-only invocation missing status for %q", fixtureID)
	}
	if modeOutcome.StreamInvocation.Status == "" {
		t.Fatalf("response-stream invocation missing status for %q", fixtureID)
	}
}
