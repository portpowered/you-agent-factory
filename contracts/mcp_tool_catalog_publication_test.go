package contracts_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/contractvalidator"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
)

func TestMCPToolCatalogPublication_AuthoredCatalogPassesGuards(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	if err := mcpfactorycatalog.VerifyCatalogAliasExclusion(resolved); err != nil {
		t.Fatalf("VerifyCatalogAliasExclusion() error = %v", err)
	}
	if err := mcpfactorycatalog.VerifyCatalogModalityPolicy(resolved); err != nil {
		t.Fatalf("VerifyCatalogModalityPolicy() error = %v", err)
	}
	if err := mcpfactorycatalog.VerifyAuthoredCatalogStagingBoundary(resolved); err != nil {
		t.Fatalf("VerifyAuthoredCatalogStagingBoundary() error = %v", err)
	}
	if err := mcpfactorycatalog.VerifyCatalogByteStability(resolved); err != nil {
		t.Fatalf("VerifyCatalogByteStability() error = %v", err)
	}
}

func TestMCPToolCatalogPublication_OnDiskCatalogIsCanonicallyByteStable(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	onDisk, err := os.ReadFile(filepath.Join(root, "contracts/mcp/tools.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	var document any
	if err := json.Unmarshal(onDisk, &document); err != nil {
		t.Fatalf("unmarshal authored catalog: %v", err)
	}
	canonical, err := mcpfactorycatalog.MarshalCatalogDocumentJSON(document)
	if err != nil {
		t.Fatalf("MarshalCatalogDocumentJSON() error = %v", err)
	}
	if !bytes.Equal(onDisk, canonical) {
		t.Fatalf("contracts/mcp/tools.json is not canonically byte-stable; re-serialize and commit the authored catalog")
	}
}

func TestMCPToolCatalogPublication_SurfaceManifestIsStagingSource(t *testing.T) {
	for _, artifact := range contractstaging.RawArtifacts() {
		if artifact.Target != "packages/api/generated/mcp/manifest.json" {
			continue
		}
		if artifact.Source != "contracts/mcp/manifest.json" {
			t.Fatalf("staged MCP manifest source = %q, want contracts/mcp/manifest.json", artifact.Source)
		}
		return
	}
	t.Fatal("MCP manifest raw artifact is missing")
}

func TestMCPToolCatalogPublication_StagedManifestMatchesAuthoredSurface(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	authored, err := os.ReadFile(filepath.Join(root, "contracts/mcp/manifest.json"))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	staged, err := os.ReadFile(filepath.Join(root, "packages/api/generated/mcp/manifest.json"))
	if err != nil {
		t.Fatalf("read staged MCP tools inventory: %v", err)
	}
	if !bytes.Equal(authored, staged) {
		t.Fatal("generated package MCP manifest differs from its authored surface")
	}
}

func TestMCPToolCatalogPublication_ContractValidatorPassesForAuthoredCatalog(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	diagnostics := contractvalidator.Validate(root, contractvalidator.MCPRegistry(), "mcp", "1.0.0")
	for _, diagnostic := range diagnostics {
		if diagnostic.Document != "contracts/mcp/tools.json" {
			continue
		}
		switch diagnostic.Code {
		case "catalog.publication.alias",
			"catalog.publication.modality",
			"catalog.publication.staging_boundary",
			"catalog.publication.byte_stability":
			t.Fatalf("unexpected publication diagnostic: %+v", diagnostic)
		}
	}
}
