package climanifestgen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestWorkflowMCPFamiliesUseClassificationAppropriateSources(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	production, err := climanifest.LoadProduction(filepath.Join(repoRoot, climanifestgen.ProductionManifestPath))
	if err != nil {
		t.Fatal(err)
	}
	compatibility, err := climanifest.LoadCompatibility(filepath.Join(repoRoot, climanifestgen.CompatibilityManifestPath))
	if err != nil {
		t.Fatal(err)
	}

	mcp, err := climanifestgen.ExtractMCPFamily(production)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := climanifestgen.ExtractWorkflowCompatibilityFamily(compatibility)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range climanifestgen.MCPFamilyCommandIDs {
		if _, ok := mcp.Commands[id]; !ok {
			t.Errorf("canonical MCP artifact missing %q", id)
		}
		if _, promoted := workflow.Commands[id]; promoted {
			t.Errorf("compatibility artifact contains canonical command %q", id)
		}
	}
	for _, id := range climanifestgen.WorkflowCompatibilityFamilyCommandIDs {
		if _, ok := workflow.Commands[id]; !ok {
			t.Errorf("workflow compatibility artifact missing %q", id)
		}
		if _, promoted := production.Commands[id]; promoted {
			t.Errorf("primary manifest promotes compatibility command %q", id)
		}
	}
}

func TestWorkflowMCPFamilyCommandIDAssertionsRespectClassification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		assertID   func(string) error
		acceptedID string
		rejectedID string
	}{
		{
			name:       "canonical MCP",
			assertID:   climanifestgen.AssertMCPFamilyCommandID,
			acceptedID: "you.mcp.serve",
			rejectedID: "you.workflow.validate",
		},
		{
			name:       "workflow compatibility",
			assertID:   climanifestgen.AssertWorkflowCompatibilityFamilyCommandID,
			acceptedID: "you.workflow.preview",
			rejectedID: "you.mcp",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.assertID(tc.acceptedID); err != nil {
				t.Fatalf("assert accepted ID %q: %v", tc.acceptedID, err)
			}
			if err := tc.assertID(tc.rejectedID); err == nil || !strings.Contains(err.Error(), tc.rejectedID) {
				t.Fatalf("assert rejected ID %q error = %v, want stable-ID diagnostic", tc.rejectedID, err)
			}
		})
	}
}

func TestWorkflowMCPArtifactsPreserveParserRelationships(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	for _, tc := range []struct {
		producer func(string) ([]byte, error)
		id       string
	}{
		{producer: climanifestgen.MCPArtifact, id: "you.mcp.serve.relationship.runtime-source"},
		{producer: climanifestgen.WorkflowCompatibilityArtifact, id: "you.workflow.validate.relationship.source-exclusive"},
	} {
		payload, err := tc.producer(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(payload), tc.id) {
			t.Errorf("generated metadata missing relationship %q", tc.id)
		}
	}
}

func TestWorkflowMCPGenerationRejectsClassificationMismatchWithStableID(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	for _, path := range []string{climanifestgen.ProductionManifestPath, climanifestgen.CompatibilityManifestPath} {
		if err := copyFile(filepath.Join(repoRoot, filepath.FromSlash(path)), filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	productionPath := filepath.Join(root, filepath.FromSlash(climanifestgen.ProductionManifestPath))
	payload, err := os.ReadFile(productionPath)
	if err != nil {
		t.Fatal(err)
	}
	payload = bytes.Replace(payload, []byte(`"commands": {`), []byte(`"commands": {"you.workflow.validate":{"id":"you.workflow.validate","path":"you workflow validate"},`), 1)
	if err := os.WriteFile(productionPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = climanifestgen.MCPArtifact(root)
	if err == nil || !strings.Contains(err.Error(), "you.workflow.validate") {
		t.Fatalf("MCPArtifact() error = %v, want classification diagnostic with stable ID", err)
	}
}

func TestCheckNamesStableIDsForStaleWorkflowArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	for _, path := range []string{climanifestgen.ProductionManifestPath, climanifestgen.CompatibilityManifestPath} {
		if err := copyFile(filepath.Join(repoRoot, filepath.FromSlash(path)), filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
	}
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(climanifestgen.WorkflowCompatibilityFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := drift.CommandIDs[climanifestgen.WorkflowCompatibilityFamilyJSONPath]
	if len(ids) != 2 || ids[0] != "you.workflow.preview" || ids[1] != "you.workflow.validate" {
		t.Fatalf("affected command IDs = %v", ids)
	}
}
