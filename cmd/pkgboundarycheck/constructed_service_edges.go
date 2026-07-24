package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const processEdgesImportPath = "github.com/portpowered/infinite-you/pkg/services/edges"

type constructedServiceEdgesFinding struct {
	filePath string
	line     int
	kind     string
	detail   string
}

func scanConstructedServiceEdges(repoRoot string) ([]constructedServiceEdgesFinding, error) {
	servicesRoot := filepath.Join(repoRoot, "pkg", "services")
	if _, err := os.Stat(servicesRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat constructed-service root: %w", err)
	}

	var findings []constructedServiceEdgesFinding
	err := filepath.WalkDir(servicesRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == servicesRoot {
				return nil
			}
			relative, relErr := filepath.Rel(servicesRoot, path)
			if relErr != nil {
				return relErr
			}
			// pkg/services/edges owns the process-edge aggregator exception.
			slashRelative := filepath.ToSlash(relative)
			if slashRelative == "edges" || strings.HasPrefix(slashRelative, "edges/") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		fileFindings, scanErr := scanConstructedServiceEdgesFile(repoRoot, path)
		if scanErr != nil {
			return scanErr
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan constructed-service Edges usage: %w", err)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].filePath != findings[j].filePath {
			return findings[i].filePath < findings[j].filePath
		}
		if findings[i].line != findings[j].line {
			return findings[i].line < findings[j].line
		}
		if findings[i].kind != findings[j].kind {
			return findings[i].kind < findings[j].kind
		}
		return findings[i].detail < findings[j].detail
	})
	return findings, nil
}

func scanConstructedServiceEdgesFile(repoRoot, filePath string) ([]constructedServiceEdgesFinding, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, filePath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse constructed-service file %s: %w", filepath.ToSlash(filePath), err)
	}

	aliases := map[string]struct{}{}
	var importFinding *constructedServiceEdgesFinding
	for _, spec := range parsed.Imports {
		path, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil || path != processEdgesImportPath {
			continue
		}
		alias := "edges"
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias != "_" {
			aliases[alias] = struct{}{}
		}
		relative, relErr := filepath.Rel(repoRoot, filePath)
		if relErr != nil {
			return nil, fmt.Errorf("relativize constructed-service file %s: %w", filepath.ToSlash(filePath), relErr)
		}
		finding := constructedServiceEdgesFinding{
			filePath: filepath.ToSlash(relative),
			line:     fileSet.Position(spec.Path.Pos()).Line,
			kind:     "import",
			detail:   processEdgesImportPath,
		}
		importFinding = &finding
	}
	if importFinding == nil {
		return nil, nil
	}

	relative := importFinding.filePath
	var dependencyFindings []constructedServiceEdgesFinding
	ast.Inspect(parsed, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok {
			return true
		}
		name := fieldName(field)
		if name == "" {
			name = "unnamed"
		}
		if usesProcessEdgesType(field.Type, aliases) {
			dependencyFindings = append(dependencyFindings, constructedServiceEdgesFinding{
				filePath: relative,
				line:     fileSet.Position(field.Pos()).Line,
				kind:     "dependency",
				detail:   name,
			})
		}
		return true
	})
	if len(dependencyFindings) > 0 {
		return dependencyFindings, nil
	}
	return []constructedServiceEdgesFinding{*importFinding}, nil
}

func fieldName(field *ast.Field) string {
	if len(field.Names) == 0 {
		return ""
	}
	names := make([]string, 0, len(field.Names))
	for _, name := range field.Names {
		if name == nil || name.Name == "" || name.Name == "_" {
			continue
		}
		names = append(names, name.Name)
	}
	return strings.Join(names, ",")
}

func usesProcessEdgesType(expr ast.Expr, aliases map[string]struct{}) bool {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return usesProcessEdgesType(typed.X, aliases)
	case *ast.SelectorExpr:
		identifier, ok := typed.X.(*ast.Ident)
		if !ok || typed.Sel == nil || typed.Sel.Name != "Edges" {
			return false
		}
		_, allowed := aliases[identifier.Name]
		return allowed
	case *ast.Ident:
		// Dot-import of edges exposes Edges as a bare identifier.
		_, ok := aliases["."]
		return typed.Name == "Edges" && ok
	default:
		return false
	}
}

func writeConstructedServiceEdgesFindings(writer io.Writer, findings []constructedServiceEdgesFinding) {
	for _, finding := range findings {
		switch finding.kind {
		case "dependency":
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited constructed-service Edges dependency: edges.Edges %s (%s:%d)\n",
				finding.detail,
				finding.filePath,
				finding.line,
			)
		default:
			fmt.Fprintf(
				writer,
				"[agent-factory:pkg-boundary] prohibited constructed-service Edges import: %s (%s:%d)\n",
				finding.detail,
				finding.filePath,
				finding.line,
			)
		}
		fmt.Fprintln(writer, "  reason: constructed service implementations must not import or hold the broad edges.Edges bag.")
		fmt.Fprintln(writer, "  remediation: inject exact external-effect ports projected at pkg/wire / root.BuildProcess; keep pkg/services/edges as the process-edge aggregator, not a service locator.")
	}
}
