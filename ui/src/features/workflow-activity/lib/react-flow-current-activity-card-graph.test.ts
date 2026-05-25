// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: shared graph-handle and pending-edge coverage stays grouped around one layout fixture seam.
import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { buildGraphLayout } from "../../flowchart/lib/layout";
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
    expect(workstationNode?.data.handles).toHaveLength(5);
    expect(workstationNode?.data.handles).toEqual(
      expect.not.arrayContaining([
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
          id: "workstation-output-target",
          label: "Success",
          type: "target",
        }),
      ]),
    );
    expect(stateNode?.data.handles).toHaveLength(5);
    expect(outputEdge).toMatchObject({
      sourceHandle: "workstation-output-source",
      targetHandle: "workstation-output-target",
    });
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
      targetHandleId: "workstation-on-rejection-target",
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
      targetHandleId: "workstation-on-failure-target",
    });
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
      nodeId: "place:story:done",
      nodeKind: "work-state",
    });

    const successTarget = handles.find(
      (handle) => handle.id === "workstation-output-target",
    );

    expect(successTarget?.variant).toBe("valid-target");

    successTarget?.onButtonClick();

    expect(onConnectionAnchorClick).toHaveBeenCalledWith({
      anchorId: "workstation-output-target",
      nodeId: "place:story:done",
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
