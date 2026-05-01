import ELK from "elkjs/lib/elk.bundled.js";
import type { ElkExtendedEdge, ElkNode, LayoutOptions } from "elkjs/lib/elk.bundled.js";

import { formatDashboardPlaceLabel } from "../../components/dashboard/place-labels";
import type {
  DashboardEdgeOutcomeKind,
  DashboardPlaceKind,
  DashboardPlaceRef,
  DashboardTopology,
  DashboardWorkstationNode,
  StateCategory,
} from "../../api/dashboard/types";
import { isExhaustionWorkstation } from "./workstation-semantics";

export type PositionedNodeKind = "constraint" | "state_position" | "resource" | "workstation";

export interface PositionedBaseNode {
  column: number;
  height: number;
  nodeId: string;
  nodeKind: PositionedNodeKind;
  row: number;
  width: number;
  x: number;
  y: number;
}

export interface PositionedPlaceNode extends PositionedBaseNode {
  nodeKind: "constraint" | "resource" | "state_position";
  place: DashboardPlaceRef;
}

export interface PositionedWorkstationNode extends PositionedBaseNode {
  nodeKind: "workstation";
  workstationNodeId: string;
}

export type PositionedNode = PositionedPlaceNode | PositionedWorkstationNode;

export interface PositionedEdge {
  edgeId: string;
  fromNodeId: string;
  label: string;
  labelX: number;
  labelY: number;
  outcomeKind: DashboardEdgeOutcomeKind;
  path: string;
  sourcePlaceKind: DashboardPlaceKind | undefined;
  stateCategory: StateCategory | undefined;
  toNodeId: string;
  targetPlaceKind: DashboardPlaceKind | undefined;
}

export interface GraphLayout {
  edges: PositionedEdge[];
  height: number;
  nodes: PositionedNode[];
  width: number;
}

const WORKSTATION_NODE_WIDTH = 156;
const WORKSTATION_NODE_HEIGHT = 196;
const EXHAUSTION_NODE_WIDTH = 132;
const EXHAUSTION_NODE_HEIGHT = 58;
const STATE_NODE_WIDTH = 164;
const STATE_NODE_HEIGHT = 86;
const RESOURCE_NODE_WIDTH = 168;
const RESOURCE_NODE_HEIGHT = STATE_NODE_HEIGHT;
const CONSTRAINT_NODE_WIDTH = 156;
const CONSTRAINT_NODE_HEIGHT = 58;
const PADDING_X = 40;
const PADDING_Y = 36;
const LAYER_SPACING = 56;
const NODE_SPACING = 40;

const elk = new ELK();

const ELK_LAYOUT_OPTIONS: LayoutOptions = {
  "elk.algorithm": "layered",
  "elk.direction": "RIGHT",
  "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
  "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
  "elk.layered.spacing.nodeNodeBetweenLayers": `${LAYER_SPACING}`,
  "elk.spacing.nodeNode": `${NODE_SPACING}`,
};

interface GraphSeedNode extends ElkNode {
  height: number;
  id: string;
  nodeId: string;
  nodeKind: PositionedNodeKind;
  place?: DashboardPlaceRef;
  width: number;
  workstationNodeId?: string;
}

interface GraphSeedEdge extends ElkExtendedEdge {
  edgeId: string;
  fromNodeId: string;
  label: string;
  outcomeKind: DashboardEdgeOutcomeKind;
  sourcePlaceKind: DashboardPlaceKind | undefined;
  stateCategory: StateCategory | undefined;
  targetPlaceKind: DashboardPlaceKind | undefined;
  toNodeId: string;
}

interface GraphSeeds {
  edges: GraphSeedEdge[];
  nodes: GraphSeedNode[];
}

function workstationGraphNodeId(nodeId: string): string {
  return `workstation:${nodeId}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
}

function edgeLabel(workTypeID?: string, stateValue?: string): string {
  if (workTypeID && stateValue) {
    return `${workTypeID}:${stateValue}`;
  }
  if (stateValue) {
    return stateValue;
  }
  return workTypeID ?? "";
}

function placeNodeKind(place: DashboardPlaceRef): PositionedPlaceNode["nodeKind"] {
  if (place.kind === "work_state") {
    return "state_position";
  }
  if (place.kind === "resource") {
    return "resource";
  }
  return "constraint";
}

function placeNodeDimensions(place: DashboardPlaceRef): { height: number; width: number } {
  if (place.kind === "work_state") {
    return { height: STATE_NODE_HEIGHT, width: STATE_NODE_WIDTH };
  }
  if (place.kind === "resource") {
    return { height: RESOURCE_NODE_HEIGHT, width: RESOURCE_NODE_WIDTH };
  }
  return { height: CONSTRAINT_NODE_HEIGHT, width: CONSTRAINT_NODE_WIDTH };
}

function workstationNodeDimensions(
  workstation: DashboardWorkstationNode | undefined,
): { height: number; width: number } {
  if (workstation && isExhaustionWorkstation(workstation)) {
    return { height: EXHAUSTION_NODE_HEIGHT, width: EXHAUSTION_NODE_WIDTH };
  }

  return { height: WORKSTATION_NODE_HEIGHT, width: WORKSTATION_NODE_WIDTH };
}

function fallbackPlaceRef(placeId: string): DashboardPlaceRef {
  return {
    kind: "work_state",
    place_id: placeId,
  };
}

function collectPlacesById(topology: DashboardTopology): Map<string, DashboardPlaceRef> {
  const placesById = new Map<string, DashboardPlaceRef>();

  for (const nodeId of topology.workstation_node_ids) {
    const workstation = topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of [
      ...(workstation.input_places ?? []),
      ...(workstation.output_places ?? []),
    ]) {
      placesById.set(place.place_id, place);
    }
  }

  for (const edge of topology.edges ?? []) {
    if (!placesById.has(edge.via_place_id)) {
      placesById.set(edge.via_place_id, {
        kind: "work_state",
        place_id: edge.via_place_id,
        state_category: edge.state_category,
        state_value: edge.state_value,
        type_id: edge.work_type_id,
      });
    }
  }

  return placesById;
}

function buildOutputEdgeMetadata(topology: DashboardTopology): Map<string, GraphSeedEdge> {
  const metadataByOutput = new Map<string, GraphSeedEdge>();

  for (const edge of topology.edges ?? []) {
    const fromNodeId = workstationGraphNodeId(edge.from_node_id);
    const toNodeId = placeGraphNodeId(edge.via_place_id);
    const edgeId = `${fromNodeId}:${toNodeId}:${edge.outcome_kind ?? "accepted"}`;
    const key = `${edge.from_node_id}:${edge.via_place_id}`;
    metadataByOutput.set(key, {
      edgeId,
      fromNodeId,
      id: edgeId,
      label: edgeLabel(edge.work_type_id, edge.state_value),
      outcomeKind: edge.outcome_kind ?? "accepted",
      sources: [fromNodeId],
      sourcePlaceKind: undefined,
      stateCategory: edge.state_category,
      targets: [toNodeId],
      targetPlaceKind: "work_state",
      toNodeId,
    });
  }

  return metadataByOutput;
}

function buildGraphSeeds(topology: DashboardTopology): GraphSeeds {
  const placesById = collectPlacesById(topology);
  const outputEdgeMetadata = buildOutputEdgeMetadata(topology);
  const nodes: GraphSeedNode[] = [
    ...[...placesById.values()]
      .sort((left, right) => left.place_id.localeCompare(right.place_id))
      .map((place) => {
        const dimensions = placeNodeDimensions(place);

        return {
          height: dimensions.height,
          id: placeGraphNodeId(place.place_id),
          nodeId: placeGraphNodeId(place.place_id),
          nodeKind: placeNodeKind(place),
          place,
          width: dimensions.width,
        };
      }),
    ...topology.workstation_node_ids.map((nodeId) => {
      const dimensions = workstationNodeDimensions(topology.workstation_nodes_by_id[nodeId]);

      return {
        height: dimensions.height,
        id: workstationGraphNodeId(nodeId),
        nodeId: workstationGraphNodeId(nodeId),
        nodeKind: "workstation" as const,
        width: dimensions.width,
        workstationNodeId: nodeId,
      };
    }),
  ];
  const edges = new Map<string, GraphSeedEdge>();

  for (const nodeId of topology.workstation_node_ids) {
    const workstation = topology.workstation_nodes_by_id[nodeId];
    if (!workstation) {
      continue;
    }

    for (const place of workstation.input_places ?? []) {
      const fromNodeId = placeGraphNodeId(place.place_id);
      const toNodeId = workstationGraphNodeId(nodeId);
      const edgeId = `${fromNodeId}:${toNodeId}:input`;
      edges.set(edgeId, {
        edgeId,
        fromNodeId,
        id: edgeId,
        label: "",
        outcomeKind: "accepted",
        sources: [fromNodeId],
        sourcePlaceKind: place.kind,
        stateCategory: place.state_category,
        targets: [toNodeId],
        targetPlaceKind: undefined,
        toNodeId,
      });
    }

    for (const outputPlaceId of workstation.output_place_ids ?? []) {
      const place = placesById.get(outputPlaceId) ?? fallbackPlaceRef(outputPlaceId);
      const metadata = outputEdgeMetadata.get(`${nodeId}:${outputPlaceId}`);
      const fromNodeId = workstationGraphNodeId(nodeId);
      const toNodeId = placeGraphNodeId(outputPlaceId);
      const edgeId = metadata?.edgeId ?? `${fromNodeId}:${toNodeId}:output`;
      edges.set(edgeId, {
        edgeId,
        fromNodeId,
        id: edgeId,
        label: metadata?.label ?? formatDashboardPlaceLabel(place),
        outcomeKind: metadata?.outcomeKind ?? "accepted",
        sources: [fromNodeId],
        sourcePlaceKind: undefined,
        stateCategory: metadata?.stateCategory ?? place.state_category,
        targets: [toNodeId],
        targetPlaceKind: place.kind,
        toNodeId,
      });
    }
  }

  return {
    edges: [...edges.values()],
    nodes,
  };
}

function toPositionedNode(
  seedNode: GraphSeedNode,
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
    place: seedNode.place ?? fallbackPlaceRef(seedNode.nodeId),
    row,
    width: seedNode.width,
    x,
    y,
  };
}

function buildOrdinalAssignments(
  nodes: GraphSeedNode[],
  coordinate: "x" | "y",
): Map<string, number> {
  const sortedCoordinates = [
    ...new Set(nodes.map((node) => Math.round(node[coordinate] ?? 0))),
  ].sort((left, right) => left - right);

  return new Map(
    nodes.map((node) => [
      node.nodeId,
      sortedCoordinates.indexOf(Math.round(node[coordinate] ?? 0)),
    ]),
  );
}

function buildEdgePath(points: { x: number; y: number }[]): string {
  if (points.length === 0) {
    return "";
  }

  const [firstPoint, ...remainingPoints] = points;
  return [
    `M ${firstPoint.x} ${firstPoint.y}`,
    ...remainingPoints.map((point) => `L ${point.x} ${point.y}`),
  ].join(" ");
}

function edgePoints(edge: GraphSeedEdge, nodesById: Map<string, PositionedNode>) {
  const section = edge.sections?.[0];
  if (section) {
    return [
      section.startPoint,
      ...(section.bendPoints ?? []),
      section.endPoint,
    ];
  }

  const from = nodesById.get(edge.fromNodeId);
  const to = nodesById.get(edge.toNodeId);
  if (!from || !to) {
    return [];
  }

  return [
    { x: from.x + from.width, y: from.y + from.height / 2 },
    { x: to.x, y: to.y + to.height / 2 },
  ];
}

function toPositionedEdges(
  edges: GraphSeedEdge[],
  nodesById: Map<string, PositionedNode>,
): PositionedEdge[] {
  return edges
    .map((edge) => {
      const points = edgePoints(edge, nodesById);
      if (points.length === 0) {
        return null;
      }

      const firstPoint = points[0];
      const lastPoint = points[points.length - 1] ?? firstPoint;

      return {
        edgeId: edge.edgeId,
        fromNodeId: edge.fromNodeId,
        label: edge.label,
        labelX: firstPoint.x + (lastPoint.x - firstPoint.x) / 2,
        labelY: Math.min(firstPoint.y, lastPoint.y) - 18,
        outcomeKind: edge.outcomeKind,
        path: buildEdgePath(points),
        sourcePlaceKind: edge.sourcePlaceKind,
        stateCategory: edge.stateCategory,
        targetPlaceKind: edge.targetPlaceKind,
        toNodeId: edge.toNodeId,
      };
    })
    .filter((edge): edge is PositionedEdge => edge !== null);
}

export async function buildGraphLayout(
  topology: DashboardTopology,
): Promise<GraphLayout> {
  if (topology.workstation_node_ids.length === 0) {
    return { edges: [], height: 0, nodes: [], width: 0 };
  }

  const seeds = buildGraphSeeds(topology);
  const graph: ElkNode = {
    children: seeds.nodes,
    edges: seeds.edges,
    id: "root",
    layoutOptions: ELK_LAYOUT_OPTIONS,
  };
  const layoutedGraph = await elk.layout(graph);
  const layoutedNodes = (layoutedGraph.children ?? []) as GraphSeedNode[];
  const minX = Math.min(...layoutedNodes.map((node) => node.x ?? 0));
  const minY = Math.min(...layoutedNodes.map((node) => node.y ?? 0));
  const columnAssignments = buildOrdinalAssignments(layoutedNodes, "x");
  const rowAssignments = buildOrdinalAssignments(layoutedNodes, "y");
  const positionedNodes = layoutedNodes
    .map((node) =>
      toPositionedNode(
        node,
        columnAssignments.get(node.nodeId) ?? 0,
        rowAssignments.get(node.nodeId) ?? 0,
        (node.x ?? 0) - minX + PADDING_X,
        (node.y ?? 0) - minY + PADDING_Y,
      ),
    )
    .sort((left, right) => left.column - right.column || left.row - right.row);
  const positionedNodesById = new Map(positionedNodes.map((node) => [node.nodeId, node]));
  const rightmostX = Math.max(...positionedNodes.map((node) => node.x + node.width));
  const bottomY = Math.max(...positionedNodes.map((node) => node.y + node.height));

  return {
    edges: toPositionedEdges(seeds.edges, positionedNodesById),
    height: bottomY + PADDING_Y,
    nodes: positionedNodes,
    width: rightmostX + PADDING_X,
  };
}
