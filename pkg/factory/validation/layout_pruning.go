package validation

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PruneLayout removes stale layout references and rejects non-finite geometry
// against pending graph topology indexes. Callers must build topology indexes
// before invoking this function.
func PruneLayout(cfg *interfaces.FactoryConfig, topology interfaces.PendingFactoryGraphTopology) Result {
	if cfg == nil || cfg.Layout == nil {
		return Result{}
	}

	var targets []Target
	layout := cfg.Layout

	prunedNodes, nodeTargets := pruneLayoutNodes(layout.Nodes, topology)
	targets = append(targets, nodeTargets...)
	layout.Nodes = prunedNodes

	prunedEdges, edgeTargets := pruneLayoutEdges(layout.Edges, topology)
	targets = append(targets, edgeTargets...)
	layout.Edges = prunedEdges

	groupTargets := pruneLayoutGroups(layout, topology)
	targets = append(targets, groupTargets...)

	if layout.Viewport != nil {
		if viewportTargets := pruneLayoutViewport(layout); len(viewportTargets) > 0 {
			targets = append(targets, viewportTargets...)
		}
	}

	return Result{Targets: targets}
}

func pruneLayoutNodes(
	nodes []interfaces.FactoryLayoutNodeConfig,
	topology interfaces.PendingFactoryGraphTopology,
) ([]interfaces.FactoryLayoutNodeConfig, []Target) {
	if len(nodes) == 0 {
		return nil, nil
	}

	pruned := make([]interfaces.FactoryLayoutNodeConfig, 0, len(nodes))
	var targets []Target
	for index, node := range nodes {
		path := fmt.Sprintf("factory.layout.nodes[%d]", index)
		if node.ID == "" {
			continue
		}
		if _, ok := topology.NodeIDs[node.ID]; !ok {
			targets = append(targets, prunedLayoutReferenceTarget(
				CodeLayoutUnknownNodeReference,
				fmt.Sprintf("layout node %q was pruned during save because it does not match any pending graph node.", node.ID),
				node.ID,
				path+".id",
			))
			continue
		}
		if geometryTargets := invalidLayoutGeometryTargets(
			path,
			fmt.Sprintf("layout node %q", node.ID),
			node.Position.X,
			node.Position.Y,
		); len(geometryTargets) > 0 {
			targets = append(targets, rejectedLayoutGeometryTargets(geometryTargets, "node layout")...)
			continue
		}
		if node.Size != nil {
			if geometryTargets := invalidLayoutGeometryTargets(
				path+".size",
				fmt.Sprintf("layout node %q size", node.ID),
				node.Size.Width,
				node.Size.Height,
			); len(geometryTargets) > 0 {
				targets = append(targets, rejectedLayoutGeometryTargets(geometryTargets, "node size")...)
				continue
			}
		}
		pruned = append(pruned, node)
	}
	return pruned, targets
}

func pruneLayoutEdges(
	edges []interfaces.FactoryLayoutEdgeConfig,
	topology interfaces.PendingFactoryGraphTopology,
) ([]interfaces.FactoryLayoutEdgeConfig, []Target) {
	if len(edges) == 0 {
		return nil, nil
	}

	pruned := make([]interfaces.FactoryLayoutEdgeConfig, 0, len(edges))
	var targets []Target
	for index, edge := range edges {
		path := fmt.Sprintf("factory.layout.edges[%d]", index)
		if edge.ID == "" {
			continue
		}
		if _, ok := topology.EdgeIDs[edge.ID]; !ok {
			targets = append(targets, prunedLayoutReferenceTarget(
				CodeLayoutUnknownEdgeReference,
				fmt.Sprintf("layout edge %q was pruned during save because it does not match any pending graph edge.", edge.ID),
				edge.ID,
				path+".id",
			))
			continue
		}
		rejectEdge := false
		for waypointIndex, waypoint := range edge.Waypoints {
			if geometryTargets := invalidLayoutGeometryTargets(
				fmt.Sprintf("%s.waypoints[%d]", path, waypointIndex),
				fmt.Sprintf("layout edge %q waypoint", edge.ID),
				waypoint.X,
				waypoint.Y,
			); len(geometryTargets) > 0 {
				targets = append(targets, rejectedLayoutGeometryTargets(geometryTargets, "edge waypoint")...)
				rejectEdge = true
				break
			}
		}
		if rejectEdge {
			continue
		}
		if edge.LabelPosition != nil {
			if geometryTargets := invalidLayoutGeometryTargets(
				path+".labelPosition",
				fmt.Sprintf("layout edge %q label position", edge.ID),
				edge.LabelPosition.X,
				edge.LabelPosition.Y,
			); len(geometryTargets) > 0 {
				targets = append(targets, rejectedLayoutGeometryTargets(geometryTargets, "edge label position")...)
				continue
			}
		}
		pruned = append(pruned, edge)
	}
	return pruned, targets
}

func pruneLayoutGroups(layout *interfaces.FactoryLayoutConfig, topology interfaces.PendingFactoryGraphTopology) []Target {
	if layout == nil || len(layout.Groups) == 0 {
		return nil
	}

	var targets []Target
	for index := range layout.Groups {
		group := &layout.Groups[index]
		path := fmt.Sprintf("factory.layout.groups[%d]", index)
		groupID := group.ID
		if groupID == "" {
			groupID = fmt.Sprintf("groups[%d]", index)
		}
		if geometryTargets := invalidLayoutGeometryTargets(
			path+".bounds",
			fmt.Sprintf("layout group %q bounds", groupID),
			group.Bounds.X,
			group.Bounds.Y,
			group.Bounds.Width,
			group.Bounds.Height,
		); len(geometryTargets) > 0 {
			targets = append(targets, rejectedLayoutGeometryTargets(geometryTargets, "group bounds")...)
			continue
		}

		prunedNodeIDs := make([]string, 0, len(group.NodeIDs))
		for memberIndex, nodeID := range group.NodeIDs {
			if nodeID == "" {
				continue
			}
			if _, ok := topology.NodeIDs[nodeID]; !ok {
				targets = append(targets, prunedLayoutReferenceTarget(
					CodeLayoutUnknownGroupMemberReference,
					fmt.Sprintf("layout group %q removed unknown graph node %q during save.", groupID, nodeID),
					groupID,
					fmt.Sprintf("%s.nodeIds[%d]", path, memberIndex),
				))
				continue
			}
			prunedNodeIDs = append(prunedNodeIDs, nodeID)
		}
		group.NodeIDs = prunedNodeIDs
	}
	return targets
}

func pruneLayoutViewport(layout *interfaces.FactoryLayoutConfig) []Target {
	if layout == nil || layout.Viewport == nil {
		return nil
	}
	geometryTargets := invalidLayoutGeometryTargets(
		"factory.layout.viewport",
		"layout viewport",
		layout.Viewport.X,
		layout.Viewport.Y,
		layout.Viewport.Zoom,
	)
	if len(geometryTargets) == 0 {
		return nil
	}
	layout.Viewport = nil
	return rejectedLayoutGeometryTargets(geometryTargets, "viewport")
}

func prunedLayoutReferenceTarget(code, message, subjectID, path string) Target {
	return layoutReferenceTarget(code, message, subjectID, path)
}

func rejectedLayoutGeometryTargets(geometryTargets []Target, label string) []Target {
	rejected := make([]Target, 0, len(geometryTargets))
	for _, target := range geometryTargets {
		target.Message = fmt.Sprintf("%s was rejected during save because %s", label, trimGeometryMessage(target.Message))
		rejected = append(rejected, target)
	}
	return rejected
}

func trimGeometryMessage(message string) string {
	const suffix = " contains non-finite geometry."
	if len(message) > len(suffix) && message[len(message)-len(suffix):] == suffix {
		return message[:len(message)-len(suffix)] + " has non-finite geometry"
	}
	return message
}
