import type { FactoryValidationTarget } from "../../../../api/factory-validation";
import type { FactoryGraphTopology } from "../draft/factory-graph-draft-types";
import {
  factoryLayoutEdgeWaypoints,
  isValidFactoryLayoutPoint,
  type FactoryLayoutEdge,
} from "./factory-graph-layout-edge-waypoints";
import {
  type FactoryLayout,
  FACTORY_LAYOUT_SCHEMA_VERSION,
} from "./factory-graph-layout-operations";

export const FACTORY_LAYOUT_VALIDATION_CODE = {
  invalidGeometry: "factory.layout.invalidGeometry",
  unknownEdgeReference: "factory.layout.unknownEdgeReference",
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
    const validWaypoints = (edge.waypoints ?? []).filter(isValidFactoryLayoutPoint);
    if (validWaypoints.length > 0) {
      nextEdge.waypoints = validWaypoints;
    }
    if (
      edge.labelPosition &&
      isValidFactoryLayoutPoint(edge.labelPosition)
    ) {
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

export function resolveFactoryLayoutEdgeWaypointsForRendering(
  layout: FactoryLayout,
  edgeId: string,
): ReturnType<typeof factoryLayoutEdgeWaypoints> {
  return factoryLayoutEdgeWaypoints(layout, edgeId);
}

export function projectFactoryLayoutValidationTargets(
  layout: FactoryLayout,
  validEdgeIds: ReadonlySet<string>,
): FactoryValidationTarget[] {
  return collectFactoryLayoutEdgeValidationTargets(layout, validEdgeIds).map(
    (target) => toFactoryValidationTarget(target, layout),
  );
}

export function preparePendingFactoryLayoutForSave(
  layout: FactoryLayout,
  validEdgeIds: ReadonlySet<string>,
): {
  layout: FactoryLayout;
} {
  const { layout: prunedLayout } = pruneFactoryLayoutEdgesForTopology(
    layout,
    validEdgeIds,
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

  return {
    code: target.code,
    message: buildLayoutValidationMessage(target.code, edgeId, target.path),
    severity: "warning",
    subject: {
      id: edgeId ?? layoutGeometrySubjectId(target.path),
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

function layoutGeometrySubjectId(path: string): string {
  const trimmed = path.replace(/^factory\.layout\./, "");
  return trimmed.length > 0 ? trimmed : "layout";
}

function buildLayoutValidationMessage(
  code: (typeof FACTORY_LAYOUT_VALIDATION_CODE)[keyof typeof FACTORY_LAYOUT_VALIDATION_CODE],
  edgeId: string | undefined,
  path: string,
): string {
  switch (code) {
    case FACTORY_LAYOUT_VALIDATION_CODE.unknownEdgeReference:
      return edgeId
        ? // hardcoded-ui-copy-exception: non-product-diagnostic
          `Layout edge "${edgeId}" references a graph edge that is no longer present.`
        : // hardcoded-ui-copy-exception: non-product-diagnostic
          "Layout edge references a graph edge that is no longer present.";
    case FACTORY_LAYOUT_VALIDATION_CODE.invalidGeometry:
      return edgeId
        ? // hardcoded-ui-copy-exception: non-product-diagnostic
          `Layout edge "${edgeId}" contains non-finite geometry and will use generated routing.`
        : // hardcoded-ui-copy-exception: non-product-diagnostic
          "Layout contains non-finite geometry and will use generated routing.";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return `Recoverable layout issue at ${path}.`;
  }
}
