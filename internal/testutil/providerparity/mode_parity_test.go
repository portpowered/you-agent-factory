package providerparity_test

import (
	"context"
	"testing"

	parityfixtures "github.com/portpowered/infinite-you/internal/testutil/providerparity"
)

func TestModeParity_FullStreamPrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixtureFullStreamClaude)
}

func TestModeParity_PartialStreamPrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixturePartialStreamCodex)
}

func TestModeParity_SnapshotOnlyPrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixtureSnapshotOnlyOpenCode)
}

func TestModeParity_FinalOnlyOpenCodePrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixtureFinalOnlyOpenCode)
}

func TestModeParity_AgyFinalOnlyPrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixtureAgyFinalOnly)
}

func TestModeParity_ToolLifecyclePrimaryAndStreamAgree(t *testing.T) {
	t.Parallel()
	assertModeParityForFixture(t, parityfixtures.FixtureToolLifecycleClaude)
}

func assertModeParityForFixture(t *testing.T, fixtureID string) {
	t.Helper()

	fixture, ok := parityfixtures.FixtureByID(fixtureID)
	if !ok {
		t.Fatalf("unknown fixture %q", fixtureID)
	}
	outcome, err := parityfixtures.RunTransportParity(context.Background(), fixture)
	if err != nil {
		t.Fatalf("RunTransportParity(%q) error = %v", fixtureID, err)
	}
	if err := parityfixtures.AssertPrimaryStreamModeParity(outcome); err != nil {
		t.Fatalf("AssertPrimaryStreamModeParity(%q) error = %v", fixtureID, err)
	}
	modeOutcome, err := parityfixtures.RunModeParity(outcome)
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
