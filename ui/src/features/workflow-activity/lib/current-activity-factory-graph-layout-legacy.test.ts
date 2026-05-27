import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import { buildCurrentActivityGraphLayoutFromFactory } from "./current-activity-factory-graph-layout";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";

describe("current activity factory graph legacy replay layout", () => {
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
      handleAssignments,
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
        "place:story:new",
        "place:story:done",
        "place:story:failed",
        "workstation:Review",
      ]),
    );
    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input:place:story:new->workstation:Review",
          sourceHandle: "workstation-input-source",
          targetHandle: "workstation-input-target",
        }),
        expect.objectContaining({
          id: "workstation-on-failure:workstation:Review->place:story:failed",
          sourceHandle: "workstation-on-failure-source",
          targetHandle: "workstation-on-failure-target",
        }),
      ]),
    );
    expect(
      nodes.find((node) => node.id === "place:story:new")?.data.handles,
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
