package backenddependencygraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Package is the portion of go list metadata needed to build the graph.
type Package struct {
	ImportPath string
	Imports    []string
	Module     *Module
}

// Module identifies the module that owns a package.
type Module struct {
	Path string
	Main bool
}

// Load returns production import relationships for packages below cmd and pkg.
func Load(ctx context.Context, root, goBinary string) ([]Package, string, error) {
	command := exec.CommandContext(ctx, goBinary, "list", "-json", "./cmd/...", "./pkg/...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, "", fmt.Errorf("go list cmd/pkg packages: %w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, "", fmt.Errorf("go list cmd/pkg packages: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	packages := make([]Package, 0)
	modulePath := ""
	for {
		var pkg Package
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, "", fmt.Errorf("decode go list output: %w", err)
		}
		packages = append(packages, pkg)
		if pkg.Module != nil && pkg.Module.Main {
			modulePath = pkg.Module.Path
		}
	}
	if modulePath == "" {
		return nil, "", fmt.Errorf("go list cmd/pkg packages did not identify the main module")
	}
	return packages, modulePath, nil
}

// RenderDOT renders a deterministic graph containing only the supplied packages.
func RenderDOT(packages []Package, modulePath string) []byte {
	packages = slices.Clone(packages)
	slices.SortFunc(packages, func(left, right Package) int {
		return strings.Compare(left.ImportPath, right.ImportPath)
	})

	known := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		known[pkg.ImportPath] = struct{}{}
	}

	var output strings.Builder
	output.WriteString("digraph backend_dependencies {\n")
	output.WriteString("  graph [rankdir=LR, overlap=false, splines=true, bgcolor=\"transparent\"];\n")
	output.WriteString("  node [shape=box, style=\"rounded,filled\", fontname=\"Helvetica\", fontsize=10];\n")
	output.WriteString("  edge [color=\"#64748b\", arrowsize=0.6];\n\n")

	for _, pkg := range packages {
		label := strings.TrimPrefix(pkg.ImportPath, modulePath+"/")
		fillColor := "#dbeafe"
		if strings.HasPrefix(label, "cmd/") {
			fillColor = "#fef3c7"
		}
		fmt.Fprintf(&output, "  %q [label=%q, fillcolor=%q];\n", pkg.ImportPath, label, fillColor)
	}

	output.WriteString("\n")
	for _, pkg := range packages {
		imports := slices.Clone(pkg.Imports)
		slices.Sort(imports)
		for _, imported := range imports {
			if _, ok := known[imported]; !ok {
				continue
			}
			fmt.Fprintf(&output, "  %q -> %q;\n", pkg.ImportPath, imported)
		}
	}
	output.WriteString("}\n")
	return []byte(output.String())
}

// WriteDOT creates parent directories and writes a graph artifact.
func WriteDOT(path string, graph []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dependency graph directory: %w", err)
	}
	if err := os.WriteFile(path, graph, 0o644); err != nil {
		return fmt.Errorf("write dependency graph: %w", err)
	}
	return nil
}

// RenderSVG uses Graphviz to render a DOT artifact as SVG.
func RenderSVG(ctx context.Context, dotBinary, dotPath, svgPath string) error {
	if err := os.MkdirAll(filepath.Dir(svgPath), 0o755); err != nil {
		return fmt.Errorf("create dependency graph directory: %w", err)
	}
	command := exec.CommandContext(ctx, dotBinary, "-Tsvg", dotPath, "-o", svgPath)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("render dependency graph with Graphviz: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
