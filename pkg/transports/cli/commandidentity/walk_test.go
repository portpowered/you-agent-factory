package commandidentity_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandidentity"
	"github.com/spf13/cobra"
)

func TestWalk_SyntheticTreeRecordsCommandIdentityFields(t *testing.T) {
	root := newSyntheticCommandTree()

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	if inventory.FormatVersion != commandidentity.FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, commandidentity.FormatVersion)
	}
	if inventory.RootPath != "synth" {
		t.Fatalf("RootPath = %q, want synth", inventory.RootPath)
	}

	byPath := indexCommandsByPath(t, inventory.Commands)
	cases := syntheticCommandCases()

	if len(inventory.Commands) != len(cases) {
		t.Fatalf("Commands len = %d, want %d", len(inventory.Commands), len(cases))
	}

	for _, tc := range cases {
		record, ok := byPath[tc.path]
		if !ok {
			t.Fatalf("missing command record for path %q", tc.path)
		}
		assertSyntheticCommandRecord(t, tc, record)
	}
}

func assertSyntheticCommandRecord(t *testing.T, tc syntheticCommandCase, record commandidentity.CommandRecord) {
	t.Helper()

	if record.IDCandidate != tc.idCandidate {
		t.Fatalf("%s idCandidate = %q, want %q", tc.path, record.IDCandidate, tc.idCandidate)
	}
	if record.Name != tc.name {
		t.Fatalf("%s name = %q, want %q", tc.path, record.Name, tc.name)
	}
	if !reflect.DeepEqual(record.Aliases, tc.aliases) {
		t.Fatalf("%s aliases = %#v, want %#v", tc.path, record.Aliases, tc.aliases)
	}
	if record.GroupID != tc.groupID {
		t.Fatalf("%s groupId = %q, want %q", tc.path, record.GroupID, tc.groupID)
	}
	if record.Short != tc.short {
		t.Fatalf("%s short = %q, want %q", tc.path, record.Short, tc.short)
	}
	if record.Long != tc.long {
		t.Fatalf("%s long = %q, want %q", tc.path, record.Long, tc.long)
	}
	if record.Example != tc.example {
		t.Fatalf("%s example = %q, want %q", tc.path, record.Example, tc.example)
	}
	if record.Visibility != tc.visibility {
		t.Fatalf("%s visibility = %q, want %q", tc.path, record.Visibility, tc.visibility)
	}
	if record.Lifecycle != tc.lifecycle {
		t.Fatalf("%s lifecycle = %q, want %q", tc.path, record.Lifecycle, tc.lifecycle)
	}
	if record.DeprecatedMessage != tc.deprecatedMessage {
		t.Fatalf("%s deprecatedMessage = %q, want %q", tc.path, record.DeprecatedMessage, tc.deprecatedMessage)
	}
	if record.Runnable != tc.runnable {
		t.Fatalf("%s runnable = %t, want %t", tc.path, record.Runnable, tc.runnable)
	}
	if record.DocIDCandidate != tc.docIDCandidate {
		t.Fatalf("%s docIdCandidate = %q, want %q", tc.path, record.DocIDCandidate, tc.docIDCandidate)
	}
	if record.HandlerPresent != tc.handlerPresent {
		t.Fatalf("%s handlerPresent = %t, want %t", tc.path, record.HandlerPresent, tc.handlerPresent)
	}
}

func TestWalk_DoesNotSerializeFunctionPointers(t *testing.T) {
	root := newSyntheticCommandTree()

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	raw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}

	encoded := string(raw)
	if strings.Contains(encoded, "0x") {
		t.Fatalf("inventory JSON must not serialize function pointers: %s", encoded)
	}
}

func TestWalk_CommandsSortedByFullPath(t *testing.T) {
	root := newSyntheticCommandTree()

	inventory, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	for i := 1; i < len(inventory.Commands); i++ {
		prev := inventory.Commands[i-1].Path
		curr := inventory.Commands[i].Path
		if prev > curr {
			t.Fatalf("commands not sorted by path at index %d: %q > %q", i, prev, curr)
		}
	}
}

func TestWalk_DuplicatePathFails(t *testing.T) {
	root := newDuplicatePathCommandTree()

	_, err := commandidentity.Walk(root)
	if err == nil {
		t.Fatal("Walk() error = nil, want duplicate path failure")
	}
	if got := err.Error(); !strings.Contains(got, `duplicate command path "dup same"`) {
		t.Fatalf("Walk() error = %q, want duplicate path diagnostic naming dup same", got)
	}
}

func TestWalk_DoesNotMutateCommandTree(t *testing.T) {
	oldSorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	t.Cleanup(func() { cobra.EnableCommandSorting = oldSorting })

	root := newUnsortedChildOrderTree()
	beforeOrder := childRegistrationOrder(root)

	_, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	afterOrder := childRegistrationOrder(root)
	if !reflect.DeepEqual(beforeOrder, afterOrder) {
		t.Fatalf("walker mutated child registration order:\nbefore=%#v\nafter=%#v", beforeOrder, afterOrder)
	}
}

func TestWalk_ProducesIdenticalJSONOnRepeat(t *testing.T) {
	root := newSyntheticCommandTree()

	first, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("first Walk() error = %v", err)
	}
	firstJSON, err := commandidentity.MarshalInventory(first)
	if err != nil {
		t.Fatalf("first MarshalInventory() error = %v", err)
	}

	second, err := commandidentity.Walk(root)
	if err != nil {
		t.Fatalf("second Walk() error = %v", err)
	}
	secondJSON, err := commandidentity.MarshalInventory(second)
	if err != nil {
		t.Fatalf("second MarshalInventory() error = %v", err)
	}

	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated walks produced different JSON:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
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

func newDuplicatePathCommandTree() *cobra.Command {
	root := &cobra.Command{Use: "dup"}
	root.AddCommand(
		&cobra.Command{Use: "same", Short: "first"},
		&cobra.Command{Use: "same", Short: "second"},
	)
	return root
}

func newSyntheticCommandTree() *cobra.Command {
	root := &cobra.Command{
		Use:   "synth",
		Short: "root short",
		Long:  "root long",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	nested := &cobra.Command{
		Use:   "nested",
		Short: "nested short",
	}
	nested.AddCommand(&cobra.Command{
		Use:    "hidden-leaf",
		Short:  "hidden short",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	})

	aliased := &cobra.Command{
		Use:     "aliased",
		Short:   "aliased short",
		Aliases: []string{"alias-one", "alias-two"},
		GroupID: "operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	deprecated := &cobra.Command{
		Use:        "deprecated",
		Short:      "deprecated short",
		Deprecated: "use synth nested instead",
	}

	docs := &cobra.Command{
		Use:   "docs",
		Short: "docs short",
	}
	docs.AddCommand(&cobra.Command{
		Use:   "agents",
		Short: "agents short",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	})

	root.AddCommand(nested, aliased, deprecated, docs)
	return root
}

func indexCommandsByPath(t *testing.T, commands []commandidentity.CommandRecord) map[string]commandidentity.CommandRecord {
	t.Helper()

	index := make(map[string]commandidentity.CommandRecord, len(commands))
	for _, record := range commands {
		if _, exists := index[record.Path]; exists {
			t.Fatalf("duplicate path in inventory: %q", record.Path)
		}
		index[record.Path] = record
	}
	return index
}
