// biome-ignore lint/nursery/noExcessiveLinesPerFile: this projection keeps canonical factory graph mapping rules together.
import type {
  DashboardEdgeOutcomeKind,
  DashboardPlaceKind,
  DashboardPlaceRef,
  DashboardWorkstationNode,
  StateCategory,
} from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import { buildLayeredGraphLayout } from "../../flowchart/lib/layered-layout";
import type { GraphLayout, PositionedNode } from "../../flowchart/lib/layout";

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

function workstationGraphNodeId(workstation: FactoryWorkstation): string {
  return `workstation:${workstation.name}`;
}

function workStatePlaceId(workType: string, state: string): string {
  return `${workType}:${state}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
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

function workStatePlace(
  io: LegacyFactoryWorkstationIO,
  categories: ReadonlyMap<string, StateCategory>,
): DashboardPlaceRef {
  const workType = workstationIOWorkType(io);
  const placeId = workStatePlaceId(workType, io.state);

  return {
    kind: "work_state",
    place_id: placeId,
    state_category: categories.get(placeId),
    state_value: io.state,
    type_id: workType,
  };
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
      return "cron";
    case "POLLER":
      return "poller";
    case "REPEATER":
      return "repeater";
    case "STANDARD":
    case undefined:
      return "standard";
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

function workstationRouteIOs(
  routes: FactoryWorkstationRoute,
): LegacyFactoryWorkstationIO[] {
  if (!routes) {
    return [];
  }
  return Array.isArray(routes) ? routes : [routes];
}

function addNode(
  nodes: Map<string, FactoryGraphSeedNode>,
  node: FactoryGraphSeedNode,
) {
  if (!nodes.has(node.nodeId)) {
    nodes.set(node.nodeId, node);
  }
}

function addPlaceNode(
  nodes: Map<string, FactoryGraphSeedNode>,
  place: DashboardPlaceRef,
) {
  addNode(nodes, {
    height:
      place.kind === "resource" ? RESOURCE_NODE_HEIGHT : STATE_NODE_HEIGHT,
    id: placeGraphNodeId(place.place_id),
    nodeId: placeGraphNodeId(place.place_id),
    nodeKind: place.kind === "resource" ? "resource" : "state_position",
    place,
    width: place.kind === "resource" ? RESOURCE_NODE_WIDTH : STATE_NODE_WIDTH,
  });
}

function addAuxiliaryNode(
  nodes: Map<string, FactoryGraphSeedNode>,
  place: DashboardPlaceRef,
) {
  addNode(nodes, {
    height: AUXILIARY_NODE_HEIGHT,
    id: placeGraphNodeId(place.place_id),
    nodeId: placeGraphNodeId(place.place_id),
    nodeKind: "constraint",
    place,
    width: AUXILIARY_NODE_WIDTH,
  });
}

function addWorkstationNode(
  nodes: Map<string, FactoryGraphSeedNode>,
  workstation: FactoryWorkstation,
) {
  const nodeId = workstationGraphNodeId(workstation);
  addNode(nodes, {
    height: WORKSTATION_NODE_HEIGHT,
    id: nodeId,
    nodeId,
    nodeKind: "workstation",
    width: WORKSTATION_NODE_WIDTH,
    workstationNodeId: workstation.id || workstation.name,
  });
}

function addEdge(
  edges: Map<string, FactoryGraphSeedEdge>,
  edge: Omit<FactoryGraphSeedEdge, "id">,
) {
  edges.set(edge.edgeId, {
    ...edge,
    id: edge.edgeId,
  });
}

function routeOutcome(
  routeKind:
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
): DashboardEdgeOutcomeKind {
  switch (routeKind) {
    case "workstation-on-continue":
      return "continue";
    case "workstation-on-failure":
      return "failed";
    case "workstation-on-rejection":
      return "rejected";
    case "workstation-output":
      return "accepted";
  }
}

function appendInputEdges(
  workstation: FactoryWorkstation,
  categories: ReadonlyMap<string, StateCategory>,
  nodes: Map<string, FactoryGraphSeedNode>,
  edges: Map<string, FactoryGraphSeedEdge>,
) {
  const workstationNodeId = workstationGraphNodeId(workstation);

  for (const input of workstation.inputs ?? []) {
    const place = workStatePlace(input, categories);
    addPlaceNode(nodes, place);
    const fromNodeId = placeGraphNodeId(place.place_id);
    const edgeId = `workstation-input:${fromNodeId}->${workstationNodeId}`;
    addEdge(edges, {
      edgeId,
      fromNodeId,
      label: "",
      outcomeKind: "accepted",
      sourcePlaceKind: "work_state",
      stateCategory: place.state_category,
      targetPlaceKind: undefined,
      toNodeId: workstationNodeId,
    });
  }
}

function appendRouteEdges(
  workstation: FactoryWorkstation,
  routeKind:
    | "workstation-on-continue"
    | "workstation-on-failure"
    | "workstation-on-rejection"
    | "workstation-output",
  routes: FactoryWorkstationRoute,
  categories: ReadonlyMap<string, StateCategory>,
  nodes: Map<string, FactoryGraphSeedNode>,
  edges: Map<string, FactoryGraphSeedEdge>,
) {
  const workstationNodeId = workstationGraphNodeId(workstation);

  for (const output of workstationRouteIOs(routes)) {
    const place = workStatePlace(output, categories);
    addPlaceNode(nodes, place);
    const toNodeId = placeGraphNodeId(place.place_id);
    const edgeId = `${routeKind}:${workstationNodeId}->${toNodeId}`;
    addEdge(edges, {
      edgeId,
      fromNodeId: workstationNodeId,
      label: place.place_id,
      outcomeKind: routeOutcome(routeKind),
      sourcePlaceKind: undefined,
      stateCategory:
        routeKind === "workstation-on-failure"
          ? "FAILED"
          : place.state_category,
      targetPlaceKind: "work_state",
      toNodeId,
    });
  }
}

function appendResourceEdges(
  workstation: FactoryWorkstation,
  nodes: Map<string, FactoryGraphSeedNode>,
  edges: Map<string, FactoryGraphSeedEdge>,
) {
  const workstationNodeId = workstationGraphNodeId(workstation);

  for (const resource of workstation.resources ?? []) {
    const place = factoryEntityPlace("resource", "resource", resource.name);
    addPlaceNode(nodes, place);
    const fromNodeId = placeGraphNodeId(place.place_id);
    const edgeId = `workstation-resource:${fromNodeId}->${workstationNodeId}`;
    addEdge(edges, {
      edgeId,
      fromNodeId,
      label: "",
      outcomeKind: "accepted",
      sourcePlaceKind: "resource",
      stateCategory: undefined,
      targetPlaceKind: undefined,
      toNodeId: workstationNodeId,
    });
  }
}

function appendWorkerEdge(
  workstation: FactoryWorkstation,
  nodes: Map<string, FactoryGraphSeedNode>,
  edges: Map<string, FactoryGraphSeedEdge>,
) {
  if (!workstation.worker) {
    return;
  }

  const workerPlace = factoryEntityPlace(
    "constraint",
    "worker",
    workstation.worker,
  );
  addAuxiliaryNode(nodes, workerPlace);
  const fromNodeId = placeGraphNodeId(workerPlace.place_id);
  const toNodeId = workstationGraphNodeId(workstation);
  const edgeId = `worker-assignment:${fromNodeId}->${toNodeId}`;

  addEdge(edges, {
    edgeId,
    fromNodeId,
    label: "",
    outcomeKind: "accepted",
    sourcePlaceKind: "constraint",
    stateCategory: undefined,
    targetPlaceKind: undefined,
    toNodeId,
  });
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
    (candidate) => (candidate.id || candidate.name) === nodeId,
  );

  return workstation ? dashboardWorkstationFromFactory(workstation) : null;
}

export async function buildCurrentActivityGraphLayoutFromFactory(
  factory: CanonicalFactoryDefinition,
): Promise<GraphLayout> {
  const nodes = new Map<string, FactoryGraphSeedNode>();
  const edges = new Map<string, FactoryGraphSeedEdge>();
  const categories = stateCategoryByName(factory);

  for (const resource of factory.resources ?? []) {
    addPlaceNode(
      nodes,
      factoryEntityPlace("resource", "resource", resource.name),
    );
  }

  for (const worker of factory.workers ?? []) {
    addAuxiliaryNode(
      nodes,
      factoryEntityPlace("constraint", "worker", worker.name),
    );
    for (const resource of worker.resources ?? []) {
      const resourcePlace = factoryEntityPlace(
        "resource",
        "resource",
        resource.name,
      );
      const workerPlace = factoryEntityPlace(
        "constraint",
        "worker",
        worker.name,
      );
      addPlaceNode(nodes, resourcePlace);
      const fromNodeId = placeGraphNodeId(resourcePlace.place_id);
      const toNodeId = placeGraphNodeId(workerPlace.place_id);
      addEdge(edges, {
        edgeId: `worker-resource:${fromNodeId}->${toNodeId}`,
        fromNodeId,
        label: "",
        outcomeKind: "accepted",
        sourcePlaceKind: "resource",
        stateCategory: undefined,
        targetPlaceKind: "constraint",
        toNodeId,
      });
    }
  }

  for (const workType of factoryWorkTypes(factory)) {
    addAuxiliaryNode(
      nodes,
      factoryEntityPlace("constraint", "work-type", workType.name),
    );
    for (const state of workType.states) {
      addPlaceNode(nodes, {
        kind: "work_state",
        place_id: workStatePlaceId(workType.name, state.name),
        state_category: state.type,
        state_value: state.name,
        type_id: workType.name,
      });
    }
  }

  for (const workstation of factory.workstations ?? []) {
    const legacyWorkstation = workstation as LegacyFactoryWorkstation;
    addWorkstationNode(nodes, workstation);
    appendWorkerEdge(workstation, nodes, edges);
    appendResourceEdges(workstation, nodes, edges);
    appendInputEdges(workstation, categories, nodes, edges);
    appendRouteEdges(
      workstation,
      "workstation-output",
      workstation.outputs,
      categories,
      nodes,
      edges,
    );
    appendRouteEdges(
      workstation,
      "workstation-on-continue",
      workstation.onContinue ?? legacyWorkstation.on_continue,
      categories,
      nodes,
      edges,
    );
    appendRouteEdges(
      workstation,
      "workstation-on-rejection",
      workstation.onRejection ?? legacyWorkstation.on_rejection,
      categories,
      nodes,
      edges,
    );
    appendRouteEdges(
      workstation,
      "workstation-on-failure",
      workstation.onFailure ?? legacyWorkstation.on_failure,
      categories,
      nodes,
      edges,
    );
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
