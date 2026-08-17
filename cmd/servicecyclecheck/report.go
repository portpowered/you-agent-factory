package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// diagnosticPrefix labels this checker's output the way the other repository
// lint checkers label theirs.
const diagnosticPrefix = "[agent-factory:service-cycle-check]"

// defaultCeilingRelativePath is the small dedicated baseline that stores the
// ratchet ceiling. It is deliberately separate from ownership-inventory.json:
// the ceiling is a single derived integer, not a hand-maintained edge table.
var defaultCeilingRelativePath = filepath.Join("docs", "internal", "baselines", "service-cycle-ceiling.json")

// cycleCeiling is the parsed ceiling baseline.
type cycleCeiling struct {
	Description string `json:"description"`
	Ceiling     int    `json:"ceiling"`
}

// loadCycleCeiling reads and validates the ceiling baseline file.
func loadCycleCeiling(path string) (cycleCeiling, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return cycleCeiling{}, fmt.Errorf("read cycle ceiling baseline %s: %w", filepath.ToSlash(path), err)
	}
	var baseline cycleCeiling
	if err := json.Unmarshal(content, &baseline); err != nil {
		return cycleCeiling{}, fmt.Errorf("decode cycle ceiling baseline %s: %w", filepath.ToSlash(path), err)
	}
	if baseline.Ceiling < 0 {
		return cycleCeiling{}, fmt.Errorf("cycle ceiling baseline %s has a negative ceiling: %d", filepath.ToSlash(path), baseline.Ceiling)
	}
	return baseline, nil
}

// backEdge is one arc of the cut set: an import direction that has to be
// reversed or removed for the service graph to become acyclic.
type backEdge struct {
	from     string
	to       string
	weight   int
	carriers []string
}

// cutSet returns the arcs that point backwards in the supplied ordering,
// which is exactly the minimum feedback arc set the solver selected. The
// result is ordered heaviest-first so the most valuable cut leads the report.
func (graph *serviceGraph) cutSet(ordering []int) []backEdge {
	positions := orderedPositions(ordering)
	index := make(map[string]int, len(graph.services))
	for position, service := range graph.services {
		index[service] = position
	}

	var edges []backEdge
	for edge, weight := range graph.weights {
		from, fromKnown := index[edge.from]
		to, toKnown := index[edge.to]
		if !fromKnown || !toKnown || weight <= 0 {
			continue
		}
		if positions[from] <= positions[to] {
			continue
		}
		edges = append(edges, backEdge{
			from:     edge.from,
			to:       edge.to,
			weight:   weight,
			carriers: graph.carriers[edge],
		})
	}
	slices.SortFunc(edges, func(left, right backEdge) int {
		if left.weight != right.weight {
			return right.weight - left.weight
		}
		if byFrom := strings.Compare(left.from, right.from); byFrom != 0 {
			return byFrom
		}
		return strings.Compare(left.to, right.to)
	})
	return edges
}

// totalWeight sums a cut set's arc weights.
func totalWeight(edges []backEdge) int {
	total := 0
	for _, edge := range edges {
		total += edge.weight
	}
	return total
}

// writeCutSet prints the full cut set: every back-edge with its weight and the
// packages that carry it, so a decoupling lane can act on the report directly
// and later use the same report as proof that its cut landed.
func writeCutSet(writer io.Writer, edges []backEdge) {
	fmt.Fprintf(writer, "cut set (%d edge(s), total weight %d):\n", len(edges), totalWeight(edges))
	for _, edge := range edges {
		fmt.Fprintf(writer, "- %s -> %s (weight %d)\n", edge.from, edge.to, edge.weight)
		for _, carrier := range edge.carriers {
			fmt.Fprintf(writer, "    carrier package: %s\n", carrier)
		}
	}
}

// writeRegression reports a cycle that got deeper than the recorded ceiling.
func writeRegression(writer io.Writer, measured int, ceiling cycleCeiling, edges []backEdge) {
	fmt.Fprintf(
		writer,
		"cross-service cycle regression: minimum feedback arc weight is %d, above the recorded ceiling of %d.\n",
		measured,
		ceiling.Ceiling,
	)
	fmt.Fprintf(
		writer,
		"Remove or reverse cross-service imports until the weight is back at %d; do not raise the ceiling in %s.\n",
		ceiling.Ceiling,
		filepath.ToSlash(defaultCeilingRelativePath),
	)
	writeCutSet(writer, edges)
}

// writeUnclaimedImprovement reports a cycle that got shallower without the
// ceiling being lowered, which is what makes this check a ratchet rather than
// a one-directional cap.
func writeUnclaimedImprovement(writer io.Writer, measured int, ceiling cycleCeiling, edges []backEdge) {
	fmt.Fprintf(
		writer,
		"cross-service cycle improved and the gain is not captured: minimum feedback arc weight is %d, below the recorded ceiling of %d.\n",
		measured,
		ceiling.Ceiling,
	)
	fmt.Fprintf(
		writer,
		"Lower the ceiling to %d in %s so the improvement cannot silently regress later.\n",
		measured,
		filepath.ToSlash(defaultCeilingRelativePath),
	)
	writeCutSet(writer, edges)
}

// writeSuccess reports the measured weight against an equal ceiling.
func writeSuccess(writer io.Writer, measured int, ceiling cycleCeiling, edges []backEdge, services int) {
	fmt.Fprintf(
		writer,
		"%s minimum feedback arc weight %d across %d service(s) in %d cut edge(s) matches ceiling %d\n",
		diagnosticPrefix,
		measured,
		services,
		len(edges),
		ceiling.Ceiling,
	)
}
