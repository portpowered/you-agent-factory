package baseline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
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
func LoadIntentionalChangesLedger(store generatedartifacts.SourceStore) (IntentionalChangesLedger, error) {
	raw, err := ReadFixtureText(store, intentionalChangesLedgerFixture)
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
func ValidateIntentionalChangesLedger(store generatedartifacts.SourceStore, commandTree, runFlags string) error {
	ledger, err := LoadIntentionalChangesLedger(store)
	if err != nil {
		return err
	}

	return validateLedgerAgainstBaselines(
		ledger,
		commandTree,
		runFlags,
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
