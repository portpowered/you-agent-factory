package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// repositoryImportPrefix matches the module path declared in go.mod. The
// walking and parsing conventions in this file follow cmd/pkgboundarycheck:
// filepath.WalkDir over the repository tree, parser.ImportsOnly parsing, and
// the shared ignored-directory rules.
const repositoryImportPrefix = "github.com/portpowered/infinite-you/"

// servicesRelativeRoot is the tree whose immediate subdirectories define the
// service list. The list is always derived from this tree at run time; the
// checker never carries a hardcoded roster of service names.
var servicesRelativeRoot = filepath.Join("pkg", "services")

// ignoredWalkDirectoryNames mirrors cmd/pkgboundarycheck's ignored directory
// names so generated, vendored, and fixture trees never contribute edges.
var ignoredWalkDirectoryNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"testdata":     {},
	"vendor":       {},
}

// serviceEdge identifies one directed cross-service import relationship.
type serviceEdge struct {
	from string
	to   string
}

// serviceGraph is the derived weighted directed cross-service import graph.
// Weight is the number of non-test import statements in packages owned by
// `from` that resolve into packages owned by `to`. Self-edges are excluded.
type serviceGraph struct {
	services []string
	weights  map[serviceEdge]int
	carriers map[serviceEdge][]string
}

// matrix renders the graph as a dense adjacency matrix indexed by the sorted
// service list, which is the input form the exact MFAS solver consumes.
func (graph *serviceGraph) matrix() [][]int {
	index := make(map[string]int, len(graph.services))
	for position, service := range graph.services {
		index[service] = position
	}
	matrix := make([][]int, len(graph.services))
	for position := range matrix {
		matrix[position] = make([]int, len(graph.services))
	}
	for edge, weight := range graph.weights {
		from, fromKnown := index[edge.from]
		to, toKnown := index[edge.to]
		if !fromKnown || !toKnown {
			continue
		}
		matrix[from][to] = weight
	}
	return matrix
}

// buildServiceGraph derives the service list and the weighted cross-service
// import graph rooted at repoRoot.
func buildServiceGraph(repoRoot string) (*serviceGraph, error) {
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	servicesRoot := filepath.Join(absoluteRoot, servicesRelativeRoot)
	services, err := discoverServices(servicesRoot)
	if err != nil {
		return nil, err
	}

	graph := &serviceGraph{
		services: services,
		weights:  map[serviceEdge]int{},
		carriers: map[serviceEdge][]string{},
	}
	known := make(map[string]struct{}, len(services))
	for _, service := range services {
		known[service] = struct{}{}
	}
	if err := graph.collectEdges(absoluteRoot, servicesRoot, known); err != nil {
		return nil, err
	}
	graph.sortCarriers()
	return graph, nil
}

// discoverServices lists the immediate subdirectories of pkg/services. Each
// one is a service node, including services that import nothing.
func discoverServices(servicesRoot string) ([]string, error) {
	entries, err := os.ReadDir(servicesRoot)
	if err != nil {
		return nil, fmt.Errorf("read services root %s: %w", filepath.ToSlash(servicesRoot), err)
	}
	services := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ignored := ignoredWalkDirectoryNames[entry.Name()]; ignored {
			continue
		}
		services = append(services, entry.Name())
	}
	slices.Sort(services)
	return services, nil
}

// collectEdges walks every non-test Go file under pkg/services and records one
// unit of weight per import statement that crosses a service boundary.
func (graph *serviceGraph) collectEdges(repoRoot string, servicesRoot string, known map[string]struct{}) error {
	return filepath.WalkDir(servicesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %s: %w", filepath.ToSlash(path), walkErr)
		}
		if entry.IsDir() {
			if _, ignored := ignoredWalkDirectoryNames[entry.Name()]; ignored {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativePath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return fmt.Errorf("resolve relative path for %s: %w", filepath.ToSlash(path), err)
		}
		return graph.collectFileEdges(path, filepath.ToSlash(relativePath), known)
	})
}

// collectFileEdges records the cross-service imports of a single file.
func (graph *serviceGraph) collectFileEdges(path string, relativePath string, known map[string]struct{}) error {
	owner, ok := serviceOwnerOf(relativePath)
	if !ok {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", relativePath, err)
	}
	parsedFile, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("parse %s: %w", relativePath, err)
	}

	carrierPackage := pathDirectory(relativePath)
	for _, importSpec := range parsedFile.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			continue
		}
		target, ok := importedServiceOf(importPath)
		if !ok || target == owner {
			continue
		}
		if _, isService := known[target]; !isService {
			continue
		}
		graph.recordImport(serviceEdge{from: owner, to: target}, carrierPackage)
	}
	return nil
}

// recordImport adds one import statement's worth of weight to an edge and
// remembers the package that carries it.
func (graph *serviceGraph) recordImport(edge serviceEdge, carrierPackage string) {
	graph.weights[edge]++
	if !slices.Contains(graph.carriers[edge], carrierPackage) {
		graph.carriers[edge] = append(graph.carriers[edge], carrierPackage)
	}
}

// sortCarriers makes carrier reporting deterministic across runs.
func (graph *serviceGraph) sortCarriers() {
	for edge := range graph.carriers {
		slices.Sort(graph.carriers[edge])
	}
}

// serviceOwnerOf reports which service owns a repository-relative file path.
func serviceOwnerOf(relativePath string) (string, bool) {
	parts := strings.Split(relativePath, "/")
	if len(parts) < 4 || parts[0] != "pkg" || parts[1] != "services" {
		return "", false
	}
	return parts[2], true
}

// importedServiceOf reports which service an import path resolves into. Both
// the service root package and any nested package count as the same target.
func importedServiceOf(importPath string) (string, bool) {
	const servicesImportPrefix = repositoryImportPrefix + "pkg/services/"
	suffix, ok := strings.CutPrefix(importPath, servicesImportPrefix)
	if !ok || suffix == "" {
		return "", false
	}
	service, _, _ := strings.Cut(suffix, "/")
	if service == "" {
		return "", false
	}
	return service, true
}

// pathDirectory returns the slash-separated parent directory of a path.
func pathDirectory(relativePath string) string {
	index := strings.LastIndex(relativePath, "/")
	if index < 0 {
		return "."
	}
	return relativePath[:index]
}
