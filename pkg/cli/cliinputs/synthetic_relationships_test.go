package cliinputs_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/spf13/cobra"
)

type syntheticRelationshipCase struct {
	commandPath        string
	commandIDCandidate string
	idCandidate        string
	kind               string
	participants       []string
}

func syntheticRelationshipCases() []syntheticRelationshipCase {
	return []syntheticRelationshipCase{
		{
			commandPath:        "synth mutex-cmd",
			commandIDCandidate: "synth.mutex-cmd",
			idCandidate:        "synth.mutex-cmd.rel.mutex.alpha-beta",
			kind:               "mutually-exclusive",
			participants:       []string{"alpha", "beta"},
		},
		{
			commandPath:        "synth required-together-cmd",
			commandIDCandidate: "synth.required-together-cmd",
			idCandidate:        "synth.required-together-cmd.rel.required-together.pass-user",
			kind:               "required-together",
			participants:       []string{"pass", "user"},
		},
		{
			commandPath:        "synth at-least-one-cmd",
			commandIDCandidate: "synth.at-least-one-cmd",
			idCandidate:        "synth.at-least-one-cmd.rel.at-least-one.json-yaml",
			kind:               "at-least-one",
			participants:       []string{"json", "yaml"},
		},
	}
}

func TestWalk_SyntheticTreeRecordsFlagRelationships(t *testing.T) {
	root := newSyntheticRelationshipTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	cases := syntheticRelationshipCases()
	if len(inv.Relationships) != len(cases) {
		t.Fatalf("Relationships len = %d, want %d", len(inv.Relationships), len(cases))
	}

	byID := indexRelationshipsByID(t, inv.Relationships)
	for _, tc := range cases {
		record, ok := byID[tc.idCandidate]
		if !ok {
			t.Fatalf("missing relationship record %q", tc.idCandidate)
		}
		assertSyntheticRelationshipRecord(t, tc, record)
	}
}

func TestWalk_SyntheticTreeDoesNotInventRelationships(t *testing.T) {
	root := newSyntheticRelationshipTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for _, record := range inv.Relationships {
		if record.CommandPath == "synth" || record.CommandPath == "synth plain-cmd" {
			t.Fatalf("invented relationship %q for command %q with no flag groups", record.IDCandidate, record.CommandPath)
		}
	}
}

func TestWalk_SyntheticTreeRelationshipParticipantsAreFlagNames(t *testing.T) {
	root := newSyntheticRelationshipTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	flagNamesByCommand := map[string]map[string]bool{}
	for _, flag := range inv.Flags {
		if flagNamesByCommand[flag.CommandPath] == nil {
			flagNamesByCommand[flag.CommandPath] = map[string]bool{}
		}
		flagNamesByCommand[flag.CommandPath][flag.Long] = true
	}

	for _, record := range inv.Relationships {
		names := flagNamesByCommand[record.CommandPath]
		for _, participant := range record.Participants {
			if !names[participant] {
				t.Fatalf("relationship %q participant %q is not an inventoried flag on %q",
					record.IDCandidate, participant, record.CommandPath)
			}
		}
	}
}

func assertSyntheticRelationshipRecord(t *testing.T, tc syntheticRelationshipCase, record cliinputs.RelationshipRecord) {
	t.Helper()

	if record.CommandPath != tc.commandPath {
		t.Fatalf("%s commandPath = %q, want %q", tc.idCandidate, record.CommandPath, tc.commandPath)
	}
	if record.CommandIDCandidate != tc.commandIDCandidate {
		t.Fatalf("%s commandIdCandidate = %q, want %q", tc.idCandidate, record.CommandIDCandidate, tc.commandIDCandidate)
	}
	if record.IDCandidate != tc.idCandidate {
		t.Fatalf("%s idCandidate = %q, want %q", tc.idCandidate, record.IDCandidate, tc.idCandidate)
	}
	if record.Kind != tc.kind {
		t.Fatalf("%s kind = %q, want %q", tc.idCandidate, record.Kind, tc.kind)
	}
	if !reflect.DeepEqual(record.Participants, tc.participants) {
		t.Fatalf("%s participants = %#v, want %#v", tc.idCandidate, record.Participants, tc.participants)
	}
}

func newSyntheticRelationshipTree() *cobra.Command {
	root := &cobra.Command{
		Use:   "synth",
		Short: "synthetic relationship inventory root",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	plain := &cobra.Command{
		Use:   "plain-cmd",
		Short: "command without flag groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	plain.Flags().String("only", "", "single local flag")

	mutexCmd := &cobra.Command{
		Use:   "mutex-cmd",
		Short: "command with mutually exclusive flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	mutexCmd.Flags().Bool("alpha", false, "first mutex flag")
	mutexCmd.Flags().Bool("beta", false, "second mutex flag")
	mutexCmd.MarkFlagsMutuallyExclusive("alpha", "beta")

	requiredTogetherCmd := &cobra.Command{
		Use:   "required-together-cmd",
		Short: "command with required-together flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	requiredTogetherCmd.Flags().String("user", "", "username")
	requiredTogetherCmd.Flags().String("pass", "", "password")
	requiredTogetherCmd.MarkFlagsRequiredTogether("user", "pass")

	atLeastOneCmd := &cobra.Command{
		Use:   "at-least-one-cmd",
		Short: "command with at-least-one flags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	atLeastOneCmd.Flags().Bool("json", false, "json output")
	atLeastOneCmd.Flags().Bool("yaml", false, "yaml output")
	atLeastOneCmd.MarkFlagsOneRequired("json", "yaml")

	root.AddCommand(plain, mutexCmd, requiredTogetherCmd, atLeastOneCmd)
	return root
}

func indexRelationshipsByID(t *testing.T, relationships []cliinputs.RelationshipRecord) map[string]cliinputs.RelationshipRecord {
	t.Helper()

	index := make(map[string]cliinputs.RelationshipRecord, len(relationships))
	for _, record := range relationships {
		if _, exists := index[record.IDCandidate]; exists {
			t.Fatalf("duplicate relationship idCandidate in inventory: %q", record.IDCandidate)
		}
		index[record.IDCandidate] = record
	}
	return index
}
