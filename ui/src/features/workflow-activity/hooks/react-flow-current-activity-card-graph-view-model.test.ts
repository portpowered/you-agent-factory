// biome-ignore lint/style/noExcessiveLinesPerFile: graph view-model selection contract cases stay together.
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  buildDivergentPlaneDashboardSnapshot,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import { sessionFactoryDocumentFromSnapshot } from "../../../testing/session-factory-mocks";
import type { GraphLayout } from "../../flowchart/lib/layout";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { currentActivityCardFactoryDefinition } from "./current-activity-card-factory-definition";
import { useCurrentActivityGraphViewModel } from "./react-flow-current-activity-card-graph-view-model";

function createEditorStub(
  overrides: {
    draftState?: ReturnType<typeof createMockGraphEditorDraftState>;
    editableFactoryDocument?: typeof baseFactoryDefinitionDocument;
    editableFactoryDocumentStatus?: "error" | "pending" | "success";
    editorMode?: boolean;
  } = {},
) {
  const editableFactoryDocument =
    "editableFactoryDocument" in overrides
      ? overrides.editableFactoryDocument
      : baseFactoryDefinitionDocument;
  const draftState = overrides.draftState ?? createMockGraphEditorDraftState();

  return {
    baseFactoryDocument: draftState.baseDocument,
    editableFactoryDocument,
    editableFactoryDocumentStatus:
      overrides.editableFactoryDocumentStatus ?? "success",
    editorMode: overrides.editorMode ?? false,
    latestFactoryDocument: draftState.latestDocument,
    pendingFactoryDefinition: draftState.pendingFactoryDefinition,
  } as Parameters<typeof currentActivityCardFactoryDefinition>[0];
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this suite keeps the factory selection contract cases together.
describe("currentActivityCardFactoryDefinition", () => {
  it("returns the timeline snapshot in observe mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocumentStatus: "pending",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns the event-computed snapshot factory in observe mode while shared draft state has a saved document", () => {
    const savedDocument = {
      ...baseFactoryDefinitionDocument,
      layout: {
        nodes: [
          {
            id: "workstation:draft",
            position: { x: 540, y: 260 },
          },
        ],
        schemaVersion: 1 as const,
        viewport: { x: 14, y: 18, zoom: 1.2 },
      },
    };
    const snapshot = {
      ...structuredClone(singleNodeDashboardSnapshot),
      factory: {
        ...savedDocument,
        layout: undefined,
      },
    };

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState: createMockGraphEditorDraftState({
            baseDocument: savedDocument,
            latestDocument: savedDocument,
          }),
          editableFactoryDocument: undefined,
          editableFactoryDocumentStatus: "pending",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns the event-computed snapshot factory in observe mode once the scoped factory document succeeds", () => {
    const snapshot = buildDivergentPlaneDashboardSnapshot();

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocument: divergentDocumentPlaneFactoryDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(snapshot.factory);
  });

  it("returns null in editor mode while the scoped factory document is pending", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocumentStatus: "pending",
          editorMode: true,
        }),
        snapshot,
      ),
    ).toBeNull();
  });

  it("returns the pending document definition in editor mode after the document succeeds", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const draftState = createMockGraphEditorDraftState({
      latestDocument: baseFactoryDefinitionDocument,
      pendingFactoryDefinition: baseFactoryDefinitionDocument,
    });

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState,
          editableFactoryDocumentStatus: "success",
          editorMode: true,
        }),
        snapshot,
      ),
    ).toEqual(baseFactoryDefinitionDocument);
  });

  it("keeps the pending draft definition in editor mode when a topology save updates the saved document", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const pendingDraft = {
      ...baseFactoryDefinitionDocument,
      workers: [
        ...(baseFactoryDefinitionDocument.workers ?? []),
        {
          model: "gpt-5-mini",
          name: "reviewer",
          type: "MODEL_WORKER" as const,
        },
      ],
    };
    const savedAfterTopologyChange = {
      ...baseFactoryDefinitionDocument,
      version: {
        logical: "10",
        physical: "2026-06-01T12:00:00Z",
      },
      workstations: [
        {
          ...(baseFactoryDefinitionDocument.workstations?.[0] ?? {
            inputs: [],
            name: "draft",
            outputs: [],
            type: "MODEL_WORKSTATION" as const,
          }),
          worker: "reviewer",
        },
      ],
    };
    const draftState = createMockGraphEditorDraftState({
      baseDocument: baseFactoryDefinitionDocument,
      hasChanges: true,
      latestDocument: savedAfterTopologyChange,
      pendingFactoryDefinition: pendingDraft,
    });

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          draftState,
          editableFactoryDocument: savedAfterTopologyChange,
          editableFactoryDocumentStatus: "success",
          editorMode: true,
        }),
        snapshot,
      ),
    ).toEqual(pendingDraft);
  });
});

describe("currentActivityCardFactoryDefinition bundled docs", () => {
  it("keeps bundled docs from the saved document when observe mode prefers the timeline snapshot", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    const savedDocument = {
      ...sessionFactoryDocumentFromSnapshot(snapshot),
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

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocument: savedDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toMatchObject({
      supportingFiles: savedDocument.supportingFiles,
      workstations: snapshot.factory?.workstations,
    });
  });

  it("keeps snapshot layout in observe mode when the saved document omits it", () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    if (!snapshot.factory) {
      throw new Error("expected snapshot factory fixture");
    }

    snapshot.factory.layout = {
      nodes: [
        {
          id: "workstation:draft",
          position: { x: 480, y: 240 },
        },
      ],
      schemaVersion: 1,
      viewport: { x: 10, y: 20, zoom: 1.25 },
    };

    const savedDocument = {
      ...sessionFactoryDocumentFromSnapshot(snapshot),
    };
    delete savedDocument.layout;

    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocument: savedDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toMatchObject({
      layout: snapshot.factory.layout,
      workstations: savedDocument.workstations,
    });
  });
});

function renderGraphViewModelWithLayout(
  graphLayout: GraphLayout,
  options: {
    editor?: Partial<
      Pick<
        Parameters<typeof useCurrentActivityGraphViewModel>[0]["editor"],
        "activeTool" | "canInteractWithEditor" | "editorMode"
      >
    >;
    renderedLayout?: FactoryLayout;
    selectedWaypointEdgeId?: string | null;
    visibleGraphEdges?: GraphLayout["edges"];
  } = {},
) {
  const editor = {
    activeTool: null,
    canInteractWithEditor: true,
    editorMode: true,
    graphProjection: {
      canonicalLayoutViewport: null,
      displayFactoryDefinition: baseFactoryDefinition,
      graphLayout,
      pendingAdditionEdgeIds: new Set<string>(),
      positionedGraphLayout: graphLayout,
      renderedLayout: options.renderedLayout ?? { schemaVersion: 1 },
      visibleGraphEdges: options.visibleGraphEdges ?? [],
    },
    handleConnectionAnchorClick: vi.fn(),
    pendingConnectionSource: null,
    selectedWaypointEdgeId: options.selectedWaypointEdgeId,
    validationTargets: [],
    ...options.editor,
  };
  const snapshot = {
    ...structuredClone(singleNodeDashboardSnapshot),
    factory: baseFactoryDefinition,
    runtime: {
      ...singleNodeDashboardSnapshot.runtime,
      in_flight_dispatch_count: 0,
    },
  };
  const noop = vi.fn();

  return renderHook(
    ({ currentGraphLayout }: { currentGraphLayout: GraphLayout }) =>
      useCurrentActivityGraphViewModel({
        editor: {
          ...editor,
          graphProjection: {
            ...editor.graphProjection,
            graphLayout: currentGraphLayout,
            positionedGraphLayout: currentGraphLayout,
          },
        } as Parameters<typeof useCurrentActivityGraphViewModel>[0]["editor"],
        now: 0,
        onSelectDoc: noop,
        onSelectResource: noop,
        onSelectStateNode: noop,
        onSelectWorkID: noop,
        onSelectWorker: noop,
        onSelectWorkType: noop,
        onSelectWorkstation: noop,
        selection: null,
        snapshot,
      }),
    { initialProps: { currentGraphLayout: graphLayout } },
  );
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: node position and selection contract cases stay together.
describe("useCurrentActivityGraphViewModel node positions", () => {
  it("applies transient React Flow positions while preserving graph projection as the reset boundary", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "work-state:story:queued",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:queued",
            state_value: "queued",
            type_id: "story",
          },
          row: 0,
          width: 140,
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 160,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 360,
          y: 120,
        },
      ],
      width: 600,
    };
    const { rerender, result } = renderGraphViewModelWithLayout(graphLayout);
    const workStateNode = () =>
      result.current.nodes.find(
        (node) => node.id === "work-state:story:queued",
      );

    expect(workStateNode()?.position).toEqual({ x: 120, y: 80 });

    act(() => {
      result.current.handleNodesChange([
        {
          id: "work-state:story:queued",
          position: { x: 999, y: 999 },
          type: "position",
        },
      ]);
      result.current.handleGraphSelectionChange?.({
        edges: [],
        nodes: [{ id: "work-state:story:queued" }],
      });
    });

    expect(workStateNode()).toMatchObject({
      position: { x: 999, y: 999 },
      selected: true,
    });

    const updatedGraphLayout: GraphLayout = {
      ...graphLayout,
      nodes: graphLayout.nodes.map((node) =>
        node.nodeId === "work-state:story:queued"
          ? { ...node, x: 132, y: 96 }
          : node,
      ),
    };

    rerender({ currentGraphLayout: updatedGraphLayout });

    expect(workStateNode()).toMatchObject({
      position: { x: 132, y: 96 },
      selected: true,
    });
  });

  it("derives node and edge selected flags from editor-local graph selection", () => {
    const edgeId = "workstation-output:workstation:review->work-state:story:done";
    const graphLayout: GraphLayout = {
      edges: [
        {
          canonicalEdgeId: edgeId,
          edgeId,
          fromNodeId: "workstation:review",
          label: "done",
          labelX: 0,
          labelY: 0,
          outcomeKind: "success",
          path: "",
          sourcePlaceKind: undefined,
          stateCategory: "TERMINAL",
          targetPlaceKind: "work_state",
          toNodeId: "work-state:story:done",
        },
      ],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 120,
          nodeId: "work-state:story:done",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:done",
            state_category: "TERMINAL",
            state_value: "done",
            type_id: "story",
          },
          row: 0,
          width: 140,
          x: 420,
          y: 100,
        },
      ],
      width: 600,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout, {
      visibleGraphEdges: graphLayout.edges,
    });

    act(() => {
      result.current.graphSelection.replaceSelection({
        edgeIds: [edgeId],
        nodeIds: ["workstation:review"],
        primaryTarget: { kind: "node", id: "workstation:review" },
      });
    });

    expect(
      result.current.nodes.find((node) => node.id === "workstation:review"),
    ).toMatchObject({ selected: true });
    expect(result.current.edges[0]?.selected).toBe(true);
    expect(result.current.graphSelection.resolvePrimaryTarget()).toEqual({
      kind: "node",
      id: "workstation:review",
    });
  });

  it("syncs React Flow selection changes and clears with Esc handler", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 120,
          nodeId: "workstation:done",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "done",
          x: 420,
          y: 100,
        },
      ],
      width: 600,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout, {
      visibleGraphEdges: graphLayout.edges,
    });

    act(() => {
      result.current.handleGraphSelectionStart({ shiftKey: true });
      result.current.handleGraphSelectionChange?.({
        edges: [],
        nodes: [{ id: "workstation:done" }],
      });
    });

    expect(result.current.graphSelection.state.selectedNodeIds).toEqual(
      new Set(["workstation:done"]),
    );

    act(() => {
      result.current.handleGraphSelectionStart({ shiftKey: true });
      result.current.handleGraphSelectionChange?.({
        edges: [],
        nodes: [{ id: "workstation:review" }, { id: "workstation:done" }],
      });
    });

    expect(result.current.graphSelection.state.selectedNodeIds).toEqual(
      new Set(["workstation:review", "workstation:done"]),
    );

    act(() => {
      result.current.clearGraphSelection();
    });

    expect(result.current.graphSelection.state.selectedNodeIds).toEqual(
      new Set(),
    );
  });

  it("ignores graph selection changes while the delete tool is active", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 120,
          y: 80,
        },
      ],
      width: 600,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout, {
      editor: {
        activeTool: "delete",
        canInteractWithEditor: true,
        editorMode: true,
      },
      visibleGraphEdges: graphLayout.edges,
    });

    act(() => {
      result.current.handleGraphSelectionChange?.({
        edges: [],
        nodes: [{ id: "workstation:review" }],
      });
    });

    expect(result.current.graphSelection.state.selectedNodeIds).toEqual(
      new Set(),
    );
  });
});

describe("useCurrentActivityGraphViewModel edge waypoints", () => {
  it("decorates React Flow edges with waypoints from the rendered graph layout", () => {
    const edgeId = "workstation-output:workstation:review->work-state:story:done";
    const graphLayout: GraphLayout = {
      edges: [
        {
          canonicalEdgeId: edgeId,
          edgeId,
          fromNodeId: "workstation:review",
          label: "done",
          labelX: 0,
          labelY: 0,
          outcomeKind: "success",
          path: "",
          sourcePlaceKind: undefined,
          stateCategory: "TERMINAL",
          targetPlaceKind: "work_state",
          toNodeId: "work-state:story:done",
        },
      ],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 120,
          nodeId: "work-state:story:done",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:done",
            state_category: "TERMINAL",
            state_value: "done",
            type_id: "story",
          },
          row: 0,
          width: 140,
          x: 420,
          y: 100,
        },
      ],
      width: 600,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout, {
      renderedLayout: {
        edges: [{ id: edgeId, waypoints: [{ x: 320, y: 180 }] }],
        schemaVersion: 1,
      },
      selectedWaypointEdgeId: edgeId,
      visibleGraphEdges: graphLayout.edges,
    });

    expect(result.current.edges[0]).toMatchObject({
      data: {
        waypoints: [{ x: 320, y: 180 }],
      },
      selected: true,
      type: "factoryEditorEdge",
    });
  });
});
