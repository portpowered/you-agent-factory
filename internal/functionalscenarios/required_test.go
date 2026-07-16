package functionalscenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRequiredScenariosAcceptsRunnableShortCustomerBoundaryTest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRequiredTestFixture(t, root, "tests/functional/api_test.go", `package functional

import "testing"

func TestRequiredCustomerFlow(t *testing.T) {}
`)
	if err := CheckRequiredScenarios(root, requiredFixture("tests/functional/api_test.go::TestRequiredCustomerFlow")); err != nil {
		t.Fatalf("CheckRequiredScenarios() error = %v", err)
	}
}

func TestCheckRequiredScenariosReportsStableViolationCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		content   string
		edit      func(*RequiredScenario)
		category  string
		detail    string
	}{
		{
			name: "missing", reference: "tests/functional/missing_test.go::TestRequiredCustomerFlow",
			category: ViolationMissingTest, detail: "does not exist",
		},
		{
			name: "renamed", reference: "tests/functional/api_test.go::TestOldName",
			content: `package functional
import "testing"
func TestNewName(t *testing.T) {}
`, category: ViolationRenamedTest, detail: `test symbol "TestOldName" does not exist`,
		},
		{
			name: "skipped", reference: "tests/functional/api_test.go::TestRequiredCustomerFlow",
			content: `package functional
import "testing"
func TestRequiredCustomerFlow(t *testing.T) { t.Skip("disabled") }
`, category: ViolationSkippedTest, detail: "contains a skip path",
		},
		{
			name: "long file", reference: "tests/functional/api_long_test.go::TestRequiredCustomerFlow",
			content: `package functional
import "testing"
func TestRequiredCustomerFlow(t *testing.T) {}
`, category: ViolationLongOnly, detail: "long-lane file",
		},
		{
			name: "long lane classification", reference: "tests/functional/api_test.go::TestRequiredCustomerFlow",
			content: `package functional
import "testing"
func TestRequiredCustomerFlow(t *testing.T) {}
`, edit: func(scenario *RequiredScenario) { scenario.Lane = LaneLong }, category: ViolationLongOnly, detail: `lane is "long"`,
		},
		{
			name: "non deterministic", reference: "tests/functional/api_test.go::TestRequiredCustomerFlow",
			content: `package functional
import "testing"
func TestRequiredCustomerFlow(t *testing.T) {}
`, edit: func(scenario *RequiredScenario) { scenario.ExecutionClass = ExecutionExternal }, category: ViolationInvalidClassification, detail: `executionClass="external"`,
		},
		{
			name: "not customer boundary", reference: "tests/functional/api_test.go::TestRequiredCustomerFlow",
			content: `package functional
import "testing"
func TestRequiredCustomerFlow(t *testing.T) {}
`, edit: func(scenario *RequiredScenario) { scenario.CustomerBoundary = false }, category: ViolationInvalidClassification, detail: "customerBoundary=false",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if test.content != "" {
				path, _, _ := strings.Cut(test.reference, "::")
				writeRequiredTestFixture(t, root, path, test.content)
			}
			manifest := requiredFixture(test.reference)
			if test.edit != nil {
				test.edit(&manifest.Scenarios[0])
			}
			err := CheckRequiredScenarios(root, manifest)
			wantPrefix := `required functional scenario "cli/you.run" [` + test.category + `]:`
			if err == nil || !strings.Contains(err.Error(), wantPrefix) || !strings.Contains(err.Error(), test.detail) || !strings.Contains(err.Error(), "restore or correct") {
				t.Fatalf("CheckRequiredScenarios() error = %v, want %q, %q, and remediation", err, wantPrefix, test.detail)
			}
		})
	}
}

func requiredFixture(reference string) *RequiredManifest {
	return &RequiredManifest{
		FormatVersion: RequiredManifestFormatVersion,
		Scenarios: []RequiredScenario{{
			StableID: "cli/you.run", Test: reference, Interface: InterfaceCLI,
			Lane: LaneShort, ExecutionClass: ExecutionDeterministic, CustomerBoundary: true,
		}},
	}
}

func writeRequiredTestFixture(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
