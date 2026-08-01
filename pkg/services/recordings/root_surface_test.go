package recordings_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestRecordingsRootExposesOneServiceAndNoOperationalFunctions(t *testing.T) {
	root := recordingsRootDirectory(t)
	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read Recordings root: %v", err)
	}

	allowedDirectories := map[string]bool{"internal": true, "transports": true, "wire": true}
	var interfaces []string
	var exportedFunctions []string
	for _, entry := range files {
		if entry.IsDir() {
			if !allowedDirectories[entry.Name()] {
				t.Errorf("unexpected Recordings root directory %q", entry.Name())
			}
			continue
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil && ast.IsExported(declaration.Name.Name) {
					exportedFunctions = append(exportedFunctions, entry.Name()+":"+declaration.Name.Name)
				}
			case *ast.GenDecl:
				if declaration.Tok != token.TYPE {
					continue
				}
				for _, specification := range declaration.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if ok {
						if _, ok := typeSpec.Type.(*ast.InterfaceType); ok {
							interfaces = append(interfaces, entry.Name()+":"+typeSpec.Name.Name)
						}
					}
				}
			}
		}
	}

	sort.Strings(interfaces)
	if want := []string{"contracts.go:Service"}; len(interfaces) != len(want) || interfaces[0] != want[0] {
		t.Fatalf("Recordings root interfaces = %v, want %v", interfaces, want)
	}
	if len(exportedFunctions) != 0 {
		t.Fatalf("Recordings root exported functions = %v, want none", exportedFunctions)
	}
}

func recordingsRootDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
