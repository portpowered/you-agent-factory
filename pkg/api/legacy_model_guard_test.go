package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/handwrittensourceguard"
)

func TestNoHandwrittenLegacyReplayModelsOrGeneratedAliases(t *testing.T) {
	moduleRoot := filepath.Clean(filepath.Join("..", ".."))
	generatedImportPaths := map[string]struct{}{
		"github.com/portpowered/infinite-you/pkg/api/generated": {},
		"pkg/api/generated": {},
	}
	deletedTypeNames := map[string]struct{}{
		"FactoryEventEnvelope": {},
		"FactoryEventContext":  {},
		"FactoryEventType":     {},
		"RecordedWorkRequest":  {},
		"RecordedSubmission":   {},
		"RecordedDispatch":     {},
		"RecordedCompletion":   {},
		"SubmissionDiagnostic": {},
		"DispatchDiagnostic":   {},
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if handwrittensourceguard.ShouldSkipDir("pkg/api/legacy_model_guard_test.go", moduleRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, ".gen.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		generatedImportNames := generatedAPIAliases(file, generatedImportPaths)
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if _, deleted := deletedTypeNames[typeSpec.Name.Name]; deleted {
					t.Fatalf("%s declares deleted legacy replay/event type %s", path, typeSpec.Name.Name)
				}
				if typeSpec.Assign.IsValid() && aliasesGeneratedAPI(typeSpec.Type, generatedImportNames) {
					t.Fatalf("%s aliases generated API type %s; use generated types directly", path, typeSpec.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan handwritten API models: %v", err)
	}
}

func generatedAPIAliases(file *ast.File, generatedImportPaths map[string]struct{}) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if _, ok := generatedImportPaths[importPath]; !ok {
			continue
		}
		name := "generated"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = struct{}{}
	}
	return aliases
}

func aliasesGeneratedAPI(expr ast.Expr, generatedImportNames map[string]struct{}) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = generatedImportNames[ident.Name]
	return ok
}
