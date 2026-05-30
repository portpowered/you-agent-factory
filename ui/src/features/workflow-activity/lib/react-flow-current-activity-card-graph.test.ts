// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/nursery/noExcessiveLinesPerFile: shared graph-handle and pending-edge coverage stays grouped around one layout fixture seam.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { resolveDashboardSelection } from "../../current-selection/base/public";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../../factory-graph-editor/lib/factory-graph-customer-display";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/factory-graph-draft-graph";
import { buildGraphLayout } from "../../flowchart/lib/layout";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "./current-activity-factory-graph-layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildEditorHandles,
  supportedEditorHandleIdsForEdge,
} from "./react-flow-current-activity-card-editor-handles";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";

function loadSampleFactoryDefinition(): CanonicalFactoryDefinition {
  return JSON.parse(
    readFileSync(
      resolve(
        process.cwd(),
        "src/features/workflow-activity/lib/current-activity-sample-factory.fixture.json",
      ),
      "utf-8",
    ),
  ) as CanonicalFactoryDefinition;
}

function buildSampleFactorySnapshot(
  factory: CanonicalFactoryDefinition,
): DashboardSnapshot {
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

function resourceFamilyNodeIds(nodeIds: string[], resourceName: string) {
  return nodeIds.filter(
    (nodeId) =>
      nodeId === `resource:${resourceName}` ||
      nodeId === `work-type:${resourceName}` ||
      nodeId === `work-state:${resourceName}:available`,
  );
}

describe("current activity graph editor handles", () => {
  it("keeps a sample factory terminal state selected instead of falling back to the first workstation", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const resolvedSelection = resolveDashboardSelection({
      selection: { kind: "state-node", placeId: "task:complete" },
      snapshot,
    });

    if (
      !resolvedSelection ||
      resolvedSelection.kind === "work-item" ||
      resolvedSelection.kind === "workstation-request"
    ) {
      throw new Error(
        "Expected a graph-compatible current activity selection.",
      );
    }

    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        visibleGraphEdges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: resolvedSelection,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    expect(resolvedSelection).toEqual({
      kind: "state-node",
      placeId: "task:complete",
    });
    expect(
      nodes.find((node) => node.id === "work-state:task:complete")?.data,
    ).toMatchObject({
      kind: "work-state",
      selectedStateNode: true,
    });
    expect(
      nodes.find((node) => node.id === "workstation:process")?.data,
    ).toMatchObject({
      kind: "workstation",
      selectedWorkstation: false,
    });
  });

  it("models sample factory resource relationships through the canonical resource node", async () => {
    const factory = loadSampleFactoryDefinition();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);

    expect(graphLayout.nodes.map((node) => node.nodeId)).toEqual(
      expect.arrayContaining(["resource:executor-slot"]),
    );
    expect(graphLayout.nodes.map((node) => node.nodeId)).not.toContain(
      "place:executor-slot:available",
    );
    expect(visibleGraphEdges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          edgeId:
            "workstation-resource:resource:executor-slot->workstation:process",
          fromNodeId: "resource:executor-slot",
          toNodeId: "workstation:process",
        }),
        expect.objectContaining({
          edgeId:
            "workstation-resource:resource:executor-slot->workstation:review",
          fromNodeId: "resource:executor-slot",
          toNodeId: "workstation:review",
        }),
      ]),
    );
    expect(visibleGraphEdges.map((edge) => edge.fromNodeId)).not.toContain(
      "place:executor-slot:available",
    );
  });

  it("constrains sample factory resource availability aliases to one rendered node", async () => {
    const factory = loadSampleFactoryDefinition();
    const legacyResourceAvailabilityFactory = {
      ...factory,
      workTypes: [
        ...(factory.workTypes ?? []),
        {
          name: "executor-slot",
          states: [
            { name: "available", type: "INITIAL" as const },
            { name: "reserved", type: "PROCESSING" as const },
          ],
        },
      ],
      workstations: (factory.workstations ?? []).map((workstation) =>
        workstation.name === "process"
          ? {
              ...workstation,
              inputs: [
                ...(workstation.inputs ?? []),
                { state: "available", workType: "executor-slot" },
              ],
            }
          : workstation,
      ),
    } satisfies CanonicalFactoryDefinition;
    const rawTopology = buildFactoryGraphTopologyFromDefinition(
      legacyResourceAvailabilityFactory,
    );

    expect(
      resourceFamilyNodeIds(
        rawTopology.nodes.map((node) => node.id),
        "executor-slot",
      ),
    ).toEqual([
      "resource:executor-slot",
      "work-state:executor-slot:available",
      "work-type:executor-slot",
    ]);

    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(
      legacyResourceAvailabilityFactory,
    );
    const renderedResourceFamilyNodes = resourceFamilyNodeIds(
      graphLayout.nodes.map((node) => node.nodeId),
      "executor-slot",
    );

    expect(renderedResourceFamilyNodes).toEqual(["resource:executor-slot"]);
    expect(graphLayout.nodes).toHaveLength(10);
    expect(graphLayout.edges.map((edge) => edge.edgeId)).toContain(
      "workstation-resource:resource:executor-slot->workstation:process",
    );
    expect(graphLayout.edges.map((edge) => edge.edgeId)).not.toContain(
      "workstation-input:work-state:executor-slot:available->workstation:process",
    );
  });

  it("omits system-time topology from current-activity layout while raw topology still contains it", async () => {
    const mixedSystemTimeFactory = {
      name: "mixed-public-system-time",
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" as const },
            { name: "reviewing", type: "PROCESSING" as const },
            { name: "done", type: "TERMINAL" as const },
          ],
        },
        {
          name: SYSTEM_TIME_WORK_TYPE_ID,
          states: [{ name: "pending", type: "PROCESSING" as const }],
        },
      ],
      workstations: [
        {
          behavior: "CLASSIFIER_WORKSTATION",
          classificationRoutes: [
            {
              label: "ready",
              outputs: [{ state: "reviewing", workType: "story" }],
            },
            {
              label: "tick",
              outputs: [
                { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
              ],
            },
          ],
          id: "route-story",
          inputs: [
            { state: "new", workType: "story" },
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          name: "Route story",
          onContinue: [
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          onFailure: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
          onRejection: [
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          outputs: [
            { state: "done", workType: "story" },
            { state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID },
          ],
          worker: "router",
        },
        {
          id: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
          inputs: [{ state: "pending", workType: SYSTEM_TIME_WORK_TYPE_ID }],
          name: SYSTEM_TIME_EXPIRY_TRANSITION_ID,
          outputs: [],
          worker: "",
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const rawTopology = buildFactoryGraphTopologyFromDefinition(
      mixedSystemTimeFactory,
    );

    expect(rawTopology.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );

    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(
      mixedSystemTimeFactory,
    );
    const renderedNodeIds = graphLayout.nodes.map((node) => node.nodeId);

    expect(renderedNodeIds).not.toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", SYSTEM_TIME_WORK_TYPE_ID),
        systemTimeGraphNodeId(
          "work-state",
          SYSTEM_TIME_WORK_TYPE_ID,
          "pending",
        ),
        systemTimeGraphNodeId("workstation", SYSTEM_TIME_EXPIRY_TRANSITION_ID),
      ]),
    );
    expect(renderedNodeIds).toEqual(
      expect.arrayContaining([
        systemTimeGraphNodeId("work-type", "story"),
        systemTimeGraphNodeId("workstation", "Route story"),
      ]),
    );
    expect(
      graphLayout.edges
        .flatMap((edge) => [edge.fromNodeId, edge.toNodeId])
        .some((nodeId) => nodeId.includes(SYSTEM_TIME_WORK_TYPE_ID)),
    ).toBe(false);
  });

  it("renders sample factory work types through the semantic work-type node", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        visibleGraphEdges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    expect(nodes.find((node) => node.id === "work-type:task")).toMatchObject({
      data: {
        factoryGraphNodeId: "work-type:task",
        kind: "work-type",
      },
      type: "workType",
    });
  });

  it("uses semantic handles for visible worker and resource relationships in editor mode", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workers: [
        {
          ...baseFactoryDefinition.workers?.[0],
          name: "writer",
          resources: [{ name: "gpu" }],
          type: "MODEL_WORKER" as const,
        },
      ],
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(visibleGraphEdges);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: {
          anchorId: "worker-resource-source",
          nodeId: "resource:gpu",
        },
      },
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );

    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "worker-resource:resource:gpu->worker:writer",
          sourceHandle: "worker-resource-source",
          targetHandle: "worker-input-target",
        }),
        expect.objectContaining({
          id: "worker-assignment:worker:writer->workstation:draft",
          sourceHandle: "worker-assignment-source",
          targetHandle: "worker-assignment-target",
        }),
        expect.objectContaining({
          id: "workstation-resource:resource:gpu->workstation:draft",
          sourceHandle: "workstation-resource-source",
          targetHandle: "workstation-resource-target",
        }),
      ]),
    );
    expect(
      nodes.find((node) => node.id === "resource:gpu")?.data,
    ).toMatchObject({
      factoryGraphNodeId: "resource:gpu",
      handles: expect.arrayContaining([
        expect.objectContaining({ id: "worker-resource-source" }),
        expect.objectContaining({ id: "workstation-resource-source" }),
      ]),
      kind: "resource",
    });
    expect(
      nodes.find((node) => node.id === "worker:writer")?.data,
    ).toMatchObject({
      factoryGraphNodeId: "worker:writer",
      handles: expect.arrayContaining([
        expect.objectContaining({
          id: "worker-input-target",
          variant: "valid-target",
        }),
        expect.objectContaining({ id: "worker-assignment-source" }),
      ]),
      kind: "worker",
      onSelectWorker: expect.any(Function),
      selectedWorker: false,
    });
  });

  it("marks the selected worker node while worker selection is active", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workers: [
        {
          ...baseFactoryDefinition.workers?.[0],
          name: "writer",
          type: "MODEL_WORKER" as const,
        },
      ],
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "worker", workerName: "writer" },
      snapshot: {
        factory,
        runtime: { place_token_counts: {} },
        topology: { workstation_nodes_by_id: {} },
      } as never,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    expect(nodes.find((node) => node.id === "worker:writer")?.data).toMatchObject({
      kind: "worker",
      selectedWorker: true,
    });
  });

  it("dedupes legacy resource-place aliases when canonical resource nodes are present", async () => {
    const factory = {
      ...baseFactoryDefinition,
      resources: [{ capacity: 1, name: "gpu" }],
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          name: "draft",
          resources: [{ capacity: 1, name: "gpu" }],
        },
      ],
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const resourceNode = graphLayout.nodes.find(
      (node) => node.nodeId === "resource:gpu" && node.nodeKind === "resource",
    );
    if (!resourceNode || resourceNode.nodeKind !== "resource") {
      throw new Error("Expected canonical resource node in graph layout.");
    }

    const graphLayoutWithLegacyAlias = {
      ...graphLayout,
      nodes: [
        {
          ...resourceNode,
          nodeId: "place:gpu:available",
          place: {
            kind: "resource" as const,
            place_id: "gpu:available",
            state_value: "available",
            type_id: "gpu",
          },
        },
        ...graphLayout.nodes,
      ],
    };
    const visibleGraphEdges = buildVisibleGraphEdges(
      graphLayoutWithLegacyAlias,
    );
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout: graphLayoutWithLegacyAlias,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    expect(
      nodes.filter((node) => node.data.factoryGraphNodeId === "resource:gpu"),
    ).toHaveLength(1);
    expect(nodes.find((node) => node.id === "resource:gpu")).toBeTruthy();
    expect(nodes.find((node) => node.id === "place:gpu:available")).toBeFalsy();
  });

  it("binds shared workstation and work-state edges to the editor anchor ids", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(visibleGraphEdges);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: {
          anchorId: "workstation-output-source",
          nodeId: "workstation:review",
        },
      },
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );

    const workstationNode = nodes.find(
      (node) => node.id === "workstation:review",
    );
    const outputEdge = edges.find(
      (edge) =>
        edge.source === "workstation:review" &&
        edge.sourceHandle === "workstation-output-source" &&
        edge.targetHandle === "work-state-input-target",
    );
    const stateNode = nodes.find((node) => node.id === outputEdge?.target);

    expect(workstationNode?.data.kind).toBe("workstation");
    expect(workstationNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          buttonPressed: true,
          connectable: true,
          id: "workstation-output-source",
          label: "Success",
        }),
        expect.objectContaining({
          id: "workstation-input-target",
          label: "Input",
          type: "target",
        }),
      ]),
    );
    expect(workstationNode?.data.handles).toHaveLength(7);
    expect(workstationNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "worker-assignment-target",
          label: "Worker",
        }),
        expect.objectContaining({
          id: "workstation-resource-target",
          label: "Resource",
        }),
      ]),
    );
    expect(stateNode?.data.kind).toBe("work-state");
    expect(stateNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input-source",
          label: "Input",
          type: "source",
        }),
        expect.objectContaining({
          id: "work-state-input-target",
          label: "Input",
          type: "target",
        }),
      ]),
    );
    expect(stateNode?.data.handles).toHaveLength(2);
    expect(outputEdge).toMatchObject({
      sourceHandle: "workstation-output-source",
      targetHandle: "work-state-input-target",
    });
  });

  it("uses hidden semantic handles for observer-mode graph edges", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(visibleGraphEdges);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
    );

    const outputEdge = edges.find(
      (edge) =>
        edge.source === "workstation:review" &&
        edge.sourceHandle === "workstation-output-source" &&
        edge.targetHandle === "work-state-input-target",
    );
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:review",
    );
    const stateNode = nodes.find((node) => node.id === outputEdge?.target);

    expect(outputEdge).toBeTruthy();
    expect(workstationNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          hidden: true,
          id: "workstation-output-source",
          type: "source",
        }),
      ]),
    );
    expect(stateNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          hidden: true,
          id: "work-state-input-target",
          type: "target",
        }),
      ]),
    );
  });

  it("maps rejected and failed observer edges onto the supported shared handle ids", () => {
    expect(
      supportedEditorHandleIdsForEdge({
        edgeId: "rejected",
        fromNodeId: "workstation:review",
        label: "",
        labelX: 0,
        labelY: 0,
        outcomeKind: "rejected",
        path: "",
        sourcePlaceKind: undefined,
        stateCategory: undefined,
        targetPlaceKind: "work_state",
        toNodeId: "place:story:blocked",
      }),
    ).toEqual({
      sourceHandleId: "workstation-on-rejection-source",
      targetHandleId: "work-state-input-target",
    });

    expect(
      supportedEditorHandleIdsForEdge({
        edgeId: "failed",
        fromNodeId: "workstation:review",
        label: "",
        labelX: 0,
        labelY: 0,
        outcomeKind: "accepted",
        path: "",
        sourcePlaceKind: undefined,
        stateCategory: "FAILED",
        targetPlaceKind: "work_state",
        toNodeId: "place:story:blocked",
      }),
    ).toEqual({
      sourceHandleId: "workstation-on-failure-source",
      targetHandleId: "work-state-input-target",
    });
  });

  it("omits continue and reject editor handles for a standard processor without stopWords", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          name: "draft",
          stopWords: undefined,
        },
      ],
    };
    const snapshot = buildSampleFactorySnapshot(factory);
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
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:draft",
    );
    const handleIds = (workstationNode?.data.handles ?? []).map(
      (handle) => handle.id,
    );

    expect(handleIds).toEqual(
      expect.arrayContaining([
        "workstation-output-source",
        "workstation-on-failure-source",
      ]),
    );
    expect(handleIds).not.toContain("workstation-on-continue-source");
    expect(handleIds).not.toContain("workstation-on-rejection-source");
  });

  it("keeps hidden continue and reject handles for authored edges without stopWords", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const reviewNode = nodes.find((node) => node.id === "workstation:review");
    const rejectionHandle = reviewNode?.data.handles?.find(
      (handle) => handle.id === "workstation-on-rejection-source",
    );

    expect(
      visibleGraphEdges.some(
        (edge) =>
          edge.fromNodeId === "workstation:review" &&
          edge.outcomeKind === "rejected",
      ),
    ).toBe(true);
    expect(rejectionHandle).toEqual(
      expect.objectContaining({
        connectable: false,
        hidden: true,
        id: "workstation-on-rejection-source",
      }),
    );
  });

  it("includes continue and reject editor handles when stopWords are configured", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          name: "draft",
          stopWords: ["DONE"],
        },
      ],
    };
    const snapshot = buildSampleFactorySnapshot(factory);
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
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:draft",
    );
    const handleIds = (workstationNode?.data.handles ?? []).map(
      (handle) => handle.id,
    );

    expect(handleIds).toContain("workstation-on-continue-source");
    expect(handleIds).toContain("workstation-on-rejection-source");
  });

  it("wires shared handle click actions back through the editor anchor callback", () => {
    const onConnectionAnchorClick = vi.fn();

    const handles = buildEditorHandles({
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick,
        pendingConnectionSource: {
          anchorId: "workstation-output-source",
          nodeId: "workstation:review",
        },
      },
      nodeId: "work-state:story:done",
      nodeKind: "work-state",
    });

    const successTarget = handles.find(
      (handle) => handle.id === "work-state-input-target",
    );

    expect(successTarget?.variant).toBe("valid-target");

    successTarget?.onButtonClick();

    expect(onConnectionAnchorClick).toHaveBeenCalledWith({
      anchorId: "work-state-input-target",
      nodeId: "work-state:story:done",
    });
  });
});

describe("current activity graph active item labels", () => {
  it("prefers token names, falls back through work item labels, and skips duplicate place labels", () => {
    const labelsByPlaceId = buildActiveItemLabelsByPlaceId([
      {
        consumed_tokens: [
          {
            name: "Explicit token label",
            place_id: "story:ready",
            token_id: "token-1",
            work_id: "work-1",
          },
          {
            place_id: "story:ready",
            token_id: "token-2",
            work_id: "work-2",
          },
          {
            place_id: "story:ready",
            token_id: "token-3",
            work_id: "work-3",
          },
          {
            place_id: "story:blocked",
            token_id: "token-4",
            work_id: "missing-work",
          },
        ],
        work_items: [
          {
            display_name: "Draft story",
            work_id: "work-2",
          },
          {
            work_id: "work-3",
          },
          {
            display_name: "Draft story",
            work_id: "work-duplicate",
          },
        ],
        workstation_node_id: "review",
      },
    ] as Parameters<typeof buildActiveItemLabelsByPlaceId>[0]);

    expect(labelsByPlaceId.get("story:ready")).toEqual([
      "Explicit token label",
      "Draft story",
      "work-3",
    ]);
    expect(labelsByPlaceId.get("story:blocked")).toEqual(["token-4"]);
  });
});
