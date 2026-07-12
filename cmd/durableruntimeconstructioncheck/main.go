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

const (
	runtimeConstructorName = "NewJavaScriptRuntimeService"
	storeConstructorName   = "NewDirectoryStore"
	storeDirectoryName     = "DirForProjectRoot"
	persistenceBooleanName = "PersistSessions"
	providerInferenceName  = "Infer"
	providerPackagePath    = "github.com/portpowered/infinite-you/pkg/workers/provider"
)

var javascriptLiveChildRoots = []string{
	"pkg/factorysessionexecution/livechild/",
	"pkg/orchestrators/javascript/",
}

var approvedRuntimeConstructorFiles = map[string]struct{}{
	"pkg/factorysessionexecution/service.go":             {},
	"pkg/factorysessionexecution/testharness/harness.go": {},
}

var approvedPersistenceCompositionFiles = map[string]struct{}{
	"pkg/api/servertests/server_durable_session_execution_test.go": {},
	"pkg/cli/mcp/serve_runtime_resume_smoke_test.go":               {},
	"pkg/cli/session/smoke/resume_smoke_test.go":                   {},
	"pkg/factorysessionexecution/service.go":                       {},
	"pkg/factorysessionexecution/runtimepersist/store.go":          {},
	"pkg/factorysessionexecution/testharness/harness.go":           {},
	"pkg/mcp/factorysession/execution_test.go":                     {},
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
		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		relative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasSuffix(entry.Name(), "_test.go") && !isTransportTest(relative) {
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
		if isJavaScriptLiveChildFile(relative) {
			for _, imported := range file.Imports {
				if strings.Trim(imported.Path.Value, `"`) == providerPackagePath {
					appendFinding(&findings, fileSet, imported.Pos(), relative, providerPackagePath,
						"route production live-child provider execution through pkg/workers/providerexecution")
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CallExpr:
				name := calledName(value.Fun)
				if name == providerInferenceName && isJavaScriptLiveChildFile(relative) {
					appendFinding(&findings, fileSet, value.Pos(), relative, name,
						"route production live-child provider invocation through pkg/workers/providerexecution")
				}
				if name == runtimeConstructorName && !approved(relative, approvedRuntimeConstructorFiles) {
					appendFinding(&findings, fileSet, value.Pos(), relative, name,
						"construct durable execution in an approved composition owner")
				}
				if (name == storeConstructorName || name == storeDirectoryName) &&
					!approved(relative, approvedPersistenceCompositionFiles) {
					appendFinding(&findings, fileSet, value.Pos(), relative, name,
						"resolve and construct durable persistence at the approved application composition boundary")
				}
			case *ast.KeyValueExpr:
				if identifierName(value.Key) == persistenceBooleanName {
					appendFinding(&findings, fileSet, value.Pos(), relative, persistenceBooleanName,
						"inject a persistence store or explicit disabled policy instead of a boolean")
				}
			case *ast.CompositeLit:
				if calledName(value.Type) == "DirectoryStore" && !approved(relative, approvedPersistenceCompositionFiles) {
					appendFinding(&findings, fileSet, value.Pos(), relative, "DirectoryStore literal",
						"construct durable persistence at the approved application composition boundary")
				}
			}
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

func isJavaScriptLiveChildFile(relative string) bool {
	for _, root := range javascriptLiveChildRoots {
		if strings.HasPrefix(relative, root) {
			return true
		}
	}
	return false
}

func approved(relative string, files map[string]struct{}) bool {
	_, ok := files[relative]
	return ok
}

func appendFinding(findings *[]string, fileSet *token.FileSet, position token.Pos, relative, name, guidance string) {
	line := fileSet.Position(position).Line
	*findings = append(*findings, fmt.Sprintf(
		"[agent-factory:durable-runtime-construction] %s:%d uses %s; %s",
		relative,
		line,
		name,
		guidance,
	))
}

func isTransportTest(relative string) bool {
	for _, root := range []string{"pkg/api/", "pkg/cli/", "pkg/mcp/"} {
		if strings.HasPrefix(relative, root) {
			return true
		}
	}
	return false
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

func identifierName(expression ast.Expr) string {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func ignoredDirectory(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "testdata", "coverage", "dist", "build":
		return true
	default:
		return false
	}
}
