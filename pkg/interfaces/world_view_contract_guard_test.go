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

	"github.com/portpowered/agent-factory/internal/contractguard"
	"github.com/portpowered/agent-factory/internal/handwrittensourceguard"
)

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
					t.Fatalf("%s introduces unapproved FactoryWorld*View mirror %s; update the cleanup artifact and this allowlist before adding new boundary-only views", path, typeName)
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

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if contractguard.ShouldSkipDir(".", path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		rel := filepath.Clean(filepath.Join("interfaces", filepath.Base(path)))
		if _, ok := allowed[rel]; ok {
			return nil
		}
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

	violations, err := scanWorldViewCanonicalMirrorViolations("..")
	if err != nil {
		t.Fatalf("scan pkg go files: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("%s still contains retired mirror name %q; equivalent rg guard is `rg -n %q pkg -g \"*.go\"` from the repository root and should only hit approved guard notes", violations[0].file, violations[0].name, strings.Join(retiredFactoryCanonicalMirrorNames, "|"))
	}
}

func TestFactoryWorldContractGuard_CanonicalPkgScanSkipsGeneratedApiOutputButScansHandwrittenPackages(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeWorldViewGuardFixture(t, root, "api/generated/runtime_lookup.go", `package generated

type FactoryProviderFailure struct{}
`)
	writeWorldViewGuardFixture(t, root, "workers/runtime_lookup.go", `package workers

type FactoryProviderFailure struct{}
`)

	violations, err := scanWorldViewCanonicalMirrorViolations(root)
	if err != nil {
		t.Fatalf("scan temp pkg go files: %v", err)
	}
	if len(violations) != 1 || violations[0].file != filepath.Clean("workers/runtime_lookup.go") || violations[0].name != "FactoryProviderFailure" {
		t.Fatalf("violations = %#v, want only handwritten worker violation", violations)
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

type worldViewCanonicalMirrorViolation struct {
	file string
	name string
}

func scanWorldViewCanonicalMirrorViolations(root string) ([]worldViewCanonicalMirrorViolation, error) {
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

	var violations []worldViewCanonicalMirrorViolation
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if handwrittensourceguard.ShouldSkipDir("pkg/interfaces/world_view_contract_guard_test.go#canonical", root, path) {
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
		rel = filepath.Clean(rel)
		if _, ok := allowed[rel]; ok {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if match := matcher.FindString(string(data)); match != "" {
			violations = append(violations, worldViewCanonicalMirrorViolation{file: rel, name: match})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].file == violations[j].file {
			return violations[i].name < violations[j].name
		}
		return violations[i].file < violations[j].file
	})
	return violations, nil
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
