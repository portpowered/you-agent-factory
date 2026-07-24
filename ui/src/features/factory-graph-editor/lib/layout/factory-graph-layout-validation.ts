import type { FactoryValidationTarget } from "../../../../api/factory-validation";
import type { components } from "../../../../api/generated/openapi";
import type { FactoryGraphTopology } from "../draft/factory-graph-draft-types";
import {
  type FactoryLayoutEdge,
  factoryLayoutEdgeWaypoints,
  isValidFactoryLayoutPoint,
} from "./factory-graph-layout-edge-waypoints";
import {
  FACTORY_LAYOUT_SCHEMA_VERSION,
  type FactoryLayout,
} from "./factory-graph-layout-operations";

type FactoryLayoutGroup = NonNullable<
  components["schemas"]["Factory"]["layout"]
>["groups"] extends (infer TGroup)[] | undefined
  ? TGroup
  : never;

export const FACTORY_LAYOUT_VALIDATION_CODE = {
  invalidGeometry: "factory.layout.invalidGeometry",
  unknownEdgeReference: "factory.layout.unknownEdgeReference",
  unknownGroupMemberReference: "factory.layout.unknownGroupMemberReference",
} as const;

export type FactoryLayoutValidationTarget = {
  code: (typeof FACTORY_LAYOUT_VALIDATION_CODE)[keyof typeof FACTORY_LAYOUT_VALIDATION_CODE];
  path: string;
};

export function factoryLayoutTopologyEdgeIds(
  topology: FactoryGraphTopology,
): Set<string> {
  return new Set(topology.edges.map((edge) => edge.id));
}

export function factoryLayoutTopologyNodeIds(
  topology: FactoryGraphTopology,
): Set<string> {
  return new Set(topology.nodes.map((node) => node.id));
}

export function isValidFactoryLayoutGroupBounds(
  bounds: FactoryLayoutGroup["bounds"] | undefined,
): bounds is FactoryLayoutGroup["bounds"] {
  return (
    bounds !== undefined &&
    Number.isFinite(bounds.x) &&
    Number.isFinite(bounds.y) &&
    Number.isFinite(bounds.width) &&
    Number.isFinite(bounds.height)
  );
}

export function collectFactoryLayoutEdgeValidationTargets(
  layout: FactoryLayout,
  validEdgeIds: ReadonlySet<string>,
): FactoryLayoutValidationTarget[] {
  const targets: FactoryLayoutValidationTarget[] = [];

  for (const [index, edge] of (layout.edges ?? []).entries()) {
    const path = `factory.layout.edges[${index}]`;
    if (!edge.id) {
      continue;
    }

    if (!validEdgeIds.has(edge.id)) {
      targets.push({
        code: FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference,
        path: `${path}.id`,
      });
      continue;
    }

    for (const [waypointIndex, waypoint] of (edge.waypoints ?? []).entries()) {
      if (!isValidFactoryLayoutPoint(waypoint)) {
        targets.push({
          code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
          path: `${path}.waypoints[${waypointIndex}]`,
        });
      }
    }
  }

  return targets;
}

export function collectFactoryLayoutGroupValidationTargets(
  layout: FactoryLayout,
  validNodeIds: ReadonlySet<string>,
): FactoryLayoutValidationTarget[] {
  const targets: FactoryLayoutValidationTarget[] = [];

  for (const [index, group] of (layout.groups ?? []).entries()) {
    const path = `factory.layout.groups[${index}]`;
    if (!group.id) {
      continue;
    }

    if (!isValidFactoryLayoutGroupBounds(group.bounds)) {
      targets.push({
        code: FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry,
        path: `${path}.bounds`,
      });
    }

    for (const [memberIndex, nodeId] of (group.nodeIds ?? []).entries()) {
      if (!nodeId) {
        continue;
      }
      if (!validNodeIds.has(nodeId)) {
        targets.push({
          code: FACTORY_LAYOUT_VALIDATION_CODE.unknownGroupMemberReference,
          path: `${path}.nodeIds[${memberIndex}]`,
        });
      }
    }
  }

  return targets;
}

export function collectFactoryLayoutValidationTargets(
  layout: FactoryLayout,
  topology: FactoryGraphTopology,
): FactoryLayoutValidationTarget[] {
  return [
    ...collectFactoryLayoutEdgeValidationTargets(
      layout,
      factoryLayoutTopologyEdgeIds(topology),
    ),
    ...collectFactoryLayoutGroupValidationTargets(
      layout,
      factoryLayoutTopologyNodeIds(topology),
    ),
  ];
}

function edgeLayoutEntryHasInvalidWaypointGeometry(
  edge: FactoryLayoutEdge,
): boolean {
  return (edge.waypoints ?? []).some(
    (waypoint) => !isValidFactoryLayoutPoint(waypoint),
  );
}

export function pruneFactoryLayoutEdgesForTopology(
  layout: FactoryLayout,
  validEdgeIds: ReadonlySet<string>,
): {
  layout: FactoryLayout;
  prunedEdgeIds: string[];
} {
  const edges = layout.edges ?? [];
  if (edges.length === 0) {
    return { layout, prunedEdgeIds: [] };
  }

  const prunedEdgeIds: string[] = [];
  const keptEdges: FactoryLayoutEdge[] = [];
  let didChange = false;

  for (const edge of edges) {
    if (!edge.id || !validEdgeIds.has(edge.id)) {
      if (edge.id) {
        prunedEdgeIds.push(edge.id);
      }
      didChange = true;
      continue;
    }

    if (edgeLayoutEntryHasInvalidWaypointGeometry(edge)) {
      prunedEdgeIds.push(edge.id);
      didChange = true;
      continue;
    }

    const nextEdge: FactoryLayoutEdge = { id: edge.id };
    const validWaypoints = (edge.waypoints ?? []).filter(
      isValidFactoryLayoutPoint,
    );
    if (validWaypoints.length > 0) {
      nextEdge.waypoints = validWaypoints;
    }
    if (edge.labelPosition && isValidFactoryLayoutPoint(edge.labelPosition)) {
      nextEdge.labelPosition = edge.labelPosition;
    } else if (edge.labelPosition) {
      didChange = true;
    }

    if (!nextEdge.waypoints?.length && !nextEdge.labelPosition) {
      prunedEdgeIds.push(edge.id);
      didChange = true;
      continue;
    }

    keptEdges.push(nextEdge);
  }

  if (!didChange && keptEdges.length === edges.length) {
    return { layout, prunedEdgeIds };
  }

  return {
    layout: {
      ...layout,
      edges: keptEdges.length > 0 ? keptEdges : undefined,
      schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
    },
    prunedEdgeIds,
  };
}

export function pruneFactoryLayoutGroupsForTopology(
  layout: FactoryLayout,
  validNodeIds: ReadonlySet<string>,
): {
  layout: FactoryLayout;
  prunedGroupMemberNodeIds: string[];
  rejectedGroupIds: string[];
} {
  const groups = layout.groups ?? [];
  if (groups.length === 0) {
    return {
      layout,
      prunedGroupMemberNodeIds: [],
      rejectedGroupIds: [],
    };
  }

  const prunedGroupMemberNodeIds: string[] = [];
  const rejectedGroupIds: string[] = [];
  const keptGroups: FactoryLayoutGroup[] = [];
  let didChange = false;

  for (const group of groups) {
    if (!group.id) {
      didChange = true;
      continue;
    }

    if (!isValidFactoryLayoutGroupBounds(group.bounds)) {
      rejectedGroupIds.push(group.id);
      didChange = true;
      continue;
    }

    const prunedNodeIds: string[] = [];
    for (const nodeId of group.nodeIds ?? []) {
      if (!nodeId) {
        didChange = true;
        continue;
      }
      if (!validNodeIds.has(nodeId)) {
        prunedGroupMemberNodeIds.push(nodeId);
        didChange = true;
        continue;
      }
      prunedNodeIds.push(nodeId);
    }

    const nextGroup: FactoryLayoutGroup = {
      bounds: {
        height: group.bounds.height,
        width: group.bounds.width,
        x: group.bounds.x,
        y: group.bounds.y,
      },
      id: group.id,
      nodeIds: prunedNodeIds,
    };
    if (group.color !== undefined) {
      nextGroup.color = group.color;
    }
    if (group.label !== undefined) {
      nextGroup.label = group.label;
    }
    if (group.locked !== undefined) {
      nextGroup.locked = group.locked;
    }
    if (group.parentGroupId !== undefined) {
      nextGroup.parentGroupId = group.parentGroupId;
    }

    keptGroups.push(nextGroup);
  }

  if (!didChange && keptGroups.length === groups.length) {
    return {
      layout,
      prunedGroupMemberNodeIds,
      rejectedGroupIds,
    };
  }

  return {
    layout: {
      ...layout,
      groups: keptGroups.length > 0 ? keptGroups : undefined,
      schemaVersion: layout.schemaVersion ?? FACTORY_LAYOUT_SCHEMA_VERSION,
    },
    prunedGroupMemberNodeIds,
    rejectedGroupIds,
  };
}

export function resolveFactoryLayoutEdgeWaypointsForRendering(
  layout: FactoryLayout,
  edgeId: string,
): ReturnType<typeof factoryLayoutEdgeWaypoints> {
  return factoryLayoutEdgeWaypoints(layout, edgeId);
}

export function projectFactoryLayoutValidationTargets(
  layout: FactoryLayout,
  topology: FactoryGraphTopology,
): FactoryValidationTarget[] {
  return collectFactoryLayoutValidationTargets(layout, topology).map((target) =>
    toFactoryValidationTarget(target, layout),
  );
}

export function preparePendingFactoryLayoutForSave(
  layout: FactoryLayout,
  topology: FactoryGraphTopology,
): {
  layout: FactoryLayout;
} {
  const validEdgeIds = factoryLayoutTopologyEdgeIds(topology);
  const validNodeIds = factoryLayoutTopologyNodeIds(topology);
  const { layout: edgePrunedLayout } = pruneFactoryLayoutEdgesForTopology(
    layout,
    validEdgeIds,
  );
  const { layout: prunedLayout } = pruneFactoryLayoutGroupsForTopology(
    edgePrunedLayout,
    validNodeIds,
  );

  return {
    layout: prunedLayout,
  };
}

function toFactoryValidationTarget(
  target: FactoryLayoutValidationTarget,
  layout: FactoryLayout,
): FactoryValidationTarget {
  const edgeId = resolveEdgeIdFromValidationPath(layout, target.path);
  const groupId = resolveGroupIdFromValidationPath(layout, target.path);
  const subjectId = edgeId ?? groupId ?? layoutGeometrySubjectId(target.path);

  return {
    code: target.code,
    message: buildLayoutValidationMessage(target, layout),
    severity: "warning",
    subject: {
      id: subjectId,
      location: "REFERENCE",
      type: "FACTORY",
    },
  };
}

function resolveEdgeIdFromValidationPath(
  layout: FactoryLayout,
  path: string,
): string | undefined {
  const match = /^factory\.layout\.edges\[(\d+)\]/.exec(path);
  if (!match) {
    return undefined;
  }

  const index = Number(match[1]);
  return layout.edges?.[index]?.id;
}

function resolveGroupIdFromValidationPath(
  layout: FactoryLayout,
  path: string,
): string | undefined {
  const match = /^factory\.layout\.groups\[(\d+)\]/.exec(path);
  if (!match) {
    return undefined;
  }

  const index = Number(match[1]);
  return layout.groups?.[index]?.id;
}

function resolveNodeIdFromGroupValidationPath(
  layout: FactoryLayout,
  path: string,
): string | undefined {
  const match = /^factory\.layout\.groups\[(\d+)\]\.nodeIds\[(\d+)\]/.exec(
    path,
  );
  if (!match) {
    return undefined;
  }

  const groupIndex = Number(match[1]);
  const memberIndex = Number(match[2]);
  return layout.groups?.[groupIndex]?.nodeIds?.[memberIndex];
}

function layoutGeometrySubjectId(path: string): string {
  const trimmed = path.replace(/^factory\.layout\./, "");
  return trimmed.length > 0 ? trimmed : "layout";
}

function buildLayoutValidationMessage(
  target: FactoryLayoutValidationTarget,
  layout: FactoryLayout,
): string {
  const edgeId = resolveEdgeIdFromValidationPath(layout, target.path);
  const groupId = resolveGroupIdFromValidationPath(layout, target.path);
  const nodeId = resolveNodeIdFromGroupValidationPath(layout, target.path);

  switch (target.code) {
    case FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference:
      return edgeId
        ? // hardcoded-ui-copy-exception: non-product-diagnostic
          `Layout edge "${edgeId}" references a graph edge that is no longer present.`
        : // hardcoded-ui-copy-exception: non-product-diagnostic
          "Layout edge references a graph edge that is no longer present.";
    case FACTORY_LAYOUT_VALIDATION_CODE.unknownGroupMemberReference:
      return groupId && nodeId
        ? // hardcoded-ui-copy-exception: non-product-diagnostic
          `Layout group "${groupId}" references unknown graph node "${nodeId}".`
        : // hardcoded-ui-copy-exception: non-product-diagnostic
          "Layout group references a graph node that is no longer present.";
    case FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry:
      if (groupId && target.path.endsWith(".bounds")) {
        // hardcoded-ui-copy-exception: non-product-diagnostic
        return `Layout group "${groupId}" bounds contain non-finite geometry.`;
      }
      return edgeId
        ? // hardcoded-ui-copy-exception: non-product-diagnostic
          `Layout edge "${edgeId}" contains non-finite geometry and will use generated routing.`
        : // hardcoded-ui-copy-exception: non-product-diagnostic
          "Layout contains non-finite geometry and will use generated routing.";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return `Recoverable layout issue at ${target.path}.`;
  }
}
