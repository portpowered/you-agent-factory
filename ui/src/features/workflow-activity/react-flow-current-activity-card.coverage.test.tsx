import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import {
  useCurrentEditableFactoryDefinitionDocument,
  useSaveCurrentEditableFactoryDefinition,
} from "../current-factory-definition";
import { useFactoryGraphDraftState } from "../factory-graph-editor/factory-graph-draft";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import type { CurrentActivitySelection } from "./react-flow-current-activity-card";
import { CurrentActivityGraphViewport } from "./react-flow-current-activity-card-viewport";

const {
  mockImportController,
  mockSetStoredNodePosition,
} = vi.hoisted(() => ({
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
    Background: () => <div data-testid="graph-background" />,
    Controls: () => <div data-testid="graph-controls" />,
    ReactFlow: ({
      children,
      onNodeDragStop,
    }: {
      children: React.ReactNode;
      onNodeDragStop?: (_event: unknown, node: { id: string; position: { x: number; y: number } }) => void;
    }) => (
      <div data-testid="mock-react-flow">
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
  const actual = await vi.importActual("../factory-graph-editor/factory-graph-draft");

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
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(screen.getByText("Observe mode")).toBeTruthy();
    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(
      screen.getByText("The factory has not published any workstation graph yet."),
    ).toBeTruthy();
    expect(screen.queryByTestId("mock-react-flow")).toBeNull();
  });

  it("persists node positions after drag-stop when the viewport provides a graph key", () => {
    renderViewport({ graphKey: "graph-key" });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).toHaveBeenCalledWith("graph-key", "review", {
      x: 320,
      y: 180,
    });
  });

  it("skips node-position persistence when the viewport has no graph key", () => {
    renderViewport({ graphKey: "" });

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).not.toHaveBeenCalled();
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

function renderViewport({ graphKey }: { graphKey: string }) {
  return render(
    <CurrentActivityGraphViewport
      activeTool={null}
      canInteractWithEditor={false}
      editorMode={false}
      edges={[]}
      graphKey={graphKey}
      handleNodesChange={vi.fn()}
      hasPendingChanges={false}
      imports={mockImportController}
      initialFitViewKey="full-graph"
      initialFitViewOptions={{ padding: 0.18 }}
      nodeTypes={{}}
      nodes={[]}
      onSelectTool={vi.fn()}
      setStoredNodePosition={mockSetStoredNodePosition}
    />,
  );
}
