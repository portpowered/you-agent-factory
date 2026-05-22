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
	defaultFileLineLimit      = 1000
	defaultFunctionLineLimit  = 100
	defaultCyclomaticLimit    = 15
	pkgRoot                   = "pkg"
	ignoreFileLineDirective   = "pkgmaintcheck:ignore-file-lines"
	ignoreFunctionDirective   = "pkgmaintcheck:ignore-function-lines"
	ignoreComplexityDirective = "pkgmaintcheck:ignore-cyclomatic-complexity"
)

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

type config struct {
	root              string
	fileLineLimit     int
	functionLineLimit int
	cyclomaticLimit   int
}

type violation struct {
	packagePath string
	filePath    string
	function    string
	rule        string
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
	flag.IntVar(&cfg.fileLineLimit, "file-limit", defaultFileLineLimit, "maximum owned pkg Go file line count")
	flag.IntVar(&cfg.functionLineLimit, "function-limit", defaultFunctionLineLimit, "maximum owned pkg Go function line count")
	flag.IntVar(&cfg.cyclomaticLimit, "cyclomatic-limit", defaultCyclomaticLimit, "maximum owned pkg Go function cyclomatic complexity")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout io.Writer, stderr io.Writer) error {
	if cfg.fileLineLimit <= 0 {
		return fmt.Errorf("file limit must be positive, got %d", cfg.fileLineLimit)
	}
	if cfg.functionLineLimit <= 0 {
		return fmt.Errorf("function limit must be positive, got %d", cfg.functionLineLimit)
	}
	if cfg.cyclomaticLimit <= 0 {
		return fmt.Errorf("cyclomatic limit must be positive, got %d", cfg.cyclomaticLimit)
	}

	violations, err := scanRepo(cfg)
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		fmt.Fprintf(stdout, "[agent-factory:pkg-maint] pkg maintainability passed (file lines <= %d, function lines <= %d, cyclomatic complexity <= %d)\n", cfg.fileLineLimit, cfg.functionLineLimit, cfg.cyclomaticLimit)
		return nil
	}

	for _, finding := range violations {
		if finding.function == "" {
			fmt.Fprintf(stderr, "%s | rule=%s target=%s actual=%d limit=%d\n", finding.packagePath, finding.rule, finding.filePath, finding.actual, finding.limit)
			continue
		}
		fmt.Fprintf(stderr, "%s | rule=%s target=%s file=%s actual=%d limit=%d\n", finding.packagePath, finding.rule, finding.function, finding.filePath, finding.actual, finding.limit)
	}
	return fmt.Errorf("[agent-factory:pkg-maint] found %d maintainability violation(s)", len(violations))
}

func scanRepo(cfg config) ([]violation, error) {
	repoRoot, err := filepath.Abs(cfg.root)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	scanRoot := filepath.Join(repoRoot, pkgRoot)
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

		fileFindings, err := scanFile(repoRoot, path, cfg)
		if err != nil {
			return err
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	slices.SortFunc(findings, func(left, right violation) int {
		if byPackage := strings.Compare(left.packagePath, right.packagePath); byPackage != 0 {
			return byPackage
		}
		if byFile := strings.Compare(left.filePath, right.filePath); byFile != 0 {
			return byFile
		}
		if byFunction := strings.Compare(left.function, right.function); byFunction != 0 {
			return byFunction
		}
		return strings.Compare(left.rule, right.rule)
	})
	return findings, nil
}

func scanFile(repoRoot string, filePath string, cfg config) ([]violation, error) {
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

	fileSet := token.NewFileSet()
	parsedFile, err := parser.ParseFile(fileSet, filePath, source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relativePath, err)
	}

	var findings []violation
	lineCount := countLines(source)
	if lineCount > cfg.fileLineLimit && !hasIgnoreDirective(parsedFile.Comments, ignoreFileLineDirective) {
		findings = append(findings, violation{
			packagePath: packagePath,
			filePath:    relativePath,
			rule:        "file-lines",
			actual:      lineCount,
			limit:       cfg.fileLineLimit,
		})
	}

	for _, decl := range parsedFile.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}

		if functionLineCount := functionLineCount(fileSet, function); functionLineCount > cfg.functionLineLimit &&
			!hasIgnoreDirective(functionCommentGroups(function), ignoreFunctionDirective) {
			findings = append(findings, violation{
				packagePath: packagePath,
				filePath:    relativePath,
				function:    functionName(function),
				rule:        "function-lines",
				actual:      functionLineCount,
				limit:       cfg.functionLineLimit,
			})
		}

		complexity := cyclomaticComplexity(function)
		if complexity > cfg.cyclomaticLimit && !hasIgnoreDirective(functionCommentGroups(function), ignoreComplexityDirective) {
			findings = append(findings, violation{
				packagePath: packagePath,
				filePath:    relativePath,
				function:    functionName(function),
				rule:        "cyclomatic-complexity",
				actual:      complexity,
				limit:       cfg.cyclomaticLimit,
			})
		}
	}

	return findings, nil
}

func functionCommentGroups(function *ast.FuncDecl) []*ast.CommentGroup {
	if function.Doc == nil {
		return nil
	}
	return []*ast.CommentGroup{function.Doc}
}

func functionLineCount(fileSet *token.FileSet, function *ast.FuncDecl) int {
	return fileSet.Position(function.End()).Line - fileSet.Position(function.Pos()).Line + 1
}

func cyclomaticComplexity(function *ast.FuncDecl) int {
	if function.Body == nil {
		return 0
	}

	complexity := 1
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.BinaryExpr:
			if typed.Op == token.LAND || typed.Op == token.LOR {
				complexity++
			}
		case *ast.CaseClause:
			complexity++
		case *ast.CommClause:
			complexity++
		}
		return true
	})
	return complexity
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
