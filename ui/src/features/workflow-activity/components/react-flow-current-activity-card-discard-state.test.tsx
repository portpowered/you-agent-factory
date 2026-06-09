import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import { ReactFlowCurrentActivityCardView } from "./react-flow-current-activity-card";

vi.mock("./react-flow-current-activity-card-surface", () => ({
  CurrentActivityGraphSurface: ({
    discardPendingChanges,
  }: {
    discardPendingChanges?: () => void;
  }) => (
    <button onClick={discardPendingChanges} type="button">
      Trigger surface discard
    </button>
  ),
}));

vi.mock("./react-flow-current-activity-card-editor-dialogs", () => ({
  CurrentActivityGraphEditorDialogs: ({
    discardEditorChanges,
  }: {
    discardEditorChanges?: () => void;
  }) => (
    <button onClick={discardEditorChanges} type="button">
      Trigger dialog discard
    </button>
  ),
}));

vi.mock("./react-flow-current-activity-card-save-notifications", () => ({
  CurrentActivityGraphSaveNotifications: () => null,
}));

vi.mock("../hooks/react-flow-current-activity-card-graph-view-model", () => ({
  useCurrentActivityGraphViewModel: () => ({
    edges: [],
    graphKey: "graph-key",
    handleNodesChange: vi.fn(),
    initialFitViewKey: "full-graph",
    initialFitViewOptions: { padding: 0.18 },
    nodes: [
      {
        id: "workstation:review",
      },
    ],
    setStoredNodePosition: vi.fn(),
    storedNodePositions: {},
  }),
}));

vi.mock("../hooks/current-activity-import-controller", () => ({
  useCurrentActivityImportController: () => ({
    activateImport: vi.fn(),
    activationState: { status: "idle" },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" },
    importPreviewState: { status: "idle" },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
  }),
}));

function createImportController(): CurrentActivityImportController {
  return {
    activateImport: vi.fn(),
    activationState: { status: "idle" },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: { status: "idle" },
    importPreviewState: { status: "idle" },
    onDragEnter: vi.fn(),
    onDragLeave: vi.fn(),
    onDragOver: vi.fn(),
    onDrop: vi.fn(),
  };
}

function createEditorStub() {
  return {
    handleDiscardEditorChanges: vi.fn(),
    handleDiscardPendingChanges: vi.fn(),
  };
}

describe("ReactFlowCurrentActivityCardView discard state cleanup", () => {
  beforeEach(() => {
    useCurrentActivityGraphStore.setState({
      positionsByGraphKey: {
        "graph-key": {
          "workstation:review": { x: 240, y: 180 },
        },
      },
      viewportByGraphKey: {
        "graph-key": { x: 10, y: 20, zoom: 1.2 },
      },
    });
  });

  it("clears stored positions and viewport before toolbar discard", () => {
    const editor = createEditorStub();

    render(
      <ReactFlowCurrentActivityCardView
        editor={editor as never}
        importController={createImportController()}
        now={Date.now()}
        onSelectDoc={vi.fn()}
        onSelectResource={vi.fn()}
        onSelectStateNode={vi.fn()}
        onSelectWorkID={vi.fn()}
        onSelectWorker={vi.fn()}
        onSelectWorkType={vi.fn()}
        onSelectWorkstation={vi.fn()}
        selection={null}
        showHeaderActions={false}
        snapshot={semanticWorkflowDashboardSnapshot}
        widgetInstanceID="test"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger surface discard" }),
    );

    expect(editor.handleDiscardPendingChanges).toHaveBeenCalledTimes(1);
    expect(
      useCurrentActivityGraphStore.getState().positionsByGraphKey["graph-key"],
    ).toBeUndefined();
    expect(
      useCurrentActivityGraphStore.getState().viewportByGraphKey["graph-key"],
    ).toBeUndefined();
  });

  it("clears stored positions and viewport before leave-dialog discard", () => {
    const editor = createEditorStub();

    render(
      <ReactFlowCurrentActivityCardView
        editor={editor as never}
        importController={createImportController()}
        now={Date.now()}
        onSelectDoc={vi.fn()}
        onSelectResource={vi.fn()}
        onSelectStateNode={vi.fn()}
        onSelectWorkID={vi.fn()}
        onSelectWorker={vi.fn()}
        onSelectWorkType={vi.fn()}
        onSelectWorkstation={vi.fn()}
        selection={null}
        showHeaderActions={false}
        snapshot={semanticWorkflowDashboardSnapshot}
        widgetInstanceID="test"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger dialog discard" }),
    );

    expect(editor.handleDiscardEditorChanges).toHaveBeenCalledTimes(1);
    expect(
      useCurrentActivityGraphStore.getState().positionsByGraphKey["graph-key"],
    ).toBeUndefined();
    expect(
      useCurrentActivityGraphStore.getState().viewportByGraphKey["graph-key"],
    ).toBeUndefined();
  });
});
