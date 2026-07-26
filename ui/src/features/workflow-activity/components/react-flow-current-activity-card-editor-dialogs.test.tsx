import { fireEvent, render, screen } from "@testing-library/react";

import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";

vi.mock(
  "../../factory-graph-editor/components/add-dialog/factory-graph-editor-add-dialog",
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
  "../../factory-graph-editor/components/controls/factory-graph-editor-controls",
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
  "../../factory-graph-editor/components/dialogs/factory-graph-editor-leave-dialog",
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

vi.mock("../../import/components/dashboard-import-preview-dialog", () => ({
  FactoryImportPreviewDialog: ({
    onCancel,
    onConfirm,
    previewState,
  }: {
    onCancel: () => void;
    onConfirm: (input: {
      choice: string;
      createFactoryName: string;
      existingFactoryNames: string[];
      value: { kind: string };
    }) => void;
    previewState: { status: string; value: { kind: string } };
  }) => (
    <div data-status={previewState.status} data-testid="import-preview-dialog">
      <button onClick={onCancel} type="button">
        Trigger import cancel
      </button>
      <button
        onClick={() => {
          onConfirm({
            choice: "replace_current",
            createFactoryName: "alpha",
            existingFactoryNames: ["alpha"],
            value: previewState.value,
          });
        }}
        type="button"
      >
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

function createViewModelStub(overrides: Record<string, unknown> = {}) {
  const merged = {
    addEntityDraft: { kind: "workstation" },
    addEntityErrors: { name: "Required" },
    canSaveDraft: true,
    currentFactoryDefinition: null,
    handleAddEntitySubmit: vi.fn(),
    handleCancelRemoval: vi.fn(),
    handleConfirmRemoval: vi.fn(),
    handleDiscardEditorChanges: vi.fn(),
    cancelSaveConfirmation: vi.fn(),
    documentSave: { status: "confirming" },
    handleSaveBeforeLeavingEditor: vi.fn(),
    handleSaveDraft: vi.fn(),
    isConfirmingSave: true,
    leaveDialogOpen: true,
    pendingRemovalIntent: null,
    saveEditableDefinition: { isPending: false },
    saveSummary: {
      confirmActionLabel: "Save topology",
      description: "Save pending route changes.",
    },
    setAddEntityDraft: vi.fn(),
    setAddEntityErrors: vi.fn(),
    setIsConfirmingLeaveEditor: vi.fn(),
    setIsConfirmingSave: vi.fn(),
    ...overrides,
  };
  const saveMutation = merged.saveEditableDefinition as {
    error?: unknown;
    isPending?: boolean;
  };

  return {
    ...merged,
    addControls: {
      currentFactoryDefinition: merged.currentFactoryDefinition,
      draft: merged.addEntityDraft,
      errors: merged.addEntityErrors,
      isDialogOpen: merged.addEntityDraft !== null,
      closeDialog: () => {
        (merged.setAddEntityDraft as (draft: unknown) => void)(null);
        (merged.setAddEntityErrors as (errors: unknown) => void)({});
      },
      submit: merged.handleAddEntitySubmit,
      updateDraft: (draft: unknown) => {
        (merged.setAddEntityDraft as (nextDraft: unknown) => void)(draft);
        (merged.setAddEntityErrors as (errors: unknown) => void)({});
      },
      ...((merged as { addControls?: object }).addControls ?? {}),
    },
    saveControls: {
      canSave: merged.canSaveDraft,
      cancelConfirmation: merged.cancelSaveConfirmation,
      confirmSave: merged.handleSaveDraft,
      feedback: merged.documentSave,
      isConfirming: merged.isConfirmingSave,
      saveBeforeLeaving: merged.handleSaveBeforeLeavingEditor,
      summary: merged.saveSummary,
      ...((merged as { saveControls?: object }).saveControls ?? {}),
    },
    leaveControls: {
      cancel: () => {
        (merged.setIsConfirmingLeaveEditor as (open: boolean) => void)(false);
      },
      discardChanges: merged.handleDiscardEditorChanges,
      isOpen: merged.leaveDialogOpen,
      ...((merged as { leaveControls?: object }).leaveControls ?? {}),
    },
    removalControls: {
      blockedReason:
        (merged as { blockedRemovalReason?: unknown }).blockedRemovalReason ??
        null,
      cancel: merged.handleCancelRemoval,
      confirm: merged.handleConfirmRemoval,
      deleteEdge:
        (merged as { handleEditorEdgeDelete?: unknown })
          .handleEditorEdgeDelete ?? vi.fn(),
      deleteNode:
        (merged as { handleEditorNodeDelete?: unknown })
          .handleEditorNodeDelete ?? vi.fn(),
      pendingIntent: merged.pendingRemovalIntent,
      requestSelectionNodeRemoval:
        (merged as { handleSelectionNodeDelete?: unknown })
          .handleSelectionNodeDelete ?? vi.fn(),
      ...((merged as { removalControls?: object }).removalControls ?? {}),
    },
    status: {
      isSaving: saveMutation.isPending ?? false,
      saveError: saveMutation.error ?? null,
      ...((merged as { status?: object }).status ?? {}),
    },
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: dialog wiring cases share one mocked editor surface harness.
describe("CurrentActivityGraphEditorDialogs", () => {
  it("wires import, save, discard, and add-entity dialog actions on the shared editor surface", () => {
    const viewModel = createViewModelStub();
    const imports = createImportsStub();
    const discardEditorChanges = vi.fn();

    render(
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName="alpha"
        discardEditorChanges={discardEditorChanges}
        viewModel={viewModel as never}
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
    expect(imports.activateImport).toHaveBeenCalledWith({
      choice: "replace_current",
      createFactoryName: "alpha",
      existingFactoryNames: ["alpha"],
      value: imports.importPreviewState.value,
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger import error dismiss" }),
    );
    expect(imports.clearError).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger leave cancel" }),
    );
    expect(viewModel.setIsConfirmingLeaveEditor).toHaveBeenCalledWith(false);

    fireEvent.click(
      screen.getByRole("button", { name: "Trigger leave discard" }),
    );
    expect(discardEditorChanges).toHaveBeenCalledTimes(1);
    expect(viewModel.leaveControls.discardChanges).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Trigger leave save" }));
    expect(viewModel.handleSaveBeforeLeavingEditor).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Save factory graph changes\? cancel/i,
      }),
    );
    expect(viewModel.cancelSaveConfirmation).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Save factory graph changes\? confirm/i,
      }),
    );
    expect(viewModel.handleSaveDraft).toHaveBeenCalledTimes(1);

    expect(screen.queryByText("Remove route? confirm")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Trigger add change" }));
    expect(viewModel.setAddEntityDraft).toHaveBeenCalledWith({
      kind: "resource",
    });
    expect(viewModel.setAddEntityErrors).toHaveBeenCalledWith({});

    fireEvent.click(screen.getByRole("button", { name: "Trigger add close" }));
    expect(viewModel.setAddEntityDraft).toHaveBeenCalledWith(null);

    fireEvent.click(screen.getByRole("button", { name: "Trigger add submit" }));
    expect(viewModel.handleAddEntitySubmit).toHaveBeenCalledTimes(1);
  });

  it("wires removal confirmation actions on the shared editor surface", () => {
    const viewModel = createViewModelStub({
      isConfirmingSave: false,
      leaveDialogOpen: false,
      pendingRemovalIntent: {
        confirmDescription: "This will remove 2 graph edges.",
        confirmLabel: "Delete story work-type",
        title: "Remove story work-type?",
      },
    });

    render(
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName="alpha"
        viewModel={viewModel as never}
        imports={createImportsStub() as never}
        readyImportPreviewState={null}
        shouldRenderImportPreviewDialog={false}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: /Remove story work-type\? cancel/i,
      }),
    );
    expect(viewModel.removalControls.cancel).toHaveBeenCalledTimes(1);

    fireEvent.click(
      screen.getByRole("button", {
        name: /Remove story work-type\? confirm/i,
      }),
    );
    expect(viewModel.removalControls.confirm).toHaveBeenCalledTimes(1);
  });

  it("suppresses save-dismiss callbacks while a save is pending and skips optional import chrome when disabled", () => {
    const viewModel = createViewModelStub({
      documentSave: { status: "submitting" },
      isConfirmingSave: true,
      pendingRemovalIntent: null,
      saveEditableDefinition: { isPending: true },
    });
    const imports = createImportsStub({
      dropState: { status: "idle" as const },
    });

    render(
      <CurrentActivityGraphEditorDialogs
        currentSessionFactoryName="alpha"
        viewModel={viewModel as never}
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

    expect(viewModel.setIsConfirmingLeaveEditor).not.toHaveBeenCalled();
    expect(viewModel.cancelSaveConfirmation).not.toHaveBeenCalled();
  });
});
