package cliinputs_test

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli"
	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/portpowered/infinite-you/pkg/cli/commandidentity"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

const cliCommandsBaselineFixture = "contracts/testdata/baseline/cli-commands.json"

func TestWalk_ProductionRootJoinsCommittedCommandIdentityBaseline(t *testing.T) {
	root := cli.NewRootCommand()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandsBaselineFixture)
	baselineData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	index, err := cliinputs.LoadCommandIdentityIndexFromBaseline(baselineData)
	if err != nil {
		t.Fatalf("LoadCommandIdentityIndexFromBaseline() error = %v", err)
	}
	if err := cliinputs.ValidateCommandJoins(inv, index); err != nil {
		t.Fatalf("ValidateCommandJoins(production inventory, committed baseline) error = %v", err)
	}
}

func TestWalk_ProductionRootJoinsLiveCommandIdentityWalk(t *testing.T) {
	root := cli.NewRootCommand()

	inputsInv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	identityInv, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("commandidentity.Walk(production root) error = %v", err)
	}

	index := cliinputs.NewCommandIdentityIndex(identityInv.Commands)
	if err := cliinputs.ValidateCommandJoins(inputsInv, index); err != nil {
		t.Fatalf("ValidateCommandJoins(production inventory, live identity walk) error = %v", err)
	}
}

func TestWalk_ProductionRootRepresentativeCommandsRetainInputs(t *testing.T) {
	root := cli.NewRootCommand()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	flagsByPath := indexFlagsByCommandPath(t, inv.Flags)
	argsByPath := indexArgumentsByCommandPath(t, inv.Arguments)
	relsByPath := indexRelationshipsByCommandPath(t, inv.Relationships)

	cases := []struct {
		path             string
		commandID        string
		minArguments     int
		minFlags         int
		minRelationships int
		wantFlagLongs    []string
	}{
		{
			path:         "you run",
			commandID:    "you.run",
			minFlags:     1,
			wantFlagLongs: []string{"dir", "named", "factory"},
		},
		{
			path:         "you submit",
			commandID:    "you.submit",
			minFlags:     1,
			wantFlagLongs: []string{"name", "work-type-name", "payload"},
		},
		{
			path:         "you submit batch",
			commandID:    "you.submit.batch",
			minArguments: 1,
			minFlags:     1,
			wantFlagLongs: []string{"file", "dry-run"},
		},
		{
			path:         "you session show",
			commandID:    "you.session.show",
			minArguments: 1,
			minFlags:     1,
		},
		{
			path:         "you session create",
			commandID:    "you.session.create",
			minFlags:     1,
			minRelationships: 1,
			wantFlagLongs: []string{"dir", "init-new-factory", "validate-only"},
		},
	}

	for _, tc := range cases {
		args := argsByPath[tc.path]
		if len(args) < tc.minArguments {
			t.Fatalf("%s argument count = %d, want at least %d", tc.path, len(args), tc.minArguments)
		}
		for _, record := range args {
			if record.CommandIDCandidate != tc.commandID {
				t.Fatalf("%s argument %q commandIdCandidate = %q, want %q", tc.path, record.IDCandidate, record.CommandIDCandidate, tc.commandID)
			}
		}

		flags := flagsByPath[tc.path]
		if len(flags) < tc.minFlags {
			t.Fatalf("%s flag count = %d, want at least %d", tc.path, len(flags), tc.minFlags)
		}
		for _, record := range flags {
			if record.CommandIDCandidate != tc.commandID {
				t.Fatalf("%s flag %q commandIdCandidate = %q, want %q", tc.path, record.IDCandidate, record.CommandIDCandidate, tc.commandID)
			}
		}
		for _, longName := range tc.wantFlagLongs {
			if !flagLongPresent(flags, longName) {
				t.Fatalf("%s missing expected flag %q", tc.path, longName)
			}
		}

		rels := relsByPath[tc.path]
		if len(rels) < tc.minRelationships {
			t.Fatalf("%s relationship count = %d, want at least %d", tc.path, len(rels), tc.minRelationships)
		}
		for _, record := range rels {
			if record.CommandIDCandidate != tc.commandID {
				t.Fatalf("%s relationship %q commandIdCandidate = %q, want %q", tc.path, record.IDCandidate, record.CommandIDCandidate, tc.commandID)
			}
		}
	}
}

func TestWalk_ProductionInventoryOnlyReferencesKnownCommandPaths(t *testing.T) {
	root := cli.NewRootCommand()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk(production root) error = %v", err)
	}

	identityInv, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("commandidentity.Walk(production root) error = %v", err)
	}

	index := cliinputs.NewCommandIdentityIndex(identityInv.Commands)
	if err := cliinputs.ValidateCommandJoins(inv, index); err != nil {
		t.Fatalf("ValidateCommandJoins() error = %v", err)
	}
}

func indexFlagsByCommandPath(t *testing.T, flags []cliinputs.FlagRecord) map[string][]cliinputs.FlagRecord {
	t.Helper()

	index := make(map[string][]cliinputs.FlagRecord)
	for _, record := range flags {
		index[record.CommandPath] = append(index[record.CommandPath], record)
	}
	return index
}

func indexArgumentsByCommandPath(t *testing.T, arguments []cliinputs.ArgumentRecord) map[string][]cliinputs.ArgumentRecord {
	t.Helper()

	index := make(map[string][]cliinputs.ArgumentRecord)
	for _, record := range arguments {
		index[record.CommandPath] = append(index[record.CommandPath], record)
	}
	return index
}

func indexRelationshipsByCommandPath(t *testing.T, relationships []cliinputs.RelationshipRecord) map[string][]cliinputs.RelationshipRecord {
	t.Helper()

	index := make(map[string][]cliinputs.RelationshipRecord)
	for _, record := range relationships {
		index[record.CommandPath] = append(index[record.CommandPath], record)
	}
	return index
}

func flagLongPresent(flags []cliinputs.FlagRecord, longName string) bool {
	for _, record := range flags {
		if record.Long == longName {
			return true
		}
	}
	return false
}
