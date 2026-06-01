import type { DashboardSnapshot } from "../../../api/dashboard/types";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "./current-activity-factory-graph-layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";
import { loadFactoryGraphOnrejectionEdgeReproductionFactory } from "./test-data/factory-graph-onrejection-edge-reproduction.fixture";

function buildReproductionSnapshot(): DashboardSnapshot {
  const factory = loadFactoryGraphOnrejectionEdgeReproductionFactory();
  const workstations = (factory.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );

  return {
    factory,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: {},
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: workstations.map(
        (workstation) => workstation.node_id,
      ),
      workstation_nodes_by_id: Object.fromEntries(
        workstations.map((workstation) => [workstation.node_id, workstation]),
      ),
    },
    uptime_seconds: 0,
  };
}

describe("current activity graph review-loop worker stop token", () => {
  it("renders visible reject and continue handles for the reproduction factory in editor mode", async () => {
    const factory = loadFactoryGraphOnrejectionEdgeReproductionFactory();
    const snapshot = buildReproductionSnapshot();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-06-01T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    const reviewNode = nodes.find((node) => node.id === "workstation:review");
    const processNode = nodes.find((node) => node.id === "workstation:process");
    const reviewRejectionHandle = reviewNode?.data.handles?.find(
      (handle) => handle.id === "workstation-on-rejection-source",
    );
    const processContinueHandle = processNode?.data.handles?.find(
      (handle) => handle.id === "workstation-on-continue-source",
    );

    expect(reviewRejectionHandle).toEqual(
      expect.objectContaining({
        connectable: true,
        hidden: undefined,
        id: "workstation-on-rejection-source",
      }),
    );
    expect(processContinueHandle).toEqual(
      expect.objectContaining({
        connectable: true,
        hidden: undefined,
        id: "workstation-on-continue-source",
      }),
    );
    expect(reviewNode?.data.zAxisIncompleteHints).toBeNull();
  });

  it("wires the review reject route to the rendered reject handle", async () => {
    const factory = loadFactoryGraphOnrejectionEdgeReproductionFactory();
    const snapshot = buildReproductionSnapshot();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(
      visibleGraphEdges,
      graphLayout.nodes,
    );
    const editorNodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-06-01T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const observerNodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-06-01T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    const reviewRejectEdge = visibleGraphEdges.find(
      (edge) =>
        edge.fromNodeId === "workstation:review" &&
        edge.outcomeKind === "rejected",
    );
    expect(reviewRejectEdge).toBeTruthy();

    const editorEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      editorNodes,
    );
    const observerEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      observerNodes,
    );

    expect(
      editorEdges.find((edge) => edge.id === reviewRejectEdge?.edgeId),
    ).toMatchObject({
      source: "workstation:review",
      sourceHandle: "workstation-on-rejection-source",
      targetHandle: "work-state-input-target",
    });
    expect(
      observerEdges.find((edge) => edge.id === reviewRejectEdge?.edgeId),
    ).toMatchObject({
      source: "workstation:review",
      sourceHandle: "workstation-on-rejection-source",
      targetHandle: "work-state-input-target",
    });
  });
});
