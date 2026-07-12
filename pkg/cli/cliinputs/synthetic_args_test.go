package cliinputs_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/spf13/cobra"
)

type syntheticArgumentCase struct {
	commandPath        string
	commandIDCandidate string
	idCandidate        string
	name               string
	position           int
	required           bool
	minCardinality     int
	maxCardinality     int
	variadic           bool
	enum               []string
	completionKind     string
	doubleDashHandling string
}

func syntheticArgumentCases() []syntheticArgumentCase {
	return []syntheticArgumentCase{
		{
			commandPath:        "synth required-pos",
			commandIDCandidate: "synth.required-pos",
			idCandidate:        "synth.required-pos.arg.0",
			name:               "topic",
			position:           0,
			required:           true,
			minCardinality:     1,
			maxCardinality:     1,
			variadic:           false,
			enum:               []string{},
			completionKind:     "none",
			doubleDashHandling: "terminates-flags",
		},
		{
			commandPath:        "synth optional-pos",
			commandIDCandidate: "synth.optional-pos",
			idCandidate:        "synth.optional-pos.arg.0",
			name:               "note",
			position:           0,
			required:           false,
			minCardinality:     0,
			maxCardinality:     1,
			variadic:           false,
			enum:               []string{},
			completionKind:     "none",
			doubleDashHandling: "terminates-flags",
		},
		{
			commandPath:        "synth variadic-pos",
			commandIDCandidate: "synth.variadic-pos",
			idCandidate:        "synth.variadic-pos.arg.0",
			name:               "seed",
			position:           0,
			required:           true,
			minCardinality:     1,
			maxCardinality:     -1,
			variadic:           true,
			enum:               []string{},
			completionKind:     "none",
			doubleDashHandling: "terminates-flags",
		},
		{
			commandPath:        "synth enum-pos",
			commandIDCandidate: "synth.enum-pos",
			idCandidate:        "synth.enum-pos.arg.0",
			name:               "topic",
			position:           0,
			required:           true,
			minCardinality:     1,
			maxCardinality:     1,
			variadic:           false,
			enum:               []string{"alpha", "beta"},
			completionKind:     "static",
			doubleDashHandling: "terminates-flags",
		},
	}
}

func TestWalk_SyntheticTreeRecordsPositionalArguments(t *testing.T) {
	root := newSyntheticArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	cases := syntheticArgumentCases()
	if len(inv.Arguments) != len(cases) {
		t.Fatalf("Arguments len = %d, want %d", len(inv.Arguments), len(cases))
	}

	byID := indexArgumentsByID(t, inv.Arguments)
	for _, tc := range cases {
		record, ok := byID[tc.idCandidate]
		if !ok {
			t.Fatalf("missing argument record %q", tc.idCandidate)
		}
		assertSyntheticArgumentRecord(t, tc, record)
	}
}

func TestWalk_SyntheticTreeDoesNotRecordFlagsAsArguments(t *testing.T) {
	root := newSyntheticArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for _, record := range inv.Arguments {
		if record.Name == "label" {
			t.Fatalf("flag name %q was recorded as an argument on %q", record.Name, record.CommandPath)
		}
	}
	if len(inv.Flags) != 0 {
		t.Fatalf("Flags len = %d, want 0 until flag extraction lands", len(inv.Flags))
	}
}

func TestWalk_SyntheticTreeDoesNotInventArguments(t *testing.T) {
	root := newSyntheticArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for _, record := range inv.Arguments {
		if record.CommandPath == "synth" || record.CommandPath == "synth with-flag" {
			t.Fatalf("invented argument %q for command %q with no positional inputs", record.IDCandidate, record.CommandPath)
		}
	}
}

func assertSyntheticArgumentRecord(t *testing.T, tc syntheticArgumentCase, record cliinputs.ArgumentRecord) {
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
	if record.Name != tc.name {
		t.Fatalf("%s name = %q, want %q", tc.idCandidate, record.Name, tc.name)
	}
	if record.Position != tc.position {
		t.Fatalf("%s position = %d, want %d", tc.idCandidate, record.Position, tc.position)
	}
	if record.Kind != "positional" {
		t.Fatalf("%s kind = %q, want positional", tc.idCandidate, record.Kind)
	}
	if record.ValueType != "string" {
		t.Fatalf("%s valueType = %q, want string", tc.idCandidate, record.ValueType)
	}
	if record.Required != tc.required {
		t.Fatalf("%s required = %t, want %t", tc.idCandidate, record.Required, tc.required)
	}
	if record.MinCardinality != tc.minCardinality {
		t.Fatalf("%s minCardinality = %d, want %d", tc.idCandidate, record.MinCardinality, tc.minCardinality)
	}
	if record.MaxCardinality != tc.maxCardinality {
		t.Fatalf("%s maxCardinality = %d, want %d", tc.idCandidate, record.MaxCardinality, tc.maxCardinality)
	}
	if record.Variadic != tc.variadic {
		t.Fatalf("%s variadic = %t, want %t", tc.idCandidate, record.Variadic, tc.variadic)
	}
	if !reflect.DeepEqual(record.Enum, tc.enum) {
		t.Fatalf("%s enum = %#v, want %#v", tc.idCandidate, record.Enum, tc.enum)
	}
	if record.CompletionKind != tc.completionKind {
		t.Fatalf("%s completionKind = %q, want %q", tc.idCandidate, record.CompletionKind, tc.completionKind)
	}
	if !reflect.DeepEqual(record.InputChannels, []string{"cli"}) {
		t.Fatalf("%s inputChannels = %#v, want [cli]", tc.idCandidate, record.InputChannels)
	}
	if record.DoubleDashHandling != tc.doubleDashHandling {
		t.Fatalf("%s doubleDashHandling = %q, want %q", tc.idCandidate, record.DoubleDashHandling, tc.doubleDashHandling)
	}
}

func newSyntheticArgumentTree() *cobra.Command {
	root := &cobra.Command{
		Use:   "synth",
		Short: "synthetic argument inventory root",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	requiredPos := &cobra.Command{
		Use:   "required-pos <topic>",
		Short: "requires one positional argument",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	optionalPos := &cobra.Command{
		Use:   "optional-pos [note]",
		Short: "accepts an optional positional argument",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	variadicPos := &cobra.Command{
		Use:   "variadic-pos <seed>...",
		Short: "accepts one or more positional arguments",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	enumPos := &cobra.Command{
		Use:       "enum-pos <topic>",
		Short:     "accepts one enumerated positional argument",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"alpha", "beta\tfirst choice"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	withFlag := &cobra.Command{
		Use:   "with-flag",
		Short: "command with a local flag but no positionals",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	withFlag.Flags().String("label", "", "local flag that must not appear in arguments[]")

	root.AddCommand(requiredPos, optionalPos, variadicPos, enumPos, withFlag)
	return root
}

func indexArgumentsByID(t *testing.T, arguments []cliinputs.ArgumentRecord) map[string]cliinputs.ArgumentRecord {
	t.Helper()

	index := make(map[string]cliinputs.ArgumentRecord, len(arguments))
	for _, record := range arguments {
		if _, exists := index[record.IDCandidate]; exists {
			t.Fatalf("duplicate argument idCandidate in inventory: %q", record.IDCandidate)
		}
		index[record.IDCandidate] = record
	}
	return index
}
