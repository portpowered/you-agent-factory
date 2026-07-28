package authoring_layout_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const authoringLayoutPublicPackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"

const compilationPackageRoot = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"

var authoringLayoutCompilationFoldAllowedImportRoots = []string{
	compilationPackageRoot + "/loading",
	compilationPackageRoot + "/loadedsource",
	compilationPackageRoot + "/runtimeconfig",
}

var authoringLayoutForbiddenImportRoots = []string{
	"github.com/portpowered/infinite-you/pkg/wire",
	"github.com/portpowered/infinite-you/pkg/root",
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime",
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions",
	"github.com/portpowered/infinite-you/pkg/services/workers",
	"github.com/portpowered/infinite-you/pkg/services/models",
	"github.com/portpowered/infinite-you/pkg/services/providers",
	"github.com/portpowered/infinite-you/pkg/services/provider_sessions",
	"github.com/portpowered/infinite-you/pkg/services/automations",
	"github.com/portpowered/infinite-you/pkg/services/work",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/persistence",
}

var authoringLayoutAllowedPublicTypeImportPrefixes = []string{
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions",
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts",
}

func TestPackageBoundary_PublicSurfaceDoesNotImportForbiddenOwnership(t *testing.T) {
	t.Parallel()

	assertPackageDirectImportsForbidden(
		t,
		authoringLayoutPublicPackage,
		authoringLayoutForbiddenImportRoots,
		authoringLayoutCompilationFoldAllowedImportRoots...,
	)
}

func TestPackageBoundary_PublicSurfaceDeclaresOnlyCTRDEFAuthoringVocabulary(t *testing.T) {
	t.Parallel()

	servicePath := filepath.Join("service.go")
	file, err := parser.ParseFile(token.NewFileSet(), servicePath, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", servicePath, err)
	}

	exported := map[string]bool{}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || !typeSpec.Name.IsExported() {
				continue
			}
			exported[typeSpec.Name.Name] = true
			switch typeSpec.Name.Name {
			case "Service":
				assertServiceInterfaceUsesAllowedVocabulary(t, typeSpec)
			case "Dependencies":
				assertDependenciesUseInjectedPortsOnly(t, typeSpec)
			default:
				t.Fatalf("service.go exports unexpected type %s; public surface must stay Service and Dependencies only", typeSpec.Name.Name)
			}
		}
	}
	if !exported["Service"] || !exported["Dependencies"] {
		t.Fatalf("service.go exported types = %v, want Service and Dependencies", exported)
	}
}

func assertServiceInterfaceUsesAllowedVocabulary(t *testing.T, typeSpec *ast.TypeSpec) {
	t.Helper()

	interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		t.Fatalf("Service must remain an interface")
	}
	for _, method := range interfaceType.Methods.List {
		funcType, ok := method.Type.(*ast.FuncType)
		if !ok {
			t.Fatalf("Service method is not a function type")
		}
		for _, fieldList := range []*ast.FieldList{funcType.Params, funcType.Results} {
			if fieldList == nil {
				continue
			}
			for _, param := range fieldList.List {
				assertTypeExprUsesAllowedImports(t, param.Type)
			}
		}
	}
}

func assertDependenciesUseInjectedPortsOnly(t *testing.T, typeSpec *ast.TypeSpec) {
	t.Helper()

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		t.Fatalf("Dependencies must remain a struct")
	}
	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			t.Fatalf("Dependencies must use named injected ports")
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				t.Fatalf("Dependencies field %s must be exported", name.Name)
			}
		}
		assertTypeExprUsesAllowedImports(t, field.Type)
	}
}

func assertTypeExprUsesAllowedImports(t *testing.T, expr ast.Expr) {
	t.Helper()

	switch typed := expr.(type) {
	case *ast.StarExpr:
		assertTypeExprUsesAllowedImports(t, typed.X)
	case *ast.ArrayType:
		assertTypeExprUsesAllowedImports(t, typed.Elt)
	case *ast.Ident:
		switch typed.Name {
		case "context", "Context", "error", "string", "bool", "byte", "int", "int64":
			return
		}
		t.Fatalf("unexpected bare identifier %s on authoring_layout public surface; use factory_definitions or contracts vocabulary", typed.Name)
	case *ast.SelectorExpr:
		if ident, ok := typed.X.(*ast.Ident); ok && ident.Name == "context" && typed.Sel.Name == "Context" {
			return
		}
		prefix := selectorImportPrefix(typed)
		if prefix == "" {
			t.Fatalf("could not resolve selector %s on authoring_layout public surface", exprString(expr))
		}
		for _, allowed := range authoringLayoutAllowedPublicTypeImportPrefixes {
			if prefix == allowed {
				return
			}
		}
		t.Fatalf("authoring_layout public surface type %s must use only factory_definitions root or contracts imports", exprString(expr))
	case *ast.FuncType:
		for _, fieldList := range []*ast.FieldList{typed.Params, typed.Results} {
			if fieldList == nil {
				continue
			}
			for _, param := range fieldList.List {
				assertTypeExprUsesAllowedImports(t, param.Type)
			}
		}
	default:
		t.Fatalf("unexpected type expression %T on authoring_layout public surface", expr)
	}
}

func selectorImportPrefix(selector *ast.SelectorExpr) string {
	switch typed := selector.X.(type) {
	case *ast.Ident:
		switch typed.Name {
		case "factorydefinitions":
			return "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
		case "factorycontracts":
			return "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
		case "context":
			return "context"
		default:
			return ""
		}
	case *ast.SelectorExpr:
		return selectorImportPrefix(typed)
	default:
		return ""
	}
}

func exprString(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return exprString(typed.X) + "." + typed.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(typed.X)
	default:
		return "type"
	}
}

func assertPackageDirectImportsForbidden(
	t *testing.T,
	packagePath string,
	forbiddenRoots []string,
	allowedRoots ...string,
) {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", packagePath, err, output)
	}
	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, allowed := range allowedRoots {
			if importPath == allowed || strings.HasPrefix(importPath, allowed+"/") {
				goto nextImport
			}
		}
		for _, forbidden := range forbiddenRoots {
			if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
				t.Fatalf("%s must not import forbidden ownership %s; found direct import %s", packagePath, forbidden, importPath)
			}
		}
	nextImport:
	}
}

func TestPackageBoundary_WireSurfaceDoesNotConstructRuntimeOrSelectSiblingLeases(t *testing.T) {
	t.Parallel()

	wirePath := filepath.Join("wire", "wire.go")
	source, err := os.ReadFile(wirePath)
	if err != nil {
		t.Fatalf("read %s: %v", wirePath, err)
	}
	body := string(source)
	for _, forbidden := range []string{
		"factory_runtime",
		"petri",
		"pkg/wire",
		"internal/services/catalog",
		"internal/services/validation",
		"compilation",
		"snapshots_portability",
		"distribution",
		"provideOrchestrator",
		"NewPetri",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("%s must not construct Runtime/Petri implementations or import sibling leases; found %q", wirePath, forbidden)
		}
	}

	wirePackage := authoringLayoutPublicPackage + "/wire"
	assertPackageDirectImportsForbidden(
		t,
		wirePackage,
		authoringLayoutForbiddenImportRoots,
		authoringLayoutCompilationFoldAllowedImportRoots...,
	)
}

func TestPackageBoundary_InternalLeasePackagesDoNotImportForbiddenOwnership(t *testing.T) {
	t.Parallel()

	for _, packageSuffix := range []string{
		"/internal/service",
		"/prepare",
		"/persist",
		"/flatten",
		"/expand",
		"/authoredlayout",
	} {
		assertPackageDirectImportsForbidden(
			t,
			authoringLayoutPublicPackage+packageSuffix,
			authoringLayoutForbiddenImportRoots,
			authoringLayoutCompilationFoldAllowedImportRoots...,
		)
	}
}

