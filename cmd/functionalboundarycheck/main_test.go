package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourceAcceptsCustomerBoundaryRequestBatchScenario(t *testing.T) {
	root, path := writeFunctionalSource(t, `package runtime_api
import (
  "github.com/portpowered/infinite-you/pkg/transports/http/generated"
  "github.com/portpowered/infinite-you/tests/functional/internal/support"
)
`)

	if err := checkSource(root, path); err != nil {
		t.Fatalf("checkSource() error = %v", err)
	}
}

func TestCheckSourceRejectsDirectRequestBatchProjectionInternal(t *testing.T) {
	root, path := writeFunctionalSource(t, `package guards_batch
import "github.com/portpowered/infinite-you/pkg/services/recordings/projections"
`)

	err := checkSource(root, path)
	if err == nil {
		t.Fatal("checkSource() error = nil, want direct-internal boundary failure")
	}
	if !strings.Contains(err.Error(), diagnosticPrefix+" prohibited direct request-batch internal import: github.com/portpowered/infinite-you/pkg/services/recordings/projections") {
		t.Fatalf("checkSource() error = %q, want stable actionable diagnostic", err)
	}
	if !strings.Contains(err.Error(), "use generated REST/SSE customers or tests/functional/internal/support instead") {
		t.Fatalf("checkSource() error = %q, want customer-boundary remediation", err)
	}
}

func TestCheckSourceRejectsDirectRequestBatchRuntimeInternal(t *testing.T) {
	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime",
		"github.com/portpowered/infinite-you/pkg/service",
		"github.com/portpowered/infinite-you/pkg/orchestrators/petri",
	} {
		t.Run(importPath, func(t *testing.T) {
			root, path := writeFunctionalSource(t, "package guards_batch\nimport \""+importPath+"\"\n")
			if err := checkSource(root, path); err == nil || !strings.Contains(err.Error(), importPath) {
				t.Fatalf("checkSource() error = %v, want boundary failure for %s", err, importPath)
			}
		})
	}
}

func TestCheckSourceRejectsNonFunctionalTestPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pkg", "factory", "request_batch_test.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create non-functional fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte("package factory\n"), 0o600); err != nil {
		t.Fatalf("write non-functional fixture: %v", err)
	}
	if err := checkSource(root, path); err == nil || !strings.Contains(err.Error(), "repository tests/functional/*_test.go") {
		t.Fatalf("checkSource() error = %v, want functional-path diagnostic", err)
	}
}

func TestCheckSourceChecksMigratedScenarioWithoutLegacyQuarantine(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	for _, scenarioPath := range append([]string{defaultScenarioPath}, workSubmissionScenarioPaths...) {
		path := filepath.Join(repoRoot, filepath.FromSlash(scenarioPath))
		if err := checkSource(repoRoot, path); err != nil {
			t.Fatalf("checkSource(%q) error = %v", scenarioPath, err)
		}
	}
}

func TestCheckFunctionalCompositionTreeAcceptsRootProcess(t *testing.T) {
	root, _ := writeFunctionalSource(t, `package smoke
import (
  "github.com/portpowered/infinite-you/pkg/root"
  "github.com/portpowered/infinite-you/pkg/services/edges"
)
func scenario() {
  process, _ := root.BuildProcess(nil, edges.Edges{})
  _ = process
}
`)
	if err := checkFunctionalCompositionTree(root); err != nil {
		t.Fatalf("checkFunctionalCompositionTree() error = %v", err)
	}
}

func TestCheckFunctionalCompositionTreeAcceptsProviderSharedProcessHarness(t *testing.T) {
	root, _ := writeProviderFunctionalSource(t, `package codex
import (
  "github.com/portpowered/infinite-you/pkg/services/edges"
  "github.com/portpowered/infinite-you/tests/functional/internal/support"
)
func scenario() {
  process := support.BuildProcess(nil, edges.Edges{})
  _ = process
}
`)
	if err := checkFunctionalCompositionTree(root); err != nil {
		t.Fatalf("checkFunctionalCompositionTree() error = %v", err)
	}
}

func TestCheckFunctionalCompositionTreeAcceptsProviderPublicContracts(t *testing.T) {
	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/edges",
		"github.com/portpowered/infinite-you/pkg/services/models",
		"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract",
	} {
		t.Run(importPath, func(t *testing.T) {
			root, _ := writeProviderFunctionalSource(t, "package codex\nimport _ \""+importPath+"\"\n")
			if err := checkFunctionalCompositionTree(root); err != nil {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want public contract accepted", err)
			}
		})
	}
}

func TestCheckFunctionalCompositionTreeRejectsProviderDirectRootComposition(t *testing.T) {
	root, _ := writeProviderFunctionalSource(t, `package codex
import (
  "github.com/portpowered/infinite-you/pkg/root"
  "github.com/portpowered/infinite-you/pkg/services/edges"
)
func scenario() {
  process, _ := root.BuildProcess(nil, edges.Edges{})
  _ = process
}
`)
	err := checkFunctionalCompositionTree(root)
	if err == nil || !strings.Contains(err.Error(), "prohibited provider functional composition import") {
		t.Fatalf("checkFunctionalCompositionTree() error = %v, want provider composition failure", err)
	}
	if !strings.Contains(err.Error(), "tests/functional/internal/support.BuildProcess with exact edges.Edges replacements") {
		t.Fatalf("checkFunctionalCompositionTree() error = %v, want shared typed-edge harness remediation", err)
	}
}

func TestCheckFunctionalCompositionTreeRejectsConcreteProviderImplementation(t *testing.T) {
	for _, importPath := range forbiddenProviderImplementationImports {
		t.Run(importPath, func(t *testing.T) {
			root, _ := writeProviderFunctionalSource(t, "package codex\nimport _ \""+importPath+"\"\n")
			err := checkFunctionalCompositionTree(root)
			if err == nil || !strings.Contains(err.Error(), "prohibited concrete provider implementation import: "+importPath) {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want concrete provider failure", err)
			}
			if !strings.Contains(err.Error(), "support.BuildProcess with exact edges.Edges replacements") {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want shared typed-edge harness remediation", err)
			}
		})
	}
}

func TestCheckFunctionalCompositionTreeRejectsProviderServiceImplementation(t *testing.T) {
	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/services/models/wire",
		"github.com/portpowered/infinite-you/pkg/services/models/internal/service",
		"github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution",
	} {
		t.Run(importPath, func(t *testing.T) {
			root, _ := writeProviderFunctionalSource(t, "package codex\nimport _ \""+importPath+"\"\n")
			err := checkFunctionalCompositionTree(root)
			if err == nil || !strings.Contains(err.Error(), "prohibited provider service implementation or composition import: "+importPath) {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want service implementation failure", err)
			}
			if !strings.Contains(err.Error(), "support.BuildProcess with exact edges.Edges replacements") {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want shared typed-edge harness remediation", err)
			}
		})
	}
}

func TestCheckFunctionalCompositionTreeRejectsProviderLocalSharedSupport(t *testing.T) {
	root, _ := writeFunctionalSourceAt(
		t,
		"tests/functional/providers/internal/support/process.go",
		"package support\n",
	)
	err := checkFunctionalCompositionTree(root)
	if err == nil || !strings.Contains(err.Error(), "keep reusable process composition in tests/functional/internal/support") {
		t.Fatalf("checkFunctionalCompositionTree() error = %v, want canonical support-root failure", err)
	}
}

func TestCheckFunctionalCompositionTreeRejectsLegacyHarnessCall(t *testing.T) {
	for _, call := range []string{"NewServiceTestHarness", "GetEngineStateSnapshot"} {
		t.Run(call, func(t *testing.T) {
			root, _ := writeFunctionalSource(t, "package smoke\nfunc scenario() { "+call+"() }\n")
			err := checkFunctionalCompositionTree(root)
			if err == nil || !strings.Contains(err.Error(), "prohibited functional composition or configuration seam: "+call) {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want %s failure", err, call)
			}
		})
	}
}

func TestCheckFunctionalCompositionTreeRejectsSecondaryCompositionImport(t *testing.T) {
	for _, importPath := range []string{
		"github.com/portpowered/infinite-you/pkg/wire",
		"github.com/portpowered/infinite-you/pkg/wire/factorydefinitions",
		"github.com/portpowered/infinite-you/pkg/wire/runtimebundle",
		"github.com/portpowered/infinite-you/pkg/platform/runtimeinput",
		"github.com/portpowered/infinite-you/pkg/initializer/application",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
		"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold",
		"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl",
		"github.com/portpowered/infinite-you/pkg/services/recordings/projections",
		"github.com/portpowered/infinite-you/pkg/services/recordings/replay",
		"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection",
	} {
		t.Run(importPath, func(t *testing.T) {
			root, _ := writeFunctionalSource(t, "package smoke\nimport _ \""+importPath+"\"\n")
			err := checkFunctionalCompositionTree(root)
			if err == nil || !strings.Contains(err.Error(), "prohibited secondary composition import") {
				t.Fatalf("checkFunctionalCompositionTree() error = %v, want secondary composition failure", err)
			}
		})
	}
}

func TestCheckFunctionalCompositionTreeRejectsRuntimeConfigurationCallback(t *testing.T) {
	root, _ := writeFunctionalSource(t, `package smoke
type config struct {
  ConfigureRuntime func()
}
var fixture = config{ConfigureRuntime: func() {}}
`)
	err := checkFunctionalCompositionTree(root)
	if err == nil || !strings.Contains(err.Error(), "prohibited functional composition or configuration seam: ConfigureRuntime") {
		t.Fatalf("checkFunctionalCompositionTree() error = %v, want runtime configuration seam failure", err)
	}
}

func TestCheckAggregateProviderTestsAcceptsGrandfatheredFiles(t *testing.T) {
	root := t.TempDir()
	writeAggregateProviderTests(t, root, grandfatheredAggregateProviderTestFiles)

	if err := checkAggregateProviderTests(root); err != nil {
		t.Fatalf("checkAggregateProviderTests() error = %v", err)
	}
}

func TestCheckAggregateProviderTestsRejectsNewAggregateTest(t *testing.T) {
	root := t.TempDir()
	writeAggregateProviderTests(t, root, grandfatheredAggregateProviderTestFiles)
	writeFunctionalSourceAtRoot(t, root, providerTestRoot+"new_scenario_test.go", "package providers\n")

	err := checkAggregateProviderTests(root)
	if err == nil || !strings.Contains(err.Error(), "new aggregate provider test prohibited: tests/functional/providers/new_scenario_test.go") {
		t.Fatalf("checkAggregateProviderTests() error = %v, want new aggregate test failure", err)
	}
	if !strings.Contains(err.Error(), "dedicated provider or domain subpackage") {
		t.Fatalf("checkAggregateProviderTests() error = %v, want migration destination", err)
	}
}

func TestCheckAggregateProviderTestsRejectsStaleGrandfatheredEntry(t *testing.T) {
	root := t.TempDir()
	for name := range grandfatheredAggregateProviderTestFiles {
		if name != "helpers_test.go" {
			writeFunctionalSourceAtRoot(t, root, providerTestRoot+name, "package providers\n")
		}
	}

	err := checkAggregateProviderTests(root)
	if err == nil || !strings.Contains(err.Error(), "stale grandfathered aggregate provider test entry: tests/functional/providers/helpers_test.go") {
		t.Fatalf("checkAggregateProviderTests() error = %v, want stale grandfathered entry failure", err)
	}
	if !strings.Contains(err.Error(), "remove the migrated filename from grandfatheredAggregateProviderTestFiles") {
		t.Fatalf("checkAggregateProviderTests() error = %v, want allowlist-shrink remediation", err)
	}
}

func TestCheckAggregateProviderTestsRejectsReintroducedRemovedException(t *testing.T) {
	root := t.TempDir()
	shrunk := copyStringSet(grandfatheredAggregateProviderTestFiles)
	delete(shrunk, "helpers_test.go")
	writeAggregateProviderTests(t, root, shrunk)

	if err := checkAggregateProviderTestsAgainst(root, shrunk); err != nil {
		t.Fatalf("checkAggregateProviderTestsAgainst() after migration error = %v", err)
	}
	writeFunctionalSourceAtRoot(t, root, providerTestRoot+"helpers_test.go", "package providers\n")
	err := checkAggregateProviderTestsAgainst(root, shrunk)
	if err == nil || !strings.Contains(err.Error(), "new aggregate provider test prohibited: tests/functional/providers/helpers_test.go") {
		t.Fatalf("checkAggregateProviderTestsAgainst() error = %v, want reintroduced test failure", err)
	}
}

func TestCheckAggregateProviderTestsAcceptsDedicatedSubpackageTest(t *testing.T) {
	root := t.TempDir()
	writeAggregateProviderTests(t, root, grandfatheredAggregateProviderTestFiles)
	writeFunctionalSourceAtRoot(t, root, providerTestRoot+"codex/new_scenario_test.go", "package codex\n")

	if err := checkAggregateProviderTests(root); err != nil {
		t.Fatalf("checkAggregateProviderTests() error = %v", err)
	}
}

func writeAggregateProviderTests(t *testing.T, root string, names map[string]struct{}) {
	t.Helper()
	for name := range names {
		writeFunctionalSourceAtRoot(t, root, providerTestRoot+name, "package providers\n")
	}
}

func copyStringSet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for value := range source {
		result[value] = struct{}{}
	}
	return result
}

func writeFunctionalSource(t *testing.T, source string) (string, string) {
	t.Helper()
	return writeFunctionalSourceAt(t, "tests/functional/guards_batch/request_batch_test.go", source)
}

func writeProviderFunctionalSource(t *testing.T, source string) (string, string) {
	t.Helper()
	return writeFunctionalSourceAt(t, "tests/functional/providers/codex/process_test.go", source)
}

func writeFunctionalSourceAt(t *testing.T, relativePath, source string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := writeFunctionalSourceAtRoot(t, root, relativePath, source)
	return root, path
}

func writeFunctionalSourceAtRoot(t *testing.T, root, relativePath, source string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create functional fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write functional fixture: %v", err)
	}
	return path
}
