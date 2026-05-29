// biome-ignore lint/nursery/noExcessiveLinesPerFile: this projection keeps canonical factory graph mapping rules together.
import type {
  DashboardEdgeOutcomeKind,
  DashboardPlaceKind,
  DashboardPlaceRef,
  DashboardWorkstationNode,
  StateCategory,
} from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import { filterFactoryGraphTopologyForCustomerDisplay } from "../../factory-graph-editor/lib/factory-graph-customer-display";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-graph";
import type {
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { buildLayeredGraphLayout } from "../../flowchart/lib/layered-layout";
import type { GraphLayout, PositionedNode } from "../../flowchart/lib/layout";
import {
  CRON_WORKSTATION_KIND,
  POLLER_WORKSTATION_KIND,
  REPEATER_WORKSTATION_KIND,
  STANDARD_WORKSTATION_KIND,
} from "../../flowchart/lib/workstation-icon-metadata";

const WORKSTATION_NODE_WIDTH = 156;
const WORKSTATION_NODE_HEIGHT = 196;
const STATE_NODE_WIDTH = 164;
const STATE_NODE_HEIGHT = 86;
const RESOURCE_NODE_WIDTH = 168;
const RESOURCE_NODE_HEIGHT = STATE_NODE_HEIGHT;
const AUXILIARY_NODE_WIDTH = 156;
const AUXILIARY_NODE_HEIGHT = 58;

interface FactoryGraphSeedNode {
  height: number;
  id: string;
  nodeId: string;
  nodeKind: "constraint" | "resource" | "state_position" | "workstation";
  place?: DashboardPlaceRef;
  width: number;
  workstationNodeId?: string;
}

interface FactoryGraphSeedEdge {
  edgeId: string;
  fromNodeId: string;
  id: string;
  label: string;
  outcomeKind: DashboardEdgeOutcomeKind;
  sourcePlaceKind: DashboardPlaceKind | undefined;
  stateCategory: StateCategory | undefined;
  targetPlaceKind: DashboardPlaceKind | undefined;
  toNodeId: string;
}

type FactoryWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type FactoryWorkstationIO = FactoryWorkstation["inputs"][number];
type LegacyFactoryWorkstationIO = FactoryWorkstationIO & {
  work_type?: string;
};
type FactoryWorkstationRoute =
  | LegacyFactoryWorkstationIO
  | LegacyFactoryWorkstationIO[]
  | null
  | undefined;
type LegacyFactoryDefinition = CanonicalFactoryDefinition & {
  work_types?: CanonicalFactoryDefinition["workTypes"];
};
type LegacyFactoryWorkstation = FactoryWorkstation & {
  on_continue?: FactoryWorkstationRoute;
  on_failure?: FactoryWorkstationRoute;
  on_rejection?: FactoryWorkstationRoute;
};
type DashboardWorkstationKind = NonNullable<
  DashboardWorkstationNode["workstation_kind"]
>;

function workStatePlaceId(workType: string, state: string): string {
  return `${workType}:${state}`;
}

function factoryEntityPlace(
  kind: DashboardPlaceKind,
  entityKind: string,
  name: string,
): DashboardPlaceRef {
  if (kind === "resource" && entityKind === "resource") {
    return {
      kind,
      place_id: `${name}:available`,
      state_value: "available",
      type_id: name,
    };
  }

  return {
    kind,
    place_id: `${entityKind}:${name}`,
    state_value: name,
    type_id: entityKind,
  };
}

function stateCategoryByName(factory: CanonicalFactoryDefinition) {
  const categories = new Map<string, StateCategory>();

  for (const workType of factoryWorkTypes(factory)) {
    for (const state of workType.states) {
      categories.set(workStatePlaceId(workType.name, state.name), state.type);
    }
  }

  return categories;
}

function factoryWorkTypes(factory: CanonicalFactoryDefinition) {
  const legacyFactory = factory as LegacyFactoryDefinition;
  return legacyFactory.workTypes ?? legacyFactory.work_types ?? [];
}

function workstationIOWorkType(io: LegacyFactoryWorkstationIO): string {
  return io.workType ?? io.work_type ?? "";
}

function dashboardWorkstationKind(
  behavior: FactoryWorkstation["behavior"],
): DashboardWorkstationKind {
  switch (behavior) {
    case "CRON":
      return CRON_WORKSTATION_KIND;
    case "POLLER":
      return POLLER_WORKSTATION_KIND;
    case "REPEATER":
      return REPEATER_WORKSTATION_KIND;
    case "STANDARD":
    case undefined:
      return STANDARD_WORKSTATION_KIND;
  }
}

function dashboardWorkstationOutputRoutes(
  workstation: FactoryWorkstation,
): LegacyFactoryWorkstationIO[] {
  const legacyWorkstation = workstation as LegacyFactoryWorkstation;
  return [
    ...workstationRouteIOs(workstation.outputs),
    ...workstationRouteIOs(workstation.onContinue),
    ...workstationRouteIOs(legacyWorkstation.on_continue),
    ...workstationRouteIOs(workstation.onRejection),
    ...workstationRouteIOs(legacyWorkstation.on_rejection),
    ...workstationRouteIOs(workstation.onFailure),
    ...workstationRouteIOs(legacyWorkstation.on_failure),
  ];
}

function normalizeFactoryDefinitionForGraph(
  factory: CanonicalFactoryDefinition,
): CanonicalFactoryDefinition {
  const legacyFactory = factory as LegacyFactoryDefinition;
  return {
    ...factory,
    workTypes: factory.workTypes ?? legacyFactory.work_types ?? [],
    workstations: (factory.workstations ?? []).map((workstation) => {
      const legacyWorkstation = workstation as LegacyFactoryWorkstation;
      return {
        ...workstation,
        inputs: normalizedWorkstationRoute(workstation.inputs) ?? [],
        onContinue:
          normalizedWorkstationRoute(workstation.onContinue) ??
          normalizedWorkstationRoute(legacyWorkstation.on_continue),
        onFailure:
          normalizedWorkstationRoute(workstation.onFailure) ??
          normalizedWorkstationRoute(legacyWorkstation.on_failure),
        onRejection:
          normalizedWorkstationRoute(workstation.onRejection) ??
          normalizedWorkstationRoute(legacyWorkstation.on_rejection),
        outputs: normalizedWorkstationRoute(workstation.outputs) ?? [],
      };
    }),
  };
}

function normalizedWorkstationRoute(
  routes: FactoryWorkstationRoute,
): FactoryWorkstationIO[] | undefined {
  const routeIOs = workstationRouteIOs(routes);
  return routeIOs.length > 0
    ? routeIOs.map((io) => ({
        ...io,
        workType: workstationIOWorkType(io),
      }))
    : undefined;
}

function workstationRouteIOs(
  routes: FactoryWorkstationRoute,
): LegacyFactoryWorkstationIO[] {
  if (!routes) {
    return [];
  }
  return Array.isArray(routes) ? routes : [routes];
}

function placeForFactoryGraphNode(node: FactoryGraphNode): DashboardPlaceRef {
  switch (node.key.kind) {
    case "resource":
      return factoryEntityPlace("resource", "resource", node.key.name);
    case "worker":
      return factoryEntityPlace("constraint", "worker", node.key.name);
    case "work-state":
      return {
        kind: "work_state",
        place_id: workStatePlaceId(node.key.workTypeName, node.key.stateName),
        state_value: node.key.stateName,
        type_id: node.key.workTypeName,
      };
    case "work-type":
      return factoryEntityPlace("constraint", "work-type", node.key.name);
    case "workstation":
      return factoryEntityPlace("constraint", "workstation", node.key.name);
  }
}

function nodeKindForFactoryGraphNode(
  node: FactoryGraphNode,
): FactoryGraphSeedNode["nodeKind"] {
  switch (node.kind) {
    case "resource":
      return "resource";
    case "work-state":
      return "state_position";
    case "worker":
    case "work-type":
      return "constraint";
    case "workstation":
      return "workstation";
  }
}

function nodeDimensionsForFactoryGraphNode(node: FactoryGraphNode) {
  switch (node.kind) {
    case "resource":
      return { height: RESOURCE_NODE_HEIGHT, width: RESOURCE_NODE_WIDTH };
    case "work-state":
      return { height: STATE_NODE_HEIGHT, width: STATE_NODE_WIDTH };
    case "worker":
    case "work-type":
      return { height: AUXILIARY_NODE_HEIGHT, width: AUXILIARY_NODE_WIDTH };
    case "workstation":
      return { height: WORKSTATION_NODE_HEIGHT, width: WORKSTATION_NODE_WIDTH };
  }
}

function seedNodeFromFactoryGraphNode(
  node: FactoryGraphNode,
): FactoryGraphSeedNode {
  const dimensions = nodeDimensionsForFactoryGraphNode(node);
  if (node.kind === "workstation") {
    return {
      height: dimensions.height,
      id: node.id,
      nodeId: node.id,
      nodeKind: "workstation",
      width: dimensions.width,
      workstationNodeId: node.label,
    };
  }

  return {
    height: dimensions.height,
    id: node.id,
    nodeId: node.id,
    nodeKind: nodeKindForFactoryGraphNode(node),
    place: placeForFactoryGraphNode(node),
    width: dimensions.width,
  };
}

function placeKindForFactoryGraphNodeKind(
  kind: FactoryGraphNodeKind,
): DashboardPlaceKind | undefined {
  switch (kind) {
    case "resource":
      return "resource";
    case "work-state":
      return "work_state";
    case "worker":
    case "work-type":
      return "constraint";
    case "workstation":
      return undefined;
  }
}

function stateCategoryForFactoryGraphNode(
  node: FactoryGraphNode | undefined,
  categories: ReadonlyMap<string, StateCategory>,
) {
  if (node?.key.kind !== "work-state") {
    return undefined;
  }

  return categories.get(
    workStatePlaceId(node.key.workTypeName, node.key.stateName),
  );
}

function edgeLabelForFactoryGraphEdge(
  targetNode: FactoryGraphNode | undefined,
) {
  return targetNode?.kind === "work-state" ? targetNode.label : "";
}

function resourceAvailabilityWorkTypeNames(
  factory: CanonicalFactoryDefinition,
) {
  const resourceNames = new Set(
    (factory.resources ?? []).map((resource) => resource.name),
  );
  const resourceAvailabilityWorkTypes = new Set<string>();

  for (const workType of factoryWorkTypes(factory)) {
    if (!resourceNames.has(workType.name)) {
      continue;
    }
    if (workType.states.some((state) => state.name === "available")) {
      resourceAvailabilityWorkTypes.add(workType.name);
    }
  }

  return resourceAvailabilityWorkTypes;
}

function resourceAvailabilityNodeName(
  node: FactoryGraphNode | undefined,
  resourceAvailabilityWorkTypes: ReadonlySet<string>,
) {
  if (
    node?.key.kind === "work-state" &&
    resourceAvailabilityWorkTypes.has(node.key.workTypeName)
  ) {
    return node.key.workTypeName;
  }

  return null;
}

function isResourceAvailabilityWorkTypeNode(
  node: FactoryGraphNode,
  resourceAvailabilityWorkTypes: ReadonlySet<string>,
) {
  return (
    node.key.kind === "work-type" &&
    resourceAvailabilityWorkTypes.has(node.key.name)
  );
}

function edgeOutcomeKind(edge: FactoryGraphEdge): DashboardEdgeOutcomeKind {
  switch (edge.kind) {
    case "workstation-on-continue":
      return "continue";
    case "workstation-on-failure":
      return "failed";
    case "workstation-on-rejection":
      return "rejected";
    case "worker-assignment":
    case "worker-resource":
    case "workstation-input":
    case "workstation-output":
    case "workstation-resource":
    case "work-type-state":
      return "accepted";
  }
}

function seedEdgeFromFactoryGraphEdge(
  edge: FactoryGraphEdge,
  topology: FactoryGraphTopology,
  categories: ReadonlyMap<string, StateCategory>,
  resourceAvailabilityWorkTypes: ReadonlySet<string>,
): FactoryGraphSeedEdge | null {
  if (edge.kind === "work-type-state") {
    return null;
  }

  const sourceNode = topology.nodes.find((node) => node.id === edge.sourceId);
  const targetNode = topology.nodes.find((node) => node.id === edge.targetId);
  if (!sourceNode || !targetNode) {
    return null;
  }
  const sourceResourceName = resourceAvailabilityNodeName(
    sourceNode,
    resourceAvailabilityWorkTypes,
  );
  const targetResourceName = resourceAvailabilityNodeName(
    targetNode,
    resourceAvailabilityWorkTypes,
  );
  const fromNodeId = sourceResourceName
    ? `resource:${sourceResourceName}`
    : edge.sourceId;
  const toNodeId = targetResourceName
    ? `resource:${targetResourceName}`
    : edge.targetId;
  if (fromNodeId === toNodeId) {
    return null;
  }
  const canonicalEdgeKind =
    edge.kind === "workstation-input" && sourceResourceName
      ? "workstation-resource"
      : edge.kind;
  const edgeId =
    canonicalEdgeKind === edge.kind &&
    fromNodeId === edge.sourceId &&
    toNodeId === edge.targetId
      ? edge.id
      : `${canonicalEdgeKind}:${fromNodeId}->${toNodeId}`;
  const targetPlaceKind = targetResourceName
    ? "resource"
    : placeKindForFactoryGraphNodeKind(targetNode.kind);

  return {
    edgeId,
    fromNodeId,
    id: edgeId,
    label: targetResourceName ? "" : edgeLabelForFactoryGraphEdge(targetNode),
    outcomeKind: edgeOutcomeKind(edge),
    sourcePlaceKind: sourceResourceName
      ? "resource"
      : placeKindForFactoryGraphNodeKind(sourceNode.kind),
    stateCategory: targetResourceName
      ? undefined
      : edge.kind === "workstation-on-failure"
        ? "FAILED"
        : stateCategoryForFactoryGraphNode(targetNode, categories),
    targetPlaceKind,
    toNodeId,
  };
}

function toPositionedNode(
  seedNode: FactoryGraphSeedNode,
  column: number,
  row: number,
  x: number,
  y: number,
): PositionedNode {
  if (seedNode.nodeKind === "workstation") {
    return {
      column,
      height: seedNode.height,
      nodeId: seedNode.nodeId,
      nodeKind: "workstation",
      row,
      width: seedNode.width,
      workstationNodeId: seedNode.workstationNodeId ?? seedNode.nodeId,
      x,
      y,
    };
  }

  return {
    column,
    height: seedNode.height,
    nodeId: seedNode.nodeId,
    nodeKind: seedNode.nodeKind,
    place:
      seedNode.place ??
      factoryEntityPlace("constraint", "unknown", seedNode.nodeId),
    row,
    width: seedNode.width,
    x,
    y,
  };
}

function toPositionedEdges(
  edges: FactoryGraphSeedEdge[],
  nodesById: ReadonlyMap<string, PositionedNode>,
): GraphLayout["edges"] {
  return edges
    .filter(
      (edge) => nodesById.has(edge.fromNodeId) && nodesById.has(edge.toNodeId),
    )
    .map((edge) => ({
      edgeId: edge.edgeId,
      fromNodeId: edge.fromNodeId,
      label: edge.label,
      labelX: 0,
      labelY: 0,
      outcomeKind: edge.outcomeKind,
      path: "",
      sourcePlaceKind: edge.sourcePlaceKind,
      stateCategory: edge.stateCategory,
      targetPlaceKind: edge.targetPlaceKind,
      toNodeId: edge.toNodeId,
    }));
}

export function dashboardWorkstationFromFactory(
  workstation: FactoryWorkstation,
): DashboardWorkstationNode {
  const outputRoutes = dashboardWorkstationOutputRoutes(workstation);

  return {
    input_place_ids: (workstation.inputs ?? []).map((input) =>
      workStatePlaceId(workstationIOWorkType(input), input.state),
    ),
    input_places: (workstation.inputs ?? []).map((input) => ({
      kind: "work_state",
      place_id: workStatePlaceId(workstationIOWorkType(input), input.state),
      state_value: input.state,
      type_id: workstationIOWorkType(input),
    })),
    node_id: workstation.id || workstation.name,
    output_place_ids: outputRoutes.map((output) =>
      workStatePlaceId(workstationIOWorkType(output), output.state),
    ),
    output_places: outputRoutes.map((output) => ({
      kind: "work_state",
      place_id: workStatePlaceId(workstationIOWorkType(output), output.state),
      state_value: output.state,
      type_id: workstationIOWorkType(output),
    })),
    transition_id: workstation.id || workstation.name,
    workstation_kind: dashboardWorkstationKind(workstation.behavior),
    workstation_name: workstation.name,
  };
}

export function findFactoryWorkstationByNodeId(
  factory: CanonicalFactoryDefinition | undefined,
  nodeId: string,
): DashboardWorkstationNode | null {
  const workstation = (factory?.workstations ?? []).find(
    (candidate) => candidate.id === nodeId || candidate.name === nodeId,
  );

  return workstation ? dashboardWorkstationFromFactory(workstation) : null;
}

export async function buildCurrentActivityGraphLayoutFromFactory(
  factory: CanonicalFactoryDefinition,
): Promise<GraphLayout> {
  const nodes = new Map<string, FactoryGraphSeedNode>();
  const edges = new Map<string, FactoryGraphSeedEdge>();
  const normalizedFactory = normalizeFactoryDefinitionForGraph(factory);
  const categories = stateCategoryByName(normalizedFactory);
  const resourceAvailabilityWorkTypes =
    resourceAvailabilityWorkTypeNames(normalizedFactory);
  const topology = filterFactoryGraphTopologyForCustomerDisplay(
    buildFactoryGraphTopologyFromDefinition(normalizedFactory),
  );

  for (const node of topology.nodes) {
    if (
      resourceAvailabilityNodeName(node, resourceAvailabilityWorkTypes) ||
      isResourceAvailabilityWorkTypeNode(node, resourceAvailabilityWorkTypes)
    ) {
      continue;
    }
    nodes.set(node.id, seedNodeFromFactoryGraphNode(node));
  }

  for (const edge of topology.edges) {
    const seedEdge = seedEdgeFromFactoryGraphEdge(
      edge,
      topology,
      categories,
      resourceAvailabilityWorkTypes,
    );
    if (seedEdge) {
      edges.set(seedEdge.edgeId, seedEdge);
    }
  }

  return layoutFactoryGraphSeeds(nodes, edges);
}

async function layoutFactoryGraphSeeds(
  nodes: ReadonlyMap<string, FactoryGraphSeedNode>,
  edges: ReadonlyMap<string, FactoryGraphSeedEdge>,
): Promise<GraphLayout> {
  if (nodes.size === 0) {
    return { edges: [], height: 0, nodes: [], width: 0 };
  }

  const layeredLayout = await buildLayeredGraphLayout({
    edges: [...edges.values()].map((edge) => ({
      ...edge,
      sources: [edge.fromNodeId],
      targets: [edge.toNodeId],
    })),
    nodes: [...nodes.values()],
  });
  const positionedNodes = layeredLayout.nodes.map((node) =>
    toPositionedNode(node, node.column, node.row, node.x, node.y),
  );
  const positionedNodesById = new Map(
    positionedNodes.map((node) => [node.nodeId, node]),
  );

  return {
    edges: toPositionedEdges([...edges.values()], positionedNodesById),
    height: layeredLayout.height,
    nodes: positionedNodes,
    width: layeredLayout.width,
  };
}
