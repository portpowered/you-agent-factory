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

func TestIntentionalChangesLedger_RecordsCLIShapeCutover(t *testing.T) {
	ledger, err := baseline.LoadIntentionalChangesLedger(fixtureSourceStore())
	if err != nil {
		t.Fatalf("load intentional changes ledger: %v", err)
	}

	if len(ledger.PlannedRemovals) != 0 || len(ledger.PlannedMoves) != 0 {
		t.Fatalf("planned changes = removals %#v, moves %#v; want reconciled empty plan", ledger.PlannedRemovals, ledger.PlannedMoves)
	}
	if len(ledger.CompletedRemovals) != 1 {
		t.Fatalf("completed removals = %#v, want one Session dispatch removal", ledger.CompletedRemovals)
	}
	removal := ledger.CompletedRemovals[0]
	if removal.Surface != "command" || removal.CommandPath != "you session dispatches" || removal.Rationale == "" {
		t.Fatalf("completed removal = %#v, want the retired Session dispatch command", removal)
	}
	if len(ledger.CompletedMoves) != 2 {
		t.Fatalf("completed moves = %#v, want two command moves", ledger.CompletedMoves)
	}
	wantMoves := map[string]string{
		"you factory query":  "you factory show",
		"you work visualize": "you work render",
	}
	for _, move := range ledger.CompletedMoves {
		if move.Surface != "command" || move.Rationale == "" {
			t.Fatalf("completed move = %#v, want a documented command move", move)
		}
		if want, ok := wantMoves[move.FromCommandPath]; !ok || move.ToCommandPath != want {
			t.Fatalf("unexpected completed move = %#v, want %v", move, wantMoves)
		} else {
			delete(wantMoves, move.FromCommandPath)
		}
	}
	if len(wantMoves) != 0 {
		t.Fatalf("completed moves missing = %#v", wantMoves)
	}
}
