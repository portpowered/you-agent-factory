package cliinputs_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/spf13/cobra"
)

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

// Registers children in reverse path order so collection without sorting would
// differ from the documented commandPath-first ordering.
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
