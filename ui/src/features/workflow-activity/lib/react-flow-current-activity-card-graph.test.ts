import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildGraphLayout } from "../../flowchart/lib/layout";
import { buildGraphEdges } from "./react-flow-current-activity-card-edges";
import { buildVisibleGraphEdgesWithDraft } from "./react-flow-current-activity-card-draft-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";

describe("current activity graph editor handles", () => {
  it("binds shared workstation and work-state edges to the editor anchor ids", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const handleAssignments = buildHandleAssignments(visibleGraphEdges, {
      editorMode: true,
    });
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

    const workstationNode = nodes.find(
      (node) => node.id === "workstation:review",
    );
    const outputEdge = edges.find(
      (edge) =>
        edge.source === "workstation:review" &&
        edge.sourceHandle === "workstation-output-source" &&
        edge.targetHandle === "workstation-output-target",
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
    expect(stateNode?.data.kind).toBe("work-state");
    expect(stateNode?.data.handles).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-input-source",
          label: "Input",
          type: "source",
        }),
        expect.objectContaining({
          id: "workstation-output-target",
          label: "Success",
          type: "target",
        }),
      ]),
    );
    expect(outputEdge).toMatchObject({
      sourceHandle: "workstation-output-source",
      targetHandle: "workstation-output-target",
    });
  });

});

describe("current activity graph draft edges", () => {
  it("adds supported pending draft routes onto the shared observer graph surface", async () => {
    const graphLayout = await buildGraphLayout(
      semanticWorkflowDashboardSnapshot.topology,
    );
    const { pendingAdditionEdgeIds, visibleGraphEdges } =
      buildVisibleGraphEdgesWithDraft({
        draft: {
          additions: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
          edgeChanges: {
            additions: [
              {
                kind: "workstation-output",
                source: { kind: "workstation", name: "review" },
                target: {
                  kind: "work-state",
                  stateName: "blocked",
                  workTypeName: "story",
                },
              },
            ],
            removals: [],
          },
          removals: {
            resources: [],
            workers: [],
            workStates: [],
            workTypes: [],
            workstations: [],
          },
        },
        graphLayout,
      });
    const handleAssignments = buildHandleAssignments(visibleGraphEdges, {
      editorMode: true,
    });
    const edges = buildGraphEdges(
      buildActiveGraphHighlights([], visibleGraphEdges),
      handleAssignments,
      pendingAdditionEdgeIds,
      visibleGraphEdges,
    );

    expect(pendingAdditionEdgeIds).toEqual(
      new Set([
        "workstation-output:workstation:review->place:story:blocked",
      ]),
    );
    expect(edges).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "workstation-output:workstation:review->place:story:blocked",
          source: "workstation:review",
          sourceHandle: "workstation-output-source",
          style: expect.objectContaining({
            stroke: "var(--color-af-warning-ink)",
            strokeDasharray: "9 4",
          }),
          target: "place:story:blocked",
          targetHandle: "workstation-output-target",
        }),
      ]),
    );
  });
});
