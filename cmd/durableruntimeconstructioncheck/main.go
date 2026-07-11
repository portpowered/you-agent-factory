package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const constructorName = "NewJavaScriptRuntimeService"

var approvedProductionFiles = map[string]struct{}{
	"pkg/composebridge/core.go":                  {},
	"pkg/factorysessionexecution/service.go":     {},
	"pkg/service/factory_editable_definition.go": {},
}

type config struct{ root string }

func main() {
	cfg := config{}
	flag.StringVar(&cfg.root, "root", ".", "repository root to scan")
	flag.Parse()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config, stdout, stderr io.Writer) error {
	findings, err := scan(cfg.root)
	if err != nil {
		return err
	}
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "[agent-factory:durable-runtime-construction] direct construction is limited to approved composition owners")
		return nil
	}
	for _, finding := range findings {
		fmt.Fprintln(stderr, finding)
	}
	return fmt.Errorf("[agent-factory:durable-runtime-construction] found %d prohibited constructor call(s)", len(findings))
}

func scan(root string) ([]string, error) {
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	var findings []string
	err = filepath.WalkDir(repoRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != repoRoot && ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, approved := approvedProductionFiles[relative]; approved {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", relative, err)
		}
		if ast.IsGenerated(file) {
			return nil
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || calledName(call.Fun) != constructorName {
				return true
			}
			position := fileSet.Position(call.Pos())
			findings = append(findings, fmt.Sprintf("[agent-factory:durable-runtime-construction] %s:%d calls %s; construct durable execution in an approved composition owner", relative, position.Line, constructorName))
			return true
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan repository: %w", err)
	}
	sort.Strings(findings)
	return findings, nil
}

func calledName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "testdata", "coverage", "dist", "build":
		return true
	default:
		return false
	}
}
