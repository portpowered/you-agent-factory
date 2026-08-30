package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

type rawSpawn struct {
	sourcePath        string
	sourceLine        int
	sourceColumn      int
	enclosingIdentity string
}

// processLauncherCatalog is the checker-owned constructor catalog. The
// catalog intentionally records constructors that create an OS process, not
// every helper whose name contains "process": builtcliacceptance and the
// LocalAI functional fixture are in-process boundaries, while their direct
// os/exec calls remain visible wherever those calls are authored under the
// scanned tree.
var processLauncherCatalog = map[string]map[string]struct{}{
	"os/exec": {
		"Command":        {},
		"CommandContext": {},
	},
}

func scanFunctionalOSSpawns(repoRoot string) ([]spawnSite, error) {
	functionalRoot := filepath.Join(repoRoot, "tests", "functional")
	info, err := os.Stat(functionalRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []spawnSite{}, nil
		}
		return nil, fmt.Errorf("stat functional source root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("functional source root is not a directory: %s", filepath.ToSlash(functionalRoot))
	}

	var raw []rawSpawn
	err = filepath.WalkDir(functionalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk functional source %s: %w", filepath.ToSlash(path), walkErr)
		}
		if entry.IsDir() {
			if shouldSkipFunctionalDirectory(repoRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" {
			return nil
		}
		found, err := scanGoFile(repoRoot, path)
		if err != nil {
			return err
		}
		raw = append(raw, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(raw, func(left, right rawSpawn) int {
		if comparison := strings.Compare(left.sourcePath, right.sourcePath); comparison != 0 {
			return comparison
		}
		if left.sourceLine != right.sourceLine {
			return left.sourceLine - right.sourceLine
		}
		if left.sourceColumn != right.sourceColumn {
			return left.sourceColumn - right.sourceColumn
		}
		return strings.Compare(left.enclosingIdentity, right.enclosingIdentity)
	})
	return materializeSpawnSites(raw), nil
}

func shouldSkipFunctionalDirectory(repoRoot, path string) bool {
	relative, err := filepath.Rel(filepath.Join(repoRoot, "tests", "functional"), path)
	if err != nil {
		return true
	}
	slashPath := filepath.ToSlash(relative)
	if slashPath == "internal/support" || strings.HasPrefix(slashPath, "internal/support/") {
		return true
	}
	parts := strings.Split(slashPath, "/")
	for _, part := range parts {
		if part == "testdata" || part == "vendor" || part == "node_modules" || part == ".git" {
			return true
		}
	}
	return false
}

func scanGoFile(repoRoot, path string) ([]rawSpawn, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read functional source %s: %w", filepath.ToSlash(path), err)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		relative := relativeRepoPath(repoRoot, path)
		return nil, fmt.Errorf("parse functional source %s: %w", relative, err)
	}
	aliases, dotImported := osExecImports(parsed)
	if len(aliases) == 0 && !dotImported {
		return nil, nil
	}
	relative := relativeRepoPath(repoRoot, path)
	return findSpawnCalls(fileSet, parsed, relative, aliases, dotImported), nil
}

func osExecImports(file *ast.File) (map[string]struct{}, bool) {
	aliases := map[string]struct{}{}
	dotImported := false
	constructors := processLauncherCatalog["os/exec"]
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != "os/exec" || len(constructors) == 0 {
			continue
		}
		if spec.Name == nil {
			aliases["exec"] = struct{}{}
			continue
		}
		switch spec.Name.Name {
		case ".":
			dotImported = true
		case "_":
		default:
			aliases[spec.Name.Name] = struct{}{}
		}
	}
	return aliases, dotImported
}

func findSpawnCalls(fileSet *token.FileSet, file *ast.File, sourcePath string, aliases map[string]struct{}, dotImported bool) []rawSpawn {
	var result []rawSpawn
	var nodeStack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			nodeStack = nodeStack[:len(nodeStack)-1]
			return true
		}
		nodeStack = append(nodeStack, node)
		call, ok := node.(*ast.CallExpr)
		if !ok || !isOSExecCommand(call, aliases, dotImported) {
			return true
		}
		position := fileSet.Position(call.Pos())
		result = append(result, rawSpawn{
			sourcePath:        sourcePath,
			sourceLine:        position.Line,
			sourceColumn:      position.Column,
			enclosingIdentity: enclosingFunction(nodeStack),
		})
		return true
	})
	return result
}

func isOSExecCommand(call *ast.CallExpr, aliases map[string]struct{}, dotImported bool) bool {
	constructors := processLauncherCatalog["os/exec"]
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return false
		}
		if _, imported := aliases[ident.Name]; !imported {
			return false
		}
		_, cataloged := constructors[selector.Sel.Name]
		return cataloged
	}
	if ident, ok := call.Fun.(*ast.Ident); ok && dotImported {
		_, cataloged := constructors[ident.Name]
		return cataloged
	}
	return false
}

func enclosingFunction(stack []ast.Node) string {
	for index := len(stack) - 1; index >= 0; index-- {
		if function, ok := stack[index].(*ast.FuncDecl); ok {
			return declaredFunctionIdentity(function)
		}
	}
	return "package-scope"
}

func declaredFunctionIdentity(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return receiverTypeName(function.Recv.List[0].Type) + "." + function.Name.Name
}

func receiverTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.StarExpr:
		return receiverTypeName(value.X)
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return receiverTypeName(value.X)
	case *ast.IndexListExpr:
		return receiverTypeName(value.X)
	default:
		return "receiver"
	}
}

func materializeSpawnSites(raw []rawSpawn) []spawnSite {
	result := make([]spawnSite, 0, len(raw))
	occurrences := map[string]int{}
	for _, item := range raw {
		key := item.sourcePath + "\x00" + item.enclosingIdentity
		occurrences[key]++
		occurrence := occurrences[key]
		result = append(result, spawnSite{
			SiteID:            stableSiteID(item.sourcePath, item.enclosingIdentity, occurrence),
			PackagePath:       filepath.ToSlash(filepath.Dir(item.sourcePath)),
			SourcePath:        item.sourcePath,
			SourceLine:        item.sourceLine,
			EnclosingIdentity: item.enclosingIdentity,
			Occurrence:        occurrence,
		})
	}
	return result
}

func stableSiteID(sourcePath, enclosingIdentity string, occurrence int) string {
	extension := filepath.Ext(sourcePath)
	base := strings.TrimSuffix(sourcePath, extension)
	return fmt.Sprintf("OSSPAWN-%s-%s-%02d", slug(base), slug(enclosingIdentity), occurrence)
}

func slug(value string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func relativeRepoPath(repoRoot, path string) string {
	relative, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}
