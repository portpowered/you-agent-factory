package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/backendsizecheck"
)

const (
	defaultFileLineLimit     = 1000
	defaultFunctionLineLimit = 100
	ignoreFileDirective      = "backendsizecheck:ignore-file"
	ignoreFuncDirective      = "backendsizecheck:ignore-function"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root          string
	fileLineLimit int
	funcLineLimit int
}

type violation struct {
	packagePath string
	filePath    string
	function    string
	actual      int
	limit       int
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, stdoutWriter, stderrWriter); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.IntVar(&cfg.fileLineLimit, "file-limit", defaultFileLineLimit, "maximum owned Go file line count")
	flag.IntVar(&cfg.funcLineLimit, "function-limit", defaultFunctionLineLimit, "maximum owned Go function line count")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	if cfg.fileLineLimit <= 0 {
		return fmt.Errorf("file limit must be positive, got %d", cfg.fileLineLimit)
	}
	if cfg.funcLineLimit <= 0 {
		return fmt.Errorf("function limit must be positive, got %d", cfg.funcLineLimit)
	}

	violations, err := scanRepo(cfg.root, cfg.fileLineLimit, cfg.funcLineLimit)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		fmt.Fprintf(stdout, "[agent-factory:backend-size] all owned Go backend files are within file limit %d and function limit %d\n", cfg.fileLineLimit, cfg.funcLineLimit)
		return nil
	}

	for _, finding := range violations {
		if finding.function == "" {
			fmt.Fprintf(stderr, "%s | file %s has %d lines (limit %d)\n", finding.packagePath, finding.filePath, finding.actual, finding.limit)
			continue
		}
		fmt.Fprintf(stderr, "%s | function %s in %s has %d lines (limit %d)\n", finding.packagePath, finding.function, finding.filePath, finding.actual, finding.limit)
	}
	return fmt.Errorf("[agent-factory:backend-size] found %d size violation(s)", len(violations))
}

func scanRepo(root string, fileLineLimit int, funcLineLimit int) ([]violation, error) {
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	var findings []violation
	for _, entry := range backendsizecheck.Inventory() {
		rootPath := filepath.Join(repoRoot, entry.Root)
		collected, err := scanInventoryRoot(repoRoot, rootPath, fileLineLimit, funcLineLimit)
		if err != nil {
			return nil, err
		}
		findings = append(findings, collected...)
	}

	slices.SortFunc(findings, func(left, right violation) int {
		if byPackage := strings.Compare(left.packagePath, right.packagePath); byPackage != 0 {
			return byPackage
		}
		if byFile := strings.Compare(left.filePath, right.filePath); byFile != 0 {
			return byFile
		}
		return strings.Compare(left.function, right.function)
	})
	return findings, nil
}

func scanInventoryRoot(repoRoot string, scanRoot string, fileLineLimit int, funcLineLimit int) ([]violation, error) {
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

	var findings []violation
	walkErr := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", filepath.ToSlash(path), err)
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

		fileFindings, err := scanFile(repoRoot, path, fileLineLimit, funcLineLimit)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return findings, nil
}

func scanFile(repoRoot string, filePath string, fileLineLimit int, funcLineLimit int) ([]violation, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.ToSlash(filePath), err)
	}

	relativePath, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve relative path for %s: %w", filepath.ToSlash(filePath), err)
	}
	relativePath = filepath.ToSlash(relativePath)
	packagePath := filepath.ToSlash(filepath.Dir(relativePath))

	var findings []violation
	lineCount := countLines(source)
	if lineCount > fileLineLimit {
		findings = append(findings, violation{
			packagePath: packagePath,
			filePath:    relativePath,
			actual:      lineCount,
			limit:       fileLineLimit,
		})
	}

	fileSet := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fileSet, filePath, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relativePath, err)
	}
	if hasIgnoreDirective(parsedFile.Comments, ignoreFileDirective) {
		return nil, nil
	}

	for _, decl := range parsedFile.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if function.Doc != nil && hasIgnoreDirective([]*ast.CommentGroup{function.Doc}, ignoreFuncDirective) {
			continue
		}
		actual := fileSet.Position(function.End()).Line - fileSet.Position(function.Pos()).Line + 1
		if actual <= funcLineLimit {
			continue
		}
		findings = append(findings, violation{
			packagePath: packagePath,
			filePath:    relativePath,
			function:    functionName(function),
			actual:      actual,
			limit:       funcLineLimit,
		})
	}

	return findings, nil
}

func functionName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return receiverName(function.Recv.List[0].Type) + "." + function.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch node := expr.(type) {
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

func countLines(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	return strings.Count(string(source), "\n") + 1
}

func hasIgnoreDirective(groups []*ast.CommentGroup, directive string) bool {
	for _, group := range groups {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, directive) {
				return true
			}
		}
	}
	return false
}
