package climanifestparity_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/spf13/cobra"
)

func TestProductionRootFansInEachB12FamilyOnceWithHandlers(t *testing.T) {
	inventory, err := commandidentity.Walk(cli.NewRootCommand())
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	byID := make(map[string]commandidentity.CommandRecord, len(inventory.Commands))
	for _, record := range inventory.Commands {
		byID[record.IDCandidate] = record
	}

	expectedIDs := append([]string{}, climanifestgen.SessionFamilyCommandIDs...)
	expectedIDs = append(expectedIDs, climanifestgen.MCPFamilyCommandIDs...)
	expectedIDs = append(expectedIDs, climanifestgen.WorkflowCompatibilityFamilyCommandIDs...)
	expectedIDs = append(expectedIDs, climanifestgen.RunSubmitFamilyCommandIDs...)
	for _, commandID := range expectedIDs {
		record, ok := byID[commandID]
		if !ok {
			t.Fatalf("production root missing B12 command %s", commandID)
		}
		if record.Runnable && !record.HandlerPresent {
			t.Fatalf("production B12 command %s at %s is runnable without a handwritten handler", commandID, record.Path)
		}
	}
}

func TestB12LegacyAndGeneratedRollbackTreesConstructIndependently(t *testing.T) {
	sessionLegacy := cli.NewLegacySessionFamilyCommand(cli.RootCommandOptions{})
	sessionGenerated, err := cli.NewGeneratedSessionFamilyCommand(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewGeneratedSessionFamilyCommand() error = %v", err)
	}
	assertIndependentRoots(t, "session", sessionLegacy, sessionGenerated)

	workflowLegacy, workflowGenerated, err := cli.NewWorkflowMCPFamilyParityRoots()
	if err != nil {
		t.Fatalf("NewWorkflowMCPFamilyParityRoots() error = %v", err)
	}
	assertIndependentRoots(t, "workflow/MCP", workflowLegacy, workflowGenerated)

	runSubmitLegacy, runSubmitGenerated, err := cli.NewRunSubmitFamilyParityRoots(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	assertIndependentRoots(t, "run/submit", runSubmitLegacy, runSubmitGenerated)
}

func assertIndependentRoots(t *testing.T, family string, legacy, generated *cobra.Command) {
	t.Helper()
	if legacy == nil || generated == nil {
		t.Fatalf("%s parity roots must both be constructed", family)
	}
	if legacy == generated {
		t.Fatalf("%s parity roots must be independent command trees", family)
	}
	if legacy.CommandPath() != generated.CommandPath() {
		t.Fatalf("%s parity root paths = (%q, %q), want equal", family, legacy.CommandPath(), generated.CommandPath())
	}
}
