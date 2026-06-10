import { act, renderHook } from "@testing-library/react";
import type { Edge, Node } from "@xyflow/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { graphEditorNodeDimensionsForKind } from "../lib/graph-editor-node-placement";
import { useCurrentActivityGraphCardViewModel } from "./use-current-activity-graph-card-view-model";

const graphViewModelMock = vi.hoisted(() => ({
  useCurrentActivityGraphViewModel: vi.fn(),
}));

vi.mock("./react-flow-current-activity-card-graph-view-model", () => ({
  useCurrentActivityGraphViewModel:
    graphViewModelMock.useCurrentActivityGraphViewModel,
}));

function graphControllerFixture() {
  const layoutEdgeId =
    "workstation-output:workstation:review->work-state:story:done";

  return {
    addControls: {},
    connectionControls: {
      handleAnchorClick: vi.fn(),
      pendingSource: null,
    },
    edgeLayoutId: layoutEdgeId,
    edgeWaypointControls: {
      canEditWaypoints: true,
      clearSelectedWaypointEdge: vi.fn(),
      handleAddSelectedEdgeWaypoint: vi.fn(),
      handleEditorEdgeClick: vi.fn(),
      handleEditorEdgeDoubleClick: vi.fn(),
      handleMoveSelectedEdgeWaypoint: vi.fn(),
      handleRemoveSelectedEdgeWaypoint: vi.fn(),
      selectedEdgeDescription: null,
      selectedEdgeWaypoints: [],
      selectedWaypointEdgeId: layoutEdgeId,
      waypointAriaLabel: vi.fn(),
      waypointControls: null,
    },
    editorControls: {
      activeTool: "connect",
      canInteract: true,
      connect: vi.fn(),
      connectionNotice: null,
      discardPendingChanges: vi.fn(),
      isEditing: true,
      selectTool: vi.fn(),
      toggleMode: vi.fn(),
    },
    graphProjection: {
      canonicalLayoutViewport: null,
      displayFactoryDefinition: null,
      graphLayout: { edges: [], height: 0, nodes: [], width: 0 },
      pendingAdditionEdgeIds: new Set<string>(),
      positionedGraphLayout: { edges: [], height: 0, nodes: [], width: 0 },
      renderedLayout: {
        edges: [
          {
            id: layoutEdgeId,
            waypoints: [{ x: 320, y: 180 }],
          },
        ],
        schemaVersion: 1,
      },
      visibleGraphEdges: [],
    },
    layoutControls: {
      addEdgeWaypoint: vi.fn(),
      canMoveLayout: true,
      currentLayout: {
        edges: [
          {
            id: layoutEdgeId,
            waypoints: [{ x: 320, y: 180 }],
          },
        ],
        schemaVersion: 1,
      },
      moveEdgeWaypoint: vi.fn(),
      removeEdgeWaypoint: vi.fn(),
    },
    leaveControls: {},
    removalControls: {
      deleteEdge: vi.fn(),
    },
    saveControls: {},
    status: {},
    validationControls: {
      targets: [],
    },
    visibilityControls: {},
  };
}

describe("useCurrentActivityGraphCardViewModel waypoint state", () => {
  beforeEach(() => {
    graphViewModelMock.useCurrentActivityGraphViewModel.mockReset();
  });

  it("passes selected waypoint state into the graph view-model", () => {
    const edgeLayoutId =
      "workstation-output:workstation:review->work-state:story:done";
    const edge: Edge = {
      data: {
        factoryGraphEdgeId: edgeLayoutId,
        kind: "workstation-output",
        label: "done",
        pendingStatus: "none",
      },
      id: edgeLayoutId,
      source: "workstation:review",
      target: "work-state:story:done",
    };
    const nodes: Node[] = [
      {
        data: {},
        id: "workstation:review",
        position: { x: 0, y: 0 },
      },
      {
        data: {},
        id: "work-state:story:done",
        position: { x: 400, y: 0 },
      },
    ];
    graphViewModelMock.useCurrentActivityGraphViewModel.mockReturnValue({
      canonicalLayoutViewport: null,
      edges: [edge],
      graphKey: "graph-key",
      handleNodesChange: vi.fn(),
      initialFitViewKey: "graph-key",
      initialFitViewOptions: { padding: 0.18 },
      nodes,
    });

    const graphController = graphControllerFixture();
    const { result } = renderHook(() =>
      useCurrentActivityGraphCardViewModel({
        graphController,
        locale: "en",
        now: Date.parse("2026-06-10T00:00:00Z"),
        onSelectDoc: vi.fn(),
        onSelectResource: vi.fn(),
        onSelectStateNode: vi.fn(),
        onSelectWorkID: vi.fn(),
        onSelectWorker: vi.fn(),
        onSelectWorkstation: vi.fn(),
        onSelectWorkType: vi.fn(),
        selection: null,
        snapshot: { factory: undefined } as never,
      } as never),
    );

    expect(result.current.edges[0]).toMatchObject({
      data: {
        factoryGraphEdgeId: edgeLayoutId,
      },
    });
    expect(
      graphViewModelMock.useCurrentActivityGraphViewModel.mock.calls[0]?.[0]
        ?.editor,
    ).toMatchObject({
      selectedWaypointEdgeId: edgeLayoutId,
    });
  });
});

describe("useCurrentActivityGraphCardViewModel add placement", () => {
  beforeEach(() => {
    graphViewModelMock.useCurrentActivityGraphViewModel.mockReset();
  });

  it("resolves add submit placement from the latest viewport snapshot and rendered nodes", () => {
    const submit = vi.fn();
    const nodes: Node[] = [];
    graphViewModelMock.useCurrentActivityGraphViewModel.mockReturnValue({
      canonicalLayoutViewport: null,
      edges: [],
      graphKey: "graph-key",
      handleNodesChange: vi.fn(),
      initialFitViewKey: "graph-key",
      initialFitViewOptions: { padding: 0.18 },
      nodes,
    });

    const graphController = {
      ...graphControllerFixture(),
      addControls: {
        draft: {
          kind: "worker",
          model: "gpt",
          modelProvider: "CURSOR",
          name: "reviewer",
          workerType: "MODEL_WORKER",
        },
        submit,
      },
    };
    const { result } = renderHook(() =>
      useCurrentActivityGraphCardViewModel({
        graphController,
        locale: "en",
        now: Date.parse("2026-06-10T00:00:00Z"),
        onSelectDoc: vi.fn(),
        onSelectResource: vi.fn(),
        onSelectStateNode: vi.fn(),
        onSelectWorkID: vi.fn(),
        onSelectWorker: vi.fn(),
        onSelectWorkstation: vi.fn(),
        onSelectWorkType: vi.fn(),
        selection: null,
        snapshot: { factory: undefined } as never,
      } as never),
    );

    act(() => {
      result.current.addControls.updatePlacementViewport({
        height: 800,
        viewport: { x: -100, y: -50, zoom: 2 },
        width: 1000,
      });
    });
    act(() => {
      result.current.addControls.submit();
    });

    const workerSize = graphEditorNodeDimensionsForKind("worker");
    expect(submit).toHaveBeenCalledWith({
      nodeId: "worker:reviewer",
      position: {
        x: 300 - workerSize.width / 2,
        y: 225 - workerSize.height / 2,
      },
    });
  });
});
