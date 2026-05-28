import { fireEvent, render, screen } from "@testing-library/react";

import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";

vi.mock(
  "../../factory-graph-editor/components/factory-graph-editor-add-dialog",
  () => ({
    FactoryGraphEditorAddEntityDialog: ({
      isOpen,
      onChange,
      onClose,
      onSubmit,
    }: {
      isOpen: boolean;
      onChange: (draft: { kind: string }) => void;
      onClose: () => void;
      onSubmit: () => void;
    }) => (
      <div data-open={String(isOpen)} data-testid="add-entity-dialog">
        <button
          onClick={() => {
            onChange({ kind: "resource" });
          }}
          type="button"
        >
          Trigger add change
        </button>
        <button onClick={onClose} type="button">
          Trigger add close
        </button>
        <button onClick={onSubmit} type="button">
          Trigger add submit
        </button>
      </div>
    ),
  }),
);

vi.mock(
  "../../factory-graph-editor/components/factory-graph-editor-controls",
  () => ({
    FactoryGraphEditorConfirmationDialog: ({
      isOpen,
      onCancel,
      onConfirm,
      title,
    }: {
      isOpen: boolean;
      onCancel: () => void;
      onConfirm: () => void;
      title: string;
    }) => (
      <div data-open={String(isOpen)} data-testid={`confirmation-${title}`}>
        <button onClick={onCancel} type="button">
          {title} cancel
        </button>
        <button onClick={onConfirm} type="button">
          {title} confirm
        </button>
      </div>
    ),
  }),
);

vi.mock(
  "../../factory-graph-editor/components/factory-graph-editor-leave-dialog",
  () => ({
    FactoryGraphEditorLeaveDialog: ({
      isOpen,
      onCancel,
      onDiscard,
      onSave,
    }: {
      isOpen: boolean;
      onCancel: () => void;
      onDiscard: () => void;
      onSave: () => void;
    }) => (
      <div data-open={String(isOpen)} data-testid="leave-dialog">
        <button onClick={onCancel} type="button">
          Trigger leave cancel
        </button>
        <button onClick={onDiscard} type="button">
          Trigger leave discard
        </button>
        <button onClick={onSave} type="button">
          Trigger leave save
        </button>
      </div>
    ),
  }),
);

vi.mock("../../import/public", () => ({
  FactoryImportPreviewDialog: ({
    onCancel,
    onConfirm,
    previewState,
  }: {
    onCancel: () => void;
    onConfirm: () => void;
    previewState: { status: string };
  }) => (
    <div data-status={previewState.status} data-testid="import-preview-dialog">
      <button onClick={onCancel} type="button">
        Trigger import cancel
      </button>
      <button onClick={onConfirm} type="button">
        Trigger import confirm
      </button>
    </div>
  ),
}));

vi.mock("./react-flow-current-activity-card-import", () => ({
  GraphImportErrorPanel: ({
    fileName,
    onDismiss,
  }: {
    fileName: string;
    onDismiss: () => void;
  }) => (
    <div data-file-name={fileName} data-testid="graph-import-error-panel">
      <button onClick={onDismiss} type="button">
        Trigger import error dismiss
      </button>
    </div>
  ),
}));

function createEditorStub(overrides: Record<string, unknown> = {}) {
  return {
    addEntityDraft: { kind: "workstation" },
    addEntityErrors: { name: "Required" },
    canSaveDraft: true,
    currentFactoryDefinition: null,
    handleAddEntitySubmit: vi.fn(),
    handleDiscardEditorChanges: vi.fn(),
    handleSaveBeforeLeavingEditor: vi.fn(),
    handleSaveDraft: vi.fn(),
    isConfirmingSave: true,
    leaveDialogOpen: true,
    pendingRemovalIntent: null,
    saveEditableDefinition: { status: "idle" as const },
    saveSummary: { description: "Save pending route changes." },
    setAddEntityDraft: vi.fn(),
    setAddEntityErrors: vi.fn(),
    setIsConfirmingLeaveEditor: vi.fn(),
    setIsConfirmingSave: vi.fn(),
    ...overrides,
  };
}

function createImportsStub(overrides: Record<string, unknown> = {}) {
  return {
    activateImport: vi.fn(),
    activationState: { status: "idle" as const },
    clearActivationError: vi.fn(),
    clearError: vi.fn(),
    closeImportPreview: vi.fn(),
    dropState: {
      error: new Error("Import failed"),
      fileName: "factory.png",
      status: "error" as const,
    },
    importPreviewState: { status: "ready" as const, value: { kind: "png" } },
    ...overrides,
  };
}

describe("CurrentActivityGraphEditorDialogs", () => {
  it("wires import, save, discard, and add-entity dialog actions on the shared editor surface", () => {
    const editor = createEditorStub();
    const imports = createImportsStub();

    render(
      <CurrentActivityGraphEditorDialogs
        editor={editor as never}
        imports={imports as never}
        readyImportPreviewState={imports.importPreviewState}
        shouldRenderImportPreviewDialog
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger import cancel" }),
    );
    expect(imports.clearActivationError).toHaveBeenCalledTimes(1);
    expect(imports.closeImportPreview).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger import confirm" }),
    );
    expect(imports.activateImport).toHaveBeenCalledWith(
      imports.importPreviewState.value,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger import error dismiss" }),
    );
    expect(imports.clearError).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger leave cancel" }),
    );
    expect(editor.setIsConfirmingLeaveEditor).toHaveBeenCalledWith(false);

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger leave discard" }),
    );
    expect(editor.handleDiscardEditorChanges).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Trigger leave save" }));
    expect(editor.handleSaveBeforeLeavingEditor).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Save factory graph changes\? cancel/i,
      }),
    );
    expect(editor.setIsConfirmingSave).toHaveBeenCalledWith(false);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Save factory graph changes\? confirm/i,
      }),
    );
    expect(editor.handleSaveDraft).toHaveBeenCalledTimes(1);

    expect(screen.queryByText("Remove route? confirm")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Trigger add change" }));
    expect(editor.setAddEntityDraft).toHaveBeenCalledWith({ kind: "resource" });
    expect(editor.setAddEntityErrors).toHaveBeenCalledWith({});

    fireEvent.click(screen.getByRole("button", { name: "Trigger add close" }));
    expect(editor.setAddEntityDraft).toHaveBeenCalledWith(null);

    fireEvent.click(screen.getByRole("button", { name: "Trigger add submit" }));
    expect(editor.handleAddEntitySubmit).toHaveBeenCalledTimes(1);
  });

  it("suppresses save-dismiss callbacks while a save is pending and skips optional import chrome when disabled", () => {
    const editor = createEditorStub({
      isConfirmingSave: true,
      pendingRemovalIntent: null,
      saveEditableDefinition: { status: "pending" as const },
    });
    const imports = createImportsStub({
      dropState: { status: "idle" as const },
    });

    render(
      <CurrentActivityGraphEditorDialogs
        editor={editor as never}
        imports={imports as never}
        readyImportPreviewState={imports.importPreviewState}
        shouldRenderImportPreviewDialog={false}
      />,
    );

    expect(screen.queryByTestId("import-preview-dialog")).toBeNull();
    expect(screen.queryByTestId("graph-import-error-panel")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger leave cancel" }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: /Save factory graph changes\? cancel/i,
      }),
    );

    expect(editor.setIsConfirmingLeaveEditor).not.toHaveBeenCalled();
    expect(editor.setIsConfirmingSave).not.toHaveBeenCalled();
  });
});
