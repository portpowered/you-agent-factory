package validation

import (
	"fmt"
	"math"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// ValidateLayout reports recoverable layout defects against pending graph topology
// indexes. Callers must build topology indexes before invoking this function.
func ValidateLayout(cfg *interfaces.FactoryConfig, topology interfaces.PendingFactoryGraphTopology) Result {
	if cfg == nil || cfg.Layout == nil {
		return Result{}
	}

	var targets []Target
	targets = append(targets, unsupportedLayoutSchemaVersionTarget(cfg.Layout.SchemaVersion)...)
	targets = append(targets, layoutNodeTargets(cfg.Layout, topology)...)
	targets = append(targets, layoutEdgeTargets(cfg.Layout, topology)...)
	targets = append(targets, layoutGroupTargets(cfg.Layout, topology)...)
	if cfg.Layout.Viewport != nil {
		targets = append(targets, invalidLayoutGeometryTargets(
			"factory.layout.viewport",
			"layout viewport",
			cfg.Layout.Viewport.X,
			cfg.Layout.Viewport.Y,
			cfg.Layout.Viewport.Zoom,
		)...)
	}
	return Result{Targets: targets}
}

func unsupportedLayoutSchemaVersionTarget(schemaVersion int) []Target {
	if schemaVersion == interfaces.SupportedFactoryLayoutSchemaVersion {
		return nil
	}
	return []Target{{
		Code:     CodeLayoutUnsupportedSchemaVersion,
		Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"layout schemaVersion %d is not supported; supported schemaVersion is %d.",
			schemaVersion,
			interfaces.SupportedFactoryLayoutSchemaVersion,
		),
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "layout",
			Location: SubjectLocationDefinition,
		},
		Path: "factory.layout.schemaVersion",
	}}
}

func layoutNodeTargets(layout *interfaces.FactoryLayoutConfig, topology interfaces.PendingFactoryGraphTopology) []Target {
	var targets []Target
	for index, node := range layout.Nodes {
		path := fmt.Sprintf("factory.layout.nodes[%d]", index)
		if node.ID == "" {
			if node.EmptyState != nil {
				targets = append(targets, layoutReferenceTarget(
					CodeLayoutEmptyStateUnknownNodeReference,
					"layout node empty state requires a non-empty canonical graph node id.",
					node.ID,
					path+".emptyState",
					SeverityError,
				))
			}
			continue
		}
		if _, ok := topology.NodeIDs[node.ID]; !ok {
			if node.EmptyState != nil {
				targets = append(targets, layoutReferenceTarget(
					CodeLayoutEmptyStateUnknownNodeReference,
					fmt.Sprintf("layout node empty state requires canonical graph node %q.", node.ID),
					node.ID,
					path+".emptyState",
					SeverityError,
				))
				continue
			}
			targets = append(targets, layoutReferenceTarget(
				CodeLayoutUnknownNodeReference,
				fmt.Sprintf("layout node %q does not match any pending graph node.", node.ID),
				node.ID,
				path+".id",
				layoutUnknownNodeReferenceSeverity(node.ID),
			))
		}
		targets = append(targets, invalidLayoutGeometryTargets(
			path,
			fmt.Sprintf("layout node %q", node.ID),
			node.Position.X,
			node.Position.Y,
		)...)
		if node.Size != nil {
			targets = append(targets, invalidLayoutGeometryTargets(
				path+".size",
				fmt.Sprintf("layout node %q size", node.ID),
				node.Size.Width,
				node.Size.Height,
			)...)
		}
	}
	return targets
}

func layoutEdgeTargets(layout *interfaces.FactoryLayoutConfig, topology interfaces.PendingFactoryGraphTopology) []Target {
	var targets []Target
	for index, edge := range layout.Edges {
		path := fmt.Sprintf("factory.layout.edges[%d]", index)
		if edge.ID == "" {
			continue
		}
		if _, ok := topology.EdgeIDs[edge.ID]; !ok {
			targets = append(targets, layoutReferenceTarget(
				CodeLayoutUnknownEdgeReference,
				fmt.Sprintf("layout edge %q does not match any pending graph edge.", edge.ID),
				edge.ID,
				path+".id",
				SeverityWarning,
			))
		}
		for waypointIndex, waypoint := range edge.Waypoints {
			targets = append(targets, invalidLayoutGeometryTargets(
				fmt.Sprintf("%s.waypoints[%d]", path, waypointIndex),
				fmt.Sprintf("layout edge %q waypoint", edge.ID),
				waypoint.X,
				waypoint.Y,
			)...)
		}
		if edge.LabelPosition != nil {
			targets = append(targets, invalidLayoutGeometryTargets(
				path+".labelPosition",
				fmt.Sprintf("layout edge %q label position", edge.ID),
				edge.LabelPosition.X,
				edge.LabelPosition.Y,
			)...)
		}
	}
	return targets
}

func layoutGroupTargets(layout *interfaces.FactoryLayoutConfig, topology interfaces.PendingFactoryGraphTopology) []Target {
	var targets []Target
	for index, group := range layout.Groups {
		path := fmt.Sprintf("factory.layout.groups[%d]", index)
		groupID := group.ID
		if groupID == "" {
			groupID = fmt.Sprintf("groups[%d]", index)
		}
		targets = append(targets, invalidLayoutGeometryTargets(
			path+".bounds",
			fmt.Sprintf("layout group %q bounds", groupID),
			group.Bounds.X,
			group.Bounds.Y,
			group.Bounds.Width,
			group.Bounds.Height,
		)...)
		for memberIndex, nodeID := range group.NodeIDs {
			if nodeID == "" {
				continue
			}
			if _, ok := topology.NodeIDs[nodeID]; !ok {
				targets = append(targets, layoutReferenceTarget(
					CodeLayoutUnknownGroupMemberReference,
					fmt.Sprintf("layout group %q references unknown graph node %q.", groupID, nodeID),
					groupID,
					fmt.Sprintf("%s.nodeIds[%d]", path, memberIndex),
					SeverityWarning,
				))
			}
		}
	}
	return targets
}

func layoutReferenceTarget(code, message, subjectID, path string, severity Severity) Target {
	return Target{
		Code:     code,
		Severity: severity,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       subjectID,
			Location: SubjectLocationReference,
		},
		Path: path,
	}
}

func layoutUnknownNodeReferenceSeverity(nodeID string) Severity {
	if interfaces.IsBundledFileGraphNodeID(nodeID) {
		return SeverityError
	}
	return SeverityWarning
}

func invalidLayoutGeometryTargets(path, label string, values ...float64) []Target {
	for _, value := range values {
		if !invalidLayoutCoordinate(value) {
			continue
		}
		return []Target{{
			Code:     CodeLayoutInvalidGeometry,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("%s contains non-finite geometry.", label),
			Subject: Subject{
				Type:     SubjectTypeFactory,
				ID:       layoutGeometrySubjectID(path),
				Location: SubjectLocationReference,
			},
			Path: path,
		}}
	}
	return nil
}

func invalidLayoutCoordinate(value float64) bool {
	return math.IsNaN(value) || math.IsInf(value, 0)
}

func layoutGeometrySubjectID(path string) string {
	trimmed := strings.TrimPrefix(path, "factory.layout.")
	if trimmed == "" {
		return "layout"
	}
	return trimmed
}

// IsLayoutTargetCode reports whether code identifies a recoverable layout defect.
func IsLayoutTargetCode(code string) bool {
	return strings.HasPrefix(code, "factory.layout.")
}

// LayoutSaveOutcomes prunes stale layout references for save, then merges
// recoverable validation warnings (such as unsupported schemaVersion) that
// pruning does not report. Prune outcomes take precedence when both phases
// target the same code/path pair.
func LayoutSaveOutcomes(cfg *interfaces.FactoryConfig, topology interfaces.PendingFactoryGraphTopology) Result {
	pruneResult := PruneLayout(cfg, topology)
	validateResult := ValidateLayout(cfg, topology)
	return Result{Targets: MergeLayoutSaveOutcomes(pruneResult.Targets, validateResult.Targets)}
}

// MergeLayoutSaveOutcomes combines prune/reject targets with recoverable layout
// validation warnings while deduplicating by code and path.
func MergeLayoutSaveOutcomes(pruneTargets, validateTargets []Target) []Target {
	if len(validateTargets) == 0 {
		return pruneTargets
	}
	if len(pruneTargets) == 0 {
		return validateTargets
	}

	seen := make(map[string]struct{}, len(pruneTargets))
	for _, target := range pruneTargets {
		seen[layoutSaveOutcomeKey(target)] = struct{}{}
	}
	merged := append([]Target(nil), pruneTargets...)
	for _, target := range validateTargets {
		key := layoutSaveOutcomeKey(target)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, target)
		seen[key] = struct{}{}
	}
	return merged
}

func layoutSaveOutcomeKey(target Target) string {
	return target.Code + "\x00" + target.Path
}

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

	prunedGroups, groupTargets := pruneLayoutGroups(layout.Groups, topology)
	targets = append(targets, groupTargets...)
	layout.Groups = prunedGroups

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
			if node.EmptyState != nil {
				targets = append(targets, layoutReferenceTarget(
					CodeLayoutEmptyStateUnknownNodeReference,
					"layout node empty state was rejected during save because its canonical graph node id is empty.",
					node.ID,
					path+".emptyState",
					SeverityError,
				))
			}
			continue
		}
		if _, ok := topology.NodeIDs[node.ID]; !ok {
			code := CodeLayoutUnknownNodeReference
			message := fmt.Sprintf("layout node %q was pruned during save because it does not match any pending graph node.", node.ID)
			pathSuffix := ".id"
			if node.EmptyState != nil {
				code = CodeLayoutEmptyStateUnknownNodeReference
				message = fmt.Sprintf("layout node empty state for %q was rejected during save because it does not match any canonical graph node.", node.ID)
				pathSuffix = ".emptyState"
			}
			targets = append(targets, prunedLayoutReferenceTarget(
				code,
				message,
				node.ID,
				path+pathSuffix,
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

func pruneLayoutGroups(
	groups []interfaces.FactoryLayoutGroupConfig,
	topology interfaces.PendingFactoryGraphTopology,
) ([]interfaces.FactoryLayoutGroupConfig, []Target) {
	if len(groups) == 0 {
		return nil, nil
	}

	pruned := make([]interfaces.FactoryLayoutGroupConfig, 0, len(groups))
	var targets []Target
	for index, group := range groups {
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
		pruned = append(pruned, group)
	}
	return pruned, targets
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
	return layoutReferenceTarget(code, message, subjectID, path, SeverityWarning)
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
