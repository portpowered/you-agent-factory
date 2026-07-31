package workers_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecuteContractsExcludeRuntimeNetVocabulary(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	root := filepath.Dir(file)
	sources := []string{
		filepath.Join(root, "execute_contracts.go"),
		filepath.Join(root, "observation_contracts.go"),
	}
	forbidden := []string{
		"Token",
		"PlaceID",
		"Color",
		"Marking",
		"Petri",
		"Transition",
		"WorkerExecutor",
		"WorkstationRequestExecutor",
		"AssembledRuntimeBinding",
		"CurrentRuntime",
		"ProviderSessionMetadata",
		"ProviderInferenceRequest",
		"ProviderRegistry",
		"ProviderError",
	}
	for _, source := range sources {
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, source, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", source, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			name := ident.Name
			for _, item := range forbidden {
				if name == item {
					t.Fatalf("%s declares forbidden runtime vocabulary identifier %q", source, name)
				}
			}
			return true
		})
	}
}
