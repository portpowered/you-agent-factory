package cliinputs_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/cliinputs"
	"github.com/spf13/cobra"
)

func TestInventoryDocumentShape(t *testing.T) {
	inv := cliinputs.EmptyInventory()

	raw, err := cliinputs.MarshalInventory(inv)
	if err != nil {
		t.Fatalf("MarshalInventory() error = %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal inventory JSON: %v", err)
	}

	for _, key := range []string{"formatVersion", "arguments", "flags", "relationships"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("inventory JSON missing top-level key %q: %s", key, raw)
		}
	}

	if got := string(decoded["formatVersion"]); got != `"cli-command-inputs/v1"` {
		t.Fatalf("formatVersion = %s, want %q", got, cliinputs.FormatVersion)
	}
}

func TestRecordTypesExposeCommandJoinFields(t *testing.T) {
	join := cliinputs.CommandJoin{
		CommandPath:        "you run",
		CommandIDCandidate: "you.run",
	}

	argument := cliinputs.ArgumentRecord{
		CommandJoin: join,
		IDCandidate: "you.run.arg.0",
		Name:        "prompt",
		Kind:        "positional",
		Position:    0,
	}
	flag := cliinputs.FlagRecord{
		CommandJoin: join,
		IDCandidate: "you.run.flag.dir",
		Long:        "dir",
		Scope:       "local",
	}
	relationship := cliinputs.RelationshipRecord{
		CommandJoin:  join,
		IDCandidate:  "you.run.rel.mutex.factory",
		Kind:         "mutually-exclusive",
		Participants: []string{"factory", "named"},
	}

	for _, tc := range []struct {
		name string
		got  any
	}{
		{name: "argument", got: argument},
		{name: "flag", got: flag},
		{name: "relationship", got: relationship},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.got)
			if err != nil {
				t.Fatalf("marshal %s record: %v", tc.name, err)
			}

			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal %s record JSON: %v", tc.name, err)
			}

			for _, key := range []string{"commandPath", "commandIdCandidate"} {
				if _, ok := decoded[key]; !ok {
					t.Fatalf("%s record JSON missing join key %q: %s", tc.name, key, raw)
				}
			}
			if got := string(decoded["commandPath"]); got != `"you run"` {
				t.Fatalf("%s commandPath = %s, want %q", tc.name, got, join.CommandPath)
			}
			if got := string(decoded["commandIdCandidate"]); got != `"you.run"` {
				t.Fatalf("%s commandIdCandidate = %s, want %q", tc.name, got, join.CommandIDCandidate)
			}
		})
	}
}

func TestWalk_ReturnsStableEmptyFlagAndRelationshipCollections(t *testing.T) {
	root := &cobra.Command{Use: "synth"}

	inv, err := cliinputs.Walk(root)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	if inv.FormatVersion != cliinputs.FormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inv.FormatVersion, cliinputs.FormatVersion)
	}
	if inv.Flags == nil {
		t.Fatal("Flags = nil, want non-nil empty slice")
	}
	if inv.Relationships == nil {
		t.Fatal("Relationships = nil, want non-nil empty slice")
	}
	if len(inv.Flags) != 0 || len(inv.Relationships) != 0 {
		t.Fatalf("Walk() returned non-empty flag/relationship collections: flags=%d relationships=%d",
			len(inv.Flags), len(inv.Relationships))
	}
}
