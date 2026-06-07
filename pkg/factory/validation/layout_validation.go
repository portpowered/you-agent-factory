package validation

import (
	"fmt"
	"math"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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
			continue
		}
		if _, ok := topology.NodeIDs[node.ID]; !ok {
			targets = append(targets, layoutReferenceTarget(
				CodeLayoutUnknownNodeReference,
				fmt.Sprintf("layout node %q does not match any pending graph node.", node.ID),
				node.ID,
				path+".id",
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
				))
			}
		}
	}
	return targets
}

func layoutReferenceTarget(code, message, subjectID, path string) Target {
	return Target{
		Code:     code,
		Severity: SeverityWarning,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       subjectID,
			Location: SubjectLocationReference,
		},
		Path: path,
	}
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
