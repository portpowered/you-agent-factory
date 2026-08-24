// biome-ignore lint/style/noExcessiveLinesPerFile: graph view-model selection contract cases stay together.
import {
  act,
  cleanup,
  render,
  renderHook,
  waitFor,
} from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import "@xyflow/react/dist/style.css";
import { ReactFlow, ReactFlowProvider } from "@xyflow/react";
import { FACTORY_GRAPH_NODE_TYPES } from "@you-agent-factory/factory-graph";
import { createElement, useEffect } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { installDashboardBrowserTestShims } from "../../../components/dashboard/test-browser-shims";
import { singleNodeDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import {
  baseFactoryDefinition,
  baseFactoryDefinitionDocument,
  createMockGraphEditorDraftState,
  divergentDocumentPlaneFactoryDocument,
} from "../../../testing/graph-editor-harness";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import {
  FACTORY_LAYOUT_GROUP_DEFAULT_SIZE,
  fitFactoryLayoutGroupBounds,
} from "../../factory-graph-editor/lib/layout/visual-groups/factory-graph-layout-groups";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { currentActivityCardFactoryDefinition } from "./current-activity-card-factory-definition";
import {
  type CurrentActivityGraphViewModelResult,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";

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

describe("currentActivityCardFactoryDefinition", () => {
  const snapshot = {
    ...structuredClone(singleNodeDashboardSnapshot),
    factory: baseFactoryDefinition,
  };

  it("uses the event-computed Factory definition in observe mode", () => {
    expect(
      currentActivityCardFactoryDefinition(
        createEditorStub({
          editableFactoryDocument: divergentDocumentPlaneFactoryDocument,
          editableFactoryDocumentStatus: "success",
          editorMode: false,
        }),
        snapshot,
      ),
    ).toEqual(baseFactoryDefinition);
  });

  it("returns null in editor mode while the scoped factory document is pending", () => {
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

function renderGraphViewModelWithLayout(
  graphLayout: GraphLayout,
  options: {
    editor?: Partial<
      Pick<
        Parameters<typeof useCurrentActivityGraphViewModel>[0]["editor"],
        | "activeTool"
        | "canInteractWithEditor"
        | "editorMode"
        | "expandedNodeIds"
        | "nodeResizeControls"
      >
    >;
    expandedNodeIds?: ReadonlySet<string>;
    renderedLayout?: FactoryLayout;
    onGraphTargetSelected?: () => void;
    selectedWaypointEdgeId?: string | null;
    snapshot?: DashboardSnapshot;
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
  const snapshot = options.snapshot ?? {
    ...structuredClone(singleNodeDashboardSnapshot),
    factory: structuredClone(baseFactoryDefinition),
    runtime: {
      ...singleNodeDashboardSnapshot.runtime,
      in_flight_dispatch_count: 0,
    },
  };
  const noop = vi.fn();

  return renderHook(
    ({
      currentGraphLayout,
      currentSnapshot,
      currentExpandedNodeIds,
      currentRenderedLayout,
    }: {
      currentGraphLayout: GraphLayout;
      currentExpandedNodeIds?: ReadonlySet<string>;
      currentRenderedLayout?: FactoryLayout;
      currentSnapshot?: DashboardSnapshot;
    }) =>
      useCurrentActivityGraphViewModel({
        editor: {
          ...editor,
          expandedNodeIds:
            currentExpandedNodeIds ??
            options.expandedNodeIds ??
            EMPTY_EXPANDED_NODE_IDS,
          graphProjection: {
            ...editor.graphProjection,
            graphLayout: currentGraphLayout,
            positionedGraphLayout: currentGraphLayout,
            renderedLayout:
              currentRenderedLayout ??
              options.renderedLayout ??
              ({ schemaVersion: 1 } as FactoryLayout),
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
        onGraphTargetSelected: options.onGraphTargetSelected,
        selection: null,
        snapshot: currentSnapshot ?? snapshot,
      }),
    {
      initialProps: {
        currentExpandedNodeIds: options.expandedNodeIds,
        currentGraphLayout: graphLayout,
        currentSnapshot: snapshot,
      },
    },
  );
}

const EMPTY_EXPANDED_NODE_IDS = new Set<string>();

function snapshotWithGraphOverlay(
  input: {
    activeWorkItemCount?: number;
    completedWorkCount?: number;
    failedWorkCount?: number;
  } = {},
): DashboardSnapshot {
  const activeWorkItemCount = input.activeWorkItemCount ?? 0;
  const completedWorkCount = input.completedWorkCount ?? 0;
  const failedWorkCount = input.failedWorkCount ?? 0;
  const snapshot = structuredClone(singleNodeDashboardSnapshot);
  const activeWorkItems = Array.from(
    { length: activeWorkItemCount },
    (_, index) => ({
      display_name: `Active story ${index + 1}`,
      trace_id: `trace-active-story-${index + 1}`,
      work_id: `work-active-story-${index + 1}`,
      work_type_id: "story",
    }),
  );

  snapshot.factory = structuredClone(baseFactoryDefinition);
  snapshot.factory_state = activeWorkItemCount > 0 ? "RUNNING" : "IDLE";
  snapshot.runtime = {
    ...snapshot.runtime,
    active_dispatch_ids: activeWorkItemCount > 0 ? ["dispatch-review"] : [],
    active_executions_by_dispatch_id:
      activeWorkItemCount > 0
        ? {
            "dispatch-review": {
              consumed_tokens: [
                {
                  created_at: "2026-08-14T12:00:00Z",
                  entered_at: "2026-08-14T12:00:00Z",
                  place_id: "story:queued",
                  token_id: "token-story-queued",
                  work_id: "work-active-story-1",
                  work_type_id: "story",
                },
              ],
              dispatch_id: "dispatch-review",
              started_at: "2026-08-14T12:00:00Z",
              transition_id: "review",
              workstation_name: "Review",
              workstation_node_id: "review",
              work_items: activeWorkItems,
            },
          }
        : {},
    current_work_items_by_place_id: {
      ...(snapshot.runtime.current_work_items_by_place_id ?? {}),
      "story:queued": activeWorkItems,
    },
    in_flight_dispatch_count: activeWorkItemCount,
    place_token_counts: {
      ...(snapshot.runtime.place_token_counts ?? {}),
      "story:blocked": failedWorkCount,
      "story:done": completedWorkCount,
      "story:queued": activeWorkItemCount,
    },
    session: {
      ...snapshot.runtime.session,
      completed_count: completedWorkCount,
      failed_count: failedWorkCount,
    },
  };

  return snapshot;
}

function MountedCurrentActivityGraph({
  graphLayout,
  onViewModel,
  snapshot,
}: {
  graphLayout: GraphLayout;
  onViewModel: (viewModel: CurrentActivityGraphViewModelResult) => void;
  snapshot: DashboardSnapshot;
}) {
  const viewModel = useCurrentActivityGraphViewModel({
    editor: {
      activeTool: null,
      canInteractWithEditor: true,
      editorMode: false,
      graphProjection: {
        canonicalLayoutViewport: null,
        displayFactoryDefinition: baseFactoryDefinition,
        graphLayout,
        pendingAdditionEdgeIds: new Set<string>(),
        positionedGraphLayout: graphLayout,
        renderedLayout: { schemaVersion: 1 },
        visibleGraphEdges: graphLayout.edges,
      },
      handleConnectionAnchorClick: vi.fn(),
      pendingConnectionSource: null,
      selectedWaypointEdgeId: null,
      validationTargets: [],
    } as Parameters<typeof useCurrentActivityGraphViewModel>[0]["editor"],
    now: 0,
    onSelectDoc: vi.fn(),
    onSelectResource: vi.fn(),
    onSelectStateNode: vi.fn(),
    onSelectWorkID: vi.fn(),
    onSelectWorker: vi.fn(),
    onSelectWorkType: vi.fn(),
    onSelectWorkstation: vi.fn(),
    selection: null,
    snapshot,
  });

  useEffect(() => {
    onViewModel(viewModel);
  }, [onViewModel, viewModel]);

  return createElement(
    "div",
    { style: { height: 640, width: 1200 } },
    createElement(
      ReactFlowProvider,
      null,
      createElement(ReactFlow, {
        defaultViewport: { x: 24, y: 24, zoom: 0.8 },
        edges: viewModel.edges,
        fitView: false,
        nodeTypes: FACTORY_GRAPH_NODE_TYPES,
        nodes: viewModel.nodes,
        onEdgesChange: viewModel.handleEdgesChange,
        onNodesChange: viewModel.handleNodesChange,
      }),
    ),
  );
}

describe("current activity graph observe projection", () => {
  it("ignores React Flow position callbacks while observe mode is active", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 160,
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
        canInteractWithEditor: false,
        editorMode: false,
      },
    });

    act(() => {
      result.current.handleNodesChange([
        {
          id: "workstation:review",
          position: { x: 999, y: 999 },
          type: "position",
        },
      ]);
    });

    expect(result.current.nodes[0]?.position).toEqual({ x: 120, y: 80 });
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: mounted React Flow continuity is intentionally exercised across every live overlay transition.
describe("mounted current activity graph continuity", () => {
  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: mounted React Flow continuity is intentionally exercised across every live overlay transition.
  it("keeps React Flow node elements, handles, measurements, selection, and viewport stable through live overlays", async () => {
    const restoreBrowserTestShims = installDashboardBrowserTestShims();
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 160,
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
    const snapshots = [
      snapshotWithGraphOverlay({ activeWorkItemCount: 1 }),
      snapshotWithGraphOverlay({ activeWorkItemCount: 3 }),
      snapshotWithGraphOverlay({ activeWorkItemCount: 4 }),
      snapshotWithGraphOverlay({ activeWorkItemCount: 25 }),
      snapshotWithGraphOverlay({ completedWorkCount: 4 }),
      snapshotWithGraphOverlay({ failedWorkCount: 1 }),
    ];
    let latestViewModel: CurrentActivityGraphViewModelResult | null = null;
    const viewModelProps = {
      graphLayout,
      onViewModel: (viewModel: CurrentActivityGraphViewModelResult) => {
        latestViewModel = viewModel;
      },
      snapshot: snapshotWithGraphOverlay(),
    };
    const rendered = render(
      createElement(MountedCurrentActivityGraph, viewModelProps),
    );

    try {
      const nodeSelector = '.react-flow__node[data-id="workstation:review"]';
      await waitFor(() => {
        expect(rendered.container.querySelector(nodeSelector)).not.toBeNull();
      });
      const initialNode =
        rendered.container.querySelector<HTMLElement>(nodeSelector);
      const initialViewport = rendered.container.querySelector<HTMLElement>(
        ".react-flow__viewport",
      );
      expect(initialNode).not.toBeNull();
      expect(initialViewport).not.toBeNull();
      if (!initialNode || !initialViewport) {
        throw new Error("Expected the mounted graph host to render.");
      }
      const initialHandleIds = Array.from(
        initialNode.querySelectorAll<HTMLElement>("[data-node-handle-badge]"),
        (handle) => handle.dataset.nodeHandleBadge,
      );
      const initialNodeTransform = initialNode.style.transform;
      const initialViewportStyle = initialViewport.getAttribute("style");
      const initialNodeState = latestViewModel?.nodes.find(
        (node) => node.id === "workstation:review",
      );
      expect(initialNodeState).toBeDefined();

      act(() => {
        latestViewModel?.handleGraphSelectionChange({
          edges: [],
          nodes: [{ id: "workstation:review" }],
        });
      });
      await waitFor(() => {
        expect(
          latestViewModel?.nodes.find(
            (node) => node.id === "workstation:review",
          )?.selected,
        ).toBe(true);
      });

      for (const snapshot of snapshots) {
        rendered.rerender(
          createElement(MountedCurrentActivityGraph, {
            ...viewModelProps,
            snapshot,
          }),
        );

        await waitFor(() => {
          expect(rendered.container.querySelector(nodeSelector)).toBe(
            initialNode,
          );
        });
        expect(rendered.container.querySelector(".react-flow__viewport")).toBe(
          initialViewport,
        );
        expect(initialNode.style.transform).toBe(initialNodeTransform);
        expect(
          Array.from(
            initialNode.querySelectorAll<HTMLElement>(
              "[data-node-handle-badge]",
            ),
            (handle) => handle.dataset.nodeHandleBadge,
          ),
        ).toEqual(initialHandleIds);
        expect(
          rendered.container.querySelector<HTMLElement>(nodeSelector)
            ?.isConnected,
        ).toBe(true);
        expect(
          latestViewModel?.nodes.find(
            (node) => node.id === "workstation:review",
          ),
        ).toMatchObject({
          height: initialNodeState?.height,
          initialHeight: initialNodeState?.initialHeight,
          initialWidth: initialNodeState?.initialWidth,
          measured: initialNodeState?.measured,
          position: initialNodeState?.position,
          selected: true,
          width: initialNodeState?.width,
        });
        expect(
          rendered.container
            .querySelector<HTMLElement>(".react-flow__viewport")
            ?.getAttribute("style"),
        ).toBe(initialViewportStyle);
      }
    } finally {
      rendered.unmount();
      cleanup();
      restoreBrowserTestShims();
    }
  });
});

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
    const initialMeasured = workStateNode()?.measured;

    expect(workStateNode()?.position).toEqual({ x: 120, y: 80 });
    expect(initialMeasured).toEqual({ height: 120, width: 140 });

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
      measured: initialMeasured,
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

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: one mounted graph is intentionally exercised across every live overlay transition.
  it("keeps one mounted graph stable through live lifecycle and Work-count overlays", () => {
    const graphLayout: GraphLayout = {
      edges: [
        {
          canonicalEdgeId:
            "workstation-output:workstation:review->work-state:story:done",
          edgeId:
            "workstation-output:workstation:review->work-state:story:done",
          fromNodeId: "workstation:review",
          label: "done",
          labelX: 0,
          labelY: 0,
          outcomeKind: "accepted",
          path: "",
          sourcePlaceKind: undefined,
          stateCategory: "TERMINAL",
          targetPlaceKind: "work_state",
          toNodeId: "work-state:story:done",
        },
      ],
      height: 640,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "work-state:story:queued",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:queued",
            state_category: "INITIAL",
            state_value: "queued",
            type_id: "story",
          },
          row: 0,
          width: 240,
          x: 120,
          y: 80,
        },
        {
          column: 1,
          height: 260,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 320,
          workstationNodeId: "review",
          x: 420,
          y: 80,
        },
        {
          column: 2,
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
          width: 240,
          x: 840,
          y: 80,
        },
        {
          column: 2,
          height: 120,
          nodeId: "work-state:story:blocked",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:blocked",
            state_category: "FAILED",
            state_value: "blocked",
            type_id: "story",
          },
          row: 1,
          width: 240,
          x: 840,
          y: 280,
        },
      ],
      width: 1200,
    };
    const visibleGraphEdges = graphLayout.edges;
    const { rerender, result } = renderGraphViewModelWithLayout(graphLayout, {
      snapshot: snapshotWithGraphOverlay(),
      visibleGraphEdges,
    });
    const initialNodeState = new Map(
      result.current.nodes.map((node) => [
        node.id,
        {
          handles: (
            node.data as { handles?: Array<{ id: string }> }
          ).handles?.map((handle) => handle.id),
          height: node.height,
          initialHeight: node.initialHeight,
          initialWidth: node.initialWidth,
          measured: node.measured,
          position: node.position,
          width: node.width,
        },
      ]),
    );
    const initialEdgeEndpoints = result.current.edges.map((edge) => ({
      source: edge.source,
      sourceHandle: edge.sourceHandle,
      target: edge.target,
      targetHandle: edge.targetHandle,
    }));

    act(() => {
      result.current.handleGraphSelectionChange({
        edges: [],
        nodes: [{ id: "workstation:review" }],
      });
    });

    const overlaySnapshots = [
      {
        activeWorkItemCount: 1,
        snapshot: snapshotWithGraphOverlay({ activeWorkItemCount: 1 }),
      },
      {
        activeWorkItemCount: 3,
        snapshot: snapshotWithGraphOverlay({ activeWorkItemCount: 3 }),
      },
      {
        activeWorkItemCount: 4,
        snapshot: snapshotWithGraphOverlay({ activeWorkItemCount: 4 }),
      },
      {
        activeWorkItemCount: 25,
        snapshot: snapshotWithGraphOverlay({ activeWorkItemCount: 25 }),
      },
      {
        activeWorkItemCount: 0,
        snapshot: snapshotWithGraphOverlay({ completedWorkCount: 4 }),
      },
      {
        activeWorkItemCount: 0,
        snapshot: snapshotWithGraphOverlay({ failedWorkCount: 1 }),
      },
    ];

    for (const overlay of overlaySnapshots) {
      rerender({
        currentGraphLayout: graphLayout,
        currentSnapshot: overlay.snapshot,
      });

      expect(result.current.nodes.map((node) => node.id)).toEqual([
        ...initialNodeState.keys(),
      ]);
      for (const node of result.current.nodes) {
        const initial = initialNodeState.get(node.id);
        expect(initial).toBeDefined();
        expect(node).toMatchObject({
          height: initial?.height,
          initialHeight: initial?.initialHeight,
          initialWidth: initial?.initialWidth,
          measured: initial?.measured,
          position: initial?.position,
          selected: node.id === "workstation:review",
          width: initial?.width,
        });
        expect(
          (node.data as { handles?: Array<{ id: string }> }).handles?.map(
            (handle) => handle.id,
          ),
        ).toEqual(initial?.handles);
      }
      const queuedNode = result.current.nodes.find(
        (node) => node.id === "work-state:story:queued",
      );
      const doneNode = result.current.nodes.find(
        (node) => node.id === "work-state:story:done",
      );
      const blockedNode = result.current.nodes.find(
        (node) => node.id === "work-state:story:blocked",
      );
      const workstationData = result.current.nodes.find(
        (node) => node.id === "workstation:review",
      )?.data as {
        executions?: Array<{ work_items?: unknown[] }>;
      };

      expect(queuedNode?.data).toMatchObject({
        tokenCount: overlay.activeWorkItemCount,
      });
      expect(doneNode?.data).toMatchObject({
        tokenCount:
          overlay.snapshot.runtime.place_token_counts?.["story:done"] ?? 0,
      });
      expect(blockedNode?.data).toMatchObject({
        tokenCount:
          overlay.snapshot.runtime.place_token_counts?.["story:blocked"] ?? 0,
      });
      expect(
        workstationData.executions?.flatMap(
          (execution) => execution.work_items ?? [],
        ).length ?? 0,
      ).toBe(overlay.activeWorkItemCount);
      expect(
        result.current.edges.map((edge) => ({
          source: edge.source,
          sourceHandle: edge.sourceHandle,
          target: edge.target,
          targetHandle: edge.targetHandle,
        })),
      ).toEqual(initialEdgeEndpoints);
    }

    expect(
      result.current.nodes.find((node) => node.id === "work-state:story:done")
        ?.data,
    ).toMatchObject({ tokenCount: 0 });
    expect(
      result.current.nodes.find(
        (node) => node.id === "work-state:story:blocked",
      )?.data,
    ).toMatchObject({ tokenCount: 1 });
  });

  it("keeps React Flow dimension events disposable instead of treating them as authored layout", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 360,
      nodes: [
        {
          column: 0,
          height: 160,
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
    const { result } = renderGraphViewModelWithLayout(graphLayout);
    const dimensionChange = {
      dimensions: { height: 240, width: 300 },
      id: "workstation:review",
      type: "dimensions" as const,
    };

    act(() => {
      result.current.handleNodesChange([dimensionChange]);
    });

    expect(result.current.nodes[0]).toMatchObject({
      height: 160,
      width: 220,
    });
  });

  it("derives node and edge selected flags from editor-local graph selection", () => {
    const edgeId =
      "workstation-output:workstation:review->work-state:story:done";
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

  it("wires a node click into graph selection and fits a group around widely scattered members", () => {
    const graphLayout: GraphLayout = {
      edges: [],
      height: 1200,
      nodes: [
        {
          column: 0,
          height: 120,
          nodeId: "workstation:review",
          nodeKind: "workstation",
          row: 0,
          width: 220,
          workstationNodeId: "review",
          x: 40,
          y: 40,
        },
        {
          column: 1,
          height: 120,
          nodeId: "work-state:story:queued",
          nodeKind: "state_position",
          place: {
            kind: "work_state",
            place_id: "story:queued",
            state_category: "QUEUED",
            state_value: "queued",
            type_id: "story",
          },
          row: 0,
          width: 140,
          x: 900,
          y: 120,
        },
        {
          column: 2,
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
          row: 1,
          width: 140,
          x: 300,
          y: 1000,
        },
      ],
      width: 1400,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout);

    const reviewNode = result.current.nodes.find(
      (node) => node.id === "workstation:review",
    );
    const clickReviewNode = (
      reviewNode?.data as
        | { onSelectWorkstation?: (nodeId: string) => void }
        | undefined
    )?.onSelectWorkstation;
    expect(clickReviewNode).toBeInstanceOf(Function);

    act(() => {
      clickReviewNode?.("review");
    });

    expect([...result.current.graphSelection.state.selectedNodeIds]).toEqual([
      "workstation:review",
    ]);

    act(() => {
      result.current.graphSelection.addToSelection({
        nodeIds: ["work-state:story:queued", "work-state:story:done"],
      });
    });

    const selectedNodeIds = [
      ...result.current.graphSelection.state.selectedNodeIds,
    ];
    expect(new Set(selectedNodeIds)).toEqual(
      new Set([
        "workstation:review",
        "work-state:story:queued",
        "work-state:story:done",
      ]),
    );

    const nodeGeometryById = new Map(
      result.current.nodes
        .filter((node) => selectedNodeIds.includes(node.id))
        .map((node) => [
          node.id,
          {
            height: node.height ?? 0,
            position: node.position,
            width: node.width ?? 0,
          },
        ]),
    );

    const bounds = fitFactoryLayoutGroupBounds({
      nodeGeometryById,
      nodeIds: selectedNodeIds,
    });

    expect(bounds).not.toBeNull();
    expect(bounds?.width ?? 0).toBeGreaterThan(
      FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.width,
    );
    expect(bounds?.height ?? 0).toBeGreaterThan(
      FACTORY_LAYOUT_GROUP_DEFAULT_SIZE.height,
    );
    expect(bounds?.x ?? 0).toBeLessThanOrEqual(40);
    expect(bounds?.y ?? 0).toBeLessThanOrEqual(40);
    expect((bounds?.x ?? 0) + (bounds?.width ?? 0)).toBeGreaterThanOrEqual(
      900 + 140,
    );
    expect((bounds?.y ?? 0) + (bounds?.height ?? 0)).toBeGreaterThanOrEqual(
      1000 + 120,
    );
  });

  it("notifies the visual-group focus bridge when a semantic node is selected", () => {
    const onGraphTargetSelected = vi.fn();
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
          x: 40,
          y: 40,
        },
      ],
      width: 600,
    };
    const { result } = renderGraphViewModelWithLayout(graphLayout, {
      onGraphTargetSelected,
    });
    const selectWorkstation = (
      result.current.nodes[0]?.data as
        | { onSelectWorkstation?: (nodeId: string) => void }
        | undefined
    )?.onSelectWorkstation;

    act(() => {
      selectWorkstation?.("review");
    });

    expect(onGraphTargetSelected).toHaveBeenCalledTimes(1);
    expect(result.current.graphSelection.state.selectedNodeIds).toEqual(
      new Set(["workstation:review"]),
    );
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: related edge projection cases share one graph view-model fixture.
describe("useCurrentActivityGraphViewModel edge waypoints", () => {
  it("projects direct edge pointer interactions onto factory edge types", () => {
    const edgeId =
      "workstation-output:workstation:review->work-state:story:done";
    const interaction = {
      onPointerDown: vi.fn(),
    };
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
      editor: {
        edgePointerInteraction: () => interaction,
      },
      visibleGraphEdges: graphLayout.edges,
    });

    expect(result.current.edges[0]).toMatchObject({
      data: { interaction },
      type: "factoryEditorEdge",
    });
  });

  it("decorates React Flow edges with waypoints from the rendered graph layout", () => {
    const edgeId =
      "workstation-output:workstation:review->work-state:story:done";
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

function workstationResizeLayout(): GraphLayout {
  return {
    edges: [],
    height: 360,
    nodes: [
      {
        column: 0,
        height: 160,
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
}

function semanticNodeResizeLayout(): GraphLayout {
  return {
    edges: [],
    height: 520,
    nodes: [
      {
        column: 0,
        height: 160,
        nodeId: "workstation:review",
        nodeKind: "workstation",
        row: 0,
        width: 220,
        workstationNodeId: "review",
        x: 120,
        y: 80,
      },
      {
        column: 0,
        height: 86,
        nodeId: "place:story:queued",
        nodeKind: "state_position",
        place: {
          kind: "work_state",
          place_id: "story:queued",
          state_category: "INITIAL",
          state_value: "queued",
          type_id: "story",
        },
        row: 1,
        width: 164,
        x: 120,
        y: 300,
      },
      {
        column: 1,
        displayLabel: "Runbook",
        fileType: "DOC",
        height: 86,
        nodeId: "doc:factory/docs/runbook.md",
        nodeKind: "doc",
        row: 0,
        targetPath: "factory/docs/runbook.md",
        width: 168,
        x: 420,
        y: 80,
      },
    ],
    width: 720,
  };
}

function nodeResizeControls(
  result: { current: CurrentActivityGraphViewModelResult },
  nodeId: string,
) {
  return (
    result.current.nodes.find((node) => node.id === nodeId)?.data as
      | {
          resizeControls?: {
            onResize?: (dimensions: { height: number; width: number }) => void;
            onResizeEnd?: (dimensions: {
              height: number;
              width: number;
            }) => void;
          };
        }
      | undefined
  )?.resizeControls;
}

describe("current activity graph live node resize", () => {
  it("grows the rendered node while the resize pointer is still down", () => {
    const { result } = renderGraphViewModelWithLayout(
      workstationResizeLayout(),
    );

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResize?.({
        height: 300,
        width: 320,
      });
    });

    expect(result.current.nodes[0]).toMatchObject({
      height: 300,
      measured: { height: 300, width: 320 },
      width: 320,
    });
  });

  it("tracks every intermediate size rather than only the last one", () => {
    const { result } = renderGraphViewModelWithLayout(
      workstationResizeLayout(),
    );

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResize?.({
        height: 220,
        width: 240,
      });
    });
    expect(result.current.nodes[0]?.width).toBe(240);

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResize?.({
        height: 260,
        width: 300,
      });
    });

    expect(result.current.nodes[0]).toMatchObject({
      height: 260,
      width: 300,
    });
  });

  it("keeps the committed size after the drag settles", () => {
    const { result } = renderGraphViewModelWithLayout(
      workstationResizeLayout(),
    );

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResize?.({
        height: 300,
        width: 320,
      });
    });
    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResizeEnd?.({
        height: 300,
        width: 320,
      });
    });

    expect(result.current.nodes[0]).toMatchObject({
      height: 300,
      width: 320,
    });
  });

  it("holds the expanded content variant back until the drag is committed", () => {
    const { result } = renderGraphViewModelWithLayout(
      workstationResizeLayout(),
    );
    const expanded = () =>
      (result.current.nodes[0]?.data as { expanded?: boolean } | undefined)
        ?.expanded;

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResize?.({
        height: 300,
        width: 320,
      });
    });
    expect(expanded()).toBe(false);

    act(() => {
      nodeResizeControls(result, "workstation:review")?.onResizeEnd?.({
        height: 300,
        width: 320,
      });
    });

    expect(expanded()).toBe(true);
  });

  it("requests the expanded content variant for every resized semantic node family", () => {
    const { result } = renderGraphViewModelWithLayout(
      semanticNodeResizeLayout(),
    );
    const nodeIDs = [
      "workstation:review",
      "place:story:queued",
      "doc:factory/docs/runbook.md",
    ];

    const expanded = (nodeId: string) =>
      (
        result.current.nodes.find((node) => node.id === nodeId)?.data as
          | { expanded?: boolean }
          | undefined
      )?.expanded;

    for (const nodeId of nodeIDs) {
      expect(expanded(nodeId)).toBe(false);
      act(() => {
        nodeResizeControls(result, nodeId)?.onResizeEnd?.({
          height: 220,
          width: 240,
        });
      });
      expect(expanded(nodeId)).toBe(true);
    }
  });

  it("does not resize while the editor cannot be interacted with", () => {
    const { result } = renderGraphViewModelWithLayout(
      workstationResizeLayout(),
      { editor: { canInteractWithEditor: false, editorMode: false } },
    );

    expect(nodeResizeControls(result, "workstation:review")).toBeUndefined();
    expect(result.current.nodes[0]?.width).toBe(220);
  });
});

describe("current activity graph host-controlled resize", () => {
  it("projects host-committed expansion for a non-workstation node", () => {
    const placeNodeId = "place:story:queued";
    const hostResizeController = {
      enabled: true,
      onResizeEnd: vi.fn(),
    };
    const { rerender, result } = renderGraphViewModelWithLayout(
      semanticNodeResizeLayout(),
      {
        editor: { nodeResizeControls: hostResizeController },
      },
    );
    const expanded = () =>
      (
        result.current.nodes.find((node) => node.id === placeNodeId)?.data as
          | { expanded?: boolean }
          | undefined
      )?.expanded;

    expect(expanded()).toBe(false);

    act(() => {
      nodeResizeControls(result, placeNodeId)?.onResizeEnd?.({
        height: 220,
        width: 240,
      });
    });

    expect(hostResizeController.onResizeEnd).toHaveBeenCalledWith(
      {
        family: "work-state",
        nodeId: placeNodeId,
        position: { x: 120, y: 300 },
      },
      { height: 220, width: 240 },
    );
    expect(expanded()).toBe(true);

    rerender({
      currentExpandedNodeIds: new Set([placeNodeId]),
      currentGraphLayout: semanticNodeResizeLayout(),
    });

    expect(expanded()).toBe(true);
  });

  it("drops the local resize projection after the host undoes the authored size", () => {
    const placeNodeId = "place:story:queued";
    const hostResizeController = {
      enabled: true,
      onResizeEnd: vi.fn(),
    };
    const { rerender, result } = renderGraphViewModelWithLayout(
      semanticNodeResizeLayout(),
      {
        editor: { nodeResizeControls: hostResizeController },
      },
    );
    const expanded = () =>
      (
        result.current.nodes.find((node) => node.id === placeNodeId)?.data as
          | { expanded?: boolean }
          | undefined
      )?.expanded;

    act(() => {
      nodeResizeControls(result, placeNodeId)?.onResizeEnd?.({
        height: 220,
        width: 240,
      });
    });
    expect(expanded()).toBe(true);

    rerender({
      currentExpandedNodeIds: new Set([placeNodeId]),
      currentGraphLayout: semanticNodeResizeLayout(),
      currentRenderedLayout: {
        nodes: [
          {
            id: placeNodeId,
            position: { x: 120, y: 300 },
            size: { height: 220, width: 240 },
          },
        ],
        schemaVersion: 1,
      },
    });
    expect(expanded()).toBe(true);

    rerender({
      currentExpandedNodeIds: new Set(),
      currentGraphLayout: semanticNodeResizeLayout(),
      currentRenderedLayout: {
        nodes: [{ id: placeNodeId, position: { x: 120, y: 300 } }],
        schemaVersion: 1,
      },
    });
    expect(expanded()).toBe(false);
  });
});
// Component lane: requires DOM APIs.
