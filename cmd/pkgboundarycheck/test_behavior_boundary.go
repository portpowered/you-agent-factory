package main

import (
	"encoding/json"
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
	testBehaviorBaselinePath         = "test-behavior-boundary-baseline.json"
	testBehaviorBaselineStage        = "wire-injection-full-blow"
	testBehaviorPolicyKind           = "cross-owner-service-policy"
	testBehaviorCompositionKind      = "alternate-customer-composition"
	testBehaviorTransportProcessKind = "customer-process-under-transport"
	testBehaviorBaselineDeletionGate = "replace the test with an owner-local invariant, a strict public-root role, or root.BuildProcess customer coverage, then remove the exact entry"
)

// These exact tests exercise the generated command inventory or parity of the
// canonical command surface. They need the real process graph to prove that
// Wire exposes the same commands as the transport manifests. Customer runtime
// scenarios do not belong here and must live under tests/functional instead.
var reviewedTransportRootProcessTests = map[string]struct{}{
	"pkg/transports/cli/baseline/goal_failure_process_test.go":  {},
	"pkg/transports/cli/baseline/root_process_external_test.go": {},
	"pkg/transports/cli/baseline/root_process_test.go":          {},
	"pkg/transports/cli/clicontract/root_process_test.go":       {},
	"pkg/transports/cli/cliinputs/root_process_test.go":         {},
	"pkg/transports/cli/commandidentity/root_process_test.go":   {},
}

const (
	factoryDefinitionsImportPath = repositoryImportPrefix + "pkg/services/factory_definitions"
	factoryNamedPathsImportPath  = repositoryImportPrefix + "pkg/services/factory_definitions/namedpaths"
	operatorSettingsImportPath   = repositoryImportPrefix + "pkg/services/operator_settings"
	workersImportPath            = repositoryImportPrefix + "pkg/services/workers"
	factorySessionsImportPath    = repositoryImportPrefix + "pkg/services/factory_sessions"
	providerSessionsImportPath   = repositoryImportPrefix + "pkg/services/provider_sessions"
	providerSessionServicePath   = repositoryImportPrefix + "pkg/services/provider_sessions/service"
	providerSessionCursorPath    = repositoryImportPrefix + "pkg/services/provider_sessions/cursor"
	modelsImportPath             = repositoryImportPrefix + "pkg/services/models"
	workImportPath               = repositoryImportPrefix + "pkg/services/work"
	factoryRuntimeRootImportPath = repositoryImportPrefix + "pkg/services/factory_runtime"
	transportMappingImportPath   = repositoryImportPrefix + "pkg/transports/mapping"
	rootImportPath               = repositoryImportPrefix + "pkg/root"
	builtCLIHarnessImportPath    = repositoryImportPrefix + "internal/builtcliacceptance"
	cliHTTPImportPath            = repositoryImportPrefix + "pkg/transports/cli/clihttp"
	cliSubmitImportPath          = repositoryImportPrefix + "pkg/transports/cli/submit"
	transportImportRoot          = repositoryImportPrefix + "pkg/transports/"
	generatedHTTPImportPath      = repositoryImportPrefix + "pkg/transports/http/generated"
	generatedHTTPClientPath      = repositoryImportPrefix + "pkg/transports/http/client"
)

type testBehaviorOperation struct {
	kind       string
	owner      string
	importPath string
	symbol     string
}

var prohibitedCrossOwnerTestOperations = map[string]map[string]string{
	factoryDefinitionsImportPath: {
		"MapDir":              "factory_definitions",
		"NamedFactoriesRoot":  "factory_definitions",
		"ResolveCurrentDir":   "factory_definitions",
		"WriteCurrentPointer": "factory_definitions",
	},
	factoryNamedPathsImportPath: {
		"MapDir":              "factory_definitions",
		"NamedFactoriesRoot":  "factory_definitions",
		"ResolveCurrentDir":   "factory_definitions",
		"WriteCurrentPointer": "factory_definitions",
	},
	operatorSettingsImportPath: {
		"DefaultConfigPath":              "operator_settings",
		"ResolveFromHomeWithEnvironment": "operator_settings",
	},
	workersImportPath: {
		"LoadMockWorkersConfig": "workers",
	},
}

// These operations are migration debt specifically when a transport test calls
// them. Transport tests may author detached service-root values and program an
// exact public role, but must not execute owner policy or keep testing domain
// policy that was temporarily implemented beneath a transport mapping helper.
var prohibitedTransportTestPolicyOperations = map[string]map[string]string{
	factoryDefinitionsImportPath: {
		"InternalModelProviderFromPublicWorkerModelProvider": "factory_definitions",
		"PublicWorkerModelProviderFromInternal":              "factory_definitions",
	},
	factorySessionsImportPath: {
		"ApplySessionListScope":            "factory_sessions",
		"ProjectFactorySessionStopSummary": "factory_sessions",
		"ProjectWorkStopSummary":           "factory_sessions",
		"ValidateFactoryResponseEvent":     "factory_sessions",
	},
	providerSessionsImportPath: {
		"CanonicalProvider":             "provider_sessions",
		"newTestProviderSessionService": "provider_sessions",
		"scriptedProviderSessionDetail": "provider_sessions",
		"testProviderSessionService":    "provider_sessions",
	},
	providerSessionServicePath: {
		"New":         "provider_sessions",
		"NewForRoots": "provider_sessions",
	},
	providerSessionCursorPath: {
		"DefaultAgentStorageRoot":   "provider_sessions",
		"LoadDetails":               "provider_sessions",
		"NormalizeAgentStorageRoot": "provider_sessions",
	},
	factoryRuntimeRootImportPath: {
		"CategoryForState":        "factory_runtime",
		"CollectPublicWorkTokens": "factory_runtime",
		"SplitPlaceID":            "factory_runtime",
	},
	modelsImportPath: {
		"SupportedProviders": "models",
	},
	workImportPath: {
		"NewSelection":           "work",
		"NormalizeList":          "work",
		"PrepareInvocationInput": "work",
	},
	workersImportPath: {
		"CanonicalProviderSessionProvider": "workers",
	},
	transportMappingImportPath: {
		"BuildWorkflowSessionLiveResult":           "factory_runtime",
		"BuildWorkflowSessionResult":               "factory_runtime",
		"BuildWorkflowSessionResultUpdatedPayload": "factory_runtime",
	},
}

// HTTP transport tests prove protocol behavior against strict roles. Building
// engine-shaped values in them recreates projection and movement policy behind
// those roles, so they may traffic only in detached service-root results.
var prohibitedHTTPTransportTestEngineTypes = map[string]string{
	"EngineStateSnapshot":  "factory_runtime",
	"Net":                  "factory_runtime",
	"PetriMarkingSnapshot": "factory_runtime",
	"RuntimeToken":         "factory_runtime",
	"RuntimeTokenColor":    "factory_runtime",
}

// A functional scenario is a customer-scale test. It may use generated public
// clients after starting the root-built process, but it must not construct a
// handwritten transport operation as a substitute for Process.Execute. Keep
// this inventory exact so owner-local protocol tests remain permitted.
var prohibitedFunctionalTransportCompositionOperations = map[string]map[string]struct{}{
	cliHTTPImportPath: {
		"NewProtocol": {},
	},
	cliSubmitImportPath: {
		"NewSubmit": {},
	},
}

type testBehaviorFinding struct {
	Kind       string `json:"kind"`
	Owner      string `json:"owner,omitempty"`
	ImportPath string `json:"importPath"`
	Symbol     string `json:"symbol"`
	FilePath   string `json:"filePath"`
	Line       int    `json:"-"`
	Count      int    `json:"count"`
}

type testBehaviorBaseline struct {
	Version int                         `json:"version"`
	Entries []testBehaviorBaselineEntry `json:"entries"`
}

type testBehaviorBaselineEntry struct {
	Kind         string `json:"kind"`
	Owner        string `json:"owner,omitempty"`
	ImportPath   string `json:"importPath"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func scanTestBehaviorBoundaries(repoRoot string) ([]testBehaviorFinding, error) {
	findings := map[string]testBehaviorFinding{}
	err := filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				if path != repoRoot {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isTestOwnedGoFile(rel) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytesContainGeneratedMarker(content) {
			return nil
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			return fmt.Errorf("parse test behavior boundary file %s: %w", rel, err)
		}

		callerOwner, callerIsService := servicePackageOwner(rel)
		insideFunctionalTest := strings.HasPrefix(rel, "tests/functional/")
		insideHTTPTransportTest := strings.HasPrefix(rel, "pkg/transports/http/")
		insideTransportTest := strings.HasPrefix(rel, "pkg/transports/")
		importsByName := map[string]string{}
		dotImports := map[string]struct{}{}
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				continue
			}
			_, crossOwnerPolicyImport := prohibitedCrossOwnerTestOperations[importPath]
			_, transportPolicyImport := prohibitedTransportTestPolicyOperations[importPath]
			_, functionalTransportImport := prohibitedFunctionalTransportCompositionOperations[importPath]
			functionalHandwrittenTransportImport := insideFunctionalTest &&
				strings.HasPrefix(importPath, transportImportRoot) &&
				!isGeneratedFunctionalTransportPackage(importPath)
			if !crossOwnerPolicyImport && !transportPolicyImport &&
				!functionalTransportImport && !functionalHandwrittenTransportImport &&
				importPath != rootImportPath && importPath != builtCLIHarnessImportPath {
				continue
			}
			if spec.Name != nil && spec.Name.Name == "." {
				dotImports[importPath] = struct{}{}
				continue
			}
			if spec.Name != nil && spec.Name.Name == "_" {
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			} else if importPath == factoryDefinitionsImportPath {
				name = "factorydefinitions"
			} else if importPath == operatorSettingsImportPath {
				name = "operatorsettings"
			} else if importPath == transportMappingImportPath {
				name = "apisurface"
			} else if importPath == builtCLIHarnessImportPath {
				name = "builtcliacceptance"
			}
			importsByName[name] = importPath
		}

		record := func(operation testBehaviorOperation, position token.Pos) {
			key := testBehaviorKey(rel, operation.kind, operation.importPath, operation.symbol)
			finding := findings[key]
			if finding.Count == 0 {
				finding = testBehaviorFinding{
					Kind: operation.kind, Owner: operation.owner,
					ImportPath: operation.importPath, Symbol: operation.symbol,
					FilePath: rel, Line: fset.Position(position).Line,
				}
			}
			finding.Count++
			findings[key] = finding
		}

		operationFor := func(importPath, symbol string) (testBehaviorOperation, bool) {
			if symbols := prohibitedCrossOwnerTestOperations[importPath]; symbols != nil {
				owner, prohibited := symbols[symbol]
				if prohibited && !(callerIsService && callerOwner == owner) &&
					!strings.HasPrefix(rel, "pkg/wire/") {
					return testBehaviorOperation{testBehaviorPolicyKind, owner, importPath, symbol}, true
				}
			}
			insideServiceTest := strings.HasPrefix(rel, "pkg/services/")
			insidePkgTest := strings.HasPrefix(rel, "pkg/")
			insideTransportTest := strings.HasPrefix(rel, "pkg/transports/")
			if insideFunctionalTest {
				if isFunctionalTransportConstructor(importPath, symbol) {
					return testBehaviorOperation{testBehaviorCompositionKind, "", importPath, symbol}, true
				}
				if symbols := prohibitedFunctionalTransportCompositionOperations[importPath]; symbols != nil {
					if _, prohibited := symbols[symbol]; prohibited {
						return testBehaviorOperation{testBehaviorCompositionKind, "", importPath, symbol}, true
					}
				}
			}
			if insideTransportTest {
				if symbols := prohibitedTransportTestPolicyOperations[importPath]; symbols != nil {
					if owner, prohibited := symbols[symbol]; prohibited {
						return testBehaviorOperation{testBehaviorPolicyKind, owner, importPath, symbol}, true
					}
				}
			}
			switch {
			case importPath == rootImportPath && symbol == "BuildProcess" && insideServiceTest:
				return testBehaviorOperation{testBehaviorCompositionKind, "", importPath, symbol}, true
			case importPath == rootImportPath && symbol == "BuildProcess" && insideTransportTest:
				if _, reviewed := reviewedTransportRootProcessTests[rel]; !reviewed {
					return testBehaviorOperation{testBehaviorTransportProcessKind, "", importPath, symbol}, true
				}
			case importPath == builtCLIHarnessImportPath && symbol == "NewHarness" && insidePkgTest:
				return testBehaviorOperation{testBehaviorCompositionKind, "", importPath, symbol}, true
			}
			return testBehaviorOperation{}, false
		}

		if strings.HasPrefix(rel, "pkg/transports/") {
			for _, declaration := range parsed.Decls {
				switch typed := declaration.(type) {
				case *ast.FuncDecl:
					if owner, prohibited := prohibitedTransportTestPolicyOperations[providerSessionsImportPath][typed.Name.Name]; prohibited {
						record(testBehaviorOperation{testBehaviorPolicyKind, owner, providerSessionsImportPath, typed.Name.Name}, typed.Name.Pos())
					}
				case *ast.GenDecl:
					if typed.Tok != token.TYPE {
						continue
					}
					for _, rawSpec := range typed.Specs {
						spec, ok := rawSpec.(*ast.TypeSpec)
						if !ok {
							continue
						}
						if owner, prohibited := prohibitedTransportTestPolicyOperations[providerSessionsImportPath][spec.Name.Name]; prohibited {
							record(testBehaviorOperation{testBehaviorPolicyKind, owner, providerSessionsImportPath, spec.Name.Name}, spec.Name.Pos())
						}
					}
				}
			}
		}

		ast.Inspect(parsed, func(node ast.Node) bool {
			if insideTransportTest {
				function, ok := node.(*ast.FuncDecl)
				if ok && function.Name.Name == "PrepareInvocationInput" &&
					!isStrictTestCallbackDelegation(function) {
					record(testBehaviorOperation{
						testBehaviorPolicyKind,
						"work",
						workImportPath,
						"PrepareInvocationInput",
					}, function.Name.Pos())
				}
			}
			if insideHTTPTransportTest {
				literal, ok := node.(*ast.CompositeLit)
				if ok {
					selector, selected := literal.Type.(*ast.SelectorExpr)
					if selected {
						identifier, identified := selector.X.(*ast.Ident)
						if identified && importsByName[identifier.Name] == factoryRuntimeRootImportPath {
							if owner, prohibited := prohibitedHTTPTransportTestEngineTypes[selector.Sel.Name]; prohibited {
								record(testBehaviorOperation{testBehaviorPolicyKind, owner, factoryRuntimeRootImportPath, selector.Sel.Name}, selector.Sel.Pos())
							}
						}
					}
				}
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch called := call.Fun.(type) {
			case *ast.SelectorExpr:
				identifier, ok := called.X.(*ast.Ident)
				if !ok {
					return true
				}
				if importPath, imported := importsByName[identifier.Name]; imported {
					if operation, prohibited := operationFor(importPath, called.Sel.Name); prohibited {
						record(operation, called.Sel.Pos())
					}
				}
			case *ast.Ident:
				for importPath := range dotImports {
					if operation, prohibited := operationFor(importPath, called.Name); prohibited {
						record(operation, called.Pos())
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan test behavior boundaries: %w", err)
	}
	out := make([]testBehaviorFinding, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding)
	}
	slices.SortFunc(out, func(left, right testBehaviorFinding) int {
		if order := strings.Compare(left.FilePath, right.FilePath); order != 0 {
			return order
		}
		if order := strings.Compare(left.Kind, right.Kind); order != 0 {
			return order
		}
		if order := strings.Compare(left.ImportPath, right.ImportPath); order != 0 {
			return order
		}
		return strings.Compare(left.Symbol, right.Symbol)
	})
	return out, nil
}

// A transport fake may expose the Work role only by forwarding its request to
// an injected callback. Any branching, parsing, normalization, or authored
// response in the method would reproduce Work-owned invocation-input policy.
func isStrictTestCallbackDelegation(function *ast.FuncDecl) bool {
	if function.Recv == nil || function.Body == nil || len(function.Body.List) != 1 {
		return false
	}
	if len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return false
	}
	receiverName := function.Recv.List[0].Names[0].Name
	returned, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	callback, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiverField, ok := callback.X.(*ast.Ident)
	if !ok || receiverField.Name != receiverName {
		return false
	}
	for _, argument := range call.Args {
		if _, direct := argument.(*ast.Ident); !direct {
			return false
		}
	}
	return true
}

func loadTestBehaviorBaseline(repoRoot string) (testBehaviorBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, testBehaviorBaselinePath))
	if err != nil {
		if os.IsNotExist(err) {
			return testBehaviorBaseline{}, nil
		}
		return testBehaviorBaseline{}, fmt.Errorf("read test behavior baseline: %w", err)
	}
	var baseline testBehaviorBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return testBehaviorBaseline{}, fmt.Errorf("decode test behavior baseline: %w", err)
	}
	if baseline.Version != 1 {
		return testBehaviorBaseline{}, fmt.Errorf("test behavior baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(testBehaviorBaselinePath, len(baseline.Entries)); err != nil {
		return testBehaviorBaseline{}, err
	}
	return baseline, nil
}

func partitionTestBehaviorFindings(findings []testBehaviorFinding, baseline testBehaviorBaseline) ([]testBehaviorFinding, []testBehaviorBaselineEntry, error) {
	baselineByKey := make(map[string]testBehaviorBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateTestBehaviorBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := testBehaviorKey(entry.FilePath, entry.Kind, entry.ImportPath, entry.Symbol)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate test behavior baseline entry: %s -> %s.%s", entry.FilePath, entry.ImportPath, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []testBehaviorFinding
	seen := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		key := testBehaviorKey(finding.FilePath, finding.Kind, finding.ImportPath, finding.Symbol)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.Count || entry.Owner != finding.Owner {
			blocking = append(blocking, finding)
		}
	}
	var stale []testBehaviorBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right testBehaviorBaselineEntry) int {
		return strings.Compare(testBehaviorKey(left.FilePath, left.Kind, left.ImportPath, left.Symbol), testBehaviorKey(right.FilePath, right.Kind, right.ImportPath, right.Symbol))
	})
	return blocking, stale, nil
}

func validateTestBehaviorBaselineEntry(entry testBehaviorBaselineEntry) error {
	if entry.Kind == "" || entry.ImportPath == "" || entry.Symbol == "" || entry.FilePath == "" ||
		entry.Count < 1 || entry.Stage != testBehaviorBaselineStage || entry.DeletionGate != testBehaviorBaselineDeletionGate {
		return fmt.Errorf("test behavior baseline entry is incomplete or has an invalid stage/deletion gate: %#v", entry)
	}
	for _, value := range []string{entry.Kind, entry.Owner, entry.ImportPath, entry.Symbol, entry.FilePath} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("test behavior baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	switch entry.Kind {
	case testBehaviorPolicyKind:
		wantOwner, known := testBehaviorPolicyOwner(entry.ImportPath, entry.Symbol)
		if !known || entry.Owner != wantOwner {
			return fmt.Errorf("test behavior baseline entry names unknown policy operation or owner: %#v", entry)
		}
		if isTransportOnlyTestPolicy(entry.ImportPath, entry.Symbol) &&
			!strings.HasPrefix(entry.FilePath, "pkg/transports/") {
			return fmt.Errorf("test behavior baseline entry names transport-only policy outside pkg/transports: %#v", entry)
		}
	case testBehaviorCompositionKind:
		if entry.Owner != "" || !isKnownCompositionOperation(entry.ImportPath, entry.Symbol) {
			return fmt.Errorf("test behavior baseline entry names unknown composition operation: %#v", entry)
		}
		if isFunctionalTransportCompositionOperation(entry.ImportPath, entry.Symbol) &&
			!strings.HasPrefix(entry.FilePath, "tests/functional/") {
			return fmt.Errorf("test behavior baseline entry names functional transport composition outside tests/functional: %#v", entry)
		}
	case testBehaviorTransportProcessKind:
		if entry.Owner != "" || entry.ImportPath != rootImportPath || entry.Symbol != "BuildProcess" ||
			!strings.HasPrefix(entry.FilePath, "pkg/transports/") {
			return fmt.Errorf("test behavior baseline entry names unknown transport customer-process operation: %#v", entry)
		}
	default:
		return fmt.Errorf("test behavior baseline entry kind = %q is not recognized", entry.Kind)
	}
	return nil
}

func testBehaviorPolicyOwner(importPath, symbol string) (string, bool) {
	if importPath == factoryRuntimeRootImportPath {
		if owner, known := prohibitedHTTPTransportTestEngineTypes[symbol]; known {
			return owner, true
		}
	}
	if owner, known := prohibitedCrossOwnerTestOperations[importPath][symbol]; known {
		return owner, true
	}
	owner, known := prohibitedTransportTestPolicyOperations[importPath][symbol]
	return owner, known
}

func isTransportOnlyTestPolicy(importPath, symbol string) bool {
	_, known := prohibitedTransportTestPolicyOperations[importPath][symbol]
	return known
}

func isKnownCompositionOperation(importPath, symbol string) bool {
	if (importPath == rootImportPath && symbol == "BuildProcess") ||
		(importPath == builtCLIHarnessImportPath && symbol == "NewHarness") {
		return true
	}
	return isFunctionalTransportCompositionOperation(importPath, symbol)
}

func isFunctionalTransportCompositionOperation(importPath, symbol string) bool {
	_, known := prohibitedFunctionalTransportCompositionOperations[importPath][symbol]
	return known || isFunctionalTransportConstructor(importPath, symbol)
}

func isFunctionalTransportConstructor(importPath, symbol string) bool {
	if !strings.HasPrefix(importPath, transportImportRoot) || isGeneratedFunctionalTransportPackage(importPath) {
		return false
	}
	return strings.HasPrefix(symbol, "New") || strings.HasPrefix(symbol, "Build") || strings.HasPrefix(symbol, "Create")
}

func isGeneratedFunctionalTransportPackage(importPath string) bool {
	return importPath == generatedHTTPImportPath || importPath == generatedHTTPClientPath
}

func createTestBehaviorBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	path := filepath.Join(repoRoot, testBehaviorBaselinePath)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing test behavior baseline: %s", testBehaviorBaselinePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat test behavior baseline: %w", err)
	}
	findings, err := scanTestBehaviorBoundaries(repoRoot)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		return fmt.Errorf("refusing to create empty test behavior baseline: no migration debt exists")
	}
	baseline := testBehaviorBaseline{Version: 1, Entries: make([]testBehaviorBaselineEntry, 0, len(findings))}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, testBehaviorBaselineEntry{
			Kind: finding.Kind, Owner: finding.Owner, ImportPath: finding.ImportPath,
			Symbol: finding.Symbol, FilePath: finding.FilePath, Count: finding.Count,
			Stage: testBehaviorBaselineStage, DeletionGate: testBehaviorBaselineDeletionGate,
		})
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create test behavior baseline: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode test behavior baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close test behavior baseline: %w", err)
	}
	fmt.Fprintf(stdoutWriter, "[agent-factory:pkg-boundary] created %s with %d exact deletion-only edge(s)\n", testBehaviorBaselinePath, len(baseline.Entries))
	return nil
}

func testBehaviorKey(filePath, kind, importPath, symbol string) string {
	return filepath.ToSlash(filePath) + "\x00" + kind + "\x00" + importPath + "\x00" + symbol
}

func writeTestBehaviorFindings(writer io.Writer, findings []testBehaviorFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited test boundary behavior: %s %s.%s (%s:%d, %d call(s))\n", finding.Kind, finding.ImportPath, finding.Symbol, finding.FilePath, finding.Line, finding.Count)
		if finding.Owner != "" {
			fmt.Fprintf(writer, "  service owner: pkg/services/%s\n", finding.Owner)
		}
		fmt.Fprintln(writer, "  remediation: use a strict public-root role for transport-only assertions, move owner policy invariants to the owner, or exercise customer behavior through root.BuildProcess without a child process harness.")
	}
}

func writeStaleTestBehaviorBaselineEntries(writer io.Writer, entries []testBehaviorBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] stale test behavior baseline entry: %s -> %s.%s\n", entry.FilePath, entry.ImportPath, entry.Symbol)
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", testBehaviorBaselinePath)
	}
}

func writeTestBehaviorBaselineSummary(writer io.Writer, count int) {
	if count == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active test behavior migration baseline: %d exact file/symbol edge(s)\n", count)
	fmt.Fprintln(writer, "  deletion gate: preserve the named scenario at its correct owner or customer boundary, then delete its exact baseline entry.")
}
