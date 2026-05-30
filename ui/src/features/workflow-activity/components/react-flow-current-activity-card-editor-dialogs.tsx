import { FactoryGraphEditorAddEntityDialog } from "../../factory-graph-editor/components/factory-graph-editor-add-dialog";
import { FactoryGraphEditorConfirmationDialog } from "../../factory-graph-editor/components/factory-graph-editor-controls";
import { FactoryGraphEditorLeaveDialog } from "../../factory-graph-editor/components/factory-graph-editor-leave-dialog";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import { FactoryImportPreviewDialog } from "../../import/public";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { GraphImportErrorPanel } from "./react-flow-current-activity-card-import";

export function CurrentActivityGraphEditorDialogs({
  currentSessionFactoryName,
  editor,
  imports,
  locale,
  readyImportPreviewState,
  shouldRenderImportPreviewDialog,
}: {
  currentSessionFactoryName: string;
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  imports: CurrentActivityImportController;
  locale?: string;
  readyImportPreviewState: Extract<
    CurrentActivityImportController["importPreviewState"],
    { status: "ready" }
  > | null;
  shouldRenderImportPreviewDialog: boolean;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  return (
    <>
      {shouldRenderImportPreviewDialog && readyImportPreviewState ? (
        <FactoryImportPreviewDialog
          activationState={imports.activationState}
          currentSessionFactoryName={currentSessionFactoryName}
          locale={locale}
          onCancel={() => {
            imports.clearActivationError();
            imports.closeImportPreview();
          }}
          onConfirm={(input) => {
            void imports.activateImport(input.value);
          }}
          previewState={readyImportPreviewState}
        />
      ) : null}
      {imports.dropState.status === "error" ? (
        <GraphImportErrorPanel
          error={imports.dropState.error}
          fileName={imports.dropState.fileName}
          locale={locale}
          onDismiss={imports.clearError}
        />
      ) : null}
      <FactoryGraphEditorLeaveDialog
        canSave={editor.canSaveDraft}
        isOpen={editor.leaveDialogOpen}
        isSaving={editor.saveEditableDefinition.status === "pending"}
        locale={locale}
        onCancel={() => {
          if (editor.saveEditableDefinition.status !== "pending") {
            editor.setIsConfirmingLeaveEditor(false);
          }
        }}
        onDiscard={editor.handleDiscardEditorChanges}
        onSave={() => {
          void editor.handleSaveBeforeLeavingEditor();
        }}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={messages.saveConfirmAction}
        description={editor.saveSummary.description}
        isBusy={editor.saveEditableDefinition.status === "pending"}
        isOpen={editor.isConfirmingSave}
        onCancel={() => {
          if (editor.saveEditableDefinition.status !== "pending") {
            editor.setIsConfirmingSave(false);
          }
        }}
        onConfirm={() => {
          void editor.handleSaveDraft();
        }}
        title={messages.saveConfirmTitle}
      />
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={editor.currentFactoryDefinition}
        draft={editor.addEntityDraft}
        errors={editor.addEntityErrors}
        isOpen={editor.addEntityDraft !== null}
        locale={locale}
        onChange={(draft) => {
          editor.setAddEntityDraft(draft);
          editor.setAddEntityErrors({});
        }}
        onClose={() => {
          editor.setAddEntityDraft(null);
          editor.setAddEntityErrors({});
        }}
        onSubmit={editor.handleAddEntitySubmit}
      />
    </>
  );
}
