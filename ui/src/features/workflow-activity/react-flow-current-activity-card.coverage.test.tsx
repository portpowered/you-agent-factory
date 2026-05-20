import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
} from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../current-factory-definition";
import { useFactoryGraphDraftState } from "../factory-graph-editor/factory-graph-draft";
import { createEmptyFactoryGraphDraft } from "../factory-graph-editor/factory-graph-draft-types";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import { useFactoryGraphConnectionController } from "./react-flow-current-activity-card-editor-connections";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

const { mockImportController, mockSetStoredNodePosition } = vi.hoisted(() => ({
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
    Controls: ({
      style,
    }: {
      style?: Record<string, string | number>;
    }) => (
      <div
        data-controls-style={JSON.stringify(style ?? null)}
        data-testid="graph-controls"
      />
    ),
    ReactFlow: ({
      children,
      isValidConnection,
      onConnect,
      onEdgeClick,
      onNodeClick,
      onNodeDragStop,
    }: {
      children: React.ReactNode;
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

vi.mock("./current-activity-import-controller", () => ({
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

vi.mock("../current-factory-definition", async () => {
  const actual = await vi.importActual("../current-factory-definition");

  return {
    ...actual,
    useCurrentEditableFactoryDefinitionDocument: vi.fn(),
    useSaveCurrentEditableFactoryDefinition: vi.fn(),
  };
});

vi.mock("../factory-graph-editor/factory-graph-draft", async () => {
  const actual = await vi.importActual(
    "../factory-graph-editor/factory-graph-draft",
  );

  return {
    ...actual,
    useFactoryGraphDraftState: vi.fn(),
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
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: unknown,
    workItem: unknown,
  ) => void;
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
    onSelectWorkItem: vi.fn(),
    onSelectWorkstation: vi.fn(),
    selection: null,
    snapshot: semanticWorkflowDashboardSnapshot,
    ...overrides,
  };
}

describe("ReactFlowCurrentActivityCard coverage", () => {
  beforeEach(() => {
    mockSetStoredNodePosition.mockReset();
    vi.mocked(useCurrentEditableFactoryDefinitionDocument).mockReturnValue({
      data: undefined,
      error: null,
      status: "pending",
    } as never);
    vi.mocked(useSaveCurrentEditableFactoryDefinition).mockReturnValue({
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
      screen.queryByRole("heading", { name: "Current activity" }),
    ).toBeNull();
    expect(screen.getByText("Observe mode")).toBeTruthy();
    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(
      screen.getByText(
        "The factory has not published any workstation graph yet.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("mock-react-flow")).toBeNull();
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
    const { container } = renderViewport({ graphKey: "graph-key" });

    const graphFrame = container.querySelector(
      '[data-dashboard-graph-frame="true"]',
    );

    expect(graphFrame).toBeTruthy();
    expect(graphFrame?.getAttribute("aria-label")).toBe(
      "Work graph viewport",
    );
    expect(screen.getByTestId("graph-background")).toBeTruthy();
    expect(screen.getByTestId("graph-controls")).toBeTruthy();
    expect(
      screen.getByTestId("graph-background").getAttribute("data-background-color"),
    ).toBe("var(--color-af-edge-muted-soft)");
    expect(
      screen.getByTestId("graph-background").getAttribute("data-background-gap"),
    ).toBe("24");
    expect(
      screen.getByTestId("graph-background").getAttribute("data-background-size"),
    ).toBe("1");
    expect(
      screen.getByTestId("graph-controls").getAttribute("data-controls-style"),
    ).toContain("\"backgroundColor\":\"rgb(from var(--color-af-surface) r g b / 0.88)\"");
    expect(
      screen.getByTestId("graph-controls").getAttribute("data-controls-style"),
    ).toContain("\"borderRadius\":8");
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
  editorMode = false,
  graphKey,
  nodes = [],
  onConnect,
  onEditorEdgeClick,
  onEditorNodeClick,
}: {
  activeTool?: "add" | "connect" | "delete" | null;
  editorMode?: boolean;
  graphKey: string;
  nodes?: Parameters<typeof CurrentActivityGraphViewport>[0]["nodes"];
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
