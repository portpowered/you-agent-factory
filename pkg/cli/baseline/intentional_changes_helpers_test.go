package baseline

import (
	"strings"
	"testing"
)

func TestValidateLedgerAgainstBaselines(t *testing.T) {
	commandTree := "you factory create\tcreate <name>\tyou factory\nyou factory\tfactory\tyou\n"
	runFlags := "port\tdeprecated\nserver\tAPI base\n"

	cases := []struct {
		name    string
		ledger  IntentionalChangesLedger
		wantErr string
	}{
		{name: "empty ledger"},
		{
			name: "run flag present",
			ledger: IntentionalChangesLedger{
				PlannedRemovals: []PlannedRemoval{{Surface: "run_flag", Name: "server"}},
			},
		},
		{
			name:    "run flag missing name",
			ledger:  IntentionalChangesLedger{PlannedRemovals: []PlannedRemoval{{Surface: "run_flag"}}},
			wantErr: "missing name",
		},
		{
			name: "run flag absent from baseline",
			ledger: IntentionalChangesLedger{
				PlannedRemovals: []PlannedRemoval{{Surface: "run_flag", Name: "workflow"}},
			},
			wantErr: `run_flag "workflow"`,
		},
		{
			name: "command present",
			ledger: IntentionalChangesLedger{
				PlannedRemovals: []PlannedRemoval{{Surface: "command", CommandPath: "you factory create"}},
			},
		},
		{
			name:    "command missing path",
			ledger:  IntentionalChangesLedger{PlannedRemovals: []PlannedRemoval{{Surface: "command"}}},
			wantErr: "missing command_path",
		},
		{
			name: "command absent from baseline",
			ledger: IntentionalChangesLedger{
				PlannedRemovals: []PlannedRemoval{{Surface: "command", CommandPath: "you factory save"}},
			},
			wantErr: `command "you factory save"`,
		},
		{
			name:    "unsupported removal surface",
			ledger:  IntentionalChangesLedger{PlannedRemovals: []PlannedRemoval{{Surface: "api_route"}}},
			wantErr: `unsupported planned removal surface "api_route"`,
		},
		{
			name: "planned move source present",
			ledger: IntentionalChangesLedger{
				PlannedMoves: []PlannedMove{{Surface: "command", FromCommandPath: "you factory"}},
			},
		},
		{
			name:    "planned move missing from path",
			ledger:  IntentionalChangesLedger{PlannedMoves: []PlannedMove{{Surface: "command"}}},
			wantErr: "missing from_command_path",
		},
		{
			name: "planned move source absent",
			ledger: IntentionalChangesLedger{
				PlannedMoves: []PlannedMove{{Surface: "command", FromCommandPath: "you factory save"}},
			},
			wantErr: `source command "you factory save"`,
		},
		{
			name:    "unsupported move surface",
			ledger:  IntentionalChangesLedger{PlannedMoves: []PlannedMove{{Surface: "run_flag"}}},
			wantErr: `unsupported planned move surface "run_flag"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLedgerAgainstBaselines(tc.ledger, commandTree, runFlags)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestCommandPathPresent(t *testing.T) {
	tree := "you factory create\tcreate <name>\tyou factory\nyou factory\tfactory\tyou\n"

	if !commandPathPresent(tree, "you factory create") {
		t.Fatal("expected factory create path present")
	}
	if commandPathPresent(tree, "you factory save") {
		t.Fatal("expected removed path absent")
	}
}

func TestRunFlagPresent(t *testing.T) {
	flags := "port\tdeprecated\nserver\tAPI base\n"

	if !runFlagPresent(flags, "server") {
		t.Fatal("expected server flag present")
	}
	if runFlagPresent(flags, "nosuch") {
		t.Fatal("expected missing flag absent")
	}
}
