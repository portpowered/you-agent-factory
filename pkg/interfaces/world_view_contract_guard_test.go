package interfaces

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

const approvedRuntimeLookupFactoryDirOwner = "interfaces/runtime_lookup.go"
const approvedRuntimeLookupFactoryDirOwnerDisplay = "pkg/interfaces/runtime_lookup.go"

type runtimeLookupContractViolation struct {
	file   string
	kind   string
	detail string
}

func TestRuntimeLookupContractGuard_PackageScanKeepsCanonicalLookupOwnership(t *testing.T) {
	t.Parallel()

	violations, err := scanRuntimeLookupContractViolations("..")
	if err != nil {
		t.Fatalf("scan runtime lookup contract ownership: %v", err)
	}
	if len(violations) != 0 {
		var details []string
		for _, violation := range violations {
			details = append(details, violation.file+": "+violation.kind+" ("+violation.detail+")")
		}
		t.Fatalf(
			"runtime lookup ownership regression:\n%s\nOnly the canonical runtime lookup family in %s may own path-aware runtime lookup interfaces, and package-local RuntimeConfig declarations stay deleted",
			strings.Join(details, "\n"),
			approvedRuntimeLookupFactoryDirOwnerDisplay,
		)
	}
}

func TestRuntimeLookupContractGuard_DetectsPackageLocalRuntimeConfigDeclaration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, "config/runtime_config_alias.go", `package config

type RuntimeConfig = any
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}

	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"config/runtime_config_alias.go:package-local RuntimeConfig declaration"},
	)
}

func TestRuntimeLookupContractGuard_DetectsRawFactoryDirEscapeHatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, "workers/workstation_executor.go", `package workers

func factoryDir(v interface{ FactoryDir() string }) string {
	return v.FactoryDir()
}
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}

	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"workers/workstation_executor.go:raw FactoryDir escape hatch"},
	)
}

func TestRuntimeLookupContractGuard_DetectsUnapprovedRuntimeBaseDirInterfaceOwner(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, "workers/runtime_lookup.go", `package workers

type RuntimeExecutionLookup interface {
	RuntimeBaseDir() string
}
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}

	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"workers/runtime_lookup.go:unapproved RuntimeBaseDir interface owner"},
	)
}

func TestRuntimeLookupContractGuard_DetectsRawRuntimeBaseDirEscapeHatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, "workers/workstation_executor.go", `package workers

func runtimeBaseDir(v interface{ RuntimeBaseDir() string }) string {
	return v.RuntimeBaseDir()
}
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}

	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"workers/workstation_executor.go:raw RuntimeBaseDir escape hatch"},
	)
}

func TestRuntimeLookupContractGuard_SkipsHiddenAndGeneratedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, ".hidden/runtime_lookup.go", `package hidden

type RuntimeConfig = any
`)
	writeRuntimeLookupGuardFixture(t, root, "api/generated/runtime_lookup.go", `package generated

type RuntimeExecutionLookup interface {
	RuntimeBaseDir() string
}
`)
	writeRuntimeLookupGuardFixture(t, root, "workers/runtime_lookup.go", `package workers

type RuntimeExecutionLookup interface {
	RuntimeBaseDir() string
}
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}
	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"workers/runtime_lookup.go:unapproved RuntimeBaseDir interface owner"},
	)
}

func scanRuntimeLookupContractViolations(root string) ([]runtimeLookupContractViolation, error) {
	root = filepath.Clean(root)

	var violations []runtimeLookupContractViolation
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if contractguard.ShouldSkipDir(root, path, "api/generated") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fileViolations, err := scanRuntimeLookupFile(fset, path, filepath.ToSlash(filepath.Clean(rel)))
		if err != nil {
			return err
		}
		violations = append(violations, fileViolations...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file == violations[j].file {
			if violations[i].kind == violations[j].kind {
				return violations[i].detail < violations[j].detail
			}
			return violations[i].kind < violations[j].kind
		}
		return violations[i].file < violations[j].file
	})

	return violations, nil
}

func TestRuntimeLookupContractGuard_SkipsHiddenMetadataDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeRuntimeLookupGuardFixture(t, root, ".claude/runtime_lookup.go", `package claude

type RuntimeConfig = any
`)
	writeRuntimeLookupGuardFixture(t, root, "workers/runtime_config_alias.go", `package workers

type RuntimeConfig = any
`)

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}
	assertRuntimeLookupViolationKinds(
		t,
		violations,
		[]string{"workers/runtime_config_alias.go:package-local RuntimeConfig declaration"},
	)
}

func scanRuntimeLookupFile(fset *token.FileSet, path string, rel string) ([]runtimeLookupContractViolation, error) {
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var violations []runtimeLookupContractViolation
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.TypeSpec:
			violations = append(violations, runtimeLookupTypeSpecViolations(rel, typed)...)
			return false
		case *ast.InterfaceType:
			violations = append(violations, rawRuntimeLookupInterfaceViolations(rel, typed)...)
		}
		return true
	})
	return violations, nil
}

func runtimeLookupTypeSpecViolations(rel string, spec *ast.TypeSpec) []runtimeLookupContractViolation {
	var violations []runtimeLookupContractViolation
	if spec.Name.Name == "RuntimeConfig" {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "package-local RuntimeConfig declaration",
			detail: "type RuntimeConfig shadows the canonical lookup family in pkg/interfaces/runtime_lookup.go",
		})
	}

	iface, ok := spec.Type.(*ast.InterfaceType)
	if !ok {
		return violations
	}
	if interfaceDeclaresFactoryDir(iface) && !isApprovedRuntimeLookupOwner(rel, spec.Name.Name) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "unapproved FactoryDir interface owner",
			detail: "path-aware runtime lookup interfaces must stay on pkg/interfaces.RuntimeConfigLookup",
		})
	}
	if interfaceDeclaresRuntimeBaseDir(iface) && !isApprovedRuntimeLookupOwner(rel, spec.Name.Name) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "unapproved RuntimeBaseDir interface owner",
			detail: "runtime-base execution lookups must stay on pkg/interfaces.RuntimeConfigLookup",
		})
	}
	return violations
}

func rawRuntimeLookupInterfaceViolations(rel string, iface *ast.InterfaceType) []runtimeLookupContractViolation {
	var violations []runtimeLookupContractViolation
	if interfaceDeclaresFactoryDir(iface) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "raw FactoryDir escape hatch",
			detail: "replace anonymous FactoryDir interfaces with pkg/interfaces.RuntimeConfigLookup",
		})
	}
	if interfaceDeclaresRuntimeBaseDir(iface) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "raw RuntimeBaseDir escape hatch",
			detail: "replace anonymous RuntimeBaseDir interfaces with pkg/interfaces.RuntimeConfigLookup",
		})
	}
	return violations
}

func isApprovedRuntimeLookupOwner(rel string, typeName string) bool {
	return rel == approvedRuntimeLookupFactoryDirOwner && typeName == "RuntimeConfigLookup"
}

func interfaceDeclaresFactoryDir(iface *ast.InterfaceType) bool {
	return interfaceDeclaresStringNoArgMethod(iface, "FactoryDir")
}

func interfaceDeclaresRuntimeBaseDir(iface *ast.InterfaceType) bool {
	return interfaceDeclaresStringNoArgMethod(iface, "RuntimeBaseDir")
}

func interfaceDeclaresStringNoArgMethod(iface *ast.InterfaceType, methodName string) bool {
	if iface == nil || iface.Methods == nil {
		return false
	}

	for _, method := range iface.Methods.List {
		if len(method.Names) != 1 || method.Names[0].Name != methodName {
			continue
		}
		signature, ok := method.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		if signature.Params != nil && len(signature.Params.List) != 0 {
			continue
		}
		if signature.Results == nil || len(signature.Results.List) != 1 {
			continue
		}
		resultIdent, ok := signature.Results.List[0].Type.(*ast.Ident)
		if ok && resultIdent.Name == "string" {
			return true
		}
	}

	return false
}

func writeRuntimeLookupGuardFixture(t *testing.T, root, relativePath, contents string) {
	t.Helper()

	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertRuntimeLookupViolationKinds(t *testing.T, violations []runtimeLookupContractViolation, want []string) {
	t.Helper()

	got := make([]string, 0, len(violations))
	for _, violation := range violations {
		got = append(got, violation.file+":"+violation.kind)
	}
	sort.Strings(got)
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("violation count = %d, want %d\nviolations = %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("violations = %v, want %v", got, want)
		}
	}
}

var approvedWorkDispatchFields = map[string]string{
	"DispatchID":               "dispatch_id",
	"TransitionID":             "transition_id",
	"WorkerType":               "worker_type",
	"WorkstationName":          "workstation_name",
	"ProjectID":                "project_id",
	"CurrentChainingTraceID":   "current_chaining_trace_id",
	"PreviousChainingTraceIDs": "previous_chaining_trace_ids",
	"Execution":                "execution",
	"InputTokens":              "input_tokens",
	"InputBindings":            "input_bindings",
}

var retiredWorkDispatchWorkerFields = []string{
	"WorkstationType",
	"SystemPrompt",
	"UserMessage",
	"OutputSchema",
	"EnvVars",
	"Worktree",
	"WorkingDirectory",
	"Model",
	"ModelProvider",
	"Provider",
	"SessionID",
	"Command",
	"Args",
	"Stdin",
	"Env",
	"WorkDir",
}

func TestWorkDispatchContractGuard_FieldInventoryStaysDispatchOwned(t *testing.T) {
	t.Parallel()

	workDispatchType := reflect.TypeOf(WorkDispatch{})
	seen := make(map[string]struct{}, workDispatchType.NumField())
	for i := 0; i < workDispatchType.NumField(); i++ {
		field := workDispatchType.Field(i)
		wantJSONTag, ok := approvedWorkDispatchFields[field.Name]
		if !ok {
			t.Fatalf("WorkDispatch field %s is not in the approved dispatch-owned inventory; update the split-contract artifact and this guard before expanding the canonical dispatch payload", field.Name)
		}
		gotJSONTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if gotJSONTag != wantJSONTag {
			t.Fatalf("WorkDispatch field %s json tag = %q, want %q", field.Name, gotJSONTag, wantJSONTag)
		}
		seen[field.Name] = struct{}{}
	}

	for fieldName := range approvedWorkDispatchFields {
		if _, ok := seen[fieldName]; !ok {
			t.Fatalf("WorkDispatch is missing approved field %s", fieldName)
		}
	}
}

func TestWorkDispatchContractGuard_WorkerOwnedFieldsStayDeleted(t *testing.T) {
	t.Parallel()

	workDispatchType := reflect.TypeOf(WorkDispatch{})
	for _, fieldName := range retiredWorkDispatchWorkerFields {
		if _, ok := workDispatchType.FieldByName(fieldName); ok {
			t.Fatalf("WorkDispatch must not reintroduce worker-owned field %s", fieldName)
		}
	}
}

var retiredFactoryBoundaryMirrorNames = []string{
	"FactoryWorldWorkstationRequestView",
	"FactoryWorldWorkstationRequestCountView",
	"FactoryWorldWorkstationRequestRequestView",
	"FactoryWorldWorkstationRequestResponseView",
	"FactoryWorldTokenView",
	"FactoryWorldMutationView",
}

var retiredFactoryCanonicalMirrorNames = []string{
	"FactoryProviderFailure",
	"FactoryProviderSession",
	"FactoryWorkDiagnostics",
	"FactoryRenderedPromptDiagnostic",
	"FactoryProviderDiagnostic",
	"FactoryEnabledTransitionView",
	"FactoryFiringDecisionView",
	"FactoryWorldDispatchView",
	"FactoryWorldProviderSessionView",
	"FactoryWorldInferenceAttemptView",
}

var approvedBoundaryViews = map[string]struct{}{
	"FactoryWorldView":         {},
	"FactoryWorldTopologyView": {},
	"FactoryWorldRuntimeView":  {},
}

var retiredSimpleDashboardAggregateSeamNames = []string{
	"FactoryWorldView",
	"FactoryWorldTopologyView",
	"FactoryWorldRuntimeView",
}

func TestFactoryWorldContractGuard_RetiredMirrorTypesStayDeleted(t *testing.T) {
	t.Parallel()

	forbidden := toStringSet(allRetiredFactoryMirrorNames())

	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob interface package files: %v", err)
	}

	fset := token.NewFileSet()
	for _, path := range paths {
		if filepath.Ext(path) != ".go" || filepath.Base(path) == "world_view_contract_guard_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				typeName := typeSpec.Name.Name
				if _, blocked := forbidden[typeName]; blocked {
					t.Fatalf("%s reintroduces retired mirror type %s", path, typeName)
				}
				if !strings.HasPrefix(typeName, "FactoryWorld") || !strings.HasSuffix(typeName, "View") {
					continue
				}
				if _, approved := approvedBoundaryViews[typeName]; !approved {
					t.Fatalf("%s introduces unapproved FactoryWorld*View mirror %s; update docs/development/world-view-contract-cleanup-data-model.md and this allowlist before adding new boundary-only views", path, typeName)
				}
			}
		}
	}
}

func TestFactoryWorldContractGuard_RetiredBoundaryMirrorNamesStayOutOfInterfacesGoFiles(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredFactoryBoundaryMirrorNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)
	allowed := map[string]struct{}{
		filepath.Clean("interfaces/world_view_contract_guard_test.go"): {},
	}

	err := walkWorldViewMirrorFiles(".", "interfaces", false, allowed, func(path, rel string) error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s still contains retired boundary mirror name %q; keep API-owned workstation-request, token, and mutation DTOs out of pkg/interfaces", rel, match)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan interface package go files: %v", err)
	}
}

func TestFactoryWorldContractGuard_RetiredCanonicalMirrorNamesStayOutOfPkgGoFiles(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredFactoryCanonicalMirrorNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)
	allowed := map[string]struct{}{
		filepath.Clean("interfaces/world_view_contract_guard_test.go"): {},
	}

	err := walkWorldViewMirrorFiles("..", "", true, allowed, func(path, rel string) error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s still contains retired mirror name %q; equivalent rg guard is `rg -n %q pkg -g \"*.go\"` from the repository root and should only hit approved guard notes", rel, match, strings.Join(names, "|"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan pkg go files: %v", err)
	}
}

func TestFactoryWorldContractGuard_SkipsHiddenAndGeneratedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorldViewGuardFixture(t, root, ".hidden/hidden.go", `package hidden

type Hidden struct{}

var _ = "FactoryProviderFailure"
`)
	writeWorldViewGuardFixture(t, root, "api/generated/generated.go", `package generated

var _ = "FactoryProviderFailure"
`)
	writeWorldViewGuardFixture(t, root, "feature/kept.go", `package feature

var _ = "FactoryProviderFailure"
`)

	allowed := map[string]struct{}{}
	var visited []string
	if err := walkWorldViewMirrorFiles(root, "", true, allowed, func(path, rel string) error {
		visited = append(visited, rel)
		return nil
	}); err != nil {
		t.Fatalf("walk world view mirror files: %v", err)
	}
	if len(visited) != 1 || visited[0] != filepath.Clean("feature/kept.go") {
		t.Fatalf("visited = %v, want only handwritten source file %s", visited, filepath.Clean("feature/kept.go"))
	}
}

func TestFactoryWorldContractGuard_RuntimeShellUsesCanonicalSelectedTickTypes(t *testing.T) {
	t.Parallel()

	topologyType := reflect.TypeOf(FactoryWorldTopologyView{})
	assertWorldViewSliceType(t, topologyType, "SubmitWorkTypes", reflect.TypeOf(FactoryWorldSubmitWorkType{}))

	runtimeType := reflect.TypeOf(FactoryWorldRuntimeView{})
	assertWorldViewFieldType(t, runtimeType, "InferenceAttemptsByDispatchID", reflect.TypeOf(map[string]map[string]FactoryWorldInferenceAttempt{}))
	if _, ok := runtimeType.FieldByName("WorkstationRequestsByDispatchID"); ok {
		t.Fatal("FactoryWorldRuntimeView must not retain the API-owned workstation_requests_by_dispatch_id projection")
	}

	sessionField, ok := runtimeType.FieldByName("Session")
	if !ok {
		t.Fatalf("FactoryWorldRuntimeView missing Session field")
	}
	if sessionField.Type != reflect.TypeOf(FactoryWorldSessionRuntime{}) {
		t.Fatalf("FactoryWorldRuntimeView.Session = %v, want %v", sessionField.Type, reflect.TypeOf(FactoryWorldSessionRuntime{}))
	}

	activeExecutionType := reflect.TypeOf(FactoryWorldActiveExecution{})
	assertWorldViewSliceType(t, activeExecutionType, "ConsumedInputs", reflect.TypeOf(WorkstationInput{}))
	assertWorldViewFieldAbsent(t, activeExecutionType, "ConsumedTokens")
	assertWorldViewFieldAbsent(t, activeExecutionType, "OutputMutations")

	sessionType := reflect.TypeOf(FactoryWorldSessionRuntime{})
	assertWorldViewSliceType(t, sessionType, "DispatchHistory", reflect.TypeOf(FactoryWorldDispatchCompletion{}))
	assertWorldViewSliceType(t, sessionType, "ProviderSessions", reflect.TypeOf(FactoryWorldProviderSessionRecord{}))
	assertWorldViewFieldAbsent(t, sessionType, "CompletedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkLabels")
	assertWorldViewFieldAbsent(t, sessionType, "FailedWorkDetailsByWorkID")
}

func TestFactoryWorldContractGuard_SimpleDashboardSeamStaysOffBroadAggregateShell(t *testing.T) {
	t.Parallel()

	names := append([]string(nil), retiredSimpleDashboardAggregateSeamNames...)
	sort.Strings(names)
	patterns := make([]string, 0, len(names))
	for _, name := range names {
		patterns = append(patterns, regexp.QuoteMeta(name))
	}
	matcher := regexp.MustCompile(`\b(?:` + strings.Join(patterns, "|") + `)\b`)

	guardedFiles := []string{
		filepath.Clean("../service/factory.go"),
		filepath.Clean("../cli/dashboard/dashboard.go"),
	}
	for _, path := range guardedFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if match := matcher.FindString(string(data)); match != "" {
			t.Fatalf("%s references %q; the simple-dashboard aggregate-retirement decision keeps this seam on projections.BuildSimpleDashboardWorldView(...) and forbids reintroducing pkg/interfaces aggregate shell ownership here", filepath.Clean(path), match)
		}
	}
}

func assertWorldViewFieldType(t *testing.T, structType reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()

	field, ok := structType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing %s field", structType.Name(), fieldName)
	}
	if field.Type != want {
		t.Fatalf("%s.%s = %v, want %v", structType.Name(), fieldName, field.Type, want)
	}
}

func assertWorldViewSliceType(t *testing.T, structType reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()

	field, ok := structType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s missing %s field", structType.Name(), fieldName)
	}
	if field.Type.Kind() != reflect.Slice {
		t.Fatalf("%s.%s kind = %s, want slice", structType.Name(), fieldName, field.Type.Kind())
	}
	if field.Type.Elem() != want {
		t.Fatalf("%s.%s element = %v, want %v", structType.Name(), fieldName, field.Type.Elem(), want)
	}
}

func assertWorldViewFieldAbsent(t *testing.T, structType reflect.Type, fieldName string) {
	t.Helper()

	if _, ok := structType.FieldByName(fieldName); ok {
		t.Fatalf("%s must not expose display-only %s field", structType.Name(), fieldName)
	}
}

func toStringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func allRetiredFactoryMirrorNames() []string {
	names := make([]string, 0, len(retiredFactoryBoundaryMirrorNames)+len(retiredFactoryCanonicalMirrorNames))
	names = append(names, retiredFactoryBoundaryMirrorNames...)
	names = append(names, retiredFactoryCanonicalMirrorNames...)
	return names
}

func walkWorldViewMirrorFiles(root, relPrefix string, includeGenerated bool, allowed map[string]struct{}, visit func(path, rel string) error) error {
	root = filepath.Clean(root)

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			explicitSkips := []string{}
			if includeGenerated {
				explicitSkips = append(explicitSkips, "api/generated")
			}
			if contractguard.ShouldSkipDir(root, path, explicitSkips...) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if relPrefix != "" {
			rel = filepath.Join(relPrefix, rel)
		}
		rel = filepath.Clean(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		return visit(path, rel)
	})
}

func writeWorldViewGuardFixture(t *testing.T, root, relativePath, contents string) {
	t.Helper()

	path := filepath.Join(root, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
