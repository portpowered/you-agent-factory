import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
} from "../../../components/dashboard/test-fixtures";
import {
  useCurrentFactoryDocument,
  useFactoryDocumentSave,
} from "../../current-factory-definition/public";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { useFactoryGraphConnectionController } from "../hooks/react-flow-current-activity-card-editor-connections";
import {
  type CurrentActivitySelection,
  ReactFlowCurrentActivityCard,
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
              targetHandle: "work-state-input-target",
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
              targetHandle: "work-state-input-target",
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
    useFactoryDocumentSave: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/hooks/factory-graph-draft-hook", async () => {
  const actual = await vi.importActual(
    "../../factory-graph-editor/hooks/factory-graph-draft-hook",
  );

  return {
    ...actual,
    useFactoryGraphDraftState: vi.fn(),
  };
});

vi.mock("../../factory-graph-editor/hooks/use-factory-validation", () => ({
  useFactoryValidation: () => ({
    data: { targets: [] },
    isError: false,
    isFetching: false,
    isLoading: false,
    projection: {
      handleErrorsByAnchorId: new Map(),
      nodeErrorsByNodeId: new Map(),
    },
    targets: [],
  }),
}));

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
  onSelectWorker: (workerName: string) => void;
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
    onSelectWorker: vi.fn(),
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
    vi.mocked(useFactoryDocumentSave).mockReturnValue({
      error: null,
      isPending: false,
      reset: vi.fn(),
      save: vi.fn(),
      saveAsync: vi.fn(),
    } as never);
    vi.mocked(useFactoryGraphDraftState).mockReturnValue(
      defaultDraftState as never,
    );
  });

  it("renders the empty topology fallback when no workstation nodes exist", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = undefined;
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

  it("returns an empty graph outcome when no canonical factory is available", async () => {
    const snapshot = structuredClone(singleNodeDashboardSnapshot);
    snapshot.factory = undefined;
    const onSelectStateNode = vi.fn();
    const onSelectWorkID = vi.fn();
    const onSelectWorkstation = vi.fn();
    const editor = {
      activeTool: null,
      canInteractWithEditor: false,
      draftState: defaultDraftState,
      editorMode: false,
      handleConnectionAnchorClick: vi.fn(),
      pendingConnectionSource: null,
      structuralValidation: {
        projection: {
          handleErrorsByAnchorId: new Map(),
          nodeErrorsByNodeId: new Map(),
        },
        targets: [],
      },
    };
    const { result } = renderHook(() =>
      useCurrentActivityGraphViewModel({
        editor: editor as never,
        now: Date.parse("2026-04-08T12:00:00Z"),
        onSelectStateNode,
        onSelectWorkID,
        onSelectWorkstation,
        selection: null,
        snapshot,
      }),
    );

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
      targetHandle: "work-state-input-target",
    });
    expect(onEditorEdgeClick).toHaveBeenCalledWith("edge-review-done");
    expect(onEditorNodeClick).toHaveBeenCalledWith("workstation:review");
  });

  it("rejects editor connections when the target node cannot be found", () => {
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
      ],
    });

    expect(screen.getByTestId("valid-workstation-output").textContent).toBe(
      "false",
    );
  });

  it("rejects editor connections when a participating node omits its graph kind", () => {
    renderViewport({
      activeTool: "connect",
      editorMode: true,
      graphKey: "graph-key",
      nodes: [
        {
          data: {},
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

    expect(screen.getByTestId("valid-workstation-output").textContent).toBe(
      "false",
    );
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
    const connectNodes = vi.fn(() => ({
      ok: true as const,
      value: createEmptyFactoryGraphDraft(),
    }));
    const editableGraph = createConnectionEditableGraph({ connectNodes });
    const { result } = renderHook(() =>
      useFactoryGraphConnectionController({
        activeTool: "connect",
        canInteractWithEditor: true,
        draftState: createConnectionDraftState(),
        editableGraph,
      }),
    );

    act(() => {
      result.current.handleEditorConnect({
        source: "workstation:review",
        sourceHandle: "workstation-output-source",
        target: "work-state:story:done",
        targetHandle: "work-state-input-target",
      });
    });

    expect(connectNodes).toHaveBeenCalledWith({
      sourceAnchorId: "workstation-output-source",
      sourceNodeId: "workstation:review",
      targetAnchorId: "work-state-input-target",
      targetNodeId: "work-state:story:done",
    });
    expect(result.current.connectionNotice).toBeNull();
    expect(result.current.pendingConnectionSource).toBeNull();
  });

  it("keeps controller connection attempts explicit when disabled or incompatible", () => {
    const connectNodes = vi.fn(() => ({
      message:
        "Success connections from review cannot connect to Failure on story:done.",
      ok: false as const,
      reason: "INVALID_CONNECTION" as const,
    }));
    const editableGraph = createConnectionEditableGraph({ connectNodes });
    const { rerender, result } = renderHook(
      ({ activeTool, canInteractWithEditor }) =>
        useFactoryGraphConnectionController({
          activeTool,
          canInteractWithEditor,
          draftState: createConnectionDraftState(),
          editableGraph,
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
        targetHandle: "work-state-input-target",
      });
    });
    expect(connectNodes).not.toHaveBeenCalled();

    rerender({ activeTool: "connect", canInteractWithEditor: true });
    act(() => {
      result.current.handleConnectionAnchorClick({
        anchorId: "work-state-input-target",
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
        anchorId: "work-state-input-target",
        nodeId: "work-state:story:done",
      });
    });
    expect(result.current.connectionNotice).toBe(
      "Success connections from review cannot connect to Failure on story:done.",
    );
    expect(connectNodes).toHaveBeenCalledTimes(1);

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
    <QueryClientProvider client={queryClient}>
      <DashboardSessionTestProvider>{view}</DashboardSessionTestProvider>
    </QueryClientProvider>,
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

function createConnectionDraftState() {
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
    updateDraft: vi.fn(),
  } as ReturnType<typeof useFactoryGraphDraftState>;
}

function createConnectionEditableGraph(
  actions: Partial<EditableFactoryGraphViewModel["actions"]>,
): EditableFactoryGraphViewModel {
  const draft = createEmptyFactoryGraphDraft();

  return {
    actions: {
      addNode: vi.fn(() => ({ ok: true, value: draft })),
      connectNodes: vi.fn(() => ({ ok: true, value: draft })),
      discard: vi.fn(),
      disconnectEdge: vi.fn(() => ({ ok: true, value: draft })),
      removeNode: vi.fn(() => ({ ok: true, value: draft })),
      save: vi.fn(async () => true),
      updateNodeField: vi.fn(() => ({
        ok: true,
        value: semanticWorkflowDashboardSnapshot.factory ?? {},
      })),
      ...actions,
    },
  } as EditableFactoryGraphViewModel;
}
