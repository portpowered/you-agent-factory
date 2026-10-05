package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// ProviderOwnership preserves the zero-debt Providers effect and catalog
// ownership policy using compiler declarations and resolved type identities.
var ProviderOwnership = &analysis.Analyzer{
	Name: "providerownership",
	Doc:  "enforce Providers leaf effects and one catalog/execution owner",
	Run:  runProviderOwnership,
}

const providerLeaf = "pkg/services/providers/execution/inferencecontract"

func runProviderOwnership(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok || !under(unit, "pkg") || strings.HasSuffix(unit, "_test") {
		return nil, nil
	}
	for _, file := range pass.Files {
		if strings.HasSuffix(filepath.Base(pass.Fset.Position(file.Pos()).Filename), "_test.go") {
			continue
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typed := spec.(*ast.TypeSpec)
				if kind := providerOwnershipKind(pass, unit, typed); kind != "" {
					pass.Reportf(typed.Pos(), "provider-ownership-%s: %s#%s; use Providers-owned catalog/execution truth and exact %s.Provider; do not redeclare or alias the leaf effect contract", kind, unit, typed.Name.Name, providerLeaf)
				}
			}
		}
	}
	return nil, nil
}

func providerOwnershipKind(pass *analysis.Pass, unit string, spec *ast.TypeSpec) string {
	if unit != "pkg/services/edges" && competingProviderDeclaration(unit, spec) {
		return "competing-catalog-or-execution"
	}
	if unit == providerLeaf || providerWorkersBridge(pass, unit, spec) {
		return ""
	}
	if !providerEffectExpression(pass, spec.Type) && !providerEffectFields(pass, spec.Type) {
		return ""
	}
	if unit == "pkg/services/edges" {
		return "edges-redefinition"
	}
	return "durable-owner"
}

func providerNamedType(typ types.Type, path, name string) bool {
	if typ == nil {
		return false
	}
	named, ok := types.Unalias(typ).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == path && named.Obj().Name() == name
}

func providerInferSignature(pass *analysis.Pass, field *ast.Field) *types.Signature {
	if len(field.Names) != 1 || field.Names[0].Name != "Infer" {
		return nil
	}
	signature, ok := pass.TypesInfo.TypeOf(field.Type).(*types.Signature)
	if !ok || signature.Params().Len() != 2 || signature.Results().Len() != 2 {
		return nil
	}
	if !providerNamedType(signature.Params().At(0).Type(), "context", "Context") ||
		!types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil
	}
	return signature
}

func providerWorkersBridge(pass *analysis.Pass, unit string, spec *ast.TypeSpec) bool {
	if unit != "pkg/services/workers" || spec.Name.Name != "Provider" {
		return false
	}
	iface, ok := spec.Type.(*ast.InterfaceType)
	if !ok || iface.Methods == nil || len(iface.Methods.List) != 1 {
		return false
	}
	sig := providerInferSignature(pass, iface.Methods.List[0])
	return sig != nil &&
		providerNamedType(sig.Params().At(1).Type(), modulePrefix+unit, "ProviderInferenceRequest") &&
		providerNamedType(sig.Results().At(0).Type(), modulePrefix+unit, "InferenceResponse")
}

func providerLeafExpression(pass *analysis.Pass, expression ast.Expr) bool {
	return providerNamedType(pass.TypesInfo.TypeOf(expression), modulePrefix+providerLeaf, "Provider")
}

func providerEffectExpression(pass *analysis.Pass, expression ast.Expr) bool {
	if providerLeafExpression(pass, expression) {
		return true
	}
	if paren, ok := expression.(*ast.ParenExpr); ok {
		return providerEffectExpression(pass, paren.X)
	}
	iface, ok := expression.(*ast.InterfaceType)
	if !ok || iface.Methods == nil {
		return false
	}
	for _, field := range iface.Methods.List {
		if providerInferSignature(pass, field) != nil || len(field.Names) == 0 && providerLeafExpression(pass, field.Type) {
			return true
		}
	}
	return false
}

func providerEffectFields(pass *analysis.Pass, expression ast.Expr) bool {
	structure, ok := expression.(*ast.StructType)
	if !ok || structure.Fields == nil {
		return false
	}
	for _, field := range structure.Fields.List {
		// Aggregating the exact leaf is permitted; wrapping it is ownership.
		if providerLeafExpression(pass, field.Type) {
			continue
		}
		found := false
		ast.Inspect(field.Type, func(node ast.Node) bool {
			if expr, ok := node.(ast.Expr); ok && providerEffectExpression(pass, expr) {
				found = true
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

func competingProviderDeclaration(unit string, spec *ast.TypeSpec) bool {
	if under(unit, "pkg/services/providers") || !competingProviderPackage(unit) {
		return false
	}
	switch spec.Name.Name {
	case "Catalog", "Registry", "Conductor", "Provider":
		if spec.Assign.IsValid() {
			return true
		}
		switch spec.Type.(type) {
		case *ast.StructType, *ast.InterfaceType:
			return true
		}
	}
	return false
}

func competingProviderPackage(unit string) bool {
	parts := strings.Split(unit, "/")
	for i, part := range parts {
		for _, family := range []string{"catalog", "registry", "conductor", "execution"} {
			if part == "provider"+family || (part == "provider" || part == "providers") && i+1 < len(parts) && parts[i+1] == family {
				return true
			}
		}
	}
	return false
}
