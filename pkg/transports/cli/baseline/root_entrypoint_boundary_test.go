package baseline_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const cliImportPath = "github.com/portpowered/infinite-you/pkg/transports/cli"
const httpTransportImportPath = "github.com/portpowered/infinite-you/pkg/transports/http"

func TestCLIInventoryPackagesUseCanonicalProcessRoot(t *testing.T) {
	t.Parallel()

	for _, packagePath := range []string{
		"pkg/transports/cli/baseline",
		"pkg/transports/cli/clicontract",
		"pkg/transports/cli/cliinputs",
		"pkg/transports/cli/commandidentity",
	} {
		assertNoPartialRootConstruction(t, testutil.MustRepoPath(t, packagePath))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestProductionCLIConstructionIsReachableOnlyThroughWire(t *testing.T) {
	t.Parallel()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, "pkg/wire/") {
			return nil
		}

		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err == nil && importPath == cliImportPath {
				position := files.Position(imported.Pos())
				t.Errorf(
					"%s:%d imports the CLI composition package outside Wire; build the command through root.BuildProcess",
					position.Filename,
					position.Line,
				)
			}
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "NewRootCommand" || function.Name.Name == "NewRootCommandWithFactory" {
				position := files.Position(function.Pos())
				t.Errorf(
					"%s:%d declares alternate CLI construction entrypoint %s; Wire and root.BuildProcess own the graph",
					position.Filename,
					position.Line,
					function.Name.Name,
				)
			}
		}
		if strings.HasPrefix(relative, "pkg/transports/cli/") {
			ast.Inspect(parsed, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "NewCommand" {
					return true
				}
				position := files.Position(selector.Pos())
				t.Errorf(
					"%s:%d constructs a command tree inside the CLI transport; root.BuildProcess must invoke the injected CommandFactory",
					position.Filename,
					position.Line,
				)
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production CLI construction boundary: %v", err)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestTestsOutsideHTTPAndWireCannotConstructApplicationServers(t *testing.T) {
	t.Parallel()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(relative, "pkg/transports/http/") || strings.HasPrefix(relative, "pkg/wire/") {
			return nil
		}

		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		aliases := map[string]struct{}{}
		dotImported := false
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != httpTransportImportPath {
				continue
			}
			alias := "http"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				dotImported = true
			} else {
				aliases[alias] = struct{}{}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			prohibited := false
			switch function := call.Fun.(type) {
			case *ast.SelectorExpr:
				qualifier, ok := function.X.(*ast.Ident)
				_, imported := aliases[qualifierName(qualifier, ok)]
				prohibited = imported && function.Sel.Name == "NewServer"
			case *ast.Ident:
				prohibited = dotImported && function.Name == "NewServer"
			}
			if prohibited {
				position := files.Position(call.Pos())
				t.Errorf(
					"%s:%d constructs an application HTTP server outside its owner; customer-scale tests must use root.BuildProcess and transport-only tests must use strict public protocol fakes",
					position.Filename,
					position.Line,
				)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan test HTTP construction boundary: %v", err)
	}
}

func TestTestsOutsideCLICommandOwnerCannotConstructGlobalCommandTrees(t *testing.T) {
	t.Parallel()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			default:
				return nil
			}
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		directory := filepath.ToSlash(filepath.Dir(relative))
		if directory == "pkg/transports/cli" || strings.HasPrefix(relative, "pkg/wire/") {
			return nil
		}

		files := token.NewFileSet()
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil || importPath != cliImportPath {
				continue
			}
			position := files.Position(imported.Pos())
			t.Errorf(
				"%s:%d imports the global CLI composition package outside its owner; customer-scale tests must use root.BuildProcess and transport-only subpackages must construct only their focused command",
				position.Filename,
				position.Line,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test CLI construction boundary: %v", err)
	}
}

func TestGoalFailureCustomerBaselinesUseCanonicalProcess(t *testing.T) {
	t.Parallel()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	processPath := filepath.Join(repositoryRoot, "pkg", "transports", "cli", "baseline", "goal_failure_process_test.go")
	files := token.NewFileSet()
	processFile, err := parser.ParseFile(files, processPath, nil, 0)
	if err != nil {
		t.Fatalf("parse customer goal-failure process tests: %v", err)
	}
	if processFile.Name.Name != "baseline_test" {
		t.Fatalf("customer goal-failure process package = %q, want external baseline_test", processFile.Name.Name)
	}
	hasRootImport := false
	for _, imported := range processFile.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err == nil && importPath == "github.com/portpowered/infinite-you/pkg/root" {
			hasRootImport = true
		}
	}
	if !hasRootImport {
		t.Fatal("customer goal-failure process tests must import pkg/root")
	}
	hasBuildProcessCall := false
	ast.Inspect(processFile, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "BuildProcess" {
			hasBuildProcessCall = true
		}
		return true
	})
	if !hasBuildProcessCall {
		t.Fatal("customer goal-failure process tests must call root.BuildProcess")
	}
	assertNoGoalFailureAlternateComposition(t, files, processFile, nil)

	ownerPath := filepath.Join(repositoryRoot, "pkg", "transports", "cli", "goal_failure_baseline_test.go")
	ownerFiles := token.NewFileSet()
	ownerFile, err := parser.ParseFile(ownerFiles, ownerPath, nil, 0)
	if err != nil {
		t.Fatalf("parse owner-local goal-failure tests: %v", err)
	}
	ownerLocal := map[string]struct{}{
		"TestFailureBaseline_QuietLeak_RunBatchQuietSuppressesTerminalOnOperationalFailure":         {},
		"TestFailureBaseline_QuietLeak_RunFactoryQuietSuppressesTerminalOnInvocationFailure":        {},
		"TestFailureBaseline_NoServer_ModelsInvokeCommandUsesBootstrapInsteadOfUnreachableEndpoint": {},
		"TestFailureBaseline_AbsentDefault_RunNamedGoalLeavesOperatorDefaultsEmptyWithoutConfig":    {},
		"TestRunNamedGoalResolutionDefersCorruptedFactoryValidationToRuntime":                       {},
		"TestRunNamedGoalResolutionDefersInvalidInstalledTargetValidationToRuntime":                 {},
		"TestFailureBaseline_QuietLeak_RunBatchQuietSuppressesStartupChatter":                       {},
		"TestFailureBaseline_QuietLeak_RunFactoryQuietPromptKeepsStartupOutputSuppressed":           {},
		"TestFailureBaseline_QuietLeak_RunNamedGoalQuietBatchSuppressesOperatorChatter":             {},
		"TestFailureBaseline_NamedPath_RunNamedGoalSurfacesPercentEncodedFactoryDir":                {},
	}
	assertNoGoalFailureAlternateComposition(t, ownerFiles, ownerFile, ownerLocal)
}

func assertNoGoalFailureAlternateComposition(
	t *testing.T,
	files *token.FileSet,
	parsed *ast.File,
	ownerLocal map[string]struct{},
) {
	t.Helper()
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil || !strings.HasPrefix(function.Name.Name, "Test") {
			continue
		}
		usesAlternate := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch called := call.Fun.(type) {
			case *ast.Ident:
				usesAlternate = called.Name == "newLegacyTestRootCommand" ||
					called.Name == "newComposedTestRootCommand" ||
					called.Name == "newGoalFailureNamedRunEnvironment"
			case *ast.SelectorExpr:
				usesAlternate = called.Sel.Name == "NewCommand"
			}
			return !usesAlternate
		})
		if !usesAlternate {
			continue
		}
		if _, allowed := ownerLocal[function.Name.Name]; allowed {
			continue
		}
		position := files.Position(function.Pos())
		t.Errorf(
			"%s:%d customer-scale goal-failure test %s constructs an alternate command tree; use root.BuildProcess + Process.Execute or classify an exact owner-local transport invariant",
			position.Filename,
			position.Line,
			function.Name.Name,
		)
	}
}

func qualifierName(identifier *ast.Ident, ok bool) string {
	if !ok || identifier == nil {
		return ""
	}
	return identifier.Name
}

func assertNoPartialRootConstruction(t *testing.T, packageDir string) {
	t.Helper()

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read inventory package %s: %v", packageDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		assertFileHasNoPartialRootConstruction(t, filepath.Join(packageDir, entry.Name()))
	}
}

func assertFileHasNoPartialRootConstruction(t *testing.T, path string) {
	t.Helper()

	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse inventory source %s: %v", path, err)
	}
	cliAliases := make(map[string]struct{})
	for _, imported := range parsed.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil || importPath != cliImportPath {
			continue
		}
		alias := "cli"
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		cliAliases[alias] = struct{}{}
	}
	parsed, err = parser.ParseFile(files, path, nil, 0)
	if err != nil {
		t.Fatalf("parse inventory source %s: %v", path, err)
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if declaration, ok := node.(*ast.FuncDecl); ok && declaration.Name.Name == "ProductionRootCommand" {
			position := files.Position(declaration.Pos())
			t.Errorf(
				"%s:%d declares an alternate ProductionRootCommand; inventory tests must share their package-local root.BuildProcess helper",
				position.Filename,
				position.Line,
			)
		}
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "NewRootCommand" {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, imported := cliAliases[qualifier.Name]; imported {
			position := files.Position(selector.Pos())
			t.Errorf(
				"%s:%d calls cli.NewRootCommand; inventory tests must construct the production command through root.BuildProcess",
				position.Filename,
				position.Line,
			)
		}
		return true
	})
}
