package exemptionbudget

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/backendsizecheck"
)

type Scope string

const (
	ScopeAllOwned Scope = "all-owned"
	ScopePackage  Scope = "package"
)

func Discover(root string, scope Scope) ([]Directive, error) {
	if scope != ScopeAllOwned && scope != ScopePackage {
		return nil, fmt.Errorf("unsupported exemption discovery scope %q", scope)
	}
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	var directives []Directive
	for _, inventory := range backendsizecheck.Inventory() {
		if !includedRoot(inventory.Root, scope) {
			continue
		}
		scanRoot := filepath.Join(repoRoot, inventory.Root)
		found, err := discoverRoot(repoRoot, scanRoot)
		if err != nil {
			return nil, err
		}
		directives = append(directives, found...)
	}
	sortDirectives(directives)
	return directives, nil
}

func DiscoverFile(relativePath string, source []byte) ([]Directive, error) {
	if isGeneratedGo(source) {
		return nil, nil
	}

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, relativePath, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relativePath, err)
	}

	var directives []Directive
	directives = appendCommentDirectives(directives, file.Comments, RuleBackendFile, relativePath)
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			continue
		}
		directives = appendCommentDirectives(directives, []*ast.CommentGroup{group}, RulePackageFileLines, relativePath)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Doc == nil {
			continue
		}
		target := relativePath + "#" + functionName(function)
		groups := []*ast.CommentGroup{function.Doc}
		directives = appendCommentDirectives(directives, groups, RuleBackendFunction, target)
		directives = appendCommentDirectives(directives, groups, RulePackageFuncLines, target)
		directives = appendCommentDirectives(directives, groups, RulePackageComplexity, target)
	}
	sortDirectives(directives)
	return directives, nil
}

func discoverRoot(repoRoot, scanRoot string) ([]Directive, error) {
	info, err := os.Stat(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat scan root %s: %w", filepath.ToSlash(scanRoot), err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	var directives []Directive
	err = filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", filepath.ToSlash(path), walkErr)
		}
		if entry.IsDir() {
			if backendsizecheck.ShouldSkipDir(scanRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}

		source, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.ToSlash(path), err)
		}
		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", filepath.ToSlash(path), err)
		}
		found, err := DiscoverFile(filepath.ToSlash(relativePath), source)
		if err != nil {
			return err
		}
		directives = append(directives, found...)
		return nil
	})
	return directives, err
}

func includedRoot(root string, scope Scope) bool {
	switch scope {
	case ScopeAllOwned:
		return root != "vendor"
	case ScopePackage:
		return root == "pkg"
	default:
		return false
	}
}

func appendCommentDirectives(result []Directive, groups []*ast.CommentGroup, rule, target string) []Directive {
	for _, group := range groups {
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"))
			index := strings.Index(text, rule)
			if index < 0 || !directiveBoundary(text, index+len(rule)) {
				continue
			}
			reason := strings.TrimSpace(strings.TrimSuffix(text[index+len(rule):], "*/"))
			result = append(result, Directive{Rule: rule, Target: target, Reason: reason})
		}
	}
	return result
}

func directiveBoundary(text string, index int) bool {
	return index == len(text) || text[index] == ' ' || text[index] == '\t'
}

func functionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return receiverName(function.Recv.List[0].Type) + "." + function.Name.Name
}

func receiverName(expression ast.Expr) string {
	switch node := expression.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.StarExpr:
		return receiverName(node.X)
	case *ast.IndexExpr:
		return receiverName(node.X)
	case *ast.IndexListExpr:
		return receiverName(node.X)
	case *ast.SelectorExpr:
		return node.Sel.Name
	default:
		return "receiver"
	}
}

func isGeneratedGo(source []byte) bool {
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return false
		}
		if strings.HasPrefix(trimmed, "// Code generated ") && strings.HasSuffix(trimmed, " DO NOT EDIT.") {
			return true
		}
	}
	return false
}

func sortDirectives(directives []Directive) {
	slices.SortFunc(directives, func(left, right Directive) int {
		if byRule := strings.Compare(left.Rule, right.Rule); byRule != 0 {
			return byRule
		}
		return strings.Compare(left.Target, right.Target)
	})
}
