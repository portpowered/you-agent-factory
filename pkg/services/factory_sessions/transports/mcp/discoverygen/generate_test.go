package discoverygen_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	mcpfactorycatalog "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/catalog"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/discoverygen"
)

type testFileSystem struct{}

func (testFileSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (testFileSystem) Remove(path string) error             { return os.Remove(path) }
func (testFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (testFileSystem) WriteFile(path string, payload []byte, mode fs.FileMode) error {
	return os.WriteFile(path, payload, mode)
}
func (testFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func testArtifactStore() generatedartifacts.LocalStore {
	store, err := generatedartifacts.NewLocalStore(testFileSystem{})
	if err != nil {
		panic(err)
	}
	return store
}

const canonicalToolCount = 11

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

	if got := len(metadata.Tools); got != canonicalToolCount {
		t.Fatalf("generated tool count = %d, want %d", got, canonicalToolCount)
	}
	if err := discoverygen.VerifyDiscoveryToolIdentityCompleteness(metadata, canonicalIdentities()); err != nil {
		t.Fatalf("generated discovery identity completeness: %v", err)
	}
	for id, tool := range metadata.Tools {
		if tool.ID != id {
			t.Errorf("tool map key %q does not match stable id %q", id, tool.ID)
		}
		if !strings.HasPrefix(tool.Name, "you.factory_session.") && tool.Name != mcpfactorysession.ToolSubagent {
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

func TestProductionManifestProjectsCanonicalToolsAndDeclaredResourcesAndSkills(t *testing.T) {
	root := testutil.MustRepoPath(t, ".")
	manifest, err := discoverygen.LoadAuthoredManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Tools) != canonicalToolCount {
		t.Fatalf("manifest tool count = %d, want %d", len(manifest.Tools), canonicalToolCount)
	}
	if len(manifest.Resources) != 4 || len(manifest.Skills) != 1 {
		t.Fatalf("manifest surface sizes = tools:%d resources:%d skills:%d", len(manifest.Tools), len(manifest.Resources), len(manifest.Skills))
	}
	metadata, err := discoverygen.ProjectManifestDiscovery(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Tools) != canonicalToolCount ||
		metadata.Tools["mcp.tool.you.subagent"].Name != mcpfactorysession.ToolSubagent ||
		metadata.Tools["mcp.tool.you.factory_session.list"].Name != mcpfactorysession.ToolListSessions {
		t.Fatalf("projected tools = %#v", metadata.Tools)
	}
}

func TestManifestRejectsSkillReferenceWithoutDeclaredResource(t *testing.T) {
	manifest := discoverygen.MCPManifest{FormatVersion: "1.0.0", ProtocolVersion: "2024-11-05", Skills: []discoverygen.ManifestSkill{{ID: "mcp.skill.test", URI: "skill://missing", Frontmatter: map[string]any{"name": "test", "description": "test"}, ResourceURIs: []string{"skill://missing"}}}}
	if err := discoverygen.ValidateManifest(manifest); err == nil {
		t.Fatal("ValidateManifest() accepted undeclared skill resource")
	}
}

func TestValidateManifestRejectsMalformedAndDuplicateSurfaces(t *testing.T) {
	valid := func() discoverygen.MCPManifest {
		return discoverygen.MCPManifest{
			FormatVersion: "1.0.0", ProtocolVersion: "2024-11-05",
			Tools:     []discoverygen.ManifestTool{{ID: "tool-1", Name: "tool", Description: "Tool", Handler: "handleTool", InputSchema: map[string]any{"type": "object"}}},
			Resources: []discoverygen.ManifestResource{{ID: "resource-1", URI: "you://example", Name: "Example", Description: "Example resource", MIMEType: "text/plain", Handler: "handleResource"}},
			Skills:    []discoverygen.ManifestSkill{{ID: "skill-1", URI: "skill://example", Frontmatter: map[string]any{"name": "example", "description": "Example skill"}, ResourceURIs: []string{"you://example"}}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*discoverygen.MCPManifest)
		wantErr string
	}{
		{name: "unsupported version", mutate: func(m *discoverygen.MCPManifest) { m.FormatVersion = "2.0.0" }, wantErr: "unsupported format or protocol version"},
		{name: "incomplete tool", mutate: func(m *discoverygen.MCPManifest) { m.Tools[0].InputSchema = nil }, wantErr: "tool \"tool-1\" is incomplete"},
		{name: "duplicate tool id", mutate: func(m *discoverygen.MCPManifest) { m.Tools = append(m.Tools, m.Tools[0]) }, wantErr: "tool \"tool-1\" or name \"tool\" is duplicated"},
		{name: "duplicate tool name", mutate: func(m *discoverygen.MCPManifest) {
			duplicate := m.Tools[0]
			duplicate.ID = "tool-2"
			m.Tools = append(m.Tools, duplicate)
		}, wantErr: "or name \"tool\" is duplicated"},
		{name: "incomplete resource", mutate: func(m *discoverygen.MCPManifest) { m.Resources[0].Handler = "" }, wantErr: "resource \"resource-1\" is incomplete"},
		{name: "duplicate resource id", mutate: func(m *discoverygen.MCPManifest) {
			duplicate := m.Resources[0]
			duplicate.URI = "you://other"
			m.Resources = append(m.Resources, duplicate)
		}, wantErr: "resource \"resource-1\" or URI \"you://other\" is duplicated"},
		{name: "duplicate resource uri", mutate: func(m *discoverygen.MCPManifest) {
			duplicate := m.Resources[0]
			duplicate.ID = "resource-2"
			m.Resources = append(m.Resources, duplicate)
		}, wantErr: "or URI \"you://example\" is duplicated"},
		{name: "incomplete skill", mutate: func(m *discoverygen.MCPManifest) { m.Skills[0].ResourceURIs = nil }, wantErr: "skill \"skill-1\" is incomplete"},
		{name: "duplicate skill id", mutate: func(m *discoverygen.MCPManifest) {
			m.Skills[0].URI = "you://example"
			duplicate := m.Skills[0]
			m.Skills = append(m.Skills, duplicate)
		}, wantErr: "skill \"skill-1\" is duplicated"},
		{name: "skill uri undeclared", mutate: func(m *discoverygen.MCPManifest) { m.Skills[0].URI = "skill://missing" }, wantErr: "has undeclared skill URI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := valid()
			tt.mutate(&manifest)
			if err := discoverygen.ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateManifest() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestGeneratedGoCarriesEveryManifestSurface(t *testing.T) {
	root := testutil.MustRepoPath(t, ".")
	payload, err := discoverygen.DiscoveryGoArtifact(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"func PrimaryDiscovery() []DiscoveryTool",
		"func LegacyDiscovery() []DiscoveryTool",
		"func PrimaryResources() []DiscoveryResource",
		"func PrimarySkills() []DiscoverySkill",
		"you://providers/catalog",
		"skill://subagent-configuration/SKILL.md",
	} {
		if !bytes.Contains(payload, []byte(expected)) {
			t.Errorf("generated Go descriptor missing %q", expected)
		}
	}
}

func TestProductionDiscoveryExcludesUnsupportedModalities(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	payload, err := discoverygen.DiscoveryArtifact(repositoryRoot)
	if err != nil {
		t.Fatalf("DiscoveryArtifact() error = %v", err)
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

	if err := writeDiscoveryArtifacts(root); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	firstJSON := readGeneratedArtifact(t, root, discoverygen.DiscoveryJSONPath)
	firstGo := readGeneratedArtifact(t, root, discoverygen.DiscoveryGoPath)
	for _, path := range []string{discoverygen.DiscoveryJSONPath, discoverygen.DiscoveryGoPath} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("remove first generated artifact %s: %v", path, err)
		}
	}
	if err := writeDiscoveryArtifacts(root); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	secondJSON := readGeneratedArtifact(t, root, discoverygen.DiscoveryJSONPath)
	secondGo := readGeneratedArtifact(t, root, discoverygen.DiscoveryGoPath)

	if !bytes.Equal(firstJSON, secondJSON) || !bytes.Equal(firstGo, secondGo) {
		t.Fatal("generated discovery artifacts changed across two clean runs")
	}
}

func TestProductionDiscoveryArtifactMatchesGenerator(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	drift, err := checkDiscoveryArtifacts(repositoryRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production discovery metadata drift = %#v", drift)
	}
}

func TestCheckReportsMissingAndStaleArtifacts(t *testing.T) {
	repositoryRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	copyCatalog(t, repositoryRoot, root)
	if err := writeDiscoveryArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryJSONPath))); err != nil {
		t.Fatalf("remove generated JSON: %v", err)
	}
	goPath := filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryGoPath))
	if err := os.WriteFile(goPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale generated Go: %v", err)
	}

	drift, err := checkDiscoveryArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("Check() drift is empty, want missing and stale artifacts")
	}
	if len(drift.Missing) != 1 || drift.Missing[0] != discoverygen.DiscoveryJSONPath {
		t.Fatalf("missing artifacts = %#v", drift.Missing)
	}
	if len(drift.Stale) != 1 || drift.Stale[0] != discoverygen.DiscoveryGoPath {
		t.Fatalf("stale artifacts = %#v", drift.Stale)
	}
}

func writeDiscoveryArtifacts(root string) error {
	artifacts, err := discoverygen.Artifacts(root)
	if err != nil {
		return err
	}
	return testArtifactStore().Write(root, artifacts)
}

func checkDiscoveryArtifacts(root string) (generatedartifacts.Drift, error) {
	artifacts, err := discoverygen.Artifacts(root)
	if err != nil {
		return generatedartifacts.Drift{}, err
	}
	return testArtifactStore().Check(root, artifacts)
}

func TestProjectDiscoveryRejectsMalformedCatalogToolRecords(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		mutate  func(map[string]any)
		wantErr string
	}{
		{name: "empty id", mutate: func(record map[string]any) { record["id"] = "" }, wantErr: "has empty id"},
		{name: "empty name", mutate: func(record map[string]any) { record["name"] = "" }, wantErr: "has empty name"},
		{name: "id mismatch", mutate: func(record map[string]any) { record["id"] = "mcp.tool.you.factory_session.other" }, wantErr: "does not match record id"},
		{name: "noncanonical id", key: "invalid", mutate: func(record map[string]any) { record["id"] = "invalid" }, wantErr: "is not a canonical tool id"},
		{name: "missing documentation", mutate: func(record map[string]any) { delete(record, "documentation") }, wantErr: "missing documentation"},
		{name: "missing input", mutate: func(record map[string]any) { delete(record, "input") }, wantErr: "input is not an object"},
		{name: "missing schema", mutate: func(record map[string]any) { record["input"] = map[string]any{} }, wantErr: "input.schema is not an object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "mcp.tool.you.factory_session.example"
			if tt.key != "" {
				key = tt.key
			}
			record := validCatalogToolRecord(key)
			tt.mutate(record)
			_, err := discoverygen.ProjectDiscoveryFromCatalogDocument(map[string]any{
				"tools": map[string]any{key: record},
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ProjectDiscoveryFromCatalogDocument() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoveryVerificationRejectsAliasesAndNestedModalities(t *testing.T) {
	alias := discoverygen.DiscoveryMetadata{Tools: map[string]discoverygen.DiscoveryToolRecord{
		"mcp.tool.you.workflow.run": {
			ID:   "mcp.tool.you.workflow.run",
			Name: "you.workflow.run",
		},
	}}
	if err := discoverygen.VerifyDiscoveryAliasExclusion(alias); err == nil {
		t.Fatal("VerifyDiscoveryAliasExclusion() error = nil, want compatibility alias rejection")
	}

	tests := []struct {
		name    string
		schema  map[string]any
		wantErr string
	}{
		{name: "record field", schema: map[string]any{"outputSchema": map[string]any{}}, wantErr: `must not include "outputSchema"`},
		{name: "nested property", schema: map[string]any{"properties": map[string]any{"payload": map[string]any{"structuredContent": true}}}, wantErr: "properties.payload"},
		{name: "map items content", schema: map[string]any{"items": map[string]any{"content": []any{map[string]any{"type": "image"}}}}, wantErr: `content[0].type = "image"`},
		{
			name: "tuple items content",
			schema: map[string]any{
				"items": []any{
					map[string]any{"content": []any{map[string]any{"type": "audio"}}},
				},
			},
			wantErr: "items[0]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const id = "mcp.tool.you.factory_session.example"
			metadata := discoverygen.DiscoveryMetadata{Tools: map[string]discoverygen.DiscoveryToolRecord{
				id: {ID: id, Name: "you.factory_session.example", InputSchema: tt.schema},
			}}
			if err := discoverygen.VerifyDiscoveryModalityPolicy(metadata); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("VerifyDiscoveryModalityPolicy() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func validCatalogToolRecord(id string) map[string]any {
	return map[string]any{
		"id":   id,
		"name": "you.factory_session.example",
		"documentation": map[string]any{
			"documentation": map[string]any{
				"description": map[string]any{"canonicalEnglish": "Example tool."},
			},
		},
		"input": map[string]any{"schema": map[string]any{"type": "object"}},
	}
}

func copyCatalog(t *testing.T, sourceRoot, targetRoot string) {
	t.Helper()
	for _, path := range []string{discoverygen.AuthoredCatalogPath, discoverygen.AuthoredManifestPath} {
		payload, err := os.ReadFile(filepath.Join(sourceRoot, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read authored %s: %v", path, err)
		}
		target := filepath.Join(targetRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("create authored contract directory: %v", err)
		}
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			t.Fatalf("write authored %s: %v", path, err)
		}
	}
}

func readGeneratedArtifact(t *testing.T, root, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read generated discovery artifact: %v", err)
	}
	return payload
}

func canonicalIdentities() []mcpfactorycatalog.CatalogToolIdentity {
	var identities []mcpfactorycatalog.CatalogToolIdentity
	for _, binding := range mcpfactorysession.ProjectCanonicalToolHandlerBindings() {
		identities = append(identities, mcpfactorycatalog.CatalogToolIdentity{ID: binding.ToolID, Name: strings.TrimPrefix(binding.ToolID, "mcp.tool.")})
	}
	return identities
}
