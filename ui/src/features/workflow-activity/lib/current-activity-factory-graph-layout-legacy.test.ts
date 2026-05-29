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
