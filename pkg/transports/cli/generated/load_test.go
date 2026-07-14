package generated_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestModelsDocsFamilyManifestMatchesContractedIDs(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatalf("ModelsDocsFamilyManifest() error = %v", err)
	}
	if len(manifest.Commands) != len(climanifestgen.ModelsDocsFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(manifest.Commands), len(climanifestgen.ModelsDocsFamilyCommandIDs))
	}
	for _, id := range climanifestgen.ModelsDocsFamilyCommandIDs {
		record, err := manifest.CommandByID(id)
		if err != nil {
			t.Fatalf("CommandByID(%q) error = %v", id, err)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
	}
}

func TestModelsDocsFamilyCommandIDsGenMatchesGeneratorList(t *testing.T) {
	if len(generated.ModelsDocsFamilyCommandIDs) != len(climanifestgen.ModelsDocsFamilyCommandIDs) {
		t.Fatalf("generated id count = %d, want %d", len(generated.ModelsDocsFamilyCommandIDs), len(climanifestgen.ModelsDocsFamilyCommandIDs))
	}
	for i, id := range climanifestgen.ModelsDocsFamilyCommandIDs {
		if generated.ModelsDocsFamilyCommandIDs[i] != id {
			t.Fatalf("generated ids[%d] = %q, want %q", i, generated.ModelsDocsFamilyCommandIDs[i], id)
		}
	}
}

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

func TestWorkFamilyCommandIDsGenMatchesGeneratorList(t *testing.T) {
	if len(generated.WorkFamilyCommandIDs) != len(climanifestgen.WorkFamilyCommandIDs) {
		t.Fatalf("generated id count = %d, want %d", len(generated.WorkFamilyCommandIDs), len(climanifestgen.WorkFamilyCommandIDs))
	}
	for i, id := range climanifestgen.WorkFamilyCommandIDs {
		if generated.WorkFamilyCommandIDs[i] != id {
			t.Fatalf("generated ids[%d] = %q, want %q", i, generated.WorkFamilyCommandIDs[i], id)
		}
	}
}

func TestWorkFamilyManifestMatchesContractedIDs(t *testing.T) {
	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		t.Fatalf("WorkFamilyManifest() error = %v", err)
	}
	if len(manifest.Commands) != len(climanifestgen.WorkFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(manifest.Commands), len(climanifestgen.WorkFamilyCommandIDs))
	}
	for _, id := range climanifestgen.WorkFamilyCommandIDs {
		record, err := generated.WorkCommandByID(id)
		if err != nil {
			t.Fatalf("WorkCommandByID(%q) error = %v", id, err)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
	}
}

func TestWorkCommandByIDRejectsUnknownWorkFamilyID(t *testing.T) {
	if _, err := generated.WorkCommandByID("you.work.submit"); err == nil {
		t.Fatal("WorkCommandByID(you.work.submit) = nil, want error")
	}
}

func TestCommandByIDRejectsUnknownRepresentativeFamilyID(t *testing.T) {
	if _, err := generated.CommandByID("you.session.list"); err == nil {
		t.Fatal("CommandByID(you.session.list) = nil, want error")
	}
}
