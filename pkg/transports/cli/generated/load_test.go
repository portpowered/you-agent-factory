package generated_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestFactoryConfigInitFamilyManifestMatchesContractedIDs(t *testing.T) {
	manifest, err := generated.FactoryConfigInitFamilyManifest()
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyManifest() error = %v", err)
	}
	if len(manifest.Commands) != len(climanifestgen.FactoryConfigInitFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(manifest.Commands), len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	}
	for _, id := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		record, err := generated.FactoryConfigInitCommandByID(id)
		if err != nil {
			t.Fatalf("FactoryConfigInitCommandByID(%q) error = %v", id, err)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
	}
}

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

func TestRunSubmitFamilyManifestMatchesContractedIDs(t *testing.T) {
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	if len(manifest.Commands) != len(climanifestgen.RunSubmitFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(manifest.Commands), len(climanifestgen.RunSubmitFamilyCommandIDs))
	}
	for i, id := range climanifestgen.RunSubmitFamilyCommandIDs {
		record, err := manifest.CommandByID(id)
		if err != nil {
			t.Fatalf("CommandByID(%q) error = %v", id, err)
		}
		if record.ID != id || generated.RunSubmitFamilyCommandIDs[i] != id {
			t.Fatalf("run/submit id[%d] record=%q generated=%q want=%q", i, record.ID, generated.RunSubmitFamilyCommandIDs[i], id)
		}
	}
}

func TestFactoryConfigInitFamilyCommandIDsGenMatchesGeneratorList(t *testing.T) {
	if len(generated.FactoryConfigInitFamilyCommandIDs) != len(climanifestgen.FactoryConfigInitFamilyCommandIDs) {
		t.Fatalf("generated id count = %d, want %d", len(generated.FactoryConfigInitFamilyCommandIDs), len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	}
	for i, id := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		if generated.FactoryConfigInitFamilyCommandIDs[i] != id {
			t.Fatalf("generated ids[%d] = %q, want %q", i, generated.FactoryConfigInitFamilyCommandIDs[i], id)
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

func TestFactoryConfigInitCommandByIDRejectsUnknownID(t *testing.T) {
	if _, err := generated.FactoryConfigInitCommandByID("you.session.show"); err == nil {
		t.Fatal("FactoryConfigInitCommandByID(you.session.show) = nil, want error")
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
