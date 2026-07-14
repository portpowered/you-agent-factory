package generated_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestRepresentativeFamilyManifestMatchesContractedIDs(t *testing.T) {
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		t.Fatalf("RepresentativeFamilyManifest() error = %v", err)
	}
	if len(manifest.Commands) != len(climanifestgen.RepresentativeFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(manifest.Commands), len(climanifestgen.RepresentativeFamilyCommandIDs))
	}
	for _, id := range climanifestgen.RepresentativeFamilyCommandIDs {
		record, err := generated.CommandByID(id)
		if err != nil {
			t.Fatalf("CommandByID(%q) error = %v", id, err)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
	}
}

func TestRepresentativeFamilyCommandIDsGenMatchesGeneratorList(t *testing.T) {
	if len(generated.RepresentativeFamilyCommandIDs) != len(climanifestgen.RepresentativeFamilyCommandIDs) {
		t.Fatalf("generated id count = %d, want %d", len(generated.RepresentativeFamilyCommandIDs), len(climanifestgen.RepresentativeFamilyCommandIDs))
	}
	for i, id := range climanifestgen.RepresentativeFamilyCommandIDs {
		if generated.RepresentativeFamilyCommandIDs[i] != id {
			t.Fatalf("generated ids[%d] = %q, want %q", i, generated.RepresentativeFamilyCommandIDs[i], id)
		}
	}
}

func TestCommandByIDRejectsUnknownRepresentativeFamilyID(t *testing.T) {
	if _, err := generated.CommandByID("you.session.list"); err == nil {
		t.Fatal("CommandByID(you.session.list) = nil, want error")
	}
}
