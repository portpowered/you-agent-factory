import { fireEvent, render, screen } from "@testing-library/react";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

vi.mock("../../factory-graph-editor/components/factory-graph-editor-controls", () => ({
  FactoryGraphEditorNotice: ({
    children,
    title,
    tone,
  }: {
    children: React.ReactNode;
    title: string;
    tone: string;
  }) => (
    <section data-testid={`notice-${tone}`}>
      <h3>{title}</h3>
      <div>{children}</div>
    </section>
  ),
}));

vi.mock("./react-flow-current-activity-card-viewport", () => ({
  CurrentActivityGraphViewport: ({
    handleDiscardPendingChanges,
    handleSaveDraft,
    onAddAction,
    onAddMenuOpenChange,
    onConnect,
    onEditorEdgeClick,
    onEditorNodeClick,
    onSelectTool,
    saveDisabledReason,
  }: {
    handleDiscardPendingChanges: () => void;
    handleSaveDraft: () => void;
    onAddAction: () => void;
    onAddMenuOpenChange: (open: boolean) => void;
    onConnect: () => void;
    onEditorEdgeClick: () => void;
    onEditorNodeClick: () => void;
    onSelectTool: (tool: string) => void;
    saveDisabledReason: string | null;
  }) => (
    <div data-disabled-reason={saveDisabledReason ?? ""} data-testid="graph-viewport">
      <button onClick={handleSaveDraft} type="button">
        Trigger save confirm
      </button>
      <button onClick={handleDiscardPendingChanges} type="button">
        Trigger discard
      </button>
      <button onClick={onAddAction} type="button">
        Trigger add action
      </button>
      <button
        onClick={() => {
          onAddMenuOpenChange(true);
        }}
        type="button"
      >
        Trigger add menu
      </button>
      <button onClick={onConnect} type="button">
        Trigger connect
      </button>
      <button onClick={onEditorEdgeClick} type="button">
        Trigger edge delete
      </button>
      <button onClick={onEditorNodeClick} type="button">
        Trigger node delete
      </button>
      <button
        onClick={() => {
          onSelectTool("connect");
        }}
        type="button"
      >
        Trigger tool select
      </button>
    </div>
  ),
}));

function createEditorStub(overrides: Record<string, unknown> = {}) {
  return {
    activeTool: "connect",
    addMenuActions: [],
    addMenuOpen: false,
    blockedRemovalReason: "Delete the pending route from the connected state instead.",
    canInteractWithEditor: true,
    canSaveDraft: true,
    connectionNotice: "Only workstation-to-work-state routes are supported.",
    draftState: { hasChanges: true },
    editorMode: true,
    handleAddEntityAction: vi.fn(),
    handleDiscardPendingChanges: vi.fn(),
    handleEditorConnect: vi.fn(),
    handleEditorEdgeDelete: vi.fn(),
    handleEditorNodeDelete: vi.fn(),
    hasActiveWork: true,
    isStaleDraft: true,
    saveBlockedReason: "Stop active work before saving this draft.",
    saveEditableDefinition: {
      error: new Error("Save failed"),
      status: "success" as const,
    },
    setActiveTool: vi.fn(),
    setAddMenuOpen: vi.fn(),
    setIsConfirmingSave: vi.fn(),
    ...overrides,
  };
}

function createGraphStub() {
  return {
    edges: [],
    graphKey: "graph-key",
    handleNodesChange: vi.fn(),
    initialFitViewKey: "full-graph",
    initialFitViewOptions: { padding: 0.18 },
    nodes: [],
    setStoredNodePosition: vi.fn(),
  };
}

describe("CurrentActivityGraphSurface", () => {
  it("renders the empty state when no topology is loaded outside editor mode", () => {
    const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);
    snapshot.topology.workstation_node_ids = [];

    render(
      <CurrentActivityGraphSurface
        editor={createEditorStub({ editorMode: false }) as never}
        graph={createGraphStub() as never}
        imports={{} as never}
        snapshot={snapshot}
      />,
    );

    expect(screen.getByText("No workflow topology loaded")).toBeTruthy();
    expect(
      screen.getByText(
        "The factory has not published any workstation graph yet.",
      ),
    ).toBeTruthy();
    expect(screen.queryByTestId("graph-viewport")).toBeNull();
  });

  it("renders shared-surface notices and forwards viewport editor actions", () => {
    const editor = createEditorStub();
    const graph = createGraphStub();

    render(
      <CurrentActivityGraphSurface
        editor={editor as never}
        graph={graph as never}
        imports={{} as never}
        snapshot={semanticWorkflowDashboardSnapshot}
      />,
    );

    expect(screen.getByText("Removal blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Delete the pending route from the connected state instead.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Connection blocked")).toBeTruthy();
    expect(
      screen.getByText("Only workstation-to-work-state routes are supported."),
    ).toBeTruthy();
    expect(screen.getByText("Topology edits are blocked")).toBeTruthy();
    expect(
      screen.getByText(
        "Save is unavailable while active work is still running in the current factory.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("A newer factory definition is available"),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Refresh or discard the current draft before saving so you do not overwrite a newer topology version.",
      ),
    ).toBeTruthy();
    expect(screen.getByText("Topology save failed")).toBeTruthy();
    expect(screen.getByText("Save failed")).toBeTruthy();
    expect(screen.queryByText("Topology saved")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Trigger save confirm" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger discard" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger add action" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger add menu" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger connect" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger edge delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger node delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Trigger tool select" }));

    expect(editor.setIsConfirmingSave).toHaveBeenCalledWith(true);
    expect(editor.handleDiscardPendingChanges).toHaveBeenCalledTimes(1);
    expect(editor.handleAddEntityAction).toHaveBeenCalledTimes(1);
    expect(editor.setAddMenuOpen).toHaveBeenCalledWith(true);
    expect(editor.handleEditorConnect).toHaveBeenCalledTimes(1);
    expect(editor.handleEditorEdgeDelete).toHaveBeenCalledTimes(1);
    expect(editor.handleEditorNodeDelete).toHaveBeenCalledTimes(1);
    expect(editor.setActiveTool).toHaveBeenCalledWith("connect");
  });
});
