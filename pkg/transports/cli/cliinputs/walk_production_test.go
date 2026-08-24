package cliinputs_test

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliinputs"
	"github.com/spf13/cobra"
)

const cliCommandsBaselineFixture = "contracts/testdata/baseline/cli-commands.json"
const cliCommandInputsBaselineFixture = "contracts/testdata/baseline/cli-command-inputs.json"

func TestWalk_ProductionRootJoinsCommittedCommandIdentityBaseline(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inv := observation.Snapshot.Inputs

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
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inputsInv := observation.Snapshot.Inputs
	identityInv := observation.Snapshot.Commands

	index := cliinputs.NewCommandIdentityIndex(identityInv.Commands)
	if err := cliinputs.ValidateCommandJoins(inputsInv, index); err != nil {
		t.Fatalf("ValidateCommandJoins(production inventory, live identity walk) error = %v", err)
	}
}

func TestWalk_ProductionRootRepresentativeCommandsRetainInputs(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inv := observation.Snapshot.Inputs

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
			path:          "you run",
			commandID:     "you.run",
			minFlags:      1,
			wantFlagLongs: []string{"dir", "named", "factory"},
		},
		{
			path:          "you submit",
			commandID:     "you.submit",
			minFlags:      1,
			wantFlagLongs: []string{"name", "work-type-name", "payload"},
		},
		{
			path:          "you submit batch",
			commandID:     "you.submit.batch",
			minArguments:  1,
			minFlags:      1,
			wantFlagLongs: []string{"file", "dry-run"},
		},
		{
			path:         "you session show",
			commandID:    "you.session.show",
			minArguments: 1,
			minFlags:     1,
		},
		{
			path:             "you session create",
			commandID:        "you.session.create",
			minFlags:         1,
			minRelationships: 1,
			wantFlagLongs:    []string{"dir", "init-new-factory", "validate-only"},
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
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inv := observation.Snapshot.Inputs
	identityInv := observation.Snapshot.Commands

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

func TestWalk_ProductionInventoryMatchesCommittedBaseline(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inventory := observation.Snapshot.Inputs

	got, err := cliinputs.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandInputsBaselineFixture)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}

	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"CLI command inputs baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		cliCommandInputsBaselineFixture,
		len(want),
		len(got),
	)
}

func TestWriteProductionInputsInventoryBaseline(t *testing.T) {
	if os.Getenv("UPDATE_CLI_BASELINES") != "1" {
		t.Skip("set UPDATE_CLI_BASELINES=1 to rewrite fixtures")
	}

	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production CLI: %v", err)
	}
	inventory := observation.Snapshot.Inputs
	got, err := cliinputs.MarshalInventory(inventory)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, cliCommandInputsBaselineFixture)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}

func TestWalk_ArgumentsSortedByCommandPathPositionName(t *testing.T) {
	root := newDeterminismArgumentTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for i := 1; i < len(inv.Arguments); i++ {
		prev := inv.Arguments[i-1]
		curr := inv.Arguments[i]
		if !argumentRecordsSorted(prev, curr) {
			t.Fatalf(
				"arguments not sorted at index %d: %#v should precede %#v",
				i,
				prev,
				curr,
			)
		}
	}
}

func TestWalk_FlagsSortedByCommandPathLong(t *testing.T) {
	root := newSyntheticFlagTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for i := 1; i < len(inv.Flags); i++ {
		prev := inv.Flags[i-1]
		curr := inv.Flags[i]
		if !flagRecordsSorted(prev, curr) {
			t.Fatalf(
				"flags not sorted at index %d: %#v should precede %#v",
				i,
				prev,
				curr,
			)
		}
	}
}

func TestWalk_RelationshipsSortedByCommandPathKindParticipants(t *testing.T) {
	root := newSyntheticRelationshipTree()

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for i := 1; i < len(inv.Relationships); i++ {
		prev := inv.Relationships[i-1]
		curr := inv.Relationships[i]
		if !relationshipRecordsSorted(prev, curr) {
			t.Fatalf(
				"relationships not sorted at index %d: %#v should precede %#v",
				i,
				prev,
				curr,
			)
		}
	}
}

func TestWalk_DoesNotMutateCommandTree(t *testing.T) {
	oldSorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	t.Cleanup(func() { cobra.EnableCommandSorting = oldSorting })

	root := newUnsortedChildOrderTree()
	beforeOrder := childRegistrationOrder(root)

	_, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	afterOrder := childRegistrationOrder(root)
	if !reflect.DeepEqual(beforeOrder, afterOrder) {
		t.Fatalf("walker mutated child registration order:\nbefore=%#v\nafter=%#v", beforeOrder, afterOrder)
	}
}

func TestWalk_ProducesIdenticalJSONOnRepeat(t *testing.T) {
	root := newDeterminismInventoryTree()

	first, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("first Walk() error = %v", err)
	}
	firstJSON, err := cliinputs.MarshalInventory(first)
	if err != nil {
		t.Fatalf("first MarshalInventory() error = %v", err)
	}

	second, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("second Walk() error = %v", err)
	}
	secondJSON, err := cliinputs.MarshalInventory(second)
	if err != nil {
		t.Fatalf("second MarshalInventory() error = %v", err)
	}

	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated walks produced different JSON:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
}

func argumentRecordsSorted(left, right cliinputs.ArgumentRecord) bool {
	if left.CommandPath != right.CommandPath {
		return left.CommandPath < right.CommandPath
	}
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.IDCandidate < right.IDCandidate
}

func flagRecordsSorted(left, right cliinputs.FlagRecord) bool {
	if left.CommandPath != right.CommandPath {
		return left.CommandPath < right.CommandPath
	}
	if left.Long != right.Long {
		return left.Long < right.Long
	}
	return left.IDCandidate < right.IDCandidate
}

func relationshipRecordsSorted(left, right cliinputs.RelationshipRecord) bool {
	if left.CommandPath != right.CommandPath {
		return left.CommandPath < right.CommandPath
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	leftParticipants := strings.Join(left.Participants, "\x00")
	rightParticipants := strings.Join(right.Participants, "\x00")
	if leftParticipants != rightParticipants {
		return leftParticipants < rightParticipants
	}
	return left.IDCandidate < right.IDCandidate
}

func newUnsortedChildOrderTree() *cobra.Command {
	root := &cobra.Command{Use: "order"}
	root.AddCommand(
		&cobra.Command{Use: "z"},
		&cobra.Command{Use: "a"},
		&cobra.Command{Use: "m"},
	)
	return root
}

func childRegistrationOrder(root *cobra.Command) map[string][]string {
	order := make(map[string][]string)
	captureChildRegistrationOrder(root, order)
	return order
}

func captureChildRegistrationOrder(cmd *cobra.Command, order map[string][]string) {
	children := cmd.Commands()
	names := make([]string, len(children))
	for i, child := range children {
		names[i] = child.Name()
	}
	order[cmd.CommandPath()] = names
	for _, child := range children {
		captureChildRegistrationOrder(child, order)
	}
}

func newDeterminismArgumentTree() *cobra.Command {
	root := &cobra.Command{Use: "det"}

	root.AddCommand(
		&cobra.Command{
			Use:  "zebra <topic>",
			Args: cobra.ExactArgs(1),
		},
		&cobra.Command{
			Use:  "alpha <topic>",
			Args: cobra.ExactArgs(1),
		},
	)
	return root
}

func newDeterminismInventoryTree() *cobra.Command {
	root := &cobra.Command{Use: "det"}
	root.PersistentFlags().Bool("shared", false, "shared persistent flag")

	root.AddCommand(
		&cobra.Command{
			Use:  "zebra <topic>",
			Args: cobra.ExactArgs(1),
		},
		&cobra.Command{
			Use:  "alpha [note]",
			Args: cobra.MaximumNArgs(1),
		},
	)

	alpha := root.Commands()[1]
	alpha.Flags().String("local-only", "", "local flag")
	return root
}

// TestProductionModelsCLICharacterizesCommandGrammar pins the currently
// observable Models command shape at the process boundary. The inventory is
// live output from the production command tree, rather than a copy of the
// generated baseline fixture.
func TestProductionModelsCLICharacterizesCommandGrammar(t *testing.T) {
	observation, err := productionCLIObservation(t)
	if err != nil {
		t.Fatalf("observe production Models CLI: %v", err)
	}

	commands := []struct {
		path string
		id   string
		name bool
	}{
		{path: "you models list", id: "you.models.list"},
		{path: "you models inspect", id: "you.models.inspect", name: true},
		{path: "you models invoke", id: "you.models.invoke", name: true},
		{path: "you models pull", id: "you.models.pull", name: true},
	}

	for _, command := range commands {
		t.Run(command.path, func(t *testing.T) {
			assertModelsCommandCharacterization(t, observation.Snapshot.Inputs, command.path, command.id, command.name)
		})
	}
}

func assertModelsCommandCharacterization(
	t *testing.T,
	inv cliinputs.Inventory,
	commandPath, commandID string,
	wantsModelName bool,
) {
	t.Helper()
	args := modelsCharacterizationArguments(inv, commandPath)
	if wantsModelName {
		want := cliinputs.ArgumentRecord{
			CommandJoin: cliinputs.CommandJoin{CommandPath: commandPath, CommandIDCandidate: commandID},
			IDCandidate: commandID + ".arg.0", Name: "model-name", Position: 0,
			Kind: "positional", ValueType: "string", Required: true,
			MinCardinality: 1, MaxCardinality: 1, Variadic: false, Enum: []string{},
			CompletionKind: "none", InputChannels: []string{"cli"}, DoubleDashHandling: "terminates-flags",
		}
		if len(args) != 1 || !reflect.DeepEqual(args[0], want) {
			t.Fatalf("positional inputs = %#v, want exactly %#v", args, []cliinputs.ArgumentRecord{want})
		}
	} else if len(args) != 0 {
		t.Fatalf("positional inputs = %#v, want none", args)
	}

	wantFlags := []modelsCharacterizationFlag{
		{long: "debug", shorthand: "d", scope: "inherited", valueType: "bool", defaultValue: "false", noOptionDefault: "true", visibility: "visible"},
		{long: "json", scope: "inherited", valueType: "bool", defaultValue: "false", noOptionDefault: "true", visibility: "visible"},
		{long: "remote", scope: "inherited", valueType: "bool", defaultValue: "false", noOptionDefault: "true", visibility: "visible"},
		{long: "server", scope: "inherited", valueType: "string", defaultValue: "http://localhost:7437", visibility: "visible"},
		{long: "verbose", shorthand: "v", scope: "inherited", valueType: "bool", defaultValue: "false", noOptionDefault: "true", visibility: "visible"},
	}

	// The hidden compatibility flag is characterized, not endorsed: the
	// production pre-run guard rejects changing this legacy port input.
	wantFlags = append(wantFlags, modelsCharacterizationFlag{
		long: "port", scope: "local", valueType: "int", defaultValue: "0", visibility: "hidden",
	})
	if commandID == "you.models.invoke" {
		// The generic operation surface is characterized, not endorsed: it is
		// the current public enum exposed by this CLI.
		wantFlags = append(wantFlags,
			modelsCharacterizationFlag{long: "input", scope: "local", valueType: "stringArray", defaultValue: "[]", repeatable: true, visibility: "visible"},
			modelsCharacterizationFlag{long: "operation", scope: "local", valueType: "string", defaultValue: "TTS", enum: []string{"ASR", "EMBED", "OMNI", "TTS"}, normalization: "trim", completionKind: "static", visibility: "visible"},
			modelsCharacterizationFlag{long: "output", scope: "local", valueType: "string", normalization: "trim", visibility: "visible"},
			modelsCharacterizationFlag{long: "output-map", scope: "local", valueType: "stringArray", defaultValue: "[]", repeatable: true, visibility: "visible"},
			modelsCharacterizationFlag{long: "text", scope: "local", valueType: "string", normalization: "trim", visibility: "visible"},
		)
	}

	flags := modelsCharacterizationFlags(inv, commandPath)
	if len(flags) != len(wantFlags) {
		t.Fatalf("flag count = %d, want %d; flags = %#v", len(flags), len(wantFlags), flags)
	}
	for _, want := range wantFlags {
		got := findFlagRecord(t, inv, commandPath, want.long)
		if got == nil {
			t.Fatalf("missing --%s flag", want.long)
		}
		wantRecord := want.record(commandPath, commandID)
		if !reflect.DeepEqual(*got, wantRecord) {
			t.Fatalf("--%s = %#v, want %#v", want.long, *got, wantRecord)
		}
	}
}

type modelsCharacterizationFlag struct {
	long            string
	shorthand       string
	scope           string
	valueType       string
	defaultValue    string
	noOptionDefault string
	enum            []string
	normalization   string
	completionKind  string
	visibility      string
	repeatable      bool
}

func (flag modelsCharacterizationFlag) record(commandPath, commandID string) cliinputs.FlagRecord {
	completionKind := flag.completionKind
	if completionKind == "" {
		completionKind = "none"
	}
	return cliinputs.FlagRecord{
		CommandJoin: cliinputs.CommandJoin{
			CommandPath:        commandPath,
			CommandIDCandidate: commandID,
		},
		IDCandidate:       commandID + ".flag." + flag.long,
		Long:              flag.long,
		Shorthand:         flag.shorthand,
		Aliases:           []string{},
		Scope:             flag.scope,
		ValueType:         flag.valueType,
		Required:          false,
		Default:           flag.defaultValue,
		ChangedDefault:    false,
		NoOptionDefault:   flag.noOptionDefault,
		Repeatable:        flag.repeatable,
		Enum:              flag.enum,
		Normalization:     flag.normalization,
		CompletionKind:    completionKind,
		Binding:           "",
		Visibility:        flag.visibility,
		Deprecated:        false,
		DeprecatedMessage: "",
	}
}

func modelsCharacterizationArguments(inv cliinputs.Inventory, commandPath string) []cliinputs.ArgumentRecord {
	arguments := make([]cliinputs.ArgumentRecord, 0, 1)
	for _, argument := range inv.Arguments {
		if argument.CommandPath == commandPath {
			arguments = append(arguments, argument)
		}
	}
	return arguments
}

func modelsCharacterizationFlags(inv cliinputs.Inventory, commandPath string) []cliinputs.FlagRecord {
	flags := make([]cliinputs.FlagRecord, 0)
	for _, flag := range inv.Flags {
		if flag.CommandPath == commandPath {
			flags = append(flags, flag)
		}
	}
	return flags
}
