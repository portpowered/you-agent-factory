// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction lint/style/noExcessiveLinesPerFile: shared graph-handle and pending-edge coverage stays grouped around one layout fixture seam.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { FactoryValidationTarget } from "../../../api/factory-validation";
import { factoryFromDashboardTopology } from "../../../components/dashboard/fixtures";
import { mediumBranchingDashboardTopology } from "../../../components/dashboard/fixtures/topologies";
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { resolveDashboardSelection } from "../../current-selection/base/public";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft.test-helpers";
import { buildFactoryGraphTopologyFromDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-graph";
import { factoryGraphConnectionAnchorContext } from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import {
  SYSTEM_TIME_EXPIRY_TRANSITION_ID,
  SYSTEM_TIME_WORK_TYPE_ID,
  systemTimeGraphNodeId,
} from "../../factory-graph-editor/lib/operations/factory-graph-customer-display";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { buildGraphLayout } from "../../flowchart/lib/layout";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "./current-activity-factory-graph-layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildSemanticGraphHandles,
  supportedEditorHandleIdsForEdge,
} from "./react-flow-current-activity-card-editor-handles";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
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

describe("dashboardWorkstationFromFactory", () => {
  it("normalizes legacy single input routes", () => {
    const workstation = dashboardWorkstationFromFactory({
      inputs: { state: "queued", workType: "story" },
      name: "draft",
      outputs: [],
    } as CanonicalFactoryDefinition["workstations"][number]);

    expect(workstation.input_place_ids).toEqual(["story:queued"]);
    expect(workstation.input_places).toEqual([
      {
        kind: "work_state",
        place_id: "story:queued",
        state_value: "queued",
        type_id: "story",
      },
    ]);
  });
});

describe("current activity graph editor handles", () => {
  it("marks factory-derived workstation nodes active from runtime execution ids", async () => {
    const factory = baseFactoryDefinition;
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const activeExecution = {
      dispatch_id: "dispatch-review-active",
      started_at: "2026-06-09T00:00:00Z",
      transition_id: "draft",
      workstation_name: "draft",
      workstation_node_id: "draft",
      work_items: [
        {
          display_name: "Active Story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
    } satisfies DashboardActiveExecution;
    const snapshot = {
      ...buildSampleFactorySnapshot(factory),
      topology: {
        edges: [],
        workstation_node_ids: [],
        workstation_nodes_by_id: {},
      },
    } satisfies DashboardSnapshot;

    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {
        draft: [activeExecution],
      },
      activeGraphHighlights: buildActiveGraphHighlights(
        [activeExecution],
        visibleGraphEdges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-06-09T00:00:04Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "node", nodeId: "draft" },
      snapshot,
    });

    expect(
      nodes.find((node) => node.id === "workstation:draft")?.data,
    ).toMatchObject({
      active: true,
      activeFlow: true,
      selectedWorkstation: true,
      workstation: expect.objectContaining({ node_id: "draft" }),
    });
  });

  it("projects work-state state_category on factory graph layout place nodes", async () => {
    const factory = factoryFromDashboardTopology(
      mediumBranchingDashboardTopology,
    );
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const initNode = graphLayout.nodes.find(
      (node) =>
        node.nodeKind === "state_position" &&
        node.place.place_id === "story:init",
    );
    const blockedNode = graphLayout.nodes.find(
      (node) =>
        node.nodeKind === "state_position" &&
        node.place.place_id === "story:blocked",
    );

    expect(initNode?.place.state_category).toBe("INITIAL");
    expect(blockedNode?.place.state_category).toBe("FAILED");
  });

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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: resolvedSelection,
      snapshot,
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

  it("highlights work-state nodes selected by factory-graph node id", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const resolvedSelection = resolveDashboardSelection({
      selection: { kind: "node", nodeId: "work-state:task:complete" },
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
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: resolvedSelection,
      snapshot,
    });

    expect(resolvedSelection).toEqual({
      kind: "node",
      nodeId: "work-state:task:complete",
    });
    expect(
      nodes.find((node) => node.id === "work-state:task:complete")?.data,
    ).toMatchObject({
      kind: "work-state",
      selectedStateNode: true,
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
        systemTimeGraphNodeId("workstation", "route-story", "Route story"),
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });

    expect(nodes.find((node) => node.id === "work-type:task")).toMatchObject({
      data: {
        factoryGraphNodeId: "work-type:task",
        kind: "work-type",
      },
      type: "workType",
    });
  });

  it("marks default work-type nodes from the canonical factory definition", async () => {
    const factory = {
      ...loadSampleFactoryDefinition(),
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "task",
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
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
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });

    expect(
      nodes.find((node) => node.id === "work-type:task")?.data,
    ).toMatchObject({
      isDefaultWorkType: true,
      kind: "work-type",
    });
  });

  it("wires dashboard-view work type nodes to onSelectWorkType and selectedWorkType", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const onSelectWorkType = vi.fn();
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType,
      onSelectWorkstation: vi.fn(),
      selection: { kind: "work-type", workTypeName: "task" },
      snapshot,
    });

    const workTypeNode = nodes.find((node) => node.id === "work-type:task");
    expect(workTypeNode?.data).toMatchObject({
      kind: "work-type",
      onSelectWorkType: expect.any(Function),
      selectedWorkType: true,
    });

    workTypeNode?.data.onSelectWorkType?.("task");

    expect(onSelectWorkType).toHaveBeenCalledWith("task");
  });

  it("wires editor-mode work type nodes to onSelectWorkType instead of onSelectWorkstation", async () => {
    const factory = loadSampleFactoryDefinition();
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const onSelectWorkType = vi.fn();
    const onSelectWorkstation = vi.fn();
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        visibleGraphEdges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "select",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType,
      onSelectWorkstation,
      selection: { kind: "work-type", workTypeName: "task" },
      snapshot,
    });

    const workTypeNode = nodes.find((node) => node.id === "work-type:task");
    expect(workTypeNode?.data).toMatchObject({
      kind: "work-type",
      onSelectWorkType: expect.any(Function),
      selectedWorkType: true,
    });
    expect(workTypeNode?.data).not.toHaveProperty("onSelectGraphNode");

    workTypeNode?.data.onSelectWorkType?.("task");

    expect(onSelectWorkType).toHaveBeenCalledWith("task");
    expect(onSelectWorkstation).not.toHaveBeenCalled();
  });

  it.each([
    ["delete", false],
    ["add", true],
    ["connect", true],
    [null, true],
  ] as const)("omits graph node selection callbacks in delete mode and restores them for other tools (activeTool=%s)", async (activeTool, expectSelectionCallbacks) => {
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
      editor: {
        activeTool,
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });

    const workerNode = nodes.find((node) => node.id === "worker:processor");
    const resourceNode = nodes.find(
      (node) => node.id === "resource:executor-slot",
    );
    const workTypeNode = nodes.find((node) => node.id === "work-type:task");
    const workStateNode = nodes.find(
      (node) => node.id === "work-state:task:init",
    );
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:process",
    );

    if (expectSelectionCallbacks) {
      expect(workerNode?.data).toHaveProperty("onSelectWorker");
      expect(resourceNode?.data).toHaveProperty("onSelectResource");
      expect(workTypeNode?.data).toHaveProperty("onSelectWorkType");
      expect(workStateNode?.data).toHaveProperty("onSelectStateNode");
      expect(workstationNode?.data).toHaveProperty("onSelectWorkstation");
      expect(workstationNode?.data).toHaveProperty("onSelectWorkID");
    } else {
      expect(workerNode?.data).not.toHaveProperty("onSelectWorker");
      expect(resourceNode?.data).not.toHaveProperty("onSelectResource");
      expect(workTypeNode?.data).not.toHaveProperty("onSelectWorkType");
      expect(workStateNode?.data).not.toHaveProperty("onSelectStateNode");
      expect(workstationNode?.data).not.toHaveProperty("onSelectWorkstation");
      expect(workstationNode?.data).not.toHaveProperty("onSelectWorkID");
    }
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
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
      onSelectResource: expect.any(Function),
      selectedResource: false,
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

  it("marks the selected resource node while resource selection is active", async () => {
    const factory = {
      ...baseFactoryDefinition,
      resources: [{ capacity: 1, name: "gpu" }],
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "resource", resourceName: "gpu" },
      snapshot: {
        factory,
        runtime: { place_token_counts: {} },
        topology: { workstation_nodes_by_id: {} },
      } as never,
    });

    expect(
      nodes.find((node) => node.id === "resource:gpu")?.data,
    ).toMatchObject({
      kind: "resource",
      selectedResource: true,
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "worker", workerName: "writer" },
      snapshot: {
        factory,
        runtime: { place_token_counts: {} },
        topology: { workstation_nodes_by_id: {} },
      } as never,
    });

    expect(
      nodes.find((node) => node.id === "worker:writer")?.data,
    ).toMatchObject({
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
    if (resourceNode?.nodeKind !== "resource") {
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: semanticWorkflowDashboardSnapshot,
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
    expect(workstationNode?.data.zAxisIncompleteHints).toBeNull();
  });

  it("renders visible continue and reject handles for authored edges when the assigned worker has stopToken", async () => {
    const factory = loadSampleFactoryDefinition();
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
        connectable: true,
        hidden: undefined,
        id: "workstation-on-rejection-source",
      }),
    );
    expect(reviewNode?.data.zAxisIncompleteHints).toBeNull();
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
    });
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:draft",
    );
    const handleIds = (workstationNode?.data.handles ?? []).map(
      (handle) => handle.id,
    );

    expect(handleIds).toContain("workstation-on-continue-source");
    expect(handleIds).toContain("workstation-on-rejection-source");
    expect(workstationNode?.data.zAxisIncompleteHints).toBeNull();
  });

  it("omits z-axis incomplete hints outside connect editor mode", async () => {
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
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });
    const workstationNode = nodes.find(
      (node) => node.id === "workstation:draft",
    );

    expect(workstationNode?.data.zAxisIncompleteHints).toBeNull();
  });

  it("omits progress-outcome edges when rendered workstation nodes omit those handles", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
          stopWords: undefined,
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(
      visibleGraphEdges,
      graphLayout.nodes,
    );
    const observerNodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });
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
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });
    expect(
      visibleGraphEdges.some((edge) => edge.outcomeKind === "rejected"),
    ).toBe(true);
    expect(
      visibleGraphEdges.some((edge) => edge.outcomeKind === "continue"),
    ).toBe(true);

    const observerEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      observerNodes,
    );
    const editorEdges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      editorNodes,
    );

    expect(
      observerEdges.some(
        (edge) => edge.sourceHandle === "workstation-on-rejection-source",
      ),
    ).toBe(false);
    expect(
      observerEdges.some(
        (edge) => edge.sourceHandle === "workstation-on-continue-source",
      ),
    ).toBe(false);
    expect(
      editorEdges.some(
        (edge) => edge.sourceHandle === "workstation-on-rejection-source",
      ),
    ).toBe(false);
    expect(
      editorEdges.some(
        (edge) => edge.sourceHandle === "workstation-on-continue-source",
      ),
    ).toBe(false);
    expect(
      observerEdges.some(
        (edge) => edge.sourceHandle === "workstation-on-failure-source",
      ),
    ).toBe(true);
    expect(
      observerEdges.some(
        (edge) => edge.sourceHandle === "workstation-output-source",
      ),
    ).toBe(true);
  });

  it("keeps progress-outcome edges when stopWords render continue and reject handles", async () => {
    const factory = {
      ...baseFactoryDefinition,
      workTypes: [
        {
          name: "story",
          states: [
            { name: "queued", type: "INITIAL" },
            { name: "rejected", type: "FAILED" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          ...baseFactoryDefinition.workstations?.[0],
          behavior: "STANDARD",
          onContinue: [{ state: "queued", workType: "story" }],
          onFailure: [{ state: "rejected", workType: "story" }],
          onRejection: [{ state: "rejected", workType: "story" }],
          stopWords: ["DONE"],
        },
      ],
    } satisfies CanonicalFactoryDefinition;
    const snapshot = buildSampleFactorySnapshot(factory);
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(
      visibleGraphEdges,
      graphLayout.nodes,
    );
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], visibleGraphEdges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: false,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      new Set(),
      visibleGraphEdges,
      nodes,
    );

    expect(
      edges.some(
        (edge) => edge.sourceHandle === "workstation-on-rejection-source",
      ),
    ).toBe(true);
    expect(
      edges.some(
        (edge) => edge.sourceHandle === "workstation-on-continue-source",
      ),
    ).toBe(true);
  });

  it("wires shared handle click actions back through the editor anchor callback", () => {
    const onConnectionAnchorClick = vi.fn();

    const handles = buildSemanticGraphHandles({
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

  it("leaves validation markers unset when validationTargets default to empty", async () => {
    const factory = loadSampleFactoryDefinition();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], graphLayout.edges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: null,
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
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: buildSampleFactorySnapshot(factory),
    });

    expect(
      nodes.find((node) => node.id === "work-type:task")?.data,
    ).toMatchObject({
      validationError: false,
      validationMessage: undefined,
    });
  });

  it("marks work type and work state nodes with validation error treatment from canonical targets", async () => {
    const factory = loadSampleFactoryDefinition();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const validationTargets: FactoryValidationTarget[] = [
      {
        code: "factory.workType.missingCompletionState",
        message: 'work type "task" must declare a completion state.',
        severity: "error",
        subject: {
          id: "task",
          location: "STATES",
          type: "WORK_TYPE",
        },
      },
      {
        code: "factory.workState.missingTerminalCompletionPath",
        message: 'work state "task:init" has no terminal completion path.',
        severity: "error",
        subject: {
          id: "task:init",
          location: "TERMINAL",
          type: "WORK_STATE",
        },
      },
    ];
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], graphLayout.edges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: null,
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      validationTargets,
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: buildSampleFactorySnapshot(factory),
    });

    expect(
      nodes.find((node) => node.id === "work-type:task")?.data,
    ).toMatchObject({
      validationError: true,
      validationMessage: 'work type "task" must declare a completion state.',
    });
    expect(
      nodes.find((node) => node.id === "work-state:task:init")?.data,
    ).toMatchObject({
      validationError: true,
      validationMessage:
        'work state "task:init" has no terminal completion path.',
    });
  });

  it("marks workstation handles with validation error treatment from canonical targets", async () => {
    const factory = loadSampleFactoryDefinition();
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const validationTargets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation process must define a reject route.",
        severity: "error",
        subject: {
          id: "process",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "review" must define a failure route.',
        severity: "error",
        subject: {
          id: "review",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
      {
        code: "factory.workstation.conflictingWorkStateOutputs",
        message:
          'Workstation "process" routes work type "task" to conflicting output states.',
        severity: "error",
        subject: {
          id: "process",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ];
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], graphLayout.edges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: null,
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      validationTargets,
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: buildSampleFactorySnapshot(factory),
    });
    const processNode = nodes.find((node) => node.id === "workstation:process");
    const reviewNode = nodes.find((node) => node.id === "workstation:review");

    expect(processNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-on-rejection-source",
          buttonAriaLabel: "Workstation process must define a reject route.",
          validationError: true,
          validationMessage: "Workstation process must define a reject route.",
          variant: "error",
        }),
        expect.objectContaining({
          id: "workstation-output-source",
          validationError: true,
          validationMessage:
            'Workstation "process" routes work type "task" to conflicting output states.',
          variant: "error",
        }),
      ]),
    );
    expect(reviewNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-on-failure-source",
          validationError: true,
          validationMessage:
            'Workstation "review" must define a failure route.',
          variant: "error",
        }),
      ]),
    );
  });

  it("suppresses ON_REJECTION handle validation on standard processors without stopWords", () => {
    const connectionAnchorContext = factoryGraphConnectionAnchorContext({
      type: "MODEL_WORKSTATION",
      behavior: "STANDARD",
    });
    const validationProjection = projectFactoryValidationTargets([
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation draft must define a reject route.",
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ]);
    const handles = buildSemanticGraphHandles({
      connectionAnchorContext,
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      nodeId: "workstation:draft",
      nodeKind: "workstation",
      validationProjection,
    });

    expect(handles).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-on-rejection-source",
          validationError: true,
        }),
        expect.objectContaining({
          id: "workstation-on-continue-source",
          validationError: true,
        }),
      ]),
    );
    expect(
      handles.find((handle) => handle.id === "workstation-on-failure-source"),
    ).toEqual(
      expect.objectContaining({
        validationError: false,
        validationMessage: undefined,
      }),
    );
  });

  it("keeps ON_REJECTION handle validation when reject anchors are rendered", () => {
    const connectionAnchorContext = factoryGraphConnectionAnchorContext({
      type: "MODEL_WORKSTATION",
      behavior: "REPEATER",
    });
    const validationProjection = projectFactoryValidationTargets([
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation process must define a reject route.",
        severity: "error",
        subject: {
          id: "process",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ]);
    const handles = buildSemanticGraphHandles({
      connectionAnchorContext,
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      nodeId: "workstation:process",
      nodeKind: "workstation",
      validationProjection,
    });

    expect(handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-on-rejection-source",
          validationError: true,
          validationMessage: "Workstation process must define a reject route.",
          variant: "error",
        }),
      ]),
    );
  });

  it("suppresses ON_REJECTION validation on hidden authored reject handles without stop markers", async () => {
    const sampleFactory = loadSampleFactoryDefinition();
    const factory = {
      ...sampleFactory,
      workers: sampleFactory.workers?.map((worker) => ({
        ...worker,
        stopToken: undefined,
      })),
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const validationTargets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: 'Workstation "review" must define a reject route.',
        severity: "error",
        subject: {
          id: "review",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ];
    const nodes = buildCurrentActivityNodes({
      onSelectDoc: vi.fn(),
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights([], graphLayout.edges),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      editor: {
        activeTool: "connect",
        canInteractWithEditor: true,
        editorMode: true,
        onConnectionAnchorClick: vi.fn(),
        pendingConnectionSource: null,
      },
      validationTargets,
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-05-24T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot: buildSampleFactorySnapshot(factory),
    });
    const reviewNode = nodes.find((node) => node.id === "workstation:review");
    const rejectionHandle = reviewNode?.data.handles?.find(
      (handle) => handle.id === "workstation-on-rejection-source",
    );

    expect(rejectionHandle).toEqual(
      expect.objectContaining({
        hidden: true,
        validationError: false,
        validationMessage: undefined,
        variant: "default",
      }),
    );
  });

  it("renders bundled docs as distinct selectable doc nodes", async () => {
    const factory = {
      ...baseFactoryDefinition,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Guide" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
          {
            content: { encoding: "utf-8", inline: "print('setup')" },
            targetPath: "factory/scripts/setup-workspace.py",
            type: "SCRIPT",
          },
        ],
      },
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const onSelectDoc = vi.fn();

    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        graphLayout.edges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-06-08T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc,
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "doc", targetPath: "factory/docs/guide.md" },
      snapshot: buildSampleFactorySnapshot(factory),
    });

    const docNode = nodes.find(
      (node) => node.id === "doc:factory/docs/guide.md",
    );
    const scriptNode = nodes.find(
      (node) => node.id === "doc:factory/scripts/setup-workspace.py",
    );

    expect(docNode).toMatchObject({
      type: "doc",
      data: {
        displayLabel: "guide.md",
        fileType: "DOC",
        kind: "doc",
        onSelectDoc,
        selectedDoc: true,
        targetPath: "factory/docs/guide.md",
      },
    });
    expect(scriptNode).toMatchObject({
      type: "doc",
      data: {
        displayLabel: "setup-workspace.py",
        fileType: "SCRIPT",
        kind: "doc",
        selectedDoc: false,
        targetPath: "factory/scripts/setup-workspace.py",
      },
    });
    expect(scriptNode?.data).not.toHaveProperty("onSelectDoc");
  });

  it("renders nested bundled docs as distinct selectable doc nodes", async () => {
    const nestedDocPath = "factory/docs/standards/review.md";
    const factory = {
      ...baseFactoryDefinition,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Review standards" },
            targetPath: nestedDocPath,
            type: "DOC",
          },
        ],
      },
    };
    const graphLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const onSelectDoc = vi.fn();

    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        graphLayout.edges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      factoryDefinition: factory,
      graphLayout,
      now: Date.parse("2026-06-08T00:00:00Z"),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectDoc,
      onSelectResource: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: { kind: "doc", targetPath: nestedDocPath },
      snapshot: buildSampleFactorySnapshot(factory),
    });

    const docNode = nodes.find((node) => node.id === `doc:${nestedDocPath}`);

    expect(docNode).toMatchObject({
      type: "doc",
      data: {
        displayLabel: "review.md",
        fileType: "DOC",
        kind: "doc",
        onSelectDoc,
        selectedDoc: true,
        targetPath: nestedDocPath,
      },
    });
  });

  it("updates doc nodes when the saved factory document changes", async () => {
    const factory = {
      ...baseFactoryDefinition,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/overview.md",
            type: "DOC",
          },
        ],
      },
    };
    const initialLayout =
      await buildCurrentActivityGraphLayoutFromFactory(factory);
    const renamedFactory = {
      ...factory,
      supportingFiles: {
        bundledFiles: [
          {
            content: { encoding: "utf-8", inline: "# Overview" },
            targetPath: "factory/docs/guide.md",
            type: "DOC",
          },
        ],
      },
    };
    const refreshedLayout =
      await buildCurrentActivityGraphLayoutFromFactory(renamedFactory);

    expect(initialLayout.nodes.map((node) => node.nodeId)).toContain(
      "doc:factory/docs/overview.md",
    );
    expect(refreshedLayout.nodes.map((node) => node.nodeId)).toContain(
      "doc:factory/docs/guide.md",
    );
    expect(refreshedLayout.nodes.map((node) => node.nodeId)).not.toContain(
      "doc:factory/docs/overview.md",
    );
  });
});
