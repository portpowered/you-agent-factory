import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildCurrentActivityNodes,
  buildHandleAssignments,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";
import { buildCurrentActivityGraphLayoutFromFactory } from "./current-activity-factory-graph-layout";

const workstationChainFactory = {
  name: "work-state-bypass-activity-fixture",
  resources: [],
  workers: [{ name: "reviewer" }, { name: "drafter" }],
  workTypes: [
    {
      name: "story",
      states: [
        { name: "queued", type: "INITIAL" as const },
        { name: "done", type: "TERMINAL" as const },
      ],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
    {
      inputs: [{ state: "done", workType: "story" }],
      name: "draft",
      outputs: [],
      worker: "drafter",
    },
  ],
} satisfies CanonicalFactoryDefinition;

const resourceFixtureFactory = {
  ...workstationChainFactory,
  resources: [{ capacity: 1, name: "gpu" }],
  workstations: [
    {
      ...workstationChainFactory.workstations[0],
      resources: [{ capacity: 1, name: "gpu" }],
    },
    workstationChainFactory.workstations[1],
  ],
} satisfies CanonicalFactoryDefinition;

async function expectBypassEdgesRenderWithHandles(
  hiddenStateLayout: Awaited<
    ReturnType<typeof buildCurrentActivityGraphLayoutFromFactory>
  >,
) {
  const bypassEdge = hiddenStateLayout.edges.find((edge) =>
    edge.edgeId.startsWith("work-state-visibility-bypass:"),
  );
  expect(bypassEdge).toMatchObject({
    fromNodeId: "workstation:review",
    label: "Success route",
    toNodeId: "workstation:draft",
  });

  const nodes = buildCurrentActivityNodes({
    activeExecutionsByWorkstationNodeID: {},
    activeGraphHighlights: {
      activeEdgeIds: new Set(),
      activePlaceNodeIds: new Set(),
      activeWorkstationNodeIds: new Set(),
      hasActiveFlow: false,
      relatedNodeIds: new Set(),
    },
    activeItemLabelsByPlaceId: new Map(),
    editor: {
      activeTool: null,
      canInteractWithEditor: false,
      editorMode: false,
      onConnectionAnchorClick: () => undefined,
      pendingConnectionSource: null,
    },
    graphLayout: hiddenStateLayout,
    now: 0,
    onSelectStateNode: () => undefined,
    onSelectWorkID: () => undefined,
    onSelectWorker: () => undefined,
    onSelectWorkType: () => undefined,
    onSelectWorkstation: () => undefined,
    selection: null,
    snapshot: workstationChainSnapshot(workstationChainFactory),
    storedNodePositions: EMPTY_NODE_POSITIONS,
  });
  const handleAssignments = buildHandleAssignments(
    hiddenStateLayout.edges,
    hiddenStateLayout.nodes,
  );
  const reactFlowEdges = buildGraphEdges(
    {
      activeEdgeIds: new Set(),
      activePlaceNodeIds: new Set(),
      activeWorkstationNodeIds: new Set(),
      hasActiveFlow: false,
      relatedNodeIds: new Set(),
    },
    handleAssignments,
    new Set(),
    hiddenStateLayout.edges,
    nodes,
  );

  expect(reactFlowEdges).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: bypassEdge?.edgeId,
        source: "workstation:review",
        sourceHandle: "workstation-output-source",
        target: "workstation:draft",
        targetHandle: "workstation-input-target",
      }),
    ]),
  );
}

function workstationChainSnapshot(
  factory: CanonicalFactoryDefinition,
): DashboardSnapshot {
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
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 0,
  };
}

describe("buildCurrentActivityGraphLayoutFromFactory work-state visibility", () => {
  it("hides work states and renders workstation bypass edges with valid handles", async () => {
    const visibleLayout = await buildCurrentActivityGraphLayoutFromFactory(
      workstationChainFactory,
    );
    const hiddenStateLayout = await buildCurrentActivityGraphLayoutFromFactory(
      workstationChainFactory,
      new Set(["work-state"]),
    );

    expect(visibleLayout.nodes.map((node) => node.nodeId)).toEqual(
      expect.arrayContaining([
        "work-state:story:done",
        "work-state:story:queued",
      ]),
    );
    expect(hiddenStateLayout.nodes.map((node) => node.nodeId)).not.toEqual(
      expect.arrayContaining([
        "work-state:story:done",
        "work-state:story:queued",
      ]),
    );

    await expectBypassEdgesRenderWithHandles(hiddenStateLayout);
  });

  it("restores state-mediated edges when work states are shown again", async () => {
    const hiddenStateLayout = await buildCurrentActivityGraphLayoutFromFactory(
      workstationChainFactory,
      new Set(["work-state"]),
    );
    const visibleLayout = await buildCurrentActivityGraphLayoutFromFactory(
      workstationChainFactory,
      new Set(),
    );

    expect(
      hiddenStateLayout.edges.some((edge) =>
        edge.edgeId.startsWith("work-state-visibility-bypass:"),
      ),
    ).toBe(true);
    expect(
      visibleLayout.edges.some((edge) =>
        edge.edgeId.startsWith("workstation-output:"),
      ),
    ).toBe(true);
    expect(
      visibleLayout.edges.some((edge) =>
        edge.edgeId.startsWith("workstation-input:"),
      ),
    ).toBe(true);
    expect(
      visibleLayout.edges.some((edge) =>
        edge.edgeId.startsWith("work-state-visibility-bypass:"),
      ),
    ).toBe(false);
  });
});

describe("buildCurrentActivityGraphLayoutFromFactory non-bypass visibility", () => {
  it("hides worker or resource classes without synthesizing bypass edges", async () => {
    const hiddenWorkerLayout = await buildCurrentActivityGraphLayoutFromFactory(
      resourceFixtureFactory,
      new Set(["worker"]),
    );
    const hiddenResourceLayout =
      await buildCurrentActivityGraphLayoutFromFactory(
        resourceFixtureFactory,
        new Set(["resource"]),
      );

    expect(hiddenWorkerLayout.nodes.map((node) => node.nodeId)).not.toContain(
      "worker:reviewer",
    );
    expect(
      hiddenResourceLayout.nodes.map((node) => node.nodeId),
    ).not.toContain("resource:gpu");
    expect(
      hiddenWorkerLayout.edges.some((edge) =>
        edge.edgeId.startsWith("work-state-visibility-bypass:"),
      ),
    ).toBe(false);
    expect(
      hiddenResourceLayout.edges.some((edge) =>
        edge.edgeId.startsWith("work-state-visibility-bypass:"),
      ),
    ).toBe(false);
  });
});
