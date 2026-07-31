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

const transportBehaviorBaselinePath = "docs/internal/baselines/transport-behavior-baseline.json"
const transportBehaviorBaselineStage = "wire-injection-full-blow"
const transportBehaviorDeletionGate = "move the behavior to its owning service, Initializer lifecycle, or an injected external-effect edge and delete this exact entry"

type transportBehaviorFinding struct {
	kind     string
	symbol   string
	filePath string
	line     int
	count    int
}

type transportBehaviorBaseline struct {
	Version int                              `json:"version"`
	Entries []transportBehaviorBaselineEntry `json:"entries"`
}

type transportBehaviorBaselineEntry struct {
	Kind         string `json:"kind"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Count        int    `json:"count"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

var transportProcessAndFilesystemSymbols = map[string]struct{}{
	"fmt.Print": {}, "fmt.Printf": {}, "fmt.Println": {},
	"net.Listen": {}, "net.ListenPacket": {},
	"net/http.Client": {}, "net/http.DefaultClient": {}, "net/http.DefaultTransport": {},
	"os.Args": {}, "os.Chdir": {}, "os.Create": {}, "os.CreateTemp": {}, "os.DirFS": {}, "os.File": {},
	"os.Environ": {}, "os.Exit": {}, "os.Getenv": {}, "os.Getwd": {}, "os.LookupEnv": {},
	"os.Mkdir": {}, "os.MkdirAll": {},
	"os.Open": {}, "os.OpenFile": {}, "os.ReadDir": {}, "os.ReadFile": {}, "os.Remove": {},
	"os.RemoveAll": {}, "os.Rename": {}, "os.Stat": {}, "os.Stderr": {}, "os.Stdin": {},
	"os.Stdout": {}, "os.TempDir": {}, "os.UserCacheDir": {}, "os.UserConfigDir": {}, "os.UserHomeDir": {},
	"os.WriteFile":    {},
	"os/exec.Command": {}, "os/exec.CommandContext": {},
	"time.Now": {}, "time.Since": {}, "time.Until": {},
	"github.com/google/uuid.New": {}, "github.com/google/uuid.NewString": {},
}

var transportLifecycleSymbols = map[string]struct{}{
	"context.Background": {},
	"context.WithCancel": {}, "context.WithCancelCause": {}, "context.WithDeadline": {},
	"context.WithDeadlineCause": {}, "context.WithTimeout": {}, "context.WithTimeoutCause": {},
	"net/http.Server":  {},
	"os/signal.Ignore": {}, "os/signal.Notify": {}, "os/signal.NotifyContext": {},
	"os/signal.Reset": {}, "os/signal.Stop": {},
	"time.After": {}, "time.AfterFunc": {}, "time.NewTicker": {}, "time.NewTimer": {},
	"time.Sleep": {}, "time.Tick": {},
}

var transportSynchronizationTypes = map[string]struct{}{
	"sync.Cond": {}, "sync.Map": {}, "sync.Mutex": {}, "sync.Once": {}, "sync.Pool": {},
	"sync.RWMutex": {}, "sync.WaitGroup": {},
}

var transportDomainPolicyPrefixes = []string{
	"Materialize", "Orchestrate", "Project", "Replay", "Schedule", "Validate",
}

var transportFactoryDefinitionValidationPolicySymbols = map[string]struct{}{
	"ExpectedWorkerBehaviorClassForWorkstation":    {},
	"CompatibleWorkerWorkstationBehavior":          {},
	"WorkerWorkstationBehaviorMismatchMessage":     {},
	"workerWorkstationCompatibilityTargetsFromAPI": {},
}

var transportFactoryDefinitionValidationPhaseMethods = map[string]struct{}{
	"ValidateBlockingLoad":                   {},
	"WorkerWorkstationBehaviorCompatibility": {},
	"WorkTypeHandlingBehavior":               {},
}

var retiredTransportServiceEntrypoints = map[string]struct{}{
	"BuildWorkflowSessionLiveResult":           {},
	"BuildWorkflowSessionResult":               {},
	"BuildWorkflowSessionResultUpdatedPayload": {},
	"LoadFactoryConfigFromConfigFile":          {},
	"ResolveFactoryRootFromConfigFile":         {},
	"ValidateFactoryForPromptRun":              {},
}

var transportFactorySessionNormalizationSymbols = map[string]struct{}{
	"NormalizeStartRequest":             {},
	"NormalizeControlRequest":           {},
	"NormalizeApproveRequest":           {},
	"NormalizeRetryDispatchRequest":     {},
	"NormalizeInterruptDispatchRequest": {},
	"NormalizeListSessionsRequest":      {},
	"NormalizeResultRequest":            {},
	"NormalizeEventReconnectRequest":    {},
	"NewExecutionValidationError":       {},
}

var transportFactorySessionPreparationMethods = map[string]struct{}{
	"PrepareStart":             {},
	"PrepareControl":           {},
	"PrepareApprove":           {},
	"PrepareRetryDispatch":     {},
	"PrepareInterruptDispatch": {},
	"PrepareListSessions":      {},
	"PrepareResult":            {},
	"PrepareEventReconnect":    {},
}

const factoryDefinitionsRootImport = repositoryImportPrefix + "pkg/services/factory_definitions"
const modelsRootImport = repositoryImportPrefix + "pkg/services/models"
const workersRootImport = repositoryImportPrefix + "pkg/services/workers"
const workRootImport = repositoryImportPrefix + "pkg/services/work"

var transportWorkPreparationSymbols = map[string]struct{}{
	"NormalizeList":                            {},
	"ParseCanonicalWorkRequestJSON":            {},
	"ValidateCanonicalWorkRequestJSON":         {},
	"NewListRequestPreparation":                {},
	"NewFactoryRequestBatchPreparation":        {},
	"ResolveTextInput":                         {},
	"ResolveAPITextInputContent":               {},
	"NormalizeArguments":                       {},
	"NamedArgumentInputsFromAnyMap":            {},
	"NewInvocationInputPreparation":            {},
	"NewContentPreparation":                    {},
	"ResolveWorkRequestCurrentChainingTraceID": {},
}

var httpFactoryStatusProjectionSymbols = map[string]struct{}{
	"IsSystemTimeToken":             {},
	"SplitPlaceID":                  {},
	"StateCategoryForPlace":         {},
	"categorizeStatusTokens":        {},
	"countPublicStatusTokens":       {},
	"resourceTotalsFromTopology":    {},
	"statusFromEngineStateSnapshot": {},
	"statusStateCategory":           {},
}

var transportWorkersConfigLoadingSymbols = map[string]struct{}{
	"LoadMockWorkersConfig":      {},
	"NewMockWorkersConfigLoader": {},
	"NewEmptyMockWorkersConfig":  {},
	"ParseMockWorkersConfig":     {},
}

var transportWorkersFailurePolicySymbols = map[string]struct{}{
	"CanonicalProviderSessionProvider":       {},
	"NewProviderError":                       {},
	"NormalizeProviderExecutionError":        {},
	"SafeWorkDiagnosticsFromWorkDiagnostics": {},
	"WorkDiagnostics":                        {},
}

var transportModelsReadinessPolicySymbols = map[string]struct{}{
	"ErrFailed":                 {},
	"ErrLoading":                {},
	"ErrMissing":                {},
	"ErrUnsupported":            {},
	"InvocationError":           {},
	"InvocationErrorForRuntime": {},
	"IsInvocationBlocked":       {},
	"IsMissing":                 {},
	"ReadinessStateFromError":   {},
}

var transportPlatformSelectionPrefixes = []string{
	"Build", "Create", "Default", "Ensure", "New", "Open", "Provide", "Resolve",
}

const factorySessionMCPTransportPrefix = "pkg/transports/mcp/factorysession/"

// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func scanTransportBehavior(repoRoot string) ([]transportBehaviorFinding, error) {
	root := filepath.Join(repoRoot, "pkg", "transports")
	findingsByKey := map[string]transportBehaviorFinding{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		testFile := strings.HasSuffix(entry.Name(), "_test.go")
		filePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		filePath = filepath.ToSlash(filePath)
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
			return err
		}
		imports := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || spec.Name != nil && spec.Name.Name == "_" {
				continue
			}
			if spec.Name != nil && spec.Name.Name == "." {
				if isTransportBehaviorControlledImport(importPath) {
					finding := transportBehaviorFinding{
						kind: "opaque-import", symbol: importPath, filePath: filePath,
						line: fset.Position(spec.Pos()).Line, count: 1,
					}
					findingsByKey[transportBehaviorKey(filePath, finding.kind, finding.symbol)] = finding
				}
				continue
			}
			name := filepath.Base(importPath)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imports[name] = importPath
		}
		record := func(kind, symbol string, position token.Pos) {
			key := transportBehaviorKey(filePath, kind, symbol)
			finding := findingsByKey[key]
			if finding.count == 0 {
				finding = transportBehaviorFinding{kind: kind, symbol: symbol, filePath: filePath, line: fset.Position(position).Line}
			}
			finding.count++
			findingsByKey[key] = finding
		}
		if testFile {
			ast.Inspect(parsed, func(node ast.Node) bool {
				selected, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if strings.HasPrefix(filePath, "pkg/transports/mapping/") {
					if _, prohibited := transportFactorySessionPreparationMethods[selected.Sel.Name]; prohibited {
						record("factory-session-preparation", selected.Sel.Name, selected.Sel.Pos())
					}
				}
				if selected.Sel.Name == "NormalizeWorkRequest" {
					record("work-request-normalization", selected.Sel.Name, selected.Sel.Pos())
				}
				identifier, ok := selected.X.(*ast.Ident)
				if !ok {
					return true
				}
				if imports[identifier.Name] == factoryDefinitionsRootImport && selected.Sel.Name == "MapDir" {
					record("named-factory-path-policy", factoryDefinitionsRootImport+".MapDir", selected.Sel.Pos())
				}
				if imports[identifier.Name] == workersRootImport {
					if _, prohibited := transportWorkersConfigLoadingSymbols[selected.Sel.Name]; prohibited {
						record("workers-config-loading", workersRootImport+"."+selected.Sel.Name, selected.Sel.Pos())
					}
					if strings.HasPrefix(filePath, "pkg/transports/mapping/") {
						if _, prohibited := transportWorkersFailurePolicySymbols[selected.Sel.Name]; prohibited {
							record("workers-failure-policy", workersRootImport+"."+selected.Sel.Name, selected.Sel.Pos())
						}
					}
				}
				if imports[identifier.Name] == modelsRootImport && strings.HasPrefix(filePath, "pkg/transports/mapping/") {
					if _, prohibited := transportModelsReadinessPolicySymbols[selected.Sel.Name]; prohibited {
						record("models-readiness-policy", modelsRootImport+"."+selected.Sel.Name, selected.Sel.Pos())
					}
				}
				if imports[identifier.Name] == workRootImport {
					if _, prohibited := transportWorkPreparationSymbols[selected.Sel.Name]; prohibited {
						record("work-request-preparation", workRootImport+"."+selected.Sel.Name, selected.Sel.Pos())
					}
				}
				if imports[identifier.Name] != repositoryImportPrefix+"pkg/services/factory_sessions" {
					return true
				}
				if _, prohibited := transportFactorySessionNormalizationSymbols[selected.Sel.Name]; prohibited {
					record("service-normalization", imports[identifier.Name]+"."+selected.Sel.Name, selected.Sel.Pos())
				}
				return true
			})
			return nil
		}
		factoryDefinitionValidatorReceivers := map[string]struct{}{}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Type.Params == nil {
				continue
			}
			for _, field := range function.Type.Params.List {
				selected, ok := field.Type.(*ast.SelectorExpr)
				if !ok || selected.Sel.Name != "Validator" {
					continue
				}
				identifier, ok := selected.X.(*ast.Ident)
				if !ok || imports[identifier.Name] != factoryDefinitionsRootImport {
					continue
				}
				for _, name := range field.Names {
					factoryDefinitionValidatorReceivers[name.Name] = struct{}{}
				}
			}
		}
		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok {
				if _, retired := retiredTransportServiceEntrypoints[function.Name.Name]; retired {
					record("alternate-service-entrypoint", function.Name.Name, function.Name.Pos())
				}
				if _, prohibited := transportFactoryDefinitionValidationPolicySymbols[function.Name.Name]; prohibited {
					record("validation-orchestration", function.Name.Name, function.Name.Pos())
				}
				if strings.HasPrefix(filePath, "pkg/transports/http/") {
					if _, prohibited := httpFactoryStatusProjectionSymbols[function.Name.Name]; prohibited {
						record("factory-status-projection", function.Name.Name, function.Name.Pos())
					}
				}
				if strings.HasPrefix(filePath, factorySessionMCPTransportPrefix) && strings.HasPrefix(function.Name.Name, "NewClient") {
					record("alternate-test-entrypoint", function.Name.Name, function.Name.Pos())
				}
				if symbol, forwarded := transportServiceForwarder(function, imports); forwarded {
					record("service-forwarder", symbol, function.Name.Pos())
				}
				continue
			}
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			if general.Tok == token.TYPE && strings.HasPrefix(filePath, factorySessionMCPTransportPrefix) {
				for _, rawSpec := range general.Specs {
					spec, ok := rawSpec.(*ast.TypeSpec)
					if ok && spec.Name.Name == "Client" {
						record("alternate-test-entrypoint", spec.Name.Name, spec.Name.Pos())
					}
				}
				continue
			}
			if general.Tok != token.VAR {
				continue
			}
			for _, rawSpec := range general.Specs {
				spec, ok := rawSpec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, value := range spec.Values {
					if _, hiddenFunction := value.(*ast.FuncLit); hiddenFunction {
						name := "package var"
						if len(spec.Names) > 0 {
							name = spec.Names[0].Name
						}
						record("mutable-function-seam", name, value.Pos())
					}
				}
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.GoStmt:
				record("concurrency", "go", typed.Go)
			case *ast.CallExpr:
				if ident, ok := typed.Fun.(*ast.Ident); ok && ident.Name == "make" && len(typed.Args) > 0 {
					if _, ok := typed.Args[0].(*ast.ChanType); ok {
						record("concurrency", "make(chan)", ident.Pos())
					}
				}
				selected, ok := typed.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selected.X.(*ast.Ident)
				if !ok {
					return true
				}
				importPath, imported := imports[identifier.Name]
				if !imported {
					return true
				}
				qualified := importPath + "." + selected.Sel.Name
				if _, prohibited := transportLifecycleSymbols[qualified]; prohibited {
					record("lifecycle", qualified, selected.Sel.Pos())
				}
			case *ast.SelectorExpr:
				if strings.HasPrefix(filePath, "pkg/transports/mapping/") {
					if _, prohibited := transportFactorySessionPreparationMethods[typed.Sel.Name]; prohibited {
						record("factory-session-preparation", typed.Sel.Name, typed.Sel.Pos())
					}
				}
				if strings.HasPrefix(filePath, "pkg/transports/mapping/workcontent/") && typed.Sel.Name == "Normalized" {
					record("work-content-normalization", typed.Sel.Name, typed.Sel.Pos())
				}
				if strings.HasPrefix(filePath, "pkg/transports/http/") {
					if _, prohibited := httpFactoryStatusProjectionSymbols[typed.Sel.Name]; prohibited {
						record("factory-status-projection", typed.Sel.Name, typed.Sel.Pos())
					}
				}
				if _, prohibited := transportFactoryDefinitionValidationPolicySymbols[typed.Sel.Name]; prohibited {
					record("validation-orchestration", typed.Sel.Name, typed.Sel.Pos())
				}
				if typed.Sel.Name == "DefaultSourceContext" {
					record("source-default-selection", typed.Sel.Name, typed.Sel.Pos())
				}
				if typed.Sel.Name == "NormalizeWorkRequest" {
					record("work-request-normalization", typed.Sel.Name, typed.Sel.Pos())
				}
				identifier, ok := typed.X.(*ast.Ident)
				if !ok {
					return true
				}
				if _, validator := factoryDefinitionValidatorReceivers[identifier.Name]; validator {
					if _, prohibited := transportFactoryDefinitionValidationPhaseMethods[typed.Sel.Name]; prohibited {
						record("validation-orchestration", typed.Sel.Name, typed.Sel.Pos())
					}
				}
				importPath, imported := imports[identifier.Name]
				if !imported {
					return true
				}
				qualified := importPath + "." + typed.Sel.Name
				if _, prohibited := transportProcessAndFilesystemSymbols[qualified]; prohibited {
					record("external-effect", qualified, typed.Sel.Pos())
				}
				if _, prohibited := transportSynchronizationTypes[qualified]; prohibited {
					record("concurrency", qualified, typed.Sel.Pos())
				}
				if isPlatformImport(importPath) && hasExportedPrefix(typed.Sel.Name, transportPlatformSelectionPrefixes) {
					record("platform-selection", qualified, typed.Sel.Pos())
				}
				if _, servicePackage := serviceRootOwner(importPath); servicePackage && hasExportedPrefix(typed.Sel.Name, transportDomainPolicyPrefixes) {
					record("domain-policy", qualified, typed.Sel.Pos())
				}
				if importPath == repositoryImportPrefix+"pkg/services/factory_sessions" {
					if _, prohibited := transportFactorySessionNormalizationSymbols[typed.Sel.Name]; prohibited {
						record("service-normalization", qualified, typed.Sel.Pos())
					}
				}
				if importPath == factoryDefinitionsRootImport && typed.Sel.Name == "MapDir" {
					record("named-factory-path-policy", qualified, typed.Sel.Pos())
				}
				if importPath == workersRootImport {
					if _, prohibited := transportWorkersConfigLoadingSymbols[typed.Sel.Name]; prohibited {
						record("workers-config-loading", qualified, typed.Sel.Pos())
					}
					if strings.HasPrefix(filePath, "pkg/transports/mapping/") {
						if _, prohibited := transportWorkersFailurePolicySymbols[typed.Sel.Name]; prohibited {
							record("workers-failure-policy", qualified, typed.Sel.Pos())
						}
					}
				}
				if importPath == modelsRootImport && strings.HasPrefix(filePath, "pkg/transports/mapping/") {
					if _, prohibited := transportModelsReadinessPolicySymbols[typed.Sel.Name]; prohibited {
						record("models-readiness-policy", qualified, typed.Sel.Pos())
					}
				}
				if importPath == workRootImport {
					if _, prohibited := transportWorkPreparationSymbols[typed.Sel.Name]; prohibited {
						record("work-request-preparation", qualified, typed.Sel.Pos())
					}
				}
			case *ast.CompositeLit:
				selected, ok := typed.Type.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selected.X.(*ast.Ident)
				if ok && imports[identifier.Name] == "net/http" && selected.Sel.Name == "Server" {
					record("lifecycle", "net/http.Server", selected.Sel.Pos())
				}
			}
			return true
		})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan transport behavior: %w", err)
	}
	findings := make([]transportBehaviorFinding, 0, len(findingsByKey))
	for _, finding := range findingsByKey {
		findings = append(findings, finding)
	}
	slices.SortFunc(findings, compareTransportBehaviorFindings)
	return findings, nil
}

// transportServiceForwarder rejects exported transport functions whose only
// behavior is returning a call on an injected service-root parameter. Callers
// should receive and invoke that root operation directly; wrapping it creates
// a second entrypoint with no representation or protocol responsibility.
func transportServiceForwarder(function *ast.FuncDecl, imports map[string]string) (string, bool) {
	if function == nil || function.Recv != nil || function.Name == nil ||
		!ast.IsExported(function.Name.Name) || function.Body == nil ||
		len(function.Body.List) != 1 || function.Type == nil || function.Type.Params == nil {
		return "", false
	}
	returned, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return "", false
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	for _, field := range function.Type.Params.List {
		parameterType, ok := field.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		packageName, ok := parameterType.X.(*ast.Ident)
		if !ok {
			continue
		}
		importPath := imports[packageName.Name]
		if _, serviceRoot := serviceRootOwner(importPath); !serviceRoot {
			continue
		}
		for _, name := range field.Names {
			if name.Name == receiver.Name {
				return function.Name.Name + "->" + importPath + "." + selector.Sel.Name, true
			}
		}
	}
	return "", false
}

func isTransportBehaviorControlledImport(importPath string) bool {
	switch importPath {
	case "context", "net", "net/http", "os", "os/exec", "os/signal", "sync", "time":
		return true
	}
	if isPlatformImport(importPath) {
		return true
	}
	_, servicePackage := serviceRootOwner(importPath)
	return servicePackage
}

func isPlatformImport(importPath string) bool {
	return importPath == repositoryImportPrefix+"pkg/platform" ||
		strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/platform/")
}

func bytesContainGeneratedMarker(content []byte) bool {
	const marker = "Code generated "
	const suffix = " DO NOT EDIT."
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return false
		}
		if strings.HasPrefix(trimmed, "// "+marker) && strings.Contains(trimmed, suffix) {
			return true
		}
	}
	return false
}

func hasExportedPrefix(symbol string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if symbol == prefix || strings.HasPrefix(symbol, prefix) && len(symbol) > len(prefix) && symbol[len(prefix)] >= 'A' && symbol[len(prefix)] <= 'Z' {
			return true
		}
	}
	return false
}

func compareTransportBehaviorFindings(left, right transportBehaviorFinding) int {
	if comparison := strings.Compare(left.filePath, right.filePath); comparison != 0 {
		return comparison
	}
	if comparison := strings.Compare(left.kind, right.kind); comparison != 0 {
		return comparison
	}
	return strings.Compare(left.symbol, right.symbol)
}

func loadTransportBehaviorBaseline(repoRoot string) (transportBehaviorBaseline, error) {
	payload, err := os.ReadFile(filepath.Join(repoRoot, transportBehaviorBaselinePath))
	if os.IsNotExist(err) {
		return transportBehaviorBaseline{}, nil
	}
	if err != nil {
		return transportBehaviorBaseline{}, fmt.Errorf("read transport behavior baseline: %w", err)
	}
	var baseline transportBehaviorBaseline
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return transportBehaviorBaseline{}, fmt.Errorf("decode transport behavior baseline: %w", err)
	}
	if baseline.Version != 1 {
		return transportBehaviorBaseline{}, fmt.Errorf("transport behavior baseline version = %d, want 1", baseline.Version)
	}
	if err := requireNonEmptyMigrationBaseline(transportBehaviorBaselinePath, len(baseline.Entries)); err != nil {
		return transportBehaviorBaseline{}, err
	}
	return baseline, nil
}

func partitionTransportBehaviorFindings(findings []transportBehaviorFinding, baseline transportBehaviorBaseline) ([]transportBehaviorFinding, []transportBehaviorBaselineEntry, error) {
	baselineByKey := make(map[string]transportBehaviorBaselineEntry, len(baseline.Entries))
	for _, entry := range baseline.Entries {
		if err := validateTransportBehaviorBaselineEntry(entry); err != nil {
			return nil, nil, err
		}
		key := transportBehaviorKey(entry.FilePath, entry.Kind, entry.Symbol)
		if _, duplicate := baselineByKey[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate transport behavior baseline entry: %s %s %s", entry.FilePath, entry.Kind, entry.Symbol)
		}
		baselineByKey[key] = entry
	}
	var blocking []transportBehaviorFinding
	seen := map[string]struct{}{}
	for _, finding := range findings {
		key := transportBehaviorKey(finding.filePath, finding.kind, finding.symbol)
		seen[key] = struct{}{}
		entry, recorded := baselineByKey[key]
		if !recorded || entry.Count != finding.count {
			blocking = append(blocking, finding)
		}
	}
	var stale []transportBehaviorBaselineEntry
	for key, entry := range baselineByKey {
		if _, found := seen[key]; !found {
			stale = append(stale, entry)
		}
	}
	slices.SortFunc(stale, func(left, right transportBehaviorBaselineEntry) int {
		return strings.Compare(transportBehaviorKey(left.FilePath, left.Kind, left.Symbol), transportBehaviorKey(right.FilePath, right.Kind, right.Symbol))
	})
	return blocking, stale, nil
}

func validateTransportBehaviorBaselineEntry(entry transportBehaviorBaselineEntry) error {
	if entry.Kind == "" || entry.Symbol == "" || entry.FilePath == "" || entry.Count < 1 || entry.Stage != transportBehaviorBaselineStage || entry.DeletionGate != transportBehaviorDeletionGate {
		return fmt.Errorf("transport behavior baseline entry is incomplete or has an unrecognized deletion gate: %#v", entry)
	}
	for _, value := range []string{entry.Kind, entry.Symbol, entry.FilePath} {
		if strings.ContainsAny(value, "*?[]") {
			return fmt.Errorf("transport behavior baseline entry must be exact and cannot contain wildcards: %#v", entry)
		}
	}
	if !strings.HasPrefix(filepath.ToSlash(entry.FilePath), "pkg/transports/") {
		return fmt.Errorf("transport behavior baseline entry is outside pkg/transports: %s", entry.FilePath)
	}
	if !isRecognizedTransportBehavior(entry.Kind, entry.Symbol) {
		return fmt.Errorf("transport behavior baseline entry names an unrecognized rule: %s %s", entry.Kind, entry.Symbol)
	}
	return nil
}

func isRecognizedTransportBehavior(kind, symbol string) bool {
	switch kind {
	case "external-effect":
		_, recognized := transportProcessAndFilesystemSymbols[symbol]
		return recognized
	case "lifecycle":
		_, recognized := transportLifecycleSymbols[symbol]
		return recognized
	case "concurrency":
		if symbol == "go" || symbol == "make(chan)" {
			return true
		}
		_, recognized := transportSynchronizationTypes[symbol]
		return recognized
	case "platform-selection":
		separator := strings.LastIndex(symbol, ".")
		return separator > 0 &&
			isPlatformImport(symbol[:separator]) &&
			hasExportedPrefix(symbol[separator+1:], transportPlatformSelectionPrefixes)
	case "domain-policy":
		separator := strings.LastIndex(symbol, ".")
		if separator <= 0 {
			return false
		}
		_, servicePackage := serviceRootOwner(symbol[:separator])
		return servicePackage && hasExportedPrefix(symbol[separator+1:], transportDomainPolicyPrefixes)
	case "opaque-import":
		return isTransportBehaviorControlledImport(symbol)
	case "service-forwarder":
		return strings.Contains(symbol, "->"+repositoryImportPrefix+"pkg/services/")
	case "alternate-test-entrypoint":
		return symbol == "Client" || strings.HasPrefix(symbol, "NewClient")
	case "alternate-service-entrypoint":
		_, recognized := retiredTransportServiceEntrypoints[symbol]
		return recognized
	case "validation-orchestration":
		_, policySymbol := transportFactoryDefinitionValidationPolicySymbols[symbol]
		_, phaseMethod := transportFactoryDefinitionValidationPhaseMethods[symbol]
		return policySymbol || phaseMethod
	case "source-default-selection":
		return symbol == "DefaultSourceContext"
	case "work-request-normalization":
		return symbol == "NormalizeWorkRequest"
	case "factory-status-projection":
		_, recognized := httpFactoryStatusProjectionSymbols[symbol]
		return recognized
	case "service-normalization":
		const prefix = repositoryImportPrefix + "pkg/services/factory_sessions."
		if !strings.HasPrefix(symbol, prefix) {
			return false
		}
		_, recognized := transportFactorySessionNormalizationSymbols[strings.TrimPrefix(symbol, prefix)]
		return recognized
	case "factory-session-preparation":
		_, recognized := transportFactorySessionPreparationMethods[symbol]
		return recognized
	case "named-factory-path-policy":
		return symbol == factoryDefinitionsRootImport+".MapDir"
	case "workers-config-loading":
		if !strings.HasPrefix(symbol, workersRootImport+".") {
			return false
		}
		_, recognized := transportWorkersConfigLoadingSymbols[strings.TrimPrefix(symbol, workersRootImport+".")]
		return recognized
	case "workers-failure-policy":
		if !strings.HasPrefix(symbol, workersRootImport+".") {
			return false
		}
		_, recognized := transportWorkersFailurePolicySymbols[strings.TrimPrefix(symbol, workersRootImport+".")]
		return recognized
	case "models-readiness-policy":
		if !strings.HasPrefix(symbol, modelsRootImport+".") {
			return false
		}
		_, recognized := transportModelsReadinessPolicySymbols[strings.TrimPrefix(symbol, modelsRootImport+".")]
		return recognized
	case "work-request-preparation":
		if !strings.HasPrefix(symbol, workRootImport+".") {
			return false
		}
		_, recognized := transportWorkPreparationSymbols[strings.TrimPrefix(symbol, workRootImport+".")]
		return recognized
	case "work-content-normalization":
		return symbol == "Normalized"
	default:
		return false
	}
}

func transportBehaviorKey(filePath, kind, symbol string) string {
	return filepath.ToSlash(filePath) + "\x00" + kind + "\x00" + symbol
}

func createTransportBehaviorBaseline(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	path := filepath.Join(repoRoot, transportBehaviorBaselinePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create transport behavior baseline directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create transport behavior baseline: %w", err)
	}
	findings, err := scanTransportBehavior(repoRoot)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if len(findings) == 0 {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("refusing to create empty transport behavior baseline: no migration debt exists")
	}
	baseline := transportBehaviorBaseline{Version: 1}
	for _, finding := range findings {
		baseline.Entries = append(baseline.Entries, transportBehaviorBaselineEntry{
			Kind: finding.kind, Symbol: finding.symbol, FilePath: finding.filePath, Count: finding.count,
			Stage: transportBehaviorBaselineStage, DeletionGate: transportBehaviorDeletionGate,
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(baseline); err != nil {
		_ = file.Close()
		return fmt.Errorf("encode transport behavior baseline: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close transport behavior baseline: %w", err)
	}
	fmt.Fprintf(stdoutWriter, "[agent-factory:pkg-boundary] created %s with %d deletion-only edge(s)\n", transportBehaviorBaselinePath, len(baseline.Entries))
	return nil
}

func writeTransportBehaviorFindings(writer io.Writer, findings []transportBehaviorFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited transport %s behavior: %s (%s:%d, %d occurrence(s))\n", finding.kind, finding.symbol, finding.filePath, finding.line, finding.count)
		fmt.Fprintln(writer, "  remediation: inject an exact service operation or external-effect edge; lifecycle belongs to Initializer and domain policy belongs to its service owner.")
	}
}

func writeStaleTransportBehaviorBaselineEntries(writer io.Writer, entries []transportBehaviorBaselineEntry) {
	for _, entry := range entries {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] stale transport behavior baseline entry: %s %s %s\n", entry.FilePath, entry.Kind, entry.Symbol)
		fmt.Fprintf(writer, "  remediation: remove this entry from %s in the same change.\n", transportBehaviorBaselinePath)
	}
}

func writeTransportBehaviorBaselineSummary(writer io.Writer, count int) {
	if count > 0 {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] active transport behavior migration baseline: %d exact file/kind/symbol edge(s)\n", count)
		fmt.Fprintln(writer, "  deletion gate: move each behavior to its owner or injected edge, then delete its exact baseline entry.")
	}
}
