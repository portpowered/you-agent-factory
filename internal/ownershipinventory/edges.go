package ownershipinventory

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

const repositoryImportPrefix = "github.com/portpowered/infinite-you/"

// ClassifyEdge returns the committed interaction class for one distinct-owner
// edge. Process Edges edges are always construction or external_effect and are
// marked as the architecture exception surface.
func ClassifyEdge(fromOwner, toOwner string) CrossServiceEdge {
	edge := CrossServiceEdge{
		FromOwner: fromOwner,
		ToOwner:   toOwner,
	}
	switch {
	case fromOwner == DestinationEdges || toOwner == DestinationEdges:
		edge.ArchitectureException = true
		if fromOwner == DestinationEdges {
			edge.Class = EdgeClassExternalEffect
		} else {
			edge.Class = EdgeClassConstruction
		}
	case fromOwner == "wire" || fromOwner == "root":
		edge.Class = EdgeClassConstruction
	case fromOwner == "initializer" || toOwner == "initializer":
		edge.Class = EdgeClassLifecycle
	case fromOwner == "transports":
		edge.Class = EdgeClassProtocolComposition
	case toOwner == "platform":
		edge.Class = EdgeClassExternalEffect
	case toOwner == "recordings":
		edge.Class = EdgeClassEvent
	case toOwner == "factory_definitions":
		edge.Class = EdgeClassQuery
	default:
		edge.Class = EdgeClassCommand
	}
	return edge
}

// DiscoverCrossServiceEdges walks production (non-test) Go imports under pkg and
// returns one stable-sorted edge per distinct destination-owner pair.
//
// Ownership is derived from the tree by OwnerForPackage, so every live package
// participates without needing a registry row. The supplied rows only override
// that derivation, which is how an unfinished "move" row keeps attributing a
// package to the owner it is migrating towards rather than the directory it
// still sits in.
func DiscoverCrossServiceEdges(root string, packages []PackageRow) ([]CrossServiceEdge, error) {
	ownerFor := packageOwnerResolver(packages)

	evidenceByPair := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, "pkg"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		fromPackage, err := packagePathForFile(root, path)
		if err != nil {
			return err
		}
		fromOwner, ok := ownerFor(fromPackage)
		if !ok {
			return nil
		}

		imports, err := productionImports(path)
		if err != nil {
			return err
		}
		for _, importPath := range imports {
			toPackage, toOwner, ok := resolveImportedOwner(importPath, ownerFor)
			if !ok || toOwner == fromOwner {
				continue
			}
			key := fromOwner + "\x00" + toOwner
			if _, exists := evidenceByPair[key]; exists {
				continue
			}
			evidenceByPair[key] = fromPackage + " -> " + toPackage
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover cross-service edges: %w", err)
	}

	edges := make([]CrossServiceEdge, 0, len(evidenceByPair))
	for key, evidence := range evidenceByPair {
		parts := strings.Split(key, "\x00")
		edge := ClassifyEdge(parts[0], parts[1])
		edge.Evidence = evidence
		edges = append(edges, edge)
	}
	markUnresolvedBidirectionalEdges(edges)
	slices.SortFunc(edges, compareCrossServiceEdges)
	return edges, nil
}

// markUnresolvedBidirectionalEdges keeps reciprocal owner imports visible as
// convergence debt. A later convergence story may remove the reciprocal
// dependency, but regeneration must not make it disappear merely because each
// direction has an otherwise valid interaction class.
func markUnresolvedBidirectionalEdges(edges []CrossServiceEdge) {
	pairs := make(map[string]struct{}, len(edges))
	for _, edge := range edges {
		pairs[edgePairKey(edge.FromOwner, edge.ToOwner)] = struct{}{}
	}
	for i := range edges {
		reverse := edgePairKey(edges[i].ToOwner, edges[i].FromOwner)
		if _, ok := pairs[reverse]; !ok {
			continue
		}
		edges[i].Bidirectional = true
		edges[i].Unresolved = true
	}
}

func packagePathForFile(root, path string) (string, error) {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func productionImports(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		// Incomplete fixture trees may contain non-buildable stubs; skip them
		// rather than failing the freeze discovery walk.
		return nil, nil
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		if spec.Path == nil {
			continue
		}
		importPath := strings.Trim(spec.Path.Value, `"`)
		if strings.HasPrefix(importPath, repositoryImportPrefix+"pkg/") {
			out = append(out, importPath)
		}
	}
	return out, nil
}

// packageOwnerResolver resolves a production package path to its destination
// owner: an explicit row wins, otherwise the owner is derived from the tree.
func packageOwnerResolver(packages []PackageRow) func(string) (string, bool) {
	overrides := make(map[string]string, len(packages))
	for _, row := range packages {
		overrides[row.PackagePath] = row.Destination
	}
	return func(packagePath string) (string, bool) {
		if owner, ok := overrides[packagePath]; ok {
			return owner, true
		}
		return OwnerForPackage(packagePath)
	}
}

func resolveImportedOwner(importPath string, ownerFor func(string) (string, bool)) (packagePath, owner string, ok bool) {
	candidate, isRepositoryImport := strings.CutPrefix(importPath, repositoryImportPrefix)
	if !isRepositoryImport {
		return "", "", false
	}
	owner, resolved := ownerFor(candidate)
	if !resolved {
		return "", "", false
	}
	return candidate, owner, true
}

func compareCrossServiceEdges(a, b CrossServiceEdge) int {
	if cmp := strings.Compare(a.FromOwner, b.FromOwner); cmp != 0 {
		return cmp
	}
	return strings.Compare(a.ToOwner, b.ToOwner)
}

func isAllowedEdgeClass(class string) bool {
	return slices.Contains(AllowedEdgeClasses, class)
}

func edgePairKey(fromOwner, toOwner string) string {
	return fromOwner + "->" + toOwner
}

func bidirectionalPairKey(fromOwner, toOwner string) string {
	if strings.Compare(fromOwner, toOwner) <= 0 {
		return edgePairKey(fromOwner, toOwner)
	}
	return edgePairKey(toOwner, fromOwner)
}
