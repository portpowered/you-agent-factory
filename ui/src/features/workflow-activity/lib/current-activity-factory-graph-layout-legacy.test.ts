// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: legacy graph regression fixtures stay grouped around the projection they protect.
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildCurrentActivityGraphLayoutFromFactory } from "./current-activity-factory-graph-layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";

describe("current activity factory graph legacy replay layout", () => {
  it("hides internal system-time state places and expiry routes from the activity graph", async () => {
    const factory = {
      name: "System time factory",
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
        {
          name: "__system_time",
          states: [{ name: "pending", type: "PROCESSING" }],
        },
      ],
      workstations: [
        {
          id: "route-story",
          inputs: [
            { state: "new", workType: "story" },
            { state: "pending", workType: "__system_time" },
          ],
          name: "Route story",
          onContinue: [{ state: "pending", workType: "__system_time" }],
          outputs: [
            { state: "done", workType: "story" },
            { state: "pending", workType: "__system_time" },
          ],
          worker: "router",
        },
        {
          id: "__system_time:expire",
          inputs: [{ state: "pending", workType: "__system_time" }],
          name: "__system_time:expire",
          outputs: [],
          worker: "",
        },
      ],
      workers: [{ name: "router", type: "MODEL_WORKER" }],
    } as never;
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(factory);
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
      snapshot: semanticWorkflowDashboardSnapshot,
      storedNodePositions: EMPTY_NODE_POSITIONS,
    });

    expect(graphLayout.nodes.map((node) => node.nodeId)).not.toEqual(
      expect.arrayContaining([
        "work-state:__system_time:pending",
        "work-type:__system_time",
        "workstation:__system_time:expire",
      ]),
    );
    expect(visibleGraphEdges.map((edge) => edge.edgeId)).not.toEqual(
      expect.arrayContaining([
        "workstation-input:work-state:__system_time:pending->workstation:__system_time:expire",
      ]),
    );
    expect(nodes.map((node) => node.id)).not.toEqual(
      expect.arrayContaining([
        "work-state:__system_time:pending",
        "workstation:__system_time:expire",
      ]),
    );
    expect(
      nodes.find((node) => node.id === "workstation:Route story"),
    ).toBeTruthy();
  });

  it("collapses resource availability work states into the canonical resource node", async () => {
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory({
      name: "Legacy resource availability factory",
      resources: [{ capacity: 1, name: "executor-slot" }],
      workTypes: [
        {
          name: "executor-slot",
          states: [{ name: "available", type: "INITIAL" }],
        },
        {
          name: "task",
          states: [{ name: "init", type: "INITIAL" }],
        },
      ],
      workstations: [
        {
          id: "process",
          inputs: [
            { state: "init", workType: "task" },
            { state: "available", workType: "executor-slot" },
          ],
          name: "process",
          outputs: [],
          resources: [],
          type: "MODEL_WORKSTATION",
          worker: "processor",
        },
      ],
      workers: [{ name: "processor", type: "MODEL_WORKER" }],
    } as never);

    expect(graphLayout.nodes.map((node) => node.nodeId).sort()).toEqual([
      "resource:executor-slot",
      "work-state:task:init",
      "work-type:task",
      "worker:processor",
      "workstation:process",
    ]);
    expect(graphLayout.edges.map((edge) => edge.edgeId).sort()).toEqual(
      expect.arrayContaining([
        "workstation-resource:resource:executor-slot->workstation:process",
      ]),
    );
    expect(graphLayout.edges.map((edge) => edge.edgeId)).not.toContain(
      "workstation-input:work-state:executor-slot:available->workstation:process",
    );
  });

  it("keeps legacy replay factory route fields aligned with semantic handles", async () => {
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory({
      name: "Legacy replay factory",
      work_types: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", work_type: "story" }],
          name: "Review",
          on_failure: { state: "failed", work_type: "story" },
          outputs: [{ state: "done", work_type: "story" }],
          worker: "reviewer",
        },
      ],
    } as never);
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

    expect(graphLayout.nodes.map((node) => node.nodeId)).toEqual(
      expect.arrayContaining([
        "work-state:story:new",
        "work-state:story:done",
        "work-state:story:failed",
        "workstation:Review",
      ]),
    );
    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input:work-state:story:new->workstation:Review",
          sourceHandle: "workstation-input-source",
          targetHandle: "workstation-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-failure:workstation:Review->work-state:story:failed",
          sourceHandle: "workstation-on-failure-source",
          targetHandle: "work-state-input-target",
        }),
      ]),
    );
    expect(
      nodes.find((node) => node.id === "work-state:story:new")?.data.handles,
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input-source",
          type: "source",
        }),
      ]),
    );
  });
});
