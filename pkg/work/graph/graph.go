// Package graph derives deterministic Work relationship graphs.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"
)

// Node is one work item in a derived dependency graph.
type Node struct {
	ID    string
	Label string
}

// Edge is a directed relationship between two work items.
type Edge struct {
	SourceID string
	TargetID string
	Type     string
}

// Graph is a deterministic projection of work items and declared relationships.
type Graph struct {
	Nodes []Node
	Edges []Edge
}

// DeriveFromWorkRequest projects a parsed batch request into a dependency graph.
func DeriveFromWorkRequest(req work.WorkRequest) (Graph, error) {
	if err := validateBatchForGraph(req); err != nil {
		return Graph{}, err
	}
	return buildGraph(req), nil
}

func validateBatchForGraph(req work.WorkRequest) error {
	if req.Type != work.WorkRequestTypeFactoryRequestBatch {
		return fmt.Errorf("batch type must be %q", work.WorkRequestTypeFactoryRequestBatch)
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return fmt.Errorf("batch requestId is required")
	}
	if len(req.Works) == 0 {
		return fmt.Errorf("batch works must contain at least one item")
	}

	workNames := make(map[string]struct{}, len(req.Works))
	for i, work := range req.Works {
		if strings.TrimSpace(work.Name) == "" {
			return fmt.Errorf("works[%d] is missing required name", i)
		}
		if _, exists := workNames[work.Name]; exists {
			return fmt.Errorf("works[%d] has duplicate name %q", i, work.Name)
		}
		workNames[work.Name] = struct{}{}
	}

	for i, rel := range req.Relations {
		if strings.TrimSpace(rel.SourceWorkName) == "" {
			return fmt.Errorf("relations[%d] is missing sourceWorkName", i)
		}
		if strings.TrimSpace(rel.TargetWorkName) == "" {
			return fmt.Errorf("relations[%d] is missing targetWorkName", i)
		}
		if _, ok := workNames[rel.SourceWorkName]; !ok {
			return fmt.Errorf("relations[%d] references unknown sourceWorkName %q", i, rel.SourceWorkName)
		}
		if _, ok := workNames[rel.TargetWorkName]; !ok {
			return fmt.Errorf("relations[%d] references unknown targetWorkName %q", i, rel.TargetWorkName)
		}
		switch rel.Type {
		case work.WorkRelationDependsOn, work.WorkRelationParentChild:
		case "":
			return fmt.Errorf("relations[%d] is missing type", i)
		default:
			return fmt.Errorf("relations[%d] has unsupported type %q", i, rel.Type)
		}
	}
	return nil
}

func buildGraph(req work.WorkRequest) Graph {
	nodes := make([]Node, 0, len(req.Works))
	for i, work := range req.Works {
		nodes = append(nodes, Node{
			ID:    stableNodeID(work, i),
			Label: nodeLabel(work, i),
		})
	}

	edges := make([]Edge, 0, len(req.Relations))
	for _, rel := range req.Relations {
		edges = append(edges, Edge{
			SourceID: rel.SourceWorkName,
			TargetID: rel.TargetWorkName,
			Type:     string(rel.Type),
		})
	}
	sortEdges(edges)

	return Graph{
		Nodes: nodes,
		Edges: edges,
	}
}

func stableNodeID(work work.Work, index int) string {
	if strings.TrimSpace(work.Name) != "" {
		return work.Name
	}
	if strings.TrimSpace(work.WorkID) != "" {
		return work.WorkID
	}
	return fmt.Sprintf("work-%d", index+1)
}

func nodeLabel(work work.Work, index int) string {
	if strings.TrimSpace(work.Name) != "" {
		return work.Name
	}
	if strings.TrimSpace(work.WorkID) != "" {
		return work.WorkID
	}
	return fmt.Sprintf("work-%d", index+1)
}

func sortEdges(edges []Edge) {
	sort.Slice(edges, func(i, j int) bool {
		left := edges[i]
		right := edges[j]
		if left.SourceID != right.SourceID {
			return left.SourceID < right.SourceID
		}
		if left.TargetID != right.TargetID {
			return left.TargetID < right.TargetID
		}
		return left.Type < right.Type
	})
}
