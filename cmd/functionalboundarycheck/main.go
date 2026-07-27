// functionalboundarycheck keeps customer-boundary request-batch scenarios from
// regressing into direct runtime assertions.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	defaultScenarioPath = "tests/functional/runtime_api/api_request_batch_boundary_smoke_test.go"
	diagnosticPrefix    = "[agent-factory:functional-boundary]"
	providerTestRoot    = "tests/functional/providers/"
	serviceImportPrefix = "github.com/portpowered/infinite-you/pkg/services/"
)

var forbiddenRequestBatchImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/",
	"github.com/portpowered/infinite-you/pkg/service",
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri",
}

var forbiddenCompositionImports = []string{
	"github.com/portpowered/infinite-you/pkg/initializer",
	"github.com/portpowered/infinite-you/pkg/platform/runtimeinput",
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation",
	"github.com/portpowered/infinite-you/pkg/services/recordings/projections",
	"github.com/portpowered/infinite-you/pkg/services/recordings/replay",
	"github.com/portpowered/infinite-you/pkg/services/workers/executor",
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection",
	"github.com/portpowered/infinite-you/pkg/wire",
}

var forbiddenProviderImplementationImports = []string{
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/claude",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/codex",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode",
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/pi",
}

// Provider scenarios may import service-root contracts and these exact public
// external-effect ports to populate edges.Edges. Every other service
// subpackage is an implementation or composition seam and must stay behind the
// root-built process harness. Keep this set aligned with the package-boundary
// policy's publicExternalEffectContractImports.
//
// Providers Execution leaf is the durable provider-effect owner; Workers
// inferencecontract remains migration debt until later Providers packets land.
var providerPublicEffectContractImports = map[string]struct{}{
	"github.com/portpowered/infinite-you/pkg/services/providers/execution/inferencecontract": {},
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty":                        {},
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract":    {},
	"github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic":         {},
	"github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic/linear":  {},
}

var forbiddenCompositionCalls = map[string]struct{}{
	"AssertReplaySucceeds":                  {},
	"BuildRuntimeScope":                     {},
	"GetEngineStateSnapshot":                {},
	"NewProviderErrorSmokeHarness":          {},
	"NewReplayHarness":                      {},
	"NewRuntimeBuilder":                     {},
	"NewRuntimeBuilderWithModelsFactory":    {},
	"NewRuntimeFactory":                     {},
	"NewServiceTestHarness":                 {},
	"SetCustomExecutor":                     {},
	"StartFunctionalServerWithConfig":       {},
	"WithRuntimeScheduler":                  {},
	"WithWorkerExecutor":                    {},
	"startFunctionalServerWithConfig":       {},
	"startReplayFunctionalServerWithConfig": {},
}

var forbiddenFunctionalConfigFields = map[string]struct{}{
	"Configure":        {},
	"ConfigureEdges":   {},
	"ConfigureRuntime": {},
}

// grandfatheredAggregateProviderTestFiles is deletion-only: existing aggregate
// tests may remain runnable while they migrate, but new tests belong in a
// dedicated provider or provider-domain package.
var grandfatheredAggregateProviderTestFiles = map[string]struct{}{
	"cli_script_executor_test.go":                        {},
	"cli_script_executor_timeout_long_test.go":           {},
	"cli_template_resolution_long_test.go":               {},
	"cli_timeout_cleanup_process_unix_test.go":           {},
	"cli_timeout_cleanup_process_windows_test.go":        {},
	"cli_timeout_cleanup_smoke_test.go":                  {},
	"cli_timeout_companion_smoke_long_test.go":           {},
	"helpers_long_test.go":                               {},
	"helpers_test.go":                                    {},
	"mock_workers_agent_test.go":            {},
	"mock_workers_end_to_end_smoke_test.go": {},
	"mock_workers_script_test.go":                        {},
	"mock_workers_service_runner_test.go":                {},
	"packaged_script_runtime_test.go":                    {},
	"runtime_logging_smoke_test.go":                      {},
}

type config struct {
	root string
	path string
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	if err := checkSource(cfg.root, cfg.path); err != nil {
		return err
	}
	if err := checkAggregateProviderTests(cfg.root); err != nil {
		return err
	}
	return checkFunctionalCompositionTree(cfg.root)
}

func checkAggregateProviderTests(root string) error {
	return checkAggregateProviderTestsAgainst(root, grandfatheredAggregateProviderTestFiles)
}

func checkAggregateProviderTestsAgainst(root string, grandfathered map[string]struct{}) error {
	providerRoot := filepath.Join(root, filepath.FromSlash(providerTestRoot))
	entries, err := os.ReadDir(providerRoot)
	if err != nil {
		return fmt.Errorf("%s read aggregate provider tests: %w", diagnosticPrefix, err)
	}
	active := make(map[string]struct{}, len(grandfathered))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if _, recorded := grandfathered[entry.Name()]; !recorded {
			return fmt.Errorf(
				"%s new aggregate provider test prohibited: %s%s; place the test in the dedicated provider or domain subpackage",
				diagnosticPrefix, providerTestRoot, entry.Name(),
			)
		}
		active[entry.Name()] = struct{}{}
	}
	stale := make([]string, 0)
	for name := range grandfathered {
		if _, exists := active[name]; !exists {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		slices.Sort(stale)
		return fmt.Errorf(
			"%s stale grandfathered aggregate provider test entry: %s%s; remove the migrated filename from grandfatheredAggregateProviderTestFiles so it cannot be reintroduced",
			diagnosticPrefix, providerTestRoot, stale[0],
		)
	}
	return nil
}

func checkFunctionalCompositionTree(root string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("%s resolve repository root: %w", diagnosticPrefix, err)
	}
	functionalRoot := filepath.Join(absRoot, "tests", "functional")
	return filepath.WalkDir(functionalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		providerSource := isDedicatedProviderSource(relative)
		if providerSource && isProviderSharedSupportSource(relative) {
			return fmt.Errorf(
				"%s prohibited provider-local shared support (%s); keep reusable process composition in tests/functional/internal/support",
				diagnosticPrefix, relative,
			)
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("%s parse %s: %w", diagnosticPrefix, relative, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("%s read import in %s: %w", diagnosticPrefix, relative, err)
			}
			if providerSource && importPath == "github.com/portpowered/infinite-you/pkg/root" {
				return fmt.Errorf(
					"%s prohibited provider functional composition import: %s (%s); use tests/functional/internal/support.BuildProcess with exact edges.Edges replacements",
					diagnosticPrefix, importPath, relative,
				)
			}
			if providerSource && matchesImportPrefix(importPath, forbiddenProviderImplementationImports) {
				return fmt.Errorf(
					"%s prohibited concrete provider implementation import: %s (%s); exercise provider behavior through tests/functional/internal/support.BuildProcess with exact edges.Edges replacements",
					diagnosticPrefix, importPath, relative,
				)
			}
			if providerSource && isProviderServiceImplementationImport(importPath) {
				return fmt.Errorf(
					"%s prohibited provider service implementation or composition import: %s (%s); exercise provider behavior through tests/functional/internal/support.BuildProcess with exact edges.Edges replacements",
					diagnosticPrefix, importPath, relative,
				)
			}
			for _, forbidden := range forbiddenCompositionImports {
				if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
					return fmt.Errorf(
						"%s prohibited secondary composition import: %s (%s); build the customer process through root.BuildProcess and override only edges.Edges",
						diagnosticPrefix, importPath, relative,
					)
				}
			}
		}
		file, err = parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return fmt.Errorf("%s parse %s: %w", diagnosticPrefix, relative, err)
		}
		var prohibited string
		ast.Inspect(file, func(node ast.Node) bool {
			if field, ok := node.(*ast.KeyValueExpr); ok {
				if name, ok := field.Key.(*ast.Ident); ok {
					if _, forbidden := forbiddenFunctionalConfigFields[name.Name]; forbidden {
						prohibited = name.Name
						return false
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calledName(call.Fun)
			if _, forbidden := forbiddenCompositionCalls[name]; forbidden {
				prohibited = name
				return false
			}
			return prohibited == ""
		})
		if prohibited != "" {
			return fmt.Errorf(
				"%s prohibited functional composition or configuration seam: %s (%s); use customer CLI arguments and override only edges.Edges",
				diagnosticPrefix, prohibited, relative,
			)
		}
		return nil
	})
}

func isProviderServiceImplementationImport(importPath string) bool {
	if !strings.HasPrefix(importPath, serviceImportPrefix) {
		return false
	}
	if _, publicEffectContract := providerPublicEffectContractImports[importPath]; publicEffectContract {
		return false
	}
	remainder := strings.TrimPrefix(importPath, serviceImportPrefix)
	return strings.Contains(remainder, "/")
}

func isDedicatedProviderSource(relative string) bool {
	if !strings.HasPrefix(relative, providerTestRoot) {
		return false
	}
	remainder := strings.TrimPrefix(relative, providerTestRoot)
	return strings.Contains(remainder, "/")
}

func isProviderSharedSupportSource(relative string) bool {
	remainder := strings.TrimPrefix(relative, providerTestRoot)
	return strings.HasPrefix(remainder, "support/") ||
		strings.HasPrefix(remainder, "internal/support/")
}

func matchesImportPrefix(importPath string, forbiddenImports []string) bool {
	for _, forbidden := range forbiddenImports {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	return false
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("functionalboundarycheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	path := flags.String("path", defaultScenarioPath, "request-batch functional scenario source path")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	return config{root: *root, path: *path}, nil
}

func checkSource(root, path string) error {
	relativePath, sourcePath, err := functionalTestPath(root, path)
	if err != nil {
		return err
	}
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("%s parse request-batch functional scenario %s: %w", diagnosticPrefix, relativePath, err)
	}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return fmt.Errorf("%s read import in %s: %w", diagnosticPrefix, filepath.ToSlash(path), err)
		}
		if isForbiddenRequestBatchImport(importPath) {
			return prohibitedInternalImportError(relativePath, importPath)
		}
	}
	return nil
}

func functionalTestPath(root, path string) (string, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve repository root: %w", diagnosticPrefix, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve functional scenario path: %w", diagnosticPrefix, err)
	}
	relativePath, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", "", fmt.Errorf("%s resolve functional scenario location: %w", diagnosticPrefix, err)
	}
	relativePath = filepath.ToSlash(relativePath)
	if relativePath == ".." || strings.HasPrefix(relativePath, "../") || !strings.HasPrefix(relativePath, "tests/functional/") || !strings.HasSuffix(relativePath, "_test.go") {
		return "", "", fmt.Errorf("%s request-batch boundary checks apply only to repository tests/functional/*_test.go sources: %s", diagnosticPrefix, relativePath)
	}
	return relativePath, absPath, nil
}

func isForbiddenRequestBatchImport(importPath string) bool {
	for _, forbidden := range forbiddenRequestBatchImports {
		if importPath == strings.TrimSuffix(forbidden, "/") || strings.HasPrefix(importPath, forbidden) {
			return true
		}
	}
	return false
}

func prohibitedInternalImportError(path, importPath string) error {
	return fmt.Errorf(
		"%s prohibited direct request-batch internal import: %s (%s); use generated REST/SSE customers or tests/functional/internal/support instead",
		diagnosticPrefix,
		importPath,
		filepath.ToSlash(path),
	)
}
