package clicontract

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/spf13/cobra"
)

func TestDeliberateStructuralViolationFixturesReportStableIdentity(t *testing.T) {
	tests := []struct {
		name        string
		fixture     func(*testing.T) Input
		wantFinding Finding
		wantError   string
	}{
		{
			name:        "uncontracted production command",
			fixture:     uncontractedCommandFixture,
			wantFinding: newFinding(KindUncontractedCommand, "you.experimental", "you experimental", "", "public production command is absent from canonical and approved compatibility contracts"),
			wantError:   `uncontracted-command: stable ID "you.experimental" path "you experimental": public production command is absent from canonical and approved compatibility contracts`,
		},
		{
			name:        "stale generated metadata",
			fixture:     staleGeneratedMetadataFixture,
			wantFinding: newFinding(KindStaleMetadata, "you", "you", "documentation", "generated canonical metadata differs from the authored contract"),
			wantError:   `stale-generated-metadata: stable ID "you" path "you" field "documentation": generated canonical metadata differs from the authored contract`,
		},
		{
			name:        "missing handwritten handler",
			fixture:     missingHandlerFixture,
			wantFinding: newFinding(KindMissingHandler, "you.run", "you run", "handler", "runnable production command has no handwritten handler"),
			wantError:   `missing-handler: stable ID "you.run" path "you run" field "handler": runnable production command has no handwritten handler`,
		},
		{
			name:        "compatibility alias promoted as canonical",
			fixture:     aliasAsCanonicalFixture,
			wantFinding: newFinding(KindAliasAsCanonical, "you.workflow.preview", "you workflow preview", "classification", "compatibility command is present in a canonical generated family"),
			wantError:   `compatibility-alias-as-canonical: stable ID "you.workflow.preview" path "you workflow preview" field "classification": compatibility command is present in a canonical generated family`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := test.fixture(t)
			first := Validate(input)
			second := Validate(input)

			if !reflect.DeepEqual(first, second) {
				t.Fatalf("repeated findings differ:\nfirst:  %#v\nsecond: %#v", first, second)
			}
			if len(first) != 1 {
				t.Fatalf("Validate() findings = %d, want 1:\n%s", len(first), formatFindings(first))
			}
			if !reflect.DeepEqual(first[0], test.wantFinding) {
				t.Fatalf("Validate() finding = %#v, want %#v", first[0], test.wantFinding)
			}
			if got := first[0].Error(); got != test.wantError {
				t.Fatalf("Finding.Error() = %q, want %q", got, test.wantError)
			}
		})
	}
}

func uncontractedCommandFixture(t *testing.T) Input {
	t.Helper()
	input := productionInput(t)
	root := cli.NewRootCommand()
	root.AddCommand(&cobra.Command{
		Use:   "experimental",
		Short: "Synthetic uncontracted command",
		RunE: func(*cobra.Command, []string) error {
			return nil
		},
	})
	input.Production = walkFixtureTree(t, root)
	return input
}

func staleGeneratedMetadataFixture(t *testing.T) Input {
	t.Helper()
	input := productionInput(t)
	manifest := cloneManifest(input.GeneratedCanonical[0])
	command := manifest.Commands["you"]
	command.Documentation.Documentation.Title.CanonicalEnglish = "synthetic stale title"
	manifest.Commands[command.ID] = command
	input.GeneratedCanonical[0] = manifest
	return input
}

func missingHandlerFixture(t *testing.T) Input {
	t.Helper()
	input := productionInput(t)
	root := cli.NewRootCommand()
	command, remaining, err := root.Find([]string{"run"})
	if err != nil {
		t.Fatalf("find run command: %v", err)
	}
	if len(remaining) != 0 || command.CommandPath() != "you run" {
		t.Fatalf("find run command = %q remaining %v", command.CommandPath(), remaining)
	}
	command.Run = nil
	command.RunE = nil
	input.Production = walkFixtureTree(t, root)
	return input
}

func aliasAsCanonicalFixture(t *testing.T) Input {
	t.Helper()
	input := productionInput(t)
	manifest := cloneManifest(input.GeneratedCanonical[0])
	compatibility := input.Compatibility.Commands["you.workflow.preview"]
	manifest.Commands[compatibility.ID] = compatibility
	input.GeneratedCanonical[0] = manifest
	return input
}

func walkFixtureTree(t *testing.T, root *cobra.Command) commandidentity.Inventory {
	t.Helper()
	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("walk fixture tree: %v", err)
	}
	return inventory
}
