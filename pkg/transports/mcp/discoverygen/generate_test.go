package discoverygen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

const canonicalFactorySessionToolCount = 10

func TestProductionCatalogProjectsCanonicalDiscoveryMetadata(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	resolved, err := discoverygen.LoadResolvedCatalog(repositoryRoot)
	if err != nil {
		t.Fatalf("LoadResolvedCatalog() error = %v", err)
	}
	metadata, err := discoverygen.ProjectDiscoveryFromCatalogDocument(resolved)
	if err != nil {
		t.Fatalf("ProjectDiscoveryFromCatalogDocument() error = %v", err)
	}

	if got := len(metadata.Tools); got != canonicalFactorySessionToolCount {
		t.Fatalf("generated tool count = %d, want %d", got, canonicalFactorySessionToolCount)
	}
	if err := discoverygen.VerifyDiscoveryToolIdentityCompleteness(metadata, mcpfactorysession.DiscoverTools()); err != nil {
		t.Fatalf("generated discovery identity completeness: %v", err)
	}
	for id, tool := range metadata.Tools {
		if tool.ID != id {
			t.Errorf("tool map key %q does not match stable id %q", id, tool.ID)
		}
		if !strings.HasPrefix(tool.Name, "you.factory_session.") {
			t.Errorf("tool %q name = %q, want canonical Factory Session name", id, tool.Name)
		}
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %q has empty English description", id)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %q inputSchema type = %#v, want object", id, tool.InputSchema["type"])
		}
	}
}

func TestProductionDiscoveryExcludesCompatibilityAliasesAndUnsupportedModalities(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	payload, err := discoverygen.DiscoveryArtifact(repositoryRoot)
	if err != nil {
		t.Fatalf("DiscoveryArtifact() error = %v", err)
	}

	for _, alias := range mcpfactorysession.DiscoverCompatibilityAliases() {
		if bytes.Contains(payload, []byte(alias.Name)) {
			t.Errorf("generated discovery contains compatibility alias %q", alias.Name)
		}
	}
	for _, forbidden := range []string{
		`"outputSchema"`,
		`"structuredContent"`,
		`"type": "image"`,
		`"type": "audio"`,
		`"type": "resource"`,
	} {
		if bytes.Contains(payload, []byte(forbidden)) {
			t.Errorf("generated discovery advertises unsupported modality %s", forbidden)
		}
	}
}

func TestGenerateIsDeterministicAcrossCleanRuns(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	copyCatalog(t, repositoryRoot, root)

	if err := discoverygen.Generate(root); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	first := readGeneratedArtifact(t, root)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryJSONPath))); err != nil {
		t.Fatalf("remove first generated artifact: %v", err)
	}
	if err := discoverygen.Generate(root); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	second := readGeneratedArtifact(t, root)

	if !bytes.Equal(first, second) {
		t.Fatal("generated discovery metadata changed across two clean runs")
	}
}

func TestProductionDiscoveryArtifactMatchesGenerator(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	drift, err := discoverygen.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production discovery metadata drift = %#v", drift)
	}
}

func copyCatalog(t *testing.T, sourceRoot, targetRoot string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(discoverygen.AuthoredCatalogPath)))
	if err != nil {
		t.Fatalf("read authored catalog: %v", err)
	}
	target := filepath.Join(targetRoot, filepath.FromSlash(discoverygen.AuthoredCatalogPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create authored catalog directory: %v", err)
	}
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		t.Fatalf("write authored catalog: %v", err)
	}
}

func readGeneratedArtifact(t *testing.T, root string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryJSONPath)))
	if err != nil {
		t.Fatalf("read generated discovery artifact: %v", err)
	}
	return payload
}
