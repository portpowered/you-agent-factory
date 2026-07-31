import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
import { DashboardSessionTestProvider } from "../../../testing/dashboard-session-test-provider";
import { settleWorkflowActivityGraphEffects } from "../../../testing/workflow-activity-test-utils";
import { useCurrentFactoryDocument } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { EditableFactoryGraphViewModel } from "../../factory-graph-editor/hooks/use-editable-factory-graph-types";
import { createEmptyFactoryGraphDraft } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import { useFactoryGraphConnectionController } from "../hooks/react-flow-current-activity-card-editor-connections";
import { useCurrentActivityGraphViewModel } from "../hooks/react-flow-current-activity-card-graph-view-model";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

type BuildGraphLayout = (
  topology: typeof semanticWorkflowDashboardSnapshot.topology,
) => Promise<GraphLayout>;

const {
  actualBuildGraphLayoutRef,
  mockBuildGraphLayout,
  mockImportController,
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
      nodes,
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
      nodes?: Array<{
        data?: Record<string, unknown>;
        id: string;
        position: { x: number; y: number };
      }>;
      onEdgeClick?: (_event: unknown, edge: { id: string }) => void;
      onNodeClick?: (_event: unknown, node: { id: string }) => void;
      onNodeDragStop?: (
        _event: unknown,
        node: {
          data?: Record<string, unknown>;
          id: string;
          position: { x: number; y: number };
        },
      ) => void;
    }) => {
      const draggedNode = nodes?.[0] ?? {
        data: { factoryGraphNodeId: "workstation:review" },
        id: "workstation:review",
        position: { x: 0, y: 0 },
      };

      return (
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
                ...draggedNode,
                position: { x: 320, y: 180 },
              })
            }
            type="button"
          >
            Trigger drag stop
          </button>
          {children}
        </div>
      );
    },
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
  getDefaultDashboardFlowAxisLegendPhaseItems: () => [],
}));

vi.mock("./react-flow-current-activity-card-import", () => ({
  GraphDropOverlay: () => <div data-testid="graph-drop-overlay" />,
  GraphImportErrorPanel: () => <div data-testid="graph-import-error-panel" />,
  graphDropStateAttribute: () => "idle",
}));

vi.mock(
  "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  async () => {
    const actual = await vi.importActual(
      "../../current-factory-definition/hooks/useCurrentFactoryDefinition",
    );

    return {
      ...actual,
      useCurrentFactoryDocument: vi.fn(),
    };
  },
);

vi.mock(
  "../../current-factory-definition/hooks/useFactoryDocumentSave",
  () => ({
    useFactoryDocumentSave: vi.fn(),
  }),
);

vi.mock(
  "../../factory-graph-editor/hooks/factory-graph-draft-hook",
  async () => {
    const actual = await vi.importActual(
      "../../factory-graph-editor/hooks/factory-graph-draft-hook",
    );

    return {
      ...actual,
      useFactoryGraphDraftState: vi.fn(),
    };
  },
);

vi.mock(
  "../../factory-graph-editor/hooks/validation/use-factory-validation",
  () => ({
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
  }),
);

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
      docs: [],
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
      docs: [],
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
  onSelectResource: (resourceName: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (workID: string) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
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
    onSelectDoc: vi.fn(),
    onSelectResource: vi.fn(),
    onSelectWorker: vi.fn(),
    onSelectWorkType: vi.fn(),
    onSelectWorkstation: vi.fn(),
    selection: null,
    snapshot: semanticWorkflowDashboardSnapshot,
    ...overrides,
  };
}

describe("ReactFlowCurrentActivityCard coverage", () => {
  const originalBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;
  const originalResizeObserver = globalThis.ResizeObserver;

  beforeEach(() => {
    mockBuildGraphLayout.mockReset();
    HTMLElement.prototype.getBoundingClientRect = () =>
      ({
        bottom: 720,
        height: 720,
        left: 0,
        right: 1280,
        top: 0,
        width: 1280,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect;
    globalThis.ResizeObserver = class {
      public constructor(private readonly callback: ResizeObserverCallback) {}

      public disconnect(): void {}

      public observe(target: Element): void {
        this.callback(
          [
            {
              contentRect:
                HTMLElement.prototype.getBoundingClientRect.call(target),
              target,
            } as ResizeObserverEntry,
          ],
          this as unknown as ResizeObserver,
        );
      }

      public unobserve(): void {}
    } as unknown as typeof ResizeObserver;
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

  afterEach(() => {
    HTMLElement.prototype.getBoundingClientRect = originalBoundingClientRect;
    globalThis.ResizeObserver = originalResizeObserver;
  });

  it("delegates observer loading to the shared topology surface", async () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.factory = undefined;
    snapshot.topology.workstation_node_ids = [];

    renderWithQueryClient(
      <ReactFlowCurrentActivityCard {...createProps({ snapshot })} />,
    );
    await settleWorkflowActivityGraphEffects();

    expect(screen.getByLabelText("Current activity")).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "No workflow topology loaded" }),
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
      graphProjection: {
        canonicalLayoutViewport: null,
        displayFactoryDefinition: null,
        graphLayout: { edges: [], height: 0, nodes: [], width: 0 },
        pendingAdditionEdgeIds: new Set<string>(),
        positionedGraphLayout: { edges: [], height: 0, nodes: [], width: 0 },
        visibleGraphEdges: [],
      },
      handleConnectionAnchorClick: vi.fn(),
      pendingConnectionSource: null,
      validationTargets: [],
    };
    const { result } = renderHook(() =>
      useCurrentActivityGraphViewModel({
        editor: editor as never,
        now: Date.parse("2026-04-08T12:00:00Z"),
        onSelectDoc: vi.fn(),
        onSelectResource: vi.fn(),
        onSelectStateNode,
        onSelectWorkID,
        onSelectWorker: vi.fn(),
        onSelectWorkType: vi.fn(),
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
  it("routes node drag-stop through the canonical layout move command", () => {
    const moveLayoutNode = vi.fn();

    renderViewport({
      editorMode: true,
      moveLayoutNode,
      nodes: [
        {
          data: {
            factoryGraphNodeId: "workstation:review",
            kind: "workstation",
          },
          id: "workstation:review",
          position: { x: 0, y: 0 },
          type: "workstation",
        },
      ],
    });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(moveLayoutNode).toHaveBeenCalledWith("workstation:review", {
      x: 320,
      y: 180,
    });
  });

  it("renders the factory graph inside the shared dashboard graph frame", () => {
    renderViewport({});

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
    ).toBe("var(--color-outline)");
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
    ).toContain('"backgroundColor":"var(--color-surface)"');
    expect(
      screen.getByTestId("graph-controls").getAttribute("data-controls-style"),
    ).toContain('"borderRadius":8');
    expect(screen.getByTestId("connection-line-style").textContent).toContain(
      '"stroke":"var(--color-primary)"',
    );
  });

  it("renders the compact editor toolbar inside the graph card without duplicate add controls", () => {
    renderViewport({
      addMenuActions: [{ id: "workstation", label: "Workstation" }],
      editorMode: true,
      onAddAction: vi.fn(),
    });

    const toolbar = screen.getByRole("region", {
      name: "Factory graph editor tools",
    });

    expect(
      within(toolbar).getAllByRole("button", { name: "Add" }),
    ).toHaveLength(1);
    expect(
      within(toolbar).getByRole("button", {
        name: "Delete, no graph items selected",
      }),
    ).toBeTruthy();
  });

  it("skips node-position persistence when no canonical layout move command is available", () => {
    const moveLayoutNode = vi.fn();

    renderViewport({
      editorMode: true,
      includeMoveLayoutNode: false,
      moveLayoutNode,
      nodes: [
        {
          data: {
            factoryGraphNodeId: "workstation:review",
            kind: "workstation",
          },
          id: "workstation:review",
          position: { x: 0, y: 0 },
          type: "workstation",
        },
      ],
    });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(moveLayoutNode).not.toHaveBeenCalled();
  });

  it("routes editor-mode graph viewport connection and selection callbacks", () => {
    const onConnect = vi.fn();
    const onEditorEdgeClick = vi.fn();

    renderViewport({
      editorMode: true,
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
    });

    expect(screen.getByTestId("valid-workstation-output").textContent).toBe(
      "true",
    );
    expect(screen.getByTestId("invalid-workstation-output").textContent).toBe(
      "false",
    );

    fireEvent.click(screen.getByRole("button", { name: "Trigger connect" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger edge click" }));

    expect(onConnect).toHaveBeenCalledWith({
      source: "workstation:review",
      sourceHandle: "workstation-output-source",
      target: "work-state:story:done",
      targetHandle: "work-state-input-target",
    });
    expect(onEditorEdgeClick).toHaveBeenCalledWith("edge-review-done");
  });

  it("routes delete-tool node clicks through the editor node callback", () => {
    const onEditorNodeClick = vi.fn();

    renderViewport({
      activeTool: "delete",
      editorMode: true,
      nodes: [
        {
          data: { kind: "workstation" },
          id: "workstation:review",
          position: { x: 0, y: 0 },
          type: "workstation",
        },
      ],
      onEditorNodeClick,
    });

    fireEvent.click(screen.getByRole("button", { name: "Trigger node click" }));

    expect(onEditorNodeClick).toHaveBeenCalledWith("workstation:review");
  });

  it("rejects editor connections when the target node cannot be found", () => {
    renderViewport({
      editorMode: true,
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
      editorMode: true,
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
      editorMode: true,
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
        activeTool: null,
        canInteractWithEditor: true,
        draftState: createConnectionDraftState(),
        editableGraph,
        hiddenNodeClasses: new Set(),
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
          hiddenNodeClasses: new Set(),
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

    rerender({ activeTool: null, canInteractWithEditor: true });
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
  includeMoveLayoutNode = true,
  moveLayoutNode = vi.fn(),
  nodes = [],
  onAddAction,
  onConnect,
  onEditorEdgeClick,
  onEditorNodeClick,
}: {
  activeTool?: "add" | "connect" | "delete" | null;
  addMenuActions?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["addControls"]["actions"];
  editorMode?: boolean;
  includeMoveLayoutNode?: boolean;
  moveLayoutNode?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["layoutControls"]["moveNode"];
  nodes?: Parameters<typeof CurrentActivityGraphViewport>[0]["nodes"];
  onAddAction?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["addControls"]["startAction"];
  onConnect?: Parameters<typeof CurrentActivityGraphViewport>[0]["onConnect"];
  onEditorEdgeClick?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["onEditorEdgeClick"];
  onEditorNodeClick?: Parameters<
    typeof CurrentActivityGraphViewport
  >[0]["onEditorNodeClick"];
}) {
  const flowContainerRef = { current: null as HTMLElement | null };

  return render(
    <CurrentActivityGraphViewport
      addControls={{
        actions: addMenuActions,
        isMenuOpen: false,
        setMenuOpen: vi.fn(),
        startAction: onAddAction,
      }}
      editorControls={{
        activeTool,
        canInteract: editorMode,
        discardPendingChanges: vi.fn(),
        isEditing: editorMode,
        selectTool: vi.fn(),
        toggleMode: vi.fn(),
      }}
      edges={[]}
      flowContainerRef={flowContainerRef}
      handleNodesChange={vi.fn()}
      hasPendingChanges={false}
      headingID="test-heading"
      layoutControls={{
        canMoveLayout: includeMoveLayoutNode,
        canRedo: false,
        canUndo: false,
        initialFitViewKey: "full-graph",
        initialFitViewOptions: { padding: 0.18 },
        moveNode: moveLayoutNode,
        moveNodesByDelta: vi.fn(),
        redo: vi.fn(),
        reset: vi.fn(),
        undo: vi.fn(),
        updateViewport: vi.fn(),
      }}
      imports={mockImportController}
      nodeTypes={{}}
      nodes={nodes}
      onConnect={onConnect}
      onEditorEdgeClick={onEditorEdgeClick}
      onEditorNodeClick={onEditorNodeClick}
      saveControls={{ canSave: false, requestConfirmation: vi.fn() }}
      visibilityControls={{
        hiddenNodeClasses: new Set(),
        isDirty: false,
        isMenuOpen: false,
        preset: "all",
        resetPreferences: vi.fn(),
        setMenuOpen: vi.fn(),
        setPreset: vi.fn(),
        toggleHiddenNodeClass: vi.fn(),
      }}
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
