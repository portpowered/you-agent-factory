package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

const workersServiceImportPath = "github.com/portpowered/infinite-you/pkg/services/workers"

var forbiddenFunctionalWorkersProcessSymbols = map[string]struct{}{
	"CommandRunner":  {},
	"CommandRequest": {},
	"CommandResult":  {},
}

type functionalProcessEdgeFinding struct {
	filePath string
	line     int
	symbol   string
}

func scanFunctionalProcessEdges(repoRoot string) ([]functionalProcessEdgeFinding, error) {
	paths := []string{filepath.Join(repoRoot, "tests", "functional")}
	sharedFake := filepath.Join(repoRoot, "internal", "testutil", "provider_command_runner.go")
	if _, err := os.Stat(sharedFake); err == nil {
		paths = append(paths, sharedFake)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat functional process fake %s: %w", filepath.ToSlash(sharedFake), err)
	}

	var findings []functionalProcessEdgeFinding
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat functional process edge root %s: %w", filepath.ToSlash(path), err)
		}
		if !info.IsDir() {
			fileFindings, scanErr := scanFunctionalProcessEdgeFile(repoRoot, path)
			if scanErr != nil {
				return nil, scanErr
			}
			findings = append(findings, fileFindings...)
			continue
		}
		err = filepath.WalkDir(path, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(filePath) != ".go" {
				return nil
			}
			fileFindings, scanErr := scanFunctionalProcessEdgeFile(repoRoot, filePath)
			if scanErr != nil {
				return scanErr
			}
			findings = append(findings, fileFindings...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan functional process edge root %s: %w", filepath.ToSlash(path), err)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].filePath != findings[j].filePath {
			return findings[i].filePath < findings[j].filePath
		}
		if findings[i].line != findings[j].line {
			return findings[i].line < findings[j].line
		}
		return findings[i].symbol < findings[j].symbol
	})
	return findings, nil
}

func scanFunctionalProcessEdgeFile(repoRoot, filePath string) ([]functionalProcessEdgeFinding, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filePath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse functional process edge file %s: %w", filepath.ToSlash(filePath), err)
	}
	aliases := map[string]struct{}{}
	for _, spec := range parsed.Imports {
		path, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil || path != workersServiceImportPath {
			continue
		}
		alias := "workers"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias != "." && alias != "_" {
			aliases[alias] = struct{}{}
		}
	}
	if len(aliases) == 0 {
		return nil, nil
	}
	relative, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		return nil, fmt.Errorf("relativize functional process edge file %s: %w", filepath.ToSlash(filePath), err)
	}
	var findings []functionalProcessEdgeFinding
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, ok := aliases[identifier.Name]; !ok {
			return true
		}
		if _, ok := forbiddenFunctionalWorkersProcessSymbols[selector.Sel.Name]; !ok {
			return true
		}
		findings = append(findings, functionalProcessEdgeFinding{
			filePath: filepath.ToSlash(relative),
			line:     fileSet.Position(selector.Pos()).Line,
			symbol:   selector.Sel.Name,
		})
		return true
	})
	return findings, nil
}

func writeFunctionalProcessEdgeFindings(writer interface{ Write([]byte) (int, error) }, findings []functionalProcessEdgeFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:pkg-boundary] prohibited functional Workers process port: workers.%s (%s:%d)\n", finding.symbol, finding.filePath, finding.line)
		fmt.Fprintln(writer, "  remediation: inject pkg/platform/process.CommandRunner at edges.Edges and observe domain identity through public Factory Events, Work, or Factory Session projections.")
	}
}
