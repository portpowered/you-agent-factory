package main

import (
	"fmt"
	"math"
	"math/bits"
)

// maxSafeServiceCount bounds the subset dynamic program. The state space is
// 2^N, so the exact solver stays cheap well past the current service count but
// would become unreasonable if the tree grew without anyone noticing. Crossing
// the bound is a deliberate maintainer decision, so the solver refuses to run
// rather than silently degrading to a heuristic whose answer would not be
// comparable against a recorded ceiling.
const maxSafeServiceCount = 20

// feedbackArcSolution is the exact minimum feedback arc set of a weighted
// directed graph: the smallest total edge weight that has to be removed to
// make the graph acyclic, plus one linear ordering that realizes it.
type feedbackArcSolution struct {
	weight   int
	ordering []int
}

// minimumFeedbackArcSet computes the exact minimum feedback arc weight of a
// weighted directed graph given as a dense adjacency matrix.
//
// Every acyclic subgraph corresponds to a linear ordering of the vertices, and
// the arcs that must be removed for a given ordering are exactly the arcs
// pointing backwards in it. The minimum feedback arc weight is therefore the
// minimum, over all orderings, of the total backward arc weight.
//
// The minimum is found by dynamic programming over subsets: state S is the set
// of vertices already placed in the ordering's prefix, and dp[S] is the least
// backward weight induced inside that prefix. Placing vertex v immediately
// after every vertex in S turns exactly the arcs v -> u (u in S) into backward
// arcs, so dp[S|{v}] = min over v not in S of dp[S] + sum of weights(v, u) for
// u in S. dp over the full set is the exact optimum. This is a true optimum
// over all N! orderings at roughly 2^N * N cost -- not a greedy or
// nearest-neighbor approximation, both of which can be arbitrarily worse.
//
// Ties are broken towards the lowest vertex index, so repeated runs on the
// same input report the same ordering and therefore the same cut set.
func minimumFeedbackArcSet(weights [][]int) (feedbackArcSolution, error) {
	count := len(weights)
	if count > maxSafeServiceCount {
		return feedbackArcSolution{}, fmt.Errorf(
			"exact minimum feedback arc set refuses to run on %d vertices: the subset dynamic program is bounded at %d; raise maxSafeServiceCount deliberately after confirming the 2^N state space is still affordable, and never substitute a heuristic, because a heuristic result is not comparable against the recorded ceiling",
			count,
			maxSafeServiceCount,
		)
	}
	if err := validateSquareMatrix(weights); err != nil {
		return feedbackArcSolution{}, err
	}
	if count == 0 {
		return feedbackArcSolution{weight: 0, ordering: nil}, nil
	}

	best, predecessor := solveOrderingDP(weights)
	full := (1 << count) - 1
	return feedbackArcSolution{
		weight:   best[full],
		ordering: reconstructOrdering(predecessor, count),
	}, nil
}

// validateSquareMatrix rejects malformed input before the dynamic program
// indexes into it.
func validateSquareMatrix(weights [][]int) error {
	for row, columns := range weights {
		if len(columns) != len(weights) {
			return fmt.Errorf("weight matrix row %d has %d column(s), want %d", row, len(columns), len(weights))
		}
		for column, weight := range columns {
			if weight < 0 {
				return fmt.Errorf("weight matrix entry (%d,%d) is negative: %d", row, column, weight)
			}
		}
	}
	return nil
}

// solveOrderingDP fills the subset table and records, for each state, which
// vertex was placed last on a cheapest path into that state.
func solveOrderingDP(weights [][]int) ([]int, []int) {
	count := len(weights)
	states := 1 << count
	best := make([]int, states)
	predecessor := make([]int, states)
	for state := range best {
		best[state] = math.MaxInt
		predecessor[state] = -1
	}
	best[0] = 0

	for state := range states {
		placed := best[state]
		if placed == math.MaxInt {
			continue
		}
		for vertex := range count {
			bit := 1 << vertex
			if state&bit != 0 {
				continue
			}
			candidate := placed + backwardWeightInto(weights[vertex], state)
			next := state | bit
			if candidate < best[next] {
				best[next] = candidate
				predecessor[next] = vertex
			}
		}
	}
	return best, predecessor
}

// backwardWeightInto sums the arcs from the vertex owning row into every
// already-placed vertex; those arcs become backward arcs when the vertex is
// appended after the placed prefix.
func backwardWeightInto(row []int, state int) int {
	total := 0
	for remaining := state; remaining != 0; remaining &= remaining - 1 {
		total += row[bits.TrailingZeros(uint(remaining))]
	}
	return total
}

// reconstructOrdering walks the recorded predecessors back from the full state
// to recover one optimal ordering.
func reconstructOrdering(predecessor []int, count int) []int {
	ordering := make([]int, count)
	state := (1 << count) - 1
	for position := count - 1; position >= 0; position-- {
		vertex := predecessor[state]
		ordering[position] = vertex
		state &^= 1 << vertex
	}
	return ordering
}

// orderedPositions inverts an ordering into a vertex-indexed position lookup.
func orderedPositions(ordering []int) []int {
	positions := make([]int, len(ordering))
	for position, vertex := range ordering {
		positions[vertex] = position
	}
	return positions
}
