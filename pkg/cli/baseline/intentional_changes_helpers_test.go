package baseline

import (
	"strings"
	"testing"
)

func TestValidateLedgerAgainstBaselines(t *testing.T) {
	commandTree := "you factory create\tcreate <name>\tyou factory\nyou factory\tfactory\tyou\n"
	runFlags := "port\tdeprecated\nserver\tAPI base\n"

	t.Run("empty ledger", func(t *testing.T) {
		if err := validateLedgerAgainstBaselines(IntentionalChangesLedger{}, commandTree, runFlags); err != nil {
			t.Fatalf("expected empty ledger to validate: %v", err)
		}
	})

	t.Run("run flag present", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{
				Surface: "run_flag",
				Name:    "server",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err != nil {
			t.Fatalf("expected present run flag to validate: %v", err)
		}
	})

	t.Run("run flag missing name", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{Surface: "run_flag"}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), "missing name") {
			t.Fatalf("expected missing run_flag name error, got %v", err)
		}
	})

	t.Run("run flag absent from baseline", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{
				Surface: "run_flag",
				Name:    "workflow",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), `run_flag "workflow"`) {
			t.Fatalf("expected absent run_flag error, got %v", err)
		}
	})

	t.Run("command present", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{
				Surface:     "command",
				CommandPath: "you factory create",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err != nil {
			t.Fatalf("expected present command to validate: %v", err)
		}
	})

	t.Run("command missing path", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{Surface: "command"}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), "missing command_path") {
			t.Fatalf("expected missing command_path error, got %v", err)
		}
	})

	t.Run("command absent from baseline", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{
				Surface:     "command",
				CommandPath: "you factory save",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), `command "you factory save"`) {
			t.Fatalf("expected absent command error, got %v", err)
		}
	})

	t.Run("unsupported removal surface", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedRemovals: []PlannedRemoval{{Surface: "api_route"}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), `unsupported planned removal surface "api_route"`) {
			t.Fatalf("expected unsupported removal surface error, got %v", err)
		}
	})

	t.Run("planned move source present", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedMoves: []PlannedMove{{
				Surface:         "command",
				FromCommandPath: "you factory",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err != nil {
			t.Fatalf("expected present move source to validate: %v", err)
		}
	})

	t.Run("planned move missing from path", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedMoves: []PlannedMove{{Surface: "command"}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), "missing from_command_path") {
			t.Fatalf("expected missing from_command_path error, got %v", err)
		}
	})

	t.Run("planned move source absent", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedMoves: []PlannedMove{{
				Surface:         "command",
				FromCommandPath: "you factory save",
			}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), `source command "you factory save"`) {
			t.Fatalf("expected absent move source error, got %v", err)
		}
	})

	t.Run("unsupported move surface", func(t *testing.T) {
		ledger := IntentionalChangesLedger{
			PlannedMoves: []PlannedMove{{Surface: "run_flag"}},
		}
		if err := validateLedgerAgainstBaselines(ledger, commandTree, runFlags); err == nil || !strings.Contains(err.Error(), `unsupported planned move surface "run_flag"`) {
			t.Fatalf("expected unsupported move surface error, got %v", err)
		}
	})
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
