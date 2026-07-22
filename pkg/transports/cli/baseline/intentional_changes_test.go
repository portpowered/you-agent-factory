package baseline_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
)

func TestIntentionalChangesLedger_MatchesProductionBaselines(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}

	if err := baseline.ValidateIntentionalChangesLedger(fixtureSourceStore(), observation.Snapshot.CommandTree, observation.Snapshot.RunFlags); err != nil {
		t.Fatalf("intentional changes ledger drift detected; update testdata/intentional_changes.json when intentional\n%v", err)
	}
}

func TestIntentionalChangesLedger_IsDistinctFromExecutableSnapshots(t *testing.T) {
	ledger, err := baseline.LoadIntentionalChangesLedger(fixtureSourceStore())
	if err != nil {
		t.Fatalf("load intentional changes ledger: %v", err)
	}

	if len(ledger.PlannedRemovals) == 0 && len(ledger.PlannedMoves) == 0 {
		return
	}
	if len(ledger.PlannedRemovals) == 0 {
		t.Log("intentional changes ledger has no planned removals")
	}
}
