package sessionexecution_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredSessionExecutionSurfaceCannotReturn(t *testing.T) {
	retiredFiles := []string{"async.go", "normalize.go", "result.go", "source.go", "status.go", "types.go"}
	for _, name := range retiredFiles {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("retired session-execution source %s exists", name)
		}
	}

	retiredIdentifiers := map[string]struct{}{
		"StartConfig": {}, "RunConfig": {}, "ExecutionMode": {}, "SourceFileSystem": {},
		"RunSync": {}, "RunAsync": {}, "RunStatus": {}, "RunResult": {}, "NormalizeStartRequest": {},
	}
	retiredMappingIdentifiers := map[string]struct{}{"CLIStartInput": {}, "StartRequestFromCLI": {}}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob session-execution sources: %v", err)
	}
	mappingFiles, err := filepath.Glob(filepath.Join("..", "..", "mapping", "factorysession", "*.go"))
	if err != nil {
		t.Fatalf("glob Factory Session mapping sources: %v", err)
	}
	files = append(files, mappingFiles...)
	for _, path := range files {
		if filepath.Ext(path) != ".go" || len(path) >= 8 && path[len(path)-8:] == "_test.go" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		localSurface := filepath.Dir(path) == "."
		ast.Inspect(parsed, func(node ast.Node) bool {
			if declaration, ok := node.(*ast.FuncDecl); ok && declaration.Recv == nil && ast.IsExported(declaration.Name.Name) && filepath.Dir(path) == "." && declaration.Name.Name != "RunNormalizedSync" {
				t.Errorf("session-execution exports %s; want only RunNormalizedSync", declaration.Name.Name)
			}
			identifier, ok := node.(*ast.Ident)
			if ok {
				_, retired := retiredIdentifiers[identifier.Name]
				if !localSurface {
					_, retired = retiredMappingIdentifiers[identifier.Name]
				}
				if retired {
					t.Errorf("retired identifier %s reappeared in %s", identifier.Name, path)
				}
			}
			return true
		})
	}
}
