package analyzers

import (
	"slices"
	"strings"
)

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
	servicesImportPrefix := modulePrefix + "pkg/services/"
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
