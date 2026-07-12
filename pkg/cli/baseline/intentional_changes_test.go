package baseline_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/baseline"
)

func TestIntentionalChangesLedger_MatchesProductionBaselines(t *testing.T) {
	root := baseline.ProductionRootCommand()
	runCmd, err := baseline.ProductionRunCommand(root)
	if err != nil {
		t.Fatalf("resolve run command: %v", err)
	}

	if err := baseline.ValidateIntentionalChangesLedger(root, runCmd); err != nil {
		t.Fatalf("intentional changes ledger drift detected; update testdata/intentional_changes.json when intentional\n%v", err)
	}
}

func TestIntentionalChangesLedger_IsDistinctFromExecutableSnapshots(t *testing.T) {
	ledger, err := baseline.LoadIntentionalChangesLedger()
	if err != nil {
		t.Fatalf("load intentional changes ledger: %v", err)
	}

	if len(ledger.PlannedRemovals) == 0 {
		t.Fatal("expected planned removals in intentional changes ledger")
	}
	if len(ledger.PlannedMoves) == 0 {
		t.Fatal("expected planned moves in intentional changes ledger")
	}
}
