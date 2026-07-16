package providerparity

import (
	"context"
	"fmt"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// AssertCrossProviderParityForFixture runs the consolidated Batch 09 parity proofs
// for one catalog fixture: truthful fidelity, CLI/API transport parity, optional
// tool lifecycle checks, and primary-only vs response-stream finals.
func AssertCrossProviderParityForFixture(ctx context.Context, fixture Fixture) error {
	outcome, err := RunTransportParity(ctx, fixture)
	if err != nil {
		return fmt.Errorf("fixture %q transport parity: %w", fixture.ID, err)
	}
	if err := AssertTruthfulStreamingFidelity(fixture, outcome); err != nil {
		return fmt.Errorf("fixture %q fidelity: %w", fixture.ID, err)
	}
	if err := AssertCLIAPITransportParity(outcome); err != nil {
		return fmt.Errorf("fixture %q CLI/API transport parity: %w", fixture.ID, err)
	}
	if fixture.ToolLifecycle {
		if err := AssertObservableToolLifecycle(outcome.Events); err != nil {
			return fmt.Errorf("fixture %q tool lifecycle: %w", fixture.ID, err)
		}
	}
	if outcome.Terminal.Response.Content != fixture.WantContent {
		return fmt.Errorf(
			"fixture %q terminal content = %q, want %q",
			fixture.ID,
			outcome.Terminal.Response.Content,
			fixture.WantContent,
		)
	}
	apiInvocation := apisurface.InvocationResponseFromResult(outcome.InvocationResult)
	if apiInvocation.Status == "" {
		return fmt.Errorf("fixture %q API invocation projection missing status", fixture.ID)
	}
	if err := AssertPrimaryStreamModeParity(outcome); err != nil {
		return fmt.Errorf("fixture %q mode parity: %w", fixture.ID, err)
	}
	modeOutcome, err := RunModeParity(outcome)
	if err != nil {
		return fmt.Errorf("fixture %q mode projection: %w", fixture.ID, err)
	}
	if modeOutcome.PrimaryOnlyInvocation.Status == "" {
		return fmt.Errorf("fixture %q primary-only invocation missing status", fixture.ID)
	}
	if modeOutcome.StreamInvocation.Status == "" {
		return fmt.Errorf("fixture %q response-stream invocation missing status", fixture.ID)
	}
	return nil
}

// AssertCrossProviderParityCatalog runs AssertCrossProviderParityForFixture for
// every canonical catalog fixture.
func AssertCrossProviderParityCatalog(ctx context.Context) error {
	for _, fixture := range Catalog() {
		if err := AssertCrossProviderParityForFixture(ctx, fixture); err != nil {
			return err
		}
	}
	return nil
}
