import { fireEvent, render, screen } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { ReactFlowCurrentActivityCard } from "./react-flow-current-activity-card";
import type { ReactFlowCurrentActivityCardProps } from "./react-flow-current-activity-card-types";

const {
  mockGraphViewModel,
  mockImportController,
  mockSetStoredNodePosition,
} = vi.hoisted(() => ({
  mockGraphViewModel: vi.fn(),
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

vi.mock("@xyflow/react", () => ({
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
}));

vi.mock("./react-flow-current-activity-card-view-model", () => ({
  useCurrentActivityGraphViewModel: mockGraphViewModel,
}));

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
    mockGraphViewModel.mockReset();
    mockGraphViewModel.mockReturnValue({
      edges: [],
      graphKey: "graph-key",
      handleNodesChange: vi.fn(),
      initialFitViewKey: "full-graph",
      initialFitViewOptions: { padding: 0.18 },
      nodeTypes: {},
      nodes: [],
      setStoredNodePosition: mockSetStoredNodePosition,
    });
  });

  it("renders the empty topology fallback when no workstation nodes exist", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.topology.workstation_node_ids = [];

    render(<ReactFlowCurrentActivityCard {...createProps({ snapshot })} />);

    expect(
      screen.getByRole("heading", { name: "Current activity" }),
    ).toBeTruthy();
    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(
      screen.getByText("The factory has not published any workstation graph yet."),
    ).toBeTruthy();
    expect(screen.queryByTestId("mock-react-flow")).toBeNull();
  });

  it("persists node positions after drag-stop when the graph view model provides a graph key", () => {
    render(<ReactFlowCurrentActivityCard {...createProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).toHaveBeenCalledWith("graph-key", "review", {
      x: 320,
      y: 180,
    });
  });

  it("skips node-position persistence when the graph view model has no graph key", () => {
    mockGraphViewModel.mockReturnValue({
      edges: [],
      graphKey: "",
      handleNodesChange: vi.fn(),
      initialFitViewKey: "full-graph",
      initialFitViewOptions: { padding: 0.18 },
      nodeTypes: {},
      nodes: [],
      setStoredNodePosition: mockSetStoredNodePosition,
    });

    render(<ReactFlowCurrentActivityCard {...createProps()} />);

    fireEvent.click(screen.getByRole("button", { name: "Trigger drag stop" }));

    expect(mockSetStoredNodePosition).not.toHaveBeenCalled();
  });
});
