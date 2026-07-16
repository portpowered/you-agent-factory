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
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

const loggingPackagePath = "github.com/portpowered/infinite-you/pkg/platform/logging"

var approvedFiles = map[string]struct{}{
	"pkg/transports/cli/root.go":                  {},
	"pkg/transports/cli/terminalpolicy/policy.go": {},
	"pkg/platform/logging/logger.go":              {},
}

var prohibitedCalls = map[string]map[string]string{
	loggingPackagePath: {
		"BuildLogger": "accept or propagate an injected logger",
	},
	"go.uber.org/zap": {
		"L":                    "accept or propagate an injected logger instead of consulting the process-global logger",
		"S":                    "accept or propagate an injected logger instead of consulting the process-global logger",
		"NewDevelopment":       "construct the application logger only at the approved composition boundary",
		"NewDevelopmentConfig": "construct the application logger only at the approved composition boundary",
		"NewProduction":        "construct the application logger only at the approved composition boundary",
		"NewProductionConfig":  "construct the application logger only at the approved composition boundary",
		"ReplaceGlobals":       "propagate the application logger instead of mutating process-global logging",
	},
	"log/slog": {
		"Default":    "accept or propagate an injected logger instead of consulting the process-global logger",
		"SetDefault": "propagate the application logger instead of mutating process-global logging",
	},
	"log": {
		"Default": "accept or propagate an injected logger instead of consulting the process-global logger",
		"Fatal":   "return an error through the owning boundary instead of using process-global logging",
		"Fatalf":  "return an error through the owning boundary instead of using process-global logging",
		"Fataln":  "return an error through the owning boundary instead of using process-global logging",
		"Panic":   "return an error through the owning boundary instead of using process-global logging",
		"Panicf":  "return an error through the owning boundary instead of using process-global logging",
		"Panicln": "return an error through the owning boundary instead of using process-global logging",
		"Print":   "accept or propagate an injected logger instead of using process-global logging",
		"Printf":  "accept or propagate an injected logger instead of using process-global logging",
		"Println": "accept or propagate an injected logger instead of using process-global logging",
	},
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
		fmt.Fprintln(stdout, "[agent-factory:logging-boundary] production logging uses injected loggers")
		return nil
	}
	for _, finding := range findings {
		fmt.Fprintln(stderr, finding)
	}
	return fmt.Errorf("[agent-factory:logging-boundary] found %d prohibited logger acquisition(s)", len(findings))
}

func scan(root string) ([]string, error) {
	repoRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	var findings []string
	for _, sourceRoot := range []string{"cmd", "internal", "pkg"} {
		rootPath := filepath.Join(repoRoot, sourceRoot)
		if _, statErr := os.Stat(rootPath); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			return nil, fmt.Errorf("inspect %s: %w", sourceRoot, statErr)
		}
		err = filepath.WalkDir(rootPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if contractguard.ShouldSkipDir(repoRoot, path, "vendor", "testdata") {
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
			if _, ok := approvedFiles[relative]; ok {
				return nil
			}
			fileFindings, err := scanFile(path, relative)
			findings = append(findings, fileFindings...)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", sourceRoot, err)
		}
	}
	sort.Strings(findings)
	return findings, nil
}

func scanFile(path, relative string) ([]string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", relative, err)
	}
	if ast.IsGenerated(file) {
		return nil, nil
	}
	aliases := prohibitedImportAliases(file)
	findings := prohibitedDotImportFindings(fileSet, file, relative)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := aliases[qualifier.Name]
		if !ok {
			return true
		}
		guidance, prohibited := prohibitedCalls[importPath][selector.Sel.Name]
		if prohibited {
			position := fileSet.Position(call.Pos())
			findings = append(findings, fmt.Sprintf(
				"[agent-factory:logging-boundary] %s:%d uses %s.%s; %s",
				relative, position.Line, qualifier.Name, selector.Sel.Name, guidance,
			))
		}
		return true
	})
	return findings, nil
}

func prohibitedDotImportFindings(fileSet *token.FileSet, file *ast.File, relative string) []string {
	var findings []string
	for _, imported := range file.Imports {
		if imported.Name == nil || imported.Name.Name != "." {
			continue
		}
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		if _, prohibited := prohibitedCalls[path]; !prohibited {
			continue
		}
		position := fileSet.Position(imported.Pos())
		findings = append(findings, fmt.Sprintf(
			"[agent-factory:logging-boundary] %s:%d dot-imports %s; use a named import and accept or propagate an injected logger",
			relative, position.Line, path,
		))
	}
	return findings
}

func prohibitedImportAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		if _, prohibited := prohibitedCalls[path]; !prohibited {
			continue
		}
		name := filepath.Base(path)
		if imported.Name != nil {
			name = imported.Name.Name
		}
		if name != "." && name != "_" {
			aliases[name] = path
		}
	}
	return aliases
}
