package contractguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type constructionSource struct {
	path         string
	importPath   string
	file         *ast.File
	imports      map[string]string
	dotImports   []string
	set          *token.FileSet
	declarations map[ConstructionSymbol]constructionDeclaration
	mutations    map[*ast.Object]bool
}

type constructionDeclaration struct {
	source   *constructionSource
	function *ast.FuncDecl
	typeSpec *ast.TypeSpec
}

type constructionIndex struct {
	sources      []*constructionSource
	declarations map[ConstructionSymbol]constructionDeclaration
}

func loadConstructionIndex(root string) (constructionIndex, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return constructionIndex{}, fmt.Errorf("construction registry: resolve root: %w", err)
	}
	module, err := constructionModule(root)
	if err != nil {
		return constructionIndex{}, err
	}
	index := constructionIndex{declarations: make(map[ConstructionSymbol]constructionDeclaration)}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filepath.Dir(path) == filepath.Clean(root) && entry.Name() != "cmd" && entry.Name() != "internal" && entry.Name() != "pkg" {
				return filepath.SkipDir
			}
			if ShouldSkipDir(root, path) || constructionExcludedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		source := &constructionSource{path: filepath.ToSlash(rel), set: token.NewFileSet()}
		source.importPath = module + "/" + filepath.ToSlash(filepath.Dir(rel))
		if filepath.Dir(rel) == "." {
			source.importPath = module
		}
		source.file, err = parser.ParseFile(source.set, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("construction source %s: parse failed", source.path)
		}
		if ast.IsGenerated(source.file) {
			return nil
		}
		source.imports = constructionImports(source.file)
		for _, spec := range source.file.Imports {
			if spec.Name != nil && spec.Name.Name == "." {
				imported, _ := strconv.Unquote(spec.Path.Value)
				source.dotImports = append(source.dotImports, imported)
			}
		}
		index.sources = append(index.sources, source)
		return index.addDeclarations(source)
	})
	for _, source := range index.sources {
		source.declarations = index.declarations
		source.mutations = constructionValueMutations(source.file)
	}
	return index, err
}

func constructionExcludedDirectory(name string) bool {
	switch name {
	case "vendor", "node_modules", "testdata":
		return true
	default:
		return false
	}
}

func constructionModule(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("construction registry: read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("construction registry: missing module declaration in go.mod")
}

func constructionImports(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	return imports
}

func (index *constructionIndex) addDeclarations(source *constructionSource) error {
	for _, decl := range source.file.Decls {
		switch value := decl.(type) {
		case *ast.FuncDecl:
			symbol := ConstructionSymbol{ImportPath: source.importPath, Name: value.Name.Name}
			if value.Recv != nil {
				symbol.Receiver = constructionReceiver(value.Recv.List[0].Type)
			}
			if err := index.add(symbol, constructionDeclaration{source: source, function: value}); err != nil {
				return err
			}
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				if typ, ok := spec.(*ast.TypeSpec); ok {
					symbol := ConstructionSymbol{ImportPath: source.importPath, Name: typ.Name.Name}
					if err := index.add(symbol, constructionDeclaration{source: source, typeSpec: typ}); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (index *constructionIndex) add(symbol ConstructionSymbol, decl constructionDeclaration) error {
	// Target-specific declarations are not arbitrarily selected. A registry that
	// references an ambiguous declaration must fail validation rather than lie.
	if prior, exists := index.declarations[symbol]; exists {
		index.declarations[symbol] = constructionDeclaration{source: prior.source}
		return nil
	}
	index.declarations[symbol] = decl
	return nil
}

func constructionReceiver(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return constructionReceiver(value.X)
	case *ast.IndexExpr:
		return constructionReceiver(value.X)
	case *ast.IndexListExpr:
		return constructionReceiver(value.X)
	default:
		return ""
	}
}
