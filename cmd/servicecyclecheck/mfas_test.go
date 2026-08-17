package main

import (
	"slices"
	"strings"
	"testing"
)

// feedbackWeightOfOrdering scores an ordering the way the definition does:
// total weight of arcs whose source is placed after their target.
func feedbackWeightOfOrdering(weights [][]int, ordering []int) int {
	positions := orderedPositions(ordering)
	total := 0
	for from, row := range weights {
		for to, weight := range row {
			if positions[from] > positions[to] {
				total += weight
			}
		}
	}
	return total
}

// bruteForceFeedbackArcWeight enumerates every ordering. It is exponentially
// slower than the subset dynamic program but obviously correct, so it is the
// independent oracle the solver is checked against.
func bruteForceFeedbackArcWeight(weights [][]int) int {
	count := len(weights)
	ordering := make([]int, count)
	for index := range ordering {
		ordering[index] = index
	}
	best := -1
	var permute func(depth int)
	permute = func(depth int) {
		if depth == count {
			score := feedbackWeightOfOrdering(weights, ordering)
			if best < 0 || score < best {
				best = score
			}
			return
		}
		for index := depth; index < count; index++ {
			ordering[depth], ordering[index] = ordering[index], ordering[depth]
			permute(depth + 1)
			ordering[depth], ordering[index] = ordering[index], ordering[depth]
		}
	}
	permute(0)
	if best < 0 {
		return 0
	}
	return best
}

// greedyFeedbackArcWeight is the cheapest-next ("nearest neighbor") ordering
// heuristic: repeatedly append whichever unplaced vertex adds the least
// backward weight right now, breaking ties towards the lowest index. It is the
// kind of heuristic this checker must not use, because it optimizes each step
// in isolation and has no bound on how far off the optimum it can land.
func greedyFeedbackArcWeight(weights [][]int) int {
	count := len(weights)
	placed := 0
	total := 0
	for range count {
		bestVertex := -1
		bestCost := 0
		for vertex := range count {
			if placed&(1<<vertex) != 0 {
				continue
			}
			cost := backwardWeightInto(weights[vertex], placed)
			if bestVertex < 0 || cost < bestCost {
				bestVertex, bestCost = vertex, cost
			}
		}
		placed |= 1 << bestVertex
		total += bestCost
	}
	return total
}

// greedyTrapGraph is hand-checkable. Vertices 1, 2 and 3 each point at vertex 0
// with weight 3, and the only other arcs are the light chain 0->1->2->3 with
// weight 1 each. Placing 0 last costs just the single light arc 0->1, so the
// optimum is 1. The cheapest-next heuristic starts from an empty prefix where
// every vertex looks free, appends vertex 0 first, and then pays 3 for each of
// the three heavy arcs it stranded behind it, for a total of 9.
func greedyTrapGraph() [][]int {
	weights := make([][]int, 4)
	for index := range weights {
		weights[index] = make([]int, 4)
	}
	weights[1][0] = 3
	weights[2][0] = 3
	weights[3][0] = 3
	weights[0][1] = 1
	weights[1][2] = 1
	weights[2][3] = 1
	return weights
}

// completeDigraph has every ordered pair of distinct vertices joined with unit
// weight. Every ordering leaves exactly one arc of each unordered pair pointing
// backwards, so the optimum is the closed form count * (count-1) / 2.
func completeDigraph(count int) [][]int {
	weights := make([][]int, count)
	for from := range weights {
		weights[from] = make([]int, count)
		for to := range weights[from] {
			if from != to {
				weights[from][to] = 1
			}
		}
	}
	return weights
}

func TestMinimumFeedbackArcSetReturnsTrueOptimumWhereGreedyDoesNot(t *testing.T) {
	t.Parallel()

	weights := greedyTrapGraph()
	solution, err := minimumFeedbackArcSet(weights)
	if err != nil {
		t.Fatalf("minimumFeedbackArcSet returned an error: %v", err)
	}

	if solution.weight != 1 {
		t.Fatalf("exact minimum feedback arc weight = %d, want 1", solution.weight)
	}
	if greedy := greedyFeedbackArcWeight(weights); greedy != 9 {
		t.Fatalf("cheapest-next greedy weight = %d, want 9 so the trap still discriminates", greedy)
	}
	if got := feedbackWeightOfOrdering(weights, solution.ordering); got != solution.weight {
		t.Fatalf("reported ordering %v scores %d, but the reported weight is %d", solution.ordering, got, solution.weight)
	}
	if want := []int{1, 2, 3, 0}; !slices.Equal(solution.ordering, want) {
		t.Fatalf("optimal ordering = %v, want %v", solution.ordering, want)
	}
}

func TestMinimumFeedbackArcSetMatchesBruteForceOptimum(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		weights [][]int
	}{
		{
			name:    "acyclic chain has no back edges",
			weights: [][]int{{0, 4, 7}, {0, 0, 5}, {0, 0, 0}},
		},
		{
			name:    "unit three cycle costs one arc",
			weights: [][]int{{0, 1, 0}, {0, 0, 1}, {1, 0, 0}},
		},
		{
			name:    "weighted three cycle cuts the cheapest arc",
			weights: [][]int{{0, 9, 0}, {0, 0, 6}, {2, 0, 0}},
		},
		{
			name:    "greedy trap",
			weights: greedyTrapGraph(),
		},
		{
			name: "two arc disjoint two cycles must both be cut",
			weights: [][]int{
				{0, 3, 0, 0},
				{5, 0, 0, 0},
				{0, 0, 0, 2},
				{0, 0, 8, 0},
			},
		},
		{
			name:    "complete digraph on five vertices",
			weights: completeDigraph(5),
		},
		{
			name: "dense mixed weights on six vertices",
			weights: [][]int{
				{0, 2, 0, 1, 0, 4},
				{3, 0, 5, 0, 0, 0},
				{0, 1, 0, 7, 2, 0},
				{6, 0, 0, 0, 3, 1},
				{0, 4, 1, 0, 0, 2},
				{1, 0, 3, 0, 5, 0},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			solution, err := minimumFeedbackArcSet(testCase.weights)
			if err != nil {
				t.Fatalf("minimumFeedbackArcSet returned an error: %v", err)
			}
			if want := bruteForceFeedbackArcWeight(testCase.weights); solution.weight != want {
				t.Fatalf("minimum feedback arc weight = %d, brute-force optimum = %d", solution.weight, want)
			}
			if got := feedbackWeightOfOrdering(testCase.weights, solution.ordering); got != solution.weight {
				t.Fatalf("reported ordering %v scores %d, but the reported weight is %d", solution.ordering, got, solution.weight)
			}
		})
	}
}

func TestMinimumFeedbackArcSetOnAcyclicGraphIsZero(t *testing.T) {
	t.Parallel()

	weights := [][]int{
		{0, 5, 3, 9},
		{0, 0, 4, 2},
		{0, 0, 0, 6},
		{0, 0, 0, 0},
	}
	solution, err := minimumFeedbackArcSet(weights)
	if err != nil {
		t.Fatalf("minimumFeedbackArcSet returned an error: %v", err)
	}
	if solution.weight != 0 {
		t.Fatalf("already-acyclic graph reported weight %d, want 0", solution.weight)
	}
}

func TestMinimumFeedbackArcSetOnCompleteDigraphMatchesClosedForm(t *testing.T) {
	t.Parallel()

	for count := 2; count <= 7; count++ {
		weights := completeDigraph(count)
		solution, err := minimumFeedbackArcSet(weights)
		if err != nil {
			t.Fatalf("minimumFeedbackArcSet(%d vertices) returned an error: %v", count, err)
		}
		if want := count * (count - 1) / 2; solution.weight != want {
			t.Fatalf("complete digraph on %d vertices reported weight %d, want %d", count, solution.weight, want)
		}
	}
}

func TestMinimumFeedbackArcSetIsDeterministic(t *testing.T) {
	t.Parallel()

	weights := completeDigraph(6)
	first, err := minimumFeedbackArcSet(weights)
	if err != nil {
		t.Fatalf("first run returned an error: %v", err)
	}
	second, err := minimumFeedbackArcSet(weights)
	if err != nil {
		t.Fatalf("second run returned an error: %v", err)
	}
	if !slices.Equal(first.ordering, second.ordering) {
		t.Fatalf("repeated runs reported different orderings: %v then %v", first.ordering, second.ordering)
	}
}

func TestMinimumFeedbackArcSetRefusesGraphsAboveTheSafeBound(t *testing.T) {
	t.Parallel()

	oversized := completeDigraph(maxSafeServiceCount + 1)
	if _, err := minimumFeedbackArcSet(oversized); err == nil {
		t.Fatal("expected an explicit error above the safe bound, got none")
	} else if !strings.Contains(err.Error(), "refuses to run") || !strings.Contains(err.Error(), "heuristic") {
		t.Fatalf("error must tell a maintainer to raise the bound rather than accept a heuristic, got: %v", err)
	}

	atBound := completeDigraph(maxSafeServiceCount)
	if _, err := minimumFeedbackArcSet(atBound); err != nil {
		t.Fatalf("the safe bound itself must still be solvable, got: %v", err)
	}
}

func TestMinimumFeedbackArcSetRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	if _, err := minimumFeedbackArcSet([][]int{{0, 1}, {1}}); err == nil {
		t.Fatal("expected an error for a non-square weight matrix, got none")
	}
	if _, err := minimumFeedbackArcSet([][]int{{0, -1}, {0, 0}}); err == nil {
		t.Fatal("expected an error for a negative weight, got none")
	}
	solution, err := minimumFeedbackArcSet(nil)
	if err != nil {
		t.Fatalf("empty graph returned an error: %v", err)
	}
	if solution.weight != 0 || len(solution.ordering) != 0 {
		t.Fatalf("empty graph reported %+v, want zero weight and an empty ordering", solution)
	}
}
