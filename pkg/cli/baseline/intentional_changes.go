package baseline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/spf13/cobra"
)

const intentionalChangesLedgerFixture = "testdata/intentional_changes.json"

// IntentionalChangesLedger records commands and flags deliberately planned for
// removal or relocation during upcoming CLI migrations.
type IntentionalChangesLedger struct {
	PlannedRemovals []PlannedRemoval `json:"planned_removals"`
	PlannedMoves    []PlannedMove    `json:"planned_moves"`
}

// PlannedRemoval names a production command or run flag slated for removal.
type PlannedRemoval struct {
	Surface     string `json:"surface"`
	Name        string `json:"name,omitempty"`
	CommandPath string `json:"command_path,omitempty"`
	Rationale   string `json:"rationale"`
}

// PlannedMove names a production command slated for relocation.
type PlannedMove struct {
	Surface         string `json:"surface"`
	FromCommandPath string `json:"from_command_path"`
	ToCommandPath   string `json:"to_command_path"`
	Rationale       string `json:"rationale"`
}

// LoadIntentionalChangesLedger reads the committed intentional-change ledger.
func LoadIntentionalChangesLedger() (IntentionalChangesLedger, error) {
	raw, err := ReadFixtureText(intentionalChangesLedgerFixture)
	if err != nil {
		return IntentionalChangesLedger{}, err
	}

	var ledger IntentionalChangesLedger
	if err := json.Unmarshal([]byte(raw), &ledger); err != nil {
		return IntentionalChangesLedger{}, fmt.Errorf("decode intentional changes ledger: %w", err)
	}
	return ledger, nil
}

// ValidateIntentionalChangesLedger asserts each ledger entry still appears in
// today's production command-tree and/or run-flag baselines.
func ValidateIntentionalChangesLedger(root *cobra.Command, runCmd *cobra.Command) error {
	ledger, err := LoadIntentionalChangesLedger()
	if err != nil {
		return err
	}

	return validateLedgerAgainstBaselines(
		ledger,
		SerializeCommandTree(root),
		SerializeRunFlags(runCmd),
	)
}

func validateLedgerAgainstBaselines(ledger IntentionalChangesLedger, commandTree, runFlags string) error {
	for _, removal := range ledger.PlannedRemovals {
		switch removal.Surface {
		case "run_flag":
			if removal.Name == "" {
				return fmt.Errorf("planned removal run_flag is missing name")
			}
			if !runFlagPresent(runFlags, removal.Name) {
				return fmt.Errorf("planned removal run_flag %q is not present in the run-flag baseline", removal.Name)
			}
		case "command":
			if removal.CommandPath == "" {
				return fmt.Errorf("planned removal command is missing command_path")
			}
			if !commandPathPresent(commandTree, removal.CommandPath) {
				return fmt.Errorf("planned removal command %q is not present in the command-tree baseline", removal.CommandPath)
			}
		default:
			return fmt.Errorf("unsupported planned removal surface %q", removal.Surface)
		}
	}

	for _, move := range ledger.PlannedMoves {
		if move.Surface != "command" {
			return fmt.Errorf("unsupported planned move surface %q", move.Surface)
		}
		if move.FromCommandPath == "" {
			return fmt.Errorf("planned move command is missing from_command_path")
		}
		if !commandPathPresent(commandTree, move.FromCommandPath) {
			return fmt.Errorf("planned move source command %q is not present in the command-tree baseline", move.FromCommandPath)
		}
	}

	return nil
}

func commandPathPresent(commandTree, commandPath string) bool {
	prefix := commandPath + "\t"
	for _, line := range strings.Split(commandTree, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func runFlagPresent(runFlags, name string) bool {
	prefix := name + "\t"
	for _, line := range strings.Split(runFlags, "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// ProductionRootCommand returns the canonical production CLI root command.
func ProductionRootCommand() *cobra.Command {
	return cli.NewRootCommand()
}

// ProductionRunCommand resolves the production you run command from root.
func ProductionRunCommand(root *cobra.Command) (*cobra.Command, error) {
	runCmd, _, err := root.Find([]string{"run"})
	if err != nil {
		return nil, fmt.Errorf("find run command: %w", err)
	}
	return runCmd, nil
}
