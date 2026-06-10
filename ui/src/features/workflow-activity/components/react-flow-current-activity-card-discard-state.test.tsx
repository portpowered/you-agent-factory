import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { semanticWorkflowDashboardSnapshot } from "../../../components/dashboard/test-fixtures";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
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

function createViewModelStub() {
  const discardPendingChanges = vi.fn();
  const discardEditorChanges = vi.fn();
  return {
    editorControls: {
      discardPendingChanges,
    },
    leaveControls: {
      discardChanges: discardEditorChanges,
    },
  };
}

describe("ReactFlowCurrentActivityCardView discard routing", () => {
  it("routes toolbar discard to the editor pending-change handler", () => {
    const viewModel = createViewModelStub();

    render(
      <ReactFlowCurrentActivityCardView
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
        viewModel={viewModel as never}
        widgetInstanceID="test"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger surface discard" }),
    );

    expect(
      viewModel.editorControls.discardPendingChanges,
    ).toHaveBeenCalledTimes(1);
  });

  it("routes leave-dialog discard to the editor discard handler", () => {
    const viewModel = createViewModelStub();

    render(
      <ReactFlowCurrentActivityCardView
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
        viewModel={viewModel as never}
        widgetInstanceID="test"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger dialog discard" }),
    );

    expect(viewModel.leaveControls.discardChanges).toHaveBeenCalledTimes(1);
  });
});
