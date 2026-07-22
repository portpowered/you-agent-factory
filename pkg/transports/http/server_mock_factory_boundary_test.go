package http

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const omnibusHTTPTestFactoryImport = "github.com/portpowered/infinite-you/internal/testutil"

func TestHTTPTestsDoNotAddOmnibusMockFactoryDependencies(t *testing.T) {
	t.Helper()

	found := make(map[string]struct{})
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		usesMockFactory, err := testFileUsesOmnibusMockFactory(path)
		if err != nil {
			return err
		}
		if usesMockFactory {
			found[filepath.ToSlash(path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan HTTP test construction boundaries: %v", err)
	}

	for path := range found {
		t.Errorf("%s constructs internal/testutil.MockFactory; inject an exact public service-root role", path)
	}
}

func TestOmnibusMockFactoryBoundaryDetectsOnlyImportedFactoryConstruction(t *testing.T) {
	t.Parallel()

	write := func(name, source string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write boundary fixture: %v", err)
		}
		return path
	}

	violating := write("violating_test.go", `package fixture
import support "github.com/portpowered/infinite-you/internal/testutil"
var factory = support.MockFactory{}
`)
	got, err := testFileUsesOmnibusMockFactory(violating)
	if err != nil {
		t.Fatalf("scan violating fixture: %v", err)
	}
	if !got {
		t.Fatal("expected imported MockFactory construction to be rejected")
	}

	ownerLocal := write("owner_local_test.go", `package fixture
type MockFactory struct{}
var factory = MockFactory{}
`)
	got, err = testFileUsesOmnibusMockFactory(ownerLocal)
	if err != nil {
		t.Fatalf("scan owner-local fixture: %v", err)
	}
	if got {
		t.Fatal("owner-local type with the same name must not match the omnibus import")
	}
}

func testFileUsesOmnibusMockFactory(path string) (bool, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return false, err
	}
	aliases := make(map[string]struct{})
	for _, imported := range parsed.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil || importPath != omnibusHTTPTestFactoryImport {
			continue
		}
		alias := "testutil"
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		aliases[alias] = struct{}{}
	}
	uses := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "MockFactory" {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		_, uses = aliases[qualifier.Name]
		return !uses
	})
	return uses, nil
}
