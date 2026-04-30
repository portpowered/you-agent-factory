package interfaces

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/internal/contractguard"
)

const approvedRuntimeLookupFactoryDirOwner = "interfaces/runtime_lookup.go"

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
			approvedRuntimeLookupFactoryDirOwner,
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

func scanRuntimeLookupContractViolations(root string) ([]runtimeLookupContractViolation, error) {
	root = filepath.Clean(root)

	var violations []runtimeLookupContractViolation
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if contractguard.ShouldSkipDir(root, path) {
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

	violations, err := scanRuntimeLookupContractViolations(root)
	if err != nil {
		t.Fatalf("scan temp runtime lookup ownership: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("hidden metadata fixtures should be skipped, got violations = %v", violations)
	}
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
			detail: "type RuntimeConfig shadows the canonical lookup family in pkg/interfaces",
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
			detail: "path-aware runtime lookup interfaces must stay on interfaces.RuntimeConfigLookup",
		})
	}
	if interfaceDeclaresRuntimeBaseDir(iface) && !isApprovedRuntimeLookupOwner(rel, spec.Name.Name) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "unapproved RuntimeBaseDir interface owner",
			detail: "runtime-base execution lookups must stay on interfaces.RuntimeConfigLookup",
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
			detail: "replace anonymous FactoryDir interfaces with interfaces.RuntimeConfigLookup",
		})
	}
	if interfaceDeclaresRuntimeBaseDir(iface) {
		violations = append(violations, runtimeLookupContractViolation{
			file:   rel,
			kind:   "raw RuntimeBaseDir escape hatch",
			detail: "replace anonymous RuntimeBaseDir interfaces with interfaces.RuntimeConfigLookup",
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
