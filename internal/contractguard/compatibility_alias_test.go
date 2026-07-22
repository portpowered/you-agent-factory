package contractguard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

func TestLoadCompatibilityAliasTerms_ProductionInventoriesContainNoWorkflowAliases(t *testing.T) {
	root := repositoryRoot(t)
	terms, err := contractguard.LoadCompatibilityAliasTerms(root)
	if err != nil {
		t.Fatalf("LoadCompatibilityAliasTerms() error = %v", err)
	}
	if len(terms) != 0 {
		t.Fatalf("production workflow alias terms = %#v, want none", terms)
	}
}

func TestScanCompatibilityAliasViolations_RejectsDeliberateAdoption(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"contracts/mcp/deprecated.json": `{
  "formatVersion": "1.0.0",
  "family": "mcp",
  "records": {
    "mcp.alias.you.workflow.validate": {
      "itemId": "mcp.alias.you.workflow.validate",
      "family": "mcp",
      "publicName": "you.workflow.validate"
    }
  }
}`,
		"contracts/cli/deprecated.json": `{"formatVersion":"1.0.0","family":"cli","records":{}}`,
		"contracts/api/deprecated.json": `{"formatVersion":"1.0.0","family":"api","records":{}}`,
		"pkg/work/adopter.go": `package work

const adoptedAlias = "you.workflow.validate"
`,
	})

	terms, err := contractguard.LoadCompatibilityAliasTerms(root)
	if err != nil {
		t.Fatalf("LoadCompatibilityAliasTerms() error = %v", err)
	}
	violations, err := contractguard.ScanCompatibilityAliasViolations(root, terms, contractguard.DefaultCompatibilityAliasBoundaryPrefixes)
	if err != nil {
		t.Fatalf("ScanCompatibilityAliasViolations() error = %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one deliberate adoption", violations)
	}
	if violations[0].FilePath != "pkg/work/adopter.go" || violations[0].Term != "you.workflow.validate" {
		t.Fatalf("violations[0] = %#v, want pkg/work/adopter.go you.workflow.validate", violations[0])
	}
}

func TestScanCompatibilityAliasViolations_AllowsApprovedBoundary(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"contracts/mcp/deprecated.json": `{
  "formatVersion": "1.0.0",
  "family": "mcp",
  "records": {
    "mcp.alias.you.workflow.validate": {
      "itemId": "mcp.alias.you.workflow.validate",
      "family": "mcp",
      "publicName": "you.workflow.validate"
    }
  }
}`,
		"contracts/cli/deprecated.json": `{"formatVersion":"1.0.0","family":"cli","records":{}}`,
		"contracts/api/deprecated.json": `{"formatVersion":"1.0.0","family":"api","records":{}}`,
		"pkg/transports/mcp/factorysession/tool.go": `package factorysession

const ToolWorkflowValidate = "you.workflow.validate"
`,
	})

	terms, err := contractguard.LoadCompatibilityAliasTerms(root)
	if err != nil {
		t.Fatalf("LoadCompatibilityAliasTerms() error = %v", err)
	}
	violations, err := contractguard.ScanCompatibilityAliasViolations(root, terms, contractguard.DefaultCompatibilityAliasBoundaryPrefixes)
	if err != nil {
		t.Fatalf("ScanCompatibilityAliasViolations() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("approved boundary produced violations: %#v", violations)
	}
}

func TestScanCompatibilityAliasViolations_PassesOnRepositoryRoot(t *testing.T) {
	root := repositoryRoot(t)
	terms, err := contractguard.LoadCompatibilityAliasTerms(root)
	if err != nil {
		t.Fatalf("LoadCompatibilityAliasTerms() error = %v", err)
	}
	violations, err := contractguard.ScanCompatibilityAliasViolations(root, terms, contractguard.DefaultCompatibilityAliasBoundaryPrefixes)
	if err != nil {
		t.Fatalf("ScanCompatibilityAliasViolations() error = %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("repository scan produced violations: %#v", violations)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func fixtureRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create fixture directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", relative, err)
		}
	}
	return root
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
