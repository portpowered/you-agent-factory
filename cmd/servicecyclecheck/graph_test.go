package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBuildServiceGraphDerivesServicesFromTheDirectoryTree(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t,
		[]string{"alpha", "beta", "lonely"},
		[]fixtureFile{{path: "pkg/services/alpha/client.go", imports: []string{"pkg/services/beta"}}},
	)
	// A stray file and an ignored directory must not become services.
	if err := os.WriteFile(filepath.Join(root, "pkg", "services", "README.md"), []byte("notes"), 0o600); err != nil {
		t.Fatalf("write stray file: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg", "services", "testdata"), 0o755); err != nil {
		t.Fatalf("create ignored directory: %v", err)
	}

	graph, err := buildServiceGraph(root)
	if err != nil {
		t.Fatalf("buildServiceGraph returned an error: %v", err)
	}

	want := []string{"alpha", "beta", "lonely"}
	if !slices.Equal(graph.services, want) {
		t.Fatalf("derived services = %v, want %v (services with no imports still count)", graph.services, want)
	}
}

func TestBuildServiceGraphWeighsEachImportStatementAndSkipsSelfEdges(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t,
		[]string{"alpha", "beta", "gamma"},
		[]fixtureFile{
			{path: "pkg/services/alpha/a.go", imports: []string{
				"pkg/services/beta",
				"pkg/services/beta/internal/store",
				"pkg/services/alpha/internal/self",
				"pkg/platform/logging",
			}},
			{path: "pkg/services/alpha/internal/b.go", imports: []string{"pkg/services/beta/wire"}},
			{path: "pkg/services/gamma/c.go", imports: []string{"pkg/services/beta"}},
		},
	)

	graph, err := buildServiceGraph(root)
	if err != nil {
		t.Fatalf("buildServiceGraph returned an error: %v", err)
	}

	if got := graph.weights[serviceEdge{from: "alpha", to: "beta"}]; got != 3 {
		t.Fatalf("alpha -> beta weight = %d, want 3 (one per crossing import statement)", got)
	}
	if got := graph.weights[serviceEdge{from: "gamma", to: "beta"}]; got != 1 {
		t.Fatalf("gamma -> beta weight = %d, want 1", got)
	}
	if got := graph.weights[serviceEdge{from: "alpha", to: "alpha"}]; got != 0 {
		t.Fatalf("alpha -> alpha weight = %d, want 0 because self-edges are excluded", got)
	}
	if len(graph.weights) != 2 {
		t.Fatalf("recorded %d edge(s), want 2; non-service imports must not create edges: %v", len(graph.weights), graph.weights)
	}
}

func TestBuildServiceGraphRecordsCarrierPackagesAndIgnoresTestFiles(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t,
		[]string{"alpha", "beta"},
		[]fixtureFile{
			{path: "pkg/services/alpha/transports/http/adapter.go", imports: []string{"pkg/services/beta"}},
			{path: "pkg/services/alpha/internal/store.go", imports: []string{"pkg/services/beta/contract"}},
			{path: "pkg/services/alpha/wire/provider_test.go", imports: []string{"pkg/services/beta"}},
			{path: "pkg/services/alpha/testdata/sample/fake.go", imports: []string{"pkg/services/beta"}},
		},
	)

	graph, err := buildServiceGraph(root)
	if err != nil {
		t.Fatalf("buildServiceGraph returned an error: %v", err)
	}

	edge := serviceEdge{from: "alpha", to: "beta"}
	if got := graph.weights[edge]; got != 2 {
		t.Fatalf("alpha -> beta weight = %d, want 2; _test.go and testdata imports must not count", got)
	}
	want := []string{"pkg/services/alpha/internal", "pkg/services/alpha/transports/http"}
	if !slices.Equal(graph.carriers[edge], want) {
		t.Fatalf("carrier packages = %v, want %v", graph.carriers[edge], want)
	}
}

func TestServiceGraphMatrixIndexesBySortedServiceList(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t,
		[]string{"alpha", "beta", "gamma"},
		[]fixtureFile{
			{path: "pkg/services/gamma/a.go", imports: []string{"pkg/services/alpha", "pkg/services/alpha/wire"}},
			{path: "pkg/services/beta/b.go", imports: []string{"pkg/services/gamma"}},
		},
	)

	graph, err := buildServiceGraph(root)
	if err != nil {
		t.Fatalf("buildServiceGraph returned an error: %v", err)
	}

	matrix := graph.matrix()
	if len(matrix) != 3 {
		t.Fatalf("matrix has %d row(s), want 3", len(matrix))
	}
	// services sort to [alpha beta gamma], so gamma -> alpha is (2,0).
	if matrix[2][0] != 2 {
		t.Fatalf("matrix[gamma][alpha] = %d, want 2", matrix[2][0])
	}
	if matrix[1][2] != 1 {
		t.Fatalf("matrix[beta][gamma] = %d, want 1", matrix[1][2])
	}
	if matrix[0][1] != 0 {
		t.Fatalf("matrix[alpha][beta] = %d, want 0", matrix[0][1])
	}
}

func TestBuildServiceGraphReportsAMissingServicesRoot(t *testing.T) {
	t.Parallel()

	if _, err := buildServiceGraph(t.TempDir()); err == nil {
		t.Fatal("expected an error when pkg/services is absent, got none")
	}
}

func TestServiceGraphCutSetNamesBackEdgesWithCarriers(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t, ratchetFixtureServices, ratchetFixtureFiles())
	graph, err := buildServiceGraph(root)
	if err != nil {
		t.Fatalf("buildServiceGraph returned an error: %v", err)
	}
	solution, err := minimumFeedbackArcSet(graph.matrix())
	if err != nil {
		t.Fatalf("minimumFeedbackArcSet returned an error: %v", err)
	}

	if solution.weight != 2 {
		t.Fatalf("fixture minimum feedback arc weight = %d, want 2", solution.weight)
	}
	edges := graph.cutSet(solution.ordering)
	if totalWeight(edges) != solution.weight {
		t.Fatalf("cut set total weight %d does not match the solver weight %d", totalWeight(edges), solution.weight)
	}
	if len(edges) != 2 {
		t.Fatalf("cut set has %d edge(s), want 2: %+v", len(edges), edges)
	}
	for _, edge := range edges {
		if len(edge.carriers) == 0 {
			t.Fatalf("cut edge %s -> %s reported no carrier package", edge.from, edge.to)
		}
	}
}
