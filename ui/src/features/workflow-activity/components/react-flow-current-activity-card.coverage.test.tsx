import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  within,
} from "@testing-library/react";

import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDocument,
  useSaveCurrentFactory,
} from "../../current-factory-definition/public";
import {
  createEmptyFactoryGraphDraft,
  useFactoryGraphDraftState,
} from "../../factory-graph-editor/public";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { useFactoryGraphConnectionController } from "../hooks/react-flow-current-activity-card-editor-connections";
import {
  ReactFlowCurrentActivityCard,
  type CurrentActivitySelection,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

type BuildGraphLayout = (
  topology: typeof semanticWorkflowDashboardSnapshot.topology,
) => Promise<GraphLayout>;

const {
  actualBuildGraphLayoutRef,
  mockBuildGraphLayout,
  mockImportController,
  mockSetStoredNodePosition,
} = vi.hoisted(() => ({
  actualBuildGraphLayoutRef: { current: null as BuildGraphLayout | null },
  mockBuildGraphLayout: vi.fn(),
  mockImportController: {
    activateImport: vi.fn().mockResolvedValue(undefined),
    activationState: { status: "idle" as const },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" as const },
    importPreviewState: { status: "idle" as const },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
  },
  mockSetStoredNodePosition: vi.fn(),
}));

vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual("@xyflow/react");

  return {
    ...actual,
    Background: ({
      color,
      gap,
      size,
    }: {
      color?: string;
      gap?: number;
      size?: number;
    }) => (
      <div
        data-background-color={color}
        data-background-gap={String(gap ?? "")}
        data-background-size={String(size ?? "")}
        data-testid="graph-background"
      />
    ),
    Controls: ({ style }: { style?: Record<string, string | number> }) => (
      <div
        data-controls-style={JSON.stringify(style ?? null)}
        data-testid="graph-controls"
      />
    ),
    ReactFlow: ({
      children,
      connectionLineStyle,
      edgesFocusable,
      isValidConnection,
      onConnect,
      onEdgeClick,
      onNodeClick,
      onNodeDragStop,
    }: {
      children: React.ReactNode;
      connectionLineStyle?: Record<string, string | number>;
      edgesFocusable?: boolean;
      isValidConnection?: (connection: {
        source?: string | null;
        sourceHandle?: string | null;
        target?: string | null;
        targetHandle?: string | null;
      }) => boolean;
      onConnect?: (connection: {
        source?: string | null;
        sourceHandle?: string | null;
        target?: string | null;
        targetHandle?: string | null;
      }) => void;
      onEdgeClick?: (_event: unknown, edge: { id: string }) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
      onNodeDragStop?: (
        _event: unknown,
        node: { id: string; position: { x: number; y: number } },
      ) => void;
    }) => (
      <div data-testid="mock-react-flow">
        <output data-testid="edges-focusable">
          {String(edgesFocusable ?? false)}
        </output>
        <output data-testid="connection-line-style">
          {JSON.stringify(connectionLineStyle ?? null)}
        </output>
        <output data-testid="valid-workstation-output">
          {String(
            isValidConnection?.({
              source: "workstation:review",
              sourceHandle: "workstation-output-source",
              target: "work-state:story:done",
              targetHandle: "workstation-output-target",
            }) ?? false,
          )}
        </output>
        <output data-testid="invalid-workstation-output">
          {String(
            isValidConnection?.({
              source: "workstation:review",
              sourceHandle: "workstation-output-source",
              target: "work-state:story:done",
              targetHandle: "workstation-input-source",
            }) ?? false,
          )}
        </output>
        <button
          onClick={() =>
            onConnect?.({
              source: "workstation:review",
              sourceHandle: "workstation-output-source",
              target: "work-state:story:done",
              targetHandle: "workstation-output-target",
            })
          }
          type="button"
        >
          Trigger connect
        </button>
        <button
          onClick={() => onEdgeClick?.(null, { id: "edge-review-done" })}
          type="button"
        >
          Trigger edge click
        </button>
        <button
          onClick={() => onNodeClick?.(null, { id: "workstation:review" })}
          type="button"
        >
          Trigger node click
        </button>
        <button
          onClick={() =>
            onNodeDragStop?.(null, {
              id: "review",
              position: { x: 320, y: 180 },
            })
          }
          type="button"
        >
          Trigger drag stop
        </button>
        {children}
      </div>
    ),
  };
});

vi.mock("../hooks/current-activity-import-controller", () => ({
  useCurrentActivityImportController: () => mockImportController,
}));

vi.mock("./dashboard-flow-axis-legend", () => ({
  DashboardFlowAxisLegend: ({ className }: { className?: string }) => (
    <div className={className} data-testid="dashboard-flow-axis-legend" />
  ),
  getDefaultDashboardFlowAxisLegendEdgeItems: () => [],
  getDefaultDashboardFlowAxisLegendIconItems: () => [],
}));

vi.mock("./react-flow-current-activity-card-import", () => ({
  GraphDropOverlay: () => <div data-testid="graph-drop-overlay" />,
  GraphImportErrorPanel: () => <div data-testid="graph-import-error-panel" />,
  graphDropStateAttribute: () => "idle",
}));

vi.mock("../../current-factory-definition/public", async () => {
  const actual = await vi.importActual(
    "../../current-factory-definition/public",
  );

  return {
    ...actual,
    useCurrentFactoryDocument: vi.fn(),
    useSaveCurrentFactory: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/public", async () => {
  const actual = await vi.importActual("../../factory-graph-editor/public");

  return {
    ...actual,
    useFactoryGraphDraftState: vi.fn(),
  };
});

vi.mock("../../flowchart/lib/layout", async () => {
  const actual = await vi.importActual("../../flowchart/lib/layout");
  actualBuildGraphLayoutRef.current = actual.buildGraphLayout;

  return {
    ...actual,
    buildGraphLayout: (...args: Parameters<typeof actual.buildGraphLayout>) => {
      const implementation = mockBuildGraphLayout.getMockImplementation();
      if (implementation) {
        return mockBuildGraphLayout(...args);
      }

      return actual.buildGraphLayout(...args);
    },
  };
});

const defaultDraftState = {
  baseDocument: null,
  draft: {
    additions: {
      resources: [],
      workers: [],
      workStates: [],
      workTypes: [],
      workstations: [],
    },
    edgeChanges: {
      additions: [],
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
  graph: {
    edges: [],
    nodes: [],
  },
  hasChanges: false,
  latestDocument: null,
  pendingFactoryDefinition: null,
  replaceDraft: vi.fn(),
  resetDraft: vi.fn(),
  source: "projection" as const,
  updateDraft: vi.fn(),
  validationErrors: [],
};

interface ReactFlowCurrentActivityCardProps {
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (workID: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
  snapshot: typeof semanticWorkflowDashboardSnapshot;
}

function createProps(
  overrides: Partial<ReactFlowCurrentActivityCardProps> = {},
): ReactFlowCurrentActivityCardProps {
  return {
    now: Date.parse("2026-04-08T12:00:00Z"),
    onSelectStateNode: vi.fn(),
    onSelectWorkID: vi.fn(),
    onSelectWorkstation: vi.fn(),
    selection: null,
    snapshot: semanticWorkflowDashboardSnapshot,
    ...overrides,
  };
}

describe("ReactFlowCurrentActivityCard coverage", () => {
  beforeEach(() => {
    mockBuildGraphLayout.mockReset();
    mockSetStoredNodePosition.mockReset();
    vi.mocked(useCurrentFactoryDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    vi.mocked(useSaveCurrentFactory).mockReturnValue({
      mutateAsync: vi.fn(),
      reset: vi.fn(),
      status: "idle",
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue(
      defaultDraftState as never,
    );
  });

  it("renders the empty topology fallback when no workstation nodes exist", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.topology.workstation_node_ids = [];

    renderWithQueryClient(
      <ReactFlowCurrentActivityCard {...createProps({ snapshot })} />,
    );

    expect(screen.getByLabelText("Current activity")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(screen.getByText("Observe mode")).toBeTruthy();
    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(
      screen.getByText(
        "The factory has not published any workstation graph yet.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("mock-react-flow")).toBeNull();
  });

  it("falls back to the empty graph outcome when a replacement current-activity layout fails", async () => {
    const loadedSnapshot = structuredClone(singleNodeDashboardSnapshot);
    const rejectedSnapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    const onSelectStateNode = vi.fn();
    const onSelectWorkID = vi.fn();
    const onSelectWorkstation = vi.fn();
    const loadedLayout: GraphLayout = {
      edges: [],
      height: 196,
      nodes: [
        {
          column: 0,
          height: 196,
          nodeId: "workstation:intake",
          nodeKind: "workstation",
          row: 0,
          width: 156,
          workstationNodeId: "intake",
          x: 0,
          y: 0,
        },
      ],
      width: 156,
    };

    mockBuildGraphLayout.mockImplementation(async (topology) => {
      if (topology === rejectedSnapshot.topology) {
        throw new Error("layout failed");
      }
      return loadedLayout;
    });

    const { result, rerender } = renderHook(
      ({ snapshot }) =>
        useCurrentActivityGraphViewModel({
          now: Date.parse("2026-04-08T12:00:00Z"),
          onSelectStateNode,
          onSelectWorkID,
          onSelectWorkstation,
          selection: null,
          snapshot,
        }),
      {
        initialProps: {
          snapshot: loadedSnapshot,
        },
      },
    );

    await waitFor(() => {
      expect(result.current.nodes.length).toBeGreaterThan(0);
    });

    rerender({ snapshot: rejectedSnapshot });

    await waitFor(() => {
      expect(result.current.nodes).toHaveLength(0);
      expect(result.current.edges).toHaveLength(0);
    });
  });
  it("persists node positions after drag-stop when the viewport provides a graph key", () => {
    renderViewport({ graphKey: "graph-key" });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).toHaveBeenCalledWith(
      "graph-key",
      "review",
      {
        x: 320,
        y: 180,
      },
    );
  });

  it("renders the factory graph inside the shared dashboard graph frame", () => {
    renderViewport({ graphKey: "graph-key" });

    const graphFrame = screen.getByRole("region", {
      name: "Work graph viewport",
    });

    expect(graphFrame).toBeTruthy();
    expect(graphFrame.getAttribute("aria-label")).toBe("Work graph viewport");
    expect(screen.getByTestId("graph-background")).toBeTruthy();
    expect(screen.getByTestId("graph-controls")).toBeTruthy();
    expect(
      screen
        .getByTestId("graph-background")
        .getAttribute("data-background-color"),
    ).toBe("var(--color-af-edge-muted-soft)");
    expect(
      screen
        .getByTestId("graph-background")
        .getAttribute("data-background-gap"),
    ).toBe("24");
    expect(
      screen
        .getByTestId("graph-background")
        .getAttribute("data-background-size"),
    ).toBe("1");
    expect(
      screen.getByTestId("graph-controls").getAttribute("data-controls-style"),
    ).toContain('"backgroundColor":"var(--color-af-graph-controls-surface)"');
    expect(
      screen.getByTestId("graph-controls").getAttribute("data-controls-style"),
    ).toContain('"borderRadius":8');
    expect(screen.getByTestId("connection-line-style").textContent).toContain(
      '"stroke":"var(--color-af-accent)"',
    );
  });

  it("renders the compact editor toolbar inside the graph card without duplicate add controls", () => {
    renderViewport({
      addMenuActions: [{ id: "workstation", label: "Workstation" }],
      editorMode: true,
      graphKey: "graph-key",
      onAddAction: vi.fn(),
    });

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });

    expect(
      within(toolbar).getByRole("button", { name: "Open add entity menu" }),
    ).toBeTruthy();
    expect(
      within(toolbar).getByRole("button", { name: "Connect" }),
    ).toBeTruthy();
    expect(
      within(toolbar).getByRole("button", { name: "Delete" }),
    ).toBeTruthy();
    expect(within(toolbar).queryByRole("button", { name: "Add" })).toBeNull();
  });

  it("skips node-position persistence when the viewport has no graph key", () => {
    renderViewport({ graphKey: "" });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).not.toHaveBeenCalled();
  });

  it("routes editor-mode graph viewport connection and selection callbacks", () => {
    const onConnect = vi.fn();
    const onEditorEdgeClick = vi.fn();
    const onEditorNodeClick = vi.fn();

    renderViewport({
      activeTool: "connect",
      editorMode: true,
      graphKey: "graph-key",
      nodes: [
        {
          data: { kind: "workstation" },
          id: "workstation:review",
          position: { x: 0, y: 0 },
          type: "workstation",
        },
        {
          data: { kind: "work-state" },
          id: "work-state:story:done",
          position: { x: 240, y: 0 },
          type: "workState",
        },
      ],
      onConnect,
      onEditorEdgeClick,
      onEditorNodeClick,
    });

    expect(screen.getByTestId("valid-workstation-output").textContent).toBe(
      "true",
    );
    expect(screen.getByTestId("invalid-workstation-output").textContent).toBe(
      "false",
    );

    fireEvent.click(screen.getByRole("button", { name: "Trigger connect" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger edge click" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger node click" }));

    expect(onConnect).toHaveBeenCalledWith({
      source: "workstation:review",
      sourceHandle: "workstation-output-source",
      target: "work-state:story:done",
      targetHandle: "workstation-output-target",
    });
    expect(onEditorEdgeClick).toHaveBeenCalledWith("edge-review-done");
    expect(onEditorNodeClick).toHaveBeenCalledWith("workstation:review");
  });

  it("keeps editor edges focusable outside delete mode so hidden labels stay reachable", () => {
    renderViewport({
      activeTool: "connect",
      editorMode: true,
      graphKey: "graph-key",
      nodes: [
        {
          data: { kind: "workstation" },
          id: "workstation:review",
          position: { x: 0, y: 0 },
          type: "workstation",
        },
        {
          data: { kind: "work-state" },
          id: "work-state:story:done",
          position: { x: 240, y: 0 },
          type: "workState",
        },
      ],
    });

    expect(screen.getByTestId("edges-focusable").textContent).toBe("true");
  });

  it("adds draft edges from valid controller connections", () => {
    const updateDraft = vi.fn();
    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState: createConnectionDraftState(updateDraft),
      }),
    );

    act(() => {
      result.current.handleEditorConnect({
        source: "workstation:review",
        sourceHandle: "workstation-output-source",
        target: "work-state:story:done",
        targetHandle: "workstation-output-target",
      });
    });

    expect(updateDraft).toHaveBeenCalledTimes(1);
    const nextDraft = updateDraft.mock.calls[0][0](
      createEmptyFactoryGraphDraft(),
    );
    expect(nextDraft.edgeChanges.additions).toEqual([
      {
        kind: "workstation-output",
        source: { kind: "workstation", name: "review" },
        target: {
          kind: "work-state",
          stateName: "done",
          workTypeName: "story",
        },
      },
    ]);
    expect(result.current.connectionNotice).toBeNull();
    expect(result.current.pendingConnectionSource).toBeNull();
  });

  it("keeps controller connection attempts explicit when disabled or incompatible", () => {
    const updateDraft = vi.fn();
    const { rerender, result } = renderHook(
      ({ activeTool, canInteractWithEditor }) =>
        useFactoryGraphConnectionController({
          activeTool,
          canInteractWithEditor,
          draftState: createConnectionDraftState(updateDraft),
        }),
      {
        initialProps: {
          activeTool: "delete" as const,
          canInteractWithEditor: true,
        },
      },
    );

    act(() => {
      result.current.handleEditorConnect({
        source: "workstation:review",
        sourceHandle: "workstation-output-source",
        target: "work-state:story:done",
        targetHandle: "workstation-output-target",
      });
    });
    expect(updateDraft).not.toHaveBeenCalled();

    rerender({ activeTool: "connect", canInteractWithEditor: true });
    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "workstation-output-target",
        nodeId: "work-state:story:done",
      });
    });
    expect(result.current.connectionNotice).toBe(
      "Select a source anchor before choosing a target anchor.",
    );

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "workstation-output-source",
        nodeId: "workstation:review",
      });
    });
    expect(result.current.pendingConnectionSource).toEqual({
      anchorId: "workstation-output-source",
      nodeId: "workstation:review",
    });
    expect(result.current.connectionNotice).toBeNull();

    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "workstation-on-failure-target",
        nodeId: "work-state:story:done",
      });
    });
    expect(result.current.connectionNotice).toBe(
      "Success connections from review cannot connect to Failure on story:done.",
    );

    rerender({ activeTool: "delete", canInteractWithEditor: true });
    expect(result.current.pendingConnectionSource).toBeNull();
  });
});

function renderWithQueryClient(view: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}

function renderViewport({
  activeTool = null,
  addMenuActions,
  editorMode = false,
  graphKey,
  nodes = [],
  onAddAction,
  onConnect,
  onEditorEdgeClick,
  onEditorNodeClick,
}: {
  activeTool?: "add" | "connect" | "delete" | null;
  addMenuActions?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["addMenuActions"];
  editorMode?: boolean;
  graphKey: string;
  nodes?: Parameters<typeof CurrentActivityGraphViewport>[0]["nodes"];
  onAddAction?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["onAddAction"];
  onConnect?: Parameters<typeof CurrentActivityGraphViewport>[0]["onConnect"];
  onEditorEdgeClick?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["onEditorEdgeClick"];
  onEditorNodeClick?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["onEditorNodeClick"];
}) {
  return render(
    <CurrentActivityGraphViewport
      activeTool={activeTool}
      addMenuActions={addMenuActions}
      canInteractWithEditor={editorMode}
      editorMode={editorMode}
      edges={[]}
      graphKey={graphKey}
      handleNodesChange={vi.fn()}
      hasPendingChanges={false}
      imports={mockImportController}
      initialFitViewKey="full-graph"
      initialFitViewOptions={{ padding: 0.18 }}
      nodeTypes={{}}
      nodes={nodes}
      onAddAction={onAddAction}
      onConnect={onConnect}
      onEditorEdgeClick={onEditorEdgeClick}
      onEditorNodeClick={onEditorNodeClick}
      onSelectTool={vi.fn()}
      setStoredNodePosition={mockSetStoredNodePosition}
    />,
  );
}

function createConnectionDraftState(updateDraft: ReturnType<typeof vi.fn>) {
  return {
    ...defaultDraftState,
    draft: createEmptyFactoryGraphDraft(),
    graph: {
      edges: [],
      nodes: [
        {
          id: "workstation:review",
          key: { kind: "workstation", name: "review" },
          kind: "workstation",
          label: "review",
        },
        {
          id: "work-state:story:done",
          key: { kind: "work-state", stateName: "done", workTypeName: "story" },
          kind: "work-state",
          label: "story:done",
        },
      ],
    },
    updateDraft,
  } as ReturnType<typeof useFactoryGraphDraftState>;
}
