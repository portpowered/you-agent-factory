import { isFactoryDocumentSaveSubmitting } from "../../current-selection/base/hooks/factory-document-save-types";
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
            void imports.activateImport(input);
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
        isSaving={editor.saveEditableDefinition.isPending}
        locale={locale}
        onCancel={() => {
          if (!editor.saveEditableDefinition.isPending) {
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
        confirmLabel={editor.saveSummary.confirmActionLabel}
        description={editor.saveSummary.description}
        isBusy={
          editor.saveEditableDefinition.isPending ||
          isFactoryDocumentSaveSubmitting(editor.documentSave)
        }
        isOpen={editor.isConfirmingSave}
        onCancel={() => {
          if (
            !editor.saveEditableDefinition.isPending &&
            !isFactoryDocumentSaveSubmitting(editor.documentSave)
          ) {
            editor.cancelSaveConfirmation();
          }
        }}
        onConfirm={() => {
          void editor.handleSaveDraft();
        }}
        title={messages.saveConfirmTitle}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel={messages.leaveDialogKeepEditing}
        confirmLabel={
          editor.pendingRemovalIntent?.confirmLabel ??
          messages.removalFallbackConfirmLabel
        }
        confirmTone="destructive"
        description={
          editor.pendingRemovalIntent?.confirmDescription ??
          messages.removalFallbackConfirmDescription
        }
        isOpen={Boolean(
          editor.pendingRemovalIntent &&
            !editor.pendingRemovalIntent.ineligibleReason,
        )}
        onCancel={editor.handleCancelRemoval}
        onConfirm={editor.handleConfirmRemoval}
        title={
          editor.pendingRemovalIntent?.title ?? messages.removalFallbackTitle
        }
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
